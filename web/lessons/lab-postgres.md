## What you'll do

Run a real Go program against PostgreSQL 16 that demonstrates the concepts from the Foundations track. Read the code, run each command, then complete the exercises.

```bash
make up                         # start the databases
go run ./labs/postgres          # list commands
go run ./labs/postgres all      # run everything
```

| Command | Concept | Source |
|---|---|---|
| `setup` | Schema, constraints, bulk load with `COPY` | `labs/postgres/setup.go` |
| `crud` | Placeholders, upsert, `CollectRows`, joins | `labs/postgres/demos.go` |
| `tx` | Serializable transactions plus a retry loop | `transfer`, `withRetry` |
| `anomaly` | Lost update, and three fixes | `anomaly` |
| `explain` | `EXPLAIN (ANALYZE, BUFFERS)` before and after an index | `explain` |
| `pool` | Connection pool limits | `poolDemo` |

## 1. setup: bulk loading with COPY

```go
pool.CopyFrom(ctx, pgx.Identifier{"orders"},
	[]string{"id", "customer_id", "status", "created_at"},
	pgx.CopyFromRows(rows))
```

`COPY` streams rows in Postgres' bulk format. It loads ~70k rows in well under a second, roughly 10–100x faster than individual `INSERT`s. After loading explicit IDs, the code calls `setval` so `BIGSERIAL` sequences don't collide, then `ANALYZE` so the planner has fresh statistics.

## 2. tx: serializable transfers with retries

Fifty goroutines transfer money in both directions concurrently at `SERIALIZABLE`. Expected output:

```text
alice=100000 bob=100000 total=200000 (must be 200000) — 257 serialization retries were needed
```

The retry count shows **SSI aborting conflicting transactions**. The invariant (total money) holds only because the code retries on SQLSTATE `40001`:

```go
func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01")
}
```

## 3. anomaly: watch a lost update happen

Ten goroutines each read a balance, "think" for 20 ms, then write `balance + 100`:

```text
1. naive read-modify-write (READ COMMITTED)    balance=100100 want=101000  ✗ LOST 9 updates
2. fix: atomic UPDATE ... SET x = x + 100      balance=101000 want=101000  ✓ correct
3. fix: SELECT ... FOR UPDATE (pessimistic)    balance=101000 want=101000  ✓ correct
4. fix: SERIALIZABLE + retry (optimistic)      balance=101000 want=101000  ✓ correct
```

Nine out of ten deposits vanished in the naive version, **without any error**. That's why isolation matters. Notice how each fix trades off differently: atomic SQL is the simplest and fastest, `FOR UPDATE` serialises access by blocking, and `SERIALIZABLE` lets transactions run optimistically and retries the losers.

## 4. explain: the power of the right index

On a 300k-row table:

```text
WITHOUT an index:  Parallel Seq Scan …  Rows Removed by Filter: 149997 …  Execution Time: 17.6 ms
WITH (user_id, created_at):  Bitmap Index Scan on idx_events_user_created …  Execution Time: 0.04 ms
```

About **400x faster**, and the buffers touched dropped from 2,048 pages to 9. Equality column first, range column second (the leftmost-prefix rule in action).

## 5. pool: concurrency is capped by the pool

```text
running 30 queries that each take 100ms through a pool of MaxConns=5 …
took 648ms (≈ 30 jobs / 5 conns × 100ms = 600ms)
acquires=30  had-to-wait=30  total-wait=8.155s  open-conns=5
```

Goroutines are cheap, but database connections aren't. The pool queues work so the database sees at most 5 concurrent queries.

## Exercises

> **Exercise 1 (write skew):** Add a `doctors(name, on_call bool)` table with two doctors on call. Write two concurrent transactions that each check "is the *other* doctor on call?" and set themselves off call. Show that `REPEATABLE READ` allows both to commit (write skew) and `SERIALIZABLE` aborts one.

> **Exercise 2 (keyset pagination):** Implement `ListOrders(ctx, afterID int64, limit int)` with `WHERE id > $1 ORDER BY id LIMIT $2`. Benchmark page 900 vs `OFFSET 18000` with `EXPLAIN ANALYZE`.

> **Exercise 3 (covering index):** Make the explain query an **Index Only Scan** using `CREATE INDEX ... INCLUDE (...)`, and run `VACUUM` so the visibility map is set. Explain why vacuum matters for index-only scans.

> **Exercise 4 (JSONB):** Add `attrs JSONB` to products, store `{"colour": "red", "size": "M"}`, create a GIN index and query with `@>`. Compare with how you'd model this in MongoDB.

> **Exercise 5 (LISTEN/NOTIFY):** Use pgx's `WaitForNotification` to build a tiny real-time feed: a trigger on `orders` calls `pg_notify`. Compare with MongoDB change streams in the next lab.

> **Exercise 6 (sqlc):** Rewrite the CRUD queries with **sqlc** to get type-safe generated Go code.

```quiz
Q: In the anomaly demo, why did 9 of 10 deposits disappear under READ COMMITTED?
- Postgres dropped the writes due to a bug
+ Each transaction read the same old balance and overwrote it with old+100; later commits overwrote earlier ones
- The connection pool was too small
- READ COMMITTED doesn't persist data
? A classic lost update: read-modify-write without locking, atomic updates or serializable isolation.

Q: Why does the tx demo report hundreds of retries but still end with the correct total?
+ SSI aborts transactions that would break serializability, and the code retries them until they succeed
- Retries are a sign of data corruption
- The pool retried failed connections
- Postgres merged conflicting writes automatically
? Aborts are normal under SERIALIZABLE. Correctness comes from retrying.

Q: Why is COPY much faster than many INSERT statements for bulk loads?
- COPY skips constraints
+ It streams rows in one bulk protocol operation, avoiding per-statement round trips, parsing and planning
- COPY writes no WAL
- COPY disables indexes permanently
? Per-row statements pay network and parse overhead each time. COPY amortises it.
```
