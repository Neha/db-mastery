## Advanced SQL: CTEs, windows & EXPLAIN

This is the SQL that separates "can query" from "can analyse". Databricks interviews in particular often include a live SQL exercise, and window functions show up almost every time.

## Common Table Expressions (CTEs)

A CTE names a sub-result so a complex query reads top to bottom, like declaring `const`s in a function.

```sql
with customer_spend as (
  select o.customer_id, sum(oi.qty * oi.unit_price_cents) as spend_cents
  from orders o
  join order_items oi on oi.order_id = o.id
  where o.status = 'paid'
  group by o.customer_id
),
ranked as (
  select customer_id, spend_cents,
         ntile(10) over (order by spend_cents desc) as decile
  from customer_spend
)
select c.name, r.spend_cents / 100.0 as spend_gbp
from ranked r join customers c on c.id = r.customer_id
where r.decile = 1           -- top 10% of spenders
order by spend_gbp desc;
```

### Recursive CTEs (hierarchies and graphs)

```sql
-- employees(id, name, manager_id): everyone under manager 1
with recursive org as (
  select id, name, manager_id, 1 as depth from employees where id = 1
  union all
  select e.id, e.name, e.manager_id, org.depth + 1
  from employees e join org on e.manager_id = org.id
)
select * from org order by depth;
```

## Window functions

A window function computes over a set of rows *related to the current row* **without collapsing them** the way `group by` does.

```text
function(...) over (partition by <group> order by <sort> rows between <frame>)
```

```sql
select
  customer_id,
  id as order_id,
  created_at,
  row_number() over (partition by customer_id order by created_at)        as nth_order,
  lag(created_at) over (partition by customer_id order by created_at)     as previous_order_at,
  count(*) over (partition by customer_id)                                as total_orders
from orders;
```

| Function | Use |
|---|---|
| `row_number()` | Unique sequence; **dedup** and "latest per group" |
| `rank()` / `dense_rank()` | Ranking with ties (gaps / no gaps) |
| `lag()` / `lead()` | Previous / next row: deltas, sessionisation, churn |
| `sum() over (order by …)` | Running totals |
| `avg() over (… rows between 6 preceding and current row)` | Moving averages |
| `ntile(n)` | Buckets: quartiles, deciles |
| `first_value()` / `last_value()` | Boundary values in a frame |

![PARTITION BY country, ORDER BY spend: the running sum walks each country in spend order and resets when the country changes.](diagrams/advanced-sql-window.svg)

> **Interview:** When do you use `RANK`, `DENSE_RANK` and `ROW_NUMBER`?

> **Model answer:** `ROW_NUMBER` assigns a unique sequence, so it is the tool for "latest per group" and dedup (`rn = 1`). `RANK` and `DENSE_RANK` are for leaderboards: ties share a rank. `RANK` leaves gaps (1, 1, 3); `DENSE_RANK` does not (1, 1, 2). Never use `RANK` to pick a single winner — two rows can share rank 1.

### The "latest row per group" pattern (asked constantly)

```sql
select * from (
  select o.*, row_number() over (partition by customer_id order by created_at desc) as rn
  from orders o
) t
where rn = 1;

-- Databricks SQL / Snowflake / BigQuery shortcut:
-- select * from orders qualify row_number() over (partition by customer_id order by created_at desc) = 1;
-- Postgres alternative:
-- select distinct on (customer_id) * from orders order by customer_id, created_at desc;
```

### Running total and 7-day moving average

```sql
with daily as (
  select date_trunc('day', o.created_at) as day, sum(oi.qty * oi.unit_price_cents) / 100.0 as revenue
  from orders o join order_items oi on oi.order_id = o.id
  group by 1
)
select day, revenue,
       sum(revenue) over (order by day)                                     as running_total,
       avg(revenue) over (order by day rows between 6 preceding and current row) as ma_7d
from daily order by day;
```

## Upserts

```sql
-- Postgres
insert into products (sku, name, category, price_cents, stock)
values ('PEN-01', 'Pen', 'stationery', 150, 100)
on conflict (sku) do update set price_cents = excluded.price_cents, stock = excluded.stock;

-- Databricks / Delta Lake (and SQL standard)
merge into products t
using updates s on t.sku = s.sku
when matched then update set *
when not matched then insert *;
```

`MERGE` is at the heart of CDC pipelines on Databricks. You'll see it again in the Lakehouse track.

## Reading EXPLAIN

```sql
explain (analyze, buffers)
select c.name, count(*) from customers c join orders o on o.customer_id = c.id
where c.country = 'UK' group by c.name;
```

```text
HashAggregate  (actual time=45.1..46.0 rows=812 loops=1)
  ->  Hash Join  (cost=… rows=…) (actual time=3.2..38.7 rows=20311 loops=1)
        Hash Cond: (o.customer_id = c.id)
        ->  Seq Scan on orders o  (actual rows=100000)
        ->  Hash
              ->  Seq Scan on customers c  (actual rows=812)
                    Filter: (country = 'UK')
                    Rows Removed by Filter: 9188
Buffers: shared hit=1234 read=56
```

How to read it:

1. **Read inside-out, bottom-up.** The most indented nodes run first.
2. Compare **estimated rows vs actual rows**. A big mismatch means stale statistics (run `analyze`), which leads to bad plans.
3. Look for **Seq Scan on large tables** with selective filters: a candidate for an index.
4. **Join types:** Nested Loop (small inputs or indexed inner side), Hash Join (big unsorted inputs), Merge Join (both sorted).
5. **Buffers:** `hit` = served from cache, `read` = from disk.

> **SA angle:** At Databricks the equivalent is the **query profile** in Databricks SQL and the **Spark UI** (stages, shuffles, skew). The skills transfer directly: find the expensive operator, check the row counts, check whether data is being skipped.

> **Exercise:** Using the lab schema, write: (1) each customer's days between consecutive orders using `lag`, (2) the top 3 products per category by revenue using `dense_rank`, (3) month-over-month revenue growth as a percentage.

```quiz
Q: Which function is best for deduplicating rows, keeping the latest per key?
+ row_number() over (partition by key order by updated_at desc), then filter rn = 1
- rank() over (order by key)
- count(*) over ()
? row_number assigns a unique sequence per partition, so exactly one row has rn = 1.

Q: How do window functions differ from GROUP BY aggregates?
+ They compute across related rows but keep every input row in the output
- They're always faster
- They can only be used with dates
? GROUP BY collapses rows into groups; window functions annotate each row.

Q: In EXPLAIN ANALYZE, estimated rows = 10 but actual rows = 2,000,000. What's the most likely cause and fix?
- Missing primary key; add one
+ Stale or missing statistics; run ANALYZE (and consider extended statistics)
- The query is wrong
? The planner chose a plan for 10 rows. Bad estimates lead to bad join and scan choices.

Q: What SQL statement is the workhorse of CDC/upsert pipelines on Delta Lake?
- INSERT OVERWRITE
+ MERGE INTO
- UPDATE … FROM
? MERGE handles insert, update and delete in a single atomic operation against the target table.
```
