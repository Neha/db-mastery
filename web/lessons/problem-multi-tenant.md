## The isolation question

SaaS almost always means **many customers in one product**. The data question is how hard it is for tenant A to see tenant B, and how much you pay for that guarantee. The usual names are **pool**, **bridge** and **silo**. They are a spectrum, not a religion: large estates run a pool for the long tail and silo the whales.

![Pool shares rows, bridge shares a database but not a schema, silo gives each tenant a database.](diagrams/problem-multi-tenant-isolation-models.svg)

| Model | What is shared | Isolation | Cost and ops |
|---|---|---|---|
| **Pool** | Cluster, database, schema, tables. `tenant_id` on every row | Weakest unless the database enforces it | Cheapest. One migration, one backup, one connection pool |
| **Bridge** | Cluster and database. **Schema per tenant** | Medium: a missing `search_path` is still a bug, but not a `WHERE` | Migrate every schema. Catalog bloat as tenant count grows |
| **Silo** | Often only the platform. **Database (or cluster) per tenant** | Strongest. No shared rows | Many backups, upgrades, and connection routes. Operators help |

> **Key idea:** Isolation you enforce only in application `WHERE` clauses is a future incident. Isolation the database enforces (RLS, separate schemas, separate databases, separate keys) survives a forgotten predicate.

