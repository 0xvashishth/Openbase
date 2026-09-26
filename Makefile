.PHONY: build test test-short vet lint dev up down clean

build:
	go build -o bin/openbase-server ./cmd/server

test:
	go test ./...

test-short:
	go test -short ./...

vet:
	go vet ./...

# go vet + both TypeScript typechecks. Requires `npm install` in dashboard/
# and sdk/js/ first. (The dashboard has no ESLint config yet, so its
# `npm run lint` would open an interactive setup prompt — use tsc.)
lint: vet
	cd dashboard && npm run typecheck
	cd sdk/js && npm run typecheck

# Boot the full dev stack (API + metadata Postgres).
dev:
	docker compose up --build

up:
	docker compose up -d --build

down:
	docker compose down

clean:
	rm -rf bin