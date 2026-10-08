## Write-ahead logging & crash recovery

What happens if the power fails halfway through a transaction? This lesson answers that, and the same mechanism, the **log**, turns out to underpin replication, CDC, point-in-time recovery and even Kafka.

## The problem

Data pages are cached in RAM and written to disk "later". If we crash:

- **Committed** changes might still be only in RAM, so **durability is lost**.
- **Uncommitted** changes might already be on disk, so **atomicity is lost**.
- A page write might be half-done (a **torn page**).

Writing every changed page to disk on every commit would be correct but terribly slow: random I/O on many pages.

## The solution: write-ahead logging (WAL)

> **Key idea:** Before changing any data page on disk, first append a description of the change to a sequential **log**, and flush the log to disk on commit. Data pages can be written lazily.

![The WAL is fsynced on commit; data pages are written later. After a crash, redo replays the log.](diagrams/wal-recovery-log.svg)

- Every record has a **Log Sequence Number (LSN)**, a monotonically increasing position in the log.
- Commit cost = one **sequential** fsync of the log tail. Many concurrent commits can share one fsync (**group commit**).
- `fsync` is the moment of truth. Disks and OSes buffer writes, and only fsync guarantees the bytes are on stable storage.

Names across systems:

| System | Log name |
|---|---|
| PostgreSQL | WAL (`pg_wal/`) |
| MySQL InnoDB | Redo log (plus binlog for replication) |
| MongoDB WiredTiger | Journal (plus the **oplog** for replication) |
| SQL Server | Transaction log |
| Delta Lake | `_delta_log/` (a log of *file-level* actions, covered later) |

## Checkpoints

Without checkpoints, recovery would replay the log from the beginning of time. A **checkpoint** flushes dirty pages and records "everything before LSN X is on disk". Recovery then starts from the last checkpoint.

Trade-off: frequent checkpoints mean faster recovery but more I/O spikes. Postgres tunes this with `checkpoint_timeout` and `max_wal_size`.

## Crash recovery (ARIES, simplified)

1. **Analysis**: scan the log from the last checkpoint and work out which transactions were active and which pages were dirty.
2. **Redo**: replay *all* logged changes (even from losers) so the pages reflect the log. This is called "repeating history".
3. **Undo**: roll back changes from transactions that never committed.

Postgres is simpler on the undo side: thanks to MVCC, uncommitted row versions are simply invisible, so it only needs **redo**. Old versions are cleaned up later by VACUUM.

Torn pages: Postgres writes a **full page image** into the WAL the first time a page is modified after a checkpoint (`full_page_writes`). InnoDB uses a **doublewrite buffer**.

## The log is the database

Once you have a reliable, ordered log of changes, you can ship it elsewhere:

```text
                    ┌──▶ Streaming replica (physical replication: replay WAL)
Primary ── WAL ─────┼──▶ Archive in S3 → Point-in-time recovery (PITR)
                    └──▶ Logical decoding → Debezium → Kafka → Data lake (CDC)
```

- **Physical replication** ships WAL bytes, giving an exact copy.
- **Logical replication / CDC** decodes the WAL into row changes ("insert into orders …") that any consumer can read.
- **PITR** = base backup + WAL archive → restore to *any second*.
- MongoDB's **oplog** is a capped collection of logical operations that secondaries tail; **change streams** expose it to applications.

> **SA angle:** RPO/RTO conversations are log conversations. "RPO of 5 minutes" means you need WAL/oplog shipped at least every 5 minutes. "RPO ≈ 0" means synchronous replication to another zone or region, at a latency cost. Be ready to draw this.

## Hands-on

```sql
-- current WAL position
select pg_current_wal_lsn();
insert into customers (email, name, country) values ('wal@test.io', 'WAL Test', 'UK');
select pg_current_wal_lsn();     -- it moved
select pg_walfile_name(pg_current_wal_lsn());
```

In the **KV-store lab** you implement this idea directly: an append-only log with CRC checksums, recovered by replaying it on startup.

> **Interview:** The instance crashed mid-checkpoint. Some dirty pages never reached disk. Which committed writes survive, and why?

> **Model answer:** Every committed transaction has its WAL records fsynced, including the COMMIT record. Recovery finds the last checkpoint, then redoes logged changes to the end of the WAL, so committed work is reconstructed even if the data pages were still in RAM. Uncommitted work is undone — or, in Postgres MVCC, simply invisible. The only sync I/O required at commit is the log fsync, not the data pages.

```quiz
Q: What is the only synchronous I/O strictly required at commit in a WAL-based database?
- Writing all modified data pages
+ Flushing (fsync) the log records up to the commit record
- Writing a checkpoint
? Data pages can be written later; the log has enough information to redo them.

Q: In ARIES-style recovery, what does the "redo" phase do?
+ Replays logged changes so pages reflect everything in the log, including uncommitted work
- Undoes committed transactions
- Deletes the log
? Redo repeats history; undo then removes the losers.

Q: Which capability is built directly on archiving the WAL?
- Secondary indexes
+ Point-in-time recovery
- Query optimisation
? Restore a base backup, then replay archived WAL up to the target timestamp.

Q: What does MongoDB use to replicate operations from primary to secondaries?
- The journal
+ The oplog, a capped collection of idempotent operations
- WAL shipping
? Secondaries tail the primary's oplog; change streams are built on the oplog.
```
