.PHONY: help run build test clean lint migrate-up migrate-down migrate-force sqlc fmt vet

# PostgreSQL connection variables
POSTGRES_HOST ?= localhost
POSTGRES_PORT ?= 5440
POSTGRES_DB ?= transcoderdb
POSTGRES_USER ?= transcoder
POSTGRES_PASSWORD ?= transcoder_password
VERSION ?= 1
help:
	@echo "Available commands:"
	@echo "  make run              - Run the application"
	@echo "  make build            - Build the application"
	@echo "  make test             - Run tests"
	@echo "  make test-v           - Run tests with verbose output"
	@echo "  make test-coverage    - Run tests with coverage report"
	@echo "  make clean            - Clean build artifacts"
	@echo "  make lint             - Run linter"
	@echo "  make fmt              - Format code"
	@echo "  make vet              - Run go vet"
	@echo "  make migrate-up       - Run all pending migrations"
	@echo "  make migrate-down     - Rollback last migration"
	@echo "  make migrate-force    - Force migration version"
	@echo "  make sqlc             - Generate sqlc code"
	@echo "  make swagger          - Generate Swagger documentation"

run:
	go run ./cmd/main.go

build:
	go build -o transcoder ./cmd/main.go

test:
	go test ./...

test-v:
	go test -v ./...

test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -f transcoder coverage.out coverage.html

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

migrate-up:
	migrate -path db/migrations -database "postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=disable" -verbose up

migrate-down:
	migrate -path db/migrations -database "postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=disable" -verbose down

migrate-force:
	migrate -path db/migrations -database "postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=disable" force $(VERSION)

sqlc:
	sqlc generate

swagger:
	swag init -g cmd/main.go
