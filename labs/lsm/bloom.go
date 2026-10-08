package lsm

import (
	"encoding/binary"
	"hash/fnv"
	"math"
)

// bloom is a simple Bloom filter using double hashing (Kirsch–Mitzenmacher).
// It can return false positives but never false negatives.
type bloom struct {
	bits []byte
	k    uint32
}

func newBloom(n int, fpRate float64) *bloom {
	if n < 1 {
		n = 1
	}
	m := int(math.Ceil(-float64(n) * math.Log(fpRate) / (math.Ln2 * math.Ln2)))
	k := uint32(math.Max(1, math.Round(float64(m)/float64(n)*math.Ln2)))
	return &bloom{bits: make([]byte, (m+7)/8), k: k}
}

func hashes(key []byte) (uint32, uint32) {
	h := fnv.New64a()
	h.Write(key)
	sum := h.Sum64()
	return uint32(sum), uint32(sum>>32) | 1
}

func (b *bloom) add(key []byte) {
	h1, h2 := hashes(key)
	m := uint32(len(b.bits) * 8)
	for i := uint32(0); i < b.k; i++ {
		pos := (h1 + i*h2) % m
		b.bits[pos/8] |= 1 << (pos % 8)
	}
}

func (b *bloom) mayContain(key []byte) bool {
	h1, h2 := hashes(key)
	m := uint32(len(b.bits) * 8)
	for i := uint32(0); i < b.k; i++ {
		pos := (h1 + i*h2) % m
		if b.bits[pos/8]&(1<<(pos%8)) == 0 {
			return false
		}
	}
	return true
}

func (b *bloom) marshal() []byte {
	out := make([]byte, 4+len(b.bits))
	binary.LittleEndian.PutUint32(out, b.k)
	copy(out[4:], b.bits)
	return out
}

func unmarshalBloom(data []byte) *bloom {
	return &bloom{k: binary.LittleEndian.Uint32(data), bits: data[4:]}
}
