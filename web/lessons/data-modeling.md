## Why modelling matters

Most performance problems customers bring to an SA are really **modelling problems**: a schema that fights the access patterns. Fixing a model is often worth more than any amount of hardware.

## Conceptual → logical → physical

1. **Conceptual.** Entities and relationships in business terms: *Customer places Order; Order contains Products*.
2. **Logical.** Tables or documents, attributes, keys and cardinalities, independent of any product.
3. **Physical.** Product-specific: types, indexes, partitioning, shard keys, storage settings.

## Entity–relationship (ER) basics

```text
 CUSTOMER 1 ──────< N ORDER N >────── M PRODUCT
                       │
               (ORDER_ITEM: quantity, unit_price)
```

| Cardinality | Example | Relational implementation |
|---|---|---|
| One-to-one | User ↔ Profile | Same table, or FK with UNIQUE |
| One-to-many | Customer → Orders | FK on the "many" side |
| Many-to-many | Orders ↔ Products | Junction table (`order_items`) |

## Normalisation: remove redundancy

Normalisation means organising data so **each fact is stored once**. This prevents *update anomalies* (changing a fact in one place but not another).

Start with this "spreadsheet" table:

| order_id | customer_email | customer_country | product_skus | total |
|---|---|---|---|---|
| 1 | ada@x.com | GB | A1, B2 | 45.00 |
| 2 | ada@x.com | UK | C3 | 10.00 |

Problems: Ada's country is stored twice and already inconsistent (GB vs UK), the product list is a string you can't join on, and the total can drift from the line items.

![One spreadsheet row stores Ada's country twice and a list of SKUs; 3NF splits that into customers, orders, items and products.](diagrams/data-modeling-normalize.svg)

### First normal form (1NF)
Every column holds **atomic** values with no lists or repeating groups. So `product_skus` becomes one row per order line.

### Second normal form (2NF)
1NF, plus every non-key column depends on the **whole** primary key. In `order_items(order_id, product_id, quantity, product_name)`, `product_name` depends only on `product_id`, so move it to `products`.

### Third normal form (3NF)
2NF, plus there are **no transitive dependencies**: non-key columns depend only on the key. `orders(id, customer_id, customer_country)` breaks this because country depends on the customer, not the order. Move it to `customers`.

> **Remember:** "Every non-key attribute must provide a fact about the key, the whole key, and nothing but the key."

There's also BCNF, 4NF and 5NF. Know they exist; in practice 3NF is the target for OLTP.

## Denormalisation: deliberate redundancy

Normalised schemas are great for writes and integrity, but reads may need many joins. **Denormalisation** duplicates data to make reads cheap:

- Storing `unit_price_cents` on `order_items` (the price *at the time of purchase*). This is actually **correct modelling**, not just optimisation, because the product's price changes later.
- A cached `order_count` on `customers`.
- Embedding the customer's name in an order document (MongoDB's *extended reference* pattern).

| Normalise when… | Denormalise when… |
|---|---|
| Writes are frequent and must stay consistent | Reads dominate (often 10:1 or more) |
| Data is shared by many entities | Data is read together and changes rarely |
| Ad-hoc querying is needed | Access patterns are known and stable |
| Storage is precious | Join latency hurts or you're distributed |

## Access-pattern-driven modelling

Relational modelling starts with **entities**. NoSQL modelling (MongoDB, DynamoDB, Cassandra) starts with **queries**:

1. List every query the application runs, with frequency and latency targets.
2. Design structures so the hottest queries hit **one document or partition**.
3. Accept duplication, and plan how it gets updated.

> **SA angle:** You know DynamoDB single-table design from AWS. MongoDB is more forgiving: secondary indexes, rich queries, `$lookup` and transactions mean you don't need to know *every* access pattern up front. That's a real selling point against DynamoDB. "Model for your workload, but you can still evolve it."

## Keys: natural vs surrogate

- **Natural key**: a real-world identifier (email, ISBN). It can change, and it can leak PII.
- **Surrogate key**: a generated ID (`BIGSERIAL`, UUID, MongoDB `ObjectId`). Stable and meaningless.
- **UUIDv4** keys are random, which causes B-tree page splits and poor cache locality. Prefer **UUIDv7** or ULID (time-ordered) when you need globally unique IDs. MongoDB's `ObjectId` is time-prefixed for the same reason.

> **Exercise:** Model AWS billing: accounts, services, usage line items, invoices, credits. Produce (1) a 3NF relational schema and (2) a MongoDB document design for the query "show me this month's invoice with all line items". Compare them in your notes.

> **Interview:** `orders(id, customer_id, customer_email, customer_country, product_skus, total)` is "simpler". What's wrong, and when would you denormalise anyway?

> **Model answer:** It violates 1NF (a list of SKUs) and 3NF (email and country depend on the customer). Updates diverge — GB versus UK — and you cannot join to products. Split into customers, orders, order_items and products. Denormalise when the access pattern is stable and reads dominate: snapshot `unit_price_cents` on the line (a historical fact), or embed a customer's name on an order document. Do not copy country onto every order unless you mean country at purchase time.

```quiz
Q: A table orders(id, customer_id, customer_email, total) violates which normal form?
- 1NF
- 2NF
+ 3NF
- None; it is fully normalised
? customer_email depends on customer_id, not on the order's key. That's a transitive dependency, which violates 3NF.

Q: Why is storing unit_price_cents on order_items correct even in a normalised design?
+ It captures a different fact, the price at purchase time, which must not change when the product price changes
- It's a performance hack that breaks 3NF
- Because foreign keys can't reference prices
- It isn't correct; it should be joined from products
? It looks like duplication but it's a historical fact about the order line, so it belongs there.

Q: Why are random UUIDv4 primary keys often bad for B-tree indexed tables?
- They are too long to store
+ Random inserts land on random pages, causing page splits and poor cache locality
- They can't be indexed
- They are not unique enough
? Time-ordered keys (UUIDv7, ULID, ObjectId, sequences) append to the right-most page, which is much friendlier to B-trees.
```
