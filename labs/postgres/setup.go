package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const schema = `
DROP TABLE IF EXISTS order_items, orders, products, customers, accounts CASCADE;

CREATE TABLE customers (
  id          BIGSERIAL PRIMARY KEY,
  email       TEXT NOT NULL UNIQUE,
  name        TEXT NOT NULL,
  country     TEXT NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE products (
  id           BIGSERIAL PRIMARY KEY,
  sku          TEXT NOT NULL UNIQUE,
  name         TEXT NOT NULL,
  category     TEXT NOT NULL,
  price_cents  INT  NOT NULL CHECK (price_cents >= 0),
  stock        INT  NOT NULL CHECK (stock >= 0)
);

CREATE TABLE orders (
  id           BIGSERIAL PRIMARY KEY,
  customer_id  BIGINT NOT NULL REFERENCES customers(id),
  status       TEXT NOT NULL DEFAULT 'pending',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE order_items (
  order_id          BIGINT NOT NULL REFERENCES orders(id),
  product_id        BIGINT NOT NULL REFERENCES products(id),
  quantity          INT NOT NULL CHECK (quantity > 0),
  unit_price_cents  INT NOT NULL,
  PRIMARY KEY (order_id, product_id)
);
CREATE INDEX ON orders (customer_id);

CREATE TABLE accounts (
  id             BIGINT PRIMARY KEY,
  owner          TEXT NOT NULL,
  balance_cents  BIGINT NOT NULL CHECK (balance_cents >= 0)
);
`

const (
	numCustomers = 2000
	numProducts  = 200
	numOrders    = 20000
)

var (
	countries  = []string{"GB", "US", "DE", "FR", "IN", "ES", "NL", "IE", "JP", "BR"}
	categories = []string{"books", "electronics", "home", "toys", "sports", "beauty"}
	statuses   = []string{"paid", "paid", "paid", "shipped", "pending", "refunded"}
)

func setup(ctx context.Context, pool *pgxpool.Pool) error {
	start := time.Now()
	if _, err := pool.Exec(ctx, schema); err != nil {
		return err
	}
	r := rand.New(rand.NewPCG(42, 42)) // deterministic seed data

	customers := make([][]any, numCustomers)
	for i := range customers {
		id := int64(i + 1)
		customers[i] = []any{id, fmt.Sprintf("user%d@example.com", id), fmt.Sprintf("Customer %d", id),
			countries[r.IntN(len(countries))], time.Now().Add(-time.Duration(r.IntN(900)) * 24 * time.Hour)}
	}
	products := make([][]any, numProducts)
	prices := make([]int, numProducts+1)
	for i := range products {
		id := int64(i + 1)
		prices[id] = 199 + r.IntN(20000)
		products[i] = []any{id, fmt.Sprintf("SKU-%04d", id), fmt.Sprintf("Product %d", id),
			categories[r.IntN(len(categories))], prices[id], 50 + r.IntN(500)}
	}
	orders := make([][]any, numOrders)
	var items [][]any
	for i := range orders {
		id := int64(i + 1)
		orders[i] = []any{id, int64(1 + r.IntN(numCustomers)), statuses[r.IntN(len(statuses))],
			time.Now().Add(-time.Duration(r.IntN(365*24)) * time.Hour)}
		seen := map[int64]bool{}
		for n := 1 + r.IntN(4); n > 0; n-- {
			pid := int64(1 + r.IntN(numProducts))
			if seen[pid] {
				continue
			}
			seen[pid] = true
			items = append(items, []any{id, pid, 1 + r.IntN(3), prices[pid]})
		}
	}

	// COPY is the fastest way to bulk-load Postgres (far faster than INSERTs).
	loads := []struct {
		table string
		cols  []string
		rows  [][]any
	}{
		{"customers", []string{"id", "email", "name", "country", "created_at"}, customers},
		{"products", []string{"id", "sku", "name", "category", "price_cents", "stock"}, products},
		{"orders", []string{"id", "customer_id", "status", "created_at"}, orders},
		{"order_items", []string{"order_id", "product_id", "quantity", "unit_price_cents"}, items},
	}
	for _, l := range loads {
		if _, err := pool.CopyFrom(ctx, pgx.Identifier{l.table}, l.cols, pgx.CopyFromRows(l.rows)); err != nil {
			return fmt.Errorf("copy %s: %w", l.table, err)
		}
	}
	// Explicit IDs bypass the sequences, so move them past the loaded data.
	for _, t := range []string{"customers", "products", "orders"} {
		if _, err := pool.Exec(ctx, fmt.Sprintf(
			`SELECT setval(pg_get_serial_sequence('%s','id'), (SELECT max(id) FROM %s))`, t, t)); err != nil {
			return err
		}
	}
	if _, err := pool.Exec(ctx, `ANALYZE`); err != nil {
		return err
	}
	step("schema created; loaded %d customers, %d products, %d orders, %d order items in %v",
		numCustomers, numProducts, numOrders, len(items), time.Since(start).Round(time.Millisecond))
	return nil
}
