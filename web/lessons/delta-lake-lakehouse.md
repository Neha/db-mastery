## Warehouse vs lake vs lakehouse

| | Data warehouse | Data lake | **Lakehouse** |
|---|---|---|---|
| Storage | Proprietary, managed | Open files on object storage | Open files on object storage |
| Data types | Structured | Anything (structured, semi-structured, unstructured) | Anything |
| Transactions | ✅ ACID | ❌ (files can be half-written) | ✅ ACID via table format |
| Performance for BI | ✅ Excellent | ❌ Poor ("data swamp") | ✅ Good to excellent (Photon, clustering, caching) |
| ML / data science | ❌ Data must be exported | ✅ Direct file access | ✅ Direct access to the same tables |
| Cost | $$$ | $ | $ storage + elastic compute |
| Lock-in | High | Low | Low (open formats) |

The **lakehouse** (coined by Databricks, 2020) combines both: **warehouse reliability and performance on open, low-cost lake storage**, with one copy of data serving BI, SQL, streaming, data science and AI. That copy is usually refined in three layers: bronze, silver and gold (the medallion path).

![Sources land in bronze (raw, append-only), are conformed in silver, and are served from gold. Keep bronze so you can replay when the logic changes.](diagrams/delta-lake-lakehouse-medallion.svg)

> **Interview:** Why not query bronze tables forever and skip silver and gold?

> **Model answer:** Bronze is raw and append-only: messy types, duplicates, late CDC, PII, and no stable grain. Every dashboard would re-implement cleaning and would disagree. Silver is the conformed source of truth; gold is shaped for a consumer (star schema, SLAs, row-level security). Keep bronze so you can replay; do not serve it.

## Delta Lake: how it works

A Delta table is a directory of **Parquet data files** plus a **transaction log**:

```text
s3://lake/shop/orders/
├── _delta_log/
│   ├── 00000000000000000000.json    ← commit 0: add files A, B; schema; table properties
│   ├── 00000000000000000001.json    ← commit 1: add file C
│   ├── 00000000000000000002.json    ← commit 2: remove file B, add file D (an UPDATE)
│   └── 00000000000000000010.checkpoint.parquet   ← periodic snapshot of log state
├── part-0000-A.snappy.parquet
├── part-0001-B.snappy.parquet   (logically removed, physically kept until VACUUM)
├── part-0002-C.snappy.parquet
└── part-0003-D.snappy.parquet
```

- **The log is the source of truth.** The table's current state is the set of files that were added and not later removed.
- **Atomic commits**: a writer writes new Parquet files and then atomically creates the next numbered JSON log file. If two writers race for the same number, only one succeeds (**optimistic concurrency control**); the other checks for conflicts and retries.
- **Readers** read the latest checkpoint plus subsequent JSON commits, giving **snapshot isolation**. They never see partial writes.
- **Statistics** (min/max/null counts per column per file) live in the log, enabling **data skipping** without opening files.

> **Key idea:** Delta Lake = Parquet + a write-ahead log. It's the same WAL idea from the internals track, applied to files on object storage. Say that in the interview.

## Core features

```sql
-- ACID DML on a data lake
UPDATE main.shop.orders SET status = 'refunded' WHERE id = 42;
DELETE FROM main.shop.customers WHERE gdpr_erasure_requested = true;

-- Upserts (CDC apply, SCD)
MERGE INTO main.shop.customers t
USING updates s ON t.id = s.id
WHEN MATCHED AND s.op = 'DELETE' THEN DELETE
WHEN MATCHED THEN UPDATE SET *
WHEN NOT MATCHED THEN INSERT *;

-- Time travel: query past versions (audits, reproducing ML training data, undo)
SELECT * FROM main.shop.orders VERSION AS OF 12;
SELECT * FROM main.shop.orders TIMESTAMP AS OF '2026-10-01';
RESTORE TABLE main.shop.orders TO VERSION AS OF 12;

DESCRIBE HISTORY main.shop.orders;   -- who changed what, when
```

