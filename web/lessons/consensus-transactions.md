## The consensus problem

How do several nodes **agree on a value** (who is leader, what the next log entry is, whether a transaction commits) when nodes crash and messages are delayed?

Consensus gives you:
- **Leader election** without split brain
- **Replicated logs**: every node applies the same commands in the same order (a replicated state machine)
- **Atomic commit** and coordination metadata (which shard owns what)

The FLP result shows consensus is impossible in a fully asynchronous system with even one faulty process, so practical algorithms rely on **timeouts** (partial synchrony) and **majorities**.

## Raft in five minutes

Raft (2014) was designed to be understandable. It's used by etcd (and therefore Kubernetes and OpenShift), Consul, CockroachDB, TiKV, Kafka KRaft, and MongoDB's election protocol is Raft-like.

### Roles and terms
- Each node is a **Follower**, **Candidate** or **Leader**.
- Time is divided into **terms** (monotonic integers). Each term has at most one leader.

### Leader election

![A Raft follower that times out becomes a candidate; a leader commits an entry only after a majority has stored it.](diagrams/consensus-transactions-raft.svg)

- Each node votes **at most once per term**, and only for candidates whose log is at least as up to date as its own.
- **Randomised timeouts** make split votes rare.

### Log replication

The leader appends the client's entry to its own log, then sends `AppendEntries` with the previous index and term. Followers accept only if their log matches there; otherwise the leader backs up and retries. Once a **majority** has stored the entry it is **committed**: the leader applies it and replies to the client.

### Safety
- A committed entry is on a majority, and any new leader must have won votes from a majority. The two majorities overlap, so **the new leader has every committed entry**.
- A stale leader (an old term) is rejected the moment it talks to anyone with a newer term. That's the fencing.

> **Remember:** "Majorities overlap" is the core insight behind Raft, Paxos, quorum reads and replica set elections. If you can explain that sentence, you can explain all of them.

## Paxos, briefly

Paxos (Lamport, 1989/1998) is the original. It has the same majority-overlap foundation but is notoriously hard to understand and implement. Multi-Paxos variants power Google Spanner and Chubby, and DynamoDB's per-partition replication. In interviews, it's enough to say *"Raft and Multi-Paxos are equivalent in power; Raft is structured for understandability."*

## Distributed transactions: two-phase commit (2PC)

When one transaction spans several partitions or databases, all must commit or all must abort.

```text
Coordinator                 Participants (shards)
   │── PREPARE ───────────▶  write to log, lock rows, vote YES/NO
   │◀─ votes ─────────────
   │  (all YES?) write COMMIT decision to log
   │── COMMIT / ABORT ────▶  apply, release locks
```

- ✅ Atomic across nodes.
- ❌ **Blocking**: if the coordinator dies after PREPARE, participants hold locks until it recovers.
- ❌ Extra round trips mean added latency.

Modern systems make the coordinator fault-tolerant by **running 2PC over consensus groups**: each participant and the coordinator is a Raft or Paxos group, so no single crash blocks the protocol. Spanner, CockroachDB and **MongoDB distributed (cross-shard) transactions** work this way.

## Avoiding distributed transactions: sagas

In microservices, cross-service 2PC is usually a bad idea. A **saga** is a sequence of local transactions, each with a **compensating action**:

```text
1. Orders:    create order (PENDING)          ↩ cancel order
2. Payments:  charge card                     ↩ refund
3. Inventory: reserve stock                   ↩ release stock
4. Shipping:  create shipment
If step 3 fails → run compensations 2, 1 in reverse
```

- **Choreography**: services react to each other's events (Kafka topics).
- **Orchestration**: a central workflow engine (Temporal, AWS Step Functions, Camunda) drives the steps.

Sagas give up isolation, so intermediate states are visible. Design for it with "PENDING" states and idempotency.

## The outbox pattern: reliable events

Problem: you need to update the database **and** publish an event to Kafka. Two separate writes can't be atomic, and **dual writes** cause inconsistency.

```text
BEGIN;
  INSERT INTO orders ...;
  INSERT INTO outbox (aggregate_id, type, payload) VALUES (..., 'OrderCreated', '{...}');
COMMIT;
-- Debezium tails the WAL → publishes outbox rows to Kafka (exactly the committed ones)
```

This one database transaction guarantees the event is published if and only if the order exists. In MongoDB, **change streams** play the same role natively.

## Idempotency: the unsung hero

Networks retry, so every operation in a distributed workflow should be safe to apply twice:

- Idempotency keys on APIs (as Stripe does)
- Upserts keyed by event ID
- "Insert if not exists" deduplication tables
- Kafka idempotent producers plus transactional consumers for exactly-once processing within Kafka

> **SA angle:** Red Hat interviews love this territory: Kafka (Streams for Apache Kafka), Debezium for outbox/CDC, and etcd under OpenShift. MongoDB interviews ask about transactions across shards and change streams. Databricks interviews ask about exactly-once streaming into Delta Lake (idempotent writes and checkpointing). Prepare one diagram, *"order service with outbox → Kafka → consumers"*, and adapt it to each company.

> **Interview:** The order service must reserve stock, charge a card, and publish `OrderCreated` to Kafka. 2PC, saga, or outbox — which, and why?

> **Model answer:** Three different problems. Cross-shard atomicity inside one database is 2PC over consensus — classic blocking 2PC is why you run the coordinator on a Raft group. Across services, 2PC is usually a bad idea: use a saga of local transactions with compensations, and accept that intermediate states are visible. Dual-write to Postgres and Kafka is neither — use a transactional outbox (or MongoDB change streams) so the event is published if and only if the row commits. Put an idempotency key on every step.

```quiz
Q: Why does a newly elected Raft leader always have all committed entries?
- It copies the log from the old leader before starting
+ Committed entries are on a majority, and winning an election needs votes from a majority whose logs are no newer than the candidate's; the majorities overlap
- Followers never fail
- The client resends all entries after an election
? The election restriction plus majority overlap guarantees safety.

Q: What is the main weakness of classic two-phase commit?
+ It blocks if the coordinator fails after participants have voted YES, leaving locks held
- It can't guarantee atomicity
- It only works on a single node
- It requires eventual consistency
? Participants in the prepared state can't decide alone. Running the coordinator on a consensus group fixes this.

Q: An order service writes to Postgres and then publishes to Kafka. Sometimes the event is missing. What pattern fixes this?
- Retry the Kafka publish forever
- Use two-phase commit with Kafka
+ Transactional outbox: write the event to an outbox table in the same DB transaction and have CDC publish it
- Publish first, then write to the database
? The outbox makes the state change and the event atomic. CDC (e.g. Debezium) relays it reliably.

Q: What does a saga use instead of a global atomic commit?
- Distributed locks
+ A sequence of local transactions with compensating actions to undo completed steps on failure
- Linearizable reads
- A single shared database
? Sagas trade isolation for availability and loose coupling. Compensations restore consistency.
```
