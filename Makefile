ifneq ($(wildcard .env),)
include .env
export
else
$(warning WARNING: .env file not found! Using .env.example)
include .env.example
export
endif

export PROJECT_ROOT=$(shell pwd)

BASE_STACK = docker compose -f docker-compose.yml

# HELP =================================================================================================================
# This will output the help for each task
.PHONY: help

help: ## Display this help screen
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

compose-up: ### Run docker compose (without backend and reverse proxy)
	@$(BASE_STACK) up --build -d db jaeger
	@$(BASE_STACK) logs -f
.PHONY: compose-up

compose-up-all: ### Run docker compose (with backend and reverse proxy)
	@$(BASE_STACK) up --build -d
.PHONY: compose-up-all

compose-down: ### Down docker compose
	@$(BASE_STACK) down --remove-orphans
.PHONY: compose-down

deps: ### deps tidy + verify
	@go mod tidy && go mod verify
.PHONY: deps

deps-audit: ### check dependencies vulnerabilities
	@govulncheck ./...
.PHONY: deps-audit

fix-diff: ### Show code changes by `go fix`
	@go fix -diff ./...
.PHONY: fix-diff

format: ### Run code formatter
	@go fix ./...
	@gofumpt -l -w .
	@gci write . --skip-generated -s standard -s default
.PHONY: format

run: deps
	@export LOG_FOLDER=${PROJECT_ROOT}/out/logs && \
	go mod download && \
	CGO_ENABLED=0 go run -tags migrate ./cmd/app
.PHONY: run

ps: ### Show status containers
	@docker compose ps
.PHONY: ps

docker-rm-volume: ### remove docker volume
	@docker volume rm ${PROJECT_ROOT}_pg-data
.PHONY: docker-rm-volume

linter-golangci: ### check by golangci linter
	@golangci-lint run
.PHONY: linter-golangci

linter-hadolint: ### check by hadolint linter
	@git ls-files --exclude='Dockerfile*' --ignored | xargs hadolint
.PHONY: linter-hadolint

linter-dotenv: ### check by dotenv linter
	@dotenv-linter
.PHONY: linter-dotenv

test: ### run test and generate HTML coverage report
	@go test -v -race -covermode atomic -coverprofile=coverage.out ./internal/... ./pkg/...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
.PHONY: test

MIGRATIONS_DIR ?= migrations
GOOSE_DB       ?= postgres '$(PG_URL)?sslmode=disable'

migrate-create: ## migration create (make migrate-create name=create_users)
	@if [ -z "$(name)" ]; then \
		echo 'The required "name" parameter is missing. Example: make migrate-create name=create_users'; \
		exit 1; \
	fi
	@goose -dir $(MIGRATIONS_DIR) create -s "$(name)" sql
.PHONY: migrate-create

migrate-action: ## goose action (make migrate-action action=up)
	@if [ -z "$(action)" ]; then \
		echo 'The required "action" parameter is missing. Example: make migrate-action action=up'; \
		exit 1; \
	fi
	@goose -dir $(MIGRATIONS_DIR) $(GOOSE_DB) $(action)
.PHONY: migrate-action

migrate-up: ## migration up
	@$(MAKE) migrate-action action=up
.PHONY: migrate-up

migrate-down: ## migration down
	@$(MAKE) migrate-action action=down
.PHONY: migrate-down

migrate-status: ## migration status
	@$(MAKE) migrate-action action=status
.PHONY: migrate-status

bin-deps: ### install tools
	@go install tool
	@go install -tags 'postgres' github.com/pressly/goose/v3/cmd/goose@v3.29.0
.PHONY: bin-deps

pre-commit: deps format linter-golangci test ### run pre-commit
.PHONY: pre-commit