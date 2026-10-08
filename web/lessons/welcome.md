You already have something most database candidates lack: **you are an SA at AWS.** You know how to run discovery, map business problems to architecture, present to executives and survive a whiteboard. What you need now is **database depth**: enough theory to reason from first principles, enough hands-on time to be credible in a demo, enough production scar tissue to answer "what would you do if…", and enough product knowledge to position MongoDB, Databricks or Red Hat against the alternatives (including AWS).

This portal is built for exactly that gap. Its spine follows the arc of Martin Kleppmann's *Designing Data-Intensive Applications* (DDIA): foundations and storage, then distribution, then derived data. Around that it adds the products, labs, real-world problems and interview practice an SA role needs.

![How the tracks build on each other: foundations and internals underpin everything; the product tracks apply them; real-world problems, system design and career prep turn knowledge into interview performance.](diagrams/welcome-curriculum-map.svg)

## How the curriculum is structured

| Track | What you get | Why it matters for an SA |
|---|---|---|
| 🧱 1 · Foundations | SQL, modelling, indexes, transactions, specialised stores | Every customer conversation eventually lands here |
| ⚙️ 2 · Internals | Storage engines, WAL, MVCC, the query planner, encoding | Lets you explain *why* something is slow or unsafe |
| 🍃 3 · MongoDB | Document model, schema patterns, sharding, Atlas | Core for MongoDB; useful everywhere |
| 🌐 4 · Distributed | Replication, partitioning, faults, CAP, consensus | The whiteboard questions at all three companies |
| 🧊 5 · Lakehouse | OLAP, Spark, Delta Lake, streaming, Databricks | Core for Databricks |
| 🎩 6 · Cloud-native | Postgres, Kubernetes, Kafka/Debezium, Red Hat | Core for Red Hat |
| 🐹 7 · Go labs | Real code against real databases, plus build-your-own internals | Credibility and demo material |
| 🏗️ 8 · System design | Framework + five worked designs | The architecture interview |
| 🚨 9 · Real-world problems | Slow databases, passwords, oversold stock, migrations, outages, case studies | The "what would you do?" questions, and customer credibility |
| 🎯 10 · Career | Playbooks, discovery, demos, question bank | The rest of the loop |

Each lesson has:

- **Theory** explained from first principles, with diagrams. Click any diagram to enlarge it.
- **Practical snippets**: SQL, MongoDB shell, PySpark or Go you can paste and run against the lab databases.
- **Real-world scenarios** framed the way a customer or interviewer would put them, with collapsible **model answers**. Try answering before you open them.
- **SA angle callouts**: how the concept shows up in customer conversations and interviews.
- **A quiz**: answer before moving on; your score is saved in the browser.
- **A notes box**: write your own explanation. If you can't explain it simply, re-read.

> **Tip:** Progress, quiz answers and notes are stored in your browser's `localStorage`. Nothing leaves your machine. Press `/` anywhere to search every lesson.

## How to study (the method that works)

