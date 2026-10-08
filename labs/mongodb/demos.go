package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Order is stored as ONE document: customer (extended reference), items and
// totals are embedded because they are always read together.
type Order struct {
	ID          bson.ObjectID `bson:"_id,omitempty"`
	OrderNumber string        `bson:"orderNumber"`
	Status      string        `bson:"status"`
	CreatedAt   time.Time     `bson:"createdAt"`
	Customer    CustomerRef   `bson:"customer"`
	Items       []Item        `bson:"items"`
	Total       int64         `bson:"total"`
	History     []Event       `bson:"history,omitempty"`
}

type CustomerRef struct {
	ID      int    `bson:"id"`
	Name    string `bson:"name"`
	Country string `bson:"country"`
}

type Item struct {
	SKU       string `bson:"sku"`
	Category  string `bson:"category"`
	Qty       int    `bson:"qty"`
	UnitPrice int64  `bson:"unitPrice"`
}

type Event struct {
	At     time.Time `bson:"at"`
	Status string    `bson:"status"`
}

var (
	countries  = []string{"GB", "US", "DE", "FR", "IN", "ES", "NL", "IE", "JP", "BR"}
	categories = []string{"books", "electronics", "home", "toys", "sports", "beauty"}
	statuses   = []string{"paid", "paid", "paid", "shipped", "pending", "refunded"}
)

// ---------------------------------------------------------------- seed

func seed(ctx context.Context, db *mongo.Database) error {
	start := time.Now()
	orders := db.Collection("orders")
	if err := orders.Drop(ctx); err != nil {
		return err
	}
	r := rand.New(rand.NewPCG(42, 42))
	const n = 20000
	docs := make([]any, 0, n)
	for i := 1; i <= n; i++ {
		cid := 1 + r.IntN(2000)
		o := Order{
			OrderNumber: fmt.Sprintf("ORD-%06d", i),
			Status:      statuses[r.IntN(len(statuses))],
			CreatedAt:   time.Now().Add(-time.Duration(r.IntN(365*24)) * time.Hour),
			Customer:    CustomerRef{ID: cid, Name: fmt.Sprintf("Customer %d", cid), Country: countries[cid%len(countries)]},
		}
		for k := 1 + r.IntN(4); k > 0; k-- {
			p := 1 + r.IntN(200)
			it := Item{SKU: fmt.Sprintf("SKU-%04d", p), Category: categories[p%len(categories)],
				Qty: 1 + r.IntN(3), UnitPrice: int64(199 + (p*7919)%20000)}
			o.Items = append(o.Items, it)
			o.Total += int64(it.Qty) * it.UnitPrice
		}
		docs = append(docs, o)
	}
	// InsertMany batches documents into large wire-protocol messages.
	if _, err := orders.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false)); err != nil {
		return err
	}
	step("inserted %d order documents in %v", n, time.Since(start).Round(time.Millisecond))
	return nil
}

func ensureSeeded(ctx context.Context, db *mongo.Database) error {
	n, err := db.Collection("orders").EstimatedDocumentCount(ctx)
	if err != nil {
		return err
	}
	if n < 20000 {
		step("orders collection empty — seeding first")
		return seed(ctx, db)
	}
	return nil
}

// ---------------------------------------------------------------- crud

