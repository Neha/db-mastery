## Replica sets: high availability

A **replica set** is a group of `mongod` processes holding the same data:

![Writes go to the primary; secondaries tail its oplog. A majority elects a new primary if it fails.](diagrams/mongodb-replication-sharding-replica-set.svg)

- One **primary** accepts all writes and records them in the **oplog** (a capped collection of idempotent operations).
- **Secondaries** tail the oplog and apply the operations asynchronously.
- If the primary fails, the remaining members hold an **election** (a Raft-inspired protocol) and a new primary is elected, typically within about **5–12 seconds**. Drivers detect this and retry (**retryable writes and reads**, on by default).
- You need a **majority** to elect a primary, so use an **odd number** of voting members (3 or 5) spread across failure domains (AZs or regions).

### Member types
| Member | Purpose |
|---|---|
| Primary | Accepts writes |
| Secondary | Replicates; can serve reads; eligible for election |
| Hidden / delayed | Backups, analytics, protection from "oops" deletes (delayed by N hours) |
| Arbiter | Votes but holds no data (generally discouraged; it weakens majority durability) |
| Analytics nodes (Atlas) | Tagged secondaries isolated for BI and analytics workloads |

## Write concern: how durable is "OK"?

| Write concern | Acknowledged when… | Trade-off |
|---|---|---|
| `w: 0` | Never (fire and forget) | Fastest, may lose data silently |
| `w: 1` | Primary has applied it | Can be **rolled back** if the primary fails before replicating |
| `w: "majority"` (**default**) | A majority have it in memory or durably (with `j`) | Survives failover |
| `j: true` | Written to the on-disk journal | Durable on that node |

## Read concern: what can you see?

| Read concern | Returns |
|---|---|
| `local` (default) | The node's latest data, which may later be rolled back |
| `majority` | Only majority-committed data (never rolled back) |
| `linearizable` | Reflects all majority writes that completed before the read began (primary only, slower) |
| `snapshot` | A consistent point-in-time view (transactions) |

## Read preference: where do reads go?

`primary` (default), `primaryPreferred`, `secondary`, `secondaryPreferred`, `nearest`, plus **tag sets** (for example, route BI to analytics nodes).

> **Warning:** Reading from secondaries gives you **eventual consistency**. You may not see your own write. Use **causally consistent sessions** to guarantee read-your-writes and monotonic reads across members. Don't send reads to secondaries "for scale" by default; scale with sharding or bigger nodes, and use secondaries for geo-locality or isolation.

## Sharding: horizontal scale-out

When one replica set can't handle the data size or write throughput, **shard**: split the collection across many replica sets.

![A monotonic ranged key dumps every insert on the last chunk; a compound key with a well-distributed prefix spreads writes and keeps customer queries targeted.](diagrams/mongodb-replication-sharding-shard-key.svg)

- **Shard key**: a field (or fields) that decides which shard holds each document.
- Data is split into **chunks** (contiguous shard-key ranges; 128 MB by default in recent versions).
- The **balancer** migrates chunks to even out data across shards.
- `mongos` routes queries: **targeted** (the query includes the shard key, so it goes to one shard) or **scatter-gather** (it goes to all shards).

### Choosing a shard key: the most important decision

A good shard key has:
1. **High cardinality**: many distinct values, so data can split into many chunks.
2. **Low frequency**: no single value dominates (avoid jumbo chunks).
3. **Non-monotonic** writes: avoid every insert hitting the "last" chunk.
4. **Query isolation**: common queries include it, so they're targeted.

| Shard key | Verdict |
|---|---|
| `{ _id: 1 }` with ObjectId | ❌ Monotonic, so every insert hits one shard (a hot shard) |
| `{ _id: "hashed" }` | ✅ Even writes, ❌ range queries become scatter-gather |
| `{ country: 1 }` | ❌ Low cardinality, with jumbo chunks for big countries |
| `{ customerId: 1, orderDate: 1 }` | ✅ Good cardinality, targeted per-customer queries, no single hot spot |
| `{ tenantId: 1, _id: 1 }` | ✅ Classic multi-tenant SaaS key |

**Ranged** sharding preserves locality (efficient range queries) but risks hotspots. **Hashed** sharding spreads writes evenly but scatters ranges. **Zone sharding** pins key ranges to shards in particular regions (data residency, for example EU customers stay in the EU).

> **Interview:** Why is a monotonically increasing shard key a problem, and what do you choose instead?

> **Model answer:** Every insert lands in the chunk with the highest key, so one shard takes all the write load (a hot shard). Hash the key if you only need even writes; prefer a compound key with a high-cardinality, well-distributed prefix you also query on — `{ customerId: 1, createdAt: 1 }` for orders. Mention online resharding as the safety net, not the plan.

> **Tip:** Since MongoDB 5.0 you can **reshard** a collection online (`reshardCollection`), and 4.4+ lets you refine a shard key by adding suffix fields. That's a big improvement over the old "shard key is forever" rule, and worth mentioning in interviews. Since 8.0 there's also faster resharding and the ability to move unsharded collections between shards.

## Global clusters

Atlas **Global Clusters** use zone sharding with a location field in the shard key, so writes and reads for European users go to EU shards, US users to US shards, and so on. Low latency and data sovereignty come from a single logical database.

> **SA angle:** Shard key selection is the #1 MongoDB architecture interview topic. Practise this answer structure: *"I'd look at the top 3 queries and the write pattern. For an orders workload I'd choose `{customerId: 1, createdAt: 1}`: high cardinality, the customer-centric queries are targeted, and it isn't monotonic across the whole key space. I'd avoid a hashed `_id` because order history queries would scatter."* Then mention resharding as the safety net.

```quiz
Q: Why should a replica set have an odd number of voting members?
+ Elections need a strict majority; an odd number avoids ties and maximises fault tolerance per node
- Odd numbers replicate faster
- MongoDB rejects even-sized replica sets
- So there is always an arbiter
? With 3 voters you tolerate 1 failure, and with 5 you tolerate 2. A 4th voter adds no extra tolerance.

Q: With write concern w:1, what can happen if the primary crashes right after acknowledging?
- Nothing; the write is guaranteed
+ The write may be rolled back if it hadn't replicated before a new primary was elected
- The write is automatically retried on the secondary
- The cluster shuts down
? Only majority-acknowledged writes are guaranteed to survive failover.

Q: Why is a monotonically increasing ObjectId a poor ranged shard key?
- It has low cardinality
+ All new inserts go to the chunk with the highest values, creating a hot shard
- ObjectIds can't be indexed
- It makes documents larger
? Monotonic keys concentrate writes at the max-key chunk. Use a hashed key or a compound key with a well-distributed prefix.

Q: A query on a sharded collection doesn't include the shard key. What happens?
- It fails
+ mongos broadcasts it to all shards (scatter-gather) and merges the results
- It only searches the primary shard
- The balancer runs first
? It still works but is less efficient. Targeted queries hit only the shards that own the relevant chunks.
```
