package lsm

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func openDB(t *testing.T, dir string, memBytes int) *DB {
	t.Helper()
	db, err := Open(dir, Options{MemtableBytes: memBytes})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestGetAcrossMemtableAndSSTables(t *testing.T) {
	db := openDB(t, t.TempDir(), 512) // tiny memtable → many flushes
	defer db.Close()
	for i := 0; i < 500; i++ {
		must(t, db.Put(fmt.Sprintf("user:%04d", i), []byte(fmt.Sprintf("v%d", i))))
	}
	if n := len(db.Stats().SSTables); n < 5 {
		t.Fatalf("expected several SSTables, got %d", n)
	}
	for _, i := range []int{0, 1, 17, 250, 499} {
		v, err := db.Get(fmt.Sprintf("user:%04d", i))
		if err != nil || string(v) != fmt.Sprintf("v%d", i) {
			t.Fatalf("user:%04d = %q, %v", i, v, err)
		}
	}
	if _, err := db.Get("user:9999"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestNewerVersionsAndTombstonesShadowOlder(t *testing.T) {
	db := openDB(t, t.TempDir(), 1<<20)
	defer db.Close()
	must(t, db.Put("k", []byte("old")))
	must(t, db.Put("gone", []byte("x")))
	must(t, db.Flush()) // SSTable 1 has old values
	must(t, db.Put("k", []byte("new")))
	must(t, db.Delete("gone"))
	must(t, db.Flush()) // SSTable 2 has the newer value + tombstone

	if v, _ := db.Get("k"); string(v) != "new" {
		t.Fatalf("want new, got %q", v)
	}
	if _, err := db.Get("gone"); !errors.Is(err, ErrNotFound) {
		t.Fatal("tombstone in newer SSTable must hide the older value")
	}
}

func TestWALRecovery(t *testing.T) {
	dir := t.TempDir()
	db := openDB(t, dir, 1<<20)
	must(t, db.Put("unflushed", []byte("survives")))
	must(t, db.Close()) // memtable never flushed; only the WAL has it

	db = openDB(t, dir, 1<<20)
	defer db.Close()
	if v, _ := db.Get("unflushed"); string(v) != "survives" {
		t.Fatalf("WAL replay failed: %q", v)
	}
}

func TestCompaction(t *testing.T) {
	dir := t.TempDir()
	db := openDB(t, dir, 256)
	for round := 0; round < 3; round++ {
		for i := 0; i < 100; i++ {
			must(t, db.Put(fmt.Sprintf("k%03d", i), []byte(fmt.Sprintf("r%d", round))))
		}
	}
	for i := 0; i < 50; i++ {
		must(t, db.Delete(fmt.Sprintf("k%03d", i)))
	}
	must(t, db.Flush())
	before := len(db.Stats().SSTables)
	must(t, db.Compact())
	st := db.Stats()
	if len(st.SSTables) != 1 || before < 2 {
		t.Fatalf("want 1 SSTable after compaction (had %d), got %v", before, st.SSTables)
	}
	must(t, db.Close())

	db = openDB(t, dir, 256)
	defer db.Close()
	rows, err := db.Scan("k000", "k999")
	must(t, err)
	if len(rows) != 50 || rows[0][0] != "k050" || rows[0][1] != "r2" {
		t.Fatalf("unexpected scan after compaction: %d rows, first=%v", len(rows), rows[0])
	}
	if files, _ := filepath.Glob(filepath.Join(dir, "*.sst")); len(files) != 1 {
		t.Fatalf("old SSTables not deleted: %v", files)
	}
}

func TestBloomFilterHasNoFalseNegatives(t *testing.T) {
	b := newBloom(10000, 0.01)
	for i := 0; i < 10000; i++ {
		b.add([]byte(fmt.Sprintf("key-%d", i)))
	}
	for i := 0; i < 10000; i++ {
		if !b.mayContain([]byte(fmt.Sprintf("key-%d", i))) {
			t.Fatalf("false negative for key-%d", i)
		}
	}
	fp := 0
	for i := 0; i < 10000; i++ {
		if b.mayContain([]byte(fmt.Sprintf("absent-%d", i))) {
			fp++
		}
	}
	if rate := float64(fp) / 10000; rate > 0.03 {
		t.Fatalf("false positive rate too high: %.3f", rate)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
