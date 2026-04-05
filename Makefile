.PHONY: build run test lint fmt wire mock migrate-up migrate-down dev docker-build build-sandbox

build:
	go build -o bin/goagent ./cmd/server

run:
	go run ./cmd/server

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