func crud(ctx context.Context, db *mongo.Database) error {
	coll := db.Collection("scratch_orders")
	coll.Drop(ctx)

	res, err := coll.InsertOne(ctx, Order{
		OrderNumber: "ORD-DEMO-1", Status: "pending", CreatedAt: time.Now(),
		Customer: CustomerRef{ID: 17, Name: "Ada Lovelace", Country: "GB"},
		Items:    []Item{{SKU: "KB-01", Category: "electronics", Qty: 1, UnitPrice: 12900}},
		Total:    12900,
	})
	if err != nil {
		return err
	}
	step("CREATE: inserted _id=%v", res.InsertedID)

	var o Order
	if err := coll.FindOne(ctx, bson.M{"customer.name": "Ada Lovelace"}).Decode(&o); err != nil {
		return err
	}
	step("READ: dot-notation query into an embedded doc → %s, %d item(s), total=%d", o.OrderNumber, len(o.Items), o.Total)

	// Update operators change fields atomically inside the document — no read-modify-write.
	_, err = coll.UpdateOne(ctx, bson.M{"_id": o.ID}, bson.M{
		"$set":  bson.M{"status": "paid"},
		"$push": bson.M{"items": Item{SKU: "MS-07", Category: "electronics", Qty: 2, UnitPrice: 3900}},
		"$inc":  bson.M{"total": 7800},
	})
	if err != nil {
		return err
	}
	_, err = coll.UpdateOne(ctx, bson.M{"_id": o.ID}, bson.M{
		"$push": bson.M{"history": bson.M{"$each": []Event{{At: time.Now(), Status: "paid"}}, "$slice": -20}},
	})
	if err != nil {
		return err
	}
	coll.FindOne(ctx, bson.M{"_id": o.ID}).Decode(&o)
	step("UPDATE: $set + $push + $inc in one atomic op → status=%s items=%d total=%d history=%d",
		o.Status, len(o.Items), o.Total, len(o.History))

	n, _ := coll.CountDocuments(ctx, bson.M{"items.sku": "MS-07"})
	step("READ: query matching inside an array (items.sku) → %d doc(s)", n)

	del, err := coll.DeleteOne(ctx, bson.M{"_id": o.ID})
	if err != nil {
		return err
	}
	step("DELETE: removed %d doc(s)", del.DeletedCount)
	return coll.Drop(ctx)
}

// ---------------------------------------------------------------- agg

func agg(ctx context.Context, db *mongo.Database) error {
	if err := ensureSeeded(ctx, db); err != nil {
		return err
	}
	orders := db.Collection("orders")

	step("Top 5 SKUs by revenue from paid orders ($match → $unwind → $group → $sort → $limit)")
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "status", Value: "paid"}}}},
		{{Key: "$unwind", Value: "$items"}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$items.sku"},
			{Key: "units", Value: bson.D{{Key: "$sum", Value: "$items.qty"}}},
			{Key: "revenue", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$multiply", Value: bson.A{"$items.qty", "$items.unitPrice"}}}}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "revenue", Value: -1}}}},
		{{Key: "$limit", Value: 5}},
	}
	if err := printAgg(ctx, orders, pipeline, func(m bson.M) string {
		return fmt.Sprintf("%-9v units=%-5v revenue=£%.2f", m["_id"], m["units"], float64(toInt(m["revenue"]))/100)
	}); err != nil {
		return err
	}

	step("Revenue by country and month, last 3 months ($dateToString + compound group key)")
	pipeline = mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "status", Value: "paid"},
			{Key: "createdAt", Value: bson.D{{Key: "$gte", Value: time.Now().AddDate(0, -3, 0)}}},
		}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{
				{Key: "country", Value: "$customer.country"},
				{Key: "month", Value: bson.D{{Key: "$dateToString", Value: bson.D{{Key: "format", Value: "%Y-%m"}, {Key: "date", Value: "$createdAt"}}}}},
			}},
			{Key: "revenue", Value: bson.D{{Key: "$sum", Value: "$total"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "revenue", Value: -1}}}},
		{{Key: "$limit", Value: 6}},
	}
	if err := printAgg(ctx, orders, pipeline, func(m bson.M) string {
		id := m["_id"].(bson.M)
		return fmt.Sprintf("%v %v revenue=£%.2f", id["country"], id["month"], float64(toInt(m["revenue"]))/100)
	}); err != nil {
		return err
	}

	step("Window function: each customer's running spend ($setWindowFields)")
	pipeline = mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "customer.id", Value: 42}}}},
		{{Key: "$setWindowFields", Value: bson.D{
			{Key: "partitionBy", Value: "$customer.id"},
			{Key: "sortBy", Value: bson.D{{Key: "createdAt", Value: 1}}},
			{Key: "output", Value: bson.D{
				{Key: "nth", Value: bson.D{{Key: "$documentNumber", Value: bson.D{}}}},
				{Key: "runningSpend", Value: bson.D{
					{Key: "$sum", Value: "$total"},
					{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "current"}}}},
				}},
			}},
		}}},
		{{Key: "$limit", Value: 5}},
	}
	return printAgg(ctx, orders, pipeline, func(m bson.M) string {
		return fmt.Sprintf("#%v %v total=£%.2f running=£%.2f", m["nth"], m["orderNumber"],
			float64(toInt(m["total"]))/100, float64(toInt(m["runningSpend"]))/100)
	})
}

