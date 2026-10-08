## Why replicate?

1. **High availability**: keep serving when a node, AZ or region fails.
2. **Read scaling**: spread reads across copies.
3. **Latency**: put data close to users.
4. **Durability**: a copy on another machine survives disk loss.

The hard part isn't copying data. It's **handling changes**, because every replica must apply the same writes in a sensible order.

![The three replication strategies. Where writes are accepted determines whether conflicts can happen at all.](diagrams/replication-strategies.svg)

## Strategy 1: single-leader (primary/secondary)

All writes go to one leader, which streams its log (WAL, oplog, binlog) to followers. This is used by **PostgreSQL, MySQL, MongoDB replica sets, Aurora, Kafka partitions** and SQL Server Always On.

- ✅ Simple, no write conflicts.
- ❌ The leader is a write bottleneck, and failover is needed when it dies.

### Synchronous vs asynchronous

| | Synchronous | Asynchronous | Semi-sync / quorum |
|---|---|---|---|
| Commit waits for | Followers to confirm | Leader only | Some followers (e.g. a majority) |
| Data loss on leader failure | None | Possible (replication lag) | None if the quorum survives |
| Latency and availability | Worse: one slow follower stalls writes | Best | Balanced |
| Examples | Postgres `synchronous_commit=remote_apply` | Postgres default streaming | MongoDB `w:"majority"`, Raft-based systems |

![Synchronous commit waits for a follower; asynchronous returns after the leader's fsync.](diagrams/replication-sync-async.svg)

### Replication lag problems (and fixes)

| Problem | Example | Fix |
|---|---|---|
| **Read-your-writes** | You update your profile, refresh, and see the old one (read from a lagging follower) | Read your own data from the leader; causal sessions |
| **Monotonic reads** | Refresh twice and data goes back in time (different followers) | Pin a user to one replica |
| **Consistent prefix** | You see an answer before the question | Causally ordered writes, same partition |

> **Tip:** These are front-end-visible bugs! "I posted a comment and it disappeared on refresh" is a replication lag bug. You've probably seen it in production.

![Read-your-writes, monotonic reads and consistent-prefix failures come from reading a lagging replica.](diagrams/replication-lag-anomalies.svg)

### Failover pitfalls

- **Split brain**: two nodes both think they're the leader and accept writes. Prevent it with majority quorums and **fencing tokens** (epochs or terms).
- **Lost writes**: with async replication, the new leader may lack the old leader's last writes.
- **Timeout tuning**: too short and you get false failovers during GC pauses; too long and downtime grows.

## Strategy 2: multi-leader (active-active)

Several leaders, each accepting writes, often one per region, replicating to each other.

- ✅ Local write latency in every region, and it tolerates region outages.
- ❌ **Write conflicts**: two regions edit the same record concurrently.

Conflict resolution approaches:

| Approach | How | Caveat |
|---|---|---|
| Last-writer-wins (LWW) | Highest timestamp wins | Silently loses data; clock skew |
| Merge by application | Custom logic | Complex |
| **CRDTs** | Data types that merge deterministically (counters, sets, text) | Limited data shapes |
| Avoid conflicts | Route each record's writes to a "home" region | Back to single-leader per record |

Examples include Cosmos DB multi-region writes, DynamoDB Global Tables (LWW), Cassandra, CouchDB, and collaborative editors (Google Docs, Figma use CRDT/OT ideas).

## Strategy 3: leaderless (Dynamo-style)

Clients write to (and read from) **several replicas directly**, using **quorums**.

With N replicas, a write needs acknowledgement from W nodes and a read queries R nodes. If **W + R > N**, the read and write sets overlap, so at least one node in the read set has the latest value.

```text
N=3, W=2, R=2:  write → {A,B} ✓   read → {B,C} → B has latest ✓
```

Repair mechanisms: **read repair** (fix stale replicas on read), **anti-entropy** (background Merkle-tree comparison) and **hinted handoff** (temporarily store writes for a down node).

Examples: **Cassandra, ScyllaDB, Riak**, and the original Amazon Dynamo paper (DynamoDB itself today uses leader-based Paxos per partition).

## Comparing the three

| | Single-leader | Multi-leader | Leaderless |
|---|---|---|---|
| Write conflicts | None | Yes | Yes (concurrent writes) |
| Write availability during partition | Only on the leader's side | High | High |
| Consistency | Strong possible | Eventual | Tunable (quorums) |
| Complexity | Low | High | Medium |

> **SA angle:** Map vendors to strategies. **MongoDB** uses single-leader per shard with majority writes (strong by default, failover in seconds). **Postgres** uses single-leader streaming replication (Patroni for HA on Kubernetes or OpenShift). **Delta Lake** sidesteps replication by delegating it to object storage (S3 offers eleven 9s of durability) plus optimistic commits to a log. Being able to place each system in this table is exactly the kind of clarity interviewers look for.

> **Interview:** A user posts a comment, refreshes, and it vanishes. Reads go to async replicas. What broke, and what do you change?

> **Model answer:** Read-your-writes failed because the refresh hit a lagging follower. Read the user's own writes from the leader, pin the session to one replica, or wait until that replica has caught the write's LSN. Do not make every read synchronous — that pays the latency on traffic that does not need it.

```quiz
Q: A user updates their profile, refreshes and sees the old data. Reads go to async followers. Which guarantee is missing?
- Monotonic reads
+ Read-your-writes consistency
- Linearizability of other users' writes
- Consistent prefix reads
? The user can't see their own write because the follower lags. Read from the leader for your own data, or use causal sessions.

Q: With N=5 replicas in a leaderless system, which settings guarantee reads overlap with the latest write?
- W=2, R=2
+ W=3, R=3
- W=1, R=3
- W=2, R=3
? W + R must exceed N. With N=5, W=3 and R=3 gives 6 > 5.

Q: What is the main downside of multi-leader replication?
- Writes must cross regions
+ Concurrent writes to the same data in different leaders create conflicts that must be resolved
- It can't tolerate failures
- It requires synchronous replication
? Every leader accepts writes independently, so conflict resolution (LWW, CRDTs, custom merges) is unavoidable.

Q: What prevents split brain during failover?
- Asynchronous replication
+ Majority quorums for leader election plus fencing tokens or terms that reject stale leaders
- Reading from followers
- Larger WAL files
? A leader needs a majority to be elected, and a stale leader's writes are rejected by its outdated term or epoch.
```
