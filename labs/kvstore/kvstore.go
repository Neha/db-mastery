// Package kvstore is a Bitcask-style key-value store: an append-only log on
// disk plus an in-memory hash index (keydir) mapping each key to the offset
// of its latest record.
//
// On-disk record layout (little endian):
//
//	crc32 (4) | tombstone (1) | keyLen (4) | valLen (4) | key | value
//
// The CRC covers every byte after the CRC field, so torn writes at the tail
// of the log are detected and truncated during recovery.
package kvstore

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sort"
	"sync"
)

const headerSize = 4 + 1 + 4 + 4

var (
	ErrNotFound = errors.New("kvstore: key not found")
	ErrClosed   = errors.New("kvstore: store is closed")
)

type Options struct {
	// SyncOnWrite fsyncs after every Put/Delete. Durable but slow.
	SyncOnWrite bool
}

type entry struct {
	offset int64 // start of the record
	size   int64 // total record size in bytes
}

type Store struct {
	mu     sync.RWMutex
	path   string
	opts   Options
	f      *os.File
	index  map[string]entry
	end    int64 // current end of file = next write offset
	dead   int64 // bytes occupied by overwritten/deleted records
	closed bool
}

// Open opens (or creates) the log at path and rebuilds the index by
// replaying it. A corrupt or truncated tail is cut off.
func Open(path string, opts Options) (*Store, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	s := &Store{path: path, opts: opts, f: f, index: make(map[string]entry)}
	if err := s.recover(); err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) recover() error {
	if _, err := s.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	r := bufio.NewReader(s.f)
	var off int64
	for {
		key, _, tomb, size, err := readRecord(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			// Torn or corrupt tail: keep everything before it.
			if terr := s.f.Truncate(off); terr != nil {
				return terr
			}
			break
		}
		if old, ok := s.index[string(key)]; ok {
			s.dead += old.size
		}
		if tomb {
			delete(s.index, string(key))
			s.dead += size
		} else {
			s.index[string(key)] = entry{offset: off, size: size}
		}
		off += size
	}
	s.end = off
	_, err := s.f.Seek(off, io.SeekStart)
	return err
}

func readRecord(r io.Reader) (key, val []byte, tomb bool, size int64, err error) {
	var hdr [headerSize]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		if err == io.ErrUnexpectedEOF {
			err = fmt.Errorf("truncated header: %w", err)
		}
		return
	}
	sum := binary.LittleEndian.Uint32(hdr[0:4])
	tomb = hdr[4] == 1
	klen := binary.LittleEndian.Uint32(hdr[5:9])
	vlen := binary.LittleEndian.Uint32(hdr[9:13])
	if klen > 1<<20 || vlen > 1<<30 {
		err = errors.New("implausible record lengths")
		return
	}
	buf := make([]byte, klen+vlen)
	if _, err = io.ReadFull(r, buf); err != nil {
		err = fmt.Errorf("truncated body: %w", err)
		return
	}
	h := crc32.NewIEEE()
	h.Write(hdr[4:])
	h.Write(buf)
	if h.Sum32() != sum {
		err = errors.New("checksum mismatch")
		return
	}
	return buf[:klen], buf[klen:], tomb, int64(headerSize) + int64(klen) + int64(vlen), nil
}

func encodeRecord(key, val []byte, tomb bool) []byte {
	buf := make([]byte, headerSize+len(key)+len(val))
	if tomb {
		buf[4] = 1
	}
	binary.LittleEndian.PutUint32(buf[5:9], uint32(len(key)))
	binary.LittleEndian.PutUint32(buf[9:13], uint32(len(val)))
	copy(buf[headerSize:], key)
	copy(buf[headerSize+len(key):], val)
	binary.LittleEndian.PutUint32(buf[0:4], crc32.ChecksumIEEE(buf[4:]))
	return buf
}

func (s *Store) append(rec []byte) (int64, error) {
	off := s.end
	if _, err := s.f.WriteAt(rec, off); err != nil {
		return 0, err
	}
	s.end += int64(len(rec))
	if s.opts.SyncOnWrite {
		if err := s.f.Sync(); err != nil {
			return 0, err
		}
	}
	return off, nil
}

func (s *Store) Put(key string, value []byte) error {
	if key == "" {
		return errors.New("kvstore: empty key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	rec := encodeRecord([]byte(key), value, false)
	off, err := s.append(rec)
	if err != nil {
		return err
	}
	if old, ok := s.index[key]; ok {
		s.dead += old.size
	}
	s.index[key] = entry{offset: off, size: int64(len(rec))}
	return nil
}

func (s *Store) Get(key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, ErrClosed
	}
	e, ok := s.index[key]
	if !ok {
		return nil, ErrNotFound
	}
	buf := make([]byte, e.size)
	if _, err := s.f.ReadAt(buf, e.offset); err != nil {
		return nil, err
	}
	k, v, _, _, err := readRecord(bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	if string(k) != key {
		return nil, fmt.Errorf("kvstore: index points at wrong key %q", k)
	}
	return v, nil
}

// Delete appends a tombstone so the deletion survives restarts.
func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	old, ok := s.index[key]
	if !ok {
		return ErrNotFound
	}
	rec := encodeRecord([]byte(key), nil, true)
	if _, err := s.append(rec); err != nil {
		return err
	}
	delete(s.index, key)
	s.dead += old.size + int64(len(rec))
	return nil
}

func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.index))
	for k := range s.index {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.index)
}

// Stats returns the log size and how many of those bytes are garbage.
func (s *Store) Stats() (size, dead int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.end, s.dead
}

// Compact rewrites only live records into a new file and atomically swaps it in.
func (s *Store) Compact() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	tmpPath := s.path + ".compact"
	tmp, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	newIndex := make(map[string]entry, len(s.index))
	w := bufio.NewWriter(tmp)
	var off int64
	for key, e := range s.index {
		buf := make([]byte, e.size)
		if _, err := s.f.ReadAt(buf, e.offset); err != nil {
			tmp.Close()
			return err
		}
		if _, err := w.Write(buf); err != nil {
			tmp.Close()
			return err
		}
		newIndex[key] = entry{offset: off, size: e.size}
		off += e.size
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		tmp.Close()
		return err
	}
	s.f.Close()
	s.f, s.index, s.end, s.dead = tmp, newIndex, off, 0
	return nil
}

func (s *Store) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.f.Sync()
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if err := s.f.Sync(); err != nil {
		s.f.Close()
		return err
	}
	return s.f.Close()
}