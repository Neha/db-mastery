## Why Go for data work?

Go is the language of cloud infrastructure: Kubernetes, etcd, Docker, Prometheus, CockroachDB, Vitess, TiDB, InfluxDB and most operators are written in it. It's also first-class for backend services that talk to databases. For an SA, Go fluency signals you can read the source of the platforms you sell, and it's especially valued at **Red Hat** (OpenShift and the operator ecosystem are Go).

Coming from JavaScript and TypeScript:

| JS/TS | Go |
|---|---|
| `async/await`, Promises | Goroutines (`go f()`) and channels; blocking calls are fine |
| `try/catch` | Explicit `if err != nil { return err }` |
| `interface` (structural) | `interface` (also structural, implicitly satisfied) |
| `AbortController` | `context.Context` (cancellation, deadlines) |
| npm | Go modules (`go get`, `go.mod`) |
| Jest | `go test` (built in) |
| JSON `JSON.parse` into objects | Struct tags: `json:"name"`, `bson:"name"`, `db:"name"` |

## Two ways to talk to Postgres

### 1. `database/sql` (standard library interface)

```go
import (
	"database/sql"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
)

db, err := sql.Open("pgx", "postgres://dbm:dbm@localhost:5442/dbm?sslmode=disable")
if err != nil { return err }
defer db.Close()

db.SetMaxOpenConns(20)            // cap concurrency at the database
db.SetMaxIdleConns(10)
db.SetConnMaxLifetime(30 * time.Minute)

var name string
err = db.QueryRowContext(ctx, `SELECT name FROM customers WHERE id = $1`, 42).Scan(&name)
if errors.Is(err, sql.ErrNoRows) { /* not found */ }
```

`sql.DB` is **not a connection**. It's a **pool**, safe for concurrent use. Create one at start-up and share it.

### 2. `pgx` native API (recommended for Postgres)

```go
import "github.com/jackc/pgx/v5/pgxpool"

pool, err := pgxpool.New(ctx, url)
defer pool.Close()

type Customer struct {
	ID      int64
	Name    string
	Country string
}
rows, _ := pool.Query(ctx, `SELECT id, name, country FROM customers WHERE country = $1`, "GB")
customers, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Customer])
```

pgx is faster, supports Postgres-specific types (JSONB, arrays, `COPY`, `LISTEN/NOTIFY`) and has generics helpers. Other popular tools are **sqlc** (generates type-safe Go from SQL, highly recommended), **GORM** (an ORM) and **sqlx**.

## Rule #1: always use placeholders

```go
// ❌ SQL injection: never build SQL with string formatting
q := fmt.Sprintf("SELECT * FROM users WHERE email = '%s'", email)

// ✅ Parameters are sent separately from the SQL text
pool.QueryRow(ctx, "SELECT * FROM users WHERE email = $1", email)
```

The MongoDB equivalent is to never build filters from raw user JSON. Construct `bson.M{"email": email}`, which blocks operator injection like `{"$ne": null}`.

## context.Context everywhere

Every database call takes a `ctx`. It carries **deadlines** and **cancellation**, so a slow query is abandoned when the HTTP client disconnects:

```go
func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	order, err := h.repo.FindOrder(ctx, r.PathValue("id"))
	// ...
}
```

## Transactions: the idiom

```go
err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{IsoLevel: pgx.Serializable}, func(tx pgx.Tx) error {
	// every statement uses tx, not pool
	if _, err := tx.Exec(ctx, `UPDATE accounts SET balance_cents = balance_cents - $1 WHERE id = $2`, amt, from); err != nil {
		return err // → automatic ROLLBACK
	}
	_, err := tx.Exec(ctx, `UPDATE accounts SET balance_cents = balance_cents + $1 WHERE id = $2`, amt, to)
	return err // nil → COMMIT
})
```

Handle **retryable errors** (Postgres SQLSTATE `40001` serialization failure, `40P01` deadlock) with a retry loop and backoff. The Postgres lab implements `withRetry`.

![On 40001 or 40P01 roll back and retry the whole transaction; other errors stop.](diagrams/go-database-fundamentals-retry-flow.svg)

## Talking to MongoDB

