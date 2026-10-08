## A playbook, not a guess

"The database is slow" is a symptom. It is not a diagnosis. The same sentence covers a missing index, a connection storm, an idle-in-transaction session, a working set that no longer fits in RAM, and an application that never reached the database at all.

The method is always the same: **measure the user's time, decide whether the database is on the critical path, decide whether one query or everything is slow, then name the resource that is saturated.** Only then do you pick a suspect. Jumping to `CREATE INDEX` is how you ship a second problem.

> **Key idea:** Latency is a budget. The database only owns the slice it actually spent. Pool wait, DNS, GC, JSON mapping and the next HTTP hop sit in the same request and look like "the DB" in a naïve span.

## First: is it even the database?

Open a trace for a slow request and split the time. You want three numbers: time to acquire a connection, time inside the database, and time after the rows come back.

![The app's "DB call" span includes pool wait, network and ORM work that the database never sees. Measure the user's p95, then split the bar.](diagrams/problem-slow-database-time-budget.svg)

If the orange slices (pool wait, lock wait) dominate, tuning SQL will not help. If "CPU + I/O" is a few milliseconds and the user waited 800 ms, look at the application, the pool, a downstream API, or a client-side waterfall.

```sql
-- Lab: psql postgres://dbm:dbm@localhost:5442/dbm
-- What is running right now, and what is it waiting for?
SELECT pid, usename, state, wait_event_type, wait_event,
       now() - query_start AS running_for,
       left(query, 72) AS query
FROM   pg_stat_activity
WHERE  datname = current_database()
  AND  pid <> pg_backend_pid()
ORDER  BY running_for DESC NULLS LAST;
```

`state = 'idle'` is normal. `idle in transaction` is not: that session is holding locks and pinning a snapshot. `wait_event_type` tells you whether the backend is on CPU, I/O, `Lock`, `LWLock`, `Client` or `IPC`.

On MongoDB the same first cut is `$currentOp` plus the Atlas profiler (or `db.setProfilingLevel(1, { slowms: 100 })` in the lab):

```js
db.getSiblingDB("admin").aggregate([
  { $currentOp: { allUsers: true, idleConnections: false } },
  { $match: { active: true, secs_running: { $gte: 1 } } },
  { $project: {
      op: 1, ns: 1, secs_running: 1, waitingForLock: 1,
      planSummary: 1, "command.filter": 1
  }}
])
```

> **Remember:** `pg_stat_statements` and the MongoDB profiler see *database* time. They will not show a goroutine blocked on `pool.Acquire`. That is why the time-budget bar comes first.

## Then: one query, or everything?

If one endpoint or one statement is slow, you have a plan problem. If every query slowed down together, something shared is saturated: CPU, I/O, locks, connections, or the buffer cache.

![First decide whether the database is on the path at all, then whether one query or everything is slow. The USE method names the saturated resource.](diagrams/problem-slow-database-triage.svg)

**USE** (utilisation, saturation, errors) is the cheapest way to pick a column:

| Resource | Utilisation | Saturation | Errors |
|---|---|---|---|
| CPU | `%usr` / `pg_stat_activity` on CPU | run queue, `CPU` wait events | — |
| Disk | IOPS and throughput vs the volume's cap | I/O queue depth, `DataFileRead` | EBS credit exhaustion, disk full |
| Locks | time in `wait_event_type = 'Lock'` | lock queue length | deadlocks (`40P01`) |
| Connections | `numbackends / max_connections` | clients waiting on the pooler | `too many connections` |
| Memory / cache | buffer hit ratio, `shared_buffers` | eviction, swap | OOM kills |

A 4-vCPU instance at 95% CPU with a long run queue is a *different* incident from a 4-vCPU instance at 15% CPU and a lock queue of forty. Both feel like "the database is slow".

## The toolkit

You need four views. Learn them in this order; they answer different questions.

### 1. `pg_stat_statements` — what is expensive in aggregate

The lab image already loads `pg_stat_statements`. Create the extension once, then rank by **total** time, not by mean: a 2 ms query at 20,000 calls/s beats a 200 ms query that runs twice an hour.

```sql
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

SELECT  calls,
        round(total_exec_time::numeric, 1)  AS total_ms,
        round(mean_exec_time::numeric, 2)   AS mean_ms,
        rows,
        left(query, 80) AS query
FROM    pg_stat_statements
WHERE   dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
ORDER BY total_exec_time DESC
LIMIT 15;
```

