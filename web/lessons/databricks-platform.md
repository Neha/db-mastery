## The big picture

Databricks positions itself as the **Data Intelligence Platform**: a lakehouse (open data, unified governance) plus AI that understands your data. It runs on AWS, Azure and Google Cloud, with compute either in the customer's cloud account (classic) or Databricks-managed (**serverless**).

```text
┌────────────────────────── Databricks Data Intelligence Platform ──────────────────────────┐
│  Databricks SQL / AI/BI   │  Lakeflow (ingest, pipelines, jobs)  │  Mosaic AI (ML, GenAI, agents) │
│  Genie (NL → SQL)         │  Structured Streaming                │  Model Serving, Vector Search  │
│  Lakebase (Postgres OLTP) │  Databricks Apps                     │  MLflow, Feature Store         │
├────────────────────────────────── Unity Catalog ───────────────────────────────────────────┤
│   governance · lineage · access control · discovery · audit · sharing (Delta Sharing)       │
├──────────────────────────────────── Open storage ──────────────────────────────────────────┤
│        Delta Lake / Iceberg tables + files (Volumes) in your S3 / ADLS / GCS               │
└────────────────────────────────────────────────────────────────────────────────────────────┘
```

## Unity Catalog: the centre of everything

Unity Catalog (UC) is a unified governance layer for **all data and AI assets**: tables, views, volumes (files), ML models, functions and vector indexes.

- **Three-level namespace**: `catalog.schema.table` (for example `main.shop.orders`).
- **Fine-grained access control**: SQL `GRANT`s, row filters and column masks, and attribute-based access control (ABAC) with tags.
- **Automatic lineage**: column-level lineage across notebooks, jobs, dashboards and models.
- **Audit logs and system tables**: billing, usage, query history and lineage, queryable with SQL.
- **Open**: open-sourced in 2024. Exposes an Iceberg REST catalog API, so external engines can read UC tables.
- **Delta Sharing**: an open protocol to share live data with other organisations (even non-Databricks users) without copying.
- **Lakehouse Federation**: query external databases (Postgres, MySQL, Snowflake, Redshift, BigQuery…) through UC without ingesting.

```sql
GRANT SELECT ON TABLE main.shop.orders TO `analysts`;
CREATE FUNCTION main.sec.mask_email(email STRING) RETURN
  CASE WHEN is_account_group_member('pii_readers') THEN email ELSE '***' END;
ALTER TABLE main.shop.customers ALTER COLUMN email SET MASK main.sec.mask_email;
```

> **Key idea:** Governance is often *the* buying reason for large enterprises. "One place to govern data **and** AI across clouds" is UC's core message. Your AWS Lake Formation knowledge makes for a strong comparison.

## Databricks SQL and AI/BI

- **SQL warehouses** (serverless recommended): Photon-powered compute for BI and SQL, with auto-scaling and intelligent workload management.
- **AI/BI Dashboards** and **Genie** (natural-language Q&A over your data, grounded in UC metadata).
- Connectors for Power BI, Tableau, Looker and dbt.
- Positioned head-to-head against Snowflake, Redshift, BigQuery and Synapse/Fabric.

## Lakebase: OLTP comes to the lakehouse

In 2025 Databricks acquired **Neon** (serverless Postgres) and launched **Lakebase**: a fully managed **PostgreSQL** OLTP database integrated with the lakehouse. Storage and compute are separated, it scales to zero, it supports instant branching, and it syncs with Delta tables.

Use cases include serving features or results to apps with low latency, operational state for AI agents, and reverse ETL without a separate database vendor.

> **SA angle:** Lakebase moves Databricks into **operational database territory**, overlapping with MongoDB, Aurora and others. If you interview at Databricks, know this story. If you interview at MongoDB, know it as a competitive threat. Each company is expanding into the other's space: MongoDB into analytics and AI, Databricks into OLTP.

## Mosaic AI: ML and GenAI