1. **Read actively.** After each section, close your eyes and explain it out loud as if to a customer's CTO.
2. **Run everything.** Theory without hands-on fades in a week. The labs are where it sticks.
3. **Answer before you reveal.** For every scenario, write or say your answer first, then open the model answer and note the gap.
4. **Teach it back.** Use the notes box to write a two-sentence explanation of every lesson.
5. **Whiteboard weekly.** Pick a system design case and draw it in 35 minutes, timed.
6. **Spaced repetition.** Redo the quizzes from previous weeks every Sunday, plus ten scenarios from the [rapid-fire drill](#/lesson/problem-interview-drill).

## Mapping your AWS experience

You already know much of this under different names. Use the mapping below to build bridges, and in interviews to show fast ramp-up.

| You know (AWS) | Equivalent concept | Where it shows up here |
|---|---|---|
| RDS / Aurora | Managed relational, storage/compute separation | Foundations, Postgres deep dive |
| DynamoDB | Key-value/document, partition keys, GSIs | MongoDB, Partitioning |
| DocumentDB | MongoDB-compatible API (not MongoDB itself) | MongoDB Atlas positioning |
| ElastiCache / MemoryDB | In-memory cache and data structures | Caching & Redis, Specialised stores |
| OpenSearch | Inverted-index search, vector search | Specialised stores, RAG design |
| Redshift / Athena / EMR / Glue | Warehouse, query-on-lake, Spark, ETL | Lakehouse track |
| S3 + Lake Formation | Data lake + governance | Delta Lake, Unity Catalog |
| MSK / Kinesis | Event streaming | Kafka & Debezium |
| DMS | Migration with CDC | Zero-downtime migrations |
| AWS Backup, PITR | Snapshots, log-based recovery | Backups & DR |
| KMS / Secrets Manager | Envelope encryption, credentials | Passwords, encryption & PII |
| EKS / ROSA | Kubernetes / OpenShift on AWS | Databases on Kubernetes |
| Bedrock / OpenSearch vector | GenAI, vector search | RAG design |

> **SA angle:** Every one of these three companies partners **and** competes with AWS. MongoDB Atlas, Databricks and Red Hat OpenShift (ROSA) all run on AWS and are sold through AWS Marketplace. Your AWS background is an asset. Show you can sell *with* AWS, and also explain honestly when the partner's product is the better fit.

## Your 12-week plan

About 8–10 hours a week: roughly two thirds reading and labs, one third practising out loud. If an interview is coming up sooner, pick that company's **target path** on the dashboard instead. It is an ordered list of the lessons that matter most for that role, and the sidebar dims everything else.

| Week | Focus | Lessons | Practice |
|---|---|---|---|
| 1 | Foundations I | [What is a database](#/lesson/what-is-a-database), [Relational model & SQL](#/lesson/relational-model-sql), [Data modelling](#/lesson/data-modeling), [Setup](#/lesson/setup) | Model a domain you know (e.g. your last customer) in 3NF |
| 2 | Foundations II | [Indexes](#/lesson/indexes), [Transactions](#/lesson/transactions-acid), [Advanced SQL](#/lesson/advanced-sql) | [Postgres lab](#/lesson/lab-postgres) |
| 3 | Internals | [Storage engines](#/lesson/storage-engines), [WAL](#/lesson/wal-recovery), [MVCC](#/lesson/mvcc-concurrency), [Query processing](#/lesson/query-processing) | [Build a KV store](#/lesson/lab-build-kv-store) |
| 4 | Real-world I | [The database is slow](#/lesson/problem-slow-database), [Hot keys & pagination](#/lesson/problem-scaling-hotspots), [Caching](#/lesson/problem-caching) | Diagnose a slow query live, out loud, in 10 minutes |
| 5 | Documents | [Document model](#/lesson/document-model), [Schema patterns](#/lesson/mongodb-schema-design), [Indexes & aggregation](#/lesson/mongodb-indexes-aggregation) | [MongoDB lab](#/lesson/lab-mongodb) |
| 6 | Distribution I | [Replication](#/lesson/replication), [Partitioning](#/lesson/partitioning), [Trouble with distributed systems](#/lesson/distributed-trouble), [Replica sets & sharding](#/lesson/mongodb-replication-sharding) | [Consistent hashing & MVCC lab](#/lesson/lab-distributed-building-blocks) |
| 7 | Distribution II | [Consistency & CAP](#/lesson/consistency-cap), [Consensus](#/lesson/consensus-transactions), [Concurrency bugs](#/lesson/problem-concurrency-bugs), [Encoding](#/lesson/encoding-evolution) | Whiteboard: money transfer across two services |
| 8 | Analytics | [OLTP vs OLAP](#/lesson/oltp-olap-columnar), [Spark](#/lesson/spark-fundamentals), [Delta Lake](#/lesson/delta-lake-lakehouse), [Pipelines & streaming](#/lesson/data-pipelines-streaming) | [LSM tree lab](#/lesson/lab-lsm-tree) |
| 9 | Platforms | [Atlas](#/lesson/mongodb-atlas-platform), [Databricks](#/lesson/databricks-platform), [Postgres deep dive](#/lesson/postgres-deep-dive), [Kubernetes](#/lesson/databases-on-kubernetes), [Kafka & CDC](#/lesson/kafka-debezium-cdc), [Red Hat](#/lesson/redhat-hybrid-cloud) | 10-minute product pitch for your target company |
| 10 | Real-world II | [Security & PII](#/lesson/problem-passwords-secrets), [Multi-tenancy](#/lesson/problem-multi-tenant), [Migrations](#/lesson/problem-zero-downtime-migrations), [Backups & DR](#/lesson/problem-backup-dr) | Tell one case study and one outage story as if to a customer |
| 11 | System design | [Framework](#/lesson/system-design-framework), then two designs for your target company | Two timed 45-minute mock designs |
| 12 | Interview loop | [Role transition](#/lesson/sa-role-transition), your company playbook, [Discovery & demos](#/lesson/discovery-demo-whiteboard), [Question bank](#/lesson/interview-question-bank), [Rapid-fire drill](#/lesson/problem-interview-drill) | Full mock loop with a friend: discovery, demo, design, behavioural |

> **Remember:** The goal isn't to finish the portal; it's to be able to *reason out loud* about trade-offs. When you can answer "it depends — on these three things, and here's how I'd find out" for most scenarios, you're ready.

```quiz
Q: What's the most effective way to make theory stick, according to this portal's method?
- Read every lesson twice before doing labs
+ Run the code, then explain the concept out loud as if to a customer
- Memorise the quiz answers
- Skip theory and go straight to the question bank
? Active recall (explaining out loud) plus hands-on practice beats passive re-reading by a wide margin.

Q: How should you frame your AWS background in a MongoDB/Databricks/Red Hat interview?
- Hide it — they're competitors
+ As an asset: these companies partner with AWS, and you understand the ecosystem customers already run on
- Emphasise that AWS services are always better
- Avoid mentioning any cloud provider
? All three run on and co-sell with AWS. Showing you can navigate "partner and competitor" honestly is a strong SA signal.

Q: What should you do before opening a collapsible model answer?
- Nothing — read it straight away to save time
+ Say or write your own answer first, then compare and note the gap
- Skip to the quiz
- Copy it into your notes verbatim
? Retrieval practice works because you attempt the answer first; the model answer then corrects and extends what you produced.

Q: You have a Databricks interview in three weeks. What's the best way to use the portal?
- Work through all ten tracks in order
+ Select the Databricks SA target path on the dashboard and follow its ordered lessons
- Only read the Databricks playbook
- Only do the Go labs
? The target paths order the lessons that matter most for each role, so you cover the essentials first.
```
