## The prompt

> "A streaming media company wants a real-time analytics platform: 2B playback events per day from apps and TVs. Product managers need dashboards within 1 minute of events, data scientists need full history for recommendation models, and finance needs exactly-correct daily royalty reports."

## 1. Clarify

- Event schema and its evolution? Who owns it?
- What does "real time" mean for each consumer? (Dashboards in 1 min, alerting in seconds?)
- Late and out-of-order events (offline TV apps upload hours later)?
- Exactly-once for royalties. Is dedup by event ID possible?
- Retention and privacy (GDPR deletion of user data)?
- Existing cloud and tooling (Kafka? Kinesis? Snowflake?)

## 2. Estimate

| Metric | Value |
|---|---|
| Events | 2B/day ≈ 23k/s average, peak (evenings) ≈ 100k/s |
| Event size | ~1 KB JSON, so ~2 TB/day raw, ~200–300 GB/day as compressed Parquet |
| History (3 years) | ~250 TB compressed |

## 3. Architecture (lakehouse)

```text
Apps/TVs ─▶ Ingest API (Go, stateless) ─▶ Kafka (partitioned by userId/deviceId, 3-day retention)
                                              │
                    ┌─────────────────────────┼──────────────────────────┐
                    ▼                         ▼                          ▼
          Structured Streaming        Alerting / anomaly           Raw archive
          (declarative pipeline)      (streaming job, seconds)     (bronze = replayable)
                    │
  🥉 bronze.playback_events  (append-only, as received, + ingest metadata)
                    │  parse, validate (expectations), dedupe by eventId within watermark
  🥈 silver.playback_events  (typed, cleaned, liquid clustered by (event_date, user_id))
                    │
     ┌──────────────┼────────────────────────────┬───────────────────────────┐
     ▼              ▼                            ▼                           ▼
 🥇 gold.minute_metrics   🥇 gold.daily_royalties          🥇 features.user_watch     Vector Search
 (streaming agg, 1-min)   (batch, exactly-correct,          (ML feature tables)       (content embeddings)
     │                     reprocessed nightly w/ late data)        │
     ▼                            ▼                                 ▼
 AI/BI dashboards / Genie    Finance reports (SQL)          Recommendation model → Model Serving
                    ── all governed by Unity Catalog (access, lineage, audit) ──
```

## 4. Key design decisions

### Kafka partitioning
Key by `deviceId` (or `userId`) to preserve per-device ordering, with enough partitions for peak throughput (for example 200–400) so consumers scale horizontally.

### Late data and watermarks
- Dashboards: a 10-minute watermark on the minute aggregates, updated incrementally.
- Royalties: computed in **batch** from silver after a cut-off (for example 48 hours), so late uploads are included and the numbers are final and reproducible.
- This is the honest answer to "real time *and* exactly correct": **different consumers get different latency vs completeness trade-offs from the same data**.

### Exactly-once
Kafka offsets plus streaming checkpoints plus Delta transactional writes give exactly-once into bronze and silver. Dedup by `eventId` handles client retries (at-least-once from devices):

```python
(silver_stream
   .withWatermark("event_ts", "2 hours")
   .dropDuplicatesWithinWatermark(["event_id"]))
```

### Layout and performance
- **Liquid clustering** on silver by `(event_date, user_id)`; predictive optimisation handles OPTIMIZE and VACUUM.
- Gold tables are small and pre-aggregated, so dashboards hit serverless SQL warehouses (Photon) with sub-second latency.
- Avoid partitioning by high-cardinality columns (the small-files problem).

### Schema evolution
A schema registry for Kafka events. Bronze stores the raw payload and tolerates change; silver uses schema evolution with explicit reviews.

### GDPR deletion
`DELETE FROM silver… WHERE user_id = ?` (deletion vectors make it cheap), then propagate to gold and VACUUM after the retention window, so the data is physically gone. Document lineage in Unity Catalog to prove completeness.

## 5. Alternatives to discuss

| Alternative | When it fits |
|---|---|
| Real-time OLAP store (ClickHouse, Druid, Pinot) for dashboards | Sub-second queries on seconds-fresh data at high concurrency |
| Flink instead of Spark for streaming | Very low latency, complex event processing |
| Snowflake / BigQuery for warehouse | SQL-centric org, less ML |
| MongoDB Atlas Stream Processing + time series | Operational real-time features in the app itself (e.g. "continue watching") |
| Kinesis + Firehose + Athena (AWS-native) | Smaller scale, AWS-only, fewer moving parts |

> **SA angle:** This is the canonical **Databricks** design question. Show the medallion flow, streaming and batch from one copy, governance with Unity Catalog, and ML on the same data. Quantify: "~250 GB/day of Parquet, a serverless SQL warehouse for dashboards, jobs compute for pipelines". For **MongoDB**, the twist is the operational side ("continue watching" and personalised rows served from Atlas with features computed in the lakehouse). For **Red Hat**, it's Kafka on OpenShift with Strimzi, Debezium for catalogue CDC, and a hybrid deployment where ingest runs at the edge or on-prem.

```quiz
Q: Finance needs exactly-correct daily royalties, but events arrive up to 24 hours late. What's the best approach?
- Use the 1-minute streaming aggregates directly
+ Compute royalties in batch from silver after a cut-off that covers late arrivals, so results are final and reproducible
- Drop late events
- Increase Kafka retention to 1 year
? Separate latency needs: dashboards are approximate and fast, finance is complete and final. Both come from the same silver data.

Q: Devices retry uploads, producing duplicate events. Where should deduplication happen?
+ In the bronze-to-silver stream, using event_id with a watermark (dropDuplicatesWithinWatermark)
- On the BI dashboard
- In Kafka by deleting messages
- Nowhere; duplicates are fine for royalties
? Dedup with bounded state in the streaming layer gives a clean silver table for every downstream consumer.

Q: Why avoid partitioning the silver table by user_id?
- user_id isn't a string
+ High cardinality creates millions of tiny files and directories, hurting performance; use liquid clustering instead
- Delta doesn't support partitioning
- It breaks streaming
? Partition (or better, cluster) on columns that produce reasonably sized files.
```
