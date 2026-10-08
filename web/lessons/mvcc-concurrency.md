## Concurrency control & MVCC

Isolation levels (from the Transactions lesson) are the *contract*. Concurrency control is the *implementation*. There are two families: **locking** (pessimistic) and **multi-version concurrency control** (MVCC), and most modern databases mix both.

## Locking and two-phase locking (2PL)

- **Shared (S) locks** for reads and **exclusive (X) locks** for writes. Many readers or one writer.
- **Two-phase locking:** a transaction acquires locks (growing phase) and only releases them at the end (shrinking phase). **Strict 2PL**, which holds every lock until commit, gives serializability.
- Downsides: **readers block writers and writers block readers**, and you get **deadlocks**:

![Two transactions each hold one lock and wait for the other: a deadlock the database must abort.](diagrams/mvcc-concurrency-2pl-deadlock.svg)

**Granularity** matters: lock the database, a collection or table, a page, or a row/document. Finer locks allow more concurrency at a higher bookkeeping cost. MongoDB's WiredTiger uses **document-level** concurrency.

## MVCC: keep multiple versions

> **Key idea:** Instead of overwriting a row, create a **new version**. Each transaction reads from a consistent **snapshot**, choosing the newest version that was committed before the snapshot was taken. **Readers never block writers, and writers never block readers.**

![Each update makes a new version; a snapshot picks the newest version committed before it started.](diagrams/mvcc-concurrency-version-storage.svg)

Writers still conflict with **writers**. Two transactions updating the same row: the second waits for the first, and then either proceeds (read committed) or aborts with a serialization error (repeatable read / serializable). This is the **first-committer-wins** rule.

## How the big engines do it

| | PostgreSQL | MySQL InnoDB / Oracle | MongoDB WiredTiger |
|---|---|---|---|
| Where old versions live | In the table heap itself (new tuple per update) | Undo log (row updated in place, old version reconstructable) | In-memory update chains; history bounded by time |
| Cleanup | **VACUUM** removes dead tuples | Purge thread trims undo | History kept for a window (`minSnapshotHistoryWindowInSeconds`, default 300s), then discarded |
| Classic pain point | **Bloat** and transaction ID wraparound if VACUUM falls behind | Long transactions → undo growth | Long-running transactions → cache pressure; transactions limited to 60s by default (`transactionLifetimeLimitSeconds`) |

### Postgres details interviewers like

- Each tuple has `xmin` (creating txn) and `xmax` (deleting txn). Try `select xmin, xmax, * from customers;`.
- An `update` = insert a new tuple + mark the old one dead. **HOT updates** (heap-only tuples) avoid index updates if no indexed column changed and the page has room. That's why `fillfactor` matters.
- **Autovacuum** must keep up. A long-open transaction (even an idle one in a forgotten `psql` session) prevents cleanup, so the table bloats.
- 32-bit transaction IDs **wrap around**. VACUUM *freezes* old tuples to prevent it. Ignoring this has caused real production outages.

## Snapshot isolation and write skew, concretely

![A snapshot sees tuples created before it started and ignores in-progress or future writers.](diagrams/mvcc-concurrency-snapshot-visibility.svg)

![Under snapshot isolation both doctors commit; SSI sees the rw-cycle and aborts one.](diagrams/mvcc-concurrency-ssi-write-skew.svg)

They wrote **different rows**, so there's no write-write conflict to detect. Fixes:

- `serializable` isolation. Postgres's **SSI** tracks read/write dependencies and aborts one transaction.
- Explicit locking: `select … for update` on the rows you based the decision on.
- Materialise the conflict: write to a shared row (e.g. `on_call_count`) so both transactions conflict.

## Optimistic vs pessimistic, in one table

| | Pessimistic (locks) | Optimistic (MVCC + validation) |
|---|---|---|
| Assumes | Conflicts are common | Conflicts are rare |
| Cost when no conflict | Lock overhead, blocking | Near zero |
| Cost on conflict | Waiting, deadlocks | Abort and retry |
| Good for | Hot rows (counters, inventory of a hot item) | Most web workloads |

> **SA angle:** A customer complains "random transactions fail under load" after moving to serializable or to MongoDB transactions. The answer is retries, keeping transactions short, and avoiding hot spots. For MongoDB, use the driver's `WithTransaction` callback API, which retries `TransientTransactionError` and `UnknownTransactionCommitResult` for you.

> **Exercise:** In the **MVCC lab** you implement snapshot reads, first-committer-wins conflict detection and garbage collection of old versions in about 150 lines of Go.

> **Interview:** Two doctors are on call. Each reads "two on call", each goes off call, both commit. How, and what does the application do?

> **Model answer:** Write skew under snapshot isolation: they wrote different rows, so there is no write-write conflict. Postgres `SERIALIZABLE` (SSI) aborts one with SQLSTATE `40001`; the retry sees the new count. Alternatively lock the roster with `SELECT … FOR UPDATE`, or keep an `on_call_count` row so the writes collide. Treat `40001` as retryable, not a user-facing error.

```quiz
Q: What is the main benefit of MVCC over pure locking?
+ Readers don't block writers and writers don't block readers
- It uses less disk space
- It prevents all anomalies at every isolation level
? Readers use a snapshot of committed versions, so they never wait on writers.

Q: Why can an idle, open transaction in Postgres cause table bloat?
+ VACUUM can't remove dead tuples that might still be visible to that transaction's snapshot
- It holds an exclusive lock on the table
- It fills the WAL
? The oldest active snapshot determines what's safe to clean up.

Q: Two transactions under snapshot isolation update different rows and break an invariant. What is this called?
- Dirty read
+ Write skew
- Deadlock
? There is no write-write conflict, so SI doesn't detect it; serializable does.

Q: In Postgres, what does a HOT update avoid?
- Writing to the WAL
+ Updating indexes, when no indexed column changed and the new tuple fits on the same page
- Creating a new tuple version
? Heap-Only Tuples chain the new version on the same page so indexes don't need new entries.
```
