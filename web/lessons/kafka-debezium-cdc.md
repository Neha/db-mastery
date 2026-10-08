## Why event streaming?

Batch ETL moves data hourly or nightly. Modern businesses want **real-time** reactions: fraud detection, inventory updates, personalisation, cache invalidation, search indexing. **Apache Kafka** is the standard backbone for this.

## Kafka core concepts

```text
Producers ──▶ Topic "orders" ──────────────────────────────────▶ Consumer group A (billing)
              ├── Partition 0: [0][1][2][3][4][5] ...  ──┐        ├─ consumer 1 ← P0
              ├── Partition 1: [0][1][2][3] ...          ├──▶    └─ consumer 2 ← P1, P2
              └── Partition 2: [0][1][2][3][4] ...     ──┘
                                                          ──▶ Consumer group B (analytics) reads independently
```

| Concept | Meaning |
|---|---|
| **Topic** | Named stream of records (like a table of events) |
| **Partition** | Ordered, append-only log. The unit of parallelism and ordering |
| **Offset** | Position of a record in a partition |
| **Key** | Determines the partition (`hash(key) % partitions`), so the same key keeps its order |
| **Broker** | Kafka server storing partitions |
| **Replication factor** | Copies of each partition (usually 3); one leader, with followers in the ISR (in-sync replicas) |
| **Consumer group** | Consumers sharing work; each partition is consumed by one member of the group |
| **Retention** | Time or size based, or **compacted** (keep the latest value per key) |
| **KRaft** | Kafka's built-in Raft metadata quorum (ZooKeeper removed in Kafka 4.0) |

Key properties:

- **Ordering is per partition**, not global. Use a key (for example `orderId`) to keep related events in order.
- Consumers **pull** and track offsets themselves. Data isn't deleted when read, so many consumers can replay independently.
- Durability: `acks=all` plus `min.insync.replicas=2` with a replication factor of 3.
- Delivery semantics: at-most-once, at-least-once (the default; make consumers idempotent), and exactly-once (idempotent producers plus transactions, within Kafka).

> **Key idea:** Kafka is a **distributed, replicated commit log**. You've already learned the WAL and replication. Kafka is those ideas exposed as a product.

## Kafka ecosystem

| Component | Purpose |
|---|---|
| **Kafka Connect** | Framework for source and sink connectors (DBs, S3, Elasticsearch, MongoDB, Snowflake…) |
| **Schema Registry** | Versioned Avro/Protobuf/JSON schemas and compatibility rules (Apicurio Registry at Red Hat) |
| Kafka Streams / Apache Flink | Stream processing (joins, windows, stateful transforms) |
| MirrorMaker 2 | Cross-cluster replication (DR, migration) |

Vendors: Confluent, AWS MSK, Azure Event Hubs (Kafka API), Redpanda, and **Red Hat Streams for Apache Kafka** (Strimzi-based, on OpenShift).

## Change Data Capture (CDC)

CDC turns database changes into a stream of events **by reading the transaction log**. There's no polling and no app changes, and it doesn't miss deletes.

| Approach | Problems |
|---|---|
| Polling (`WHERE updated_at > ?`) | Misses deletes, adds load, latency, clock issues |
| Triggers | Write overhead, fragile |
| Dual writes in app code | Inconsistent on failure |
| **Log-based CDC** ✅ | Reads WAL/binlog/oplog: complete, ordered, low overhead |

## Debezium

**Debezium** (open source, led by Red Hat) is the most widely used log-based CDC platform. It runs as Kafka Connect source connectors (or standalone as **Debezium Server**, sending to Kinesis, Pub/Sub, Redis and others).

| Database | Log Debezium reads |
|---|---|
| PostgreSQL | WAL via logical decoding (`pgoutput`) |
| MySQL / MariaDB | Binlog |
| MongoDB | Change streams (oplog) |
| SQL Server | CDC tables |
| Oracle | LogMiner / XStream |
| Db2, Cassandra, Spanner… | Various |

A Debezium change event (simplified):

```json
{
  "before": { "id": 42, "status": "pending" },
  "after":  { "id": 42, "status": "paid" },
  "source": { "db": "shop", "table": "orders", "lsn": 23987345, "ts_ms": 1791370000000 },
  "op": "u",
  "ts_ms": 1791370000123
}
```

