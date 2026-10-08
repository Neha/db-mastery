## Why these four

Instagram, Discord, Notion and Figma each published how they grew past a single database. None of them started distributed. They stayed on the boring store until a *measured* limit forced a change, then they sharded along the access pattern they already had.

Read them as an SA, not as folklore. The reusable parts are the ID scheme, the logical-to-physical mapping, the decision to keep related writes on one host, and the refusal to pretend a new database is a strategy.

> **SA angle:** In a customer meeting, "Instagram used Postgres" is not a recommendation. "They encoded the shard in the ID so every request was targeted, and they moved *logical* shards between hosts without rewriting keys" is.

## Instagram: Django, Postgres, and an ID that routes itself

When Instagram wrote *Sharding & IDs at Instagram*, the application was still **Django** talking to **PostgreSQL**. They evaluated a few NoSQL options and stayed on Postgres: they wanted to shard the data they already had, not rewrite the product around a new model.

The first problem after "we will have many databases" is unique IDs. A single-node `SERIAL` is not unique once inserts land on many hosts at once. Their published scheme is a 64-bit integer generated in Postgres:

| Bits | Meaning |
|---|---|
| 41 | milliseconds since a custom epoch (they quote about 41 years of IDs) |
| 13 | logical shard ID |
| 10 | per-table sequence, taken modulo 1024 |

That gives 1,024 IDs per logical shard per millisecond, and the IDs sort roughly by time. The shard bits mean the application can route a lookup from the ID alone.

They sharded by **user ID**. In the example from the post: 2,000 logical shards, user `31341`, shard `31341 % 2000 = 1341`. The next sequence value is packed into the low 10 bits. Generation lived in a `PL/pgSQL` function used as a column `DEFAULT`, with `RETURNING` on insert so the app never minted IDs itself.

![A 64-bit Instagram ID is timestamp, then logical shard, then a short sequence. The shard is derived from the user, so the ID knows where it lives.](diagrams/case-studies-scaling-instagram-id.svg)

The other published trick is **many logical shards, fewer physical hosts**. They described several thousand logical shards mapped in application code onto a much smaller set of Postgres servers. Each logical shard was a **Postgres schema**. Moving capacity meant moving a schema to another machine — not rehashing every photo ID.

```sql
-- Shape of the published generator (epoch and shard are constants per schema).
-- 41 bits of time, then 13 bits of shard, then 10 bits of sequence.
SELECT
  ((FLOOR(EXTRACT(EPOCH FROM clock_timestamp()) * 1000)::bigint - 1314220021721) << 23)
  | (1341 << 10)
  | (5001 % 1024) AS instagram_style_id;
```

> **Key idea:** The ID is a contract. Time order, uniqueness and routing are the same 64 bits. Change the mapping of logical shard → host as often as you like; do not change what those 13 bits mean.

They later ran a much larger estate (Redis, Cassandra and more appear in other Instagram engineering posts). Do not retell that era unless you are looking at a specific post. The sharding article is the one an interviewer expects.

## Discord: messages are a write-heavy, partition-shaped problem

Discord's early stack was **MongoDB** on a single replica set — fine while they were iterating. Chat history is "write forever, read the tail, never throw it away". In *How Discord Stores Billions of Messages* they said that by late 2015 the messages collection was outgrowing that replica set, and MongoDB sharding at the time was not the bet they wanted.

They moved **messages** to **Cassandra**. Other product data stayed on MongoDB for longer. The message schema is the lesson:

```text
PRIMARY KEY ((channel_id, bucket), message_id)
WITH CLUSTERING ORDER BY (message_id DESC)
```

`channel_id` and `message_id` are **Snowflakes** (time-sortable IDs). The **bucket** is a static time window derived from the Snowflake timestamp — they wrote that about **ten days** of a large channel stayed comfortably under **100 MB**, which is the partition size they were aiming for. Without the bucket, a busy channel is one unbounded partition: every write and every history read hits the same nodes, compaction never finishes, and a celebrity channel becomes a cluster-wide latency event.

