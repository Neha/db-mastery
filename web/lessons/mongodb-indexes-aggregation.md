## Indexes in MongoDB

MongoDB indexes are B-trees (in WiredTiger), with the same fundamentals as the Indexes lesson. Every collection has a unique index on `_id`.

| Type | Example | Use |
|---|---|---|
| Single field | `{ email: 1 }` | Equality and range on one field |
| Compound | `{ status: 1, createdAt: -1 }` | Multi-field filters and sorts |
| Multikey | `{ "items.sku": 1 }` | Automatically created when the field is an array, with one entry per element |
| Text | `{ title: "text" }` | Basic full-text (prefer Atlas Search) |
| 2dsphere | `{ location: "2dsphere" }` | Geospatial `$near`, `$geoWithin` |
| Hashed | `{ userId: "hashed" }` | Hashed shard keys |
| Wildcard | `{ "attrs.$**": 1 }` | Unknown or dynamic field names |
| Partial | `{ email: 1 }, { partialFilterExpression: { active: true } }` | Index a hot subset |
| TTL | `{ createdAt: 1 }, { expireAfterSeconds: 86400 }` | Auto-delete sessions and logs |
| Unique | `{ email: 1 }, { unique: true }` | Enforce uniqueness |

## The ESR rule for compound indexes

Order compound index fields as **Equality → Sort → Range**:

```js
// Query: open tickets for a team, newest first, priority >= 3
db.tickets.find({ team: "payments", status: "open", priority: { $gte: 3 } }).sort({ createdAt: -1 })

// ESR index:      E          E          S              R
db.tickets.createIndex({ team: 1, status: 1, createdAt: -1, priority: 1 })
```

Why? Equality fields narrow to a contiguous index range. The sort field is then already in order, so there's **no in-memory sort**. Range filters on trailing fields are applied while walking the index.

> **Remember:** If `explain()` shows a `SORT` stage, the index isn't providing order. Blocking in-memory sorts are a top cause of slow queries.

![Equality fields first collapse the index to a contiguous slice; the sort field can then already be in order; the range is applied while walking that slice.](diagrams/mongodb-indexes-aggregation-esr.svg)

> **Interview:** Why does the ESR rule put equality fields first in a compound index?

> **Model answer:** Equality predicates collapse the index to a contiguous range. The sort field can then already be in order, so MongoDB walks that range without an in-memory `SORT`. A range field first (or a skipped leading equality) scatters matching keys, so you lose both the tight scan and the free sort. Equality, then sort, then range.

## Reading explain()

```js
db.tickets.find({ team: "payments", status: "open" }).sort({ createdAt: -1 }).explain("executionStats")
```

Key fields:

| Field | Healthy |
|---|---|
| `winningPlan.stage` | `IXSCAN` → `FETCH` (not `COLLSCAN`) |
| `nReturned` | Close to `totalDocsExamined` |
| `totalKeysExamined` | Close to `nReturned` |
| No `SORT` stage | Index provides order |
| `executionTimeMillis` | Low |

The ratio `totalDocsExamined / nReturned` is the single best health metric: 1 is perfect, 1000 means a bad index. Atlas **Performance Advisor** and **Query Insights** surface these automatically.

## The aggregation pipeline

Aggregation transforms documents through a sequence of **stages**, like Unix pipes or chained array methods in JavaScript (`.filter().map().reduce()`):

```js
db.orders.aggregate([
  { $match: { status: "paid", createdAt: { $gte: ISODate("2026-01-01") } } },  // filter early, uses indexes
  { $unwind: "$items" },                                                      // one doc per line item
  { $group: {
      _id: "$items.sku",
      units:   { $sum: "$items.qty" },
      revenue: { $sum: { $multiply: ["$items.qty", "$items.unitPrice"] } }
  }},
  { $sort: { revenue: -1 } },
  { $limit: 10 },
  { $lookup: { from: "products", localField: "_id", foreignField: "sku", as: "product" } },
  { $project: { _id: 0, sku: "$_id", units: 1, revenue: 1, name: { $first: "$product.name" } } }
])
```

