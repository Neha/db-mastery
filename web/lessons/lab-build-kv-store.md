## What you're building

**Bitcask** is the storage engine Riak used. It's the simplest *real* database design: an **append-only log** on disk plus an **in-memory hash map** (the "keydir") from each key to the offset of its latest record. It's fast, crash-safe and around 300 lines of Go.

```text
Put("db","mongo")                       keydir (RAM)
          │                             ┌───────────────┬────────┐
          ▼                             │ "lang"        │ off 0  │
data.log: [lang=go][db=postgres][db=mongo][lang=⊘]      │ "db"          │ off 52 │
          off 0     off 23       off 52    off 76 ─── tombstone removes "lang"
```

Source: `labs/kvstore/kvstore.go`. Tests: `labs/kvstore/kvstore_test.go`.

```bash
go test ./labs/kvstore -v
go test ./labs/kvstore -bench=. -benchmem
```

## Record format

```text
crc32 (4) | tombstone (1) | keyLen (4) | valLen (4) | key | value
```

- **CRC32** over everything after it detects corrupted or partially written records.
- **Tombstone** flag marks deletes, because you can't remove bytes from an append-only log.
- Lengths make the log self-describing, so it can be replayed sequentially.

## The four core operations

| Operation | Implementation | Cost |
|---|---|---|
| `Put` | Append record, update keydir | 1 sequential write |
| `Get` | Look up offset in keydir, `ReadAt` | 1 random read (often from the OS page cache) |
| `Delete` | Append tombstone, remove from keydir | 1 sequential write |
| `Open` (recovery) | Replay the whole log, rebuilding keydir | O(log size) |

## Crash recovery

```go
for {
	key, _, tomb, size, err := readRecord(r)
	if err == io.EOF { break }
	if err != nil {
		// Torn or corrupt tail: keep everything before it.
		s.f.Truncate(off)
		break
	}
	// apply to index …
	off += size
}
```

If the process dies mid-write, the last record is incomplete, so its CRC or length check fails. Recovery **truncates the torn tail** and everything before it is intact. `TestTornWriteIsTruncated` simulates this by appending garbage bytes.

> **Key idea:** This is the WAL principle in its purest form: the log *is* the database, and the index is a rebuildable cache.

## Compaction

Overwrites and deletes leave **dead records**. `Compact()` writes only live records to a new file, `fsync`s it, and atomically `rename`s it over the old one. That's crash-safe because `rename` is atomic on POSIX filesystems.

`TestCompactionReclaimsSpace` writes the same key 1,000 times. The log shrinks by more than 10x after compaction.

## Durability knob

`Options{SyncOnWrite: true}` calls `fsync` after each write: fully durable, but much slower. Run the benchmark both ways and compare. This is the same trade-off as Postgres `synchronous_commit` or MongoDB `j:true`.

## Bitcask's trade-offs (be ready to discuss)

| Strength | Weakness |
|---|---|
| Very fast writes (sequential) | **All keys must fit in RAM** |
| One disk seek per read | No range queries (hash index is unordered) |
| Simple crash recovery | Start-up time grows with log size (hint files fix this) |
| Easy backups (copy immutable files) | Compaction needs spare disk space |

The weaknesses lead directly to the **LSM-tree** (next lab): sort keys so ranges work and keys don't all need to be in RAM.

## Exercises

> **Exercise 1 (segments):** Roll over to a new segment file after 1 MB. The keydir must store `(fileID, offset)`. Compaction merges old immutable segments while writes continue to the active one.

> **Exercise 2 (hint files):** During compaction, also write a "hint file" (key → offset only, no values) so `Open` can rebuild the keydir without reading every value.

> **Exercise 3 (group commit):** Instead of `fsync` per write, batch writes from many goroutines and `fsync` once every 2 ms. Measure throughput with `SyncOnWrite`.

> **Exercise 4 (TTL):** Add `PutWithTTL(key, val, ttl)`. Store an expiry timestamp in the record, hide expired keys in `Get` and drop them in compaction.

> **Exercise 5 (HTTP API):** Wrap the store in a `net/http` server: `GET/PUT/DELETE /kv/{key}`. You've built a tiny Redis.

```quiz
Q: Why can Bitcask recover safely after a crash in the middle of a write?
+ Records carry lengths and a CRC, so the torn last record is detected and truncated, and earlier records are intact
- It writes every record twice
- It never crashes
- It keeps a backup copy in RAM
? An append-only log with checksums makes recovery a simple replay plus truncation.

Q: What is Bitcask's main scalability limitation?
- Writes are random I/O
+ Every key must fit in the in-memory hash index
- It can't delete keys
- It needs a B-tree
? The keydir holds all keys in RAM. LSM-trees relax this with sorted on-disk files and sparse indexes.

Q: How does compaction swap in the new file safely?
- It deletes the old file first
+ It writes and fsyncs the new file, then atomically renames it over the old one
- It edits the old file in place
- It locks the filesystem
? Rename is atomic, so a crash leaves either the old or the new complete file, never a mix.
```
