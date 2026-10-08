## Why analytics needs different storage

An analytical query like *"average order value by country by month over 5 years"* touches 3 columns of a 40-column table, but **billions of rows**. A row store reads all 40 columns of every row: 93% wasted I/O.

## Column-oriented storage

![A row store keeps a whole record together; a column store keeps each field together across rows.](diagrams/oltp-olap-columnar-row-vs-column.svg)

Benefits:

1. **Read only the needed columns** (projection pushdown).
2. **Compression** is excellent because similar values sit together:
   - **Dictionary encoding**: `GB, GB, US` becomes `0, 0, 1` plus the dictionary `{GB:0, US:1}`
   - **Run-length encoding**: `GB×1,000,000`
   - **Bit-packing**, delta encoding for sorted timestamps
   - Typical compression ratios are 5–10x.
3. **Vectorised processing**: tight loops over arrays of one type, which suits SIMD and CPU caches.
4. **Data skipping**: per-file and per-block **min/max statistics** let engines skip blocks that can't match (`WHERE month = '2026-03'`).

The trade-off: updating one row means touching many column files, so column stores favour **batch, append-mostly** writes.

## Apache Parquet: the lingua franca

Parquet is the open columnar file format used by Spark, Databricks, Snowflake (external tables), BigQuery, Athena, DuckDB and pandas.

![A Parquet file is row groups of column chunks plus a footer of min/max stats the reader consults first.](diagrams/oltp-olap-columnar-parquet-anatomy.svg)

Readers read the footer first, skip row groups using statistics, and fetch only the required column chunks. On object storage that means a few targeted byte-range GETs instead of downloading everything.

**Parquet alone is just files**: no transactions, no updates, no schema enforcement. That gap is why table formats (**Delta Lake**, Apache Iceberg, Apache Hudi) exist.

## Data warehouses and dimensional modelling

The classic analytics model is Kimball's **star schema**:

![A star schema: a narrow fact table of measures joined to wide dimension tables.](diagrams/oltp-olap-columnar-star-schema.svg)

- **Fact tables**: measurable events (sales, clicks). Many rows, narrow, mostly foreign keys plus numeric measures.
- **Dimension tables**: descriptive context (who, what, where, when). Fewer rows, wide.
- **Snowflake schema**: dimensions normalised further (product → category). Fewer duplicates, more joins.
- **Grain**: what one fact row represents ("one line item per order"). Always define the grain first.

### Slowly changing dimensions (SCD)

What happens when a customer moves from London to Manchester?

| Type | Behaviour |
|---|---|
| SCD 1 | Overwrite (history lost) |
| **SCD 2** | Add a new row with `valid_from`, `valid_to` and `is_current`, so history is preserved |
| SCD 3 | Keep a "previous value" column |

SCD Type 2 with `MERGE` is a **favourite Databricks interview and whiteboard topic**.

## OLAP engine landscape

| Engine | Architecture | Notes |
|---|---|---|
| **Databricks SQL** (Photon) | Lakehouse: compute over open Delta/Iceberg tables in your cloud storage | Serverless SQL warehouses, AI/BI dashboards |
| Snowflake | Cloud warehouse, proprietary storage (plus Iceberg tables) | Separation of storage and compute |
| Amazon Redshift | MPP warehouse (RA3 managed storage, Serverless) | You'll know this from AWS |
| Google BigQuery | Serverless, Dremel-based | Pay per scan or slots |
| ClickHouse, Apache Druid, Apache Pinot | Real-time OLAP | Sub-second on fresh data |
| DuckDB | In-process OLAP ("SQLite for analytics") | Great for laptops and the edge |

## The modern data stack in one picture

```text
Sources (OLTP DBs, SaaS, events, files)
   │  ingest (CDC, batch, streaming)
   ▼
Object storage (S3/ADLS/GCS) with open table format (Delta/Iceberg)   ← single copy of data
   │  transform (Spark, SQL, dbt, declarative pipelines)
   ▼
Consumption: BI dashboards · SQL · data science · ML/GenAI · reverse ETL to apps
   + governance layer (catalog, lineage, access control) across all of it
```

> **SA angle:** Your AWS experience maps directly: S3 + Glue + Athena/EMR + Redshift + Lake Formation is AWS's version of this picture. In a Databricks interview, explain the pain: *several copies of data, governance split across services, BI and ML on different stacks*. Then show how the lakehouse collapses it into one open copy with one governance layer (Unity Catalog). Don't trash AWS; Databricks runs on AWS and co-sells with it.

> **Interview:** The CFO asks why the dashboard cannot just run on production Postgres. When do you keep OLTP and the lakehouse separate?

> **Model answer:** When the query is a multi-year scan of a few columns. A row store reads every column of every row and contends with checkout. CDC or a nightly extract into Parquet/Delta isolates analytics I/O and compression. Keep a small keyed operational report on OLTP if it is bounded; do not put a five-year `GROUP BY` on the primary.

```quiz
Q: Why does columnar storage compress so much better than row storage?
+ Values of the same column (same type, often repetitive) are stored contiguously, enabling dictionary, RLE and delta encoding
- Columnar formats discard data
- Row stores can't be compressed
- It uses a faster CPU
? Homogeneous, repetitive data compresses dramatically. Ratios of 5–10x are common.

Q: What does Parquet lack on its own that Delta Lake adds?
- Columnar layout
- Compression
+ ACID transactions, updates/deletes, schema enforcement and time travel over a set of files
- Min/max statistics
? Parquet is a file format. Delta Lake adds a transaction log that turns a directory of Parquet files into a reliable table.

Q: A customer wants to analyse sales by the customer's region at the time of purchase, even after customers move. Which technique?
- SCD Type 1
+ SCD Type 2
- Dictionary encoding
- Hash partitioning
? SCD2 keeps a versioned row per change, with validity dates, so facts join to the historically correct region.

Q: In a star schema, what is the "grain" of a fact table?
- The compression codec
+ The precise meaning of one row (e.g. one line item per order)
- The number of dimensions
- The partition column
? Defining the grain first prevents double counting and mismatched aggregations.
```
