## What is a database? The landscape

A **database** is an organised collection of data. A **database management system (DBMS)** is the software that stores it, lets many users read and change it safely at the same time, and keeps it intact when hardware fails.

As a front-end engineer you've used `localStorage`, JSON files and API responses. Why not just write JSON to a file?

## Why not just files?

Try building a shopping cart on a JSON file and you will rediscover every DBMS feature the hard way:

| Problem with files | What a DBMS gives you |
|---|---|
| Two requests write at once → one overwrites the other | **Concurrency control** (locks, MVCC) |
| Server crashes halfway through a write → corrupted file | **Durability & atomicity** (write-ahead log) |
| Finding one user means reading the whole file | **Indexes** |
| Every app re-implements "find orders by user" | **A query language** (SQL, MQL) |
| Anyone can read everything | **Security** (authn/authz, encryption, auditing) |
| One disk dies → data gone | **Replication & backup** |
| The file outgrows one machine | **Partitioning / sharding** |

> **Key idea:** A database is a set of trade-offs packaged as a product. Every database family on this page makes *different* trade-offs. Your job as an SA is to match those trade-offs to the customer's workload.

## Data models

A **data model** is how the database wants you to think about your data.

- **Relational** — tables of rows and columns, related by keys, queried with SQL. *Postgres, MySQL, Oracle, SQL Server.*
- **Document** — self-describing JSON-like documents grouped in collections. *MongoDB, Couchbase.*
- **Key-value** — an opaque value looked up by key; extremely fast and simple. *Redis, DynamoDB (also document), etcd.*
- **Wide-column** — rows with dynamic columns grouped in column families, built for massive write throughput. *Cassandra, HBase, Bigtable.*
- **Graph** — nodes and edges; queries traverse relationships. *Neo4j, Neptune.*
- **Search** — inverted indexes for full-text relevance. *Elasticsearch, OpenSearch, Atlas Search (Lucene).*
- **Time-series** — optimised for timestamped append-heavy data. *TimescaleDB, InfluxDB, MongoDB time-series collections.*
- **Vector** — stores embeddings and does nearest-neighbour search for AI. *pgvector, Atlas Vector Search, Databricks Vector Search, Pinecone.*
- **Analytical / columnar** — stores data column by column for fast scans and aggregations. *Snowflake, BigQuery, Redshift, ClickHouse, Databricks SQL on Delta.*

![Nine data models, each optimised for a different access pattern.](diagrams/what-is-a-database-data-models.svg)

Most modern products are **multi-model**. MongoDB has search, vector and time-series built in. Postgres does JSON, vectors and full-text search. Databricks unifies warehousing, streaming and ML.

## OLTP vs OLAP — the most important split

| | OLTP (transactional) | OLAP (analytical) |
|---|---|---|
| Example | "Place this order" | "Revenue by region by month for 5 years" |
| Query shape | Touch few rows, by key | Scan millions/billions of rows, few columns |
| Latency | Milliseconds | Seconds to minutes |
| Concurrency | Thousands of small writes/sec | Fewer, heavier queries |
| Storage layout | Row-oriented | Column-oriented |
| Typical systems | Postgres, MongoDB, DynamoDB | Databricks, Snowflake, BigQuery |

![A row store keeps a whole record together; a column store keeps each field together across rows.](diagrams/what-is-a-database-row-vs-column.svg)

**MongoDB lives mostly in OLTP**: operational apps. **Databricks lives mostly in OLAP + AI**, and has been moving into OLTP with Lakebase, its managed Postgres. **Red Hat** is the platform both can run on (OpenShift), and it ships its own data services around Kafka and AI.

## The anatomy of a DBMS

![A request hits connections, parser, optimiser and executor, then the storage engine: concurrency, WAL, buffer pool and data files.](diagrams/what-is-a-database-dbms-anatomy.svg)

You'll study each box in the Internals track.

> **SA angle:** When a customer says "the database is slow", this diagram is your troubleshooting map. Is it connections (pool exhausted)? The plan (missing index)? The executor (bad join)? Storage (I/O bound, cache too small)? Locks (contention)?

## Questions to ask about any database

Use this checklist whenever you meet a new database (or a competitor):

1. What's the **data model** and query language?
2. How does it achieve **durability**? (WAL? fsync? replication?)
3. What **consistency** does it offer, and is it tunable?
4. How does it **scale** reads? Writes? (replicas, sharding, storage/compute separation)
5. How does it handle **failure**? (automatic failover, RPO/RTO)
6. What's the **operational model**? (self-managed, operator, fully managed SaaS)
7. What does it **cost**, and what drives the cost? (compute, storage, I/O, egress)

> **Interview:** A customer wants each user to see their last 20 orders in under 50 ms, plus a five-year revenue-by-country dashboard. One database or two?

> **Model answer:** Two stores. Last-20-orders is OLTP: a keyed lookup with a millisecond budget, so Postgres or MongoDB. The dashboard is OLAP: scan years of rows and a few columns, so a lakehouse or warehouse. Do not make checkout share I/O with a five-year `GROUP BY`. CDC or a nightly extract into a columnar layout is the usual bridge. Multi-model products blur the marketing line; the access patterns still want different layouts.

```quiz
Q: A customer wants to show each user their last 20 orders in under 50 ms. What kind of workload is this?
+ OLTP
- OLAP
- Graph
? Point lookups and small range reads by key with strict latency are classic OLTP.

Q: Which storage layout makes "average order value per country over 3 years" fast?
- Row-oriented
+ Column-oriented
- Key-value
? Analytical queries read a few columns across many rows; columnar storage reads only those columns and compresses well.

Q: What is the main job of the write-ahead log (WAL)?
- Speed up reads
+ Guarantee durability and atomicity across crashes
- Store indexes
? Changes are logged sequentially before they're applied to the data files, so after a crash the database can replay or undo them.

Q: Which statement about MongoDB is most accurate today?
- It is a key-value store without indexes
+ It is a document database that also offers search, vector search and time-series capabilities
- It only supports analytics workloads
? MongoDB is a multi-model "developer data platform" centred on documents, with secondary indexes, search and vector search.
```
