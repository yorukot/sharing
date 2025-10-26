.PHONY: help build run clean test install dev

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install: ## Install dependencies
	go mod download
	go mod tidy

build: ## Build the application
	go build -o sharing main.go

run: ## Run the application
	go run main.go

dev: ## Run with auto-reload (requires air: go install github.com/cosmtrek/air@latest)
	air

test: ## Run tests
	go test -v ./...

clean: ## Clean build artifacts and data
	rm -f sharing
	rm -rf data/

setup: ## Initial setup (copy .env.example to .env)
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo ".env file created. Please edit it and set your API_KEY"; \
	else \
		echo ".env file already exists"; \
	fi
	@mkdir -p data
	@mkdir -p templates

fmt: ## Format Go code
	go fmt ./...

lint: ## Run linter (requires golangci-lint)
	golangci-lint run

docker-build: ## Build Docker image
	docker build -t file-sharing:latest .

docker-run: ## Run Docker container
	docker run -p 8080:8080 -v $(PWD)/data:/app/data file-sharing:latest

docker-up: ## Start services with Docker Compose
	docker compose up -d

docker-down: ## Stop services with Docker Compose
	docker compose down

docker-logs: ## View Docker Compose logs
	docker compose logs -f

docker-rebuild: ## Rebuild and restart Docker Compose services
	docker compose up -d --build

docker-clean: ## Stop services and remove volumes (WARNING: deletes all data!)
	docker compose down -v

docker-backup: ## Backup Docker volume data
	@echo "Creating backup of sharing-data volume..."
	@docker run --rm -v sharing_sharing-data:/data -v $(PWD):/backup alpine tar czf /backup/sharing-data-backup-$$(date +%Y%m%d-%H%M%S).tar.gz -C /data .
	@echo "Backup created successfully"

docker-restore: ## Restore Docker volume data (usage: make docker-restore FILE=backup.tar.gz)
	@if [ -z "$(FILE)" ]; then echo "Error: Please specify backup file with FILE=backup.tar.gz"; exit 1; fi
	@echo "Restoring backup from $(FILE)..."
	@docker run --rm -v sharing_sharing-data:/data -v $(PWD):/backup alpine sh -c "rm -rf /data/* && tar xzf /backup/$(FILE) -C /data"
	@echo "Restore completed successfully"
