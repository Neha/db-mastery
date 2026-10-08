## How to use this bank

Answer each question **out loud** before opening the model answer in your head. Time yourself (2 minutes for conceptual questions, 5 for scenarios). Note weak spots in the notes box below and revisit the linked lessons.

## Fundamentals

### 1. Explain ACID, and how isolation levels trade correctness for performance.
**Model answer:** Atomicity (all or nothing, via undo/WAL), Consistency (constraints and invariants hold, largely the app's responsibility), Isolation (concurrent transactions don't interfere, via locks or MVCC), Durability (committed data survives crashes, via WAL plus fsync and replication). Isolation levels define which anomalies are allowed. Read Committed (Postgres default) prevents dirty reads but allows non-repeatable reads and lost updates without care. Snapshot isolation (Postgres Repeatable Read, MongoDB transactions) gives a consistent snapshot but allows write skew. Serializable (SSI in Postgres) prevents all anomalies at the cost of aborts that the app must retry. I choose per use case: money gets serializable or explicit locking, and analytics reads get snapshot.

### 2. How does a B-tree index work, and when won't the database use it?
**Model answer:** A balanced tree of pages with high fan-out (hundreds of keys per page), so 3–4 levels cover billions of rows. Leaves are sorted and linked for range scans. The optimizer skips it when selectivity is low (a large fraction of rows match, so a sequential scan is cheaper), when the query doesn't use the leftmost prefix of a composite index, when a function is applied to the column without an expression index, with type mismatches, or when statistics are stale and misestimate cardinality.

### 3. B-tree vs LSM-tree?
**Model answer:** B-trees update pages in place: fast, predictable reads, but random writes and page-level write amplification. Used by Postgres, InnoDB and WiredTiger. LSM-trees buffer writes in a memtable and flush sorted immutable SSTables, merged by compaction: very high write throughput and good compression, but reads may check several files (mitigated by bloom filters) and compaction costs background I/O. Used by RocksDB, Cassandra and CockroachDB. Write-heavy ingestion favours LSM; read-heavy OLTP favours B-trees.

### 4. What is MVCC and what operational issue does it cause in Postgres?
**Model answer:** Writers create new row versions; readers see the version matching their snapshot, so readers and writers don't block each other. Postgres stores old versions in the heap, and VACUUM removes them once no snapshot needs them. Long-running or idle-in-transaction sessions hold old snapshots, which prevents cleanup and causes table and index bloat, and in the extreme, transaction ID wraparound risk. The fixes are monitoring, killing idle transactions and tuning autovacuum.

### 5. Explain CAP and PACELC honestly.
**Model answer:** During a network partition you must choose between linearizable consistency and availability on both sides. Partitions are unavoidable, so it isn't "pick 2 of 3". PACELC adds that in normal operation you trade latency against consistency, which is what users feel daily. MongoDB with majority writes is consistency-leaning (the minority side can't take writes) but tunable. Cassandra and DynamoDB default to availability and latency with tunable consistency.

### 6. How does Raft guarantee a new leader has all committed entries?
**Model answer:** An entry is committed only once stored on a majority. A candidate must get votes from a majority, and voters reject candidates whose logs are less up to date than their own. Any two majorities overlap, so at least one voter has every committed entry and won't vote for a candidate missing it. Terms fence off stale leaders.

## MongoDB

### 7. Design a shard key for a multi-tenant SaaS app with tenants of very different sizes.
**Model answer:** Start from the queries: almost all are scoped to a tenant. A compound key like `{tenantId: 1, _id: 1}` (or `{tenantId: 1, createdAt: 1}` if time-range queries dominate) gives targeted queries per tenant. The second field gives cardinality so large tenants split across many chunks, avoiding jumbo chunks, and inserts spread because tenant IDs are distributed. If one tenant is enormous and write-hot with monotonic IDs, consider hashing the second field or a hashed prefix within the tenant. Add zone sharding if tenants have residency needs. Mention resharding as a safety net.

### 8. A query got slow over time. Walk through your investigation.
**Model answer:** Reproduce it and get `explain("executionStats")`. Look for COLLSCAN, the docsExamined/nReturned ratio, and in-memory SORT stages. Check the Atlas Performance Advisor and Query Insights, and the profiler for frequency. Check index usage with `$indexStats`, and working set vs WiredTiger cache (cache eviction and page faults). Look for growing arrays or document sizes (an anti-pattern), and whether it's a scatter-gather query on a sharded cluster. Then fix: an ESR index, schema change (Subset or Bucket), a projection, or targeting the shard key. Measure before and after.

### 9. Embed or reference: an order and its line items? A product and its reviews?
**Model answer:** Line items: **embed**. They're bounded, read with the order, and an immutable snapshot. Reviews: **reference** in a separate collection with `productId` (unbounded, read paginated), plus the Subset pattern (latest 10 embedded) and the Computed pattern (count and average on the product).

## Databricks and Lakehouse

