## The bugs that look like heisenbugs

Overselling, double booking and double charges are usually **lost updates**, **missing unique constraints**, or **retries without idempotency**. They reproduce under load and vanish in a debugger because the debugger serialises the two requests.

The contract is in [Transactions and ACID](#/lesson/transactions-acid). How engines detect conflicts is in [MVCC and concurrency](#/lesson/mvcc-concurrency). Cross-service retries and the outbox are in [Consensus and distributed transactions](#/lesson/consensus-transactions). This lesson is the playbook: the race, the SQL, the MongoDB equivalent, and what you say when the interviewer hands you a whiteboard.

> **Key idea:** If the invariant is "never more sold than stock", the database must see a predicate or a lock that two concurrent transactions cannot both pass. Application-level read-modify-write on a snapshot does not provide that.

## Lost update: the last item sold twice

Two checkouts read `stock = 1`, both decide they can sell, both write `stock = 0`. Read Committed allows this: each `UPDATE` is a new snapshot of a row the other has not yet committed, or worse, each write blindly stores a number it computed in the application.

![Two checkouts both read stock = 1 and both commit a sale. There is no write-write conflict if each stores a precomputed 0. The fix is an atomic predicate or a row lock held across the decision.](diagrams/problem-concurrency-bugs-oversell.svg)

The [Postgres lab](#/lesson/lab-postgres) (`go run ./labs/postgres anomaly`) shows nine of ten deposits vanishing with naïve read-modify-write. Three fixes, in the order you should prefer them:

```sql
-- Lab: postgres://dbm:dbm@localhost:5442/dbm
-- 1. Atomic predicate. Best when the rule fits in one statement.
UPDATE products
SET    stock = stock - 1
WHERE  id = 1 AND stock >= 1;
-- row count 0 → sold out. No lost update, no extra isolation.

-- 2. Pessimistic: lock the row while you think (multi-step checkout).
BEGIN;
SELECT stock FROM products WHERE id = 1 FOR UPDATE;
-- application: tax, warehouse, payment intent …
UPDATE products SET stock = stock - 1 WHERE id = 1 AND stock >= 1;
COMMIT;

-- 3. Optimistic: version column. Retry when someone else won.
UPDATE products
SET    stock = 9, version = version + 1
WHERE  id = 1 AND version = 7;
-- 0 rows → reread and retry
```

MongoDB is the same idea with a conditional update. `matchedCount === 0` is "sold out", not an error:

```js
const res = db.inventory.updateOne(
  { _id: sku, available: { $gte: qty } },
  { $inc: { available: -qty, reserved: qty } }
)
// res.matchedCount === 0 → do not create the order
```

For a multi-item cart, wrap the reservations and the order insert in a **transaction** (SQL or MongoDB multi-document), or use a saga: reserve each SKU, create the order, confirm payment, and release reservations on failure. Reservations need a TTL so abandoned checkouts give the stock back.

> **Remember:** `SERIALIZABLE` also fixes this, via a serialization failure (`40001`) that you must retry. Prefer the atomic `UPDATE … WHERE stock >= 1` when one row holds the invariant — it is faster and does not abort.

## Double booking

Two reservations that must not overlap (a room, a seat, a timeslot) are a **constraint** problem, not a "remember to check" problem.

```sql
-- Exclusion constraint: no two bookings for the same room with overlapping time.
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS bookings (
  room_id  INT NOT NULL,
  during   TSTZRANGE NOT NULL,
  EXCLUDE USING gist (room_id WITH =, during WITH &&)
);
```

A unique constraint is the same idea for discrete seats: `UNIQUE (event_id, seat_id)` and treat a unique violation as "already taken". Do not `SELECT` the seat and then `INSERT` without the constraint — two transactions both see "free".

MongoDB: a unique index on `{ eventId: 1, seatId: 1 }` and insert the reservation document. Duplicate key (`11000`) is the conflict. For ranges you typically discretise (one document per slot) so a unique index can enforce it; overlapping-range exclusion is a Postgres strength.

> **SA angle:** When a customer says "we'll check availability in the app", ask what happens when two requests pass the check in the same millisecond. The answer you want is a unique or exclusion constraint, not a code review.

## Double charges and idempotency keys

Clients retry. Load balancers retry. Message consumers retry. At-least-once delivery is the common case. If `POST /charge` is not idempotent, a timeout followed by a retry bills the card twice — and both rows look legitimate.

![Without an idempotency key, a retried POST inserts a second charge. A unique key turns the retry into a replay of the original result.](diagrams/problem-concurrency-bugs-idempotency.svg)

```sql
CREATE TABLE IF NOT EXISTS payment_intents (
  idempotency_key  TEXT PRIMARY KEY,
  customer_id      BIGINT NOT NULL,
  amount_cents     INT    NOT NULL,
  charge_id        TEXT,
  status           TEXT   NOT NULL,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One transaction: claim the key, then charge.
BEGIN;
INSERT INTO payment_intents (idempotency_key, customer_id, amount_cents, status)
VALUES ('abc-9', 42, 2000, 'pending')
ON CONFLICT (idempotency_key) DO NOTHING;

-- 0 rows inserted → this key already exists: SELECT and return that result.
-- 1 row inserted  → you own the work; call the processor, then:
UPDATE payment_intents
SET    status = 'charged', charge_id = 'ch_123'
WHERE  idempotency_key = 'abc-9';
COMMIT;
```

The unique key is the lock. A second worker that loses the insert **must not** charge; it returns the stored response (or waits if status is still `pending`). Store the response body you need to replay.

MongoDB:

```js
try {
  db.payment_intents.insertOne({
    _id: idempotencyKey, customerId, amountCents, status: "pending"
  })
} catch (e) {
  if (e.code === 11000) {
    return db.payment_intents.findOne({ _id: idempotencyKey }) // replay
  }
  throw e
}
```

Stripe-style APIs use an `Idempotency-Key` header for exactly this. Your API should too, for any POST that creates money movement, an order, or a reservation.

> **Pitfall:** Hashing the request body as the key looks clever until a retry changes a timestamp field and mints a new key. The client must send a stable key for the *intent*, not a hash of the payload.

## Write skew: both doctors go off call

Lost update is two writers of the **same** row. **Write skew** is two writers of **different** rows who each read a snapshot that made their write legal, and together they break an invariant. The textbook case: at least one doctor must stay on call.

```text
on_call = { alice: true, bob: true }

T1: count(*) WHERE on_call  →  2   then  alice = false
T2: count(*) WHERE on_call  →  2   then  bob   = false
Both commit under snapshot isolation  →  0 doctors
```

Postgres `REPEATABLE READ` is snapshot isolation and **allows** this. `SERIALIZABLE` (SSI) detects the read/write cycle and aborts one transaction with `40001`. The application retries; the retry sees one doctor already off and refuses.

Other fixes, if you do not want SSI:

- `SELECT … FOR UPDATE` on **every row your decision depends on** (both doctors), so the second transaction waits and then re-evaluates.
- Materialise the conflict: a single `on_call_count` row that both transactions update. Now it is a lost-update problem, which row locking already handles.

MongoDB multi-document transactions use snapshot isolation. They will not catch write skew for you. Use an atomic update on a single "roster" document, or a unique constraint that encodes the invariant.

Details and the SSI dependency picture are in [MVCC](#/lesson/mvcc-concurrency). Isolation levels are in [Transactions](#/lesson/transactions-acid).

## Deadlocks

Two transactions take locks in opposite order and wait forever. The database detects the cycle and aborts one (`40P01` in Postgres, `112` TransientTransactionError-class in MongoDB). Your job is to **retry the loser** and to take locks in a **stable order** so cycles are rare.

```sql
-- Session A                                          -- Session B
BEGIN;                                                BEGIN;
UPDATE products SET stock = stock - 1 WHERE id = 1;   UPDATE orders SET status = 'paid' WHERE id = 1;
UPDATE orders SET status = 'paid' WHERE id = 1;       UPDATE products SET stock = stock - 1 WHERE id = 1;
COMMIT;                                               COMMIT;
-- One session errors: deadlock detected
```

```go
// Same retry gate as serialization failures. The lab's withRetry handles both.
// 40001 = serialization_failure, 40P01 = deadlock_detected
```

**Prevent.** Always lock rows in the same order (e.g. sort product ids before updating). Keep transactions short — no HTTP calls inside `BEGIN`. Prefer one atomic statement to two locked reads.

> **Gotcha:** Killing the victim is correct behaviour, not an outage, *if* the client retries. An ORM that surfaces `40P01` as a 500 is the bug.

## Counters

`views = views + 1` in the application is a lost update. Use the engine's atomic increment:

```sql
UPDATE posts SET views = views + 1 WHERE id = $1;
```

```js
db.posts.updateOne({ _id: id }, { $inc: { views: 1 } })
```

Hot counters (a global "orders today", a flash-sale remaining-stock) become the hotspot in the [previous lesson](#/lesson/problem-scaling-hotspots): salt them, or move the increment to Redis (`INCR`) if approximate / eventually consistent is acceptable, and reconcile.

Do not put a counter that must be exact on a cached value without a write-through story.

## The outbox: don't dual-write

Checkout often has to do two things: decrement stock and publish `OrderPlaced`. Two writes to two systems is a double-charge / lost-event factory: the process can die after the `COMMIT` and before the Kafka produce, or the other way around.

The **outbox** puts the event row in the **same** database transaction as the business write. A CDC process (Debezium, logical decoding) publishes committed outbox rows. Consumers are idempotent — they will see the event at least once.

```sql
BEGIN;
UPDATE products SET stock = stock - 1 WHERE id = $1 AND stock >= 1;
INSERT INTO orders (customer_id, status) VALUES ($2, 'pending') RETURNING id;
INSERT INTO outbox (aggregate_id, event_type, payload)
VALUES ($3, 'OrderPlaced', $4);
COMMIT;
-- A publisher reads committed outbox rows; it does not run in this transaction.
```

That is the pattern; the machinery is in [Consensus and distributed transactions](#/lesson/consensus-transactions) and [Kafka, Debezium and CDC](#/lesson/kafka-debezium-cdc).

## Scenarios

### Problem: Black Friday oversold a SKU of one

> **Real world:** "We had one unit left. Two customers got an order confirmation. Warehouse says we cannot fulfil one of them. The inventory table has stock = 0. Isolation is the default."

**Symptoms.** Two `orders` rows, `stock = 0` (or −1 if there was no check). No error in the logs. Reproduces with two concurrent `curl`s.

**Diagnose.** Read the checkout code. If it is `SELECT stock` then later `UPDATE … SET stock = $n`, you have a lost update. Confirm with the lab `anomaly` command.

**Root causes.** Read-modify-write in the application at Read Committed. No `WHERE stock >= 1`. No unique reservation row.

**Fix.** Immediate: stop selling the SKU (`stock = 0` and a sold-out flag). Durable: atomic `UPDATE … WHERE stock >= qty` (or MongoDB `$inc` with `$gte`). For multi-item carts, a transaction around the reservations. Refund / cancel the oversold order as the business repair.

**Prevent.** A constraint `CHECK (stock >= 0)` so a bad write fails loudly. A load test that hits *one* SKU. Code review that bans "read a number, write a number" on inventory.

> **Model answer — what to say in the interview:** This is a lost update, not a mysterious race. Two transactions read stock = 1 and both write a computed zero. I would change the write to UPDATE products SET stock = stock - qty WHERE id = $1 AND stock >= qty and treat a zero row count as sold out. In MongoDB I would use updateOne with available: { $gte: qty } and $inc. I would add CHECK (stock >= 0) so a future bug cannot go negative. SERIALIZABLE would also work but I would not pay SSI aborts for a one-row invariant.

### Problem: A webhook charged the customer twice

> **Real world:** "The payment provider timed out. We retried the webhook. The customer has two charge rows and two bank lines. Each request looks valid."

**Symptoms.** Two inserts, two provider ids or the same provider id twice. Happens on retries, not on the happy path.

**Diagnose.** Is there a unique idempotency key for the *intent*? Did the retry use a new key? Did the consumer treat "process this event" as "insert a charge"?

**Root causes.** Missing unique key. Key derived from a payload that changed. Consumer not idempotent.

**Fix.** Immediate: refund the duplicate; freeze the retry. Durable: `UNIQUE (idempotency_key)` (or `_id` = key in MongoDB); insert-first, charge-second; replay the stored result on conflict. If the provider also supports idempotency keys, send the same one.

**Prevent.** Every POST that moves money takes a client-generated key. Consumers key on event id.

> **Model answer — what to say in the interview:** Retries are normal; double charges are a missing unique constraint. I insert a payment_intents row keyed by the idempotency key in the same transaction that I am willing to charge. A conflict means I return the original result and I do not call the processor again. I would also make the webhook consumer idempotent on the provider's event id, because they will retry too.

### Problem: The on-call roster went to zero

> **Real world:** "Two doctors went off call at the same moment. Each UI showed the other still on. We use REPEATABLE READ because we read about phantoms."

**Symptoms.** Invariant violated, no error, two updates of *different* rows. `REPEATABLE READ` / snapshot isolation.

**Diagnose.** This is write skew. `EXPLAIN` will not help. Draw the two snapshots.

**Root causes.** Snapshot isolation does not detect read/write cycles on disjoint rows. The team picked RR thinking it was "stricter, so safer" without naming write skew.

**Fix.** `SERIALIZABLE` plus retry on `40001`, or `SELECT … FOR UPDATE` on the roster rows, or a single count row both must update.

**Prevent.** List invariants that span rows and pick a mechanism per invariant. Don't raise isolation globally.

> **Model answer — what to say in the interview:** Write skew: both transactions read a snapshot where two doctors were on call and each updated a different row. Snapshot isolation allows that; SERIALIZABLE SSI will abort one with 40001 and the retry sees the new count. I could also lock both roster rows with FOR UPDATE, or keep an on_call_count row so the writes collide. I would not claim REPEATABLE READ is serializable in Postgres — it is SI.

### Problem: Deadlocks every few minutes under load

> **Real world:** "We started seeing deadlock_detected in checkout. Throughput is fine. About one in a thousand carts fails with a 500."

**Symptoms.** `40P01` in logs. Two tables (orders and products, or two product rows) locked in opposite orders.

**Diagnose.** Postgres logs the two queries. Sort the product ids in the application and compare. Check that a payment HTTP call is not inside the transaction (that stretches lock hold time and makes deadlocks more likely).

**Root causes.** Lock order. Long transactions. An ORM that updates associations in hash-map order.

**Fix.** Immediate: retry on `40P01` (and stop returning 500). Durable: sort ids; decrement stock in id order; keep the transaction to the writes.

**Prevent.** The same retry helper as SSI. Tracing on transaction duration.

> **Model answer — what to say in the interview:** A deadlock is a cycle in the lock graph; the database will abort one transaction and it should retry. I would take row locks in a stable order — sort SKU ids — and keep HTTP calls out of the transaction so locks are held for milliseconds. I would treat 40P01 like 40001: retryable, not a user-facing error.

```quiz
Q: Two checkouts read stock = 1 and both write stock = 0. Which statement prevents the oversell without raising the isolation level?
- SELECT stock FROM products WHERE id = 1
- SET TRANSACTION ISOLATION LEVEL READ UNCOMMITTED
- DROP INDEX
+ UPDATE products SET stock = stock - 1 WHERE id = 1 AND stock >= 1
? The predicate is evaluated on the latest committed row under row locking. A zero row count means sold out. A precomputed write of 0 does not conflict.

Q: Why does REPEATABLE READ in Postgres not save the "at least one doctor on call" invariant?
+ It is snapshot isolation: two transactions can write different rows that each looked safe on the same snapshot
- REPEATABLE READ disables UPDATE
- Doctors cannot be stored in Postgres
- It converts all writes into table locks
? Write skew is the anomaly SI allows. SERIALIZABLE (SSI), explicit FOR UPDATE on the read set, or a single conflict row fixes it.

Q: A payment POST times out and the client retries. What stops a second charge?
- A longer client timeout
+ A unique idempotency key inserted before the charge, with a replay of the stored result on conflict
- SERIALIZABLE isolation on a table without a unique key
- Caching the card number
? Isolation does not collapse two intents into one. A unique key for the intent does.

Q: MongoDB inventory reservation that must not oversell looks like:
- find() then update() in the application with no predicate
+ updateOne({ _id, available: { $gte: qty } }, { $inc: { available: -qty } }) and check matchedCount
- $set: { available: 0 } on every checkout
- drop the collection
? The filter is the constraint. matchedCount === 0 is sold out. A find-then-update is the lost-update bug.

Q: The database reports deadlock_detected (SQLSTATE 40P01). What should the application do?
- Crash the process
- Retry forever with no backoff
+ Retry the whole transaction (same helper as 40001), and take locks in a stable order to make cycles rare
- SET isolation to READ UNCOMMITTED
? The engine aborted the victim to break the cycle. A retry usually succeeds. Stable lock order reduces how often it happens.

Q: Why put an OrderPlaced event in an outbox table in the same transaction as the stock update?
+ Then the event exists if and only if the business write committed, so a CDC publisher cannot lose or invent it
- So Kafka can take a row lock
- Outbox rows skip the WAL
- It avoids the need for idempotent consumers
? Dual writes to a database and a bus can diverge on a crash. The outbox makes publication a consequence of commit. Consumers still need to be idempotent (at-least-once).
```
