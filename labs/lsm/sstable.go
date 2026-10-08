package lsm

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
)

// SSTable file layout:
//
//	[data records, sorted by key]
//	[bloom filter]
//	[sparse index: every indexEvery-th key → offset]
//	footer: bloomOff u64 | indexOff u64 | count u64 | magic u32
//
// record: keyLen u32 | valLen u32 | tombstone u8 | key | value
const (
	indexEvery = 16
	footerSize = 8 + 8 + 8 + 4
	magic      = 0x15A7AB1E
)

type record struct {
	key       string
	value     []byte
	tombstone bool
}

type indexEntry struct {
	key    string
	offset int64
}

type sstable struct {
	path     string
	f        *os.File
	bloom    *bloom
	index    []indexEntry
	dataEnd  int64
	count    int
	minKey   string
	maxKey   string
}

// writeSSTable writes sorted records to path.
func writeSSTable(path string, recs []record) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)

	bf := newBloom(len(recs), 0.01)
	var index []indexEntry
	var off int64
	for i, r := range recs {
		if i%indexEvery == 0 {
			index = append(index, indexEntry{r.key, off})
		}
		bf.add([]byte(r.key))
		n, err := writeRecord(w, r)
		if err != nil {
			return err
		}
		off += n
	}

	bloomOff := off
	bb := bf.marshal()
	w.Write(bb)
	indexOff := bloomOff + int64(len(bb))
	for _, e := range index {
		var hdr [12]byte
		binary.LittleEndian.PutUint32(hdr[0:4], uint32(len(e.key)))
		binary.LittleEndian.PutUint64(hdr[4:12], uint64(e.offset))
		w.Write(hdr[:])
		w.WriteString(e.key)
	}
	var footer [footerSize]byte
	binary.LittleEndian.PutUint64(footer[0:8], uint64(bloomOff))
	binary.LittleEndian.PutUint64(footer[8:16], uint64(indexOff))
	binary.LittleEndian.PutUint64(footer[16:24], uint64(len(recs)))
	binary.LittleEndian.PutUint32(footer[24:28], magic)
	w.Write(footer[:])
	if err := w.Flush(); err != nil {
		return err
	}
	return f.Sync()
}

func writeRecord(w io.Writer, r record) (int64, error) {
	var hdr [9]byte
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(len(r.key)))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(r.value)))
	if r.tombstone {
		hdr[8] = 1
	}
	if _, err := w.Write(hdr[:]); err != nil {
		return 0, err
	}
	if _, err := io.WriteString(w, r.key); err != nil {
		return 0, err
	}
	if _, err := w.Write(r.value); err != nil {
		return 0, err
	}
	return int64(9 + len(r.key) + len(r.value)), nil
}

func readRecordFrom(r io.Reader) (record, int64, error) {
	var hdr [9]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return record{}, 0, err
	}
	klen := binary.LittleEndian.Uint32(hdr[0:4])
	vlen := binary.LittleEndian.Uint32(hdr[4:8])
	buf := make([]byte, klen+vlen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return record{}, 0, err
	}
	return record{key: string(buf[:klen]), value: buf[klen:], tombstone: hdr[8] == 1},
		int64(9 + klen + vlen), nil
}

func openSSTable(path string) (*sstable, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if st.Size() < footerSize {
		f.Close()
		return nil, fmt.Errorf("sstable %s too small", path)
	}
	var footer [footerSize]byte
	if _, err := f.ReadAt(footer[:], st.Size()-footerSize); err != nil {
		f.Close()
		return nil, err
	}
	if binary.LittleEndian.Uint32(footer[24:28]) != magic {
		f.Close()
		return nil, fmt.Errorf("sstable %s: bad magic", path)
	}
	bloomOff := int64(binary.LittleEndian.Uint64(footer[0:8]))
	indexOff := int64(binary.LittleEndian.Uint64(footer[8:16]))
	count := int(binary.LittleEndian.Uint64(footer[16:24]))

	bb := make([]byte, indexOff-bloomOff)
	if _, err := f.ReadAt(bb, bloomOff); err != nil {
		f.Close()
		return nil, err
	}
	ib := make([]byte, st.Size()-footerSize-indexOff)
	if _, err := f.ReadAt(ib, indexOff); err != nil {
		f.Close()
		return nil, err
	}
	var index []indexEntry
	for len(ib) > 0 {
		klen := binary.LittleEndian.Uint32(ib[0:4])
		off := int64(binary.LittleEndian.Uint64(ib[4:12]))
		index = append(index, indexEntry{string(ib[12 : 12+klen]), off})
		ib = ib[12+klen:]
	}
	t := &sstable{path: path, f: f, bloom: unmarshalBloom(bb), index: index, dataEnd: bloomOff, count: count}
	if len(index) > 0 {
		t.minKey = index[0].key
		all, err := t.all()
		if err != nil {
			f.Close()
			return nil, err
		}
		t.maxKey = all[len(all)-1].key
	}
	return t, nil
}

// get returns (record, found). It uses the bloom filter, then binary-searches
// the sparse index and scans at most indexEvery records.
func (t *sstable) get(key string) (record, bool, error) {
	if t.count == 0 || key < t.minKey || key > t.maxKey || !t.bloom.mayContain([]byte(key)) {
		return record{}, false, nil
	}
	i := sort.Search(len(t.index), func(i int) bool { return t.index[i].key > key }) - 1
	if i < 0 {
		return record{}, false, nil
	}
	start := t.index[i].offset
	end := t.dataEnd
	if i+1 < len(t.index) {
		end = t.index[i+1].offset
	}
	buf := make([]byte, end-start)
	if _, err := t.f.ReadAt(buf, start); err != nil {
		return record{}, false, err
	}
	r := bytes.NewReader(buf)
	for {
		rec, _, err := readRecordFrom(r)
		if err == io.EOF {
			return record{}, false, nil
		}
		if err != nil {
			return record{}, false, err
		}
		if rec.key == key {
			return rec, true, nil
		}
		if rec.key > key {
			return record{}, false, nil
		}
	}
}

// all reads every record in key order.
func (t *sstable) all() ([]record, error) {
	r := bufio.NewReader(io.NewSectionReader(t.f, 0, t.dataEnd))
	recs := make([]record, 0, t.count)
	for {
		rec, _, err := readRecordFrom(r)
		if err == io.EOF {
			return recs, nil
		}
		if err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
}

func (t *sstable) close() error { return t.f.Close() }
