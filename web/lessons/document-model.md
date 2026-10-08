## Why documents

If you've written front-end code, you already think in documents. A MongoDB document is roughly the JSON object your React component wants to render or your Go handler wants to return. The core idea is that **the shape of the data in the database should match the shape the application uses**, so a typical read is one lookup that returns one complete object.

*Designing Data-Intensive Applications* calls this **locality**: if you always need the order, its lines and the customer's name together, storing them together saves the joins and the extra index lookups. It also names the cost. The document model is weaker at many-to-many relationships, and every read pulls in the whole document even if you need one field. Everything in this lesson is about using the first property and dodging the second.

![The same order as three normalised tables joined at read time, and as one document that embeds its lines and a copy of the customer's name.](diagrams/document-model-relational-vs-document.svg)

## Documents, collections, databases

| Relational | MongoDB | Notes |
|---|---|---|
| Database | Database | A namespace; one cluster holds many |
| Table | Collection | No fixed columns; optional validation |
| Row | Document | Nested objects and arrays, up to 16 MB |
| Primary key | `_id` | Always present, unique, immutable; ObjectId by default |
| Join | Embedding, or `$lookup` | Embedding is the default tool; `$lookup` is the exception |

```js
// one document in the "orders" collection (shop database in the lab)
{
  _id: ObjectId("6703c1f2e4b0a1c2d3e4f501"),
  orderNumber: "ORD-000001",
  status: "paid",
  customer: { id: 17, name: "Ada Lovelace", country: "GB" },   // copy of the hot fields
  items: [
    { sku: "PEN-01", category: "home", qty: 2, unitPrice: 150 },
    { sku: "INK-09", category: "home", qty: 1, unitPrice: 900 }
  ],
  tags: ["gift", "express"],
  total: 1200,                                                   // integer minor units (pence)
  createdAt: ISODate("2026-10-07T10:00:00Z")
}
```

## BSON: what is actually stored

MongoDB stores **BSON**, a binary serialisation of JSON-like documents. It adds the types JSON lacks and, crucially, **length-prefixes** every document, string and sub-document, so the server can jump over fields it doesn't need instead of parsing the whole thing.

![BSON for a two-field document: a total length, then for each field a type byte, the key and the value, and a terminating zero byte.](diagrams/document-model-bson.svg)

| BSON type | Why it matters |
|---|---|
| `ObjectId` | 12 bytes: 4-byte seconds timestamp, 5-byte per-process random value, 3-byte counter. Unique without coordination and roughly time-ordered, which is great for `_id` and bad as a ranged shard key |
| `Date` | 64-bit milliseconds since the epoch, UTC. Store real dates, never strings, so range queries and TTL indexes work |
| `Decimal128` | Exact decimal arithmetic. Use it, or integer minor units, for money |
| `Int32` / `Int64` / `Double` | Distinct numeric types that compare equal across types (`1`, `NumberLong(1)` and `1.0` all match `{ qty: 1 }`) |
| `Binary` | Opaque bytes: small files, hashes, encrypted fields, and compact vector embeddings (the `vector` subtype) |
| `String` | UTF-8. Comparison is binary unless you specify a **collation** |

**Limits worth knowing.** A document can be at most **16 MB** and nested at most 100 levels deep. Field names are stored in every document, so a billion documents with `customerShippingAddressLine1` keys pay for that string a billion times in cache. On disk, WiredTiger's block compression (snappy by default, zstd optional) recovers most of it, but the in-memory cache holds uncompressed documents. That's why long keys matter more for RAM than for disk.

> **Gotcha:** BSON documents are **ordered**. A query that matches a whole embedded document, such as `{ items: { sku: "PEN-01", qty: 3 } }`, only matches if the fields appear in exactly that order. Query individual fields with dot notation or `$elemMatch` instead. The same applies to Go: use `bson.D` (ordered) rather than `bson.M` (a map with random iteration order) whenever order matters.

## CRUD in mongosh

```js
use shop

// Create
db.orders.insertOne({ orderNumber: "ORD-DEMO-1", status: "pending",
  customer: { id: 17, name: "Ada Lovelace", country: "GB" }, items: [], total: 0, createdAt: new Date() })

// Read: queries are documents too
db.orders.find({ status: "paid", "customer.country": "GB" })            // dot notation into sub-documents
db.orders.find({ total: { $gte: 1000 } }).sort({ createdAt: -1 }).limit(20)
db.orders.find({ "items.sku": "SKU-0042" })                              // matches any array element
db.orders.find({ items: { $elemMatch: { sku: "SKU-0042", qty: { $gte: 2 } } } })
db.orders.find({}, { orderNumber: 1, total: 1, _id: 0 })                 // projection

// Update: operators modify fields in place, atomically per document
db.orders.updateOne(
  { orderNumber: "ORD-DEMO-1" },
  { $set: { status: "shipped" }, $push: { history: { at: new Date(), status: "shipped" } } }
)

// Update one specific array element with arrayFilters
db.orders.updateOne(
  { orderNumber: "ORD-000001" },
  { $inc: { "items.$[line].qty": 1 } },
  { arrayFilters: [{ "line.sku": "SKU-0042" }] }
)

// Update with an aggregation pipeline: recompute a derived field from the document itself
db.orders.updateOne({ orderNumber: "ORD-000001" }, [
  { $set: { total: { $sum: { $map: { input: "$items", in: { $multiply: ["$$this.qty", "$$this.unitPrice"] } } } } } }
])

// Upsert: create the cart on first add
db.carts.updateOne(
  { userId: 17 },
  { $setOnInsert: { createdAt: new Date() }, $push: { items: { sku: "PEN-01", qty: 1 } } },
  { upsert: true }
)

// Delete
db.sessions.deleteMany({ expiresAt: { $lt: new Date() } })
```

> **Key idea:** **A write to a single document is always atomic**, including every nested array and sub-document it touches. Combined with embedding, most operations that need a transaction in SQL (insert the order and its lines) are one atomic write. Multi-document ACID transactions exist (see [Replica Sets & Sharding](#/lesson/mongodb-replication-sharding)), but needing them on every request is a modelling smell.

A conditional update is also MongoDB's compare-and-set. `updateOne({ sku: "PEN-01", stock: { $gte: 1 } }, { $inc: { stock: -1 } })` either decrements or matches nothing, with no race between the check and the write. `findOneAndUpdate` does the same and returns the document. The [MVCC & Concurrency](#/lesson/mvcc-concurrency) lesson explains why this beats read-modify-write in application code.

| Category | Operators |
|---|---|
| Comparison | `$eq $ne $gt $gte $lt $lte $in $nin` |
| Logical | `$and $or $not $nor` |
| Element | `$exists $type` |
| Array | `$elemMatch $all $size` |
| Update | `$set $unset $inc $min $max $push` (with `$each`, `$slice`, `$sort`) `$pull $addToSet $setOnInsert $currentDate`, plus positional `$`, `$[]` and `$[name]` |

## Document growth and the 16 MB ceiling

The 16 MB limit is a guardrail, not a target. Problems start long before it:

- **Write amplification.** WiredTiger doesn't update documents in place on disk. A changed document is written out again as a new version when its page is reconciled. Incrementing a counter in a 2 MB document many times a second rewrites megabytes for a few bytes of change. The oplog entry is small (updates are logged as diffs), but the storage work isn't.
- **Cache pressure.** Every read loads the whole document into the WiredTiger cache, even if you project one field. Large documents mean fewer fit in RAM, so the working set spills to disk.
- **Network and decode cost.** Fetching a 2 MB profile to render a name on a list page is slow in the driver as well as the server.

The pattern behind most oversized documents is an **unbounded array**: comments, events, followers or readings pushed into a parent forever. The fixes (Subset, Bucket, Outlier) are in [Schema Design Patterns](#/lesson/mongodb-schema-design). Genuinely large binary content belongs in object storage (S3) with a reference in the document; GridFS exists but is rarely the best choice in the cloud.

## Embed or reference?

This is the central design decision. Ask three questions in order:

![Ask whether the data is read together, whether the child set is bounded, and whether it is shared or updated independently; the answers lead to reference, bucket, extended reference or embed.](diagrams/document-model-embed-or-reference.svg)

- **Embed** what belongs to the parent and is read with it: order lines, a user's two addresses, a product's variants.
- **Reference** what is unbounded, read on its own, or owned by something else: reviews, log events, the full customer profile.
- **Extended reference** is the common middle: keep the `customerId` but copy the name and country that every order screen shows. Accept that the copy may go stale, or that it is *deliberately* historical (the shipping address at the time of purchase should not change when the customer moves).

Normalisation isn't wrong in MongoDB; it's just no longer the default. You denormalise where the read path needs it and accept the cost of updating copies, which is the same trade-off *DDIA* describes for any denormalised read model.

## Flexible schema is not no schema

Documents in a collection *can* differ, which is useful for polymorphic data and gradual migrations. Production systems still need guardrails, and MongoDB enforces them on the server with `$jsonSchema` validation:

```js
db.createCollection("customers", {
  validator: { $jsonSchema: {
    bsonType: "object",
    required: ["email", "name", "country"],
    properties: {
      email:   { bsonType: "string", pattern: "^[^@\\s]+@[^@\\s]+$" },
      country: { enum: ["GB", "US", "DE", "IN", "FR"] },
      loyalty: { bsonType: "object", properties: {
        points: { bsonType: ["int", "long"], minimum: 0 }
      } }
    }
  }},
  validationLevel: "strict",     // "moderate" skips existing documents that already fail
  validationAction: "error"      // "warn" logs violations instead of rejecting
})

db.customers.insertOne({ email: "bad", name: "X", country: "UK" })   // MongoServerError: Document failed validation
```

- **Roll validation out gradually** on an existing collection: start with `validationAction: "warn"`, read the log, fix the writers, then switch to `"error"`. `collMod` changes the validator without downtime.
- **Rejected writes explain themselves.** The error's `errInfo.details` lists exactly which rule failed, which makes validators practical to debug.
- **Validation is per collection, not a type system.** Keep the application model as the source of truth and use the validator to catch bugs and rogue writers.

> **Gotcha:** Numeric types depend on the writer. mongosh stores whole numbers that fit in 32 bits as `Int32` but larger ones (epoch milliseconds, big counters) as `Double`, and each driver has its own rules. Validate with `bsonType: ["int", "long"]` or `"number"` unless you truly need one width, and check real data with `{ $type: "$field" }` in an aggregation.

## Mapping documents to Go structs

The Go driver maps structs to BSON with struct tags, much like `encoding/json`. This is the lab's order model (driver v2):

```go
type Order struct {
	ID          bson.ObjectID `bson:"_id,omitempty"` // omitempty lets the driver generate _id on insert
	OrderNumber string        `bson:"orderNumber"`
	Status      string        `bson:"status"`
	CreatedAt   time.Time     `bson:"createdAt"`      // stored as BSON Date (millisecond precision, UTC)
	Customer    CustomerRef   `bson:"customer"`       // embedded sub-document
	Items       []Item        `bson:"items"`          // embedded array
	Total       int64         `bson:"total"`          // integer minor units
	History     []Event       `bson:"history,omitempty"`
}

var o Order
err := orders.FindOne(ctx, bson.D{{Key: "orderNumber", Value: "ORD-000001"}}).Decode(&o)
```

- **Nested structs become sub-documents** and slices become arrays, so a well-modelled document decodes into one struct with no ORM.
- **`omitempty` matters**: without it, a zero `ObjectID` is inserted literally and the second insert fails with a duplicate key.
- **Use `bson.Decimal128` for decimal money** and `time.Time` for dates. Note that BSON dates truncate to milliseconds, so a round-tripped `time.Now()` won't compare equal to the original.
- **Use `bson.D` for commands, sorts and pipelines** where order matters; use `bson.M` only for unordered filters.
- **Use `,inline`** to flatten an embedded struct into the parent, which is handy for shared base fields in polymorphic collections.

The [Go & Databases](#/lesson/go-database-fundamentals) lesson covers connection pooling, contexts and error handling in the driver.

## When documents beat tables (and when they don't)

| Documents shine | Relational shines |
|---|---|
| Data read and written together as an aggregate (order and lines, product and variants) | Highly interconnected data queried in many unpredictable ways |
| Polymorphic or evolving attributes (catalogues, IoT payloads, user profiles) | Strict, stable schema with many cross-entity constraints |
| Horizontal scale-out with built-in sharding | Ad-hoc analytical joins across many entities (better still: a warehouse or lakehouse) |
| Objects map directly to code; no ORM | Teams and BI tools that live in SQL against the OLTP database |

The line has blurred from both sides. Postgres has `jsonb` (see [Data Modelling](#/lesson/data-modeling)) and MongoDB has multi-document transactions, `$lookup` and schema validation. The honest difference is the **default**: MongoDB optimises for the aggregate and makes joins the exception, Postgres optimises for normalised tables and makes documents the exception.

> **SA angle:** Don't lead with "MongoDB is schemaless". That phrase worries architects and is not true in production. Lead with **developer productivity** ("your data looks like your code"), **flexibility** ("add a field without a migration window"), and **scale** ("sharding is built in"). Then prove it by modelling one of *their* entities on the whiteboard and showing that the hot read becomes one document fetch.

## Real-world scenario

### Problem: Profile updates got slower every month

> **Real world:** A media app stores each user as one document with their profile, settings, and an `activity` array that receives an entry every time they watch something. It was fast at launch. A year later, `updateOne` on active users takes hundreds of milliseconds, cache usage is high, and a few writes fail with *"BSONObjectTooLarge"*.

**Symptoms.** p99 write latency climbs steadily with account age, not with traffic. WiredTiger cache "bytes read into cache" is high. The slow query log shows `updateOne` on `users` with large `docsExamined` sizes, and errors for a handful of power users.

**Diagnose.**

```js
// Which documents are big, and what is making them big?
db.users.aggregate([
  { $project: { size: { $bsonSize: "$$ROOT" }, activity: { $size: { $ifNull: ["$activity", []] } } } },
  { $sort: { size: -1 } },
  { $limit: 5 }
])
db.users.stats().avgObjSize        // average document size in bytes
```

**Root causes.** An unbounded array (`activity`) embedded in a frequently updated, frequently read document. Every new entry rewrites an ever larger document, and every profile read drags the history into cache.

**Fix.** Immediately, stop the bleeding by capping the array on write with `$push: { activity: { $each: [entry], $slice: -50 } }`, so it keeps only the 50 most recent items. Durably, move history to its own collection with a parent reference, and keep a small **subset** of recent items on the user:

```js
db.activity.insertOne({ userId: 17, titleId: "t-981", at: new Date(), seconds: 1260 })
db.activity.createIndex({ userId: 1, at: -1 })
db.users.updateOne({ _id: 17 }, { $push: { recent: { $each: [{ titleId: "t-981", at: new Date() }], $slice: -20 } } })
```

Backfill the new collection in batches, then `$unset` the old array.

**Prevent.** Alert on `avgObjSize` and on the largest documents per collection. Add a `$jsonSchema` rule with `maxItems` on arrays that must stay small. Review every `$push` in code review with the question "what bounds this?".

> **Model answer — what to say in the interview:** The document is growing without bound because history is embedded in the user. WiredTiger rewrites the whole document on update and loads it whole into cache, so latency grows with document size, and eventually it hits 16 MB. I'd cap the array with `$push` plus `$slice` as a hotfix, then move history to its own collection keyed by `userId` and `at`, keeping the last 20 items on the user as a subset for the home screen. I'd backfill in batches and add monitoring on document size so it can't creep back.

> **Lab:** Run `go run ./labs/mongodb crud`, then in `mongosh` (`docker exec -it dbm-mongo mongosh shop`) find all orders containing a given SKU with quantity of at least 2, newest first, using `$elemMatch`. Then add the `customers` validator above, insert an invalid document, and read `errInfo.details` in the error.

```quiz
Q: What is the maximum size of a single MongoDB document?
- 1 MB
- 4 MB
+ 16 MB
- Unlimited
? Large binary content belongs in object storage with a reference. Documents approaching the limit almost always signal an unbounded array.

Q: Which statement about atomicity in MongoDB is true?
+ Any write to a single document, including its nested arrays and sub-documents, is atomic
- Only top-level fields are updated atomically
- Atomicity requires a transaction even for one document
- Writes are atomic only when using write concern majority
? Single-document atomicity plus embedding covers most transactional needs. Write concern controls durability, not atomicity.

Q: Which query finds orders where the SAME line item has sku "PEN-01" and qty of at least 2?
- { "items.sku": "PEN-01", "items.qty": { $gte: 2 } }
- { items: { sku: "PEN-01", qty: 2 } }
+ { items: { $elemMatch: { sku: "PEN-01", qty: { $gte: 2 } } } }
? Without $elemMatch the two conditions can be satisfied by different array elements. The embedded-document match requires exact field order and values.

Q: A team stores a 3 MB activity history inside each user document and updates it constantly. What is the main performance problem?
- The _id index becomes unbalanced
+ Each update rewrites a large document and each read loads all of it into cache, so latency grows with document size
- BSON can't store arrays longer than 1,000 elements
- MongoDB locks the whole collection during the update
? WiredTiger writes a new version of the whole document and caches documents whole. The fix is to reference the history and keep a small subset.

Q: A collection has a $jsonSchema validator requiring points to be bsonType "int". Writes of 3000000000 from mongosh fail. Why?
- Validators can't check numeric fields
- mongosh sends all numbers as strings
+ Whole numbers that don't fit in 32 bits are sent as Double, which is not "int"
- The validator only applies to updates
? mongosh encodes small whole numbers as Int32 and larger ones as Double. Allow ["int", "long"] or "number", or use NumberLong explicitly.

Q: Why should you prefer bson.D over bson.M for a sort specification in Go?
- bson.D is faster to allocate
+ bson.D preserves field order, and the order of sort keys changes the result
- bson.M can't hold integers
- bson.M is deprecated in driver v2
? A map has no defined iteration order, so { a: 1, b: -1 } might be sent as { b: -1, a: 1 }, which sorts differently and may not match your index.
```