| Capability | Purpose |
|---|---|
| **MLflow** (open source, created by Databricks) | Experiment tracking, model registry (in UC), GenAI tracing and evaluation |
| **Feature engineering / Feature Store** | Governed features with point-in-time correctness and online serving |
| **Model Serving** | Serverless endpoints for custom models and foundation models (open and proprietary) |
| **AI Gateway** | Governance, rate limits, guardrails and usage tracking for LLM endpoints |
| **Vector Search** | Vector indexes that sync automatically from Delta tables |
| **Agent Bricks / Agent Framework** | Build, evaluate and deploy agents and RAG apps on your governed data |
| **AI Functions** | Call LLMs from SQL: `ai_query()`, `ai_classify()`, `ai_extract()` |

```sql
SELECT review_id, ai_classify(review_text, ARRAY('positive','negative','neutral')) AS sentiment
FROM main.shop.reviews;
```

## Compute options

| Compute | Use |
|---|---|
| Serverless notebooks and jobs | Default for most new workloads: instant start, no cluster management |
| Classic all-purpose / job clusters | Customer-VPC requirements, special libraries, GPUs |
| Serverless SQL warehouses | BI and SQL |
| Model Serving | Real-time inference |

**Pricing** is based on **DBUs** (Databricks Units, consumption per second by compute type), plus the cloud infrastructure cost for classic compute. Serverless bundles infrastructure into the DBU rate. Customers often pre-commit spend.

## Competitive landscape

| Competitor | Their angle | Databricks angle |
|---|---|---|
| **Snowflake** | Easy SQL warehouse, strong data sharing | Open formats and no lock-in; unified data engineering, streaming and ML/GenAI on one copy; price-performance |
| **AWS native** (Glue, EMR, Redshift, SageMaker, Lake Formation) | Breadth, integration, EDP spend | One integrated platform and governance instead of stitching 6+ services; multi-cloud. Databricks on AWS also draws down AWS commits via Marketplace |
| **Microsoft Fabric** | Bundled with Microsoft 365 / Power BI | Maturity at scale, openness, multi-cloud, governance across clouds |
| **Google BigQuery** | Serverless warehouse | Open lakehouse, multi-cloud, ML/AI breadth |
| **Cloudera / Hadoop** | On-prem legacy | Modernisation and migration target |

> **SA angle:** Databricks SAs are expected to be hands-on: build a demo in a notebook live, debug a Spark job, write SQL and Python fluently, and explain architecture to a CDO. Typical loop elements: a technical screen, a **take-home or live demo/presentation** (often "present a solution to a customer scenario"), a Spark/SQL deep dive, and behavioural rounds. Earn the **Databricks Certified Data Engineer Associate** (and ideally Professional, or the GenAI Engineer Associate) before applying. It signals commitment.

```quiz
Q: What is Unity Catalog's role in the Databricks platform?
- A Spark execution engine
+ Unified governance for data and AI assets: access control, lineage, audit, discovery and sharing
- A BI dashboard tool
- A file format
? UC governs tables, files, models, functions and vector indexes with one permission model across workspaces and clouds.

Q: What is Lakebase?
- A Delta Lake optimisation command
+ A managed PostgreSQL OLTP database integrated with the Databricks lakehouse (from the Neon acquisition)
- A vector database
- Databricks' name for S3
? Lakebase brings low-latency operational Postgres into the platform, with branching, scale-to-zero and lakehouse sync.

Q: Which Databricks capability lets you call an LLM directly from a SQL query?
+ AI Functions such as ai_query() and ai_classify()
- Delta Sharing
- Auto Loader
- Liquid clustering
? AI Functions bring GenAI into SQL pipelines and dashboards.

Q: What's a credible way to position Databricks against AWS-native analytics services?
- AWS services don't work
+ A single integrated platform with unified governance replaces stitching many services together, while still running on AWS and drawing down AWS commits
- Databricks doesn't run on AWS
- Databricks is always cheaper in every case
? Be credible: acknowledge AWS's breadth, emphasise integration, openness and governance, and highlight the partnership.
```