`op` is one of `c` (create), `u` (update), `d` (delete) or `r` (snapshot read). Debezium first takes a **consistent snapshot** of existing data, then streams changes from that exact log position.

```yaml
# Strimzi KafkaConnector resource on OpenShift
apiVersion: kafka.strimzi.io/v1beta2
kind: KafkaConnector
metadata:
  name: orders-cdc
  labels: { strimzi.io/cluster: my-connect }
spec:
  class: io.debezium.connector.postgresql.PostgresConnector
  tasksMax: 1
  config:
    database.hostname: orders-db-rw
    database.port: 5432
    database.user: debezium
    database.password: ${secrets:orders-db/password}
    database.dbname: shop
    topic.prefix: shop
    plugin.name: pgoutput
    table.include.list: public.orders,public.outbox
```

## Patterns built on CDC

1. **Transactional outbox**: the app writes the business row and an outbox event in one DB transaction, and Debezium publishes the outbox (it has a dedicated *outbox event router*). This is reliable microservice events without dual writes.
2. **Cache invalidation and search indexing**: keep Redis or Elasticsearch in sync with the source DB.
3. **Microservice data replication**: share read models without shared databases.
4. **Strangler-fig migration**: stream data from a legacy monolith DB to new services incrementally (a common Red Hat modernisation story).
5. **Feeding the lakehouse**: CDC → Kafka → Delta Lake bronze → `MERGE` into silver (Databricks).
6. **Database migration**: Oracle → Postgres with near-zero downtime.

![The handler writes the order and an outbox row in one database transaction; Debezium reads the WAL and publishes to Kafka for billing, search and the lakehouse.](diagrams/kafka-debezium-cdc-outbox.svg)

> **Interview:** Dual-write the order to Postgres and a Kafka produce in the same handler — what's wrong, and what do you do instead?

> **Model answer:** The two writes are not atomic. The database can commit and the produce fail (or the reverse), so consumers and the source of truth diverge. Write the business row and an outbox row in one database transaction; Debezium (or the outbox event router) publishes from the WAL. At-least-once delivery plus an idempotency key on the consumer.

## Gotchas to mention

- **Replication slots** in Postgres retain WAL until the consumer confirms it. A stopped Debezium connector can **fill the disk**. Monitor slot lag.
- Schema changes must be handled (Schema Registry plus compatibility rules).
- Ordering is guaranteed per key or partition only.
- Large initial snapshots need planning (incremental snapshots help).

> **SA angle:** At **Red Hat**, Debezium plus Streams for Apache Kafka plus OpenShift is a flagship modernisation story ("liberate data from the monolith"). At **Databricks**, CDC feeds bronze tables, and Lakeflow Connect offers managed CDC from databases. At **MongoDB**, change streams and the Kafka Connector (source and sink) integrate MongoDB with event-driven architectures, and Atlas Stream Processing handles stream transforms. Prepare the same architecture diagram with three different emphases.

```quiz
Q: In Kafka, how do you guarantee that all events for the same order are processed in order?
- Use a single consumer for the whole topic
+ Use the order ID as the record key so all its events go to the same partition
- Set replication factor to 1
- Enable log compaction
? Ordering is only guaranteed within a partition, and the key determines the partition.

Q: Why is log-based CDC (e.g. Debezium) preferred over polling an updated_at column?
+ It captures every change including deletes, in commit order, with low overhead and latency
- It doesn't need Kafka
- It works without database permissions
- It avoids snapshots
? Polling misses deletes and intermediate updates and loads the DB. The log already records every committed change.

Q: A Debezium connector for Postgres was stopped for a week and the database disk fills up. Why?
- Debezium writes data into Postgres
+ The logical replication slot retains WAL until the consumer acknowledges it
- Kafka was deleting data
- Autovacuum was disabled by Debezium
? Replication slots prevent WAL removal. Monitor slot lag and drop abandoned slots.

Q: What does Kafka 4.0 use for cluster metadata management?
- ZooKeeper
+ KRaft, Kafka's built-in Raft-based controller quorum
- etcd
- Paxos via Chubby
? KRaft replaced ZooKeeper, simplifying operations and improving scalability.
```
