## Why the storage engine matters

The **storage engine** is the part of the database that puts bytes on disk and gets them back. Everything above it (SQL, the query planner, replication) is built on its guarantees and inherits its costs. Two families dominate: **B-trees**, which update fixed-size pages in place, and **log-structured merge trees (LSM-trees)**, which only ever append and tidy up later.

You rarely choose an engine directly. You choose Postgres, MongoDB or Cassandra and get its engine with it. But knowing the engine lets you predict how a database will behave under a workload *before* you benchmark it, and that's exactly the judgement an SA is paid for.

## Disks, pages and the buffer pool

- Databases read and write fixed-size **pages**: 8 KB in Postgres, 16 KB in InnoDB. WiredTiger uses variable-size pages that are compressed on disk, so an on-disk page is usually smaller than its in-memory form.
- **Sequential I/O beats random I/O.** On spinning disks the gap is about 100×. On SSDs it's smaller but real (more on why below), and on cloud block storage such as EBS every random page write spends an IOPS credit.
- The **buffer pool** keeps hot pages in RAM: Postgres `shared_buffers`, the InnoDB buffer pool, the WiredTiger cache. If the **working set** (the data *and indexes* you actually touch) fits, reads are memory-speed. When it stops fitting, latency falls off a cliff rather than degrading gently.

Engines differ in how they share RAM with the operating system's **page cache**:

| Engine | Own cache | OS page cache | Typical sizing |
|---|---|---|---|
| Postgres | `shared_buffers` | Used heavily (buffered I/O), so hot pages can be cached twice | ~25% of RAM for `shared_buffers`, the rest left to the OS |
| InnoDB | Buffer pool | Usually bypassed with `O_DIRECT` | 50–75% of RAM |
| WiredTiger | WiredTiger cache (uncompressed pages) | Holds *compressed* file blocks | Default is the larger of 50% of (RAM − 1 GB) or 256 MB |

> **SA angle:** "How much RAM do we need?" is one of the most common sizing questions. The answer starts with *how big is the working set, including indexes?* For MongoDB you'll hear "indexes should fit in RAM", which is the same rule stated for the most frequently touched structures.

## The simplest engine: an append-only log with a hash index

DDIA starts with the simplest possible key-value store, and so should you. Writes append `key,value` to the end of a file. An in-memory **hash map** records, for every key, the byte offset of its latest value. A read is one hash lookup plus one disk seek. This is essentially **Bitcask**, the default engine in Riak.

![Bitcask appends every write to the active segment; an in-memory hash map points each key at its latest offset. Old segments are merged in the background, keeping only the newest value per key.](diagrams/storage-engines-bitcask.svg)

- **Writes are pure sequential appends**, so they're very fast. A crash can only damage the tail, which checksums detect.
- The log is split into **segments**. Closed segments are immutable, and a background **compaction** merges them, keeping only the latest value for each key. Deletes append a special **tombstone**.
- **The catch:** every key must fit in RAM, and range queries are hopeless because the hash map has no order.