`calls` exploding on the same query shape is the N+1 signature. `mean_exec_time` jumping after a release is a plan or data-shape change.

### 2. `EXPLAIN (ANALYZE, BUFFERS)` — why *this* statement is slow

`EXPLAIN` without `ANALYZE` is a guess. `ANALYZE` runs the query. `BUFFERS` tells you whether it was a cache hit or a disk read. Always compare **estimated rows** to **actual rows**.

```sql
-- Lab: 20k orders. No index on created_at, so this is a sort + seq scan.
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, customer_id, status, created_at
FROM   orders
WHERE  created_at > now() - interval '7 days'
ORDER  BY created_at DESC
LIMIT  20;
```

Then add the index the leftmost-prefix rule wants and re-run:

```sql
CREATE INDEX IF NOT EXISTS orders_created_at_idx ON orders (created_at DESC);

EXPLAIN (ANALYZE, BUFFERS)
SELECT id, customer_id, status, created_at
FROM   orders
WHERE  created_at > now() - interval '7 days'
ORDER  BY created_at DESC
LIMIT  20;
```

> **Try it:** Compare `Shared Hit` vs `Shared Read` in the two plans. Hits are buffer-cache pages; reads are OS/disk. A plan that is "fast" on a warm cache and terrible on a cold one is a working-set problem in disguise.

On MongoDB the equivalent is `explain("executionStats")`. You want `nReturned`, `totalKeysExamined` and `totalDocsExamined` within a small factor of each other, and you do not want `COLLSCAN` on a large collection.

```js
db.orders.find({ customer_id: 42 }).sort({ created_at: -1 }).limit(20)
  .explain("executionStats")
```

