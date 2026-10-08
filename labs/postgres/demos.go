package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------------------------------------------------------------- crud

func crud(ctx context.Context, pool *pgxpool.Pool) error {
	if err := ensureSeeded(ctx, pool); err != nil {
		return err
	}

	var id int64
	err := pool.QueryRow(ctx,
		`INSERT INTO customers (email, name, country) VALUES ($1, $2, $3)
		 ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
		 RETURNING id`,
		"ada@example.com", "Ada Lovelace", "GB").Scan(&id)
	if err != nil {
		return err
	}
	step("CREATE: upserted Ada with id=%d (always use $1 placeholders — never fmt.Sprintf SQL)", id)

	type Customer struct {
		ID      int64
		Name    string
		Country string
	}
	rows, err := pool.Query(ctx, `SELECT id, name, country FROM customers WHERE country = $1 ORDER BY id LIMIT 3`, "GB")
	if err != nil {
		return err
	}
	gb, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Customer])
	if err != nil {
		return err
	}
	step("READ: first 3 GB customers → %+v", gb)

	tag, err := pool.Exec(ctx, `UPDATE products SET stock = stock - 1 WHERE id = $1 AND stock > 0`, 1)
	if err != nil {
		return err
	}
	step("UPDATE: atomic stock decrement affected %d row(s)", tag.RowsAffected())

	tag, err = pool.Exec(ctx, `DELETE FROM customers WHERE email = $1`, "ada@example.com")
	if err != nil {
		return err
	}
	step("DELETE: removed %d row(s)", tag.RowsAffected())

	step("REPORT: top 5 customers by paid spend (3-way join + GROUP BY)")
	rows, err = pool.Query(ctx, `
		SELECT c.name, c.country, count(DISTINCT o.id) AS orders,
		       sum(oi.quantity * oi.unit_price_cents) / 100.0 AS spend
		FROM customers c
		JOIN orders o       ON o.customer_id = c.id AND o.status = 'paid'
		JOIN order_items oi ON oi.order_id = o.id
		GROUP BY c.id
		ORDER BY spend DESC
		LIMIT 5`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, country string
		var orders int
		var spend float64
		if err := rows.Scan(&name, &country, &orders, &spend); err != nil {
			return err
		}
		fmt.Printf("    %-14s %s  %3d orders  £%10.2f\n", name, country, orders, spend)
	}
	return rows.Err()
}

func ensureSeeded(ctx context.Context, pool *pgxpool.Pool) error {
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.order_items') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		step("schema missing — running setup first")
		return setup(ctx, pool)
	}
	return nil
}

// ---------------------------------------------------------------- tx

func resetAccounts(ctx context.Context, pool *pgxpool.Pool) error {
	if err := ensureSeeded(ctx, pool); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, `
		TRUNCATE accounts;
		INSERT INTO accounts VALUES (1, 'alice', 100000), (2, 'bob', 100000)`)
	return err
}

// isRetryable reports whether Postgres aborted the transaction because of a
// concurrency conflict that a retry may resolve.
func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01")
}

// withRetry runs fn in a transaction at the given isolation level, retrying
// serialization failures and deadlocks with jittered exponential backoff.
func withRetry(ctx context.Context, pool *pgxpool.Pool, iso pgx.TxIsoLevel, fn func(pgx.Tx) error) (retries int, err error) {
	for attempt := 0; ; attempt++ {
		err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{IsoLevel: iso}, fn)
		if err == nil || !isRetryable(err) || attempt == 20 {
			return attempt, err
		}
		backoff := time.Duration(1<<min(attempt, 6)) * time.Millisecond
		time.Sleep(backoff + time.Duration(rand.IntN(int(backoff))))
	}
}

