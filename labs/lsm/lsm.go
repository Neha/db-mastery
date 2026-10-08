// Package lsm is a teaching-sized Log-Structured Merge tree:
//
//	Put/Delete → WAL (durability) → memtable (in memory)
//	memtable full → flush to an immutable, sorted SSTable (with bloom filter + sparse index)
//	Get → memtable → SSTables newest-to-oldest (first hit wins; tombstones hide older values)
//	Compact → merge all SSTables into one, dropping shadowed versions and tombstones
package lsm

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var ErrNotFound = errors.New("lsm: key not found")

type Options struct {
	// MemtableBytes is the approximate memtable size that triggers a flush.
	MemtableBytes int
}

type DB struct {
	mu       sync.RWMutex
	dir      string
	opts     Options
	mem      map[string]record
	memBytes int
	wal      *os.File
	tables   []*sstable // newest first
	nextSeq  int
}

func Open(dir string, opts Options) (*DB, error) {
	if opts.MemtableBytes <= 0 {
		opts.MemtableBytes = 1 << 20
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	db := &DB{dir: dir, opts: opts, mem: make(map[string]record), nextSeq: 1}

	paths, _ := filepath.Glob(filepath.Join(dir, "*.sst"))
	sort.Strings(paths) // zero-padded sequence numbers sort chronologically
	for _, p := range paths {
		t, err := openSSTable(p)
		if err != nil {
			return nil, err
		}
		db.tables = append([]*sstable{t}, db.tables...)
		var seq int
		fmt.Sscanf(filepath.Base(p), "%06d.sst", &seq)
		if seq >= db.nextSeq {
			db.nextSeq = seq + 1
		}
	}
	if err := db.replayWAL(); err != nil {
		return nil, err
	}
	return db, nil
}

func (db *DB) walPath() string { return filepath.Join(db.dir, "wal.log") }

func (db *DB) replayWAL() error {
	f, err := os.OpenFile(db.walPath(), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	r := bufio.NewReader(f)
	var good int64
	for {
		rec, n, err := readRecordFrom(r)
		if err != nil {
			if err != io.EOF {
				// torn tail from a crash: discard it
				f.Truncate(good)
			}
			break
		}
		db.applyMem(rec)
		good += n
	}
	if _, err := f.Seek(good, io.SeekStart); err != nil {
		return err
	}
	db.wal = f
	return nil
}

func (db *DB) applyMem(r record) {
	if old, ok := db.mem[r.key]; ok {
		db.memBytes -= len(old.key) + len(old.value)
	}
	db.mem[r.key] = r
	db.memBytes += len(r.key) + len(r.value)
}

func (db *DB) write(r record) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if _, err := writeRecord(db.wal, r); err != nil {
		return err
	}
	db.applyMem(r)
	if db.memBytes >= db.opts.MemtableBytes {
		return db.flushLocked()
	}
	return nil
}

func (db *DB) Put(key string, value []byte) error {
	if key == "" {
		return errors.New("lsm: empty key")
	}
	return db.write(record{key: key, value: value})
}

// Delete writes a tombstone. The key's older versions in SSTables are
// hidden immediately and physically removed at compaction.
func (db *DB) Delete(key string) error {
	return db.write(record{key: key, tombstone: true})
}

func (db *DB) Get(key string) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if r, ok := db.mem[key]; ok {
		if r.tombstone {
			return nil, ErrNotFound
		}
		return r.value, nil
	}
	for _, t := range db.tables {
		r, ok, err := t.get(key)
		if err != nil {
			return nil, err
		}
		if ok {
			if r.tombstone {
				return nil, ErrNotFound
			}
			return r.value, nil
		}
	}
	return nil, ErrNotFound
}

// Scan returns live key/value pairs with start <= key < end, in order.
func (db *DB) Scan(start, end string) ([][2]string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	merged := make(map[string]record)
	// oldest → newest so newer versions overwrite older ones
	for i := len(db.tables) - 1; i >= 0; i-- {
		recs, err := db.tables[i].all()
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			if r.key >= start && r.key < end {
				merged[r.key] = r
			}
		}
	}
	for k, r := range db.mem {
		if k >= start && k < end {
			merged[k] = r
		}
	}
	var out [][2]string
	for k, r := range merged {
		if !r.tombstone {
			out = append(out, [2]string{k, string(r.value)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out, nil
}

// Flush forces the memtable to disk as a new SSTable.
func (db *DB) Flush() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.flushLocked()
}

func (db *DB) flushLocked() error {
	if len(db.mem) == 0 {
		return nil
	}
	recs := make([]record, 0, len(db.mem))
	for _, r := range db.mem {
		recs = append(recs, r)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].key < recs[j].key })

	path := filepath.Join(db.dir, fmt.Sprintf("%06d.sst", db.nextSeq))
	if err := writeSSTable(path, recs); err != nil {
		return err
	}
	t, err := openSSTable(path)
	if err != nil {
		return err
	}
	db.nextSeq++
	db.tables = append([]*sstable{t}, db.tables...)
	db.mem = make(map[string]record)
	db.memBytes = 0
	// The data is now durable in the SSTable, so the WAL can be reset.
	if err := db.wal.Truncate(0); err != nil {
		return err
	}
	_, err = db.wal.Seek(0, io.SeekStart)
	return err
}

// Compact merges every SSTable into one. Because it is a full merge,
// tombstones can be dropped (no older table remains that they must shadow).
func (db *DB) Compact() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if len(db.tables) < 2 {
		return nil
	}
	merged := make(map[string]record)
	for i := len(db.tables) - 1; i >= 0; i-- {
		recs, err := db.tables[i].all()
		if err != nil {
			return err
		}
		for _, r := range recs {
			merged[r.key] = r
		}
	}
	var recs []record
	for _, r := range merged {
		if !r.tombstone {
			recs = append(recs, r)
		}
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].key < recs[j].key })

	path := filepath.Join(db.dir, fmt.Sprintf("%06d.sst", db.nextSeq))
	if err := writeSSTable(path, recs); err != nil {
		return err
	}
	t, err := openSSTable(path)
	if err != nil {
		return err
	}
	db.nextSeq++
	old := db.tables
	db.tables = []*sstable{t}
	for _, o := range old {
		o.close()
		os.Remove(o.path)
	}
	return nil
}

// Stats describes the current shape of the tree.
type Stats struct {
	MemtableKeys int
	SSTables     []string
}

func (db *DB) Stats() Stats {
	db.mu.RLock()
	defer db.mu.RUnlock()
	s := Stats{MemtableKeys: len(db.mem)}
	for _, t := range db.tables {
		s.SSTables = append(s.SSTables, fmt.Sprintf("%s(%d keys)", strings.TrimSuffix(filepath.Base(t.path), ".sst"), t.count))
	}
	return s
}

func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	for _, t := range db.tables {
		t.close()
	}
	if err := db.wal.Sync(); err != nil {
		return err
	}
	return db.wal.Close()
}
