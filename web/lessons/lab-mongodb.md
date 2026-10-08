## What you'll do

Use the official **MongoDB Go driver v2** against a local single-node replica set to practise document modelling, update operators, aggregation, indexing, transactions and change streams.

```bash
make up
go run ./labs/mongodb           # list commands
go run ./labs/mongodb all
```

| Command | Concept | Source |
|---|---|---|
| `seed` | Embedded document model, `InsertMany` | `labs/mongodb/demos.go` → `seed` |
| `crud` | Dot notation, array queries, atomic update operators | `crud` |
| `agg` | `$unwind`/`$group`, compound keys, `$setWindowFields` | `agg` |
| `index` | `explain("executionStats")`, the ESR rule | `indexDemo` |
| `tx` | `WithTransaction` with automatic retry | `txDemo` |
| `watch` | Change streams with `fullDocument: updateLookup` | `watch` |

## 1. The document model in Go

```go
type Order struct {
	ID          bson.ObjectID `bson:"_id,omitempty"`
	OrderNumber string        `bson:"orderNumber"`
	Status      string        `bson:"status"`
	CreatedAt   time.Time     `bson:"createdAt"`
	Customer    CustomerRef   `bson:"customer"` // extended reference: id + name + country
	Items       []Item        `bson:"items"`    // embedded: always read with the order
	Total       int64         `bson:"total"`    // computed pattern: precomputed on write
}
```

One `FindOne` returns the entire order. Compare that with the four-table join in the Postgres lab.

## 2. Atomic update operators

```go
coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
	"$set":  bson.M{"status": "paid"},
	"$push": bson.M{"items": newItem},
	"$inc":  bson.M{"total": 7800},
})
```

All three changes apply **atomically** to the single document, with no transaction needed. The `history` update uses `$push` with `$each` and `$slice: -20`: a bounded array that keeps only the latest 20 events (avoiding the unbounded-array anti-pattern).

## 3. Aggregation output

```text
Top 5 SKUs by revenue …
  SKU-0149  units=283   revenue=£56967.90
Revenue by country and month …
  DE 2026-09 revenue=£53788.42
Window function: each customer's running spend
  #1 ORD-004915 total=£500.70 running=£500.70
  #2 ORD-010760 total=£138.06 running=£638.76
```

Pipelines are built with `mongo.Pipeline` of `bson.D` stages, because order matters.

## 4. Index impact (ESR)

```text
WITHOUT index:  plan=SORT←COLLSCAN       keysExamined=0   docsExamined=20000  nReturned=20
WITH ESR index: plan=LIMIT←FETCH←IXSCAN  keysExamined=21  docsExamined=20     nReturned=20
```

Two wins: **docsExamined/nReturned fell from 1000:1 to 1:1**, and the **SORT stage disappeared** because the index already yields `createdAt` order.

## 5. Transactions

```go
_, err = sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
	res, err := accounts.UpdateOne(ctx,
		bson.M{"_id": from, "balance": bson.M{"$gte": amount}},     // guard in the filter
		bson.M{"$inc": bson.M{"balance": -amount}})
	if res.MatchedCount == 0 { return nil, errors.New("insufficient funds") } // → abort
	accounts.UpdateOne(ctx, bson.M{"_id": to}, bson.M{"$inc": bson.M{"balance": amount}})
	ledger.InsertOne(ctx, bson.M{"from": from, "to": to, "amount": amount})
	return nil, nil
})
```

Output: the failed transfer leaves **no trace**, with balances and ledger unchanged. `WithTransaction` also retries `TransientTransactionError` automatically.

## 6. Change streams

```text
insert → LIVE-1 status=pending
update → LIVE-1 status=paid
…
```

Change streams read the oplog through a resumable cursor. Store the **resume token** to continue after a restart without missing events. They power cache invalidation, notifications, Atlas Triggers and the Kafka source connector.

## Exercises

> **Exercise 1 (schema validation):** Add a `$jsonSchema` validator to `orders` requiring `orderNumber`, `status` (enum) and `total >= 0`. Prove an invalid insert fails.

> **Exercise 2 (Bucket pattern):** Create a `readings` time series collection (`timeseries: {timeField, metaField, granularity}`). Insert 100k sensor readings and compare storage size with a regular collection (`collStats`).

> **Exercise 3 (Subset pattern):** Add a `products` collection with `recentReviews` (latest 10) and a separate `reviews` collection. Write `AddReview` that inserts the review and `$push`es with `$slice: -10` in one transaction.

> **Exercise 4 ($lookup vs embed):** Move the customer to a `customers` collection and rewrite the "top customers" report with `$lookup`. Compare `explain` stats. When is the join acceptable?

> **Exercise 5 (resume tokens):** Persist the change stream's resume token to a file. Kill the program mid-stream, restart with `SetResumeAfter`, and show no events were missed.

> **Exercise 6 (Atlas):** Point the lab at a free Atlas M0 cluster (`MONGODB_URI=...`). Create an Atlas Search index and add a `search` command using `$search` with fuzzy matching on product names.

```quiz
Q: Why did the ESR index remove the SORT stage from the plan?
+ After the equality fields, the index entries are already ordered by createdAt, so documents come out in sort order
- MongoDB disables sorting when an index exists
- The limit made sorting unnecessary
- Sorting moved to the client
? Equality-then-Sort ordering lets the index provide order, avoiding a blocking in-memory sort.

Q: Why doesn't the CRUD demo need a transaction to update status, items and total together?
- MongoDB ignores atomicity for embedded fields
+ Single-document operations are always atomic, and all three fields live in one document
- Transactions are disabled locally
- $inc implies a transaction
? Good document design keeps related data together, so most writes are single-document atomic operations.

Q: What must you store to resume a change stream after a crash without missing events?
- The last document _id
- The wall-clock time of the last event
+ The resume token of the last processed event
- The oplog size
? Resume tokens identify a position in the oplog. Pass it with resumeAfter or startAfter on restart.
```
