## The SA version of system design

Software engineer interviews test whether you can *build* the system. SA interviews test whether you can **lead a customer to the right architecture**: gather requirements, reason about trade-offs, communicate clearly, and connect it to your platform without forcing it.

## The framework (about 45 minutes)

| Step | Time | What you do |
|---|---|---|
| 1. Clarify requirements | 5–8 min | Functional and non-functional; ask, don't assume |
| 2. Estimate scale | 3–5 min | Users, QPS, data size, growth, read/write ratio |
| 3. Define the API and data model | 5–8 min | Entities, access patterns, hot queries |
| 4. High-level architecture | 8–10 min | Boxes and arrows, the data flow end to end |
| 5. Deep dive | 10–15 min | Storage choice, partitioning, consistency, the hardest part |
| 6. Failure modes and operations | 5 min | What breaks, monitoring, DR, security, cost |
| 7. Summarise | 2 min | Recap decisions and trade-offs, and what you'd do next |

![Seven SA design steps: clarify and estimate before any boxes; architecture sits in the middle; you finish on operations and a recap.](diagrams/system-design-framework-steps.svg)

> **Interview:** You jump straight to drawing boxes. What should you have done first, and how do you recover?

> **Model answer:** Stop and go back to requirements. Boxes without scale, consistency and access patterns are decoration. Spend the first five minutes on functional and non-functional needs, then a back-of-the-envelope, then the data model. The interviewer is scoring whether you can lead a customer, not whether you remember a reference architecture.

## Step 1: requirements questions

**Functional:** What are the core use cases? Who are the users? What must happen in real time vs eventually?

**Non-functional (the "-ilities"):**

| Question | Why it matters |
|---|---|
| Latency target (p99)? | Caching, data locality, sync vs async |
| Availability target (99.9 vs 99.99)? | 8.7 hours vs 52 minutes of downtime per year, so multi-AZ vs multi-region |
| Consistency needs per data type? | Strong for money, eventual for counters |
| RPO / RTO? | Backup strategy, replication mode |
| Data residency / compliance (GDPR, HIPAA, PCI)? | Region pinning, encryption, zone sharding |
| Growth over 3 years? | Partitioning strategy from day 1 or later |
| Existing stack, team skills, cloud commitments? | **SA-specific**: the best architecture is one the customer can run |
| Budget? | Managed vs self-managed, tiering |

## Step 2: back-of-the-envelope estimation

Numbers to memorise:

| Fact | Value |
|---|---|
| Seconds per day | ~86,400 ≈ 10⁵ |
| 1M requests/day | ~12 per second average; peak ≈ 2–5x |
| 1 KB × 1B rows | 1 TB |
| RAM / SSD / network read 1 MB sequentially | ~10 µs / ~50 µs / ~1 ms (10 Gbps) |
| Cross-region round trip | 60–150 ms |
| One well-tuned Postgres/MongoDB node | Thousands to tens of thousands of simple ops/sec |

Worked example: *100M users, 10% daily active, each makes 20 reads and 2 writes.*
- Reads: 10M × 20 / 10⁵ ≈ **2,000 req/s average, ~10,000 at peak**
- Writes: 10M × 2 / 10⁵ ≈ **200/s, ~1,000 at peak**
- Storage: 2 writes × 1 KB × 10M/day ≈ 20 GB/day ≈ **7 TB/year** (before indexes and replication ×3)

Conclusion: the read-heavy workload needs caching or read scaling. 7 TB per year suggests sharding within 1–2 years, or tiering old data.

## Step 3: model around access patterns

List the top queries with frequency and latency:

```text
Q1  Get product page by id                    10k/s   p99 < 20ms
Q2  Search products by text + filters          2k/s   p99 < 100ms
Q3  Place order (decrement stock, create order) 200/s  must be correct
Q4  Order history for a customer                500/s  p99 < 50ms
Q5  Daily revenue by category (analytics)       hourly batch
```

Then design storage so that **each hot query hits one partition and one index**.

## Step 4–5: choosing storage, a decision guide

| Need | Strong candidates |
|---|---|
| Flexible entities, rich queries, scale-out OLTP | **MongoDB** |
| Strict relational integrity, complex joins, moderate scale | **PostgreSQL** (Aurora, Lakebase, CloudNativePG on OpenShift) |
| Massive key-value at predictable latency | DynamoDB, Cassandra |
| Full-text relevance | Atlas Search, Elasticsearch/OpenSearch |
| Semantic or vector search | Atlas Vector Search, pgvector, Databricks Vector Search |
| Analytics over large history, BI, ML | **Databricks lakehouse**, Snowflake, BigQuery |
| Real-time event pipeline | Kafka (Streams for Apache Kafka, MSK, Confluent) |
| Caching, rate limiting, sessions | Redis |
| Global strong consistency | Spanner, CockroachDB (or MongoDB global clusters with care) |

> **Tip:** Polyglot persistence is normal, but every extra database adds operational cost and sync pipelines. A strong SA answer explicitly weighs "one platform that does 90%" against "best-of-breed for each". That's literally MongoDB's and Databricks' pitch.

## Deep-dive checklist

- **Partitioning**: key choice, hotspots, rebalancing
- **Replication**: sync vs async, read-your-writes, failover time
- **Consistency**: which data needs what, and the isolation level for transactions
- **Caching**: what, where, TTL, invalidation (CDC or change streams)
- **Async processing**: queues and streams for anything not needed in the request path
- **Idempotency** and retries
- **Schema evolution** and migrations

## Step 6: failure modes and operations

Walk through: node failure, AZ failure, region failure, bad deploy, data corruption or accidental deletion (PITR), traffic spike (auto-scaling, load shedding), noisy neighbour, dependency outage. Then add security (encryption at rest and in transit, least privilege, network isolation, auditing), observability (metrics, logs, traces, slow-query analysis), and cost (right-sizing, tiering, reserved or committed spend).

## Communication tips for SA rounds

1. **Think out loud**, and narrate trade-offs: "I'm choosing X over Y because…".
2. **Draw first, detail later.** Get the end-to-end flow on the board in 10 minutes.
3. **Use the customer's language** (business outcomes), then go deep when asked.
4. **Invite collaboration**: "Does this match what you're seeing?" It's an SA skill, so show it.
5. **Name the risks** before the interviewer does.
6. **Bring in your platform naturally**: "On Atlas, this would be a global cluster with zone sharding…"

```quiz
Q: What's the first thing you should do in a system design interview?
- Draw the architecture
- Pick the database
+ Clarify functional and non-functional requirements
- Estimate the cost
? Requirements drive every later decision. Skipping them is the most common failure.

Q: 50M requests per day is roughly how many requests per second on average?
- 50
+ About 580
- About 5,800
- About 50,000
? 50M / 86,400 ≈ 580/s. Peak may be 2–5x higher.

Q: What distinguishes an SA system design answer from a pure engineering one?
+ Explicitly weighing the customer's constraints (skills, existing stack, compliance, cost) and communicating trade-offs in business terms
- Using more boxes
- Avoiding any specific products
- Focusing only on algorithms
? SAs must land an architecture the customer can adopt and operate. Context and communication matter as much as correctness.
```
