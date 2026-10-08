## ETL vs ELT

- **ETL** (Extract → Transform → Load): transform before loading into the warehouse. The classic approach with on-prem tools.
- **ELT** (Extract → Load → Transform): land raw data first, transform inside the scalable platform. This is the modern default in lakehouses and cloud warehouses.

## The medallion architecture

```text
 Sources ──▶  🥉 BRONZE            ──▶  🥈 SILVER                ──▶  🥇 GOLD
              raw, append-only          cleaned, conformed,          business-level aggregates,
              as-ingested (JSON,        deduplicated, typed,         star schemas, feature tables,
              CDC events), full         joined; "single version      BI-ready, AI-ready
              history, replayable       of truth" entities
```

- **Bronze** keeps the raw truth, so you can always reprocess when logic changes.
- **Silver** enforces quality: schema, dedup, null handling, CDC applied with `MERGE`.
- **Gold** is shaped for consumption, often per business domain (data products).

> **Tip:** Medallion is a *guideline*, not a law. Interviewers like hearing "we might skip gold for data science, or add a platinum serving layer, depending on consumers".

## Batch vs streaming

| | Batch | Streaming |
|---|---|---|
| Latency | Minutes to hours | Seconds to sub-second |
| Data | Bounded | Unbounded |
| Complexity | Lower | Higher (late data, state, exactly-once) |
| Cost | Can be cheaper (run then stop) | Always-on compute (or triggered micro-batches) |

**Lambda architecture**: a batch layer plus a speed layer, giving two code paths (painful). **Kappa architecture**: everything is a stream, and you replay the log for reprocessing. Spark Structured Streaming plus Delta makes "one code path for batch and streaming" practical: the same DataFrame logic runs with `trigger(availableNow=True)` (batch-like, incremental) or continuously.

## Structured Streaming essentials

```python
from pyspark.sql.functions import col, from_json, window

events = (spark.readStream
    .format("kafka")
    .option("kafka.bootstrap.servers", "broker:9092")
    .option("subscribe", "clicks")
    .load()
    .select(from_json(col("value").cast("string"), schema).alias("e")).select("e.*"))

agg = (events
    .withWatermark("ts", "10 minutes")                    # tolerate 10 min late data, bound the state
    .groupBy(window("ts", "1 minute"), "page")
    .count())

(agg.writeStream
    .option("checkpointLocation", "/Volumes/main/chk/clicks_agg")   # offsets + state → exactly-once
    .outputMode("append")
    .toTable("main.gold.clicks_per_minute"))
```

Concepts you must be able to explain:

- **Event time vs processing time**: when it happened vs when we saw it.
- **Watermarks**: how late data can arrive before a window is finalised and its state dropped.