func transfer(ctx context.Context, pool *pgxpool.Pool, from, to, cents int64) (int, error) {
	return withRetry(ctx, pool, pgx.Serializable, func(tx pgx.Tx) error {
		var bal int64
		if err := tx.QueryRow(ctx, `SELECT balance_cents FROM accounts WHERE id = $1`, from).Scan(&bal); err != nil {
			return err
		}
		if bal < cents {
			return fmt.Errorf("insufficient funds: have %d, need %d", bal, cents)
		}
		if _, err := tx.Exec(ctx, `UPDATE accounts SET balance_cents = balance_cents - $1 WHERE id = $2`, cents, from); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE accounts SET balance_cents = balance_cents + $1 WHERE id = $2`, cents, to)
		return err
	})
}

func txDemo(ctx context.Context, pool *pgxpool.Pool) error {
	if err := resetAccounts(ctx, pool); err != nil {
		return err
	}
	step("50 concurrent transfers of £10 in both directions at SERIALIZABLE …")
	var wg sync.WaitGroup
	var mu sync.Mutex
	totalRetries := 0
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			from, to := int64(1), int64(2)
			if i%2 == 1 {
				from, to = 2, 1
			}
			n, err := transfer(ctx, pool, from, to, 1000)
			if err != nil {
				fmt.Println("    transfer failed:", err)
			}
			mu.Lock()
			totalRetries += n
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	var a, b int64
	pool.QueryRow(ctx, `SELECT (SELECT balance_cents FROM accounts WHERE id=1), (SELECT balance_cents FROM accounts WHERE id=2)`).Scan(&a, &b)
	step("alice=%d bob=%d total=%d (must be 200000) — %d serialization retries were needed", a, b, a+b, totalRetries)
	if a+b != 200000 {
		return errors.New("money was created or destroyed")
	}
	return nil
}

// ---------------------------------------------------------------- anomaly

func anomaly(ctx context.Context, pool *pgxpool.Pool) error {
	const workers = 10
	run := func(label string, deposit func() error) error {
		if err := resetAccounts(ctx, pool); err != nil {
			return err
		}
		var wg sync.WaitGroup
		errs := make(chan error, workers)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := deposit(); err != nil {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			return err
		}
		var bal int64
		pool.QueryRow(ctx, `SELECT balance_cents FROM accounts WHERE id = 1`).Scan(&bal)
		want := int64(100000 + workers*100)
		verdict := "\033[32m✓ correct\033[0m"
		if bal != want {
			verdict = fmt.Sprintf("\033[31m✗ LOST %d updates\033[0m", (want-bal)/100)
		}
		fmt.Printf("    %-46s balance=%d want=%d  %s\n", label, bal, want, verdict)
		return nil
	}

	step("%d goroutines each deposit 100 into account 1 using read → sleep → write", workers)

	err := run("1. naive read-modify-write (READ COMMITTED)", func() error {
		return pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
			var bal int64
			if err := tx.QueryRow(ctx, `SELECT balance_cents FROM accounts WHERE id = 1`).Scan(&bal); err != nil {
				return err
			}
			time.Sleep(20 * time.Millisecond) // simulate app logic between read and write
			_, err := tx.Exec(ctx, `UPDATE accounts SET balance_cents = $1 WHERE id = 1`, bal+100)
			return err
		})
	})
	if err != nil {
		return err
	}

	err = run("2. fix: atomic UPDATE ... SET x = x + 100", func() error {
		_, err := pool.Exec(ctx, `UPDATE accounts SET balance_cents = balance_cents + 100 WHERE id = 1`)
		return err
	})
	if err != nil {
		return err
	}

	err = run("3. fix: SELECT ... FOR UPDATE (pessimistic)", func() error {
		return pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			var bal int64
			if err := tx.QueryRow(ctx, `SELECT balance_cents FROM accounts WHERE id = 1 FOR UPDATE`).Scan(&bal); err != nil {
				return err
			}
			time.Sleep(20 * time.Millisecond)
			_, err := tx.Exec(ctx, `UPDATE accounts SET balance_cents = $1 WHERE id = 1`, bal+100)
			return err
		})
	})
	if err != nil {
		return err
	}

	return run("4. fix: SERIALIZABLE + retry (optimistic)", func() error {
		_, err := withRetry(ctx, pool, pgx.Serializable, func(tx pgx.Tx) error {
			var bal int64
			if err := tx.QueryRow(ctx, `SELECT balance_cents FROM accounts WHERE id = 1`).Scan(&bal); err != nil {
				return err
			}
			time.Sleep(20 * time.Millisecond)
			_, err := tx.Exec(ctx, `UPDATE accounts SET balance_cents = $1 WHERE id = 1`, bal+100)
			return err
		})
		return err
	})
}

// ---------------------------------------------------------------- explain

func explain(ctx context.Context, pool *pgxpool.Pool) error {
	step("creating events table with 300,000 rows via generate_series …")
	_, err := pool.Exec(ctx, `
		DROP TABLE IF EXISTS events;
		CREATE TABLE events AS
		SELECT g AS id,
		       (random() * 10000)::int AS user_id,
		       (ARRAY['click','view','purchase','signup'])[1 + (random()*3)::int] AS type,
		       now() - (random() * interval '365 days') AS created_at
		FROM generate_series(1, 300000) g;
		ANALYZE events;`)
	if err != nil {
		return err
	}
	query := `SELECT count(*) FROM events WHERE user_id = 4242 AND created_at > now() - interval '30 days'`
	fmt.Printf("    query: %s\n", query)

	show := func(label string) error {
		rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+query)
		if err != nil {
			return err
		}
		lines, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		step("%s", label)
		for _, l := range lines {
			fmt.Println("      " + l)
		}
		return nil
	}
	if err := show("WITHOUT an index (expect Seq Scan):"); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `CREATE INDEX idx_events_user_created ON events (user_id, created_at); ANALYZE events;`); err != nil {
		return err
	}
	if err := show("WITH index (user_id, created_at) — equality column first, range column second:"); err != nil {
		return err
	}
	step("compare 'Execution Time' and 'Buffers' between the two plans")
	return nil
}

// ---------------------------------------------------------------- pool

func poolDemo(ctx context.Context, _ *pgxpool.Pool) error {
	cfg, err := pgxpool.ParseConfig(databaseURL())
	if err != nil {
		return err
	}
	cfg.MaxConns = 5
	small, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer small.Close()

	const jobs = 30
	step("running %d queries that each take 100ms through a pool of MaxConns=%d …", jobs, cfg.MaxConns)
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			small.Exec(ctx, `SELECT pg_sleep(0.1)`)
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	s := small.Stat()
	step("took %v (≈ %d jobs / %d conns × 100ms = %v)", elapsed.Round(time.Millisecond), jobs, cfg.MaxConns,
		time.Duration(jobs/int(cfg.MaxConns))*100*time.Millisecond)
	fmt.Printf("    acquires=%d  had-to-wait=%d  total-wait=%v  open-conns=%d\n",
		s.AcquireCount(), s.EmptyAcquireCount(), s.AcquireDuration().Round(time.Millisecond), s.TotalConns())
	step("lesson: the pool caps concurrency at the database; size it to the DB's capacity, not the number of goroutines")
	return nil
}