This is a [modelling](#/lesson/data-modeling) decision first. `tenant_id` is a fact about almost every entity in a pool. If you skip it on a child table, joins invent cross-tenant rows.

## Pool: shared tables and tenant_id

Every tenant's rows live together. Every query, unique index and foreign key must include `tenant_id` (or an equivalent). Unique `email` becomes `UNIQUE (tenant_id, email)` unless emails are truly global.

The failure mode is obvious and common:

```sql
-- intended
SELECT * FROM orders WHERE tenant_id = $1 AND id = $2;

-- shipped
SELECT * FROM orders WHERE id = $2;
```

That is not a bug. It is a **security incident**: cross-tenant disclosure. Treat it like a leaked backup. Contain, audit, notify.

![A query without a tenant predicate returns every tenant; RLS or a forced tenant_id cannot.](diagrams/problem-multi-tenant-forgotten-where.svg)

### Row-level security (Postgres)

Postgres can refuse to return (or write) rows that do not match a policy, even if the SQL forgot `tenant_id`.

```sql
ALTER TABLE orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE orders FORCE ROW LEVEL SECURITY;  -- table owner is otherwise exempt

CREATE POLICY tenant_isolation ON orders
  USING (tenant_id = current_setting('app.tenant_id')::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);
```

The application does this on every request, inside the transaction:

```sql
SELECT set_config('app.tenant_id', $1, true);  -- true = local to this transaction
```

RLS is not magic. Superusers and roles with `BYPASSRLS` walk through it. A connection pool that reuses sessions must **clear** `app.tenant_id` or you leak the previous tenant. Migrations and analytics need an explicit, audited bypass — not a forgotten superuser.

MongoDB has no equivalent built-in RLS. You filter in the driver, use a separate database per tenant, or encrypt fields so another tenant's document is unreadable. Do not pretend a convention in the ORM is the same as a database policy.

## Bridge: schema per tenant

One database, `tenant_a.orders`, `tenant_b.orders`. Connection setup is `SET search_path TO tenant_a, public`. Isolation is better than a pool: a query without a schema qualifier still has to pick a path, and a `SELECT * FROM tenant_b.orders` is an obvious cross-tenant read in review.

Cost: every DDL runs N times. Tools that assume one `public` schema fight you. Postgres catalogs and `pg_stat` get noisy at thousands of schemas. Backup is still one cluster, so a restore still contains everyone — erasure and ransomware response stay coupled.

## Silo: database per tenant

A dedicated database (sometimes a dedicated cluster) per tenant. Forgotten `WHERE` cannot return another tenant's rows because they are not there. Per-tenant upgrades, limits, encryption keys and data-residency ("this customer stays in Frankfurt") become ordinary operations.

Cost is operational. You need automated provisioning, migrations, backup and failover for hundreds or thousands of databases. That is exactly the case [operators on Kubernetes](#/lesson/databases-on-kubernetes) are good at: many small Postgres instances, each a Custom Resource, backups to object storage. It is a poor fit for a single giant write-hot cluster you already struggle to fail over.

A common production shape: **pool for the many, silo for the few**. Move a noisy or regulated tenant out; do not rewrite the product.

## Noisy neighbours and tenant sharding

In a pool, one tenant's report or bulk import eats CPU, `work_mem`, WAL and cache. Everyone else slows down. Mitigations, cheapest first:

- `statement_timeout` and per-role `work_mem`
- separate connection pools or queueing for heavy jobs
- put large tenants on their own shard or silo
- only then: more hardware

**Tenant sharding** puts each tenant on one shard (`shard = hash(tenant_id)`). Queries that omit `tenant_id` cannot be routed — which is a gift, because they fail closed. A tenant that outgrows a shard becomes a silo: a dedicated shard with one value.

Cross-tenant reporting (the vendor's own admin view, or a lakehouse) must go through an explicit, audited path, not through the OLTP role the app uses.

## Per-tenant encryption keys

Give each tenant a DEK, wrap it with a KMS KEK, as in [envelope encryption](#/lesson/problem-passwords-secrets). Benefits:

- a dumped pool is not uniformly readable
- destroying the DEK **crypto-shreds** that tenant, which is the realistic answer to "delete us" when backups still exist
- residency and bring-your-own-key conversations have a concrete mechanism

You still need `tenant_id` (or a silo). Encryption without a query constraint does not stop the application returning the wrong ciphertext to the wrong caller.

## When each model wins

| Choose | When |
|---|---|
| **Pool + RLS** | Hundreds to millions of small tenants, identical schema, cost-sensitive, you can set `app.tenant_id` on every transaction |
| **Bridge** | Tens to low thousands of tenants, some schema drift, one ops team, isolation stronger than a `WHERE` |
| **Silo** | Regulated customers, contractual data residency, noisy whales, or tenants who need their own upgrade cadence |
| **Pool + shard + a few silos** | The usual mature shape |

> **SA angle:** Customers ask "is it multi-tenant-safe?" Draw the three boxes, then ask where a forgotten predicate lands. If the answer is "another customer's PII", the design is not done. Sell RLS, separate keys, or a silo — not a promise that the ORM will remember.

> **Interview:** "Design multi-tenant SaaS data." Start with the isolation table, pick pool-plus-RLS as the default, show `tenant_id` on every key, then explain which tenant you would silo and why. Mention that a missing `WHERE tenant_id` is an incident, not a ticket.

```quiz
Q: In a pooled model, a query omits WHERE tenant_id = …. What is that?
+ A security incident: cross-tenant data disclosure
- A performance bug
- Fine if the ORM usually adds it
- Fine because unique IDs never collide
? Application conventions fail. Treat a missing tenant predicate like a leak.

Q: What does FORCE ROW LEVEL SECURITY do in Postgres?
- Encrypts each row with a tenant key
- Moves each tenant into its own schema
- Enables backups per tenant
+ Applies RLS to the table owner, who would otherwise bypass policies
? Table owners bypass RLS unless you force it. Superusers and BYPASSRLS still can.

Q: When does database-per-tenant (silo) usually win?
- Always; pool is obsolete
- You have more than ten rows
+ A regulated or very large tenant needs isolation, residency or its own noisy-neighbour boundary
- You want the cheapest possible design
? Silo is an ops cost you pay for isolation. Most tenants can stay in a pool.

Q: Why put tenant_id in the shard key?
- It makes unique emails global
- Postgres requires it for RLS
- It removes the need for backups
+ Each tenant lives on one shard, so a query without tenant_id cannot be routed and fails closed
? Routing by tenant is both a scale and a safety property.

Q: How do per-tenant DEKs help a deletion request?
- They make RLS unnecessary
- They shrink the WAL
+ Destroying the tenant's DEK crypto-shreds live data even when backup bytes remain for a while
- They let you store passwords with AES
? Keys are part of erasure. You still need a tombstone path for replicas and derived stores.
```
