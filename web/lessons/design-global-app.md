## The prompt

> "A fintech app is expanding from the UK to the EU, US and India. Users need low latency everywhere. Regulators require EU and Indian customer data to stay in-region. The ledger must never be wrong. Target availability is 99.99%."

## 1. Clarify

- Do users travel? (A UK user in New York: whose region serves them?)
- Which data is regulated (PII, transactions) vs global (product catalogue, FX rates)?
- Ledger semantics: per-account strong consistency, or cross-account global transactions?
- RPO/RTO per data class?
- Is the customer on one cloud, multi-cloud or hybrid (on-prem data centres in India)?

## 2. Classify data, the key move

| Data | Consistency | Locality | Pattern |
|---|---|---|---|
| User profile and PII | Strong per user | **Pinned to home region** | Partition by home region |
| Ledger / balances | **Strong (linearizable per account)** | Home region | Single-writer per account |
| Sessions, preferences | Read-your-writes | Follow the user | Cache plus home-region source |
| Product catalogue, FX rates | Eventual | Global, read everywhere | Replicate everywhere |
| Analytics | Eventual | Aggregated, anonymised | Regional lakes, federated governance |

> **Key idea:** "Multi-region" isn't one decision. **Classify data, then pick a replication and locality strategy per class.** This is what separates senior answers.

## 3. Strategy options

### A. Home-region partitioning (single-writer per record) ✅ recommended for the ledger
Each user and account has a **home region**. All writes for that account go there, so there are no write conflicts and you get strong consistency within the region. Users travelling abroad pay cross-region latency for writes (100–150 ms, acceptable for payments).

**MongoDB**: a **global cluster with zone sharding**. The shard key starts with `region` (`{ region: 1, accountId: 1 }`), and zones map `EU` to EU shards, `IN` to India shards, and so on. Each zone's replica set spans 3 AZs in-region, with optional read-only members elsewhere.

**Postgres**: separate regional clusters (Patroni or CloudNativePG on OpenShift, or Aurora per region) with an application routing layer. For Postgres-compatible geo-partitioning, consider distributed SQL (CockroachDB or YugabyteDB with row-level region pinning).

### B. Active-active multi-leader
Every region accepts writes for any record, with asynchronous conflict resolution. It suits likes, presence and carts with CRDTs. ❌ **Never for ledgers** (conflicting debits).

### C. Global strong consistency (Spanner-style)
Consensus across regions on every write, so it's always correct but every write pays cross-region latency (and Spanner or CockroachDB adoption). It's justified when you truly need global transactions across regions.

## 4. Architecture

```text
            Global DNS / anycast LB (latency routing) + edge auth
                 │                  │                   │
          ┌──── EU (Frankfurt) ──┐ ┌── UK (London) ──┐ ┌── IN (Mumbai) ─┐ ┌── US ──┐
          │ API services (k8s)   │ │ …               │ │ …              │ │ …      │
          │ Zone shard: EU data  │ │ Zone shard: UK  │ │ Zone shard: IN │ │ …      │
          │  RS across 3 AZs     │ │                 │ │                │ │        │
          └──────────┬───────────┘ └───────┬─────────┘ └──────┬─────────┘ └───┬────┘
                     └─────── one logical sharded cluster (global cluster) ────┘
   Global read-only data (catalogue, FX) replicated to every region; PII never leaves its zone
```

Request flow for a UK user in New York:
1. US edge authenticates and looks up the home region (in a global, non-PII directory: `userId → region`).
2. Reads of non-sensitive cached data come from the US. Ledger writes route to the UK zone.
3. Alternatively, the API in the US calls the UK API, keeping all PII processing in the UK.

## 5. The ledger itself

- **Double-entry**, append-only entries: `{entryId, accountId, amount, currency, txnId, ts}`. Balances are derived or materialised atomically with the entries.
- Idempotency key per payment request, enforced with a unique index on `(txnId, accountId)`.
- Majority write concern, linearizable or majority reads for balance checks.
- Cross-region transfers (UK to EU account): a **saga** with escrow (debit the UK account into "in-flight", credit EU, confirm), or a dedicated settlement service. Avoid synchronous cross-region 2PC in the hot path.

## 6. Availability math and DR

- 99.99% means about 52 minutes of downtime per year. A single AZ can't deliver it, so use 3 AZs per region.
- Regional outage: for regulated data, **failover must stay in-jurisdiction** (EU to another EU region, not the US). Design each zone with an in-country DR region where possible, or accept a higher RTO.
- Backups: continuous PITR, regional and encrypted with regional keys (customer-managed KMS).
- Run regular game days that simulate a region failure.

## 7. Security and compliance

Encryption in transit and at rest with **per-region keys**, field-level or **Queryable Encryption** for the most sensitive fields, audit logging, least-privilege access, private networking (PrivateLink or peering), and documented data flows for regulators.

> **SA angle:** **MongoDB**: global clusters plus zone sharding is a signature differentiator, so practise drawing it. **Red Hat**: the twist is often hybrid, with India requiring on-prem. OpenShift gives one platform across the Mumbai data centre and public cloud regions, with operators managing databases consistently and ACM managing all clusters. **Databricks**: regional workspaces with Unity Catalog governance, and **Delta Sharing** to share only anonymised aggregates across regions.

```quiz
Q: Why is active-active multi-leader replication unsuitable for a ledger?
+ Concurrent writes to the same account in different regions can conflict, and automatic resolution like LWW can lose or duplicate money
- It's too slow
- It doesn't support encryption
- It can't span more than two regions
? Ledgers need a single source of truth per account, such as home-region single-writer or global consensus.

Q: In a MongoDB global cluster, how is EU data kept in the EU?
- By using read preference nearest
+ Zone sharding: a shard key starting with region, with zones mapping EU key ranges to shards located in EU regions
- With a TTL index
- By encrypting the data
? Zone sharding pins key ranges to specific shards and regions.

Q: What's the first step when designing a multi-region data architecture?
- Choose a database that supports multi-region
+ Classify data by consistency, locality and regulatory needs, then choose a strategy per class
- Replicate everything everywhere
- Pick the cheapest regions
? Different data needs different trade-offs. One strategy for everything is either too expensive or incorrect.
```
