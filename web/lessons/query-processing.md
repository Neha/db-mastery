## Query processing & optimisation

You write *what* you want; the database decides *how*. The component that decides is the **query optimiser**, arguably the most sophisticated piece of a database. Spark's Catalyst, Postgres's planner and MongoDB's query planner all solve the same problem with different strategies.

## The pipeline

![SQL is parsed, bound and rewritten, then the optimiser picks a physical plan the executor runs.](diagrams/query-processing-pipeline.svg)

## Logical vs physical plans

**Logical**: *what* operations happen (filter, join, aggregate). **Physical**: *which algorithm* runs each one (index scan vs seq scan, hash join vs merge join).

### Rule-based rewrites (always good)

- **Predicate pushdown**: filter as early as possible, even down into the storage layer or Parquet file.
- **Projection pushdown / column pruning**: read only the needed columns. Huge for columnar formats.
- **Constant folding**: `where price > 10 * 100` → `where price > 1000`.
- **Subquery decorrelation**: turn a per-row subquery into a join.

### Cost-based choices (depend on data)

The optimiser estimates the **cost** (I/O + CPU) of candidate plans using **statistics**:

- Row counts and page counts per table.
- Per-column: number of distinct values, null fraction, **most common values**, **histograms**.
- Correlation between physical order and column order.

```sql
select attname, n_distinct, most_common_vals, histogram_bounds
from pg_stats where tablename = 'orders';
```

**Bad statistics → bad cardinality estimates → bad plans.** This is the root cause of a huge share of "it was fast yesterday" incidents.

## Access paths

| Path | When the optimiser picks it |
|---|---|
| Sequential / full scan | Low selectivity, small tables, or no usable index |
| Index scan | High selectivity; fetches rows from the table in index order |
| Index-only scan | All needed columns are in the index (covering) |
| Bitmap index scan (Postgres) | Medium selectivity; collect matching row locations, then read pages in physical order |

## Join algorithms — know all three

```text
Nested loop:  for each row r in outer: find matches in inner (ideally via index)
              ✓ small outer + indexed inner   ✗ big × big = O(n·m)

Hash join:    build a hash table on the smaller input, probe with the larger
              ✓ big unsorted inputs, equality joins   ✗ needs memory (spills to disk)

Merge join:   both inputs sorted on the join key → walk them together
              ✓ already-sorted inputs, range joins    ✗ sorting cost if not sorted
```

**Join order** matters enormously. Joining 5 tables has 5! = 120 orders (and far more tree shapes). Optimisers use dynamic programming for small joins and heuristics or genetic search for large ones (Postgres `geqo`).

### Distributed joins (Spark / Databricks)

| Strategy | When |
|---|---|
| **Broadcast hash join** | One side is small (default ≤ 10 MB, `spark.sql.autoBroadcastJoinThreshold`): copy it to every executor, so there's no shuffle |
| **Shuffle hash join / sort-merge join** | Both sides big: repartition both by join key across the cluster (expensive network shuffle) |

**Adaptive Query Execution (AQE)** in Spark re-optimises *at runtime* using real statistics from completed stages. It switches to broadcast joins, coalesces small partitions and splits skewed ones.

## How MongoDB plans queries

MongoDB uses an **empirical** planner rather than a full cost model:

1. Generate candidate plans from the available indexes.
2. **Race them** for a short trial period.
3. Pick the winner and **cache** it for that *query shape*.
4. Re-plan if performance degrades or indexes change.

Inspect it with `db.orders.find({…}).explain("executionStats")`. Look at `totalKeysExamined` and `totalDocsExamined` vs `nReturned`; ideally all three are close. A `COLLSCAN` stage means no index was used, and `SORT` means an in-memory sort (consider an index that provides the order).

## Execution models

- **Volcano / iterator model**: each operator implements `next()` and pulls one row at a time. Simple, but with high per-row overhead.
- **Vectorised execution**: operators process **batches of column values**, which is CPU-cache- and SIMD-friendly. Used by DuckDB, ClickHouse and **Databricks Photon**.
- **Code generation**: compile the query into tight machine code. Spark's *whole-stage codegen*, HyPer/Umbra.

> **SA angle:** When Databricks says *Photon* makes SQL faster, the technical story is "a C++ vectorised engine replacing JVM row-at-a-time processing for supported operators". When MongoDB talks about the *slot-based execution engine (SBE)*, it's a similar move away from a pure document-at-a-time model. Being able to explain *why* this is faster, in one sentence, is SA gold.

> **Interview:** A query was fast yesterday and is a sequential scan today. The SQL did not change. Where do you look first?

> **Model answer:** Statistics and the plan, not the SQL text. Stale `ANALYZE`, a stats target that missed new skew, or a parameter that flipped selectivity can make the optimiser pick a sequential scan. In Postgres compare `rows` estimated versus actual in `EXPLAIN ANALYZE`; in MongoDB compare keys and docs examined versus `nReturned`. Fix the stats or the index the new data needs — do not add a hint as the first move.

```quiz
Q: What is predicate pushdown?
+ Applying filters as early as possible — ideally in the storage/scan layer
- Moving filters to the end of the plan
- Converting WHERE into HAVING
? Less data flows through the rest of the plan; with Parquet/Delta, entire files and row groups can be skipped.

Q: Which join algorithm is best for two very large, unsorted inputs joined on equality?
- Nested loop without index
+ Hash join
- Cross join
? Build a hash table on the smaller side and probe it with the larger: linear time if memory allows.

Q: In Spark, when is a broadcast hash join chosen?
+ When one side is small enough to copy to every executor, avoiding a shuffle of the big side
- When both sides are huge
- Only for streaming queries
? Broadcasting a small dimension table is the classic way to avoid an expensive shuffle.

Q: How does MongoDB's query planner choose between candidate plans?
- A full statistics-based cost model
+ It trial-runs candidate plans, picks the most productive, and caches the winner per query shape
- It always uses the _id index
? The empirical "race" approach is cached in the plan cache and re-evaluated when needed.

Q: Why is vectorised execution faster than the Volcano iterator model?
+ It processes batches of column values per call, cutting per-row overhead and using CPU caches/SIMD well
- It skips reading data
- It uses more threads
? Fewer virtual calls and tight loops over contiguous arrays are the main wins.
```
