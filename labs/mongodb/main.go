// Command mongodb is the "MongoDB from Go" lab (official driver v2).
//
//	go run ./labs/mongodb <command>
//
// Commands: seed, crud, agg, index, tx, watch, all
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

var commands = []struct {
	name string
	help string
	run  func(context.Context, *mongo.Database) error
}{
	{"seed", "load 20k embedded order documents", seed},
	{"crud", "insert / find / update operators / delete", crud},
	{"agg", "aggregation pipelines: top products, revenue by month, window functions", agg},
	{"index", "explain() stats before and after an ESR compound index", indexDemo},
	{"tx", "multi-document transfer with WithTransaction (auto-retry)", txDemo},
	{"watch", "change streams: react to inserts and updates in real time", watch},
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017/?directConnection=true"
	}
	client, err := mongo.Connect(options.Client().
		ApplyURI(uri).
		SetTimeout(30 * time.Second).
		// decode nested documents inside bson.M as bson.M (the v2 default is bson.D)
		SetBSONOptions(&options.BSONOptions{DefaultDocumentM: true}))
	if err != nil {
		fail(err)
	}
	defer client.Disconnect(context.Background())

	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	defer cancelPing()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		fail(fmt.Errorf("cannot reach MongoDB at %s (did you run `make up`?): %w", uri, err))
	}
	db := client.Database("shop")

	name := os.Args[1]
	for _, c := range commands {
		if name == "all" || name == c.name {
			heading(c.name + " — " + c.help)
			if err := c.run(ctx, db); err != nil {
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

func usage() {
	fmt.Println("usage: go run ./labs/mongodb <command>")
	for _, c := range commands {
		fmt.Printf("  %-6s %s\n", c.name, c.help)
	}
	fmt.Printf("  %-6s %s\n", "all", "run every command in order")
}

func heading(s string) { fmt.Printf("\n\033[1;32m▶ %s\033[0m\n", s) }
func step(format string, a ...any) {
	fmt.Printf("  \033[1m%s\033[0m\n", fmt.Sprintf(format, a...))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