func printAgg(ctx context.Context, coll *mongo.Collection, p mongo.Pipeline, format func(bson.M) string) error {
	cur, err := coll.Aggregate(ctx, p)
	if err != nil {
		return err
	}
	var rows []bson.M
	if err := cur.All(ctx, &rows); err != nil {
		return err
	}
	for _, r := range rows {
		fmt.Println("    " + format(r))
	}
	return nil
}

func toInt(v any) int64 {
	switch n := v.(type) {
	case int32:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	}
	return 0
}

// ---------------------------------------------------------------- index

func indexDemo(ctx context.Context, db *mongo.Database) error {
	if err := ensureSeeded(ctx, db); err != nil {
		return err
	}
	orders := db.Collection("orders")
	if err := orders.Indexes().DropAll(ctx); err != nil {
		return err
	}

	filter := bson.D{
		{Key: "customer.country", Value: "GB"},
		{Key: "status", Value: "paid"},
		{Key: "total", Value: bson.D{{Key: "$gte", Value: 20000}}},
	}
	sort := bson.D{{Key: "createdAt", Value: -1}}
	fmt.Println(`    query: find({"customer.country":"GB", status:"paid", total:{$gte:20000}}).sort({createdAt:-1}).limit(20)`)

	explainIt := func(label string) error {
		cmd := bson.D{
			{Key: "explain", Value: bson.D{
				{Key: "find", Value: "orders"},
				{Key: "filter", Value: filter},
				{Key: "sort", Value: sort},
				{Key: "limit", Value: 20},
			}},
			{Key: "verbosity", Value: "executionStats"},
		}
		var out bson.M
		if err := db.RunCommand(ctx, cmd).Decode(&out); err != nil {
			return err
		}
		stats := out["executionStats"].(bson.M)
		step("%s", label)
		fmt.Printf("      plan=%s  keysExamined=%v  docsExamined=%v  nReturned=%v  time=%vms\n",
			planStages(out["queryPlanner"]), stats["totalKeysExamined"], stats["totalDocsExamined"],
			stats["nReturned"], stats["executionTimeMillis"])
		return nil
	}

	if err := explainIt("WITHOUT index:"); err != nil {
		return err
	}
	_, err := orders.Indexes().CreateOne(ctx, mongo.IndexModel{
		// ESR: Equality (country, status) → Sort (createdAt) → Range (total)
		Keys: bson.D{
			{Key: "customer.country", Value: 1},
			{Key: "status", Value: 1},
			{Key: "createdAt", Value: -1},
			{Key: "total", Value: 1},
		},
		Options: options.Index().SetName("esr_country_status_created_total"),
	})
	if err != nil {
		return err
	}
	if err := explainIt("WITH ESR index {country:1, status:1, createdAt:-1, total:1}:"); err != nil {
		return err
	}
	step("note: no SORT stage with the index — the index already returns documents in createdAt order")
	return nil
}

// planStages flattens the winning plan into "STAGE ← STAGE ← …" (outermost first).
func planStages(qp any) string {
	m, ok := qp.(bson.M)
	if !ok {
		return "?"
	}
	plan, _ := m["winningPlan"].(bson.M)
	if inner, ok := plan["queryPlan"].(bson.M); ok { // slot-based engine wraps the plan
		plan = inner
	}
	out := ""
	for plan != nil {
		if out != "" {
			out += "←"
		}
		out += fmt.Sprint(plan["stage"])
		next, _ := plan["inputStage"].(bson.M)
		plan = next
	}
	return out
}

