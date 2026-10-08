## What "consistency" means (it's overloaded)

| Context | Meaning |
|---|---|
| ACID "C" | Invariants and constraints hold |
| CAP "C" | **Linearizability**: the system behaves like a single copy |
| "Eventual consistency" | Replicas converge if writes stop |
| Isolation levels | How concurrent *transactions* interleave |

When a customer says "we need consistency", ask which one they mean.

## The spectrum of consistency models

From strongest to weakest:

1. **Linearizability (strong consistency)**: once a write completes, every later read (by anyone) sees it. The system acts like one copy. Examples: etcd, ZooKeeper, Spanner, MongoDB with `majority` writes plus `linearizable` reads.
2. **Sequential consistency**: everyone sees operations in the same order, but not necessarily in real-time order.
3. **Causal consistency**: operations that are causally related (a reply after a post) are seen in order by everyone; concurrent ones may differ. MongoDB **causal sessions** give you this.
4. **Read-your-writes / monotonic reads**: session-level guarantees.
5. **Eventual consistency**: replicas converge eventually, with no ordering promises meanwhile. Examples: DNS, Cassandra at `ONE`, S3 historically (S3 has been strongly consistent since 2020).

> **Key idea:** Stronger models need more coordination (round trips, quorums), which costs latency and availability. Pick the **weakest model that keeps the business correct**.

## CAP: the honest version

The CAP theorem (Brewer, 2000; proved by Gilbert and Lynch, 2002): when a **network Partition** happens, a distributed system must choose between **Consistency** (linearizability) and **Availability** (every request to a non-failed node gets a non-error response).

Common misconceptions:

- ❌ "Pick 2 of 3." Partitions aren't optional, so the real choice is C or A **during a partition**.
- ❌ "MongoDB is CP, so it's never available." The minority side refuses writes during a partition, while the majority side keeps working. With the defaults it's CP-leaning; with `secondary` reads plus `w:1` it behaves more AP.
- ❌ CAP says nothing about latency or normal operation.

| During a partition… | Behaviour | Examples |
|---|---|---|
| **CP** | Minority side rejects writes (and perhaps reads) to avoid divergence | etcd, ZooKeeper, HBase, MongoDB (default), Spanner |
| **AP** | All sides accept writes, then reconcile later | Cassandra, Riak, DynamoDB Global Tables, CouchDB |

## PACELC: the more useful framing

**PACELC** (Abadi, 2010): if **P**artition, choose **A** or **C**; **E**lse (normal operation), choose **L**atency or **C**onsistency.

![During a partition you choose C or A; otherwise you still trade latency against consistency.](diagrams/consistency-cap-pacelc.svg)

The "else" half is what customers feel every day: synchronous replication across regions adds about 70 ms or more per write.

| System | P → | E → | Notes |
|---|---|---|---|
| DynamoDB (default reads) | A | L | Eventually consistent reads by default; strong reads optional |
| Cassandra | A | L | Tunable per query |
| MongoDB (defaults) | C | C | `w:majority` is the default; tunable towards L |
| Spanner / CockroachDB | C | C | Pays latency for global consistency (TrueTime / HLC) |
| Cosmos DB | tunable | tunable | Five consistency levels |

## Quorums and tunable consistency

With N replicas, write quorum W and read quorum R:

- **W + R > N**: reads see the latest acknowledged write (with caveats around concurrent writes and sloppy quorums).
- Cassandra levels: `ONE`, `QUORUM`, `LOCAL_QUORUM` (within a datacentre), `ALL`.
- MongoDB's equivalents are write concern + read concern + read preference.

```text
                   Latency ◀───────────────────────▶ Consistency
MongoDB:  w:1 + secondary reads ...... w:majority + readConcern majority ...... + linearizable
Cassandra: ONE/ONE .................. QUORUM/QUORUM ........................... ALL
```

## Time and ordering

Distributed systems have no global clock. Clocks drift, and NTP adjusts them backwards.

- **Lamport clocks**: a logical counter giving a total order consistent with causality.
- **Vector clocks**: detect concurrent vs causally ordered updates (Dynamo, Riak).
- **Hybrid logical clocks (HLC)**: physical time plus a logical counter. Used by CockroachDB, YugabyteDB and MongoDB's cluster time.
- **TrueTime** (Spanner): GPS and atomic clocks with bounded uncertainty; waits out the uncertainty before committing.

> **Warning:** Last-writer-wins with wall-clock timestamps silently loses data when clocks skew. That's a common hidden bug in multi-region designs.

> **SA angle:** Strong interview answers sound like this: *"For the payment ledger I'd want linearizable writes, so MongoDB with majority write concern and reads from the primary, accepting a few milliseconds of latency. For the product view counter, eventual consistency is fine, so I'd use `w:1` or batch increments. That keeps the expensive guarantees where the business needs them."*

> **Interview:** "We're a CP shop" / "CAP says pick two." What do you say?

> **Model answer:** Partitions happen, so you choose C or A *during a partition*, not two of three. The everyday trade-off is PACELC's else-clause: latency versus consistency when the network is fine. Pick the weakest model that keeps the business correct — majority writes and primary reads for a ledger; eventual for a view counter. Do not call MongoDB "unavailable" because the minority side refuses writes; the majority side keeps serving.

```quiz
Q: According to CAP, what must a system choose between when a network partition occurs?
- Latency and throughput
+ Consistency (linearizability) and availability
- Durability and isolation
- Replication and partitioning
? Partitions happen, so the real choice is C vs A while the partition lasts.

Q: What does the "ELC" part of PACELC add?
+ In normal operation (no partition), systems still trade latency against consistency
- Elections happen when latency is high
- Eventual consistency is the default everywhere
- Extra replicas are needed for linearizability
? Coordination costs latency even without failures. That's the everyday trade-off customers feel.

Q: A social app shows a reply before the original post for some users. Which model would prevent this while remaining cheaper than linearizability?
- Eventual consistency
+ Causal consistency
- Read uncommitted
- Last-writer-wins
? Causal consistency orders causally related operations (post then reply) for every observer.

Q: Why is last-writer-wins with wall-clock timestamps risky?
- It's too slow
+ Clock skew can make an older write appear newer, silently discarding the real latest update
- It requires consensus
- It only works on a single node
? Physical clocks drift. LWW then drops data without any error.
```
