## The shape of a hotspot

A system that is "fine at 100 qps" fails at 1,000 for a handful of boring reasons. The instance is not too small. **One key, one page, one connection pool or one query pattern** is taking all the work. This lesson is those patterns and the fixes you can actually ship.

Cross-links you will need: [Indexes](#/lesson/indexes) for composite keys and leftmost prefix, [Partitioning](#/lesson/partitioning) for shard keys and salting, [PostgreSQL deep dive](#/lesson/postgres-deep-dive) for process-per-connection and PgBouncer, [Go database fundamentals](#/lesson/go-database-fundamentals) for pool sizing.

## Connection storms

Postgres forks a backend process per connection (roughly 5–10 MB of RAM, plus lock-table and `work_mem` headroom). A 4-vCPU primary wants on the order of **tens** of busy connections, not hundreds. Little's law: 2,000 queries/s × 5 ms service time ≈ 10 connections busy. Everything else is idle overhead or queueing.

The storm happens when you multiply:

```text
40 pods  ×  MaxConns=20  =  800 potential backends
max_connections = 200     →  "too many connections" or a 30-second startup brownout
```

Kubernetes makes this worse: a rolling deploy starts a new ReplicaSet before the old one dies, so you briefly have 2× pods. Each new process opens its pool eagerly.

**Fix.** Size the *cluster* total, not the pod. Cap `MaxConns` so `pods × MaxConns` (including surge) stays under what the primary can take. Put **PgBouncer** (or RDS Proxy, or a similar pooler) in **transaction** mode in front: thousands of client sessions multiplex onto tens of server backends. Session mode is required only if you need session state (`SET`, `LISTEN`, temp tables, advisory locks).

```go
// Lab DSN: postgres://dbm:dbm@localhost:5442/dbm?sslmode=disable
cfg, _ := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
cfg.MaxConns = 8          // per process — multiply by replica count yourself
cfg.MinConns = 2
cfg.MaxConnIdleTime = 5 * time.Minute
pool, err := pgxpool.NewWithConfig(ctx, cfg)
```

```sql
SHOW max_connections;
SELECT count(*) AS backends FROM pg_stat_activity;
```

> **Gotcha:** Raising `max_connections` is not the fix. You will fit more idle backends and then run out of RAM or CPU on context switches. Pool, then size.

## N+1 queries

The ORM loads a page of orders, then issues one query per order for the customer or the line items. Ten orders look fine. A page of 50 with 4 items each is 1 + 50 + 200 statements and 250 round trips.

```sql
-- Death by a thousand cuts: 50 extra round trips
SELECT * FROM orders WHERE created_at > now() - interval '1 day';
-- then, in a loop:
SELECT * FROM customers WHERE id = $1;

-- One round trip. Use this, or WHERE customer_id = ANY($1).
SELECT o.id, o.status, o.created_at, c.email, c.name
FROM   orders o
JOIN   customers c ON c.id = o.customer_id
WHERE  o.created_at > now() - interval '1 day';
```

In `pg_stat_statements` the signature is a modest `mean_exec_time` and a huge `calls` on the child query. In traces you see a staircase of identical spans.

**Fix.** Join, `IN` / `= ANY`, or a dataloader that batches by ID per tick. In MongoDB, `$lookup` or embed the fields you always read with the parent (see [the document model](#/lesson/document-model)). Do not "fix" N+1 by opening more connections.

## Hot partitions and hot keys

Hashing a shard key spreads *keys*. It does not spread a **single** key. A flash-sale SKU, a celebrity timeline, or `tenant_id = 'default'` lands on one row, one page and one shard. That shard's CPU, its WiredTiger cache, its Raft group and its row lock become the ceiling.

![Writes to one SKU serialise on a single row. Salting spreads the writes across N sub-keys; the read pays a gather.](diagrams/problem-scaling-hotspots-hot-key.svg)

**Write sharding (salting):** store stock as `SKU-42#0` … `SKU-42#9`. Each checkout decrements a random shard that still has stock. A read of "how many left?" sums the ten documents. You trade a scatter-gather read for write throughput. The same idea is in [Partitioning](#/lesson/partitioning) and the [e-commerce design](#/lesson/design-ecommerce).

Other mitigations, in order of how often they are the right one:

- **Cache the hot read** in front (next lesson) so the row is not hit on every page view.
- **Queue** the flash-sale SKU (a waiting room) so the database sees a smooth write rate.
- **Split** a hot MongoDB chunk or DynamoDB partition (the platform can do this only if the key has enough cardinality — a single document will not split).
- **Don't use a monotonic shard key** (`created_at`, `BIGSERIAL`) as the leading partition key; every insert hits the newest range.

```js
// Decrement a random shard that still has stock.
const salt = Math.floor(Math.random() * 10);
db.inventory.updateOne(
  { _id: `${sku}#${salt}`, available: { $gte: qty } },
  { $inc: { available: -qty } }
)
// matchedCount === 0 → try another salt, or fail
```

> **Warning:** A hashed shard key on `sku` still pins *one* SKU to *one* shard. Salting changes the key. Hashing does not.

## OFFSET pagination

`LIMIT 20 OFFSET 100000` asks the database to walk 100,020 rows and throw 100,000 away. Cost grows with page number. Concurrent inserts also make page 5 of "latest orders" a moving window: rows appear twice or vanish.

![OFFSET walks and discards every prior row, so deep pages get linearly more expensive. Keyset seeks the last seen key and reads only the next page.](diagrams/problem-scaling-hotspots-keyset-offset.svg)

**Keyset** (seek / cursor) pagination: pass the last sort key you showed and ask for the next N rows after it. The sort key must be unique — use `(created_at, id)` if `created_at` can tie.

```sql
-- Lab: 20k orders. Compare the two plans.
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, customer_id, status, created_at
FROM   orders
ORDER  BY id
OFFSET 15000
LIMIT  20;

EXPLAIN (ANALYZE, BUFFERS)
SELECT id, customer_id, status, created_at
FROM   orders
WHERE  id > 15000
ORDER  BY id
LIMIT  20;
```

The first plan still scans (or at least walks the index) to row 15,000. The second is an index seek plus 20 rows. On a million-row table the gap is the incident.

```sql
-- Stable keyset when the UI sorts by time
SELECT id, customer_id, status, created_at
FROM   orders
WHERE  (created_at, id) < ($last_created_at, $last_id)
ORDER  BY created_at DESC, id DESC
LIMIT  20;
```

You lose "jump to page 500". That is usually fine: humans want *next*, and search jumps by filter, not by page number. If a client truly needs random page N, precompute a page map or accept that it is an expensive admin path.

## `COUNT(*)` at scale

`SELECT count(*) FROM orders` is an honest scan (or an index-only scan of every leaf). On a large heap it is seconds, it fights the buffer cache, and dashboards that poll it become a self-inflicted DDoS.

```sql
-- Exact (touches the table)
EXPLAIN (ANALYZE, BUFFERS)
SELECT count(*) FROM orders;

-- Planner estimate: cheap, good enough for "about 20,000"
SELECT reltuples::bigint AS estimate
FROM   pg_class
WHERE  oid = 'orders'::regclass;
```

`reltuples` is updated by `ANALYZE` / autovacuum and is approximate. For a filtered count (`WHERE status = 'paid'`), maintain a **counter table** (or a Redis key) updated in the same transaction as the write, or accept an estimate from a covering partial index's `reltuples` after `ANALYZE`.

MongoDB: `estimatedDocumentCount()` reads metadata; `countDocuments({ status: "paid" })` is a query and needs an index. Atlas / WiredTiger also expose `collStats`.

> **Tip:** "How many orders?" on a home page is a cached estimate with a 10-second TTL. "How many in *this* search?" is a capped `LIMIT` plus `has_more`, not a full count.

## ID generation

The primary key is a write hotspot if it is sequential, and a cache-killer if it is random. You are choosing where the pain goes.

![A sequence appends to the rightmost B-tree leaf. UUIDv4 writes a random leaf and splits pages. UUIDv7, snowflake IDs and ObjectId are time-ordered, so inserts stay local without being a single counter.](diagrams/problem-scaling-hotspots-id-keys.svg)

| Scheme | Shape | Insert locality | Notes |
|---|---|---|---|
| `BIGSERIAL` / sequence | 64-bit monotonic | Excellent (rightmost leaf) | Tiny, sortable. The leaf and the WAL can contend at very high insert rates. One sequence is a single point of increment — usually fine, the B-tree leaf is the bigger hotspot. |
| UUIDv4 | 128-bit random | Poor | No coordination, easy to generate in the app. Random inserts fragment the index, bloat WAL, and waste `shared_buffers` on cold leaves. |
| UUIDv7 | 128-bit, Unix-ms prefix | Good | Time-ordered UUID (RFC 9562). Sortable, unique without a central sequence. Prefer this over v4 as a PK. |
| Snowflake-style | 64-bit: timestamp + worker + seq | Good | Compact, sortable, generated in the app. You must assign worker IDs. |
| MongoDB `ObjectId` | 12 bytes: time + random + counter | Good | Default `_id`. Time-prefixed so inserts append; smaller than a UUID. |

**Rule of thumb.** Default to a sequence or identity column in Postgres, `ObjectId` in MongoDB, and UUIDv7 when the client must mint the id before the write (offline, multi-master, or "create the row and the S3 key in one step"). Do not use UUIDv4 as a clustered or heap-primary key on a write-heavy table without a reason.

```sql
-- Sequences remain the simplest PK on a single-node lab.
-- Mint UUIDv7 in the application when the client must assign the id.
SELECT pg_get_serial_sequence('orders', 'id');
SELECT last_value FROM pg_sequences WHERE sequencename LIKE '%orders%';
```

> **SA angle:** Interviewers love "why is UUID a bad primary key?" The precise answer is "v4 is a bad *clustered / leading index* key because of random I/O; v7 and ObjectId are not the same thing."

## Interview problems

### Problem: After a Kubernetes deploy, Postgres refuses connections

> **Real world:** "We shipped a new API tag. During the rollout half the pods returned `too many connections`. Then it recovered. The new build didn't change any SQL."

**Symptoms.** Errors only during the surge. `numbackends` hits `max_connections`. After the old ReplicaSet dies, everything is fine.

**Diagnose.** Compute `replicas × (1 + surge) × MaxConns`. Check PgBouncer `cl_waiting` vs `sv_active`. Confirm the new build did not raise the pool size "to be safe".

**Root causes.** Rolling update doubled the pods. Each process opened its full pool. No pooler, or the pooler was also sized per pod.

**Fix.** Immediate: slow the rollout (`maxUnavailable` / smaller surge), restart the pooler, temporarily raise `max_connections` only if you must survive the window. Durable: PgBouncer in transaction mode, a hard cap on `MaxConns`, and a connection budget in the Helm chart that includes surge.

**Prevent.** Load-test the deploy, not just the steady state. Alert on backends > 70% of `max_connections`.

> **Model answer — what to say in the interview:** A brownout that tracks a rolling deploy is almost always connections, not SQL. I multiply pod count including surge by the per-process pool and compare it to max_connections and to what the instance can actually run. The fix is a pooler and a cluster-wide budget, not a bigger max_connections. I would mention that Postgres connections are processes, so idle backends still cost RAM.

### Problem: Page 500 of the admin order list times out

> **Real world:** "Support paginates orders twenty at a time. The first pages are instant. Page 500 dies. They need a stable sort by created_at."

**Symptoms.** Time and buffers grow linearly with `OFFSET`. `EXPLAIN` shows an index scan that walks from the start, or a sort of the whole set.

**Diagnose.** The two `EXPLAIN` queries above, with `OFFSET 0`, `OFFSET 10000`, `OFFSET 15000`. If time tracks the offset, it is not "needs more RAM".

**Root causes.** `OFFSET`. Possibly a sort that does not match an index (`ORDER BY created_at` with no `(created_at, id)` index).

**Fix.** Keyset on `(created_at DESC, id DESC)`. Return `next_cursor` to the client. Keep `OFFSET` only for shallow pages if you must.

**Prevent.** Ban `OFFSET` past a few hundred in code review. For "jump to page N", offer a filter (date, id search) instead.

> **Model answer — what to say in the interview:** OFFSET is O(offset + limit). I switch to keyset pagination: WHERE (created_at, id) < (last_created_at, last_id) ORDER BY created_at DESC, id DESC LIMIT 20, with a matching composite index. I give up random page numbers, which support rarely needs. I would also mention that OFFSET is unstable under concurrent inserts.

### Problem: One SKU sold out the primary on Black Friday

> **Real world:** "The flash sale was one product. Inventory is a single row. Writes queued, then checkout 500'd. Other products were fine."

**Symptoms.** One wait event, one hot block, one shard's CPU. `pg_locks` or `currentOp` shows everyone waiting on the same key.

**Diagnose.** Confirm the access pattern: `UPDATE inventory SET stock = stock - 1 WHERE sku = $1`. Check whether the shard key *is* the SKU.

**Root causes.** Hot key. Hashing the SKU does not help. A sequence PK on the orders table is a separate, milder hotspot.

**Fix.** Immediate: a waiting room / queue in front of that SKU; cache the sold-out flag so reads stop hitting the row. Durable: salt the inventory into N documents or rows; reserve stock asynchronously; pre-split stock by warehouse.

**Prevent.** Identify flash-sale SKUs before the event. Load-test *one* key, not a uniform random keyspace.

> **Model answer — what to say in the interview:** A single-row inventory counter is a hotspot by construction. I would shard the counter with a salt, pick a random shard on decrement, and sum on read. For the duration of the sale I would also put a queue in front so the database sees a smooth rate. I would not offer "hash the sku" as the fix — the key is still one value.

### Problem: Pick an ID scheme for a globally writable orders table

> **Interview:** "Clients mint the order id before calling two regions. We need roughly time-ordered ids for pagination. We are on Postgres today and may add MongoDB. What do you use?"

> **Model answer — 2 minutes:** I would not use UUIDv4 as the primary key: random inserts scatter the B-tree and the buffer cache. A sequence is the wrong tool if two regions must mint ids without a round trip. UUIDv7 gives a 128-bit, time-ordered value the client can generate; a snowflake-style 64-bit id is smaller if we can assign worker ids per region. MongoDB's ObjectId is the same idea in 12 bytes and is the default `_id`. I would keyset-paginate on `(id)` if it is time-ordered, or on `(created_at, id)` otherwise. I would mention the rightmost-leaf hotspot of a pure sequence as the reason we don't blindly use BIGSERIAL in a multi-writer design.

```quiz
Q: 30 API pods each open a pool of 20 connections to a 4-vCPU Postgres. What is the main risk?
+ 600 backends far exceed what the instance wants; you will hit max_connections or spend RAM on idle processes
- The primary will run out of disk
- Postgres cannot run on fewer than 8 cores
- The planner will stop using indexes
? Connections are processes. Size the cluster total (and put a pooler in front), not the per-pod pool in isolation.

Q: Why is LIMIT 20 OFFSET 100000 a poor way to fetch "page 5000"?
- OFFSET is not valid SQL
- LIMIT cannot be used with OFFSET
+ The executor must walk and discard 100,000 rows, so cost grows with page depth
- It disables MVCC
? Keyset pagination seeks the last seen unique sort key and reads only the next page.

Q: Hashing the shard key productId still left one flash-sale SKU hot. Why?
- Hash functions are too slow
- You must hash twice
- Product ids cannot be hashed
+ Every write for that SKU still maps to the same hash, so it still lives on one partition
? Hashing spreads different keys. A single hot key remains one key. Salt the key (productId#0..n) to spread writes.

Q: When is UUIDv4 a bad primary key on a write-heavy Postgres table?
+ Random inserts hit random B-tree leaves, causing page splits and cache churn
- UUIDs cannot be indexed
- Postgres rejects UUID primary keys
- UUIDv4 values are not unique
? v4 is random by design. v7, snowflake ids and ObjectId are time-ordered and keep inserts local.

Q: You need "about how many orders exist" for a dashboard every 5 seconds. What should you use?
- SELECT count(*) FROM orders on every poll
+ pg_class.reltuples (or a cached counter), refreshed by ANALYZE or maintained on write
- DROP INDEX
- EXPLAIN ANALYZE of a sequential scan
? Exact count(*) walks the table (or every index leaf). Metadata estimates or a maintained counter are the right tool for a dashboard.

Q: Which PgBouncer pool mode multiplexes best for a typical stateless API?
- session
+ transaction
- statement, and also keep multi-statement transactions
- replication
? Transaction mode holds a server connection only for the duration of a transaction. Statement mode cannot run multi-statement transactions; session mode holds the backend until disconnect.
```
