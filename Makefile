COMPOSE = docker compose -f labs/docker-compose.yml

.PHONY: portal dev check up down ps reset test test-labs logs

portal:            ## Run the learning portal on http://localhost:8080
	go run .

dev:               ## Run the portal serving web/ from disk (edits show on refresh)
	go run . -dev

up:                ## Start PostgreSQL and MongoDB
	$(COMPOSE) up -d --wait

down:              ## Stop databases (keeps data)
	$(COMPOSE) down

reset:             ## Stop databases and delete all data
	$(COMPOSE) down -v

ps:
	$(COMPOSE) ps

logs:
	$(COMPOSE) logs -f

check:             ## Validate lessons, quizzes and diagrams
	node scripts/check-content.mjs

test:              ## Run the build-your-own labs (no database needed)
	go test ./labs/kvstore/... ./labs/lsm/... ./labs/hashring/... ./labs/mvcc/...

test-labs: test    ## Run every DB lab end to end (requires `make up`)
	go run ./labs/postgres all
	go run ./labs/mongodb all
