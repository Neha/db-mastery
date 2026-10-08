## Transactions, ACID & isolation levels

A **transaction** groups several operations into one logical unit: *all of it happens, or none of it does.* Moving £100 from Alice to Bob is two updates, and the world must never see only one of them.

```sql
begin;
update accounts set balance = balance - 100 where id = 'alice';
update accounts set balance = balance + 100 where id = 'bob';
commit;   -- or rollback;
```

## ACID

| Letter | Guarantee | How databases implement it |
|---|---|---|
| **A**tomicity | All or nothing | Undo information / WAL; rollback on failure |
| **C**onsistency | Constraints hold before and after | Constraints, FKs, checks (plus your app logic) |
| **I**solation | Concurrent transactions don't interfere (to a chosen degree) | Locks, MVCC, snapshot isolation |
| **D**urability | Once committed, it survives crashes | WAL flushed with `fsync`, replication |

> **Key idea:** "C" is the odd one out. It's mostly the *application's* job; the database only enforces the rules you declare. The interesting engineering is in **A, I and D**.

## Concurrency anomalies

When transactions run at the same time, these things can go wrong:

| Anomaly | What happens | Example |
|---|---|---|
| **Dirty read** | Read another txn's uncommitted data | You see Alice's -£100 before her txn rolls back |
| **Non-repeatable read** | Same row read twice gives different values | Balance is £500, then £400 within your txn |
| **Phantom read** | Same query returns new/missing rows | `count(*)` changes between two reads |
| **Lost update** | Two read-modify-write cycles; one overwrites the other | Two clerks both set stock = 10 - 1 = 9 (should be 8) |
| **Write skew** | Two txns read overlapping data and write disjoint rows, breaking an invariant | Two doctors both go off call because each saw the other still on call |

![Dirty reads see uncommitted data; lost updates overwrite a concurrent increment; write skew breaks a cross-row invariant.](diagrams/transactions-acid-anomalies.svg)

## Isolation levels (SQL standard)

| Level | Dirty read | Non-repeatable | Phantom | Notes |
|---|---|---|---|---|
| Read uncommitted | possible | possible | possible | Postgres treats it as Read Committed |
| **Read committed** | ✗ | possible | possible | **Postgres & Oracle default** |
| **Repeatable read** | ✗ | ✗ | possible* | **MySQL InnoDB default**; *Postgres RR is snapshot isolation and also prevents phantoms |
| **Serializable** | ✗ | ✗ | ✗ | As if transactions ran one at a time. Postgres uses SSI (serializable snapshot isolation) |

**Snapshot isolation (SI)** means each transaction sees a consistent snapshot as of when it started. It prevents most anomalies, but **not write skew**. Only true serializable prevents write skew.

MongoDB multi-document transactions use **snapshot isolation**, combined with read concern `"snapshot"` and write concern `"majority"`.

## Fixing lost updates — three techniques

```sql
-- 1. Atomic update (best when possible)
update products set stock = stock - 1 where id = 42 and stock > 0;

-- 2. Pessimistic locking: lock the row while you think
begin;
select stock from products where id = 42 for update;
-- application logic …
update products set stock = 9 where id = 42;
commit;

-- 3. Optimistic concurrency: version column, retry on conflict
update products set stock = 9, version = version + 1
where id = 42 and version = 7;    -- 0 rows updated → someone else won, retry
```

Optimistic concurrency will feel familiar from front-end work: it's the same pattern as ETags and `If-Match` headers in REST APIs.

## Serializable needs retries

At serializable (and sometimes repeatable read), the database will **abort** one of two conflicting transactions with a *serialization failure* (Postgres SQLSTATE `40001`). Your application must catch it and **retry the whole transaction**. MongoDB labels these errors `TransientTransactionError`, and its drivers' `WithTransaction` helpers retry automatically.

```go
// sketch — the full version is in the Postgres lab
for attempt := 0; attempt < 5; attempt++ {
    err := runTransfer(ctx, db)
    if isSerializationFailure(err) { continue } // 40001 → retry
    return err
}
```

## Durability in practice

"Committed" means different things depending on configuration:

- Postgres: `synchronous_commit = on` waits for the local WAL fsync. With synchronous replicas configured, it can also wait for a standby.
- MongoDB: **write concern** `w: 1` (primary only) vs `w: "majority"` (a majority of replica set members), plus `j: true` (journaled).
- The trade-off is always **latency vs durability**. Know where your customer sits.

> **SA angle:** A classic MongoDB objection is "MongoDB isn't ACID." It has had multi-document ACID transactions since 4.0 (2018) and across shards since 4.2, and single-document operations have always been atomic. The better point: with good document design (embedding), most operations touch **one document**, so you rarely need a multi-document transaction.

> **Interview:** Two checkouts of the last item both return success. Stock is 0 afterwards, with two orders. What happened?

> **Model answer:** Lost update: both read stock = 1 and both wrote a computed 0. Fix in the database: `UPDATE products SET stock = stock - 1 WHERE id = $1 AND stock >= 1` and treat zero rows as sold out, or `SELECT … FOR UPDATE` in the same transaction as the order insert. `SERIALIZABLE` also works, but you pay abort-and-retry. Do not fix it only in Redis unless Redis is the authority.

```quiz
Q: Two transactions each read stock=10, compute 9, and write 9. Final stock is 9 instead of 8. Which anomaly is this?
- Dirty read
- Phantom read
+ Lost update
? Concurrent read-modify-write cycles overwrite each other. Fix with an atomic update, SELECT … FOR UPDATE, or optimistic versioning.

Q: What is the default isolation level in PostgreSQL?
+ Read committed
- Repeatable read
- Serializable
? Each statement sees data committed before that statement began.

Q: Which anomaly is NOT prevented by snapshot isolation?
- Dirty reads
- Non-repeatable reads
+ Write skew
? Two transactions can read the same snapshot and write disjoint rows, breaking a cross-row invariant. Serializable (e.g. Postgres SSI) prevents it.

Q: Under SERIALIZABLE, a transaction fails with SQLSTATE 40001. What should the app do?
- Report a bug
+ Retry the entire transaction
- Lower the isolation level permanently
? Serialization failures are expected under serializable; the contract is "retry".

Q: In MongoDB, which write concern ensures a write survives a primary failover?
- w: 0
- w: 1
+ w: "majority"
? Writes acknowledged by a majority can't be rolled back by a subsequent election.
```
