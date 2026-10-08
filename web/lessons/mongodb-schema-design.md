## The design process

MongoDB's recommended methodology:

1. **Describe the workload.** List the operations (reads and writes), their frequency, latency needs and data sizes.
2. **Identify relationships.** For each, decide to **embed** or **reference**.
3. **Apply patterns** to optimise the hot paths.

> **Key idea:** In relational design you ask "what are the entities?". In MongoDB you ask "what does the application read and write, and how often?"

## Embed vs reference

| Embed when… | Reference when… |
|---|---|
| "Contains" or one-to-few relationship (order → line items, user → addresses) | One-to-many-many or unbounded (product → millions of reviews) |
| Data is read together | Data is read independently |
| Child data rarely changes or belongs to the parent | Child is shared by many parents (many-to-many) |
| Combined size stays well under 16 MB | Arrays would grow without bound |

### One-to-few: embed
```js
{ _id: 1, name: "Ada", addresses: [ { type: "home", city: "London" }, { type: "work", city: "Cambridge" } ] }
```

### One-to-many: array of references (in the parent)
```js
// product
{ _id: "KB-01", name: "Keyboard", partIds: ["P1", "P2", "P3"] }
```

### One-to-squillions: parent reference (in the child)
```js
// log entry: millions per host, so never an array in the host document
{ _id: ObjectId(), hostId: "web-07", level: "error", msg: "...", at: ISODate() }
```

### Many-to-many: references on one or both sides, or a linking collection
```js
{ _id: "student1", courseIds: ["c1", "c2"] }
{ _id: "c1", title: "Databases", studentCount: 240 }
```

## The patterns every MongoDB SA should know

![Four ways to place related data: embed when it is small and read together; reference when it is unbounded or shared; bucket many tiny events; design for the common case and overflow the outliers.](diagrams/mongodb-schema-design-patterns.svg)

### 1. Extended reference
Copy the frequently needed fields of a referenced document to avoid a `$lookup`:
```js
{ _id: "ORD-1", customer: { _id: 17, name: "Ada Lovelace" /* not the whole profile */ }, ... }
```
Trade-off: you must update the copies if the name changes (rare, and often you *want* the historical value).

### 2. Subset
Embed only the hot subset, and keep the rest in another collection:
```js
// product page shows the 10 most recent reviews; the full list lives in `reviews`
{ _id: "KB-01", name: "Keyboard", recentReviews: [ /* 10 */ ], reviewCount: 18234 }
```

### 3. Computed
Pre-compute aggregates on write instead of on every read:
```js
db.products.updateOne({ _id: "KB-01" }, { $inc: { reviewCount: 1, ratingSum: 5 } })
// avg = ratingSum / reviewCount, computed cheaply on read
```

### 4. Bucket
Group many small items into a document per time window or count. This is classic for IoT:
```js
{ sensorId: "s-1", hour: ISODate("2026-10-07T10:00Z"), count: 60,
  readings: [ { m: 0, t: 21.5 }, { m: 1, t: 21.6 }, /* ... */ ], sum: 1290.4 }
```
This gives fewer documents, smaller indexes and pre-aggregated stats. MongoDB **time series collections** do this automatically.

### 5. Outlier
Most documents are small, but a few (a celebrity with 50M followers) are huge. Design for the common case and flag the outliers:
```js
{ _id: "book1", buyers: [ /* up to 1000 */ ], hasOverflow: true }   // extra buyers in an overflow collection
```

### 6. Attribute
Many similar, sparse fields? Turn them into key-value pairs so one index covers all of them:
```js
// instead of { releaseUS: ..., releaseUK: ..., releaseFR: ... }
{ releases: [ { k: "US", v: ISODate() }, { k: "UK", v: ISODate() } ] }
db.movies.createIndex({ "releases.k": 1, "releases.v": 1 })
```

### 7. Polymorphic
Different shapes in one collection, distinguished by a type field. Example: a `products` collection holding books, laptops and shirts, each with type-specific attributes. Queries for the shared fields work across all of them.