You build exactly this in the [KV-store lab](#/lesson/lab-build-kv-store).

## SSTables and LSM-trees

Fix the catch by keeping each segment **sorted by key**. A sorted segment is a **Sorted String Table (SSTable)**, and sorting buys three things:

1. **Merging is cheap.** Combining segments is a streaming merge-sort, even when they're far bigger than RAM.
2. **The index can be sparse.** You only need one index entry every few kilobytes; scan forward from the nearest entry. Blocks between entries are compressed.
3. **Range scans work**, because neighbouring keys are stored together.

But how do you write sorted files when writes arrive in random order? Buffer them in memory.

![Writes go to the WAL and a sorted in-memory memtable, which is flushed as an immutable SSTable. Compaction merges files down the levels; reads check newest to oldest, using Bloom filters to skip files.](diagrams/storage-engines-lsm.svg)

**Write path.** Append the change to a **write-ahead log** (for durability; see [WAL & recovery](#/lesson/wal-recovery)) and insert it into the **memtable**, a sorted in-memory structure (a skip list in RocksDB). When the memtable reaches a threshold (tens of megabytes), it becomes immutable and is flushed to disk as a new SSTable in **level 0**, in one sequential write. Then the corresponding WAL can be discarded.

**Read path.** Check the memtable, then SSTables from newest to oldest; the first hit wins. A miss is the expensive case, because in principle it touches every level.

**Bloom filters** make misses cheap. Each SSTable carries a compact bit array that answers "definitely not here" or "maybe here". There are no false negatives, and about **10 bits per key gives roughly a 1% false-positive rate**, so most point lookups read one data block. Bloom filters don't help range scans (RocksDB offers prefix filters for that).

**Updates and deletes** are just newer writes. An update shadows the old value; a delete writes a tombstone. Old versions and tombstones linger until compaction drops them, which is why a delete-heavy LSM workload can make reads slower before it makes the disk smaller.

> **Gotcha:** Tombstones in Cassandra must survive at least `gc_grace_seconds` (10 days by default) so that replicas which missed the delete don't resurrect the data during repair. A queue-like table with heavy deletes can accumulate millions of tombstones that every read must step over.

### Compaction strategies

Compaction is where LSM engines differ most, and the choice is a direct trade between the three amplification costs described later in this lesson.

![Size-tiered compaction merges similar-sized files into bigger ones; leveled compaction keeps each level a single sorted run ten times larger than the last.](diagrams/storage-engines-compaction.svg)

- **Size-tiered** (Cassandra's long-time default, HBase-style): when several SSTables of similar size accumulate, merge them into one larger file. Each byte is rewritten only a few times, so **write amplification is low**. But a key may live in many overlapping files (**higher read amplification**), and a big merge temporarily needs as much free space as its inputs. Plan for up to 50% headroom in the worst case.
- **Leveled** (LevelDB, RocksDB's default, Cassandra's LCS): each level beyond L0 is one sorted run of non-overlapping files, about **10× larger** than the level above. A point read touches at most one file per level, and space overhead is roughly 10%. The price is **higher write amplification**: pushing a byte down each level rewrites it many times, commonly 10–30× in total.
- **Time-window** (Cassandra's TWCS) groups SSTables by write time, so an expired window is dropped as a whole file. That's ideal for TTL'd time-series data.

> **Pitfall:** Compaction is background work, not free work. If ingest outpaces it, L0 files pile up and RocksDB deliberately **stalls writes**. Sustained-throughput benchmarks must run long enough to reach compaction steady state; a ten-minute LSM benchmark mostly measures the memtable.

## B-trees: update in place

The B-tree has been the default since the 1970s and still powers nearly every relational database. Instead of variable-size segments, it divides the database into fixed-size pages arranged as a tree. The [indexes lesson](#/lesson/indexes) covers lookups and fan-out; here we care about what a write does.

Inside, most engines use a **slotted page**:

```text
+---------------------------------------------------------------+
| header (LSN, checksum, free-space pointers)                   |
| slot 1 | slot 2 | slot 3 | ...  -->                           |
|                        free space                             |
|                     <--  ... | tuple 3 | tuple 2 | tuple 1    |
+---------------------------------------------------------------+
```

The slot array grows forward and tuples grow backward, so rows can move within the page without changing their external address (page number plus slot number, which is a Postgres `ctid`).

**Write path.** Walk to the leaf, modify the page in the buffer pool, append a WAL record. The dirty page goes to disk *later*, at a checkpoint or under cache pressure. When a page is full, it **splits** into two half-full pages and a new pointer goes into the parent. That's a multi-page change which must be crash-safe, which is one of the jobs the [WAL](#/lesson/wal-recovery) does.

Consequences:

- Reads are **O(log n) and very predictable**: 3–4 page reads, usually with the top levels cached.
- Each key lives in exactly one place, which makes **range locks and transactional isolation** simpler to implement. This is one reason relational databases stayed on B-trees.
- A 100-byte change dirties a whole 8–16 KB page. If many changes hit the same page between checkpoints, that cost is amortised. If changes are scattered randomly across a big table, it isn't.
- **Fill factor** leaves room on each page for updates. Postgres uses that room for **HOT updates**: if no indexed column changes and the page has space, the new row version stays on the same page and no index needs touching.

## Heap vs clustered storage

Where do the *rows* live relative to the primary key?

- **Postgres** stores rows in an unordered **heap**. Every index, including the primary key, points to a `ctid`. Because [MVCC](#/lesson/mvcc-concurrency) writes a new row version on update, a non-HOT update must touch *every* index.
- **InnoDB** (and SQL Server with a clustered index) stores rows **inside the primary-key B-tree**. Secondary indexes store the PK value, so a secondary lookup is two tree walks: secondary → PK → row. Without a declared PK, InnoDB uses the first unique non-null index, or failing that a hidden 6-byte row ID.
- **WiredTiger** stores documents in a B-tree keyed by an internal `RecordId`; `_id` and secondary indexes map to it. **Clustered collections** (MongoDB 5.3+) key the collection by `_id` instead.

> **Warning:** With a clustered PK, **random keys (UUIDv4) are painful**. Inserts land on random leaf pages, causing splits, half-empty pages and a working set the size of the whole index. Time-ordered keys (auto-increment, UUIDv7, ObjectId) append to the right-most leaf and keep the hot set tiny.

## The amplification triangle

Every engine pays three costs, and improving one usually worsens another. This is the **RUM conjecture**: you can optimise for two of **R**ead, **U**pdate and **M**emory/space overhead, but not all three.

![Every engine sits somewhere in the triangle between read, write and space amplification. B-trees favour reads, size-tiered LSM favours writes, and leveled LSM trades write cost for space efficiency.](diagrams/storage-engines-amplification.svg)

| | Meaning | B-tree | LSM, leveled | LSM, size-tiered |
|---|---|---|---|---|
| **Read amplification** | Pages read per logical read | Low: one path down the tree | Low–moderate: one file per level, Bloom filters | Higher: many overlapping files |
| **Write amplification** | Bytes written per byte changed | Whole pages + WAL, worst for scattered small updates | High: 10–30× from compaction | Low |
| **Space amplification** | Disk used ÷ live data | Fragmentation, partly empty pages | ~1.1× | Up to ~2× during merges |

DDIA's summary still holds: LSM-trees typically sustain **higher write throughput** and compress better (no half-empty pages), while B-trees give **more predictable read latency** and avoid compaction interfering with foreground traffic. "Typically" matters here, because the answer depends on key distribution, value size and how much of the data fits in RAM. Test with *your* workload at steady state.

## Why append-only suits SSDs and cloud volumes

SSDs can't overwrite in place. Flash is written in pages (around 4–16 KB) but erased only in much larger **erase blocks**, so the drive's flash translation layer remaps writes and garbage-collects partially dead blocks behind the scenes. Small random overwrites create lots of partially dead blocks, which means extra internal copying (**device-level write amplification**), lower sustained throughput and more wear. Large sequential writes, which is what LSM flushes and compactions produce, fill and free whole blocks cleanly.

Cloud volumes add an accounting reason. EBS SSD volumes count an I/O of up to 256 KiB as one operation, and merge small sequential I/Os, so a 64 MB SSTable flush costs a few hundred IOPS, whereas the same bytes as scattered 16 KB page writes would cost thousands. On provisioned-IOPS budgets that can decide the instance size.

## Engines you'll meet

| Engine | Structure | Where | Notes |
|---|---|---|---|
| **InnoDB** | B+tree, clustered by PK | MySQL, Aurora MySQL | Redo + undo logs, doublewrite buffer, change buffer for secondary indexes |
| **Postgres heap + nbtree** | Heap plus B+tree indexes | Postgres, Aurora/RDS Postgres | Relies on the OS page cache; MVCC versions live in the heap |
| **WiredTiger** | B+tree, MVCC, document-level concurrency | MongoDB (default since 3.2) | Snappy compression by default, index prefix compression, checkpoints every 60 s plus a journal |
| **RocksDB** | LSM, leveled by default | MyRocks, Kafka Streams and Flink state stores, TiKV, YugabyteDB (forked) | Fork of LevelDB; hugely tunable |
| **Pebble** | LSM (RocksDB-inspired, in Go) | CockroachDB | Replaced RocksDB in CockroachDB |
| **Cassandra / ScyllaDB engine** | LSM with pluggable compaction | Cassandra, ScyllaDB, Amazon Keyspaces (compatible API) | STCS, LCS, TWCS; UCS in Cassandra 5 |

## Row vs column layout (preview)

Orthogonal to B-tree vs LSM is *how values are laid out* within the files. A **row store** keeps each record's fields together, which is ideal for "fetch order 42". A **column store** keeps each column together, so "sum `spend` by `country` over a billion rows" reads only two columns, and similar values side by side compress brilliantly (dictionary, run-length, bit-packing). OLTP engines are row stores; Parquet, Delta Lake, Snowflake, BigQuery and ClickHouse are columnar. Columnar files are also immutable and merged in the background, which is the LSM idea again. See [OLTP vs OLAP & columnar storage](#/lesson/oltp-olap-columnar).

## Problem: Choosing an engine for a write-heavy IoT workload

> **Real world:** A smart-meter company has 2 million meters, each reporting a reading every 15 seconds (about 130,000 writes per second, roughly 1 KB each). Readings are never updated. Dashboards query "last 24 hours for meter X", and analysts want monthly aggregates. Raw data is kept for 90 days. Their prototype on a single RDS Postgres instance with a `UUID` primary key fell over at 20,000 writes per second. They ask you which database to use.

**Symptoms.** Insert latency climbing with table size, the buffer cache hit ratio falling, EBS write IOPS pinned at the provisioned limit, autovacuum running constantly, and nightly `DELETE` of expired rows taking hours.

**Diagnose.**

1. What's the write shape? Append-only, time-ordered, no updates: a textbook LSM or time-partitioned workload.
2. What's the read shape? Point-in-time ranges *per meter* (needs data clustered by meter and time) plus big scans (columnar territory).
3. Why did the prototype fail? A random `UUID` key scattered inserts across the whole index, so the working set was the entire index. Every row also carried secondary-index maintenance, and `DELETE`-based retention created bloat and vacuum load.

**Root causes.** An engine and key design that turns sequential arrivals into random page writes; retention implemented as row deletes rather than file or partition drops.

**Fix.**

- **Keep Postgres** if the team knows it: partition by time (daily), use a key of `(meter_id, ts)` within each partition, a **BRIN** index on `ts`, and retention by `DROP TABLE` on old partitions. TimescaleDB automates this and adds columnar compression (see [specialized databases](#/lesson/specialized-databases)). Batch inserts with `COPY` or multi-row `INSERT`.
- **Or choose an LSM store** such as Cassandra or ScyllaDB, with partition key `(meter_id, day)`, clustering key `ts`, **TWCS** compaction and a TTL of 90 days, so expired data disappears as whole SSTables.
- **Or MongoDB time series collections**, which bucket readings per meter and compress them in a columnar format, with `expireAfterSeconds` for retention.
- In every case, send the analytical workload to columnar storage (Delta Lake or Parquet in S3) through CDC or a stream, rather than running monthly scans on the ingest cluster.

**Prevent.** Load-test at the target rate *for hours*, until compaction or vacuum reaches steady state. Alert on compaction backlog or pending L0 files (LSM) or on dead tuples and checkpoint warnings (Postgres), and track disk headroom against the compaction strategy's worst case.

> **Model answer — what to say in the interview:** The workload is append-only, time-ordered and retention-driven, so I want an engine that turns writes into sequential I/O and expires data by dropping whole files or partitions rather than deleting rows. That points to either an LSM store with time-window compaction and TTLs, or a time-partitioned engine such as TimescaleDB or MongoDB time series collections. The prototype failed because random UUID keys made every insert a random page write into an index bigger than RAM, and row-level deletes added vacuum load. I'd key by device and time, batch writes, and keep heavy analytics off the ingest path in columnar storage. Then I'd prove it with a multi-hour load test at the target rate, watching compaction backlog and p99 latency.

```quiz
Q: Why can an LSM-tree sustain higher write throughput than a B-tree on the same hardware?
- It never writes to disk until a checkpoint
+ It buffers writes in memory and flushes them as large sequential, immutable files
- It skips the write-ahead log
- It stores only one version of each key
? Sequential appends avoid the random page writes of in-place B-tree updates; compaction later merges files in the background.

Q: An LSM-based store must answer GET for a key that does not exist. What keeps this cheap?
- The memtable stores every key ever written
- Leveled compaction guarantees one file per database
+ Per-SSTable Bloom filters rule out files that definitely don't contain the key
- Tombstones mark missing keys
? A miss would otherwise touch every level. Bloom filters have no false negatives, so most files are skipped without a disk read.

Q: A Cassandra cluster ingests TTL'd sensor data and disk usage keeps growing past the expected size. Which change most directly helps?
- Switch to leveled compaction to reduce write amplification
- Increase the memtable size
+ Use time-window compaction so expired data is dropped as whole SSTables
- Add more Bloom filter bits per key
? TWCS groups data by write time, so once every row in a window has expired the entire file is deleted, with no rewriting needed.

Q: Compared with size-tiered compaction, what does leveled compaction trade away?
+ Higher write amplification, in exchange for lower read and space amplification
- Lower write amplification, in exchange for higher space amplification
- Durability, in exchange for speed
- Range scan support
? Leveled rewrites data as it moves down roughly 10x-larger levels, but keeps one sorted run per level, so reads touch fewer files and old versions are purged sooner.

Q: Why are random UUIDv4 primary keys especially bad for InnoDB?
- InnoDB can't index 16-byte values
+ Rows live in the clustered PK B-tree, so random keys cause page splits and spread the hot set across the whole index
- UUIDs disable the doublewrite buffer
- They force InnoDB to use a heap instead
? Time-ordered keys (auto-increment, UUIDv7, ObjectId) append to the right-most leaf, keeping inserts sequential and the working set small.

Q: A customer's p99 read latency spikes once the dataset grows past the server's RAM, on an otherwise unchanged workload. Most likely explanation?
- The WAL has grown too large
+ The working set no longer fits in the cache, so reads now hit disk
- Bloom filters stop working on large datasets
- The CPU can't parse larger pages
? A falling cache hit ratio is the classic sign. Add RAM, shrink the working set (fewer or smaller indexes, archiving), or scale out.
```
