## The relational model & SQL essentials

In 1970 Edgar Codd proposed storing data as **relations** (tables) and querying it **declaratively**: you describe *what* you want and the database works out *how*. Fifty years later SQL is still the most valuable data skill you can have. Databricks SQL, Postgres, Snowflake, BigQuery and even MongoDB's Atlas SQL interface all speak it.

> **Tip:** Think of SQL like CSS selectors or React's declarative rendering. You describe the result, and an engine (the *optimiser*) decides the steps.

## Vocabulary

| Term | Meaning | Front-end analogy |
|---|---|---|
| Table (relation) | A set of rows with the same columns | An array of objects with the same shape |
| Row (tuple) | One record | One object |
| Column (attribute) | A named, typed field | A property with a TypeScript type |
| Primary key (PK) | Column(s) uniquely identifying a row | A React `key` |
| Foreign key (FK) | A column referencing another table's PK | An ID pointing to another object |
| Schema | The set of tables, types and constraints | Your TypeScript interfaces |

## The sample schema used throughout this portal

```sql
create table customers (
  id         bigserial primary key,
  email      text not null unique,
  name       text not null,
  country    text not null,
  created_at timestamptz not null default now()
);

create table products (
  id          bigserial primary key,
  sku         text not null unique,
  name        text not null,
  category    text not null,
  price_cents int  not null check (price_cents >= 0),
  stock       int  not null default 0 check (stock >= 0)
);

create table orders (
  id          bigserial primary key,
  customer_id bigint not null references customers(id),
  status      text not null default 'pending',
  created_at  timestamptz not null default now()
);

create table order_items (
  order_id         bigint not null references orders(id) on delete cascade,
  product_id       bigint not null references products(id),
  qty              int not null check (qty > 0),
  unit_price_cents int not null,
  primary key (order_id, product_id)
);
```

Notice the **constraints** (`not null`, `unique`, `check`, `references`). They are business rules the database enforces for you, whatever bugs the application code has. This is one of the strongest arguments for relational databases.

## CRUD

```sql
-- Create
insert into customers (email, name, country)
values ('ada@example.com', 'Ada Lovelace', 'UK')
returning id;

-- Read
select id, name from customers where country = 'UK' order by created_at desc limit 10;

-- Update
update products set stock = stock - 1 where id = 42 and stock > 0;

-- Delete
delete from orders where status = 'cancelled' and created_at < now() - interval '1 year';
```

> **Watch out:** An `update` or `delete` without a `where` clause touches every row. Make it a habit to run the matching `select` first.

## Joins: combining tables

```sql
-- Orders with customer names (only matching rows)
select o.id, c.name, o.status
from orders o
join customers c on c.id = o.customer_id;

-- All customers, including those with no orders (left join keeps the left side)
select c.name, count(o.id) as order_count
from customers c
left join orders o on o.customer_id = c.id
group by c.name;
```

| Join | Returns |
|---|---|
| `inner join` | Only rows with a match on both sides |
| `left join` | All left rows; nulls where the right has no match |
| `right join` | Mirror of left join (rarely used) |
| `full outer join` | All rows from both sides |
| `cross join` | Every combination (Cartesian product) |

![An inner join keeps only matching pairs; a left join keeps every customer and fills NULLs. A WHERE filter on the right-hand table treats those NULLs as unknown and silently becomes an inner join.](diagrams/relational-model-sql-joins.svg)

> **Interview:** A `LEFT JOIN` is supposed to keep every customer. The result still drops customers with no orders. What happened?

> **Model answer:** A predicate on the right-hand table in `WHERE` treats `NULL` as unknown, so unmatched left rows disappear and the outer join becomes an inner join. Put right-side filters in the `ON` clause (`LEFT JOIN orders o ON o.customer_id = c.id AND o.status = 'paid'`), or allow the unmatched case explicitly (`WHERE o.id IS NULL OR o.status = 'paid'`). Filtering the left table in `WHERE` is fine.

## Aggregation

```sql
-- Revenue per category, only categories above £1,000
select p.category,
       sum(oi.qty * oi.unit_price_cents) / 100.0 as revenue_gbp,
       count(distinct oi.order_id)              as orders
from order_items oi
join products p on p.id = oi.product_id
group by p.category
having sum(oi.qty * oi.unit_price_cents) > 100000
order by revenue_gbp desc;
```

**Logical order of evaluation**, which explains most SQL errors:

```text
FROM / JOIN → WHERE → GROUP BY → HAVING → SELECT → DISTINCT → ORDER BY → LIMIT
```

This is why you can't use a `select` alias inside `where`: when `where` runs, the alias doesn't exist yet. `where` filters rows *before* grouping, and `having` filters groups *after*.

## NULL is not a value

`NULL` means "unknown". Any comparison with it returns unknown, and unknown is treated as not true:

```sql
select * from customers where country = null;   -- returns nothing!
select * from customers where country is null;  -- correct
```

## Subqueries and EXISTS

```sql
-- Customers who have never ordered
select c.*
from customers c
where not exists (select 1 from orders o where o.customer_id = c.id);
```

> **SA angle:** Customers migrating from relational to MongoDB often ask "how do I do joins?" The honest answer is that MongoDB has `$lookup`, but with a good document design you usually don't need the join: data accessed together is stored together. You can only make that argument credibly if you understand joins deeply first.

> **Exercise:** Start the lab databases (`docker compose up -d` in `labs/`), run `go run ./labs/postgres` once to create and seed the schema, then connect with `psql postgres://dbm:dbm@localhost:5432/dbm` and write queries for:
> 1. The top 5 customers by total spend.
> 2. Products that have never been ordered.
> 3. Each country's average order value.

```quiz
Q: Why does `SELECT price * qty AS total FROM t WHERE total > 100` fail in most databases?
- `total` is a reserved word
+ WHERE is evaluated before SELECT, so the alias doesn't exist yet
- You can't multiply columns
? Logical evaluation order is FROM → WHERE → GROUP BY → HAVING → SELECT. Repeat the expression or use a subquery/CTE.

Q: Which join returns all customers, even those with zero orders?
- INNER JOIN customers to orders
+ customers LEFT JOIN orders
- CROSS JOIN
? A LEFT JOIN keeps every row from the left table and fills in NULLs where there is no match.

Q: What does `WHERE country = NULL` return?
+ No rows
- Rows where country is null
- An error in every database
? Comparisons with NULL yield UNKNOWN, which is not TRUE. Use `IS NULL`.

Q: What's the difference between WHERE and HAVING?
+ WHERE filters rows before grouping; HAVING filters groups after aggregation
- They are synonyms
- HAVING is faster
? Use WHERE whenever you can: it reduces the rows that need to be grouped.
```