- **Schema enforcement**: writes with mismatched schemas are rejected. **Schema evolution** (`mergeSchema`) adds new columns deliberately.
- **Change Data Feed**: read the row-level changes between versions, which feeds downstream incremental pipelines.
- **Deletion vectors**: mark rows deleted without rewriting whole files, making updates and deletes much faster.

## Performance and layout

| Technique | What it does |
|---|---|
| **OPTIMIZE** | Compacts small files into larger ones (about 1 GB), fixing the small-file problem |
| **Z-ORDER BY (cols)** | Co-locates related values across files so data skipping works on several columns (older approach) |
| **Liquid clustering** (`CLUSTER BY`) | Incremental, flexible clustering that replaces partitioning and Z-order for most tables; keys can change without a rewrite |
| **Predictive optimisation** | Databricks runs OPTIMIZE/VACUUM/ANALYZE automatically for Unity Catalog managed tables |
| **VACUUM** | Physically deletes files no longer referenced (after the retention period, default 7 days, which limits time travel) |
| Partitioning | Directory-level pruning; use only for large, low-cardinality columns (date), or skip it in favour of liquid clustering |

```sql
CREATE TABLE main.shop.events (event_id STRING, user_id STRING, ts TIMESTAMP, type STRING)
CLUSTER BY (user_id, ts);
```

## The open table format landscape

| Format | Origin | Notes |
|---|---|---|
| **Delta Lake** | Databricks (Linux Foundation) | Default on Databricks; huge ecosystem |
| **Apache Iceberg** | Netflix | Strong adoption by Snowflake, AWS (S3 Tables, Athena), and others |
| Apache Hudi | Uber | Strong in streaming upserts |

**Databricks acquired Tabular** (founded by Iceberg's creators) in 2024, and supports interoperability via **Delta UniForm**: one copy of data readable as Delta *and* Iceberg. **Unity Catalog** exposes tables through the Iceberg REST catalog API. The message: *format wars are ending; pick an open format and stay interoperable.*

> **SA angle:** A sharp Databricks interview answer to *"Why not just use Snowflake/Redshift?"*: "Your data stays in **your** cloud storage in **open formats** (Delta/Iceberg), so any engine can read it, with no lock-in or egress tax. One copy serves BI, streaming, data science and GenAI, governed once in Unity Catalog. And you pay for compute only when you use it." Then acknowledge the competitor's strengths honestly. Interviewers test for credibility, not zealotry.

> **Hands-on:** In Databricks Free Edition, create a Delta table, run 5 updates, inspect `DESCRIBE HISTORY`, time travel to version 1, and run `OPTIMIZE`. Then look at the `_delta_log` JSON files and read one. It'll click instantly.

```quiz
Q: How does Delta Lake provide atomic commits on object storage?
+ Writers add new Parquet files, then atomically create the next numbered entry in the _delta_log; conflicting concurrent commits are detected and retried
- It locks the S3 bucket during writes
- It uses a separate relational database for all data
- It writes each row to the log
? The transaction log is the source of truth, and optimistic concurrency control handles concurrent writers.

Q: What does VACUUM do on a Delta table?
- Compacts small files
+ Physically deletes data files no longer referenced by the log (older than the retention period), which limits time travel
- Updates statistics
- Converts Delta to Iceberg
? OPTIMIZE compacts. VACUUM removes unreferenced files. After VACUUM you can't time travel to versions needing those files.

Q: Which feature is now recommended over manual partitioning and Z-ordering for most Delta tables?
- Bucketing
+ Liquid clustering
- Hash partitioning
- Secondary indexes
? Liquid clustering is incremental and flexible: clustering keys can change without rewriting the table.

Q: What does Delta UniForm enable?
- Running Delta on MongoDB
+ Reading the same table as Delta, Iceberg (and Hudi) without copying data
- Converting Parquet to CSV
- Uniform pricing across clouds
? UniForm generates the metadata for other formats so engines like Iceberg readers can query one copy of the data.
```