```go
import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

client, err := mongo.Connect(options.Client().ApplyURI(uri))
defer client.Disconnect(ctx)
orders := client.Database("shop").Collection("orders")

cur, err := orders.Find(ctx,
	bson.M{"status": "paid"},
	options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(10))
var results []Order
err = cur.All(ctx, &results)
```

- `mongo.Client` is also a **pool**: create one per process.
- `bson.D` is ordered (use it for sorts, pipelines and commands); `bson.M` is an unordered map (fine for filters).
- Retryable reads and writes are on by default. The driver retries once after a failover.

## Connection pooling: why it matters

| Symptom | Cause |
|---|---|
| "too many connections" errors | Each pod opens 100 connections × 50 pods |
| Latency spikes under load | Pool too small, so goroutines wait for a connection |
| DB CPU at 100% with low throughput | Pool too large; context switching and lock contention in the DB |

A rule of thumb for Postgres is `connections ≈ (CPU cores × 2) + effective spindles` **across all clients**. Put PgBouncer in front for many pods.

![Thousands of goroutines share a small pool; size it from Little's law, not from pod count.](diagrams/go-database-fundamentals-pool-sizing.svg)

## Testing database code

- **Unit tests** with interfaces and fakes for business logic.
- **Integration tests** against a real database using **testcontainers-go**, which starts Postgres or MongoDB in Docker per test run:

```go
func TestRepo(t *testing.T) {
	ctx := context.Background()
	pg, err := postgres.Run(ctx, "postgres:16", postgres.WithDatabase("test"),
		postgres.BasicWaitStrategies())
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { pg.Terminate(ctx) })
	url, _ := pg.ConnectionString(ctx, "sslmode=disable")
	// run migrations, then test the real SQL
}
```

- Migrations: **golang-migrate**, **goose** or **Atlas** (the schema tool, not MongoDB Atlas).

## Project structure for a data service

```text
service/
├── cmd/api/main.go           # wiring: config, pools, HTTP server
├── internal/order/
│   ├── order.go              # domain types + business rules
│   ├── repository.go         # interface: FindByID, Save …
│   ├── postgres.go           # implementation with pgx
│   └── mongo.go              # alternative implementation
├── migrations/
└── go.mod
```

Hiding the store behind an interface lets you demo **the same service on Postgres and MongoDB**. That's a great SA portfolio piece for explaining trade-offs.

> **Lab:** Next up: **Lab: PostgreSQL from Go** and **Lab: MongoDB from Go**. Both are runnable programs in `labs/` with real output to study.

> **Interview:** A reviewer sees `pgxpool.New` inside the HTTP handler. What's wrong, and what do you do instead?

> **Model answer:** A pool per request opens and tears down connections on every call: extra latency, `too many connections`, and leaked sockets. `sql.DB`, `pgxpool.Pool` and `mongo.Client` are concurrency-safe pools. Create one at start-up, inject it, and close it on shutdown. Size `MaxOpenConns` from the cluster-wide budget, not from what feels right in this service.

```quiz
Q: What is a *sql.DB (or pgxpool.Pool / mongo.Client) in Go?
- A single database connection
+ A concurrency-safe connection pool that should be created once and shared
- A transaction
- A query builder
? These are long-lived pools. Creating one per request is a classic performance bug.

Q: Why should every DB call take a context.Context?
+ To propagate deadlines and cancellation, so work stops when the caller gives up
- It's required for SQL injection protection
- It stores the connection string
- It enables transactions
? Context ties query lifetimes to request lifetimes, preventing piles of orphaned slow queries.

Q: In the MongoDB Go driver, when must you use bson.D instead of bson.M?
- For all filters
+ When field order matters, e.g. sort specifications, aggregation pipelines and commands
- When the document has nested fields
- Never; they're identical
? Go maps are unordered. bson.D is an ordered slice of key/value pairs.

Q: Which is the correct way to include user input in a SQL query?
- fmt.Sprintf with quotes around the value
- strings.ReplaceAll to escape quotes, then concatenate
+ Use placeholders ($1, $2) and pass values as arguments
- Base64 encode the input
? Placeholders send values separately from SQL text, eliminating injection.
```