The planner, join algorithms and how to read a plan are in [Query processing](#/lesson/query-processing). Index shape is in [Indexes](#/lesson/indexes).

### 3. Locks — who is waiting on whom

```sql
SELECT blocked.pid  AS waiting_pid,
       blocking.pid AS blocking_pid,
       blocked.wait_event,
       now() - blocked.query_start AS waited,
       left(blocked.query, 56)  AS waiting_for,
       left(blocking.query, 56) AS blocker,
       blocking.state
FROM   pg_stat_activity blocked
JOIN   pg_stat_activity blocking
         ON blocking.pid = ANY (pg_blocking_pids(blocked.pid));
```

The classic outage is not two `UPDATE`s on the same row. It is a forgotten `BEGIN` in a laptop `psql`, or an ORM that starts a transaction and then calls a payment API. That session holds an `AccessShareLock`. A later `ALTER TABLE` needs `AccessExclusiveLock`, so it waits. Every new query on that table queues behind the `ALTER`.

![An idle-in-transaction session holding a weak lock stalls a later ALTER; every new query on the table queues behind the ALTER, not behind the idle session.](diagrams/problem-slow-database-lock-queue.svg)

Mitigations you can name in an interview: `idle_in_transaction_session_timeout` (start around 30s), `lock_timeout` on the session that runs DDL, and `statement_timeout` on the application role. Then terminate the blocker.

```sql
-- After you have identified the pid. Do not guess.
-- SELECT pg_terminate_backend(12345);
SHOW idle_in_transaction_session_timeout;
SHOW lock_timeout;
SHOW statement_timeout;
```

### 4. Bloat and the working set

Postgres MVCC leaves dead tuples in the heap until `VACUUM` reclaims them. A long snapshot — including idle-in-transaction — prevents that cleanup. Scans then read empty space; indexes grow; the working set no longer fits in `shared_buffers`.

```sql
SELECT relname,
       n_live_tup,
       n_dead_tup,
       last_autovacuum,
       last_autoanalyze
FROM   pg_stat_user_tables
ORDER  BY n_dead_tup DESC;

-- Cache hit ratio for this database. 99% vs 95% is not "a bit worse".
SELECT datname,
       blks_hit, blks_read,
       round(100.0 * blks_hit / NULLIF(blks_hit + blks_read, 0), 2) AS hit_pct
FROM   pg_stat_database
WHERE  datname = current_database();
```

![Latency stays flat while the working set fits in RAM, then climbs as misses become random reads, then queues once IOPS are exhausted. A hit ratio drop from 99.9% to 95% is a 50× rise in disk reads.](diagrams/problem-slow-database-working-set.svg)

That 50× figure is arithmetic, not a benchmark: miss rate went from 0.1% to 5%. The working set is "the pages you touch in a typical interval". If it exceeds RAM, every extra miss is a random read, and on a burstable volume those reads spend burst credits.

Bloat, snapshots and why VACUUM exists are in [MVCC and concurrency](#/lesson/mvcc-concurrency). Process model, `shared_buffers` and pooling are in [PostgreSQL deep dive](#/lesson/postgres-deep-dive).

## The eight usual suspects

Once you know which resource is saturated, you are choosing from a short list. Most incidents are one of these.

![Eight causes cover most "the database is slow" incidents. Start from the saturation you measured; do not start by adding an index.](diagrams/problem-slow-database-eight-suspects.svg)

| Suspect | What you see | First move |
|---|---|---|
| Missing or wrong index | `Seq Scan`, `COLLSCAN`, `Rows Removed by Filter` in the millions | Composite index that matches the filter and sort; see [Indexes](#/lesson/indexes) |
| Bad plan / stale stats | `rows=` estimate off by 10× or more; plan flipped after a release | `ANALYZE`; look at bind-parameter skew |
| Working set > RAM | hit ratio falling, `DataFileRead`, evictions | more RAM, less bloat, a covering index, or less data in the hot path |
| Lock / idle in transaction | `wait_event_type = Lock`, `idle in transaction` | terminate the blocker; set timeouts; shorten transactions |
| Connection storm | `too many connections`, pool wait in the app | pooler, smaller per-pod pools; [Go database fundamentals](#/lesson/go-database-fundamentals) |
| N+1 | thousands of identical statements in `pg_stat_statements` | join, `IN` / `= ANY`, or a loader; [Scaling hotspots](#/lesson/problem-scaling-hotspots) |
| Vacuum / bloat | `n_dead_tup` large, fat indexes, autovacuum falling behind | fix the long transaction, then tune autovacuum on the hot table |
| Noisy neighbour / I/O credits | IOPS at the volume cap, burst balance empty, *other* tenants noisy | move off burstable storage, isolate the tenant, or cut the read amplification |

> **SA angle:** In a customer call you narrate this list out loud. "I'm checking whether it's one query or the instance, then CPU vs I/O vs locks vs connections." That sentence is the difference between an engineer who SSH-es in and an SA who can run a war-room.

## Scenarios

### Problem: Checkout p95 jumps every weekday at 09:00

> **Real world:** "Checkout is fine at 08:55 and unusable at 09:05. CPU on the primary is only 40%. The APM says 'DB' is 700 ms. It started after we doubled the number of API pods."

**Symptoms.** p95/p99 climb on one endpoint; average looks acceptable. `pg_stat_activity` shows many `idle` backends and a few active ones. The app's DB span is long; `pg_stat_statements` mean time is not.

**Diagnose.** Split the time budget. Compare `numbackends` to `max_connections`. Check pool wait in the client (`pgxpool` `Acquire` wait, Hikari, PgBouncer `cl_waiting`). Count `calls` for the checkout statements — if one query shape jumped by a factor of pods, you have a storm or an N+1.

```sql
SHOW max_connections;
SELECT count(*) FILTER (WHERE state = 'active')  AS active,
       count(*) FILTER (WHERE state = 'idle')    AS idle,
       count(*) FILTER (WHERE state = 'idle in transaction') AS idle_in_tx,
       count(*) AS total
FROM   pg_stat_activity
WHERE  datname = current_database();
```

**Root causes.** Most likely a **connection storm**: 40 pods × `MaxConns=20` = 800 backends against a `max_connections` of 200, or against a 4-vCPU primary that wanted about 10 busy connections (Little's law: 2,000 qps × 5 ms ≈ 10). Next: N+1 on the order lines. Least likely: a missing index that only hurts at 09:00.

**Fix.** Cut per-pod pool size so the *cluster* total fits the instance. Put **PgBouncer** in transaction mode in front of Postgres. Load the checkout in one query (join or `= ANY`). Immediate mitigation: shed load, scale the pooler, not the primary.

**Prevent.** A dashboard of backends, pool wait and `max_connections` headroom. Cap `MaxConns` in the Helm chart. Connection storms are covered again in [Scaling hotspots](#/lesson/problem-scaling-hotspots) and [Go database fundamentals](#/lesson/go-database-fundamentals).

> **Model answer — what to say in the interview:** I would not start with EXPLAIN. I would open a trace and split pool wait from database time. If pool wait dominates and backend count tracks pod count, it is a connection storm: each Postgres connection is a process, so I size pools for the instance, not per pod, and put PgBouncer in transaction mode in front. I would also check pg_stat_statements for a query shape whose call count equals items per cart — that is N+1. Then I would re-measure p95, not CPU.

### Problem: One merchandising report now takes forty seconds

> **Real world:** "The overnight report was two seconds last month. It is forty seconds today. OLTP traffic looks fine."

**Symptoms.** One statement. `pg_stat_statements` shows a huge `mean_exec_time` and a modest `calls`. CPU or I/O rises only while it runs.

**Diagnose.** `EXPLAIN (ANALYZE, BUFFERS)` on the statement. Look at `Seq Scan` vs index, estimated vs actual rows, and whether a hash join spilled (`Sort Method: external merge` or a temp-file bump in `pg_stat_database`). Check `pg_stat_user_tables.last_autoanalyze` and `pg_stats.n_distinct`.

```sql
SELECT attname, n_distinct, most_common_vals
FROM   pg_stats
WHERE  tablename = 'orders';

SELECT last_autoanalyze, n_live_tup, n_dead_tup
FROM   pg_stat_user_tables
WHERE  relname = 'orders';
```

**Root causes.** Missing or misordered index; **stale statistics** after a bulk load; a parameter that used to be selective (`status = 'pending'`) and now matches half the table; a join order flip.

**Fix.** Immediate: `ANALYZE orders;` and confirm the plan. Durable: the right composite index, a partial index for the rare status, or move the report off the primary (replica, or the lakehouse). Do not add an index for a daily scan of 40% of the table — a sequential scan is the right plan.

**Prevent.** `ANALYZE` after `COPY` and large backfills (the [Postgres lab](#/lesson/lab-postgres) does this). Autovacuum/analyze thresholds on the growing table. A statement timeout on ad-hoc roles.

> **Model answer — what to say in the interview:** One slow statement is a plan problem until proven otherwise. I run EXPLAIN (ANALYZE, BUFFERS), compare estimated and actual rows, and check when ANALYZE last ran. If the estimate is off I refresh statistics before I add indexes. If it is a genuine miss I design a composite index for the filter and sort, or I move a heavy report off the OLTP primary. I would not cache a forty-second scan without knowing it is reused.

### Problem: Everything queued during a "fast" migration

> **Real world:** "We added a nullable column. The migrate job hung, then the API timed out. We rolled back the deploy but the site stayed down until someone killed a session."

**Symptoms.** All queries on one table pile up. CPU is low. `wait_event` is `relation` / `Lock`. One backend is `idle in transaction` or is the `ALTER TABLE` itself.

**Diagnose.** The blocking-tree query above. Identify the lock types: `AccessShareLock` (the idle reader) vs `AccessExclusiveLock` (the DDL). Check how long the idle transaction has been open.

**Root causes.** Idle-in-transaction from a forgotten session or an ORM that held a transaction across a network call. DDL that needed an exclusive lock. A long `CREATE INDEX` without `CONCURRENTLY`.

**Fix.** Immediate: `pg_terminate_backend` on the idle blocker; cancel the DDL if it is the queue head. Durable: `lock_timeout` on migrators, `CREATE INDEX CONCURRENTLY`, expand/contract migrations (see [zero-downtime schema changes](#/lesson/problem-zero-downtime-migrations)), and `idle_in_transaction_session_timeout`.

**Prevent.** Migrations that take only locks they can get quickly. No transaction around "call Stripe". Observability on `idle in transaction` count and max age.

> **Model answer — what to say in the interview:** Low CPU plus a growing lock queue is almost never "needs a bigger instance". I look for idle-in-transaction and for DDL waiting on AccessExclusiveLock. I terminate the blocker, set lock_timeout on the migrator, and use CREATE INDEX CONCURRENTLY. The durable fix is shorter transactions and an expand/contract migration so we never take a lock that queues the whole table.

### Problem: Latency sawtooth on a burstable volume

> **Real world:** "p99 is 8 ms for twenty minutes, then 400 ms for five, then it recovers. Disk queue depth spikes in the bad window. We didn't change the queries."

**Symptoms.** Everything slows together. Hit ratio may still look "fine" (95%). IOPS sit at a suspiciously round cap. Burst balance (gp2-style credits, or a cloud "I/O credit" graph) drains and refills.

**Diagnose.** Working-set diagram: are you past the RAM line, the IOPS line, or both? `blks_read` rate, `wait_event = DataFileRead`, volume IOPS vs provisioned. Check bloat so you are not reading dead tuples. Check whether a new tenant or a sequential dump is the noisy neighbour.

**Root causes.** Working set grew past RAM (new index, new tenant, bloat). Burstable storage spent its credits. A neighbour on shared I/O (another database on the same host, a `pg_dump` of a huge table, a vacuum that finally caught up).

**Fix.** Immediate: stop the dump / vacuum if it is the spike; raise IOPS or move off burstable disks; add RAM if the working set is the cause. Durable: provisioned IOPS, isolate noisy tenants, cut read amplification (covering indexes, less `SELECT *`), keep autovacuum current so the working set is live rows.

**Prevent.** Alert on burst balance and on hit-ratio *change*, not on a static 99%. Don't share a burstable volume between OLTP and analytics dumps.

> **Model answer — what to say in the interview:** A sawtooth with a hard IOPS ceiling is queueing, not a bad plan. I check whether the working set still fits in RAM — a hit ratio that slipped from 99.9% to 95% is fifty times more disk reads — and whether the volume has burst credits. The fix is more cache, less bloat, or provisioned IOPS, not another index on an already-cached table.

## What to say when they hand you a laptop

A crisp sequence you can recite:

1. Reproduce one slow request. Split **user time** vs **database time** vs **pool wait**.
2. One query or everything? `pg_stat_statements` + `pg_stat_activity` (or `$currentOp`).
3. Name the saturated resource (USE).
4. If one query: `EXPLAIN (ANALYZE, BUFFERS)` / `explain("executionStats")`. Estimated vs actual. Index or stats.
5. If everything: locks, connections, hit ratio, IOPS, bloat — in that order if you have no better hint.
6. Change one thing. Re-measure p95.

> **Interview:** They will try to trap you into "I'd add an index". Name the method first. The index is step 4, and only if the plan says so.

```quiz
Q: A request's APM span labelled "DB" is 600 ms, but pg_stat_statements shows the SQL took 8 ms. What should you look at first?
- A missing index on the table
+ Time spent waiting for a pooled connection, plus network and ORM mapping
- VACUUM FULL
- Increasing shared_buffers
? The database only accounts for the time it ran. Pool wait and client-side work sit in the same span and are invisible to pg_stat_statements.

Q: Why is a buffer-cache hit ratio falling from 99.9% to 95% a much bigger deal than it sounds?
- It means 5% of rows are corrupt
+ The miss rate rose from 0.1% to 5%, so disk reads can rise about 50× for the same traffic
- Postgres stops using indexes below 99%
- 95% is below the default shared_buffers
? Hit ratio is the complement of miss rate. Small drops in a high ratio multiply I/O.

Q: Which wait is the typical signature of a forgotten BEGIN in a laptop session that then blocks a migration?
- DataFileRead
- ClientRead
+ idle in transaction, with later backends waiting on Lock
- IO:WALWrite
? An open transaction holds locks and a snapshot. DDL that needs AccessExclusiveLock queues, and every new query on that table queues behind the DDL.

Q: You need to know which statements burn the most time in production. Which view do you start with?
- pg_locks
+ pg_stat_statements ordered by total_exec_time
- EXPLAIN without ANALYZE
- pg_stat_replication
? Total time (calls × mean) finds both "death by a thousand cuts" and true heavy queries. EXPLAIN is for one statement once you have a candidate.

Q: A MongoDB find() is slow. Which explain fields tell you the index is not doing its job?
- only nReturned
- wiredTiger.cache.used
+ totalDocsExamined and totalKeysExamined much larger than nReturned, or a COLLSCAN stage
- rs.status().ok
? A healthy plan examines about as many keys and documents as it returns. COLLSCAN or a huge examined/returned ratio means the index (or the schema) is wrong.

Q: Checkout is slow only when you scale API pods from 10 to 40. CPU on Postgres is 30%. What is the most likely cause?
- Stale statistics
- A sequential scan on products
+ Connection storm: per-pod pools × pod count exceeded what the instance or max_connections can take
- Need for a covering index on every table
? Pod count multiplies pool size. Postgres pays for each connection as a process. Low CPU plus high pool wait is the signature.
```
