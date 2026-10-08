## What Spark is

**Apache Spark** is a distributed compute engine for large-scale data processing: batch, SQL, streaming and ML, with one API. It was created at UC Berkeley's AMPLab by the founders of **Databricks**. Databricks runs an optimised Spark (the **Databricks Runtime**) plus the **Photon** vectorised engine.

## Architecture

```text
            ┌───────────── Driver ─────────────┐
            │ SparkSession · builds DAG ·       │
            │ schedules tasks · collects results│
            └──────┬───────────┬───────────┬───┘
                   ▼           ▼           ▼
             Executor 1   Executor 2   Executor N     (JVM processes on worker nodes)
             [task][task] [task][task] [task][task]   one task per partition per core
             cache        cache        cache
```

- **Driver**: runs your program, plans the work and coordinates.
- **Executors**: run **tasks** in parallel and cache data.
- **Cluster manager**: allocates resources (Databricks, YARN, Kubernetes, standalone). On Databricks **serverless** compute, you don't manage clusters at all.

## Core abstractions

- **DataFrame / Dataset**: a distributed table with a schema. This is what you use 99% of the time (via Python, Scala, SQL or R).
- **RDD**: the low-level distributed collection underneath. Rarely used directly now.
- **Partition**: a chunk of data processed by one task. Parallelism equals the number of partitions.

```python
df = spark.read.table("main.shop.orders")                     # lazy: nothing runs yet
result = (df.filter("status = 'paid'")
            .groupBy("country")
            .agg({"amount": "sum"})
            .orderBy("sum(amount)", ascending=False))
result.show()                                                  # action: triggers execution
```

The same thing in SQL:

```sql
SELECT country, sum(amount) FROM main.shop.orders
WHERE status = 'paid' GROUP BY country ORDER BY 2 DESC;
```

## Lazy evaluation: transformations vs actions

| Transformations (lazy) | Actions (trigger a job) |
|---|---|
| `select`, `filter`, `withColumn`, `join`, `groupBy`, `union` | `show`, `count`, `collect`, `write`, `save`, `toPandas` |

Spark builds a **logical plan** from transformations and optimises the whole chain only when an action runs. That's how it can push filters down into the file scan and prune unused columns.

## Narrow vs wide transformations, and shuffles

- **Narrow**: each output partition depends on one input partition (`filter`, `select`, `map`). No data movement.
- **Wide**: output partitions need data from many input partitions (`groupBy`, `join`, `distinct`, `orderBy`). This requires a **shuffle**: data is written to local disk, sent over the network and re-partitioned by key.

![Narrow transforms stay inside one partition; a wide transform shuffles every key across the network and starts a new stage.](diagrams/spark-fundamentals-shuffle.svg)

A **job** splits into **stages** at shuffle boundaries, and a stage splits into **tasks**, one per partition.

> **Key idea:** Shuffles are the most expensive part of most Spark jobs: disk, network and serialisation. Spark performance tuning is mostly about **shuffling less data**.

## The Catalyst optimiser and Tungsten

1. Parse → **unresolved logical plan**
2. Resolve against the catalog → **analysed logical plan**
3. Rule-based optimisation (predicate pushdown, constant folding, column pruning) → **optimised logical plan**
4. Cost-based selection of physical strategies (broadcast vs sort-merge join) → **physical plan**
5. **Whole-stage code generation** (Tungsten) → compact JVM bytecode

**Photon** (Databricks) replaces much of the execution layer with a native C++ vectorised engine. It's transparent to users and much faster for SQL and DataFrame workloads.

## Adaptive Query Execution (AQE)

AQE (on by default since Spark 3.2) re-optimises **at runtime** using real shuffle statistics:

- **Coalesces** many small shuffle partitions into fewer larger ones.
- **Switches join strategy** to a broadcast join if one side turns out to be small.
- **Splits skewed partitions** so one giant key doesn't stall the job.

## Common performance problems

| Symptom | Cause | Fix |
|---|---|---|
| One task runs for 40 minutes while others take 30 seconds | **Data skew** on a join or group key | AQE skew join, salting, filter nulls/hot keys |
| Millions of tiny files, slow reads | Over-partitioned writes | `OPTIMIZE`, auto-compaction, liquid clustering |
| Executor OOM | Huge partitions, `collect()` on big data, wide rows | More partitions, avoid `collect`, increase memory |
| Huge shuffle | Joining two big tables | Broadcast the small one, pre-filter, cluster on the join key |
| Slow Python UDFs | Row-at-a-time serialisation to Python | Built-in functions, or vectorised Pandas UDFs |
| Recomputing the same DataFrame | No caching across actions | `cache()` / `persist()` (then unpersist) |

> **Interview:** One task in a join stage runs for 40 minutes; the rest finish in 30 seconds. What happened, and what do you do?

> **Model answer:** Data skew: one join key (or a flood of nulls) landed in a single partition, so one task shuffles and joins almost everything. Confirm in the Spark UI (max vs median task time, shuffle bytes). Then AQE skew join, salt the hot key, filter nulls, or broadcast the small side if it fits.

```python
from pyspark.sql.functions import broadcast
orders.join(broadcast(countries), "country_code")   # hint: ship small table to all executors
```

> **Warning:** `df.collect()` pulls the entire distributed dataset into the driver's memory. On a billion rows it crashes the driver. This is the "Spark equivalent of `SELECT *` without `LIMIT`".

## Structured Streaming (preview)

The same DataFrame API works on unbounded streams. Spark treats a stream as a continuously appended table and runs incremental micro-batches:

```python
(spark.readStream.table("bronze.events")
   .groupBy(window("ts", "5 minutes"), "country").count()
   .writeStream.option("checkpointLocation", "/chk/agg")
   .outputMode("complete").toTable("gold.events_5min"))
```

Covered fully in the pipelines lesson.

## Spark and Go?

Spark is JVM-based, with first-class Python, SQL, Scala and R. **Spark Connect** (Spark 3.4+) introduced a thin client-server protocol (gRPC), and there's an Apache **spark-connect-go** client, so Go programs can build DataFrame queries remotely. For the Databricks SA role, **Python and SQL** are what you'll demo; Go is a nice "I know the architecture" bonus.

> **SA angle:** Databricks SA interviews often include a **Spark UI walkthrough**: "this job is slow, what do you look at?" The answer flow: Jobs → Stages (find the long stage) → Tasks (look at the max vs median duration for skew) → shuffle read/write sizes → SQL tab for the physical plan (sort-merge vs broadcast) → fix. Practise this on Databricks Free Edition with a deliberately skewed join.

```quiz
Q: What triggers actual execution in Spark?
- Calling filter() or select()
+ An action such as count(), show(), collect() or write
- Creating a SparkSession
- Defining a schema
? Transformations build a lazy plan. Actions trigger optimisation and execution.

Q: Which operation causes a shuffle?
- filter
- select
+ groupBy
- withColumn
? groupBy is a wide transformation: rows with the same key must be brought together across partitions.

Q: One task in a stage takes 50x longer than the rest. What's the most likely cause?
+ Data skew: one key or partition holds a disproportionate amount of data
- Too many executors
- The driver is too large
- Photon is disabled
? Skew is the classic straggler cause. AQE skew handling or salting the key helps.

Q: What does Adaptive Query Execution do?
- Caches every DataFrame automatically
+ Re-optimises the plan at runtime using actual shuffle statistics (coalescing partitions, switching to broadcast joins, splitting skew)
- Converts Python UDFs to SQL
- Adds more nodes to the cluster
? AQE corrects planner misestimates using real data sizes observed during execution.
```
