## Why PostgreSQL matters for all three roles

- **Red Hat**: Postgres is the default open-source database on RHEL and OpenShift (Red Hat ships PostgreSQL in RHEL application streams, and partners like EDB and Crunchy Data run Postgres operators on OpenShift).
- **Databricks**: **Lakebase** is Postgres.
- **MongoDB**: Postgres (with JSONB) is the #1 competitor you'll position against.

It's been the most popular database among developers in the Stack Overflow survey for several years running.

## Process architecture

![Postgres forks one backend process per connection; they share buffers, WAL and background writers.](diagrams/postgres-deep-dive-processes.svg)

> **Warning:** One **OS process per connection** (about 5–10 MB each) means Postgres doesn't like thousands of direct connections. Use a pooler: **PgBouncer** (transaction pooling), RDS Proxy on AWS, or app-side pools like `pgxpool` in Go. This comes up constantly in microservices and Kubernetes designs.

![PgBouncer multiplexes many client sessions onto a few server connections, holding a backend only for a transaction.](diagrams/postgres-deep-dive-pgbouncer.svg)

## Storage recap

- Tables are **heap files** split into 8 KB pages. Indexes are separate files (B-tree by default).
- MVCC keeps row versions in the heap (`xmin`/`xmax`). **Autovacuum** removes dead tuples and freezes old XIDs.
- **TOAST** stores large values (big text or JSONB) out of line, compressed.
- The WAL lives in `pg_wal/` and is used for crash recovery, replication and PITR.

## Rich types and indexes

| Feature | Example |
|---|---|
| JSONB | `attrs @> '{"size":"M"}'`, GIN index on `attrs` |
| Arrays | `tags && ARRAY['sale']` |
| Range types | `tstzrange` with exclusion constraints (no double-booking) |
| Full-text search | `to_tsvector`, `tsquery`, GIN |
| Geospatial | **PostGIS** extension |
| Index types | B-tree, Hash, GIN, GiST, SP-GiST, BRIN |
| Partitioning | Declarative `PARTITION BY RANGE / LIST / HASH` |

```sql
-- No overlapping room bookings, enforced by the database
CREATE EXTENSION btree_gist;
CREATE TABLE bookings (
  room_id INT, during TSTZRANGE,
  EXCLUDE USING gist (room_id WITH =, during WITH &&)
);
```

## Extensions: Postgres' superpower

| Extension | Adds |
|---|---|
| **pgvector** | `vector` type, HNSW/IVFFlat indexes: vector search for RAG |
| **PostGIS** | Geospatial |
| **TimescaleDB** | Time-series hypertables and compression |
| **Citus** | Distributed (sharded) Postgres |
| pg_stat_statements | Query performance statistics (always enable this) |
| pg_partman | Partition management |
| pgaudit | Audit logging for compliance |

```sql
CREATE EXTENSION vector;
CREATE TABLE docs (id BIGSERIAL PRIMARY KEY, body TEXT, embedding VECTOR(1024));
CREATE INDEX ON docs USING hnsw (embedding vector_cosine_ops);
SELECT id, body FROM docs ORDER BY embedding <=> $1 LIMIT 5;   -- cosine distance
```

## Replication and high availability

| Mechanism | What it does | Use |
|---|---|---|
| **Streaming (physical) replication** | Ships WAL bytes; replica is a byte-identical read-only copy | HA standby, read replicas |
| Synchronous replication | `synchronous_standby_names`; commits wait for standby | Zero data loss (RPO=0) |
| **Logical replication** | Publishes row changes per table (`CREATE PUBLICATION/SUBSCRIPTION`) | Selective replication, major-version upgrades, CDC (Debezium uses logical decoding) |
| **PITR** | Base backup plus archived WAL (pgBackRest, Barman, WAL-G) | Restore to any point in time |

Postgres has **no built-in automatic failover**. You add it with:

- **Patroni**: the de facto HA manager, using etcd/Consul/Kubernetes as a consensus store for leader election.
- **Kubernetes operators**: CloudNativePG, Crunchy PGO, Zalando, EDB. Covered in the next lesson.
- Managed services: RDS/Aurora, Azure Flexible Server, Cloud SQL/AlloyDB, Lakebase, Neon, Supabase.

## Scaling Postgres

1. **Vertical** first. Modern machines go very far (hundreds of cores, TBs of RAM).
2. **Read replicas** for read-heavy workloads (mind replication lag).
3. **Connection pooling** (PgBouncer).
4. **Partitioning** for large tables (time-based).
5. **Sharding**: Citus, or application-level, or distributed SQL (CockroachDB, YugabyteDB, which are Postgres-compatible).
6. **Caching** (Redis) for hot reads.

## Tuning essentials

| Parameter | Rule of thumb |
|---|---|
| `shared_buffers` | ~25% of RAM |
| `effective_cache_size` | ~50–75% of RAM (a planner hint) |
| `work_mem` | Per sort/hash operation per connection, so be careful (4–64 MB) |
| `maintenance_work_mem` | Larger for vacuum and index builds |
| `max_connections` | Keep moderate; use a pooler |
| autovacuum settings | More aggressive for update-heavy tables |

Monitor `pg_stat_statements` (slowest queries), `pg_stat_activity` (what's running, `idle in transaction`), `pg_stat_user_tables` (dead tuples, last vacuum) and replication lag.

> **SA angle:** At Red Hat, the conversation is *"run Postgres reliably on OpenShift across on-prem and cloud with an operator, GitOps and consistent security"*. At MongoDB, it's *"here's when the document model and Atlas beat Postgres, and here's where they don't"*. At Databricks, it's *"Lakebase gives you managed Postgres that's natively connected to your lakehouse"*. Same database, three narratives. Practise all three.

> **Lab:** The **PostgreSQL from Go** lab uses pgx, a connection pool, transactions, isolation levels and EXPLAIN.

> **Interview:** Fifty pods each open a pool of 40. Postgres `max_connections` is 2000 and you still see p99 spikes. Why?

> **Model answer:** Each connection is an OS process, roughly 5–10 MB, plus lock-table and buffer-pool contention. Fifty times 40 is 2,000 backends even when most are idle. Size for concurrent queries — arrival rate times service time — not for pod count. Put PgBouncer in transaction mode in front and cap the sum of pod pools to what the instance can actually run: often tens of busy connections on a small box, not thousands.

```quiz
Q: Why do Postgres deployments with many microservices usually need PgBouncer or a similar pooler?
+ Each connection is a separate OS process with significant memory overhead, so thousands of direct connections hurt performance
- Postgres limits each database to 10 connections
- PgBouncer adds replication
- It's needed for SSL
? A process-per-connection model makes connection count expensive. Pooling multiplexes many client sessions onto fewer server connections.

Q: Which replication type would you use to upgrade Postgres across major versions with minimal downtime?
- Physical streaming replication
+ Logical replication
- Synchronous replication
- WAL shipping only
? Physical replication requires identical major versions. Logical replication replicates row changes between different versions.

Q: Postgres has no built-in automatic failover. What commonly provides it?
- pgvector
+ Patroni or a Kubernetes operator such as CloudNativePG or Crunchy PGO
- pg_stat_statements
- TOAST
? Patroni and operators use a consensus store (etcd, the Kubernetes API) for leader election and orchestrate promotion.

Q: Which extension turns Postgres into a vector store for RAG?
- PostGIS
- Citus
+ pgvector
- pgaudit
? pgvector adds the vector type, distance operators and HNSW/IVFFlat indexes.
```
