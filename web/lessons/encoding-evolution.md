## Why encoding is a compatibility problem

Services do not upgrade in lockstep. For a while, **old and new producers and consumers share the same topic, file or RPC**. The bytes on the wire have to mean something to every version that still runs.

That is schema evolution: changing what a record contains without breaking the other side. It is the same problem as a rolling database migration, applied to messages. You will meet it again on [Kafka, Debezium and CDC](#/lesson/kafka-debezium-cdc) and in [data pipelines](#/lesson/data-pipelines-streaming).

> **Key idea:** Compatibility is a property of a *pair* of schemas (writer and reader), not of a format. JSON can evolve if everyone is careful; Avro can break if you skip the registry.

## JSON, Avro, Protobuf, Thrift

| Format | Schema on the wire? | How fields are identified | Typical home |
|---|---|---|---|
| **JSON** | No (JSON Schema is optional and out of band) | Field *names*, text | APIs, logs, document stores |
| **Avro** | Writer schema is required to decode | Field *names* (plus aliases) | Kafka, Hadoop, data lakes |
| **Protocol Buffers** | Schema on both sides; numbers in the bytes | Field *numbers* | gRPC, many internal RPCs |
| **Thrift** | Same idea as Protobuf | Field *numbers* | Older RPC stacks (historically Facebook) |

JSON wins at *humans*: you can read it, every language parses it, and adding a field often "just works" because unknown keys are ignored. It is painful at *evolution*:

- **No single schema.** Every consumer invents types. A number is a `float64` in one language and a string in another.
- **Numbers are sloppy.** JavaScript cannot represent integers above 2^53 − 1 exactly. Money and IDs silently corrupt.
- **No binary, no date type.** You invent conventions (`createdAt` as ISO-8601, bytes as Base64) and then argue about them.
- **Renames and removals are silent breaks.** There is nothing to refuse an incompatible change unless you add a registry and JSON Schema yourself.
- **Verbose.** Fine for an HTTP API; wasteful as the encoding of a multi-TB log.

Binary schema-driven formats fix the last four. They are not more "enterprisey"; they make rolling upgrades *mechanically checkable*.

## Protobuf and Thrift: field numbers are the contract

On the wire, a Protobuf field is a **number**, a wire type and a value. The name in the `.proto` file is for humans and generated code. That has strict rules:

- **Never reuse a field number.** A new meaning under an old number is decoded as the old type by old binaries. Mark retired numbers `reserved`.
- **Adding an optional field** (a new number) is the safe change: old readers skip unknown fields; new readers treat a missing field as the default. That is how you get *both* backward and forward compatibility.
- **Do not change a field's type.** The bytes will be parsed as the wrong wire type.
- **Avoid `required`.** proto2 required fields make a field that can never disappear. proto3 dropped `required` for this reason.

Thrift's binary and compact protocols follow the same tag-number idea. If you remember one sentence for interviews: *names may change; numbers may not.*

## Avro: writer schema plus reader schema

Avro does not tag fields with numbers. A record is decoded by **resolving two schemas**:

1. The **writer schema** — the schema used when the bytes were produced. It must be available at read time (embedded, or a registry ID on each message).
2. The **reader schema** — the schema the application compiled against.

Resolution matches fields **by name** (and aliases). Then:

- Writer has a field the reader does not → skip it (old code reads new data).
- Reader has a field the writer does not → fill the **default** (new code reads old data). No default means the schemas are incompatible.
- Types must be resolvable (a widening `int` → `long` is fine; `string` → `int` is not).
- Optional fields are a **union** with `null`, usually `["null", "string"]`, and the default is `null`.

> **Gotcha:** Avro is not "schemaless JSON in binary". If you lose the writer schema, the bytes are undecodable. That is why Kafka deployments put a schema ID in a five-byte prefix and fetch the writer schema from a registry.

## Backward, forward and full compatibility

Definitions from *Designing Data-Intensive Applications* (and the Confluent registry, which uses the same sense):

| Word | Meaning | Safe rolling order |
|---|---|---|
| **Backward** | *New* code can read data written by *old* code | Upgrade **consumers** first, then producers |
| **Forward** | *Old* code can read data written by *new* code | Upgrade **producers** first, then consumers |
| **Full** | Both | Either order; required when one process both produces and consumes |

![Backward compatibility is a new reader filling defaults for missing fields. Forward compatibility is an old reader skipping fields it does not know.](diagrams/encoding-evolution-compat.svg)

A change is compatible only in a *direction*. Examples:

| Change | Backward | Forward |
|---|---|---|
| Add a field with a default (Avro) or an optional new number (Protobuf) | Yes | Yes |
| Add a field *without* a default | No — new readers cannot decode old records | Yes — old readers skip it |
| Remove a field that had a default | Yes — new readers ignore it | No — old readers expected it |
| Rename without an Avro alias / reuse a Protobuf number | No | No |

> **Warning:** People swap the words. In an interview, say the sentence: *"backward means the new consumer can still read yesterday's messages."* If they use Confluent's `BACKWARD` mode, that is exactly this sense.

## Schema registries

A **schema registry** stores versioned schemas, hands out IDs, and **rejects a new version that breaks the configured compatibility mode**. Producers register (or look up) a schema before writing; consumers fetch the writer schema by ID.

| Registry | Where you meet it |
|---|---|
| **Confluent Schema Registry** | Kafka (self-managed or Confluent Cloud); Avro, Protobuf, JSON Schema |
| **AWS Glue Schema Registry** | MSK, Kinesis, Glue jobs |
| **Apicurio Registry** | Red Hat / Streams for Apache Kafka on OpenShift |

The registry is not optional ceremony. Without it, Avro has no writer schema at the consumer, and nothing stops someone from shipping an incompatible `.avsc` on Friday night.

## Rolling upgrades

While you roll a deployment, v1 and v2 producers and v1 and v2 consumers are all alive. The topic contains a mix of encodings.

![During a rolling upgrade, v1 and v2 producers and consumers share one topic. Full compatibility is what makes that mix safe; a registry is what refuses an unsafe schema.](diagrams/encoding-evolution-rolling.svg)

Practical rules:

- Prefer **full compatibility** for any public topic. Adding an optional field with a default is the change you can always ship.
- If the topic is **backward-only**, finish the consumer roll *before* any producer writes the new schema.
- If it is **forward-only**, finish the producer roll first, and do not remove fields the old consumers still read.
- A service that both produces and consumes (a stream processor) needs **full** compatibility, because it is old-reader-of-new-data *and* new-reader-of-old-data at once.
- Databases use the same idea as **expand/contract**: add the new column (expand), dual-write, flip readers, then drop the old column (contract). Never rename in place.

> **SA angle:** Databricks interviews ask this around Avro/Protobuf on Kafka into Spark and Delta. Red Hat interviews ask it around AMQ Streams and Apicurio. MongoDB interviews ask the cousin question: additive document fields plus schema validation, not a wire registry. Same instinct — additive, defaulted, reversible.

## Problem: rolling upgrade of the order service

> **Real world:** `orders` is a Kafka topic, Avro-encoded, 40 producer pods, 20 consumer pods, Confluent Schema Registry in `BACKWARD` mode. You need to add `promoCode` (optional string) without downtime and without poison messages.

**Symptoms.** The change is a new field. If you get the order wrong, new producers write v2, old consumers throw on decode, the consumer group stalls, and the DLQ fills.

**Diagnose.** Read the subject's compatibility mode and the current schema. Confirm whether any process is both a producer and a consumer of `orders`. Check that the new field has a default (`null` in a `["null","string"]` union).

**Root causes.** Shipping producers first under `BACKWARD` is the usual foot-gun: old consumers cannot read the new writer schema. A field without a default is not backward compatible even if you upgrade consumers first.

**Fix.** Add `promoCode` with a default. Register the schema; the registry should accept it under `BACKWARD` (and under `FULL` if you switch). Roll **consumers** to the new reader schema, wait until the group is stable, then roll **producers**. If anything both produces and consumes, set the subject to `FULL` first.

**Prevent.** Make the registry the gate in CI: a schema that fails `FULL` (or the agreed mode) fails the build. Never publish a payload that was encoded against an unregistered schema.

> **Model answer — what to say in the interview:** I would add `promoCode` as an optional Avro field with a null default so old records still decode. Under backward compatibility the new consumer can read old messages, so I upgrade consumers first, then producers. If any service both reads and writes the topic I would require full compatibility, because that process is an old reader of new data as well. The registry is the safety check: I register before rolling, and I do not let a producer use a schema the registry rejected. JSON would force me to invent that discipline myself.

```quiz
Q: "Backward compatible" means:
+ New code can read data written by old code
- Old code can read data written by new code
- Producers and consumers must restart together
- Field names can never change
? Backward = new reader, old data. That is why you upgrade consumers first in BACKWARD mode.

Q: Why must you never reuse a Protobuf field number?
- The compiler forbids new numbers above 15
+ Old binaries still decode that number as the previous field and type, so the bytes silently mean the wrong thing
- Kafka partitions are numbered from field IDs
- Avro resolution uses the same numbers
? The number *is* the field. Retired numbers go in reserved.

Q: An Avro reader has a field the writer never wrote. What makes this safe?
- Avro tags every field with a number, so the reader can skip
- The consumer falls back to JSON
+ The reader schema supplies a default for that field
- The registry rewrites the old bytes
? Resolution is by name. Missing writer fields become defaults; no default means incompatible.

Q: JSON is painful for evolution mainly because:
- It cannot represent nested objects
+ There is no shared schema or compatibility check, and types (especially numbers and dates) are conventional
- It is a binary format that needs a writer schema
- Field numbers collide after field 15
? Human-readable is the win. The loss is mechanical compatibility.

Q: A Kafka subject is in BACKWARD mode. You add an optional field with a default. Safe order?
- Producers first, then consumers
- Brokers first, then everyone else
- Either order is forbidden
+ Consumers first, then producers
? New consumers must already understand the old encoding before anyone writes the new one. FULL would allow either order.

Q: What does a schema registry do that a Git repo of .avsc files does not?
- It stores the data as well as the schema
+ It issues IDs used on the wire and refuses a version that breaks the configured compatibility mode
- It replaces Kafka consumer groups
- It converts JSON to Protobuf automatically
? The ID is how the consumer fetches the *writer* schema. The compatibility gate is how you stop Friday-night poison messages.
```
