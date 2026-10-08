## What you're building

A small but real **LSM-tree**, the design behind RocksDB, LevelDB, Cassandra, ScyllaDB and CockroachDB's Pebble.

```text
Put/Delete ─▶ WAL (wal.log) ─▶ memtable (map in RAM)
                                  │ full (MemtableBytes)
                                  ▼
                         000003.sst  ← newest (immutable, sorted, bloom + sparse index)
                         000002.sst
                         000001.sst  ← oldest
Get: memtable → 000003 → 000002 → 000001   (first hit wins; tombstone = deleted)
Compact: merge all .sst → one file, drop shadowed versions + tombstones
```

| File | What's inside |
|---|---|
| `labs/lsm/lsm.go` | DB: WAL, memtable, flush, Get/Scan, compaction |
| `labs/lsm/sstable.go` | SSTable writer and reader (data, bloom filter, sparse index, footer) |
| `labs/lsm/bloom.go` | Bloom filter with double hashing |
| `labs/lsm/lsm_test.go` | Behaviour tests |

```bash
go test ./labs/lsm -v
```

## SSTable layout

```text
[ record | record | … sorted by key ]        keyLen u32 | valLen u32 | tombstone u8 | key | value
[ bloom filter bits ]                         ~10 bits/key → ~1% false positives
[ sparse index: every 16th key → offset ]
[ footer: bloomOff | indexOff | count | magic ]
```

Reading a key from an SSTable:

1. Check `minKey ≤ key ≤ maxKey`, and the **bloom filter**. If it says "definitely not here", skip the file with **zero disk reads**.
2. **Binary search** the sparse index for the block that could contain the key.
3. Read that block (16 records at most) and scan.

That's how LSM engines keep reads acceptable despite many files.

## Why each piece exists

| Piece | Problem it solves |
|---|---|
| WAL | The memtable is in RAM, so without a WAL a crash loses unflushed writes (`TestWALRecovery`) |
| Memtable | Batches random writes in memory, then flushes them as one sequential write |
| Immutable SSTables | No in-place updates means no torn pages, easy caching and simple backups |
| Tombstones | You can't delete from immutable files, so you write a marker that shadows older values (`TestNewerVersionsAndTombstonesShadowOlder`) |
| Bloom filters | Avoid reading files that don't contain the key (`TestBloomFilterHasNoFalseNegatives`) |
| Compaction | Bounds read amplification and space amplification (`TestCompaction`) |

> **Warning:** A tombstone can only be dropped when no *older* SSTable might still contain the key. This lab compacts **all** tables at once, so dropping is safe. Real leveled compaction must keep tombstones until they reach the bottom level. This is a subtle bug source in real systems (and a great interview detail).

## Measure the amplifications

Add instrumentation and observe:

- **Write amplification**: total bytes written to disk (WAL + flushes + compactions) ÷ bytes of user data.
- **Read amplification**: SSTables consulted per `Get` (count bloom-filter skips separately).
- **Space amplification**: total `.sst` size ÷ live data size, before and after `Compact()`.

## Exercises

> **Exercise 1 (leveled compaction):** Organise tables into L0 (overlapping, flushed files) and L1+ (non-overlapping key ranges, 10x larger per level). Compact L0 into L1 when L0 has 4 files. Keep tombstones unless compacting into the last level.

> **Exercise 2 (merge iterator):** Replace `Scan`'s map-based merge with a heap-based **k-way merge iterator** over sorted sources. That's how real engines stream range scans without loading everything.

> **Exercise 3 (skip list memtable):** Replace the `map` with a skip list so the memtable is always sorted (enables `Scan` without sorting and lock-free reads).

> **Exercise 4 (block cache):** Add an LRU cache of decoded SSTable blocks. Measure the hit rate for a Zipfian workload.

> **Exercise 5 (benchmark vs Bitcask):** Write 1M random keys to both stores. Compare write throughput, `Get` latency, range scan support and memory usage. Write up the trade-offs as if for a customer.

```quiz
Q: Why does an LSM-tree need a WAL in addition to SSTables?
+ Recent writes live only in the in-memory memtable until flushed; the WAL makes them durable across crashes
- SSTables can't be read without a WAL
- The WAL stores the bloom filters
- It speeds up compaction
? Memtables are volatile. The WAL is replayed on start-up to rebuild them.

Q: A key was deleted (tombstone in 000005.sst) but an older value exists in 000002.sst. When can the tombstone be discarded?
- Immediately after writing it
+ Only when compaction merges it with every older file that might contain the key (e.g. into the bottom level)
- Never
- When the memtable flushes
? Dropping the tombstone too early would resurrect the older value. That's the classic "zombie data" bug.

Q: What does a bloom filter with a 1% false positive rate guarantee?
- 1% of present keys will be reported missing
+ Absent keys are reported as "maybe present" about 1% of the time; present keys are never reported missing
- Lookups are exactly 1% slower
- 99% of keys are cached
? Bloom filters have no false negatives. False positives just cost an unnecessary block read.
```
