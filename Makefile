.PHONY: build test vet lint dev up down clean

build:
	go build -o bin/openbase-server ./cmd/server

test:
	go test ./...

test-short:
	go test -short ./...

vet:
	go vet ./...

# Boot the full dev stack (API + metadata Postgres).
dev:
	docker compose up --build

up:
	docker compose up -d --build

down:
	docker compose down

clean:
	rm -rf bin