// ---------------------------------------------------------------- tx

func txDemo(ctx context.Context, db *mongo.Database) error {
	accounts := db.Collection("accounts")
	accounts.Drop(ctx)
	// Collections must exist before use inside a transaction.
	if _, err := accounts.InsertMany(ctx, []any{
		bson.M{"_id": "alice", "balance": 100000},
		bson.M{"_id": "bob", "balance": 100000},
	}); err != nil {
		return err
	}
	ledger := db.Collection("ledger")
	ledger.Drop(ctx)
	db.CreateCollection(ctx, "ledger")

	transfer := func(from, to string, amount int) error {
		sess, err := db.Client().StartSession()
		if err != nil {
			return err
		}
		defer sess.EndSession(ctx)
		// WithTransaction commits, and retries on TransientTransactionError /
		// UnknownTransactionCommitResult automatically.
		_, err = sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
			res, err := accounts.UpdateOne(ctx,
				bson.M{"_id": from, "balance": bson.M{"$gte": amount}},
				bson.M{"$inc": bson.M{"balance": -amount}})
			if err != nil {
				return nil, err
			}
			if res.MatchedCount == 0 {
				return nil, errors.New("insufficient funds")
			}
			if _, err := accounts.UpdateOne(ctx, bson.M{"_id": to}, bson.M{"$inc": bson.M{"balance": amount}}); err != nil {
				return nil, err
			}
			_, err = ledger.InsertOne(ctx, bson.M{"from": from, "to": to, "amount": amount, "at": time.Now()})
			return nil, err
		})
		return err
	}

	step("transfer 30000 alice → bob, then 999999 bob → alice (should fail and roll back)")
	if err := transfer("alice", "bob", 30000); err != nil {
		return err
	}
	if err := transfer("bob", "alice", 999999); err != nil {
		fmt.Printf("    second transfer rejected as expected: %v\n", err)
	}

	var a, b bson.M
	accounts.FindOne(ctx, bson.M{"_id": "alice"}).Decode(&a)
	accounts.FindOne(ctx, bson.M{"_id": "bob"}).Decode(&b)
	n, _ := ledger.CountDocuments(ctx, bson.M{})
	step("alice=%v bob=%v ledger entries=%d (the failed transfer left no trace)", a["balance"], b["balance"], n)
	return nil
}

// ---------------------------------------------------------------- watch

func watch(ctx context.Context, db *mongo.Database) error {
	coll := db.Collection("live_orders")
	coll.Drop(ctx)
	db.CreateCollection(ctx, "live_orders")

	cs, err := coll.Watch(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "operationType", Value: bson.D{{Key: "$in", Value: bson.A{"insert", "update"}}}}}}},
	}, options.ChangeStream().SetFullDocument(options.UpdateLookup))
	if err != nil {
		return err
	}
	defer cs.Close(ctx)

	go func() {
		time.Sleep(300 * time.Millisecond)
		for i := 1; i <= 3; i++ {
			res, _ := coll.InsertOne(context.Background(), bson.M{"orderNumber": fmt.Sprintf("LIVE-%d", i), "status": "pending"})
			time.Sleep(150 * time.Millisecond)
			coll.UpdateOne(context.Background(), bson.M{"_id": res.InsertedID}, bson.M{"$set": bson.M{"status": "paid"}})
		}
	}()

	step("watching live_orders (a writer goroutine inserts 3 orders and marks each paid) …")
	timeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for seen := 0; seen < 6 && cs.Next(timeout); seen++ {
		var ev struct {
			OperationType string `bson:"operationType"`
			FullDocument  bson.M `bson:"fullDocument"`
		}
		if err := cs.Decode(&ev); err != nil {
			return err
		}
		fmt.Printf("    %-6s → %v status=%v\n", ev.OperationType, ev.FullDocument["orderNumber"], ev.FullDocument["status"])
	}
	if err := cs.Err(); err != nil {
		return err
	}
	step("store cs.ResumeToken() durably to resume after a crash without missing events")
	return nil
}