### 10. How does Delta Lake provide ACID on object storage?
**Model answer:** Data is immutable Parquet files. The `_delta_log` holds ordered JSON commits listing files added and removed, with periodic Parquet checkpoints. A write produces new files, then atomically creates the next log entry. Concurrent writers use optimistic concurrency: if two race for the same version, one wins, and the other checks for logical conflicts and retries or fails. Readers reconstruct a snapshot from the log (snapshot isolation). The log also enables time travel, statistics for data skipping, schema enforcement and the change data feed.

### 11. A Spark job went from 20 minutes to 3 hours. Troubleshoot.
**Model answer:** Check what changed (data volume, code, cluster config, runtime). In the Spark UI, find the slow stage and compare task duration distribution (max vs median means skew), shuffle read/write sizes, spill to disk, GC time, and the SQL plan (did a broadcast join become sort-merge? Is a filter not pushed down?). Common fixes: AQE skew handling or salting, broadcasting small tables, fixing small files with OPTIMIZE or liquid clustering, refreshing statistics, removing Python UDFs, right-sizing partitions and compute, or moving to serverless and Photon.

### 12. Design an SCD Type 2 dimension with MERGE.
**Model answer:** A dimension with business key, attributes, `valid_from`, `valid_to` and `is_current`. For each batch of changes: MERGE on the business key where `is_current = true`. When matched and attributes differ, expire the old row (set `valid_to` and `is_current = false`), and insert the new version (often via a union of the staged updates with a null merge key, so new versions go through the NOT MATCHED branch). Or use declarative `AUTO CDC ... STORED AS SCD TYPE 2` in Lakeflow pipelines.

## Red Hat and Cloud-native

### 13. Operator vs Helm chart vs StatefulSet?
**Model answer:** A StatefulSet gives pods stable identities and per-pod volumes. Helm templates and installs resources (packaging, day 1). An Operator runs a reconciliation loop with domain knowledge: failover, backups, upgrades and scaling (day 2), via CRDs. For production databases you want an operator such as CloudNativePG or Crunchy, often installed via OLM and managed with GitOps.

### 14. Design Postgres HA on OpenShift across two data centres with RPO=0.
**Model answer:** An operator-managed cluster with instances spread across both sites (topology spread constraints), **synchronous replication** to at least one standby in the other site (RPO=0), and automatic failover via the operator. Quorum needs a third failure domain (a lightweight witness or arbiter site, or a stretched control plane with three zones) to avoid split brain. Add continuous WAL archiving to object storage for PITR, OADP for namespace backups, and ODF or storage replication as an additional layer. Discuss the latency cost of synchronous commits between sites, and test failover regularly.

### 15. Explain the outbox pattern with Debezium.
**Model answer:** The service writes the business change and an event row in an outbox table in the same local transaction. Debezium reads the WAL via logical decoding, picks up committed outbox rows, and its outbox event router publishes them to Kafka topics keyed by aggregate ID. This guarantees the event is published if and only if the transaction committed, with per-aggregate ordering, no dual writes and no 2PC. Consumers must be idempotent (at-least-once delivery).

## System design quick-fire

16. *"Design a URL shortener."* Key-value access by short code, a base62 ID from a sequence or a Snowflake-style ID generator, caching for hot links, async click analytics into a lakehouse, and a TTL for expiring links.
17. *"Design a leaderboard for 50M players."* Redis sorted sets for the real-time top N, MongoDB for profiles and history, per-region or sharded leaderboards, and periodic snapshots.
18. *"Design an IoT platform for 1M devices sending data every 10s."* Ingest via MQTT/Kafka (100k msgs/s), time series collections or Bucket pattern for recent data, the lakehouse for history and ML, and downsampling and TTL.
19. *"Design a chat app's message storage."* Partition by conversation ID, time-ordered message IDs, an append-heavy workload (an LSM store or MongoDB with `{convId, ts}`), and per-conversation pagination.

## Behavioural (prepare STAR stories)

20. Tell me about a time you turned around a failing customer engagement.
21. Describe a technical decision you made with incomplete information.
22. When did you say no to a customer or salesperson, and why?
23. Tell me about a time you learned a complex technology quickly. *(This portal is your answer: what you built, what you learned and how you'd apply it.)*
24. How do you keep up with a fast-moving ecosystem?
25. Why are you leaving AWS, and why us specifically? *(Positive framing: depth in data and AI, a passion for the product, the customer impact you want to have.)*

> **SA angle:** For every technical answer, end with **"what this means for the customer"**: "…so the customer gets correct balances without paying the latency of serializable everywhere." That habit is what makes an SA answer stand out from an engineer's.

```quiz
Q: In an SA interview, what's the best way to end a technical answer?
- With a list of product features
+ By connecting it to customer impact or the business trade-off
- By asking the interviewer if they agree
- With a joke
? Translating depth into customer value is the core SA skill being assessed.

Q: Which is the best evidence for "tell me about learning a technology quickly"?
+ A concrete project: e.g. building an LSM-tree and MVCC store in Go, what was hard, what you learned and how you'd use it with customers
- "I read a lot of blogs"
- "I have many certifications"
- "I learn fast"
? Specific, demonstrable outcomes beat claims. Your labs and demos are the story.
```
