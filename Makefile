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