### 8. Schema versioning
Add `schemaVersion: 2` to new documents. The app handles both shapes and migrates lazily or in the background. There's no big-bang migration and no downtime.

### 9. Archive
Move old, cold data to cheaper storage: Atlas **Online Archive** to object storage, queryable via federation.

## Anti-patterns to spot in a customer review

| Anti-pattern | Symptom | Fix |
|---|---|---|
| **Unbounded arrays** | Documents grow forever, slow updates, 16 MB errors | Reference, Bucket, Subset |
| **Massive number of collections** | One collection per customer or day, so thousands of files and indexes | Single collection plus a tenant or date field |
| **Unnecessary indexes** | Slow writes, RAM pressure | `$indexStats`, drop unused |
| **Bloated documents** | Reading 2 MB to show a name | Subset, split cold fields |
| **Separating data accessed together** | `$lookup` on every request | Embed or extended reference |
| **Case-insensitive queries without a collation index** | Collection scans | Index with collation, or Atlas Search |
| **Relational schema ported 1:1** | Joins everywhere via `$lookup` | Redesign around the workload |

> **Interview:** A product document embeds every review in an array. What's wrong, and what do you do instead?

> **Model answer:** The array is unbounded: documents grow forever, updates rewrite the whole document, and you will hit the 16 MB limit. Store reviews in their own collection with `productId` (parent reference). Embed a small recent subset on the product if the page needs it (Subset), and keep a computed `reviewCount`.

> **SA angle:** A schema review is one of the highest-value things a MongoDB SA does. The typical flow: customer complains about performance → you find a relational model ported 1:1 or unbounded arrays → redesign with 2–3 patterns → 10x improvement. Prepare one such story (real or from a lab) for the interview.

## Worked example: a blogging platform

Workload: show a post with author name, the first 20 comments and a like count (100k reads per minute); add a comment (2k writes per minute); show an author's profile with post titles.

```js
// posts
{ _id, slug, title, body, tags: [...],
  author: { _id, name, avatarUrl },              // extended reference
  likeCount: 4211, commentCount: 382,             // computed
  recentComments: [ /* 20 newest */ ] }           // subset

// comments (one-to-squillions → parent reference)
{ _id, postId, author: { _id, name }, text, createdAt }
db.comments.createIndex({ postId: 1, createdAt: -1 })
```

The hot read is **one document fetch**. Adding a comment is an insert into `comments` plus one `updateOne` with `$push` and `$slice: -20` on the post. Wrap both in a transaction if you need strict consistency.

> **Exercise:** Design the MongoDB schema for a ride-hailing app: riders, drivers, live driver locations (updated every 4s), trips and payments. Which patterns do you use? Where do you need a transaction? Which fields get geospatial (`2dsphere`) indexes?

```quiz
Q: A product can have millions of reviews. How should reviews be modelled?
- Embed all reviews in the product document
+ Store reviews in their own collection with a productId reference, optionally embedding a small subset of recent ones in the product
- Store reviews in a single giant document
- Create one collection per product
? Unbounded arrays break the 16 MB limit and hurt performance. The parent reference plus Subset pattern is the standard answer.

Q: An IoT platform stores one document per sensor reading (1 per second per sensor), and indexes are huge. Which pattern helps most?
- Polymorphic
- Extended reference
+ Bucket (or a native time series collection)
- Schema versioning
? Bucketing readings per sensor per time window shrinks document and index counts and enables pre-aggregation.

Q: Which is an anti-pattern?
- Embedding addresses in a user document
+ Creating a separate collection for each customer tenant
- Storing a computed reviewCount
- Using $jsonSchema validation
? Thousands of collections means thousands of files and indexes, which hurts performance and manageability.

Q: What does the Extended Reference pattern do?
+ Copies frequently used fields of a related document into the referencing document to avoid joins
- Extends the 16 MB document limit
- Creates foreign key constraints
- Adds a TTL to documents
? E.g. storing customer.name in an order, accepting occasional duplication for far cheaper reads.
```