![A channel without a time bucket is one growing partition. Discord's published schema adds a window derived from the Snowflake so each partition stays bounded.](diagrams/case-studies-scaling-discord-buckets.svg)

Cassandra writes are cheap (commit log plus memtable). Reads are not: they may touch the memtable and several SSTables. Discord ran quorum reads and writes, so a hot partition on one replica slowed every query that needed that replica in the quorum.

By the 2017 post they were on **12 Cassandra nodes** storing billions of messages. The follow-up, *How Discord Stores Trillions of Messages* (2023), is honest about the operational cost: at the start of 2022 the messages cluster was **177 nodes**, repairs and compaction were toil, and JVM GC pauses were a regular page. They had already moved every database *but* messages to **ScyllaDB** (Cassandra-compatible, C++, no JVM GC) around **2020**; messages followed in **May 2022**.

Published outcomes they actually wrote down:

- 177 Cassandra nodes → **72** ScyllaDB nodes
- disks from about **4 TB** per Cassandra node to **9 TB** per ScyllaDB node
- historical-message p99 from **40–125 ms** on Cassandra to about **15 ms** on ScyllaDB
- insert p99 from **5–70 ms** to a steady **5 ms**

They also put **Rust data services** in front of the cluster: one gRPC endpoint per query, **request coalescing**, and consistent-hash routing by channel ID so a World Cup goal does not become N identical reads of the same partition.

> **Remember:** Discord did not "pick Cassandra because chat is NoSQL". They picked a store whose partition model matched *channel + time*, then spent years paying for wide partitions, GC and repair until they changed the engine *without* changing the data model.

## Notion: the block is the product, the workspace is the shard

Notion's published data model is aggressively simple: **everything is a block** — paragraph, heading, page, list item, a row in a Notion database. A block is a Postgres row with a type, properties, a `content` array of child block IDs, and a `parent` pointer used for permissions. Loading a page walks that tree (`loadPageChunk` in their data-model post).

That model is why they stayed on **Postgres** for so long. A page edit is a small tree of related rows that you want in **one transaction**. They ran a monolith through "five years and four orders of magnitude of growth" (their phrasing). By mid-2020 on-call was waking up to CPU spikes, and catalog-only migrations were becoming unsafe. The trigger they name for sharding is not "we read a blog about Citus". It is **VACUUM stalling** and the risk of **transaction-ID wraparound** — Postgres stopping writes to protect the cluster.

*Herding elephants* (October 2021) is the sharding post. Decisions they published:

1. **Application-level sharding**, not Citus or Vitess. They wanted the routing in their code.
2. **Shard every table reachable from `block`** by a foreign key (discussions, comments, collections, spaces, …). Related rows that must commit together stay on one host. A block on shard A and its comments on shard B is a distributed transaction waiting to happen.
3. **Partition key = workspace ID.** Notion is a team product; users almost always query inside one workspace. Workspace IDs are UUIDs, so they bucket the UUID space. They are explicit that a large enterprise workspace is hotter than many personal ones — the key is about *locality*, not perfect balance.
4. **480 logical shards / 32 physical databases / 15 Postgres schemas per host.** 480 is not a power of two: it has many factors, so they can grow 32 → 40 → 48 hosts without doubling. They later published a reshard that kept **480 logical shards** and spread them across **96** physical databases (five schemas each).

![Instagram and Notion both put many logical shards (schemas) on fewer physical hosts, then moved a whole schema when they needed capacity.](diagrams/case-studies-scaling-logical-physical.svg)

The cutover used an **audit log** for double-writes (logical replication could not keep up with `block` on the initial snapshot), a backfill that compared versions, **dark reads** that compared old and new and discarded the new result, and a reverse audit log they hoped not to use. They took about **five minutes** of scheduled downtime for the switch. In hindsight they wrote that they should have sharded earlier: waiting until the monolith was already strained ruled out the nicer double-write options.

> **Pitfall:** Shard key = "the biggest table's primary key" is how you get cross-shard joins on every page load. Notion sharded by the *unit of user work* (the workspace), then dragged every related table with it.

## Figma: two systems, two sources of truth

A common interview line is "Figma used OT, like Google Docs". Their own posts contradict that.

*How Figma's multiplayer technology works* says they **looked at operational transformation and rejected it**. OT's transform rules grow painful, and a design file is not a text buffer: it is a tree of objects with properties. They built a **server-authoritative** protocol over WebSockets:

- each open file has a **dedicated server process**
- clients apply edits locally, send property deltas
- the server **orders** them and broadcasts
- concurrent edits to the same property are **last-writer-wins**
- child order uses **fractional indexes** (a real number / arbitrary-precision fraction between two neighbours), not OT and not a CRDT

Offline reconnect is deliberately dumb: download a fresh copy, replay outstanding local edits, continue. They later wrote *Making multiplayer more reliable*: the in-memory document is **checkpointed** every 30–60 seconds, and they added a **transaction log** of sequenced changes. They chose **DynamoDB** for that journal — they already ran Postgres, but they did not want this write volume on it yet.

**Postgres is the source of truth for everything that is not the canvas**: comments, users, teams, projects, permissions. *LiveGraph* sits on top of that Postgres. It is a query engine, not a second database. It **tails the replication / WAL stream** (distributed via Kafka in their write-up), invalidates in-memory views, and pushes updates to clients. Polling would have multiplied read load by the number of open tabs.

A later LiveGraph post notes the database under it going from **one Postgres instance to vertical and horizontal shards**. Multiplayer never became "put the file in Postgres and SELECT it on every keystroke".

> **Key idea:** Figma split *collaboration on a mutable document* (in-memory, sequenced, checkpointed) from *relational product data* (Postgres + WAL-tail fan-out). Same company, opposite designs, each matching its access pattern.

## Patterns an SA should reuse

| Pattern | Where it showed up | What you say to a customer |
|---|---|---|
| **Don't start distributed** | All four | One well-run primary plus replicas gets most products surprisingly far. Shard when you can name the limit (IOPS, wraparound, partition size, working set). |
| **IDs that carry location** | Instagram; Discord Snowflakes | If the ID encodes time and a shard or bucket, every lookup is targeted and you can page without `OFFSET`. |
| **Logical ≫ physical** | Instagram schemas; Notion 480/32 | Over-partition on paper, pack onto fewer hosts, move a whole logical shard when one box is hot. Prefer shard counts with many factors. |
| **Shard by the unit of work** | Instagram user; Notion workspace; Discord channel+time | Put the rows that commit and read together on one host. Cross-shard joins are an admission that the key is wrong. |
| **Bound the partition** | Discord time buckets | "Partition by customer" is not a scheme if one customer is unbounded in time or size. Add a window or a salt. |
| **Keep related writes in one transaction** | Notion block tree; Figma metadata in Postgres | The moment a delete and its children span two primaries, you have invented a distributed transaction. |
| **Change the engine before the model** | Discord Cassandra → ScyllaDB | If the data model is right and the implementation is the toil, a compatible engine is cheaper than a rewrite. |
| **Two stores if the workloads disagree** | Figma multiplayer vs LiveGraph | Real-time document state and relational metadata are different problems. Pretending they are one table is how you get a slow canvas *and* a fragile comment thread. |
| **Measure, then cut over** | Notion dark reads; Discord dual reads | Double-write, backfill, compare, then flip. A five-minute window they chose is still better than an untested big-bang. |

See [partitioning](#/lesson/partitioning) for range vs hash vs a fixed partition count, [replication](#/lesson/replication) for the lag bugs these architectures still have, and [MongoDB schema design](#/lesson/mongodb-schema-design) for the same "access pattern first" rule on documents.

> **Interview:** If they ask "would you have sharded earlier?", steal Notion's own retrospective: waiting until VACUUM is already failing closes off the safe migration paths. The opposite error — sharding in month three — is how you freeze a product model you do not yet understand.

```quiz
Q: What do the 13 middle bits of Instagram's published 64-bit ID represent?
- A random salt so IDs cannot be guessed
+ The logical shard, derived from the user ID in their example
- The physical server number, rewritten when they add a host
- A CRC so replicas can detect bit flips
? 41 bits of millisecond time, 13 bits of logical shard, 10 bits of sequence. Physical hosts are a mapping in front of those logical shards, so adding a server does not rewrite IDs.

Q: Why did Discord add a time bucket to the Cassandra partition key?
- Cassandra cannot store more than one clustering column
+ A busy channel would otherwise be one unbounded partition, which they sized (~10 days) to stay around 100 MB
- Buckets let them delete history automatically after ten days
- Snowflake IDs are not unique without a bucket
? They keep history. The bucket is derived from the Snowflake so recent reads hit a bounded partition; a quiet channel may scan a few buckets, which they said was acceptable.

Q: Notion sharded by workspace ID and dragged every table reachable from block with it. Why?
- UUIDs hash more evenly than integer primary keys
- Postgres cannot shard a single table
+ Page edits need related rows (blocks, comments, discussions) in one transaction on one host
- Workspaces never grow large, so the key cannot hotspot
? They say the opposite about enterprise workspaces: those are hotter than many personal ones. The key is locality of the unit of work, not perfect balance.

Q: Which statement matches Figma's published multiplayer design?
- They started with CRDTs and later switched to OT
- Every keystroke is a row in Postgres, streamed with LISTEN/NOTIFY
+ They rejected OT as too complex for a scene graph, and use a server-ordered last-writer-wins protocol; Postgres holds metadata, not the live canvas
- LiveGraph is a second copy of the file used as the source of truth
? Multiplayer is an in-memory, process-per-file service with checkpoints (and later a DynamoDB journal). LiveGraph tails Postgres WAL for comments, teams and similar metadata.

Q: What is the SA-reusable reason Instagram and Notion used many logical shards on fewer physical hosts?
+ You can move a schema or logical shard to a new machine without rehashing keys
- Logical shards avoid the need for backups
- One logical shard per row maximises parallelism
- Physical hosts cannot run more than one database
? Over-partition on paper, pack onto today's hardware, rebalance by moving whole shards. Notion even picked 480 because it has many factors.

Q: All four stories share which sequencing?
- Pick the most scalable database on day one, then model the product around it
- Introduce multi-region writes before the first customer
+ Stay on one primary until a named operational limit appears, then shard along an existing access pattern
- Replace SQL with a mesh of microservices so each table can scale independently
? Instagram stayed on Postgres, Notion waited until wraparound risk, Discord moved messages only when MongoDB could not hold them, Figma kept Postgres for metadata. Distributed is a response, not an aesthetic.
```
