package mvcc

import (
	"errors"
	"strconv"
	"sync"
	"testing"
)

func seed(t *testing.T, s *Store, kv map[string]string) {
	t.Helper()
	tx := s.Begin()
	for k, v := range kv {
		tx.Set(k, v)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotReadsIgnoreLaterCommits(t *testing.T) {
	s := New()
	seed(t, s, map[string]string{"balance": "100"})

	reader := s.Begin()
	writer := s.Begin()
	writer.Set("balance", "50")
	if err := writer.Commit(); err != nil {
		t.Fatal(err)
	}
	if v, _ := reader.Get("balance"); v != "100" {
		t.Fatalf("reader's snapshot changed underneath it: got %s (non-repeatable read)", v)
	}
	if v, _ := s.Begin().Get("balance"); v != "50" {
		t.Fatalf("new transaction should see the commit, got %s", v)
	}
}

func TestNoDirtyReadsAndReadYourWrites(t *testing.T) {
	s := New()
	seed(t, s, map[string]string{"x": "1"})
	t1 := s.Begin()
	t1.Set("x", "2")
	if v, _ := t1.Get("x"); v != "2" {
		t.Fatal("transaction must read its own uncommitted write")
	}
	if v, _ := s.Begin().Get("x"); v != "1" {
		t.Fatal("dirty read: saw another transaction's uncommitted write")
	}
	t1.Rollback()
	if v, _ := s.Begin().Get("x"); v != "1" {
		t.Fatal("rolled-back write became visible")
	}
}

func TestFirstCommitterWins(t *testing.T) {
	s := New()
	seed(t, s, map[string]string{"stock": "10"})
	a, b := s.Begin(), s.Begin()
	av, _ := a.Get("stock")
	bv, _ := b.Get("stock")
	n, _ := strconv.Atoi(av)
	a.Set("stock", strconv.Itoa(n-1))
	m, _ := strconv.Atoi(bv)
	b.Set("stock", strconv.Itoa(m-1))

	if err := a.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := b.Commit(); !errors.Is(err, ErrConflict) {
		t.Fatalf("lost update not prevented: want ErrConflict, got %v", err)
	}
}

func TestConcurrentIncrementsWithRetry(t *testing.T) {
	s := New()
	seed(t, s, map[string]string{"counter": "0"})
	var wg sync.WaitGroup
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				for { // retry loop: the standard pattern for optimistic concurrency
					tx := s.Begin()
					v, _ := tx.Get("counter")
					n, _ := strconv.Atoi(v)
					tx.Set("counter", strconv.Itoa(n+1))
					if tx.Commit() == nil {
						break
					}
				}
			}
		}()
	}
	wg.Wait()
	if v, _ := s.Begin().Get("counter"); v != "1000" {
		t.Fatalf("want 1000, got %s", v)
	}
}

// Write skew: both doctors check "is the other on call?" and go off call.
// They write different keys, so snapshot isolation sees no conflict.
func TestWriteSkewIsAllowedUnderSnapshotIsolation(t *testing.T) {
	s := New()
	seed(t, s, map[string]string{"alice": "on", "bob": "on"})
	t1, t2 := s.Begin(), s.Begin()
	if v, _ := t1.Get("bob"); v == "on" {
		t1.Set("alice", "off")
	}
	if v, _ := t2.Get("alice"); v == "on" {
		t2.Set("bob", "off")
	}
	if err := t1.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := t2.Commit(); err != nil {
		t.Fatal(err)
	}
	tx := s.Begin()
	a, _ := tx.Get("alice")
	b, _ := tx.Get("bob")
	if a != "off" || b != "off" {
		t.Fatalf("expected write skew to occur, got alice=%s bob=%s", a, b)
	}
	t.Log("write skew happened: nobody is on call. Serializable (SSI) would abort one transaction.")
}

func TestGarbageCollection(t *testing.T) {
	s := New()
	for i := 0; i < 10; i++ {
		seed(t, s, map[string]string{"k": strconv.Itoa(i)})
	}
	old := s.Begin() // pins the snapshot at version 9
	for i := 10; i < 20; i++ {
		seed(t, s, map[string]string{"k": strconv.Itoa(i)})
	}
	if s.VersionCount() != 20 {
		t.Fatalf("want 20 versions, got %d", s.VersionCount())
	}
	s.GC()
	if v, _ := old.Get("k"); v != "9" {
		t.Fatalf("GC removed a version still needed by an active snapshot: got %s", v)
	}
	if got := s.VersionCount(); got != 11 {
		t.Fatalf("want 11 versions retained (1 for old snapshot + 10 newer), got %d", got)
	}
	old.Rollback()
	s.GC()
	if got := s.VersionCount(); got != 1 {
		t.Fatalf("want 1 version after the old snapshot ends, got %d", got)
	}
}
