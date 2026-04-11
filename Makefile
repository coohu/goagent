.PHONY: build build-server build-cli run run-cli test lint fmt \
        migrate-up migrate-down dev docker-build build-sandbox

build: build-server build-cli

build-server:
	go build -o bin/goagent-server ./cmd/server

build-cli:
	go build -o bin/goagent ./cmd/cli

run:
	go run ./cmd/server

run-cli:
	go run ./cmd/cli

test:
	go test ./... -v -race -coverprofile=coverage.out

test-integration:
	go test ./... -v -tags=integration

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

mock:
	go generate ./...

migrate-up:
	migrate -path internal/infra/db/migrations \
		-database "$(POSTGRES_DSN)" up

migrate-down:
	migrate -path internal/infra/db/migrations \
		-database "$(POSTGRES_DSN)" down 1

build-sandbox:
	docker build -f deployments/sandbox.Dockerfile -t goagent-sandbox:latest .

dev:
	docker-compose -f deployments/docker-compose.yml up -d
	sleep 3
	$(MAKE) migrate-up
	$(MAKE) run

docker-build:
	docker build -t goagent:latest .
