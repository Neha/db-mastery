package kvstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func open(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPutGetOverwriteDelete(t *testing.T) {
	s := open(t, filepath.Join(t.TempDir(), "data.log"))
	defer s.Close()

	must(t, s.Put("lang", []byte("go")))
	must(t, s.Put("db", []byte("postgres")))
	must(t, s.Put("db", []byte("mongodb")))

	if v, _ := s.Get("db"); string(v) != "mongodb" {
		t.Fatalf("want latest value mongodb, got %q", v)
	}
	must(t, s.Delete("lang"))
	if _, err := s.Get("lang"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
	if s.Len() != 1 {
		t.Fatalf("want 1 live key, got %d", s.Len())
	}
}

func TestRecoveryAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.log")
	s := open(t, path)
	for i := 0; i < 100; i++ {
		must(t, s.Put(fmt.Sprintf("k%03d", i), []byte(fmt.Sprintf("v%d", i))))
	}
	must(t, s.Put("k007", []byte("updated")))
	must(t, s.Delete("k042"))
	must(t, s.Close())

	s = open(t, path)
	defer s.Close()
	if v, _ := s.Get("k007"); string(v) != "updated" {
		t.Fatalf("overwrite lost after reopen: %q", v)
	}
	if _, err := s.Get("k042"); !errors.Is(err, ErrNotFound) {
		t.Fatal("delete lost after reopen: tombstone not replayed")
	}
	if s.Len() != 99 {
		t.Fatalf("want 99 keys, got %d", s.Len())
	}
}

func TestTornWriteIsTruncated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.log")
	s := open(t, path)
	must(t, s.Put("a", []byte("1")))
	must(t, s.Put("b", []byte("2")))
	must(t, s.Close())

	// Simulate a crash mid-write: append half a record of garbage.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.Write([]byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x05})
	f.Close()

	s = open(t, path)
	defer s.Close()
	if v, _ := s.Get("b"); string(v) != "2" {
		t.Fatalf("lost committed data: %q", v)
	}
	must(t, s.Put("c", []byte("3"))) // must append after the truncated tail
	must(t, s.Close())

	s = open(t, path)
	if v, _ := s.Get("c"); string(v) != "3" {
		t.Fatalf("write after recovery not readable: %q", v)
	}
}

func TestCompactionReclaimsSpace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.log")
	s := open(t, path)
	for i := 0; i < 1000; i++ {
		must(t, s.Put("counter", []byte(fmt.Sprintf("%d", i))))
	}
	must(t, s.Put("keep", []byte("me")))
	before, dead := s.Stats()
	if dead == 0 {
		t.Fatal("expected dead bytes from overwrites")
	}
	must(t, s.Compact())
	after, _ := s.Stats()
	if after >= before/10 {
		t.Fatalf("compaction did not shrink log enough: %d → %d bytes", before, after)
	}
	if v, _ := s.Get("counter"); string(v) != "999" {
		t.Fatalf("wrong value after compaction: %q", v)
	}
	must(t, s.Close())

	s = open(t, path)
	defer s.Close()
	if v, _ := s.Get("keep"); string(v) != "me" {
		t.Fatal("data lost after compaction + reopen")
	}
}

func BenchmarkPut(b *testing.B) {
	s, _ := Open(filepath.Join(b.TempDir(), "bench.log"), Options{})
	defer s.Close()
	val := make([]byte, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Put(fmt.Sprintf("key-%d", i), val)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
