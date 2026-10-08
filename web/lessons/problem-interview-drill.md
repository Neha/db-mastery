## How to use this drill

Say each answer **out loud**, timed (about two minutes). Then open the model answer and note the gap. The quiz at the end is about *how* you answer, not about memorising these scenarios.

A strong SA answer has the same shape every time:

1. **Clarify** the access pattern, consistency need and scale. State one or two assumptions.
2. **Name the mechanism**, not the brand. "WAL + fsync" beats "we use RDS".
3. **Give one alternative** and why you are not picking it.
4. **End on the customer impact** (correctness, RTO, cost, what you would measure next).

![Pick the store from how the data is read and written, not from what is fashionable.](diagrams/problem-interview-drill-decision.svg)

![Say these latency gaps out loud. A sync cross-region write is two orders of magnitude above a same-AZ commit.](diagrams/problem-interview-drill-latency.svg)

Cross-links sit in the model answers. If a topic feels thin, go to the full lesson before you drill it again.

---

### 1. URL shortener schema

> **Interview:** Design the data model for `https://short.ly/aB3xY9` → a long URL, plus optional expiry and click counts.

> **Model answer:** One primary access: lookup by short code. Table `links(code PK, url, created_at, expires_at)`. `code` is a 7–8 character base62 encoding of a monotonic ID (sequence or Snowflake-style), not a hash of the URL — collisions and "same URL, two owners" are messy. Clicks are not a `UPDATE links SET n = n+1` on the hot row; append to `clicks(code, ts)` or a stream and roll up. Cache the mapping in Redis with a TTL. Analytics land in a lakehouse. See [data modelling](#/lesson/data-modeling) and [system design](#/lesson/system-design-framework).

### 2. How do you store passwords?

> **Interview:** A customer wants to `SHA256` the password in the app and store the hex. What do you tell them?

> **Model answer:** Never store a fast hash of the password. Use a **password-hashing** function — Argon2id or bcrypt — with a unique salt per row and a work factor that costs tens of milliseconds on your hardware. SHA256 and even unsalted SHA256+pepper are brute-forceable on a GPU. Do not encrypt passwords (you would have to decrypt to check). Reset tokens are random, single-use, hashed at rest, short TTL. TLS in flight is assumed, not a substitute. Full walk-through: [passwords, encryption and GDPR](#/lesson/problem-passwords-secrets).

### 3. Slow query every weekday at 9am

> **Interview:** Checkout p99 jumps from 40 ms to 2 s between 09:00 and 09:20, then recovers. The instance is "fine" the rest of the day.

> **Model answer:** Treat it as a **capacity cliff**, not a random slow query. I would ask: is it one statement or everything? What started at 09:00 — a batch, a report, a cache expiry, a connection storm from the app pool recycling? Check `pg_stat_activity` / wait events, `pg_stat_statements` by total time, and whether the working set just fell out of RAM. Usual suspects: a 9am BI scan, autovacuum on a hot table, lock convoy behind a report, pool stampedes after a deploy. Mitigate by moving the batch, adding the missing index, or queueing the report; then make 9am a load test. Playbook: [the database is slow](#/lesson/problem-slow-database).

### 4. Oversold stock

> **Interview:** Two customers buy the last item. Both see "success". How do you stop that?

> **Model answer:** This is a lost update. `READ COMMITTED` plus `SET stock = stock - 1` in the app will lose. Fix in the database: `UPDATE items SET stock = stock - 1 WHERE id = $1 AND stock >= 1 RETURNING stock` and treat zero rows as sold out; or `SELECT … FOR UPDATE` in the same transaction as the order insert. For extreme hot SKUs, a single-threaded queue per SKU or a conditional document update (`stock: { $gte: 1 }`) in MongoDB. Do not "fix" it only in Redis unless Redis is the authority and the DB is a projection. See [overselling and deadlocks](#/lesson/problem-concurrency-bugs) and [transactions](#/lesson/transactions-acid).

### 5. GDPR delete

> **Interview:** A user exercises the right to be forgotten. They exist in Postgres, MongoDB, Redis, logs and the lakehouse.

> **Model answer:** Start from a **data map**, not from `DELETE FROM users`. Production OLTP: delete or irreversibly anonymise in the same transaction as an audit row (who, when, legal basis). Cascades and FKs need an explicit plan — orphan orders usually stay as a non-identifying stub. Caches and search indexes are part of the delete. Backups and object storage are the hard part: you cannot surgically edit every snapshot; you document retention, and you re-apply the delete after restore. Lakehouse: a delete vector / `DELETE` on Delta that you can prove. "We tombstoned the row" is not enough if the email is still in last week's parquet. See [passwords and PII](#/lesson/problem-passwords-secrets) and [Delta Lake](#/lesson/delta-lake-lakehouse).

### 6. Hot key

> **Interview:** One product ID gets 80% of writes during a flash sale. DynamoDB / Cassandra / MongoDB p99 explodes. Everyone else slows down too.

> **Model answer:** Hashing does not save a single key. Spread **writes** with a salt or shard suffix (`sku#0..15`) and aggregate on read; buffer increments in Redis and flush; cache the read path so the partition is not also serving the world. In MongoDB, split the hot chunk; in DynamoDB, adaptive capacity helps *after* you are already hurting. Long-term, that SKU may need its own store or a queue. Discord's time buckets are the same idea: do not let one channel be one unbounded partition. See [partitioning](#/lesson/partitioning) and [hotspots](#/lesson/problem-scaling-hotspots).

### 7. Pagination

> **Interview:** `OFFSET 100000 LIMIT 20` is timing out on the activity feed. Marketing wants "page 5000".

> **Model answer:** `OFFSET` walks and discards rows. Keyset / seek pagination: `WHERE (created_at, id) < ($t, $id) ORDER BY created_at DESC, id DESC LIMIT 20` with an index on that pair. Stable, index-only, no drift if you accept that "page 5000" is the wrong UI — use "load more" or jump by time. If they truly need random page numbers, a precomputed page table or a search engine. Covering index so you do not heap-fetch 20 wide rows. See [indexes](#/lesson/indexes) and [hotspots](#/lesson/problem-scaling-hotspots).

### 8. Cache stampede

> **Interview:** A popular key expires at the top of the hour. The database instantly sits at 100% CPU.

> **Model answer:** Thundering herd: N request handlers miss together and all recompute. Fixes: **single-flight / request coalescing** (one loader, the rest wait — Discord's data services are this); **probabilistic early expiry** so they do not share a clock; **lock around regenerate** (Redis `SET NX` with a short TTL); stale-while-revalidate. Also ask why the key is that hot and that expensive — maybe it should not expire on a wall clock. See [caching](#/lesson/problem-caching).

### 9. SQL vs MongoDB vs Redis

> **Interview:** New product: users, orders, a session cart, and a "customers also bought" widget. One database or three?

> **Model answer:** I split by invariant, not by team fashion. **Orders and stock** want constraints and multi-row transactions → Postgres. **Catalog / "also bought"** is a document-shaped aggregate with flexible attributes → MongoDB (or Postgres JSONB if the team is small). **Session cart** is ephemeral, per-user, TTL → Redis, with the checkout write landing in Postgres. I would *not* put money in Redis or join six collections for the invoice. One platform that does 90% is a valid SA call if the team cannot operate three stores. See the diagram above and [what is a database](#/lesson/what-is-a-database).

### 10. Multi-region writes

> **Interview:** The customer wants users in Sydney and Frankfurt to both write with "no lag" and "no conflicts".

> **Model answer:** Those two requirements fight. Sync cross-region commit is ~60–150 ms — say it. True multi-writer needs conflict resolution (last-write-wins, CRDTs, or merge queues) and some user-visible anomalies. Prefer **geo-partition** (a user or tenant has a home region, strong locally, async elsewhere) plus explicit residency. Global ledgers that must be linearizable belong on Spanner / Cockroach / a single-region primary. I would ask which data *actually* has to be written in both places. See [design a global app](#/lesson/design-global-app) and [CAP](#/lesson/consistency-cap).

### 11. Schema change with no downtime

> **Interview:** Add `email_verified_at` to a 400 million row `users` table in Postgres. No maintenance window.

> **Model answer:** Expand / contract. Add a **nullable** column (fast in modern Postgres; still check the version and whether it rewrites). Deploy app code that writes both old and new meaning. Backfill in batches with a low `lock_timeout` and sleeps; do not one-shot `UPDATE` 400 million rows. Then move reads to the new column, then drop the old. No in-place type change of a used column; no `UPDATE` in the same transaction as a heavy `CREATE INDEX` without `CONCURRENTLY`. See [zero-downtime migrations](#/lesson/problem-zero-downtime-migrations).

### 12. RPO and RTO

> **Interview:** The CFO says "we cannot lose more than five minutes and we must be up in an hour". What do you actually design?

> **Model answer:** **RPO 5 minutes** means a restore path whose *data* is at most five minutes stale — usually continuous WAL / binlog / oplog archive to another failure domain, not a nightly dump. **RTO 1 hour** is the *clock* to serving again: restore, verify, re-point DNS, warm caches. I would write both numbers against a **tested** restore, and say what is excluded (region gone, ransomware, bad deploy). Replicas give you a short RTO for node death; they do not give you an RPO for `DELETE FROM` or a logical bomb. See [backups and DR](#/lesson/problem-backup-dr) and [GitLab's postmortem](#/lesson/case-studies-outages).

### 13. N+1 queries

> **Interview:** The order list page does 1 + 50 queries. It was fine at 10 orders.

> **Model answer:** Classic ORM: load orders, then a query per order for lines or customer. Fix: `JOIN` or `WHERE order_id = ANY($1)`, MongoDB `$lookup` / embed line items (they are bounded), or a dataloader that batches in the request. Measure with a query counter in tests so it cannot regress. N+1 is not a Postgres bug; it is an access-pattern bug that indexes cannot hide once N is 50 and each query is 2 ms. See [query processing](#/lesson/query-processing) and [Go database work](#/lesson/go-database-fundamentals).

### 14. Connection pool sizing

> **Interview:** 40 app pods, `MaxOpenConns = 100` each, Postgres `max_connections = 500`. Intermittent `too many connections` and p99 spikes.

> **Model answer:** 40 × 100 is 4,000 potential backends; Postgres forks a process per connection. Size from **Little's law**: concurrent queries ≈ arrival rate × service time, then add headroom — a 4 vCPU box often wants tens of active connections, not thousands. Put **PgBouncer** (transaction pooling) in front, cap the *sum* of pod pools to what the pooler will hold, and kill idle-in-transaction. See [pool sizing](#/lesson/go-database-fundamentals) and [Postgres deep dive](#/lesson/postgres-deep-dive).

### 15. Write skew

> **Interview:** Two doctors on-call. A constraint says "at least one on call". Both read "two", both go off call, both commit. How?

> **Model answer:** **Write skew** under snapshot isolation / Postgres Repeatable Read: each transaction's snapshot is true, neither updated the other's row, both commit. SSI (`SERIALIZABLE` in Postgres) will abort one; or you take `SELECT … FOR UPDATE` on the on-call set; or you model it as one row (`on_call_count`) so the writes collide. This is the example from *Designing Data-Intensive Applications*. Read Committed does not save you either without a lock or a single-row invariant. See [MVCC](#/lesson/mvcc-concurrency) and [transactions](#/lesson/transactions-acid).

### 16. Shard key choice

> **Interview:** Multi-tenant SaaS. Tenants from 10 rows to 10 billion events. Almost every query is `WHERE tenant_id = ?`.

> **Model answer:** Lead with the query. Compound key `{tenant_id, id}` (or `{tenant_id, created_at}`) so a request is targeted, not scatter-gather. The second field gives cardinality so a huge tenant can split across chunks. Hash-only `tenant_id` keeps a jumbo tenant on one chunk forever. Mention the outlier tenant: salt, dedicated shard, or zone. Notion's workspace ID is this pattern. See [MongoDB sharding](#/lesson/mongodb-replication-sharding) and [partitioning](#/lesson/partitioning).

### 17. `COUNT(*)` timed out

> **Interview:** Admin dashboard does `SELECT COUNT(*) FROM events` and now times out. They want an exact number.

> **Model answer:** Exact `COUNT(*)` on a big heap is a sequential scan (MVCC cannot trust a counter). For an estimate: `pg_class.reltuples` after `ANALYZE`, or a HyperLogLog. For an exact dashboard number: a maintained counter (trigger, incremental aggregate, or a nightly rollup) or a covering index only if the table is still in the "millions, not billions" club. In MongoDB, `estimatedDocumentCount()` vs `countDocuments()`. Ask whether the business needs exact-to-the-row at 09:00. See [hotspots](#/lesson/problem-scaling-hotspots).

### 18. Accidental `DELETE`

> **Interview:** An engineer ran `DELETE FROM orders WHERE created_at > '2024-01-01'` and hit enter. Noticed two minutes later.

> **Model answer:** Stop writes if you can; take a new backup so recovery does not race. If you have **PITR**, restore a clone to *just before* the statement and copy the rows back — do not rewind the primary under a live app if you can avoid it. Delayed replicas exist for this. Logical dumps from last night lose today's good data. Binlog / WAL + `xmin` will not undelete in place. After: revoke ad-hoc prod `DELETE`, require a transaction with `SELECT` first, and add a recycle-bin table or soft-delete if the domain allows. GitLab is the cautionary tale when PITR is a rumour. See [backups](#/lesson/problem-backup-dr).

### 19. Vector search vs keyword

> **Interview:** "We need AI search. We'll just embed every row and drop the inverted index."

> **Model answer:** They solve different queries. **Keyword / inverted index** (Atlas Search, OpenSearch): SKUs, error codes, "order-12345", filters, BM25. **Vector search**: paraphrase, "something like this screenshot", RAG. Most product search is *both* — lexical for precision, vector for recall, with a filter on tenant and ACL *before* or inside the ANN so you do not leak. Embeddings go stale when the document changes; plan the update path. See [specialised databases](#/lesson/specialized-databases) and [RAG design](#/lesson/design-rag-genai).

### 20. When not to cache

> **Interview:** A principal wants Redis in front of every Postgres read. When do you say no?

> **Model answer:** Do not cache what is written more than it is read, what must be linearizable (balances, stock, "did this payment settle"), or what you cannot invalidate (no key design, no TTL story, no CDC). A cache with a 10% hit rate and a 2 ms miss tax makes the p99 worse. Also skip it when the working set already fits in the database buffer pool. Cache is a correctness and ops cost; it has to buy a latency or load win you can measure. See [caching](#/lesson/problem-caching).

### 21. Idempotent payments

> **Interview:** The client retried a `POST /charge` after a 502. The user was billed twice.

> **Model answer:** At-least-once delivery plus a non-idempotent write. Require an **idempotency key** (client-generated UUID) stored uniquely with the charge; the second `POST` returns the first result. The unique index is the invariant; "we checked in Redis" is not. Make the payment provider call idempotent too. 502 after send is the case you design for — you do not know whether the first attempt committed. See [concurrency bugs](#/lesson/problem-concurrency-bugs) and [consensus and sagas](#/lesson/consensus-transactions).

### 22. Tenant isolation leak

> **Interview:** Tenant A saw tenant B's invoices on one page. Row-level security is "on".

> **Model answer:** Treat it as a Sev-1 until proven otherwise. Typical causes: a query that forgot `tenant_id`, a connection pool that **reuses a session** after `SET ROLE` / `SET nls` / a GUCs-based RLS context, a cache key without tenant, a global secondary index scanned without a tenant filter, or an analytics extract that denormalised tenants together. RLS only works if every path sets the context and the pooler does not leak it — transaction pooling plus session-level `SET` is a known foot-gun. Fix: force `tenant_id` in the data API, composite keys, tests that try to read across tenants, and rotate anything that may have been cached. See [multi-tenant SaaS](#/lesson/problem-multi-tenant).

### 23. UUID primary key pain

> **Interview:** They migrated PKs to `gen_random_uuid()`. Writes are slower, the index is huge, and the table is bloated.

> **Model answer:** Random UUIDs as btree PKs **scatter inserts** across leaf pages — cache miss, page splits, poor locality. Prefer `uuidv7` (time-ordered), ULID, or a Snowflake / Instagram-style int64 if you do not need global uniqueness without a coordinator. If you must keep UUIDv4, consider a sequential surrogate PK and a unique UUID column, or BRIN/clustering will not save the random btree. This is the same locality story as Instagram putting time in the high bits. See [indexes](#/lesson/indexes) and [Instagram's IDs](#/lesson/case-studies-scaling).

### 24. Replica-lag UX bug

> **Interview:** User posts a comment, hits refresh, the comment is gone, they post again. Two comments appear later.

> **Model answer:** **Read-your-writes** failure: write went to the primary, refresh hit a replica that had not replayed the WAL. Fixes: read that user's recent writes from the primary; **causal / session token** (MongoDB causal sessions, or "read after LSN X"); pin the user to one replica; or hide "success" until the replica you will read is caught up. Do not "fix" it by making every read go to the primary without talking about write load. See [replication](#/lesson/replication).

### 25. CDC vs dual write

> **Interview:** We need orders in Postgres and in Elasticsearch. The app will write both.

> **Model answer:** Dual write is a distributed transaction you pretend is two `try` blocks — one can succeed. Prefer **CDC**: commit in Postgres, Debezium (or Atlas Stream) reads the WAL / change stream, sink to the index. You get at-least-once delivery, so the sink must be idempotent. Outbox pattern if you need to publish a domain event in the same commit. Search will be slightly behind; say that. See [Kafka and Debezium](#/lesson/kafka-debezium-cdc).

### 26. Lakehouse vs warehouse

> **Interview:** "Shall we buy a warehouse or put everything in the lakehouse?"

> **Model answer:** I ask what they already have on object storage and who must share it. A **warehouse** (Redshift, Snowflake, BigQuery) is excellent SQL on managed storage. A **lakehouse** (Delta / Iceberg / Hudi on S3 plus a query engine) keeps **one copy** for BI, data science and streaming, with ACID and time travel on files. If they already pay for a swamp of parquet and a separate warehouse copy, Databricks' pitch is "stop copying". If they have three analysts and no ML, a warehouse is simpler. See [OLTP vs OLAP](#/lesson/oltp-olap-columnar) and [Delta Lake](#/lesson/delta-lake-lakehouse).

### 27. Kubernetes for databases — yes or no?

> **Interview:** Platform team wants every Postgres on OpenShift. DBA team wants RDS.

> **Model answer:** Not a religious answer. **Yes**, with an **operator** (CloudNativePG, Crunchy, the MongoDB operator), CSI storage that does snapshots, anti-affinity, and a team that has done failover on that stack — especially hybrid / sovereign / many small DBs. **No** (or not yet) for the one 40 TB system of record if the team has never run a StatefulSet through a zone loss. StatefulSet alone is not a database product. See [databases on Kubernetes](#/lesson/databases-on-kubernetes).

### 28. Explain CAP honestly

> **Interview:** "We're a CP shop" / "CAP says pick two."

> **Model answer:** CAP is about a **partition**. You cannot avoid partitions, so you choose **linearizability vs remaining available on both sides**. It is not "pick two of three in normal life". MongoDB with majority writes is CP-leaning: the minority side stops. Cassandra / DynamoDB default AP and reconcile. **PACELC** is the useful half: *else*, you trade latency for consistency every day — that is the 70 ms sync-cross-region conversation. See [consistency, CAP and PACELC](#/lesson/consistency-cap).

### 29. Design a like-counter

> **Interview:** Instagram-style likes. Peak tens of thousands of likes per second on one photo.

> **Model answer:** Do not `UPDATE photos SET likes = likes + 1` on the hot row. Append-only `likes(photo_id, user_id PK)` for the invariant "one like per user", and a **sharded counter** (`photo_id, bucket`) or Redis `INCR` flushed asynchronously for the display number. Read path serves the cache / rollup; the exact table is the source of truth for "have I liked this?". Accept brief drift on the displayed number. Same pattern as the URL-shortener click count. See [concurrency](#/lesson/problem-concurrency-bugs).

### 30. Queue in front of the database

> **Interview:** A partner will burst 50k webhooks in ten seconds. The API writes each to Postgres today and falls over.

> **Model answer:** The database is the worst burst absorber you have. Put a **queue** (SQS, Kafka) in front: ingest is append-only and cheap, workers write at a rate Postgres can take, idempotency keys handle retries. You have traded a hard 500 for a lag that you must expose (or hide behind "accepted"). If they need the row *before* the HTTP 200, you cannot queue the authoritative write — you can only shed or scale. See [pipelines](#/lesson/data-pipelines-streaming).

---

### 31. "We have replicas, so backups can wait"

> **Interview:** A director wants to drop the backup budget because they run three replicas.

> **Model answer:** Replicas copy **mistakes at line rate** — `DROP TABLE`, ransomware, bad migrations, wraparound. GitHub's replicas did not help with split history; GitLab's replica was already broken; Sentry's replica would have refused writes too. Backups plus a **restore you timed last month** are the control for those. Replicas are for node and AZ loss. I would not trade them off against each other. See [outages](#/lesson/case-studies-outages).

### 32. Explain isolation in one minute

> **Interview:** Which isolation level for seat booking vs a dashboard?

> **Model answer:** Seat booking: prevent two tickets for one seat — `SERIALIZABLE` or an explicit lock / unique constraint on `seat_id`. Dashboard: a slightly stale snapshot is fine — `REPEATABLE READ` / snapshot so the report does not see half of a load. Default `READ COMMITTED` is the wrong answer only if I cannot name the anomaly I am accepting. See [transactions](#/lesson/transactions-acid).

---

> **SA angle:** If you freeze, buy time with the framework: "Two assumptions, then the access pattern, then one store, then what breaks." Interviewers mark the *method* more than whether you picked MongoDB or Postgres.

```quiz
Q: What should you do in the first 20 seconds of a scenario question?
- Name the database you like most
+ Restate the access pattern, scale and consistency need, and say what you are assuming
- Draw a multi-region Kafka mesh
- Ask which company they want you to pitch
? Method first. A wrong store with a clear assumption is a better SA answer than a silent leap to a product.

Q: How do you handle a question where you do not remember a vendor feature name?
- Guess a feature name confidently
- Say you have not used databases
+ Describe the mechanism (WAL, quorum, PITR) and the trade-off; offer the vendor term if you know it
- Change the subject to a lab you built
? Interviewers are testing whether you reason. Mechanisms survive rebrands; feature names do not.

Q: Why end a technical answer with customer impact?
- It fills the remaining time
+ The SA loop scores whether you can turn a mechanism into a risk, cost or outcome the buyer cares about
- It avoids follow-up questions
- Customers never want the technical detail
? "So checkout cannot oversell, and we pay a few milliseconds for the row lock" is the SA sentence. Then stop.

Q: A question is really a product pitch in disguise. What do you do?
- List every SKU on the pricing page
+ Give a fair trade-off, pick a default for *this* access pattern, and only then map it to Atlas, Databricks or OpenShift
- Refuse to mention any vendor
- Recommend the opposite vendor to look independent
? Platform knowledge is required; forcing it before the requirement is what interviewers punish.

Q: You realise halfway that you recommended sharding too early. What is the recovery?
- Double down so you look decisive
+ Say so, name the single-node limit you would measure first, and shrink the design
- Restart the entire answer from zero
- Blame the interviewer for a vague prompt
? Course-correcting with a metric is maturity. Instagram and Notion both published "we stayed on one primary until X".

Q: Which of these is the weakest close to a two-minute answer?
- "I would measure replica lag and the miss rate before adding Redis."
- "Assumption: one region, 2k writes/s; if that is wrong the store changes."
+ "We should use the cloud-native AI-ready modern stack going forward."
- "The risk is double-charge; the control is a unique idempotency key."
? Empty adjectives are not a trade-off. Close on a control, a measurement or a named assumption.
```