| Stage | SQL analogue |
|---|---|
| `$match` | `WHERE` |
| `$project` / `$addFields` / `$set` | `SELECT` expressions |
| `$group` | `GROUP BY` |
| `$sort`, `$limit`, `$skip` | `ORDER BY`, `LIMIT`, `OFFSET` |
| `$lookup` | `LEFT OUTER JOIN` |
| `$unwind` | Flatten an array (like a lateral join) |
| `$facet` | Several aggregations in one pass (search facets) |
| `$bucket` / `$bucketAuto` | Histograms |
| `$setWindowFields` | Window functions |
| `$graphLookup` | Recursive CTE |
| `$unionWith` | `UNION ALL` |
| `$merge` / `$out` | `INSERT INTO ... SELECT` (materialised views) |
| `$search`, `$vectorSearch` | Atlas Search and Vector Search (must be the first stage) |

### Pipeline optimisation rules

1. **`$match` and `$sort` early** so they can use indexes. Only the leading stages can use indexes.
2. **`$project` early** to drop fields you don't need, keeping documents small through the pipeline.
3. **`$limit` right after `$sort`** lets the engine keep only the top N (a top-k sort).
4. Stages have a **100 MB memory limit**; `allowDiskUse` spills to disk (the default since 6.0).
5. On sharded clusters, early stages run **on each shard** in parallel and `mongos` (or a merging shard) combines the results.

### Window functions in MongoDB

```js
db.orders.aggregate([
  { $setWindowFields: {
      partitionBy: "$customer.id",
      sortBy: { createdAt: 1 },
      output: {
        orderNo:      { $documentNumber: {} },
        runningSpend: { $sum: "$totals.grand", window: { documents: ["unbounded", "current"] } }
      }
  }}
])
```

### Materialised views with $merge

```js
// Nightly: refresh daily revenue per country into a reporting collection
db.orders.aggregate([
  { $match: { status: "paid" } },
  { $group: { _id: { country: "$shipping.country", day: { $dateTrunc: { date: "$createdAt", unit: "day" } } },
              revenue: { $sum: "$totals.grand" } } },
  { $merge: { into: "daily_revenue", whenMatched: "replace", whenNotMatched: "insert" } }
])
```

## Aggregation from Go

```go
pipeline := mongo.Pipeline{
	{{Key: "$match", Value: bson.D{{Key: "status", Value: "paid"}}}},
	{{Key: "$unwind", Value: "$items"}},
	{{Key: "$group", Value: bson.D{
		{Key: "_id", Value: "$items.sku"},
		{Key: "units", Value: bson.D{{Key: "$sum", Value: "$items.qty"}}},
	}}},
	{{Key: "$sort", Value: bson.D{{Key: "units", Value: -1}}}},
	{{Key: "$limit", Value: 5}},
}
cur, err := orders.Aggregate(ctx, pipeline)
var results []bson.M
err = cur.All(ctx, &results)
```

Use `bson.D` (an ordered slice) for pipelines and sort specs because **field order matters**. Use `bson.M` (a map) only where order doesn't matter.

> **SA angle:** A great MongoDB demo is taking a customer's slow SQL report and rewriting it as an aggregation pipeline with the right compound index, showing `explain()` before and after. Another is **operational analytics**: aggregation directly on live data (on an analytics-node secondary) without ETL to a warehouse.

> **Lab:** `go run ./labs/mongodb agg` seeds 20k orders and runs top products, revenue by country and month, and a `$setWindowFields` query. `go run ./labs/mongodb index` shows explain stats before and after an ESR index.

```quiz
Q: Following the ESR rule, what index best serves find({status:"open", region:"EU", amount:{$gt:100}}).sort({createdAt:-1})?
- { amount: 1, status: 1, region: 1, createdAt: -1 }
+ { status: 1, region: 1, createdAt: -1, amount: 1 }
- { createdAt: -1, amount: 1 }
- { amount: 1 }
? Equality fields (status, region) first, then the sort field (createdAt), then the range field (amount).

Q: explain() shows totalDocsExamined = 500000 and nReturned = 20. What does this indicate?
+ The query scans far more documents than it returns; a better (more selective) index is needed
- The query is optimal
- The collection needs sharding immediately
- The documents are too big
? A ratio close to 1 is ideal. 25,000:1 means the index (if any) isn't selective for this query.

Q: Why should $match appear as early as possible in a pipeline?
- $match must always be the last stage
+ Early $match can use indexes and reduces the number of documents flowing through later stages
- $match only works after $group
- It enables $lookup
? Only leading stages can use indexes, and filtering early cuts the work for every later stage.

Q: Which stage would you use to build a materialised view collection from an aggregation?
- $project
- $facet
+ $merge
- $unwind
? $merge (or $out) writes pipeline results into a collection, incrementally updating it with $merge.
```
