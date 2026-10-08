# DB Mastery

A self-paced portal from database foundations to Solutions Architect interview readiness — in the spirit of *Designing Data-Intensive Applications*.

Theory and internals, hands-on Go labs against real PostgreSQL and MongoDB, the lakehouse, production problems and postmortems, system design, and SA prep for MongoDB, Databricks and Red Hat.

Progress, quiz scores and notes stay in your browser (`localStorage`). Nothing is sent anywhere.

## Quick start

You need [Go](https://go.dev/dl/) 1.22+ and [Docker](https://docs.docker.com/get-docker/) (or Podman).

```bash
make up        # PostgreSQL on localhost:5442 + MongoDB replica set on localhost:27017
make portal    # learning portal at http://localhost:8080
make dev       # same, but serves web/ from disk so lesson edits show on refresh
```

| | |
|---|---|
| Postgres | `postgres://dbm:dbm@localhost:5442/dbm?sslmode=disable` (port **5442** so it does not clash with a local 5432) |
| MongoDB | `mongodb://localhost:27017/` (single-node replica set `rs0`, no auth) |

`make down` stops the databases. `make reset` also deletes their data volumes.

The portal is a static site (Go only serves files). You can host the `web/` folder on Vercel, Netlify or GitHub Pages. Labs still need Docker on your machine.

## Labs

| Command | What it does |
|---|---|
| `go run ./labs/postgres all` | pgx: schema, COPY, transactions + retry, lost-update anomaly, EXPLAIN, pooling |
| `go run ./labs/mongodb all` | Go driver: CRUD, aggregation, ESR indexes, transactions, change streams |
| `make test` | Build-your-own internals: Bitcask KV store, LSM tree, consistent hashing, MVCC |
| `make test-labs` | Internals tests plus both database labs (needs `make up`) |

Override connection strings with `DATABASE_URL` / `MONGODB_URI`.

## Curriculum

Ten tracks, 61 lessons: foundations and storage, MongoDB, distributed systems, Spark and the lakehouse, Postgres on Kubernetes, Go labs, system design, real-world incidents, and company playbooks.

Open [http://localhost:8080](http://localhost:8080) and pick **Full curriculum**, or a MongoDB / Databricks / Red Hat path.

## Writing content

Lessons are Markdown with callouts, quizzes and collapsible model answers. Diagrams are hand-written SVGs that follow the site theme. See [docs/authoring-guide.md](docs/authoring-guide.md).

```bash
make check                                            # validate lessons, quizzes and diagrams
node scripts/preview-diagram.mjs web/diagrams/x.svg   # PNG preview (needs Chrome)
```

## Layout

```
main.go              static file server (embeds web/; -dev serves from disk)
web/                 portal
web/lessons/*.md     one file per lesson; listed in web/curriculum.js
web/diagrams/*.svg   themed diagrams: ![caption](diagrams/x.svg)
docs/                authoring guide
scripts/             content checker and diagram previewer
labs/                Go labs + docker-compose.yml
```

## License

MIT. Lesson text is yours to reuse with attribution. Company names and product marks belong to their owners. Case studies summarise public postmortems; go to the originals for the record.
