## Why read other people's worst days

These are not campfire stories. Each one is a public postmortem from the company that ran the database. The facts below are only what they published. If a number or quote is not in that post, it is not here.

The SA job in a discovery call is to turn that into a **guardrail**: a control you can put on a slide and a test you can put in a runbook. Cross-link the mechanisms as you go — [backups and disaster recovery](#/lesson/problem-backup-dr), [replication](#/lesson/replication), [distributed failure modes](#/lesson/distributed-trouble).

> **Customer:** "We have replicas, so we are fine." After these four incidents you should be able to answer that without theatre.

## GitLab.com — 31 January 2017

**Published source:** *Postmortem of database outage of January 31* (GitLab).

GitLab.com ran **one Postgres primary** and **one hot-standby secondary**, used for failover, not for reads. That afternoon an engineer took a fresh **LVM snapshot** of production to refresh staging (the automatic staging snapshot ran once a day). Database load then rose — they suspected spam, and later found a background job hard-deleting a flagged employee account. Around **23:00 UTC** the secondary **fell so far behind that the WAL it needed had already been removed** on the primary. GitLab.com was **not archiving WAL**, so the only repair was to wipe the secondary and `pg_basebackup` from the primary.

`pg_basebackup` appeared to hang (it was waiting for the primary). After fiddling with `max_wal_senders` and `max_connections`, an engineer wiped a Postgres data directory **believing they were on the secondary**. They were on the **primary**. They stopped the `rm` within a couple of seconds; about **300 GB** was already gone.

Then every recovery path they thought they had failed:

| Path they believed in | What the postmortem says |
|---|---|
| Daily `pg_dump` to S3 | The S3 bucket was **empty**. The job ran `pg_dump` **9.2** against Postgres **9.6** (Omnibus defaulted to 9.2 on app servers that had no `PG_VERSION`). The dump aborted. |
| Email alerts on cron failure | GitLab.com used **DMARC**. The cron mail was **rejected**. Nobody saw the failures. |
| Azure disk snapshots | In use for NFS (Git data). **Not enabled** on the database servers. |
| The secondary | Already wiped / unsynchronised. No WAL archive to rebuild from. |
| LVM snapshots | One ~24 hours old, plus the **manual** snapshot from about **17:20 UTC**. That was the only usable copy. |

They restored from the 17:20 LVM snapshot. Database data written between **17:20 and 00:00 UTC** was gone — GitLab's estimate: roughly **5,000 projects, 5,000 comments, 700 new accounts**. Git **repositories and wikis were not in that database** and were not lost; they were just unavailable during the outage. Self-managed and Enterprise customers were unaffected. Copying the snapshot off slow staging disks took on the order of **18 hours**. Sequences were incremented by 100,000 so IDs from the lost window could not be reused.

![GitLab had four stories about backups. Three were empty, silent or never configured. The fourth was a staging LVM snapshot that happened to be six hours old.](diagrams/case-studies-outages-backup-untested.svg)

**Why backups and failover did not save them.** Replication is not a backup: the secondary needed WAL the primary had already deleted, and there was no archive. The logical dump had been failing for an unknown time with no page. Nobody had restored a backup on a timer. Destructive commands were possible on the primary with no second pair of eyes.

**Guardrail to recommend.**

- A backup job is not done until a **restore** has been run on a schedule, from the same artefact you would use in anger, and the restore duration is written down (RTO).
- **WAL archiving** (or a vendor PITR) so a lagging replica is not a dead end. See [WAL and recovery](#/lesson/wal-recovery).
- Version-pin `pg_dump` to the server major; alert on dump size and on "bucket empty", not on "cron started".
- Two-person rule / restricted production roles for `rm` of data directories. Recreate a replica from a **runbook**, not from memory at 23:30.

> **Incident:** "We take backups" is not a control. "We restored last Tuesday; it took 47 minutes; here is the ticket" is.

## GitHub.com — 21 October 2018

**Published source:** *October 21 post-incident analysis* (GitHub Blog, 30 October 2018).

Git metadata (issues, pull requests, auth, background jobs) lived in **several MySQL clusters**, functionally sharded, each with many read replicas. Git object storage was separate and **stayed up**. Orchestrator managed topology and failover, using **Raft** for its own consensus.

At **22:52 UTC**, replacing failing 100G optical kit **partitioned** the US East hub from the primary East data centre for **43 seconds**. In that window:

1. East primaries **accepted writes** that did not reach the West.
2. Orchestrator in East lost Raft leadership. Nodes on the **US West Coast plus US East public cloud** formed a quorum and **promoted West replicas to primary**.
3. Connectivity returned. The application followed Orchestrator and **wrote to the new West primaries**.

Both sites now had writes the other did not. GitHub could not fail back to East without throwing away West's subsequent writes. They **chose integrity over availability**: stay failed-forward, pause webhooks and Pages builds, restore from backups, rebuild topology, then reconcile.

The cost they published: **24 hours 11 minutes** of degraded service. Cross-country write latency made the site painful while primaries lived in the West. Restoring multi-terabyte backups from off-site object storage took hours — they test restores daily, but they had never before rebuilt **entire clusters** from backup (they usually used delayed replicas). Replica catch-up was slower than a linear estimate once morning traffic arrived. About **five million** webhook events and **80,000** Pages builds queued; some ~200,000 webhooks exceeded an internal TTL and were dropped until they raised it. A few seconds of East-only writes needed **manual reconciliation** (one busy cluster had **954** such writes). They said **no user data was lost**; some of those writes had to be applied or confirmed with users.

![A 43-second partition was enough for East to take unreplicated writes and for Orchestrator to elect West primaries. Failover succeeded; the application could not live with the topology.](diagrams/case-studies-outages-split-brain.svg)

**Why failover did not save them.** Orchestrator did what it was configured to do. The **application was not written for a sudden East→West primary**. Cross-region promotion is a different product to in-region failover. A short partition plus automated failover produced **split history**, which is worse than a brief write outage. See [distributed trouble](#/lesson/distributed-trouble) for why a majority on the "wrong" side of a partition is still a majority.

**Guardrail to recommend.**

- **Do not auto-promote a primary across a latency domain** the application has not been tested against. In-region leader election is a different change to a transcontinental one.
- Failover configuration must encode **application assumptions** (where the writers live, what RTT they can pay), not just "is there a replica with a valid copy".
- Practise the thing you will actually do: rebuild a *whole* cluster from backup, not only spin up one delayed replica.
- Inject partitions on purpose before the optical cable does it for you.

> **SA angle:** Automated failover is not "more available". It is a policy for *which* inconsistency you will accept. GitHub's published follow-up was to stop Orchestrator promoting across regional boundaries.

## Cloudflare — 2 November 2020

**Published source:** *A Byzantine failure in the real world* (Cloudflare Blog). There is no well-known 14 November 2020 database postmortem from them; this is the control-plane incident from that month.

Cloudflare's **edge kept working**. The **control plane** (API and dashboard) did not. Most microservice databases are writable in **one region at a time**. A **partially failed network switch** produced inconsistent traffic — a Byzantine fault, not a clean "link down". Their **etcd** cluster saw conflicting views, could not keep a stable leader, and went **read-only**.

The database cluster manager uses etcd. When etcd stopped answering "who is primary?", **two clusters automatically promoted a synchronous replica**. The promotions themselves completed **without data loss**. The damage was the aftermath:

- A defect meant **every replica had to be rebuilt** after a promotion.
- The **authentication** database is read-heavy and **depends on a replica** to spare the primary. With no replica, the new primary took the full read load and **fell over**.
- Impact they published: **6 hours 33 minutes**, API success rate periodically down to about **75%**, dashboard up to **80×** slower.
- Replicas already existed in the **secondary data centre**, but steering reads there was **not automated**. Doing it by hand improved the API. Service recovered when the first rebuilt replica came online (**21:20 UTC**).

**Why failover did not save them.** The promotion was correct and lossless — and still caused the outage, because **the topology after failover could not carry production read load**. Coordination (etcd) and data (SQL) failed in different ways; a flaky switch looked like "primary is gone".

**Guardrail to recommend.**

- A failover runbook must include **"do we still have enough read capacity?"** If the answer is no, promoting can be worse than staying put.
- Automate **cross-site read steering**; do not discover in the incident that it is a manual change.
- Do not treat a **sick** consensus store as "the primary has died". Tune how fast automation is allowed to act on a Byzantine coordinator.
- Keep the data path (the edge) independent of the control plane — they did, and that is why customers' sites stayed up.

> **Warning:** "We fail over automatically" without "we can serve reads after failover" is how you turn a 43-second network blip — or a confused etcd — into a six-hour dashboard outage.

## Sentry — 20 July 2015

**Published source:** *Transaction ID Wraparound in Postgres* (Sentry Blog, 23 July 2015).

Hosted Sentry was down for **most of a US working day**. Postgres **stopped accepting writes** because **transaction-ID wraparound protection** tripped. Sentry is write-heavy; they had been tuning autovacuum, and it still fell behind on a very large mapping table (event ID → rollup, one row per event).

They let in-flight autovacuums finish rather than bounce into single-user mode and lose that work. After hours, one relation was still stuck and logs were not verbose enough to say why. They **truncated** that mapping table — useful for lookup-by-event-id and duplicate guards, not for the core event payload — and were writable again about **five minutes** later. They flushed an event backlog that had piled up in Redis rather than make the recovery worse.

**Why ordinary HA did not save them.** A replica that is a physical copy of a wraparound-stopped primary is not a way out. This is a **cluster-wide safety interlock**, not a crashed node. Failover copies the problem.

**Guardrail to recommend.**

- Treat **`xid_age` / wraparound** as a paging metric, not a curiosity. See [Postgres deep dive](#/lesson/postgres-deep-dive) and [MVCC](#/lesson/mvcc-concurrency).
- Size **autovacuum** (workers, cost limits, maintenance memory) for the write rate you actually have.
- Turn on **autovacuum logging** before you need it in a postmortem.
- Know which tables you would **truncate or drop** under wraparound, and which you must not.

Notion's later sharding post names the *same* wraparound risk as the reason they would not keep scaling a monolith. The mechanism is not exotic.

## What you put on the architecture slide

| Incident | What actually failed | The sentence to write in the SoW |
|---|---|---|
| GitLab 2017 | Human `rm` on the primary; untested, silently failing dumps; no WAL archive | Restore is rehearsed; WAL is archived; destructive ops need two people |
| GitHub 2018 | Orchestrator promoted across a region the app could not bear; 43 s of split writes | Failover stays inside a latency domain; partitions are injected in tests |
| Cloudflare 2020 | Automatic promotion left the auth primary without read replicas | Failover is gated on remaining capacity, not only on "primary unhealthy" |
| Sentry 2015 | XID wraparound; autovacuum lost | XID age is paged; vacuum is capacity-planned |

If you only remember one line for the interview: **replicas are for availability of a live primary, backups-plus-restore-tests are for the day you delete the live primary, and failover is a product change you have to design.**

```quiz
Q: Why could GitLab not restore from its daily pg_dump?
- S3 was down for the region
+ The dump used Postgres 9.2 client binaries against a 9.6 server, aborted, and the failure mail was rejected
- The dump was encrypted with a lost KMS key
- pg_dump cannot back up a primary that has a replica
? Omnibus defaulted to 9.2 on app servers with no PG_VERSION. DMARC dropped the cron errors. The S3 bucket was empty when they needed it.

Q: GitHub's Orchestrator failover did what, in the published analysis?
- Deleted the East Coast writes so the cluster could fail back immediately
+ Formed a Raft quorum outside the East primary site and promoted West replicas, after East had taken unreplicated writes
- Switched Git object storage to the West Coast
- Disabled all backups to save I/O
? The 43-second partition was enough for split history. They failed forward and rebuilt, because the application could not pay transcontinental primary latency.

Q: What is the SA lesson from Cloudflare's 2 November 2020 control-plane incident?
- Always run etcd and SQL in the same process
+ Automatic promotion can succeed without data loss and still take you down if the new primary has no read replicas
- The edge should have failed too, so the blast radius was smaller
- Byzantine faults cannot happen on a single switch
? A partial switch confused etcd; failover rebuilt replicas; the auth primary then absorbed all reads. Secondary-site replicas existed but steering was manual.

Q: Why did failing over Postgres not help Sentry in July 2015?
- They had no replica
- Orchestrator refused the promotion
+ Wraparound protection stops writes on the database; a physical replica is in the same state
- The primary disk was full and replicas were read-only by policy
? XID wraparound is a safety interlock on the data, not a node crash. You fix vacuum (or truncate a disposable relation), you do not hope a copy will accept writes.

Q: Which statement is true of GitLab's data loss?
- They lost Git repositories and the Postgres metadata
+ They lost about six hours of database metadata (projects, comments, accounts); repositories lived on NFS and were not in the deleted data directory
- They lost nothing, because the secondary was current
- They lost 30 days of WAL
? The LVM snapshot they restored was from 17:20 UTC. Repos were unavailable during the outage but not wiped with the primary's PGDATA.

Q: Which customer control would have changed the outcome in more than one of these incidents?
- Switching from SQL to a document store
- Adding a third cloud region for every database
+ Regular restore drills, plus failover policies that encode application and capacity constraints
- Turning off automated failover everywhere
? GitLab needed a tested restore and WAL archive. GitHub and Cloudflare needed failover that matched what the app and the remaining replicas could actually do. Automation is fine; untested automation is not.
```
