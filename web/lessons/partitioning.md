## Why partition?

Replication copies the *same* data. **Partitioning** (sharding) splits *different* data across nodes so that the dataset and the write load can exceed one machine. Usually you do both: each partition is replicated.

![Range partitions put every new timestamp in the latest shard; hashing scatters writes but loses range locality.](diagrams/partitioning-range-hash.svg)

| System | Unit name |
|---|---|
| MongoDB | Chunk (in a shard) |
| Cassandra | Token range (vnode) |
| DynamoDB | Partition |
| Kafka | Topic partition |
| Spark / Delta Lake | Partition (in memory) / file partition directory |
| CockroachDB / TiKV | Range / Region |
| Postgres | Declarative partitions (single node) or Citus shards |

## Strategy 1: range partitioning

Assign contiguous key ranges to partitions: `[A–F] [G–M] [N–Z]` or `[2026-01, 2026-02, ...]`.

- ✅ Efficient **range scans** (all of `customer:1000–2000` is on one node).
- ❌ **Hotspots** with monotonic keys: timestamps put every write in the latest partition.
- Used by MongoDB ranged shard keys, HBase, Bigtable, CockroachDB and Spanner (which split ranges automatically).

## Strategy 2: hash partitioning

`partition = hash(key) mod N`, or better, hash into a fixed key space.

- ✅ Even distribution of data and writes.
- ❌ Range queries must hit **all** partitions.
- Used by MongoDB hashed shard keys, Cassandra, DynamoDB and Kafka (by message key).

> **Warning:** Naive `hash(key) % N` is a trap. Changing N (adding a node) moves almost **every** key. Real systems use consistent hashing or a fixed number of partitions.

## Consistent hashing

Place both nodes and keys on a **ring** (hash space 0…2³²). Each key belongs to the first node clockwise from it.

```text
               0
          N3 ●   ● k1 → N1
        ●k4        ● N1
        N2 ●     ● k2 → N2 ...
```

- Adding a node only moves keys between it and its predecessor, about **1/N** of keys.
- **Virtual nodes**: each physical node owns many points on the ring, which smooths distribution and lets bigger machines take more vnodes.
- Used by Cassandra, DynamoDB (originally), Riak, CDNs and memcached clients.

## Strategy 3: a fixed number of partitions

Create many more partitions than nodes (for example 1,000 partitions on 10 nodes). Rebalancing moves **whole partitions**, and keys never change partition. This is used by Elasticsearch, Riak, Couchbase (vBuckets) and Kafka.

## Hot keys and skew

Even with perfect hashing, one key can be hot: a celebrity's timeline, a flash-sale product, a "null" tenant.

Mitigations:

- **Key salting**: append a random suffix `productId#0..9` to spread writes, then aggregate on read.
- **Write sharding** with buffered counters.
- **Caching** hot reads in front.
- **Splitting** hot chunks or ranges (MongoDB auto-splitting, DynamoDB adaptive capacity).
- In Spark: **skew joins** (AQE splits skewed partitions automatically), or salting join keys.

## Secondary indexes on partitioned data

| | Local (document-partitioned) | Global (term-partitioned) |
|---|---|---|
| Where the index lives | Each partition indexes its own data | Index partitioned by indexed value |
| Writes | Fast (local) | Slower (may touch another partition, often async) |
| Reads by indexed field | **Scatter-gather** to all partitions | Targeted to one index partition |
| Examples | MongoDB, Cassandra, Elasticsearch | DynamoDB GSIs (eventually consistent) |

## Request routing

How does a client find the right partition?

1. **Routing tier**: MongoDB `mongos`, which uses config server metadata.
2. **Any node forwards**: Cassandra gossip, where every node knows the ring.
3. **Partition-aware client**: Kafka clients, smart drivers.

Coordinating the metadata ("which node owns which partition?") usually needs a consensus system: ZooKeeper, etcd, MongoDB config servers or Kafka KRaft.

## Partitioning in analytics (preview)

In data lakes and Delta Lake, "partitioning" means **directory layout**: `/events/date=2026-10-07/`. Queries filtering on `date` skip whole directories (**partition pruning**). Over-partitioning (for example by `user_id`) creates millions of tiny files, a classic performance killer. Databricks now recommends **liquid clustering** instead of manual partitioning for most tables.

> **SA angle:** The same idea, "choose a key that matches your access pattern and spreads load", applies to MongoDB shard keys, Kafka message keys, DynamoDB partition keys and Delta clustering columns. In interviews, generalise: *"Partitioning is always a trade-off between locality, which helps range queries, and spread, which helps write throughput."*

> **Lab:** In **Lab: Consistent hashing & MVCC** you'll implement a hash ring with virtual nodes in Go and measure how many keys move when you add a node, compared with naive modulo hashing.

> **Interview:** How do you choose a shard key for a multi-tenant SaaS where almost every query is `WHERE tenant_id = ?`?

> **Model answer:** Lead with the query. A compound `{tenant_id, id}` or `{tenant_id, created_at}` keeps a request on one shard and gives the second field enough cardinality that a huge tenant can split. Hash-only `tenant_id` pins a jumbo tenant to one chunk forever. Plan the outlier: salt, a dedicated shard, or a zone. Do not range-partition on a timestamp if ingest is the hot path — every write lands in the latest partition.

```quiz
Q: Why is hash(key) % N a poor partitioning scheme for a growing cluster?
+ Changing N remaps almost every key, forcing massive data movement
- Hash functions are too slow
- It creates range-query hotspots
- It can't handle string keys
? Consistent hashing or a fixed number of partitions limits movement to about 1/N of keys.

Q: An events table is range-partitioned by timestamp. What problem will heavy ingestion cause?
- Slow range scans
+ A write hotspot: all new events go to the latest partition
- Too many partitions
- Lost writes
? Monotonic keys concentrate writes. Hash part of the key, or prefix it with a well-distributed field.

Q: What's the benefit of virtual nodes in consistent hashing?
- They encrypt data
+ They spread each node's ownership across many ring positions for smoother balance and flexible weighting
- They eliminate replication
- They make range queries efficient
? Many small ranges per node even out load and spread rebalancing across all nodes.

Q: In a system with local secondary indexes, a query on a non-shard-key indexed field must…
- Hit exactly one partition
+ Query all partitions (scatter-gather) and merge results
- Fail
- Use the global index
? Each partition only indexes its own documents, so every partition must be consulted.
```