![Events that arrive before the watermark still update the window; once the watermark passes, that window's state is dropped and later events are ignored.](diagrams/data-pipelines-streaming-watermark.svg)

- **Checkpointing**: stores source offsets and operator state, so a restarted query resumes exactly where it left off.
- **Exactly-once**: replayable sources (Kafka offsets, files), checkpointed progress and idempotent or transactional sinks (Delta commits) give end-to-end exactly-once results.
- **Output modes**: `append`, `update`, `complete`.
- **Triggers**: default micro-batch, `processingTime="1 minute"`, `availableNow=True` (process everything available, then stop), and real-time mode for low-latency use cases.

> **Interview:** A candidate says Structured Streaming into Delta is exactly-once. What do you check?

> **Model answer:** Exactly-once is a property of the whole path, not the engine. You need a replayable source (Kafka offsets or files), a checkpoint of offsets and state, and an idempotent or transactional sink (Delta commits). At-least-once sources, disabled checkpoints, or a non-idempotent sink (a webhook, a non-transactional JDBC write) give at-least-once or at-most-once. Say "end-to-end exactly-once" only when all three are true.

## Ingestion on Databricks

| Tool | Use |
|---|---|
| **Auto Loader** (`cloudFiles`) | Incrementally ingest new files from cloud storage, with schema inference and evolution, scaling to millions of files |
| **Lakeflow Connect** | Managed connectors for databases (CDC) and SaaS apps (Salesforce, Workday, etc.) |
| `COPY INTO` | Idempotent SQL batch file loading |
| Kafka / Kinesis / Event Hubs sources | Streaming ingestion |

```python
(spark.readStream.format("cloudFiles")
   .option("cloudFiles.format", "json")
   .option("cloudFiles.schemaLocation", "/Volumes/main/chk/orders_schema")
   .load("s3://landing/orders/")
   .writeStream.option("checkpointLocation", "/Volumes/main/chk/orders_bronze")
   .toTable("main.bronze.orders"))
```

## Declarative pipelines

Instead of hand-wiring streams, checkpoints and retries, **Lakeflow Declarative Pipelines** (formerly Delta Live Tables, DLT, now also open-sourced as **Spark Declarative Pipelines**) let you declare the tables and their dependencies. The engine handles orchestration, incremental processing, retries and data quality:

```sql
CREATE OR REFRESH STREAMING TABLE bronze_orders
AS SELECT * FROM STREAM read_files('s3://landing/orders/', format => 'json');

CREATE OR REFRESH STREAMING TABLE silver_orders (
  CONSTRAINT valid_amount EXPECT (amount > 0) ON VIOLATION DROP ROW,
  CONSTRAINT has_customer EXPECT (customer_id IS NOT NULL) ON VIOLATION FAIL UPDATE
)
AS SELECT * FROM STREAM bronze_orders;

CREATE OR REFRESH MATERIALIZED VIEW gold_daily_revenue
AS SELECT date(ts) AS day, country, sum(amount) AS revenue FROM silver_orders GROUP BY ALL;
```

Applying CDC (including SCD Type 2) becomes declarative with `AUTO CDC` (formerly `APPLY CHANGES INTO`).

## Orchestration and quality

- **Lakeflow Jobs** (formerly Databricks Workflows) orchestrates notebooks, SQL, pipelines, dbt and ML tasks with dependencies, retries and schedules or triggers. Airflow is a common external alternative.
- **Data quality**: expectations (above), Lakehouse Monitoring for drift and anomalies, and unit tests on transformation code.
- **dbt** is widely used for SQL transformations on Databricks.

> **SA angle:** Many Databricks deals start as **migrations**: Hadoop/EMR, legacy ETL (Informatica, DataStage), or warehouse offload. Prepare a migration story: *assess workloads → land data in bronze with Auto Loader or Lakeflow Connect → convert ETL logic to declarative pipelines → validate with reconciliation queries → cut over BI → decommission.* Mention measurable outcomes (cost, SLA, freshness). Your AWS EMR/Glue knowledge is directly relevant here.

```quiz
Q: What is the purpose of the bronze layer in the medallion architecture?
- Store BI-ready aggregates
+ Keep raw, as-ingested data with full history so pipelines can be replayed
- Store only deduplicated records
- Serve ML features in real time
? Bronze preserves the raw source of truth. Silver and gold can always be rebuilt from it.

Q: What does a watermark do in Structured Streaming?
+ Defines how late events may arrive, allowing windows to be finalised and old state dropped
- Encrypts the stream
- Sets the micro-batch interval
- Marks records as processed in Kafka
? Without watermarks, aggregation state grows forever, waiting for potential late data.

Q: Which combination gives end-to-end exactly-once results in Spark Structured Streaming into Delta?
- At-most-once sources and any sink
+ Replayable sources, checkpointed offsets and state, and an idempotent or transactional sink like Delta
- Disabling checkpoints for speed
- Using collect() in each micro-batch
? Replay plus checkpoints plus transactional commits mean each record affects the output exactly once.

Q: What problem does Auto Loader solve?
- Running SQL dashboards
+ Incrementally and scalably ingesting new files from cloud storage with schema inference and evolution
- Training ML models
- Replicating MongoDB
? It tracks which files are new (via listing or notifications) so you don't reprocess or miss files.
```
