# Load .env file if it exists
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

# Project configuration
PROJECT_NAME := shorehamex2
DB_CONTAINER := shorehamex2db
DB_USER := Francesco99975
DB_HOST := localhost
DB_NAME := shorehamex2db
SERVER_STAGING := Francesco99975@staging.com
SERVER_PRODUCTION := Francesco99975@prod.com
IMAGE_NAME := $(PROJECT_NAME):latest

# Default target
.DEFAULT_GOAL := help

# Helpers
.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Commands
.PHONY: test
test: ## Run all tests
	go test -v -race -bench=. -benchmem ./...

.PHONY: coverage
coverage: ## Run tests with coverage
	go test -race -count=3 -timeout=30m  -bench=. -benchmem -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

.PHONY: dev
dev: ## Run the app in development mode using Air
	air

.PHONY: initdb
initdb: ## Run the app in production mode
	migrate create -ext sql -dir sql/migrations -seq init_schema

# Apply database migrations
.PHONY: migrate-up
migrate-up:
	@if [ -z "$(DSN)" ]; then \
		echo "Error: DSN environment variable not set"; \
		exit 1; \
	fi
	migrate -path sql/migrations -database $(DSN) up

# Revert database migrations
.PHONY: migrate-down
migrate-down:
	@if [ -z "$(DSN)" ]; then \
		echo "Error: DSN environment variable not set"; \
		exit 1; \
	fi
	migrate -path sql/migrations -database $(DSN) down

.PHONY: migrate-new
migrate-new: ## Create a new migration (Usage: make migrate-new NAME=<migration_name>)
	@if [ -z "$(DSN)" ]; then \
		echo "Error: DSN environment variable not set"; \
		exit 1; \
	fi
	@if [ -z "$(NAME)" ]; then \
		echo "Usage: make migrate-new NAME=<migration_name>"; \
		exit 1; \
	fi
	migrate create -ext sql -dir sql/migrations -seq $(NAME)

.PHONY: sql
sql:
	sqlc generate

.PHONY: db
db: ## Connect to the local development database
	docker exec -it $(DB_CONTAINER) psql -U $(DB_USER) -h $(DB_HOST) -d $(DB_NAME)

.PHONY: build
build: ## Build the Docker image
	docker buildx build --load -t $(IMAGE_NAME) .
	docker save $(IMAGE_NAME) > $(PROJECT_NAME).tar
	trivy image --severity HIGH,CRITICAL --exit-code 1 $(IMAGE_NAME)

.PHONY: vulns
vulns: ## Scan the Docker image for vulnerabilities
	./scripts/pentest.sh

.PHONY: run
run: ## Run the app (non-development mode)
	go run .

.PHONY: lint
lint: ## Run linters (requires golangci-lint)
	golangci-lint run

.PHONY: clean
clean: ## Clean up build artifacts
	go clean -modcache
	rm -rf ./bin ./dist
	docker buildx prune -f

.PHONY: deps
deps: ## Download and tidy dependencies
	go mod tidy

.PHONY: fmt
fmt: ## Format the code
	go fmt ./...

.PHONY: vet
vet: ## Analyze code for potential issues
	go vet ./...

.PHONY: ci
ci: lint vet fmt coverage ## Run all checks (tests, lint, vet, format)

.PHONY: staging
staging: ## Deploy to staging environment
	scp $(PROJECT_NAME).tar docker-compose.yml $(SERVER_STAGING):~/apps/$(PROJECT_NAME)

.PHONY: production
production: ## Deploy to production environment
	scp $(PROJECT_NAME).tar docker-compose.yml $(SERVER_PRODUCTION):~/apps/$(PROJECT_NAME)
