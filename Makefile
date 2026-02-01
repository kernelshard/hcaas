.PHONY: up down build logs ps clean test tidy help

# Default target
all: up

# Docker Compose Commands
up: ## Start all services with Docker Compose (builds if necessary)
	docker compose up --build -d

down: ## Stop and remove all containers
	docker compose down

build: ## Rebuild services without starting them
	docker compose build

logs: ## Follow logs for all services
	docker compose logs -f

ps: ## List running containers
	docker compose ps

clean: ## Stop containers and remove volumes/orphans
	docker compose down --volumes --remove-orphans

postgres: ## Connect to the main database
	docker compose exec hcaas_db psql -U hcaas_user -d hcaas_db

# Go Development Commands
test: ## Run tests for all services
	@echo "Running tests for all services..."
	cd services/url && go test -v ./...
	cd services/auth && go test -v ./...
	cd services/notification && go test -v ./...

tidy: ## Run go mod tidy for all services
	@echo "Tidying modules..."
	cd services/url && go mod tidy
	cd services/auth && go mod tidy
	cd services/notification && go mod tidy

# Database Migrations

migrate-auth-up: ## Run up migrations for auth service
	cd services/auth && go run cmd/migrate/main.go up

migrate-auth-down: ## Run down migrations for auth service
	cd services/auth && go run cmd/migrate/main.go down

migrate-auth-create: ## Create a new migration file. Usage: make migrate-auth-create name=migration_name
	@if [ -z "$(name)" ]; then echo "Error: name argument is required"; exit 1; fi
	cd services/auth && go run cmd/migrate/main.go create $(name)

migrate-notification-up: ## Run up migrations for notification service
	cd services/notification && go run cmd/migrate/main.go up

migrate-notification-down: ## Run down migrations for notification service
	cd services/notification && go run cmd/migrate/main.go down

migrate-notification-create: ## Create a new migration file. Usage: make migrate-notification-create name=migration_name
	@if [ -z "$(name)" ]; then echo "Error: name argument is required"; exit 1; fi
	cd services/notification && go run cmd/migrate/main.go create $(name)

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
