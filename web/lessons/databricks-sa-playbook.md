## What Databricks SAs do

Databricks Solutions Architects (often titled SA, Senior SA, or Specialist SA in areas like GenAI, Streaming, Data Warehousing or Platform) work with account executives to drive **consumption** and new use cases on the Data Intelligence Platform. The work spans discovery, architecture design, hands-on POCs and "technical wins", migration planning (Hadoop/EMR, legacy ETL, warehouses), enablement workshops and executive presentations.

> **Key idea:** Databricks revenue is consumption-based, so SAs are measured on **use cases that go to production and grow**, not just signed contracts. Talk about "landing use cases and driving adoption".

## What they look for

1. **Hands-on data engineering**: Spark (PySpark and SQL), Delta Lake, streaming, pipelines. You'll likely code live.
2. **Lakehouse architecture**: medallion, governance (Unity Catalog), performance tuning.
3. **Data warehousing**: SQL, dimensional modelling, BI integration, migration from Snowflake, Redshift or Teradata.
4. **ML and GenAI**: MLflow, feature engineering, Model Serving, Vector Search, agents and RAG evaluation.
5. **Cloud depth**: networking, security and IAM on AWS, Azure or GCP. **Your AWS background is a strong asset here.**
6. **Communication**: explaining complex trade-offs to both engineers and executives.

## The core pitch (2 minutes)

> "Most enterprises have data scattered across warehouses, lakes and dozens of tools: one copy for BI, another for data science, a third for streaming, each with different security. That means duplicated cost, inconsistent numbers and AI projects that stall on data access. Databricks unifies it: your data stays in **your cloud storage in open formats** (Delta Lake and Iceberg), governed once by **Unity Catalog**, and every workload, from ETL and streaming to SQL analytics, ML and GenAI agents, runs on that single copy. Serverless compute and Photon give strong price-performance, and you avoid lock-in because the data is open."

## Discovery questions

- What are the top 3 data and AI initiatives this year? Which have executive sponsorship?
- Where does data live today (warehouse, lake, Hadoop, SaaS)? How many copies?
- How long does it take an analyst to get access to a new dataset? How is access governed?
- How fresh does data need to be (daily, hourly, real time)?
- What's blocking ML or GenAI from reaching production?
- What's the current spend on the warehouse, EMR or ETL tools? What renewal dates are coming up?
- Who are the personas (data engineers, analysts, data scientists) and their skills (SQL, Python)?

## Objection handling

| Objection | Response approach |
|---|---|
| "Snowflake is simpler for SQL." | Acknowledge their SQL ease. Show Databricks SQL serverless and AI/BI Genie for analysts. Then the broader value: one platform for engineering, streaming, ML and GenAI on open formats with no proprietary storage lock-in. Offer a **price-performance benchmark on their queries**. |
| "We're all-in on AWS native." | Databricks runs on AWS, is sold via AWS Marketplace (counts towards AWS commits) and integrates with S3, IAM, Glue and Kinesis. Compare the integrated experience and unified governance vs stitching Glue, EMR, Redshift, SageMaker and Lake Formation. |
| "Spark is complex." | Serverless compute (no clusters to tune), SQL-first options, declarative pipelines, the Assistant for code generation. Many users never touch Spark configuration. |
| "Cost is unpredictable." | Serverless with budgets and policies, system tables for cost observability, tagging, committed-use discounts. |
| "Open source Spark plus Delta is free; why pay?" | Photon performance, serverless operations, Unity Catalog governance, Mosaic AI, security and compliance, support, and the TCO of building it yourself. |
| "We use Iceberg." | Unity Catalog supports Iceberg (managed Iceberg tables, Iceberg REST catalog), and UniForm gives interoperability. Databricks acquired Tabular, the company founded by Iceberg's creators. |

## Demo / POC ideas

1. **End-to-end lakehouse in 20 minutes**: Auto Loader ingest → declarative pipeline (bronze/silver/gold with expectations) → AI/BI dashboard → Genie natural-language questions → lineage in Unity Catalog.
2. **Warehouse migration POC**: run 20 of the customer's top queries on a serverless SQL warehouse and compare cost and latency.
3. **GenAI agent on enterprise docs**: Vector Search synced from Delta, an agent with tools (SQL plus retrieval), MLflow evaluation with judges, deployed to Model Serving.
4. **Streaming**: Kafka → Structured Streaming → Delta → a live dashboard updating every minute.
5. **Governance**: row filters and column masks, ABAC tags, audit via system tables, Delta Sharing to a partner.

## Technical prep checklist

- [ ] Lakehouse track complete, plus Query processing, Partitioning and the Real-time analytics design
- [ ] Comfortable writing PySpark and SQL live: joins, windows, aggregations, MERGE, SCD2
- [ ] Can read the Spark UI and diagnose skew, spill and shuffle-heavy plans
- [ ] Built a declarative pipeline with expectations and a streaming table
- [ ] Unity Catalog: catalogs, schemas, grants, volumes, lineage, row filters and masks
- [ ] Mosaic AI: Vector Search, Model Serving, MLflow tracing and evaluation, AI Functions
- [ ] Databricks Certified Data Engineer Associate (Professional is a bonus); GenAI Engineer Associate for AI-focused roles
- [ ] Know the pricing model (DBUs, serverless vs classic) and how to estimate costs
- [ ] Competitive one-pagers: Snowflake, AWS native, Microsoft Fabric, BigQuery

## Sample interview questions

1. "Walk me through designing a lakehouse for a retailer with 200 stores, POS data, e-commerce clickstream and a demand forecasting goal."
2. "A Spark job that used to take 20 minutes now takes 3 hours. How do you troubleshoot?"
3. "Explain how Delta Lake guarantees ACID on S3."
4. "How would you migrate 500 Redshift tables and 2,000 queries to Databricks?"
5. "A customer's RAG app gives wrong answers. How do you improve and measure it?"
6. "Present Databricks to a CFO in 5 minutes." (Business value: cost consolidation, time to insight, AI ROI, risk reduction.)

> **SA angle:** Databricks interviews often include a **live coding or take-home presentation**. Rehearse a 30-minute session: 5 minutes discovery recap, 15 minutes architecture and demo, 10 minutes Q&A. Use Databricks Free Edition to build the demo and keep notebooks clean and narrated, as if a customer will rerun them.

```quiz
Q: Why do Databricks SAs focus on "use cases in production" rather than just closing contracts?
+ Revenue is consumption-based; value and revenue grow only when workloads run and expand on the platform
- Contracts don't matter at Databricks
- Production use cases are free
- It's a legal requirement
? Consumption models reward adoption, so SAs drive use cases from POC to production to expansion.

Q: A customer is all-in on AWS. What's a strong Databricks position?
- Ask them to move to Azure
+ Databricks runs natively on AWS, is purchasable through AWS Marketplace towards their commits, and offers unified governance and an integrated platform vs stitching many services
- Say AWS services are obsolete
- Avoid mentioning AWS
? Partner-friendly positioning respects the customer's strategy while highlighting integration and governance value.
```
