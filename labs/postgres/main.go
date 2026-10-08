// Command postgres is the "PostgreSQL from Go" lab.
//
//	go run ./labs/postgres <command>
//
// Commands: setup, crud, tx, anomaly, explain, pool, all
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var commands = []struct {
	name string
	help string
	run  func(context.Context, *pgxpool.Pool) error
}{
	{"setup", "create the shop schema and seed data", setup},
	{"crud", "insert / query / update / delete + a reporting join", crud},
	{"tx", "money transfer in a SERIALIZABLE transaction with retries", txDemo},
	{"anomaly", "reproduce a lost update, then fix it three ways", anomaly},
	{"explain", "EXPLAIN ANALYZE before and after adding an index", explain},
	{"pool", "connection pool behaviour under concurrency", poolDemo},
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	url := databaseURL()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		fail(err)
	}
	defer pool.Close()
	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	defer cancelPing()
	if err := pool.Ping(pingCtx); err != nil {
		fail(fmt.Errorf("cannot reach Postgres at %s (did you run `make up`?): %w", url, err))
	}

	name := os.Args[1]
	for _, c := range commands {
		if name == "all" || name == c.name {
			heading(c.name + " — " + c.help)
			if err := c.run(ctx, pool); err != nil {
				fail(err)
			}
			if name != "all" {
				return
			}
		}
	}
	if name != "all" {
		usage()
		os.Exit(2)
	}
}

func databaseURL() string {
	if u := os.Getenv("DATABASE_URL"); u != "" {
		return u
	}
	return "postgres://dbm:dbm@localhost:5442/dbm?sslmode=disable"
}

func usage() {
	fmt.Println("usage: go run ./labs/postgres <command>")
	for _, c := range commands {
		fmt.Printf("  %-8s %s\n", c.name, c.help)
	}
	fmt.Printf("  %-8s %s\n", "all", "run every command in order")
}

func heading(s string) { fmt.Printf("\n\033[1;36m▶ %s\033[0m\n", s) }
func step(s string, a ...any) {
	fmt.Printf("  \033[1m%s\033[0m\n", fmt.Sprintf(s, a...))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
