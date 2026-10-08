// Package hashring implements consistent hashing with virtual nodes, the
// partitioning scheme used by Cassandra, DynamoDB (originally), Riak and many
// caches. Compare it with Modulo, which remaps almost every key on resize.
package hashring

import (
	"crypto/sha1"
	"encoding/binary"
	"sort"
	"strconv"
	"sync"
)

type Ring struct {
	mu     sync.RWMutex
	vnodes int
	hashes []uint32          // sorted positions on the ring
	owner  map[uint32]string // position → physical node
	nodes  map[string]struct{}
}

// New creates a ring where each physical node owns vnodes positions.
func New(vnodes int) *Ring {
	if vnodes < 1 {
		vnodes = 1
	}
	return &Ring{vnodes: vnodes, owner: make(map[uint32]string), nodes: make(map[string]struct{})}
}

func hash(s string) uint32 {
	sum := sha1.Sum([]byte(s))
	return binary.BigEndian.Uint32(sum[:4])
}

func (r *Ring) Add(node string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nodes[node]; ok {
		return
	}
	r.nodes[node] = struct{}{}
	for i := 0; i < r.vnodes; i++ {
		h := hash(node + "#" + strconv.Itoa(i))
		r.owner[h] = node
		r.hashes = append(r.hashes, h)
	}
	sort.Slice(r.hashes, func(i, j int) bool { return r.hashes[i] < r.hashes[j] })
}

func (r *Ring) Remove(node string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nodes[node]; !ok {
		return
	}
	delete(r.nodes, node)
	kept := r.hashes[:0]
	for _, h := range r.hashes {
		if r.owner[h] == node {
			delete(r.owner, h)
			continue
		}
		kept = append(kept, h)
	}
	r.hashes = kept
}

// Get returns the node owning key: the first vnode clockwise from hash(key).
func (r *Ring) Get(key string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.hashes) == 0 {
		return ""
	}
	return r.owner[r.hashes[r.search(hash(key))]]
}

// GetN returns up to n distinct nodes for key, walking clockwise. This is
// how Dynamo-style systems pick a key's replica set (its "preference list").
func (r *Ring) GetN(key string, n int) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.hashes) == 0 {
		return nil
	}
	if n > len(r.nodes) {
		n = len(r.nodes)
	}
	out := make([]string, 0, n)
	seen := make(map[string]bool, n)
	for i, start := 0, r.search(hash(key)); len(out) < n && i < len(r.hashes); i++ {
		node := r.owner[r.hashes[(start+i)%len(r.hashes)]]
		if !seen[node] {
			seen[node] = true
			out = append(out, node)
		}
	}
	return out
}

func (r *Ring) search(h uint32) int {
	i := sort.Search(len(r.hashes), func(i int) bool { return r.hashes[i] >= h })
	if i == len(r.hashes) {
		i = 0 // wrap around the ring
	}
	return i
}

func (r *Ring) Nodes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.nodes))
	for n := range r.nodes {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Modulo is naive hash(key) % N partitioning, for comparison.
type Modulo struct{ Nodes []string }

func (m Modulo) Get(key string) string {
	if len(m.Nodes) == 0 {
		return ""
	}
	return m.Nodes[hash(key)%uint32(len(m.Nodes))]
}
