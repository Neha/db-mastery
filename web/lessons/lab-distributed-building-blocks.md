## Two building blocks, two packages

| Package | Concept | Used by |
|---|---|---|
| `labs/hashring` | Consistent hashing with virtual nodes, replica placement | Cassandra, Dynamo, Riak, CDNs, cache clients |
| `labs/mvcc` | Multi-version concurrency control with snapshot isolation | Postgres, MongoDB WiredTiger, CockroachDB, Delta Lake (table versions) |

```bash
go test ./labs/hashring ./labs/mvcc -v
```

## Part 1: consistent hashing

```go
ring := hashring.New(200)      // 200 virtual nodes per physical node
ring.Add("node-1"); ring.Add("node-2"); ring.Add("node-3")
owner := ring.Get("user:42")              // first vnode clockwise from hash(key)
replicas := ring.GetN("user:42", 3)       // preference list: 3 distinct nodes clockwise
```

How it works:

- Each node is hashed onto a 32-bit ring at `vnodes` positions (`node-1#0`, `node-1#1`, …).
- Positions are kept in a **sorted slice**, and lookup is a **binary search** for the first position ≥ `hash(key)`, wrapping at the end.
- `GetN` keeps walking clockwise, skipping vnodes of nodes already chosen. That's how Dynamo picks replicas.

### What the tests show

```text
4 → 5 nodes: consistent hashing moved 18.1% of keys, modulo moved 79.6%
vnodes=  1 → worst node deviates 90.2% from ideal
vnodes= 10 → worst node deviates 12.8% from ideal
vnodes=100 → worst node deviates 11.1% from ideal
vnodes=500 → worst node deviates  7.5% from ideal
```

- Adding a 5th node ideally moves **1/5 = 20%** of keys. Consistent hashing gets 18%, while `hash % N` reshuffles 80%. That's 4x more data movement during scaling.
- With **one** position per node, load is wildly uneven (one node had 90% more than its share). Virtual nodes smooth it out, with diminishing returns.

## Part 2: MVCC with snapshot isolation

```go
s := mvcc.New()
tx := s.Begin()                 // snapshot = all commits so far
v, _ := tx.Get("balance")       // newest version with commitTS ≤ startTS
tx.Set("balance", "50")         // buffered privately (no dirty reads)
err := tx.Commit()              // ErrConflict if someone committed this key after our snapshot
```

Every key stores a list of versions `{value, deleted, commitTS}`. The rules:

1. **Reads** pick the newest version with `commitTS ≤ startTS`, so readers never block and never see later commits.
2. **Writes** stay in the transaction's private buffer, so there are no dirty reads, and your own writes are visible to you.
3. **Commit** checks every written key: if its latest version's `commitTS > startTS`, someone else got there first, so **abort** (first-committer-wins). That prevents lost updates.
4. **GC** removes versions older than what the oldest active snapshot needs (Postgres' VACUUM in miniature).

### What the tests prove

| Test | Demonstrates |
|---|---|
| `TestSnapshotReadsIgnoreLaterCommits` | Repeatable reads from a stable snapshot |
| `TestNoDirtyReadsAndReadYourWrites` | Isolation of uncommitted writes |
| `TestFirstCommitterWins` | Lost update prevented with `ErrConflict` |
| `TestConcurrentIncrementsWithRetry` | 20 goroutines × 50 increments = exactly 1000, using a retry loop |
| `TestWriteSkewIsAllowedUnderSnapshotIsolation` | The doctors problem: **both go off call**. SI doesn't prevent write skew |
| `TestGarbageCollection` | A long-running snapshot pins old versions; GC reclaims them once it ends |

> **Key idea:** `TestGarbageCollection` *is* the "idle-in-transaction connection causes Postgres bloat" incident, in 20 lines. One old snapshot prevents cleanup of every newer version. You can now explain that production issue from first principles.

## Exercises

> **Exercise 1 (weighted nodes):** Let `Add(node, weight)` assign `vnodes × weight` positions so bigger machines take proportionally more keys. Test the distribution.

> **Exercise 2 (bounded loads):** Implement "consistent hashing with bounded loads" (Google, 2017): if a node exceeds `(1+ε) × average` load, continue clockwise. Measure the max load with a skewed key distribution.

> **Exercise 3 (rendezvous hashing):** Implement highest-random-weight hashing as an alternative and compare movement and balance.

> **Exercise 4 (serializable):** Make the MVCC store **serializable** by tracking each transaction's read set and aborting at commit if any key it *read* was modified after its snapshot (optimistic validation). Show the write-skew test now aborts one transaction.

> **Exercise 5 (distributed KV):** Combine all three labs: an HTTP front-end uses `hashring.GetN(key, 3)` to route writes to 3 `kvstore` instances (goroutines or processes) with W=2, R=2 quorums. Kill one and show reads still succeed.

```quiz
Q: In the test, why did consistent hashing move ~18% of keys when going from 4 to 5 nodes?
+ Only keys in the ring segments now owned by the new node's vnodes move, ideally 1/5 of keys
- Because 18% of keys were corrupted
- Because the ring was rebuilt from scratch
- Because modulo hashing was used internally
? The new node takes over slices of the ring from its neighbours. Everything else stays put.

Q: Why does the MVCC store allow write skew?
- Because it has no conflict detection at all
+ It only detects write-write conflicts on the same keys; write skew involves transactions writing different keys based on overlapping reads
- Because GC removed the versions
- Because reads aren't from a snapshot
? Detecting write skew requires tracking read sets (as SSI does). That's Exercise 4.

Q: What causes old versions to be retained in TestGarbageCollection?
+ An active transaction with an old snapshot might still need to read them
- The GC runs only once per hour
- Versions are never deleted in MVCC
- The commit timestamp overflowed
? GC can only remove versions invisible to every active snapshot. Long transactions block cleanup.
```
