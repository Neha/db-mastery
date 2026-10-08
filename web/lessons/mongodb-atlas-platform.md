## From database to developer data platform

MongoDB the company makes most of its revenue from **Atlas**, its fully managed multi-cloud service (AWS, Azure, Google Cloud). As an SA you'll mostly sell and architect Atlas, with Enterprise Advanced for self-managed and on-prem customers.

> **Key idea:** MongoDB's pitch is "**one platform, one API**" for operational data: transactions, search, vectors, time series, streaming and analytics without bolting on Elasticsearch, Pinecone, Kafka consumers and sync jobs.

## The Atlas building blocks

| Capability | What it does | Replaces |
|---|---|---|
| **Atlas clusters** | Replica sets or sharded clusters, auto-scaling, backups with PITR, multi-region and multi-cloud | Self-managed MongoDB, DynamoDB/DocumentDB |
| **Atlas Search** | Lucene-based full-text search (`$search`): fuzzy, autocomplete, facets, synonyms, relevance scoring; kept in sync automatically | Elasticsearch/OpenSearch plus a sync pipeline |
| **Atlas Vector Search** | ANN search over embeddings (`$vectorSearch`, HNSW) with pre-filters, combinable with full-text (hybrid search) | Pinecone, Weaviate, a separate vector DB |
| **Voyage AI** (acquired 2025) | Embedding and reranking models integrated with Atlas | Third-party embedding APIs |
| **Search Nodes** | Dedicated infrastructure for search and vector workloads, scaled independently | — |
| **Atlas Stream Processing** | Continuous processing of Kafka/change streams using aggregation syntax | Custom Kafka consumers, Flink for simple jobs |
| **Time series collections** | Bucketed, compressed storage for measurements | InfluxDB, Timestream |
| **Online Archive / Data Federation** | Tier cold data to object storage; query across Atlas and S3 with MQL | Manual archiving, Athena |
| **Atlas SQL / BI Connector, Charts** | SQL access for BI tools, native dashboards | ETL to a warehouse for simple reporting |
| **Triggers and Functions** | Serverless reactions to database changes | Lambda plus change-stream plumbing |
| **Queryable Encryption** | Query encrypted fields where the server never sees plaintext (equality and range) | Application-level encryption that kills querying |
| **Relational Migrator** | Converts relational schemas and data to MongoDB designs | Hand-written migration scripts |

## Atlas Search example

```js
db.products.aggregate([
  { $search: {
      index: "default",
      compound: {
        must:   [{ text: { query: "wireless keybaord", path: ["name", "description"], fuzzy: { maxEdits: 1 } } }],
        filter: [{ range: { path: "price", lte: 15000 } }]
      }
  }},
  { $limit: 10 },
  { $project: { name: 1, price: 1, score: { $meta: "searchScore" } } }
])
```

Note the typo "keybaord", which fuzzy matching handles. The search index is maintained asynchronously from the change stream, so there's no dual-write logic in the app.

## Vector Search example (RAG retrieval)

```js
db.docs.aggregate([
  { $vectorSearch: {
      index: "vector_index",
      path: "embedding",
      queryVector: [/* 1024 floats from your embedding model */],
      numCandidates: 200,
      limit: 5,
      filter: { tenantId: "acme", lang: "en" }      // pre-filter on indexed metadata
  }},
  { $project: { text: 1, source: 1, score: { $meta: "vectorSearchScore" } } }
])
```

The key selling point: **vectors live next to the operational data and metadata**. Filtering by tenant, permissions or freshness is just part of the query, with no syncing between a vector DB and the source of truth.

## Deployment options and pricing model

- **Atlas tiers**: Free (M0), Flex (low-cost, usage-based dev and small apps), Dedicated (M10+). Pricing is per hour per node size plus storage, backup and data transfer.
- **Multi-cloud clusters**: nodes across AWS, Azure and GCP in one replica set, for resilience and avoiding lock-in.
- **Enterprise Advanced (EA)**: self-managed with Ops Manager, the Kubernetes Operator, and advanced security. Relevant for regulated industries and on-prem.
- **Community Edition**: free, source-available under the SSPL license.

## Competitive positioning cheat sheet

| Competitor | Their pitch | MongoDB counter |
|---|---|---|
| **Amazon DynamoDB** | Serverless, infinite scale, AWS-native | Rich queries, secondary indexes without GSI gymnastics, aggregation, search and vector built in, multi-cloud, no 400 KB item limit (16 MB docs) |
| **Amazon DocumentDB** | "MongoDB-compatible" | Partial API compatibility, lags versions; no Atlas Search/Vector parity, no multi-cloud, different engine underneath |
| **Azure Cosmos DB** | Multi-model, global distribution | Simpler, predictable pricing vs RU model; true MongoDB, not emulation; richer aggregation |
| **PostgreSQL (+JSONB)** | Free, relational plus JSON | Native sharding, flexible schema at scale, developer velocity, integrated platform |
| **Elasticsearch** | Best-in-class search | Atlas Search removes the sync pipeline and a whole second cluster for most app search use cases |
| **Pinecone / standalone vector DBs** | Purpose-built vectors | Vectors alongside operational data, a single security model, no sync, hybrid search |

> **SA angle:** You come from AWS, which is an asset. MongoDB partners closely with AWS (Atlas is sold through AWS Marketplace, and there are Bedrock and Amazon Q integrations). Frame your background as: *"I know where DynamoDB and DocumentDB fit, and where they don't. I can help AWS customers adopt Atlas on AWS and burn down their EDP commitments through Marketplace."* That's a very attractive message to MongoDB hiring managers.

> **Hands-on:** Create a free Atlas M0 cluster, load the **sample_mflix** dataset, create an Atlas Search index on `movies` with autocomplete, and a Vector Search index on `embedded_movies.plot_embedding`. Build a tiny Go endpoint that serves search-as-you-type results. This is a ready-made interview demo.

```quiz
Q: What is the main architectural advantage of Atlas Vector Search over a standalone vector database for RAG?
- It uses a newer ANN algorithm
+ Vectors live alongside operational data and metadata, so filtering, security and freshness need no separate sync pipeline
- It's free at all scales
- It doesn't require embeddings
? Fewer moving parts: one platform, one security model, one query API, and no dual writes.

Q: A customer runs DocumentDB and wants Atlas Search-like features. What's an accurate positioning statement?
+ DocumentDB emulates parts of the MongoDB API on a different engine; Atlas is MongoDB itself with integrated search, vector search and multi-cloud
- DocumentDB and Atlas are identical
- DocumentDB is MongoDB's managed service on AWS
- Atlas only runs on Azure
? DocumentDB is an AWS-built compatibility layer. Feature and version parity gaps are common migration triggers.

Q: Which Atlas feature lets you query encrypted fields without the server seeing plaintext?
- Client-side field level encryption only
+ Queryable Encryption
- TLS
- Encrypted storage engine
? Queryable Encryption supports equality and range queries over data that stays encrypted on the server.
```
