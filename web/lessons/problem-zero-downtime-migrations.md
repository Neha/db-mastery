## Why "just ALTER TABLE" takes production down

Schema change is a compatibility problem wearing a SQL costume. Old application code and new application code must both run against the database during a rolling deploy. A lock that surprises you queues every session behind it. A rewrite of a 200 GB table is an outage even if nobody blocked.

The durable pattern is **expand / contract**: add the new shape, move the data, switch readers, then remove the old shape. Each step is its own deploy and, until the last one, is reversible. The same idea applies to [encoding evolution](#/lesson/encoding-evolution) for event schemas.

![Expand adds a nullable column, dual-write and backfill move the data, contract drops the old column after a soak.](diagrams/problem-zero-downtime-migrations-expand-contract.svg)

> **Key idea:** Never change the meaning of a column in place while old code still reads it. Add, dual-write, backfill, flip a flag, then drop.

## Expand / contract in practice

Take a rename: `name` → `full_name`.

1. **Add** `full_name` nullable. Catalog-only in Postgres if there is no volatile default. Old code ignores it.
2. **Dual-write** both columns in a new app version. Reads still use `name`.
3. **Backfill** existing rows in batches (below). Dual-writes keep new rows aligned.
4. **Read the new column**, behind a feature flag. Compare old vs new on a sample.
5. **Write only the new column** once reads look clean.
6. **Drop** `name` after a soak. This is the first step you cannot casually roll back.

Feature flags belong on **reads first**, then writes. A flag lets you revert the behaviour without reverting the schema.

> **Pitfall:** Dual-write without a backfill leaves old rows NULL in the new column. Dual-write without handling partial failure leaves the two columns disagreeing. Make the write idempotent and repair from the backfill job.

## Safe DDL in Postgres

Postgres takes locks you cannot see from the migration filename. The dangerous one is **ACCESS EXCLUSIVE**: it waits for every current reader and writer, and while it waits it sits in the lock queue and **blocks new ACCESS SHARE** (ordinary `SELECT`) behind it. A report that holds ACCESS SHARE for minutes plus an `ALTER TABLE` is how you exhaust the pool and return 5xx.

![A long SELECT holds ACCESS SHARE; ALTER waits for ACCESS EXCLUSIVE and then queues the whole application behind it.](diagrams/problem-zero-downtime-migrations-lock-queue.svg)

| Change | Rewrites the table? | Lock notes |
|---|---|---|
| `ADD COLUMN` nullable, no default | No | Brief ACCESS EXCLUSIVE on the catalog |
| `ADD COLUMN … DEFAULT` constant (Postgres 11+) | No | Default lives in the catalog |
| `ADD COLUMN … DEFAULT` volatile (`now()`, `random()`) | **Yes** | Treat as a rewrite |
| `DROP COLUMN` | No | Catalog only; space comes back later |
| `CREATE INDEX` | No | `SHARE` — blocks writes |
| `CREATE INDEX CONCURRENTLY` | No | Allows writes; **cannot run in a transaction**; failed run leaves an `INVALID` index you must drop |
| `SET NOT NULL` on a populated column | Scan | ACCESS EXCLUSIVE historically; in Postgres 12+ it is fast if a valid `CHECK (col IS NOT NULL)` already exists |
| Change type (`int` → `bigint`, most casts) | **Often yes** | Plan expand/contract instead: new column, backfill, swap |

Online `NOT NULL`:

```sql
ALTER TABLE orders ADD CONSTRAINT orders_email_not_null
  CHECK (email IS NOT NULL) NOT VALID;
ALTER TABLE orders VALIDATE CONSTRAINT orders_email_not_null;  -- SHARE UPDATE EXCLUSIVE
ALTER TABLE orders ALTER COLUMN email SET NOT NULL;            -- cheap once the check exists
```

Always set `lock_timeout` (and usually `statement_timeout`) on the session that runs DDL, so you fail and retry instead of queueing the estate. The lock-queue diagram's fix is not optional.

```sql
SET lock_timeout = '2s';
SET statement_timeout = '5s';
```

> **Tip:** `CREATE INDEX CONCURRENTLY` is the default you want in production. Non-concurrent `CREATE INDEX` is for empty tables and for the lab.

## Online backfills

A single `UPDATE … SET full_name = name` on a large table is a long transaction: WAL spike, bloat, replication lag, cancelled autovacuum. Batch it.

```sql
-- keyset batches; run from a job, not one transaction around the whole table
UPDATE users
SET full_name = name
WHERE id IN (
  SELECT id FROM users
  WHERE full_name IS NULL AND name IS NOT NULL
  ORDER BY id
  LIMIT 1000
);
```

Sleep between batches. Watch replica lag and WAL rate. Throttle. The [model](#/lesson/data-modeling) should make the backfill obvious: copy from a source column or recompute from a documented rule, not from application memory.

MySQL's ecosystem uses `pt-online-schema-change` and `gh-ost` (shadow table, triggers or binlog, cut-over). Same expand/contract idea, different plumbing. Do not run untested rewrite DDL on a primary and hope.

## Dual writes, flags, and when CDC is better

Dual writes in application code are fine for one database and two columns. They are a poor way to keep **two systems** in sync: the second write can fail, and you will not notice until cutover. For system-to-system migrations, prefer **log-based CDC** so the source commit is the source of truth. That is the [Debezium / Kafka](#/lesson/kafka-debezium-cdc) lesson applied to migration.

Feature flags decide who reads the new store. Canary a percentage of tenants, compare results, then raise.

## Oracle → Postgres and SQL → MongoDB

Same expand/contract, larger boxes.

1. **Map the model.** Types, sequences, packages, `NUMBER`, `VARCHAR2`; or documents instead of joins. Do this on paper before you move bytes. See [data modelling](#/lesson/data-modeling).
2. **Snapshot** the source (consistent export or an initial CDC snapshot).
3. **Catch up** with CDC (Debezium on redo/LogMiner, DMS, GoldenGate; or a MongoDB sink from the same log). Wait until lag is ~0.
4. **Dual read.** The app reads the target, falls back to the source, and logs disagreements. Checksum row samples.
5. **Cut over writes** to the target. Freeze or reject writes on the source. Drain the log.
6. **Soak** with the source read-only. Only then decommission.

![CDC tails the source log into Kafka and the target; the app dual-reads behind a flag until checksums match and writes cut over.](diagrams/problem-zero-downtime-migrations-cdc-dual-read.svg)

SQL → MongoDB adds a modelling step the CDC tool will not do for you: embed vs reference, what a "transaction" is after the move, and how you will keep a relational reporting copy if you still need one. CDC events themselves have schemas; plan compatibility as in [encoding evolution](#/lesson/encoding-evolution).

> **SA angle:** Customers hear "zero downtime migration" as a product feature. It is a **sequence of reversible deploys** plus a log. Quote RPO during cutover (how much you are willing to pause writes) and the checksum method. Never promise a one-shot `pg_dump | psql` on a live primary.

> **Interview:** Walk expand/contract on a column rename, name `CREATE INDEX CONCURRENTLY` and `lock_timeout`, then sketch CDC + dual read for Oracle → Postgres. If you only say "we'll take a maintenance window", you have failed the prompt.

```quiz
Q: What is the expand step of a column rename?
+ ADD the new column (nullable) so old code keeps working
- DROP the old column, then ADD the new one
- CHANGE the column name in one ALTER
- VACUUM FULL
? Expand is additive. Old binaries must keep running against the new catalog.

Q: Why can CREATE INDEX CONCURRENTLY not run inside a transaction?
- Postgres forbids all DDL in transactions
- It needs ACCESS EXCLUSIVE
+ Concurrent index builds take two passes and cannot be rolled back as a single transactional unit; a failure leaves an INVALID index
- It only works on replicas
? Run it on its own. If it fails, DROP INDEX and retry.

Q: A long reporting SELECT is running. You start ALTER TABLE that needs ACCESS EXCLUSIVE. What happens next?
- The ALTER skips the report
- Postgres kills the report automatically
- Only writers are affected
+ The ALTER waits, and new application queries queue behind it until the report ends
? ACCESS EXCLUSIVE is FIFO with the waiters. Set lock_timeout and retry.

Q: Why backfill in small batches instead of one UPDATE of the whole table?
- SQL cannot update more than 1000 rows
- Batches avoid the need for dual writes
+ A huge transaction spikes WAL, bloats the table, lags replicas and holds resources
- The catalog forbids large UPDATEs
? Online backfill is a job with a throttle, not a one-line migration.

Q: You are moving Oracle to Postgres with a low downtime budget. What keeps the target current after the initial load?
- Nightly pg_dump from Oracle
- Dual writes in the app and no checksum
- A weekend cutover only
+ Log-based CDC into the target, then dual read and a write cutover
? The source log is the backlog. Application dual-write across two products is how the copies diverge.
```
