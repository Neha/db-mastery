## Backup is a restore you have already practised

A file named `backup.sql` in object storage is a hope. A backup is **unproven until you have restored it** on a clock, with the people who would do it at 3am, into an environment that is not production. Everything in this lesson is in service of two numbers the customer already wrote in a slide:

| Number | Means | You buy it with |
|---|---|---|
| **RPO** (recovery point objective) | How much recent data you may lose | How often the **log** leaves the machine |
| **RTO** (recovery time objective) | How long you may stay down | How fast you can **restore or fail over** and re-point clients |

![RPO is the gap from the last good copy to the failure; RTO is the gap from the failure to service restored.](diagrams/problem-backup-dr-rpo-rto.svg)

> **Key idea:** RPO is a [WAL / oplog shipping](#/lesson/wal-recovery) question. RTO is a restore, [failover](#/lesson/replication) and DNS/connection-string question. Mixing them up is how you sell daily snapshots to a customer who cannot lose more than 30 seconds.

A replica is not a backup. It is a hot copy of the same mistakes: `DROP TABLE`, a bad migration, ransomware that encrypts every node the app can reach. You want **something the primary cannot delete**.

## Snapshots vs logical dumps vs continuous WAL

| Method | What you get | PITR? | Typical use |
|---|---|---|---|
| **Logical dump** (`pg_dump`, `mongodump`) | Portable SQL/BSON. Slow on large data. `pg_dump --single-transaction` is one consistent snapshot of one database | No | Migrations, small estates, logical copies |
| **Physical base backup** (`pg_basebackup`, filesystem snapshot of data + WAL) | Cluster-shaped, version-specific, fast to take | Only if you also archive WAL | The default for Postgres |
| **Volume snapshot** (EBS, CSI, Ceph) | Fast, cheap, easy to automate | Crash-consistent at best unless you freeze/checkpoint and keep WAL | Complements a base backup; do not treat a lone dirty snapshot as gospel |
| **Continuous WAL / oplog archive** | Every committed change after the last base backup | **Yes** | The RPO story |

Point-in-time recovery is "restore the base backup, then replay the log up to timestamp T". Without the log you can only land on snapshot times.

![A Sunday base backup plus archived WAL lets you stop at the second before Tuesday's accidental DELETE.](diagrams/problem-backup-dr-pitr.svg)

### Postgres: base backup + WAL archive

```text
Primary ──▶ pg_wal (local, finite) ──▶ archive_command / WAL shipper ──▶ object storage
       └──▶ pg_basebackup (nightly / continuous) ──▶ object storage
```

Restore: fetch a base backup older than T, configure `restore_command` to pull WAL, set `recovery_target_time` (or `recovery_target_lsn` / `recovery_target_xid`), start the instance, promote when replay finishes. Tools you will actually operate — **pgBackRest**, **WAL-G**, **Barman**, CloudNativePG's object-store backups — wrap this. The mechanism is still the [write-ahead log](#/lesson/wal-recovery).

`wal_keep_size` / replication slots keep WAL on the primary for standbys. They are not an archive. A slot with a dead consumer **fills the disk**. An archive in another account, with immutability, is the backup.

### MongoDB: oplog and Atlas PITR

Self-managed replica sets have an **oplog** (a capped collection). Secondaries tail it; restore-to-a-timestamp needs a snapshot plus enough oplog (or a separately retained oplog) to cover the gap. Size the oplog for the window you advertise, or you will discover at restore time that the window was three hours, not three days.

**Atlas** continuous backup / PITR is this design managed: snapshots plus oplog (or equivalent) inside a **configured window**. Quote the window from the current Atlas docs for that tier; do not memorise a number. `mongodump` alone is a logical copy, not PITR.

## Restore drills

Schedule them. Restore into an isolated network, measure wall-clock RTO, and check that the data is the data you expected (row counts, a known canary row, application smoke tests). Drill the paths you fear:

- whole-cluster loss
- one database / one collection
- "the backup bucket is empty" (credentials, wrong prefix, lifecycle rule)
- restore that is **newer than the incident** (you replayed the DELETE)

Write the runbook from the last drill, not from memory.

> **Incident:** GitLab's 2017 public postmortem described an engineer removing a production Postgres data directory. Several backup methods had been failing quietly. Recovery depended on a delayed copy and a slow restore. The published lesson is the one every SA repeats: a backup you have never restored is not a backup. More postmortems in [Famous outages](#/lesson/case-studies-outages).

## Accidental DELETE

`DELETE FROM orders` without a `WHERE`, a bad `drop()`, a migration that drops the wrong column: the database did what you asked. Options, best first if you planned:

1. **Delayed replica** (`recovery_min_apply_delay` in Postgres): a standby that is hours behind. Pause apply, extract the lost rows, or fail the app over to it and accept the RPO of the delay.
2. **PITR** to the second before the statement. This is a new instance (or a fork in managed cloud). You then either swap it in (lose later good writes) or copy the lost rows forward.
3. **Soft delete** in the model (`deleted_at`) so "delete" is an update you can undo without WAL archaeology.
4. Logical dumps: only if they are newer than the mistake and you accept replacing everything.

Do not PITR the primary in place while it is still taking writes you want to keep. Fork, extract, merge.

## Ransomware and the copy the attacker cannot reach

If the attacker has the database role, they have the replicas the database can write to. Defence is **offline or immutable** copies: object-storage retention lock, a second account, air-gapped media. The old **3-2-1** rule still holds: three copies, two kinds of media, one off-site. Encrypt backups with keys the database host cannot use to delete the ciphertext.

A restore drill against ransomware includes "can we rebuild identity, DNS and the app from a cold start?" — RTO is rarely just `pg_basebackup` time.

> **Remember:** Retention is also a privacy setting. The longer you keep WAL for perfect PITR, the longer a [deletion request](#/lesson/problem-passwords-secrets) remains incomplete. Pick the window on purpose.

> **SA angle:** Ask for RPO/RTO in **numbers**, then map each number to a mechanism (sync replica, async replica, WAL every 60 seconds, nightly dump). Offer the cost of the tighter number. "We have backups" is not an architecture.

```quiz
Q: A customer wants RPO of 5 minutes. What must you actually do?
+ Ship the WAL or oplog off the machine at least every 5 minutes (or continuously)
- Take a pg_dump every night
- Keep three replicas in one AZ
- Enable TLS
? Replicas in the same blast radius and nightly dumps cannot meet a 5-minute RPO.

Q: Why is a streaming replica not a substitute for a backup?
- Replicas cannot serve reads
- Replicas never lag
+ It will replay DROP TABLE and often ransomware; it is not an isolated point-in-time copy
- Backups cannot use the WAL
? You need a copy the primary cannot destroy. Delayed replicas and immutable archives are that copy.

Q: What do you need for Postgres point-in-time recovery?
- Only a logical dump
- Only an EBS snapshot of the data volume, with WAL discarded
- The replica's pg_stat_statements
+ A base backup plus a continuous WAL archive, then replay to a target time
? PITR is "base + log". A dump or a single dirty volume snapshot cannot stop at an arbitrary second.

Q: You ran DELETE without a WHERE at 14:03 and noticed at 14:20. Writes since 14:03 are good. What is the usual fix?
- PITR the primary in place to 14:02 and lose the good writes
- Drop the WAL archive
+ Fork via PITR (or a delayed replica) to 14:02, copy the lost rows forward onto the live primary
- Restore last month's mongodump over the cluster
? Fork and merge preserves later work. In-place PITR is how you double the incident.

Q: When is a backup real?
- When archive_command returns zero
- When the object-storage prefix is not empty
- When the vendor dashboard shows a green tick
+ When you have restored it in a drill and measured RTO
? Green ticks fail silently. Restores do not.
```
