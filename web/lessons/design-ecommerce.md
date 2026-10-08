## The prompt

> "Design the data architecture for an online retailer: 5M products, 20M customers, 50k orders per hour at peak (Black Friday 10x). Product search, a personalised home page, cart, checkout and order history, plus analytics for merchandising."

## 1. Clarify (questions to ask)

- Is the catalogue shared across regions, or per country? Multi-currency?
- Inventory: one warehouse or many? Is overselling ever acceptable (backorders)?
- What's the checkout latency target? Payment provider?
- Search: typo tolerance, facets, synonyms? Personalised ranking?
- Analytics freshness: daily, or near real time?
- Existing stack and team? Cloud? Compliance (PCI for payments)?

## 2. Estimate

| Metric | Value |
|---|---|
| Orders | 50k/hour ≈ 14/s, Black Friday ≈ 140/s writes (plus inventory updates) |
| Product reads | Usually 100–1000x writes, so ~10–50k reads/s at peak |
| Catalogue size | 5M × ~5 KB ≈ 25 GB (fits in RAM on a decent cluster) |
| Orders per year | ~200M × 2 KB ≈ 400 GB/year |

## 3. Access patterns

| # | Pattern | Frequency | Consistency |
|---|---|---|---|
| A | Product detail by ID/slug | Very high | Eventual OK |
| B | Search with text, filters, facets | High | Seconds-stale OK |
| C | Cart read/update | High | Read-your-writes |
| D | Checkout: reserve stock, create order, take payment | Medium | **Strong** |
| E | Order history per customer | Medium | Read-your-writes |
| F | Merchandising analytics | Batch / near real time | Eventual |

## 4. Architecture (MongoDB-centred variant)

![Catalogue, orders and search are separate stores with different consistency needs; change streams feed Kafka and the lakehouse rather than sharing the write path.](diagrams/design-ecommerce-architecture.svg)

Cart (MongoDB or Redis, TTL) and inventory (one document per SKU per warehouse, atomic `$inc`) sit beside those three. CDN serves images.

### Data model highlights

```js
// products: polymorphic + attribute pattern, embedded variants (one-to-few)
{ _id: "SKU-123", type: "shoe", title: "Trail Runner", brand: "Acme", price: { amount: 8999, currency: "GBP" },
  attrs: [ { k: "colour", v: "blue" }, { k: "size", v: "9" } ],
  variants: [ { sku: "SKU-123-9-BL", size: 9, colour: "blue" } ],
  rating: { avg: 4.6, count: 1823 } }                     // computed pattern

// orders: immutable snapshot of what was bought (extended reference)
{ _id, orderNumber, customer: { id, name, email }, status, items: [ { sku, title, qty, unitPrice } ],
  totals: {...}, shipping: {...}, payment: { provider, ref, status }, createdAt }
// index: { "customer.id": 1, createdAt: -1 }   → pattern E

// inventory: one document per SKU per warehouse
{ _id: { sku: "SKU-123-9-BL", wh: "LON1" }, available: 42, reserved: 3 }
```

### Checkout without overselling

```js
// atomic conditional decrement: succeeds only if enough stock remains
db.inventory.updateOne(
  { _id: { sku, wh }, available: { $gte: qty } },
  { $inc: { available: -qty, reserved: qty } }
)
// matchedCount === 0 → out of stock
```

For a multi-item cart, wrap the reservations plus order creation in a **multi-document transaction**, or use a **saga**: reserve each item, then create the order, then confirm payment, with compensations (release reservations) on failure. Reservations get a TTL so abandoned checkouts release stock.

### Hot-key risk on Black Friday

A single hot SKU means one inventory document takes thousands of writes per second. Options:
- **Shard the counter**: split stock into N sub-documents (`{sku, shard: 0..9}`) and decrement a random one with stock.
- Queue purchases for that SKU (a virtual waiting room).
- Pre-allocate stock to regions or warehouses.

## 5. The Postgres-centred alternative (and how to compare)

A relational design (Postgres or Aurora) works well too: `products` + JSONB attributes, `orders`/`order_items`, `inventory` with `UPDATE … WHERE available >= qty`, plus Elasticsearch/OpenSearch for search and Debezium CDC to keep it in sync. The trade-offs to articulate:

| | MongoDB design | Postgres design |
|---|---|---|
| Catalogue flexibility | Native polymorphism | JSONB, workable |
| Search | Integrated Atlas Search, no sync pipeline | Separate cluster plus CDC |
| Scale-out writes | Native sharding | Vertical, Citus, or app sharding |
| Complex ad-hoc SQL | Aggregation pipeline / Atlas SQL | Excellent |
| Team familiarity | Depends | Usually high |

## 6. Analytics path (Databricks angle)

Change streams or Debezium feed Kafka, which lands in **bronze** Delta tables. A declarative pipeline produces **silver** orders and inventory, then **gold** `fact_sales` + dimensions (SCD2 for product price history). AI/BI dashboards, demand forecasting and recommendation models consume gold, and Vector Search provides similar products. Results are served back via the operational DB or Lakebase.

## 7. Failure modes

| Failure | Mitigation |
|---|---|
| Primary node failure during checkout | Replica set failover (seconds) plus retryable writes plus idempotency key on order creation |
| Payment provider timeout | Order stays PENDING; reconciliation job; never double-charge (idempotency key) |
| Search index lag | Acceptable (seconds); show stock from the inventory service at add-to-cart |
| Region outage | Multi-region replica set (writes fail over to another region); RPO near 0 with majority writes across regions |
| Bad deploy corrupts data | PITR backups; delayed secondary |

> **SA angle:** For **MongoDB**, lead with the catalogue (polymorphism, Atlas Search) and checkout correctness (transactions). For **Databricks**, lead with the analytics and personalisation loop (lakehouse, ML, serving). For **Red Hat**, lead with the platform (services on OpenShift, Kafka/Debezium for events, operators for the databases, the same stack in-store at the edge and in the cloud). Same problem, three winning narratives.

```quiz
Q: How do you prevent overselling a SKU when two customers check out the last unit simultaneously?
- Read the stock, then write the new value in application code
+ An atomic conditional update (decrement only if available >= qty), or a transaction with the same guard
- Cache inventory in Redis with a 1-minute TTL
- Rely on eventual consistency and refund later
? The condition and decrement must be a single atomic operation (or protected by locking or serializable isolation).

Q: Why snapshot item titles and prices inside the order document?
+ An order must reflect what was bought at that time, even if the product changes later
- To save storage
- Because MongoDB can't do joins
- To make search faster
? This is the extended reference pattern applied to a legal and business record.

Q: A single SKU receives thousands of inventory writes per second on Black Friday. What helps?
- Adding more indexes
+ Sharding the counter into sub-documents, or queueing purchases for that SKU
- Increasing the document size
- Switching to read preference secondary
? Hot keys need write spreading or serialisation via a queue. More hardware doesn't fix single-document contention.
```
