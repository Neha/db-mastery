// Package mvcc is an in-memory multi-version key-value store providing
// snapshot isolation:
//
//   - every committed write creates a new version stamped with a commit timestamp
//   - a transaction reads the newest version committed at or before its start timestamp
//   - writes are buffered privately until commit (no dirty reads)
//   - write-write conflicts are detected at commit: first committer wins
//
// Snapshot isolation still permits write skew, as the tests demonstrate.
package mvcc

import (
	"errors"
	"sort"
	"sync"
)

var (
	ErrConflict = errors.New("mvcc: write-write conflict, transaction aborted")
	ErrNotFound = errors.New("mvcc: key not found")
	ErrTxDone   = errors.New("mvcc: transaction already finished")
)

type version struct {
	value    string
	deleted  bool
	commitTS uint64
}

type Store struct {
	mu       sync.Mutex
	clock    uint64               // timestamp of the latest commit
	versions map[string][]version // per key, ordered by commitTS ascending
	active   map[*Tx]struct{}
}

func New() *Store {
	return &Store{versions: make(map[string][]version), active: make(map[*Tx]struct{})}
}

type Tx struct {
	s       *Store
	startTS uint64
	writes  map[string]*string // nil value = delete
	done    bool
}

// Begin starts a transaction whose snapshot includes every commit so far.
func (s *Store) Begin() *Tx {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx := &Tx{s: s, startTS: s.clock, writes: make(map[string]*string)}
	s.active[tx] = struct{}{}
	return tx
}

func (tx *Tx) Get(key string) (string, error) {
	if tx.done {
		return "", ErrTxDone
	}
	if v, ok := tx.writes[key]; ok { // read your own writes
		if v == nil {
			return "", ErrNotFound
		}
		return *v, nil
	}
	tx.s.mu.Lock()
	defer tx.s.mu.Unlock()
	vs := tx.s.versions[key]
	for i := len(vs) - 1; i >= 0; i-- {
		if vs[i].commitTS <= tx.startTS {
			if vs[i].deleted {
				return "", ErrNotFound
			}
			return vs[i].value, nil
		}
	}
	return "", ErrNotFound
}

func (tx *Tx) Set(key, value string) error {
	if tx.done {
		return ErrTxDone
	}
	tx.writes[key] = &value
	return nil
}

func (tx *Tx) Delete(key string) error {
	if tx.done {
		return ErrTxDone
	}
	tx.writes[key] = nil
	return nil
}

// Commit installs the buffered writes atomically, or aborts with ErrConflict
// if another transaction committed a write to any of the same keys after
// this transaction's snapshot was taken.
func (tx *Tx) Commit() error {
	if tx.done {
		return ErrTxDone
	}
	s := tx.s
	s.mu.Lock()
	defer s.mu.Unlock()
	tx.done = true
	delete(s.active, tx)

	for key := range tx.writes {
		vs := s.versions[key]
		if len(vs) > 0 && vs[len(vs)-1].commitTS > tx.startTS {
			return ErrConflict
		}
	}
	if len(tx.writes) == 0 {
		return nil
	}
	s.clock++
	for key, v := range tx.writes {
		ver := version{commitTS: s.clock}
		if v == nil {
			ver.deleted = true
		} else {
			ver.value = *v
		}
		s.versions[key] = append(s.versions[key], ver)
	}
	return nil
}

func (tx *Tx) Rollback() {
	if tx.done {
		return
	}
	tx.done = true
	tx.s.mu.Lock()
	delete(tx.s.active, tx)
	tx.s.mu.Unlock()
}

// GC drops versions no active or future transaction can ever read: for each
// key it keeps the newest version visible to the oldest active snapshot,
// plus everything newer. It returns how many versions were removed.
func (s *Store) GC() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	horizon := s.clock
	for tx := range s.active {
		if tx.startTS < horizon {
			horizon = tx.startTS
		}
	}
	removed := 0
	for key, vs := range s.versions {
		keepFrom := 0
		for i := range vs {
			if vs[i].commitTS <= horizon {
				keepFrom = i
			}
		}
		if keepFrom > 0 {
			removed += keepFrom
			vs = append([]version(nil), vs[keepFrom:]...)
		}
		if len(vs) == 1 && vs[0].deleted && vs[0].commitTS <= horizon {
			delete(s.versions, key)
			removed++
			continue
		}
		s.versions[key] = vs
	}
	return removed
}

// VersionCount returns the total number of stored versions (for observing GC).
func (s *Store) VersionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, vs := range s.versions {
		n += len(vs)
	}
	return n
}

// History returns the committed versions of key, oldest first (for teaching/debugging).
func (s *Store) History(key string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	vs := append([]version(nil), s.versions[key]...)
	sort.Slice(vs, func(i, j int) bool { return vs[i].commitTS < vs[j].commitTS })
	out := make([]string, len(vs))
	for i, v := range vs {
		if v.deleted {
			out[i] = "<deleted>"
		} else {
			out[i] = v.value
		}
	}
	return out
}
