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

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
