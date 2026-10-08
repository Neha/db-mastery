## Why a cache is a second database

A cache is not "Redis in front". It is a **copy of the truth with a freshness policy**. Every policy is a lie you have decided to live with: the row may be stale, missing, or present after the source deleted it. The work is choosing which lie, and what happens when a popular key expires.

This lesson is the patterns, the failure modes, and when the cache makes the outage worse. For when Redis earns its place versus a search engine or a time-series store, see [specialised databases](#/lesson/specialized-databases). For why a covering index is often a better "cache" than Redis, see [Indexes](#/lesson/indexes).

> **Key idea:** Add a cache when you have a measured hot read, a freshness budget, and an invalidation story. Do not add one because the database "might" be slow.

## Three write paths

![Cache-aside lets the app fill on miss and delete on write. Write-through updates cache and database together. Write-behind acknowledges from the cache and flushes later — faster, and you can lose the buffer.](diagrams/problem-caching-write-paths.svg)

### Cache-aside (lazy load)

The application owns the policy. Read: `GET` the key; on miss, read the database, `SET` with a TTL. Write: write the database, then **delete** the key (not rewrite it — a concurrent reader can otherwise refill from the old row).

```text
GET product:42
  hit  → return
  miss → SELECT … → SET product:42  NX/EX 60 → return
PUT product:42
  UPDATE products … → DEL product:42
```

This is the default for a reason. The cache only holds what was recently read. A cold cache is slow but correct once the database answers. Failure mode: **stampede** on a hot key's expiry (below).

### Write-through

Every write updates the cache and the database before you acknowledge the client. The cache stays warm for keys you write even if nobody has read them. You pay two hops on the write path, and you must handle a partial failure (cache written, database not — or the reverse).

Use it for session-like objects where a read-after-write must see the new value and the working set is small enough to keep in RAM.

### Write-behind (write-back)

The write hits the cache; a flusher writes the database later. This is how some stores absorb bursts. It is also how you **lose acknowledged writes** if the cache node dies with a dirty buffer. Treat it as a durability decision, not a performance trick. If the customer said RPO=0, this pattern is off the table.

> **Warning:** "We'll just put Redis in front of Postgres" without saying which of the three paths you mean is not a design. The interviewer will ask what happens on a crash and what happens on a write.

## TTL, invalidation, and the two clocks

A **TTL** is a freshness *upper bound*. It is not invalidation. A 60-second TTL on a price means you can sell at the old price for up to a minute after merchandising changes it.

**Explicit invalidation** deletes (or rewrites) the key when the source changes. You can do that in the application (cache-aside delete), or from the database: Postgres `LISTEN/NOTIFY`, logical decoding, Debezium, MongoDB change streams. CDC-based invalidation scales when many services read the same keys.

In practice you use **both**: delete on write so the common path is fresh, and a TTL so a missed delete cannot poison the key forever.

```text
write path:  DB commit → DEL key          (fast, best-effort)
safety net:  TTL 60–300 s                 (bounds the missed delete)
```

> **Gotcha:** Rewriting the cache on write races with a reader that loaded the old row. Delete-on-write is the safe default. If you must write-through, do it *after* the database commit and accept that a crash between the two leaves a miss, not a stale hit.

## When the cache makes things worse

### Stampede (thundering herd)

A hot key expires. Ten thousand requests miss in the same millisecond and all run the same expensive query. The cache you added to protect the database becomes the thing that DDoSes it. Combined with a connection storm, this is a classic cascading failure.

![When a popular key's TTL hits zero, every client misses together and the database sees N identical queries. Coalesce the rebuild, serve stale while refreshing, or expire early with jitter.](diagrams/problem-caching-stampede.svg)

Three mitigations, and you usually want more than one:

1. **Request coalescing** (singleflight): one in-flight rebuild per key; everyone else waits for that result. In Go, `golang.org/x/sync/singleflight`. In Redis, a lock (`SET key:lock NX EX 10`) around the loader — with a hard timeout so a dead loader does not pin the key.
2. **Stale-while-revalidate**: serve the expired value and refresh in the background. Users see slightly stale data; the database sees one query. Redis does not do this natively; you store `{value, fresh_until}` and treat `fresh_until < now` as "serve + refresh", not as a miss.
3. **Jittered TTL and probabilistic early expiry**: do not expire every copy of a popular key at second 60. Add random jitter (`TTL + rand(0, 10)`). Better: as the key approaches expiry, each reader refreshes with probability that rises toward 1, so rebuilds spread out (the "XFetch" / early-expiry idea).

```go
// Coalesce: one loader per key, callers share the result.
var sf singleflight.Group

func product(ctx context.Context, id int64) (Product, error) {
    v, err, _ := sf.Do(strconv.FormatInt(id, 10), func() (any, error) {
        if p, ok := cache.Get(id); ok {
            return p, nil
        }
        p, err := db.FindProduct(ctx, id) // the expensive path
        if err == nil {
            cache.Set(id, p, 60*time.Second)
        }
        return p, err
    })
    return v.(Product), err
}
```

### Caching things that cannot hit

A unique query per request (`WHERE session_token = $1` on a one-time token, or an admin filter that never repeats) has a hit ratio near zero. You paid a network hop and still hit the database. Cache **hot, repeated** keys. Measure hit ratio *per key prefix*, not a global 80% that hides a dead namespace.

### Tiny TTL, huge working set

A 5-second TTL on a 50 GB working set means you are rebuilding constantly. The cache is a very expensive random-replacement layer. Either raise the TTL and invalidate on write, or cache a smaller computed object (the product card, not the whole catalogue).

### Stampede + pool exhaustion

The stampede opens N database connections and holds them for the duration of the slow query. You now have the previous lesson's connection storm, caused by the cache. Coalescing is what prevents that pairing.

## Hot keys in Redis

Redis (and Redis Cluster) executes commands for a given key on **one** shard, largely single-threaded. A key that is read 100,000 times a second is one CPU. `MONITOR`, `SLOWLOG`, and `redis-cli --hotkeys` (or the equivalent in a managed console) find it.

Mitigations:

- A **local** (in-process) LRU in front of Redis for the hottest keys, with a 1–5 s TTL. Most of the 100,000 reads never leave the pod.
- Read replicas for read-heavy keys (still one writer).
- **Split** the key if it is a huge HASH or a counter you can shard (`views:post:42#0..n`), same salting idea as [hot keys](#/lesson/problem-scaling-hotspots).
- Do not store a 20 MB JSON blob under one key and `GET` it on every page.

## Cache poisoning

Two different bugs share this name. Be precise.

1. **Untrusted cache keys.** If the key includes raw user input (`cache.Get(req.URL)`), an attacker can flood the keyspace, evict hot entries, or (in a shared cache) read another user's response. Key only on canonical, authenticated fields: `product:{id}`, `session:{sid}` after you have validated `sid`.
2. **Caching an attacker-controlled or error body.** A 500 from the database stored under the product key, or a page that reflected a query string, will be served to everyone until TTL. Do not cache 5xx. Do not cache responses that include unsanitised input. In a shared CDN, `Cache-Control` and `Vary` are part of the same conversation.

## Redis data structures (the ones you actually use)

| Structure | What it is | Typical use |
|---|---|---|
| `STRING` | bytes, optional `INCR` | cached JSON, counters, locks (`SET NX EX`) |
| `HASH` | field → value | one object; `HSET` a single field without rewriting the blob |
| `LIST` | linked list | recent N, simple queues (`LPUSH`/`BRPOP`) |
| `SET` | unique members | membership, tags |
| `ZSET` | member + score | leaderboards, range-by-time |
| `STREAM` | append-only entries | consumer groups, a light event log |
| `HyperLogLog` | approximate cardinality | "how many unique IPs today" |
| `GEO` / bitmap | specialised | radius search, feature flags |

Pick the structure that lets you **update a field without reading the whole value**. A 50 KB JSON `STRING` that you `GET`, mutate, `SET` is a hot-key and a race. A `HASH` with `HINCRBY views 1` is not.

Sessions, rate limits and leaderboards belong in Redis. The source of truth for orders and balances does not. If you need the cache to survive a restart, you have probably found a database.

## Scenarios

### Problem: The cache caused the outage

> **Real world:** "We put Redis in front of the product page. p99 dropped from 40 ms to 4 ms. This morning we deployed a TTL change from 10 minutes to 60 seconds. The primary's CPU went to 100% and checkout failed. Redis itself was fine."

**Symptoms.** Redis hit ratio collapsed. Identical `SELECT` text flooded `pg_stat_statements`. Connection count jumped with the miss rate.

**Diagnose.** Which keys expired together? Was the TTL aligned to deploy time (every key set in a warmup now expires together)? Is there coalescing? Did the miss path open a new DB connection per request?

**Root causes.** Stampede on a popular key. Synchronised TTLs. No singleflight. Pool sized for the hit path.

**Fix.** Immediate: raise TTL, serve stale, or feature-flag the cache off *and* shed load (turning the cache off without shedding repeats the stampede). Durable: coalescing, jitter, stale-while-revalidate, a local LRU for the top keys.

**Prevent.** Load-test expiry, not just a warm cache. Alert on miss rate and on DB connections, together.

> **Model answer — what to say in the interview:** A cache that suddenly misses is a thundering herd. I would not "scale the database". I would coalesce rebuilds so one request loads the key, serve stale while that runs, and jitter TTLs so popular keys do not expire together. I would also check that the miss path cannot open more connections than the primary can take — the stampede and the connection storm are the same incident.

### Problem: Prices stay wrong for ten minutes after a merchandising update

> **Real world:** "Merchandising updates a price in Postgres. The site shows the old price until the TTL dies. They want 'a few seconds' of staleness, not ten minutes."

**Symptoms.** Cache hit ratio is healthy. The data is wrong. TTL is 10 minutes. Writes do not touch Redis.

**Diagnose.** Confirm it is cache-aside with TTL-only freshness. Check whether the write path can publish a delete (application, `NOTIFY`, or CDC).

**Root causes.** No invalidation. TTL set for hit ratio, not for the business freshness budget.

**Fix.** Delete the key in the same request that commits the price (or from a change stream). Keep a short TTL as a safety net. If many keys must change (a category-wide sale), bump a **generation** prefix (`product:v17:42`) instead of walking the keyspace.

**Prevent.** Write down the freshness budget per key type: prices (seconds), product copy (minutes), recommendations (hours).

> **Model answer — what to say in the interview:** TTL is a bound, not an invalidation strategy. I delete the key after the database commit, or I consume a change stream and delete. I keep a TTL so a missed delete cannot last forever. For a bulk price change I would bump a version prefix rather than DEL a million keys.

### Problem: One Redis shard is at 90% CPU, the others are idle

> **Real world:** "We sharded Redis. One of six shards is pegged. The keyspace looks even in `INFO keyspace`."

**Symptoms.** One shard's CPU and `ops/sec` dwarf the others. Memory may look even (many cold keys on the quiet shards).

**Diagnose.** Hot-key analysis. Identify the key and the command (`GET` of a huge STRING vs `ZRANGE` of a huge ZSET vs `HGETALL`).

**Root causes.** One key, not one slot imbalance. A celebrity product, a global config document, or a leaderboard ZSET that everyone reads.

**Fix.** Local LRU in the app for that key. Split the structure if it is a counter or a HASH you can shard. Replica reads if it is read-only. Do not "add more shards" — the key still hashes to one of them.

**Prevent.** Treat "global" keys as a design smell. Cache them in the process.

> **Model answer — what to say in the interview:** Redis Cluster places a key on one shard. A hot key is a CPU problem, not a slot-count problem. I would put a short-TTL process-local cache in front, and split the key if it is a writable counter. Adding shards will not move a single key.

```quiz
Q: In cache-aside, what should the write path do to the cache after a successful database update?
- SET the new value immediately, before the database commit
- FLUSHALL
- Nothing; the TTL will fix it
+ Delete the key (and rely on the next read to refill), optionally with a TTL as a safety net
? Delete-on-write avoids a race where a concurrent reader refills from the old row. TTL alone leaves a freshness hole as wide as the TTL.

Q: A popular key expires and the database CPU spikes. What is the first mitigation to name?
+ Coalesce the rebuild so only one request loads the key; others wait or receive stale data
- Double the Redis memory
- Switch from cache-aside to write-behind
- Disable TTLs on every key
? The spike is N identical misses. One loader (and stale-while-revalidate) turns N into 1.

Q: When is write-behind the wrong caching pattern?
- When reads outnumber writes
+ When a lost buffer would violate the durability / RPO the customer asked for
- When using Redis
- When the value is a string
? Write-behind acknowledges before the database flush. A cache-node crash loses dirty writes.

Q: Why can a Redis Cluster still have a hot shard after you add nodes?
- Cluster mode does not shard
+ A single hot key still lives on one shard; resharding moves slots, not fields inside one key
- Redis cannot store more than one key
- Adding nodes always makes CPU worse
? Throughput for one key is capped by one thread on one shard. Split the key or cache it locally.

Q: Which Redis structure lets you increment one field of an object without rewriting the whole blob?
- STRING only
- STREAM
+ HASH with HINCRBY / HSET
- HyperLogLog
? A JSON STRING requires GET-mutate-SET (a race and a hot key). A HASH updates one field in place.

Q: Caching every SQL query string, including those with a unique session token, is a bad idea because:
+ Those keys never repeat, so you pay a cache hop and still hit the database
- Redis cannot store query text
- TTLs do not work on long keys
- The planner will ignore indexes
? Caches need reuse. Unique-per-request keys have a hit ratio near zero and add latency.
```
