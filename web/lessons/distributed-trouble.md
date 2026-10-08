## Partial failure is the defining property

On one machine, a call either returns or the process is dead. In a distributed system a remote node can be **dead, slow, paused, or partitioned from you but healthy to someone else**, and you cannot tell which. Packets, clocks and garbage collectors all lie. This chapter is why [replication](#/lesson/replication), [quorums](#/lesson/consistency-cap) and [consensus](#/lesson/consensus-transactions) exist.

> **Key idea:** You never observe "node B has crashed". You observe "I have had no reply that I trust." Timeouts are guesses.

## Unreliable networks

Messages can be **lost, delayed, duplicated or reordered**. TCP repairs loss and reorder *on one connection*; it does not bound delay, and it does not tell you whether the remote process applied the request before the connection died. A retry may therefore run the same write twice (hence idempotency keys).

Delays are **unbounded** on the networks we actually use. A packet can sit in a switch buffer, a overloaded NIC, or a kernel queue for far longer than your 200 ms timeout. The timeout firing means only that *you* gave up.

> **Warning:** A short timeout plus automatic failover is how you manufacture a split brain during a GC pause. A long timeout is how you sit dark while a node is truly dead. There is no correct number, only a trade-off.

Failure detectors are probabilistic. φ-accrual detectors (used around Cassandra and Akka) track recent inter-arrival times and raise a *suspicion* score instead of a boolean. They still guess.

## Process pauses

A process that is "up" can still stop handling packets:

- **Stop-the-world garbage collection** in the JVM and similar runtimes. Typical pauses are milliseconds; multi-second (and, in pathological heaps, far longer) pauses happen.
- **Disk stalls** (`fsync` on a dying volume), swapping, noisy neighbours on a shared VM.
- **OS and hypervisor scheduling**, live migration, a stopped debugger.

From the outside this is indistinguishable from a crash plus a late resurrection. The process then continues with **stale in-memory state**: it still believes it is the leader, still holds a lock, still has a generation number from before the pause.

That is why lease-based leadership must be **fenced** at the storage layer, not trusted in the client.

## Unreliable clocks

Two kinds of clock, and they are not interchangeable:

| Clock | What it measures | Across machines? | Can it jump? |
|---|---|---|---|
| **Time-of-day** (wall clock, `CLOCK_REALTIME`) | Civil time, Unix epoch | Yes, if you trust NTP | Yes: NTP step, admin change, **leap seconds** |
| **Monotonic** (`CLOCK_MONOTONIC`) | Elapsed time since an arbitrary boot point | **No** — origins differ | Not backwards; still pauses if the process pauses |

Use monotonic clocks for timeouts and "how long did this take?". Use time-of-day for display, expiry that humans understand, and *only with care* for ordering.

**NTP** usually keeps well-run LAN hosts within tens of milliseconds, and internet hosts within tens to hundreds. It can also **step backwards**. A leap second inserts an extra 23:59:60; smearing (what the large public clouds do) spreads that second over hours so clocks stay monotonic. Applications that assume "time only moves forward by 1× real time" still break.

Do not implement last-writer-wins with wall-clock timestamps across nodes. The later *real* write can carry the smaller timestamp.

![Node A is 80 ms fast and writes first; node B is 20 ms slow and writes later. Last-writer-wins keeps A's value because B's wall timestamp is smaller.](diagrams/distributed-trouble-clock-skew.svg)

Google Spanner's TrueTime is the honest version of "use a clock": GPS and atomic clocks with an **uncertainty interval**. A commit waits out the uncertainty. Most of us do not have that, so we use **logical clocks** (Lamport, vector, hybrid logical clocks) or we refuse to order events by wall time. See [Consistency and CAP](#/lesson/consistency-cap).

> **SA angle:** If a design says "the replica with the newest timestamp wins", ask how clocks are synced and what happens during an NTP step. MongoDB's cluster time is a hybrid logical clock, not `Date.now()`.

## Fencing tokens

A **fencing token** is a number that only goes up (a lock generation, a Raft term, an etcd revision, a ZooKeeper `zxid`). Storage accepts a write only if the token is **greater than or equal to** the last token it has seen. A paused leader can wake and send a write; the storage rejects it.

![Client A pauses after receiving token 33. Client B is fenced in with 34 and writes. A's late write is rejected because the store has already seen 34.](diagrams/distributed-trouble-fencing.svg)

The lock service alone is not enough. If the store is a disk, an object API or a database, *it* must enforce the token. ZooKeeper / etcd give you the monotonically increasing number; your data plane has to check it.

This is the same idea as Raft terms: a stale leader of term 5 is ignored the moment a follower has seen term 6.

## Byzantine faults versus crash-stop

Most data systems assume **crash-stop** (a node fails by stopping) plus **omission** (it may drop or delay messages). They do not assume a node will send **contradictory or fabricated** messages.

**Byzantine** faults do that: a compromised or buggy node lies to different peers. Tolerating them needs more replicas, signed messages or both (PBFT, Tendermint, blockchains). You do not get this from Raft or a MongoDB replica set.

In interviews, say: *"We assume crash-stop and unbounded delay, not Byzantine. Majority quorums give safety under that model. If a node can lie, we are in a different protocol family."*

## Majority is the source of truth

A **majority** (strictly more than `n/2` of a fixed replica set) has a property no minority has: **any two majorities overlap**. Therefore:

- A value that is stored on a majority cannot be lost if a minority crashes.
- A newly elected leader that needs a majority of votes **must have seen every value the previous majority committed**.
- During a partition, **at most one side** can gather a majority. The other side must stop taking writes.

That overlap is why Raft, Paxos, etcd, ZooKeeper, MongoDB `w:majority` and Cassandra `QUORUM` (when `R + W > N`) all work. The bytes on one disk are not the truth. The **quorum** is.

> **Remember:** "The primary said so" is not a safety argument. "A majority stored it, and elections require a majority" is.

## Split brain: two leaders, one cluster

**Split brain** is two nodes both accepting writes for the same data, usually because a network partition (or a pause that looks like one) made each side decide the other was dead.

What you observe: conflicting rows, unique-key violations after the heal, clients that read different values depending on which load balancer they hit, a failover that "succeeded" twice.

Prevention is mechanical:

1. **Quorum for leadership.** A node becomes leader only with a majority. The minority side goes read-only or dark.
2. **Fencing tokens** so a stale primary cannot write after a new one has been elected.
3. **A single lock service** that is itself a consensus group (etcd under Kubernetes / OpenShift, ZooKeeper, a Raft-backed operator). STONITH ("shoot the other node in the head") in Pacemaker-style HA is the brutal version of fencing.

Do not prevent split brain with "a very short ping timeout" or "NTP will keep them aligned". Those make it more likely.

## Problem: two primaries after a network blip

> **Real world:** A two-node Patroni/etcd Postgres pair (or a replica set someone ran with `w:1` and a hair-trigger failover) partitions for 40 seconds. When the link returns, both sides have accepted orders. Unique constraints start failing. The interviewer asks what happened and how you would design it so it cannot.

**Symptoms.** Two nodes with `primary` in their status. Application logs show writes succeeding on both. After reconnect: duplicate keys, divergent sequences, or a replica that cannot resume because histories forked.

**Diagnose.** Timeline: when did each node lose the other, what write concern / sync standings were in force, did etcd/DCS have a majority, and which generation or timeline ID each Postgres node is on. Check whether the load balancer still sent traffic to both.

**Root causes.** Most likely: failover decided with a **minority** (a two-node cluster has no safe majority if one node is gone — you need three voting members), **no fencing** of the old primary's writes, or async commit so the new primary never had the old primary's last transactions.

**Fix.** Immediately take one side read-only, freeze writes, and reconcile by application identity (idempotency keys), not by wall-clock timestamps. Restore from the timeline you choose as canonical. Do not "merge the WALs" by hand unless you have a practised procedure.

**Prevent.** Three voters so a partition leaves one majority. Majority commit before ack. Fencing: the old primary must fail writes as soon as it cannot refresh its leader lock. Timeouts longer than typical GC. This is the same story as [replication failover](#/lesson/replication) and [Raft elections](#/lesson/consensus-transactions).

> **Model answer — what to say in the interview:** Split brain means two primaries accepted writes because each thought the other was dead. I would not trust wall clocks or a two-node majority — two nodes cannot form a majority if one is unreachable. I want three voting members, a consensus lock (etcd/Raft) that issues a fencing token, and the storage or old primary refusing writes with a stale token. After the fact I pick one timeline as truth and reconcile with idempotency keys, not timestamps. The lasting fix is quorum plus fencing, not a shorter ping.

```quiz
Q: A timeout expires waiting for a replica. What do you actually know?
- The replica crashed
- The replica is partitioned from every other node
+ You have not received a reply you trust within the time you chose to wait
- The replica's clock is behind
? Loss, delay, pause and crash are indistinguishable from one side. Timeouts are guesses.

Q: Which clock should you use to implement "retry after 200 ms"?
- Time-of-day, so it matches the user's watch
- The replica's advertised wall time
- A leap-smeared UTC timestamp stored in the database
+ A monotonic clock, which measures elapsed time and does not jump backwards with NTP
? Monotonic clocks are for durations. They are not comparable across machines.

Q: Last-writer-wins using wall-clock timestamps across two regions is dangerous because:
- NTP is always accurate to the microsecond
+ Clocks skew and can step, so the later real write can carry the smaller timestamp and be discarded
- Monotonic clocks jump on leap seconds
- Quorums forbid timestamps
? LWW plus clock skew is a silent lost-update bug. Use a logical clock or a leader.

Q: What is a fencing token for?
- Encrypting replication traffic
+ A monotonically increasing generation that storage checks, so a paused ex-leader's writes are rejected
- Bounding NTP error
- Detecting Byzantine replicas
? The lock service issues the number; the data plane must enforce it.

Q: Why is a majority the source of truth, not "the node that was primary"?
- Primaries never pause
+ Any two majorities overlap, so a committed value cannot vanish and at most one side of a partition can keep writing
- Majority equals every replica
- Majority is faster than talking to one node
? Overlap is the whole argument for Raft, etcd and w:majority.

Q: Two nodes both accept writes for the same key during a partition. This is:
+ Split brain
- A Byzantine attack
- Causal consistency
- A successful quorum write
? Split brain is two actives. Prevent it with quorum elections and fencing, not with shorter pings.
```
