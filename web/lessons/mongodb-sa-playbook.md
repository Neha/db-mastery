## What MongoDB SAs do

MongoDB Solutions Architects (pre-sales) partner with account executives to win new workloads on **Atlas** (and Enterprise Advanced). Day to day: discovery calls, schema design sessions, architecture reviews, demos, POCs or "technical validations", competitive positioning (DynamoDB, DocumentDB, Cosmos DB, Postgres) and enabling partners (AWS, GCP, Azure, SIs).

## What they look for

1. **Document model fluency**: schema design patterns, embed vs reference, anti-patterns.
2. **Distributed systems**: replica sets, sharding, shard keys, read and write concerns.
3. **Developer empathy**: can you code a demo, talk to developers and show velocity?
4. **Platform breadth**: Atlas Search, Vector Search, Stream Processing, time series, Queryable Encryption.
5. **Business acumen**: tying technical value to outcomes (time to market, TCO, risk).
6. **Competitive knowledge**: honest, credible comparisons.

## The core pitch (2 minutes)

> "Most teams spend more time fighting their data layer than building features: ORMs and migrations for every change, a separate search engine kept in sync, a vector database bolted on for AI, and a cache in front of it all. MongoDB's document model maps directly to how developers think in code, so teams ship faster. Atlas then gives you transactions, full-text search, vector search, stream processing and time series in **one platform with one API and one security model**, on any cloud, with built-in high availability, global distribution and enterprise security. You get fewer moving parts, faster development and a foundation for AI applications."

## Discovery questions for a MongoDB opportunity

- What application or initiative is this for? What's the business deadline?
- What's slowing your developers down today? How long does a schema change take?
- How many data stores does this app touch (DB, cache, search, vector)? Who maintains the sync jobs?
- Peak throughput, data size and growth? Global users? Data residency?
- What happened in your last outage or performance incident?
- Who decides, and how (architecture board, a developer-led POC)?
- What's driving change: cost, scale, AI, modernisation off Oracle or SQL Server?

## Objection handling

| Objection | Response approach |
|---|---|
| "MongoDB isn't ACID." | Multi-document ACID transactions since 4.0 (2018), across shards since 4.2. Single-document ops are atomic. Default majority write concern. Show the transactions demo. |
| "We'll just use Postgres JSONB." | Acknowledge Postgres is great. Then cover: JSONB is document storage *inside* a relational engine, with no native horizontal sharding, partial updates rewriting the whole value, and limited indexing over nested arrays. Atlas adds integrated search and vectors plus a managed global platform. Ask: "what happens at 10x?" |
| "DynamoDB is cheaper and serverless." | Rich ad-hoc queries without designing every GSI up front, aggregation, a 16 MB vs 400 KB item limit, integrated search and vectors, multi-cloud. Compare total cost including the extra services (OpenSearch, ETL) and developer time. |
| "DocumentDB is MongoDB-compatible." | It emulates part of the API on a different engine. Check feature and version gaps (e.g. aggregation stages, change streams nuances, Atlas Search/Vector, multi-cloud). Run their test suite on both. |
| "Schemaless means messy data." | Flexible ≠ schemaless: `$jsonSchema` validation, the schema versioning pattern, Relational Migrator. |
| "Licensing (SSPL) concerns." | Atlas is a managed service, so licensing doesn't affect customers building apps. Enterprise Advanced is commercially licensed. Stay factual. |
| "Vendor lock-in." | Runs on AWS, Azure and GCP (even multi-cloud in one cluster), on-prem with EA, and on Kubernetes. Standard drivers in every language. |

## Demo ideas (pick one and polish it)

1. **"From relational to document in 10 minutes"**: take a 5-table schema, use Relational Migrator or a hand-designed model, show the same API endpoint with one query instead of four joins, and measure latency.
2. **Search-as-you-type**: Atlas Search autocomplete plus fuzzy matching plus facets on `sample_mflix`, with a React front-end (your superpower).
3. **RAG chatbot**: Voyage or OpenAI embeddings plus `$vectorSearch` with tenant filters plus an LLM, with change streams re-embedding updated documents.
4. **Resilience**: trigger a failover in Atlas ("Test Failover") while a Go app writes continuously, and show retryable writes surviving with zero errors.
5. **Performance Advisor**: run a slow query, accept the suggested index, show the explain plan before and after.

## Technical prep checklist

- [ ] All lessons in the MongoDB track plus Replication, Partitioning and CAP
- [ ] Can design schemas for 5 domains (e-commerce, IoT, social, gaming, healthcare) using the patterns
- [ ] Can choose and defend a shard key in 2 minutes
- [ ] Can explain WiredTiger cache, MVCC and the oplog
- [ ] Can write aggregation pipelines from memory (`$group`, `$lookup`, `$unwind`, `$setWindowFields`)
- [ ] Atlas: created clusters, Search and Vector indexes, Triggers, Charts, Online Archive
- [ ] MongoDB Associate Developer certification (Go or another language)
- [ ] Polished demo, rehearsed at 10 and 20 minutes
- [ ] Competitive one-pagers: DynamoDB, DocumentDB, Cosmos DB, Postgres, Couchbase

## Sample interview questions

1. "Design the schema for a multi-tenant SaaS CRM. Pick a shard key."
2. "A customer's queries got slow after 6 months. How do you investigate?"
3. "Explain what happens during a replica set election and how the application experiences it."
4. "When would you recommend *against* MongoDB?" (They want honesty: heavy cross-entity analytics with complex joins, or an existing team deeply invested in relational with no pain.)
5. "Present how MongoDB would help a bank modernise off a mainframe or Oracle."

> **SA angle:** MongoDB values **developer-first** thinking. Your front-end background is a differentiator. Talk about developer experience, iteration speed and how the data shape matches the UI's JSON. Very few SA candidates can build a slick live demo UI, and you can.

```quiz
Q: A prospect says "MongoDB isn't ACID." What's the best response?
+ Correct it factually (multi-document ACID since 4.0, distributed since 4.2, majority writes by default), explain that good modelling makes most writes single-document atomic, and offer a demo
- Agree and say it doesn't matter
- Say MongoDB is faster so ACID isn't needed
- Change the subject to pricing
? Bust the myth with facts and evidence, then connect it to modelling best practice.

Q: When would a credible MongoDB SA recommend against MongoDB?
- Never
+ When the workload is primarily complex ad-hoc relational analytics and the team has no pain with an existing relational system
- When the customer uses JSON
- When the customer needs high availability
? Honesty builds trust. Interviewers test whether you can qualify opportunities rather than force fit.
```
