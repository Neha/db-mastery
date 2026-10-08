## Why specialised stores exist

A general-purpose OLTP database (Postgres, MongoDB) is the right default for most applications. Specialised engines earn their place when the *access pattern* is so skewed that a general engine wastes I/O, RAM or developer time.

You already met the families in [What is a database?](#/lesson/what-is-a-database). This lesson is the mechanism and the decision: what each one actually does, when it beats SQL, and when it becomes an expensive second system of record.

> **Key idea:** Pick the specialised store for the *query shape*, not the *data type*. Timestamps in Postgres are fine. Billions of append-only sensor readings with automatic downsampling are a time-series problem.

## Redis: data structures in RAM

Redis is an in-memory data structure server. You address values by key, but the values are not opaque blobs: they are **strings, lists, hashes, sets, sorted sets, streams**, plus bitmaps, HyperLogLog and geospatial indexes. Operations run on the server (`INCR`, `ZINCRBY`, `XADD`, `HINCRBY`), so you do not pull a document to Go, mutate it and write it back.

Typical uses: session store, rate limiter, leaderboard, distributed lock, pub/sub, and a hot working set in front of a database. The patterns, stampedes and invalidation bugs live in [Caching & Redis](#/lesson/problem-caching).

Command execution is single-threaded. That keeps operations atomic and latency predictable (often well under a millisecond when the working set is in RAM). I/O threads exist for the network path; they do not run your commands in parallel.

### Persistence

Redis can run as a pure cache (data dies with the process) or persist to disk:

| Mechanism | What it writes | Trade-off |
|---|---|---|
| **RDB** | A point-in-time snapshot, via `fork` | Compact; a crash can lose everything since the last snapshot |
| **AOF** | An append-only log of write commands | `appendfsync everysec` (the common setting) can lose about a second; `always` is durable and slower |
| **Both** | Snapshot plus incremental AOF | Faster restarts than AOF alone |

Replication to replicas is independent of persistence. A replica is not a substitute for a durable primary if you treat Redis as the system of record.

### Eviction

`maxmemory` caps RAM. When you hit it, `maxmemory-policy` decides what happens:

| Policy | Behaviour |
|---|---|
| `noeviction` (default) | Writes return errors; reads still work |
| `allkeys-lru` / `allkeys-lfu` | Evict any key, least recently / frequently used |
| `volatile-lru` / `volatile-lfu` / `volatile-ttl` | Evict only keys that have a TTL |
| `allkeys-random` / `volatile-random` | Evict at random |

> **Warning:** The default is `noeviction`. A cache that you forgot to cap will grow until the host OOMs. A cache you capped but left on `noeviction` will start failing writes at the worst moment. Set `maxmemory` *and* an eviction policy when Redis is a cache.

### When not to use Redis

- **The working set does not fit in RAM.** Community Redis keeps the dataset in memory. If most keys are cold, a disk-backed database is cheaper and simpler.
- **You need ad-hoc query, joins or secondary indexes.** Redis is access-by-key (and a few built-in indexes such as sorted sets). It is not a query engine.
- **You need a durable system of record** with point-in-time recovery, rich backups and multi-entity ACID. Configure persistence if you must, but a DBMS is the honest tool.
- **Cross-slot multi-key operations in Cluster.** Keys that must be mutated together need a hash tag so they land in the same slot.

> **SA angle:** "We will just put it in Redis" is a smell when the speaker means *primary data*. Ask: what happens on a full power loss? How do you restore to Tuesday 14:00? Who owns the schema? If those answers are vague, Redis is the cache, not the database.

## Search: inverted indexes

A B-tree answers `WHERE email = ?`. Full-text search answers "which documents are *about* this phrase, ranked by relevance, with typos allowed?" That needs an **inverted index**: a map from each term to a **posting list** of document IDs (plus positions and frequencies).

![An inverted index maps each term to the documents that contain it. A query is an intersection or union of posting lists, then a rank.](diagrams/specialized-databases-inverted-index.svg)

Building the index:

1. **Analyse** the text: tokenise, lowercase, drop stop words, optionally stem (`running` → `run`).
2. For each term, append the document ID to its posting list.
3. At query time, analyse the query the same way, fetch the lists, combine them, and **rank** (BM25 is the usual default).

Inverted indexes are the same idea as a GIN index on `tsvector` in Postgres (see [Indexes](#/lesson/indexes)). Search engines add analysers, relevance, fuzzy matching, highlighting and faceted navigation.

| Product | What you are buying |
|---|---|
| **OpenSearch** (and Elasticsearch) | A Lucene cluster: shards, replicas, near-real-time refresh, aggregations |
| **Atlas Search** | Lucene indexes maintained on your Atlas collections; query with `$search` in an aggregation |
| Postgres `tsvector` / GIN | Good enough for modest full-text on data that already lives in Postgres |

> **Tip:** Search is a *derived* index, not a system of record. The operational database stays the source of truth; the search index is rebuilt or incrementally updated (change streams, CDC). Expect refresh lag of about a second on Lucene-based engines unless you tune it.

Do not use a search cluster as your primary store. Relevance ranking is the product; durability and multi-document transactions are not.

## Time-series engines

Time-series workloads share a shape: **high ingest**, almost append-only, queries that filter a time range and then aggregate (`avg` CPU by host by minute), and a need to **expire or downsample** old data.

A row store will accept the inserts and then suffer: indexes churn, vacuum or compaction never catches up, and `WHERE time BETWEEN … GROUP BY time_bucket` becomes a large scan. Time-series engines specialise the storage:

- **Chunk or bucket by time** (and often by a meta key such as device ID), so a range query reads a few contiguous files.
- **Compress** runs of timestamps and slowly changing tags (delta encoding, Gorilla-style XOR on floats).
- **Retention and continuous aggregates** as first-class operations.

| Engine | Shape | Reach for it when |
|---|---|---|
| **TimescaleDB** | Postgres extension: hypertables partitioned into chunks | You want SQL, joins to relational tables, and time-series in the same database |
| **InfluxDB** | Purpose-built TSDB (measurements, tags, fields) | Metrics or IoT ingest is the product and you do not need rich joins |
| **MongoDB time-series collections** | Documents bucketed by `timeField` + `metaField` | The measurements already live next to operational MongoDB data |

Prometheus is the usual choice for *infrastructure* metrics (pull model, PromQL, short retention). It is not a general application TSDB.

> **Remember:** A `created_at` column does not make a table a time-series database. The engine has to cluster, compress and expire around time.

## Graph: when relationship hops beat joins

A **property graph** stores **nodes** and **relationships** as first-class records, with properties on both. Neo4j is the canonical example (Cypher: `MATCH (a:Person)-[:KNOWS]->(b)`). Amazon Neptune and several multi-model stores play in the same space.

The mechanism that matters is **index-free adjacency**: each node holds direct references to its edges. Walking one relationship is a pointer chase whose cost depends on the node's *degree*, not on the size of a join table.

A SQL 3-hop query ("friends of friends of friends") is a chain of joins. Each hop multiplies the intermediate result. The planner does index lookups into a large `knows` table every time. That is fine for one or two hops on modest data and painful for variable-length paths, pattern matching ("is there a fraud ring of length 3–6?") and repeated traversal.

Reach for a graph store when:

- The query *is* the path: recommendations, access-control inheritance, knowledge graphs, fraud rings, bill-of-materials explosions.
- Depth is variable or greater than two, and you filter on edge types and properties along the way.

Stay on SQL (or documents) when:

- You mostly fetch an entity by key and a handful of foreign keys.
- The hard work is aggregation over the whole set ("revenue by region"), not traversal.
- Recursive common table expressions already solve the rare 2–3 hop query you have.

> **Interview:** "Why not just join?" Answer with *index-free adjacency* and *intermediate result explosion*, not "graphs are faster". A 1-hop lookup by primary key is not a graph problem.

## Vector indexes

An embedding is a point in a high-dimensional space (often 384–3072 floats). "Find passages like this question" is **nearest-neighbour search**. You have already designed the application around this in [RAG / GenAI](#/lesson/design-rag-genai); here is the index.

**Exact** k-nearest neighbours scans every vector (or uses an exact spatial index). Recall is 100%. At millions of wide vectors that is too slow for an interactive query.

**Approximate nearest neighbour (ANN)** trades a little recall for a large speed-up.

| Index | How it works | Cost |
|---|---|---|
| **HNSW** | A layered proximity graph; search greedily from a sparse top layer down to a dense base layer | High recall, fast queries, **memory-hungry** (the graph sits in RAM) |
| **IVF** (inverted file) | Cluster vectors (k-means); search only the `nprobe` nearest clusters | Cheaper RAM, recall depends on `nprobe`; a train step is required |

![Exact search visits every vector. HNSW walks a small number of graph edges, so latency stays interactive while recall stays high.](diagrams/specialized-databases-hnsw.svg)

Product quantisation and `int8` or binary embeddings cut memory further, at more recall cost.

### Where to put the index

| Choice | Use when |
|---|---|
| **pgvector** | Vectors live next to relational rows, you want SQL filters and transactions, and the scale is millions rather than billions |
| **Atlas Vector Search** | Operational documents *are* the chunks; you need metadata filters, hybrid search and one security model |
| **Databricks Vector Search** | Embeddings are produced from Delta tables and must stay under Unity Catalog governance |
| Dedicated (Pinecone, Milvus, Weaviate) | Vector QPS or scale is the product, and you accept a sync pipeline from the system of record |

> **SA angle:** Do not start with a dedicated vector database. Start where the source data already lives. A second store is justified when the index memory or query load would harm the operational cluster, or when the lakehouse *is* the system of record.

## Match the store to the workload

| Workload | Reach for | Do not reach for | Why |
|---|---|---|---|
| Session, rate limit, leaderboard, hot key | Redis | The OLTP database as a cache | RAM-speed, purpose-built structures |
| System of record, joins, constraints | Postgres / MongoDB | Redis, OpenSearch | Durability, query, transactions |
| Relevance, fuzzy text, facets | OpenSearch or Atlas Search | `LIKE '%foo%'` on the primary | Inverted index + BM25 |
| Sensor / metric ingest, downsample, expire | Timescale, Influx, MongoDB TS | An unpartitioned row table that grows forever | Time chunking and compression |
| Variable-depth relationship walk | Neo4j (or a graph projection) | A 5-way self-join you cannot explain | Index-free adjacency |
| Semantic search, RAG | pgvector / Atlas Vector / Databricks Vector | Keyword-only search for "meaning" | ANN over embeddings |
| Analytics over years of facts | Columnar / lakehouse | Redis or a graph store | Scan few columns, many rows |

Multi-model platforms collapse some rows: Atlas can do documents, search, vector and time-series; Postgres can do relational, `jsonb`, full text, pgvector and Timescale. That is a *packaging* win (one security model, one backup) provided each feature is actually fit for the load.

## Problem: "Can we just put the catalogue in Redis?"

> **Real world:** A retailer tells you checkout crawls every weekday at 9am. Someone ran a Redis demo, saw sub-millisecond `GET`s, and proposed moving the product catalogue *and* stock counters into Redis so Postgres is "out of the path". They ask you, in an interview, whether you agree.

**Symptoms.** p99 checkout latency spikes at the morning peak. Postgres CPU and I/O are high. Cache hit rate, if any, is unknown. Stock has been wrong before after a restart.

**Diagnose.** Ask what is slow: product reads, stock decrement, or payment? Check whether the working set fits in the Postgres buffer pool. Look for missing indexes, lock pile-ups on a hot stock row, and connection-pool saturation. Measure Redis only if it is already in the path.

**Root causes.** Usually the catalogue *reads* want a cache, while stock *writes* want a durable increment with constraints. Redis as the only copy of stock loses updates on crash unless persistence and failover are designed as carefully as the database they are trying to leave.

**Fix.** Cache the immutable-ish catalogue (cache-aside, TTL plus invalidation on publish) in Redis. Keep stock in Postgres or MongoDB with an atomic decrement and a check that quantity stays non-negative. If stock is the hotspot, shard or queue the hot SKUs; do not make Redis the ledger.

**Prevent.** Treat Redis as a cache unless you can answer RPO, backup and failover in the same breath as for the primary. Load-test eviction and a replica failover before go-live.

> **Model answer — what to say in the interview:** I would split the problem. Catalogue *reads* at 9am are a caching problem: put Redis in front of Postgres or MongoDB, invalidate on publish, and size `maxmemory` with an LRU policy so a stampede cannot OOM the box. Stock is a system-of-record write: it needs an atomic conditional decrement and a crash story, so it stays in the database. Redis as the only copy of stock means designing persistence, replication and restore to the same standard as Postgres, at which point you have not simplified anything. The 9am spike is more often a working-set, index or lock problem on the primary, which I would prove with an explain plan before adding a store.

```quiz
Q: Redis is a poor system of record for stock counters primarily because:
+ Crash and restart behaviour depends on persistence settings, and you lose the DBMS's recovery, constraints and point-in-time restore
- Sorted sets cannot store integers
- Redis cannot increment a value atomically
- Redis is always multi-threaded, so increments race
? Redis can persist, but you then own RPO, backup and failover yourself. An atomic INCR is not the same as a durable ledger.

Q: What does an inverted index store?
- A B-tree from document ID to the full document text
+ A map from each term to the list of documents (and positions) that contain it
- A hash of every document used for deduplication
- A vector embedding of each document
? Search is term → postings, then intersection and BM25. That is the opposite of "scan every document for this phrase".

Q: Why can a graph store beat SQL at "friends of friends of friends"?
- Cypher compiles to faster SQL under the hood
- Graph databases keep the whole graph in a single row
+ Index-free adjacency walks pointers whose cost tracks node degree, while each SQL hop joins a large table and multiplies intermediates
- SQL cannot express a three-table join
? The win is traversal cost, not a magic language. A one-hop primary-key lookup is not a graph problem.

Q: HNSW is preferred over exact k-nearest neighbours when:
- You must have 100% recall and can wait
+ You need interactive latency at millions of embeddings and can tolerate a small recall loss
- The vectors are 2-dimensional geospatial points
- You have no RAM for an index
? Exact search is O(n). HNSW is an ANN graph: fast and high-recall, but the graph is memory-heavy.

Q: You are embedding support tickets that already live in MongoDB Atlas and must be filtered by tenant at query time. The default recommendation is:
- Databricks Vector Search synced from a nightly dump
- Redis with a hash per ticket
+ Atlas Vector Search on the same documents, with a tenant filter in the vector query
- A new Neo4j cluster of ticket nodes
? Keep vectors next to the source data and apply the security filter at retrieval. Dedicated stores wait until scale or QPS forces them.

Q: A `created_at` timestamp on an orders table means you should move orders into InfluxDB.
- Yes: any time-stamped table is a time-series workload
- Only if you also add a graph edge per order
- Only if Redis eviction is set to volatile-ttl
+ No: time-series engines earn their place on high-ingest, append-heavy measurements that you bucket, compress and expire
? Orders are operational records with updates, joins and constraints. Sensors and metrics are the time-series shape.
```
