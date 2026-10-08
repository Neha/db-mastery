## The problem indexes solve

Without an index, `SELECT * FROM customers WHERE email = 'ada@x.com'` must read **every row**. That's a *full table scan*, O(n). With an index it's a tree walk, O(log n). At 100 million rows that's the difference between seconds and about 30 microseconds.

> **Key idea:** An index is a separate, sorted data structure that maps *key → location of the row*. It's exactly like the index at the back of a book.

## The B-tree (actually B+tree)

Almost every OLTP database (Postgres, MySQL InnoDB, Oracle, SQL Server, MongoDB/WiredTiger) uses B+trees for indexes.

![A B+tree lookup for key 57 touches one page per level. Leaves are sorted and linked, so range scans walk sideways instead of climbing back up the tree.](diagrams/indexes-btree.svg)

- Each node is a **page** (8 KB in Postgres, 16 KB in InnoDB) holding hundreds of keys. The high **fan-out** keeps the tree shallow: 3–4 levels for billions of rows.
- Leaves are **sorted** and **linked**, so range queries (`BETWEEN`, `>`, `ORDER BY`, prefix `LIKE 'abc%'`) are efficient.
- Lookups, inserts and deletes are O(log n). Inserts may **split** pages.

## Kinds of indexes

| Index type | What it is | Use |
|---|---|---|
| Primary / clustered | Table rows stored in index order (InnoDB, SQL Server, MongoDB `_id` is not clustered by default) | PK lookups and ranges |
| Secondary | Points to the row (heap TID in Postgres, PK in InnoDB) | Other lookups |
| Unique | Enforces no duplicates | Emails, SKUs |
| Composite | Multiple columns `(a, b, c)` | Multi-column filters and sorts |
| Covering | Includes every column the query needs (`INCLUDE (...)`) | Index-only scans, no table visit |
| Partial | Only rows matching a predicate (`WHERE status='active'`) | Small, hot subsets |
| Expression | Indexes a function `lower(email)` | Case-insensitive lookup |
| Hash | Equality only | Exact match lookups |
| GIN / inverted | Many keys per row (arrays, JSONB, full text) | `@>`, text search, MongoDB multikey |
| GiST / R-tree | Geometric and range types | Geospatial |
| BRIN | Min/max per block range | Huge append-only time-ordered tables |
| Vector (HNSW, IVF) | Approximate nearest neighbour | Semantic search, RAG |

## Composite indexes and the leftmost-prefix rule

An index on `(country, created_at)` is sorted by country, then by date within each country.

```sql
CREATE INDEX idx_cust_country_created ON customers (country, created_at);

-- ✅ Uses the index: equality on the first column, range on the second
SELECT * FROM customers WHERE country = 'GB' AND created_at > '2026-01-01';

-- ✅ Uses the index: leftmost prefix
SELECT * FROM customers WHERE country = 'GB';

-- ❌ Can't seek efficiently: skips the leading column
SELECT * FROM customers WHERE created_at > '2026-01-01';
```

![An index on (country, created_at) can seek on country, then date; a filter on date alone cannot.](diagrams/indexes-leftmost-prefix.svg)

> **Remember:** Column order matters. A good default is **equality columns first, then sort columns, then range columns**. MongoDB calls this the **ESR rule** (Equality, Sort, Range), and you'll see it again in the MongoDB track.

## Selectivity

**Selectivity** is the fraction of rows a predicate matches. Indexes help when selectivity is low (few rows match).

- `WHERE email = ?` matches 1 row, so the index is great.
- `WHERE is_active = true` matches 95% of rows, so a full scan is cheaper. The optimizer will (correctly) ignore the index.

Random I/O through an index to fetch 95% of the table is slower than a sequential scan. That's why "I added an index and it's not used!" is usually the optimizer being right.

## Indexes aren't free

- Every `INSERT`/`UPDATE`/`DELETE` must also update every index (**write amplification**).
- They consume memory and disk. Ideally the working set of indexes fits in RAM.
- More indexes give the planner more options to consider (and get wrong).

> **Warning:** A common customer anti-pattern: "we indexed every column". Writes slow down, RAM fills with rarely used indexes, and performance gets worse. Use `pg_stat_user_indexes` (Postgres) or `$indexStats` (MongoDB) to find unused indexes.

## Reading an execution plan

```sql
EXPLAIN ANALYZE
SELECT * FROM customers WHERE email = 'ada@example.com';
```

```text
Index Scan using customers_email_key on customers  (cost=0.42..8.44 rows=1 width=64)
                                                   (actual time=0.031..0.032 rows=1 loops=1)
  Index Cond: (email = 'ada@example.com'::text)
Planning Time: 0.09 ms
Execution Time: 0.05 ms
```

| You see | Meaning |
|---|---|
| `Seq Scan` | Full table scan |
| `Index Scan` | Index walk, then fetch rows from the table |
| `Index Only Scan` | Answered from the index alone (covering) |
| `Bitmap Heap Scan` | Collect matching row locations, then read pages in order |
| `rows=` estimated vs actual | Big mismatches mean stale statistics, so run `ANALYZE` |

> **SA angle:** In a customer performance review, the conversation goes: *slow query → explain plan → missing or misordered index or a modelling issue → fix → measure*. Practise doing this live. It's one of the most common SA demos at MongoDB (Atlas **Performance Advisor** suggests indexes automatically) and a classic tuning story at Databricks (where the "index" equivalents are data skipping, Z-ordering and liquid clustering).

> **Lab:** In `go run ./labs/postgres explain` you'll load 200k rows, run a query with and without an index, and compare plans and timings.

> **Interview:** You have an index on `(country, created_at)`. A report filters only on `created_at`. Why is it slow?

> **Model answer:** Leftmost-prefix rule. The index is sorted by country first, so `created_at` values for one day are scattered across every country. The planner cannot seek; it may scan the index or fall back to a sequential scan. Add `(created_at)` or put the range column after an equality you actually have. Equality columns first, then sort, then range — MongoDB's ESR rule is the same idea.

```quiz
Q: You have an index on (country, created_at). Which query can NOT use it efficiently for seeking?
- WHERE country = 'GB'
- WHERE country = 'GB' AND created_at > '2026-01-01'
+ WHERE created_at > '2026-01-01'
- WHERE country = 'GB' ORDER BY created_at
? Without the leading column (country), the index isn't sorted by created_at, so there's no efficient seek.

Q: Why might the optimizer ignore an index on a boolean column is_active?
+ Low selectivity: most rows match, so a sequential scan is cheaper than random index lookups
- Boolean columns can't be indexed
- The index is corrupt
- Indexes only work on primary keys
? When a predicate matches a large fraction of rows, sequential I/O beats many random page fetches.

Q: What is a covering index?
- An index on every column of the table
+ An index containing all columns a query needs, allowing an index-only scan
- An index that covers multiple tables
- A unique index on the primary key
? If the index has every needed column, the database never touches the table, which saves a lot of I/O.

Q: What is the main cost of adding many indexes?
- Slower reads
+ Slower writes and more memory and disk used, because every index must be maintained on each write
- Queries return wrong results
- The table can no longer be joined
? Each write must update every index. That's write amplification plus extra RAM pressure.
```
