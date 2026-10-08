## What you'll install

| Tool | Why | Check |
|---|---|---|
| Go 1.22+ | All labs are written in Go | `go version` |
| Docker Desktop (or Podman) | Runs PostgreSQL and MongoDB locally | `docker compose version` |
| A SQL client (optional) | `psql`, pgAdmin, DBeaver or TablePlus | — |
| `mongosh` (optional) | MongoDB shell; also runs inside the container | — |
| VS Code / Cursor + Go extension | Editing and debugging | — |

> **Tip:** Red Hat interviewers love to hear you used **Podman**. It's daemonless and rootless, and `podman compose` works with the same file. Try it once.

## Project layout

```text
db-mastery/
├── main.go                 # serves this portal (go run .)
├── web/                    # the portal (HTML/CSS/JS + lessons/*.md)
├── Makefile
└── labs/
    ├── docker-compose.yml  # PostgreSQL 16 + MongoDB 7 (single-node replica set)
    ├── postgres/           # Lab: PostgreSQL from Go
    ├── mongodb/            # Lab: MongoDB from Go
    ├── kvstore/            # Lab: build a Bitcask KV store
    ├── lsm/                # Lab: build a mini LSM-tree
    ├── hashring/           # Lab: consistent hashing
    └── mvcc/               # Lab: snapshot isolation with MVCC
```

## Start the databases

```bash
cd db-mastery
make up          # docker compose -f labs/docker-compose.yml up -d
make ps          # both containers should be "healthy"
```

Connection details:

| Database | URL | Credentials |
|---|---|---|
| PostgreSQL 16 | `postgres://dbm:dbm@localhost:5442/dbm?sslmode=disable` | user `dbm`, password `dbm` |
| MongoDB 7 | `mongodb://localhost:27017/?directConnection=true` | no auth (local only) |

> **Note:** Postgres is published on host port **5442** (not 5432) so it doesn't clash with a Postgres you may already have installed locally. Override with `DATABASE_URL` if needed.

MongoDB runs as a **single-node replica set** (`rs0`). That's required for transactions and change streams, which you'll use in the lab.

![The portal on :8080 talks to Postgres on 5442 and a one-node MongoDB replica set on 27017.](diagrams/setup-lab-topology.svg)

## Smoke test

```bash
# Postgres: open psql inside the container
docker exec -it dbm-postgres psql -U dbm -c "select version();"

# MongoDB: open mongosh inside the container
docker exec -it dbm-mongo mongosh --eval "rs.status().members[0].stateStr"   # PRIMARY

# Go labs that need no database
go test ./labs/kvstore/... ./labs/lsm/... ./labs/hashring/... ./labs/mvcc/...

# Go labs against the databases
go run ./labs/postgres crud
go run ./labs/mongodb crud
```

## Running the portal

```bash
go run .              # http://localhost:8080
go run . -addr :3000  # another port
```

Your progress, quiz scores and notes are stored in your browser's `localStorage`. Nothing leaves your machine.

## Cloud free tiers worth creating now

- **MongoDB Atlas** has a free M0 cluster. You'll need it for Atlas Search and Vector Search, which don't run in the local container.
- **Databricks Free Edition** is a free workspace for learning Spark, Delta Lake, SQL and AI features.
- **Red Hat Developer Sandbox** gives you a free OpenShift cluster for 30 days at a time. Use it for the Kubernetes lesson.

> **Lab:** Get everything above running, then write in your notes: which ports are used, where the data volumes live (`docker volume ls`), and how you'd reset everything (`make reset`).

> **Interview:** Why does this lab run MongoDB as a one-node replica set, and why is Postgres published on 5442?

> **Model answer:** Transactions and change streams need an oplog, which only exists on replica-set members. A single-member set is the usual local-development trick. Postgres is on 5442 so it does not collide with a system Postgres on 5432. The portal on 8080 serves the lessons; it does not proxy your queries.

```quiz
Q: Why does the local MongoDB container run as a single-node replica set rather than a standalone server?
- Replica sets are faster on a single machine
+ Multi-document transactions and change streams require a replica set (or sharded cluster)
- Standalone mode is deprecated in MongoDB 7
- It enables automatic sharding
? Transactions and change streams rely on the oplog, which only exists on replica set members. A one-member replica set is the standard trick for local development.
```
