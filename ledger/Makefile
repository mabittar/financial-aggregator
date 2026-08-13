.PHONY: help build test test-unit test-integration lint sqlc-generate sqlc-verify migrate-up migrate-down migrate-status clean

# Default target
help:
	@echo "Ledger Service - Available Targets:"
	@echo ""
	@echo "Build & Compile:"
	@echo "  make build              - Compile ledger binary (cmd/server/server)"
	@echo ""
	@echo "Testing:"
	@echo "  make test               - Run all tests (unit + integration)"
	@echo "  make test-unit          - Run unit tests only (fast)"
	@echo "  make test-integration   - Run integration tests with containers (slow)"
	@echo ""
	@echo "Code Quality:"
	@echo "  make lint               - Run go vet and check for code issues"
	@echo ""
	@echo "Database Migrations (golang-migrate CLI required):"
	@echo "  make migrate-up         - Apply all pending migrations"
	@echo "  make migrate-down       - Rollback last migration"
	@echo "  make migrate-status     - Show current migration version"
	@echo ""
	@echo "Maintenance:"
	@echo "  make clean              - Remove build artifacts"
	@echo ""

# Compile ledger binary
build:
	@echo "Building ledger service..."
	go build -o cmd/server/server ./cmd/server
	@echo "✓ Binary created at cmd/server/server"

# Run all tests
test:
	@echo "Running all tests..."
	go test -v -race -coverprofile=coverage.out ./...
	@echo "✓ Tests passed"

# Run unit tests only (tests that don't require containers)
test-unit:
	@echo "Running unit tests..."
	go test -v -race -short ./internal/auth ./internal/config
	@echo "✓ Unit tests passed"

# Run integration tests with testcontainers
test-integration:
	@echo "Running integration tests (requires Docker)..."
	go test -v -race ./internal/db ./internal/handler
	@echo "✓ Integration tests passed"

# Run code linter
lint:
	@echo "Running code quality checks..."
	go vet ./...
	@echo "✓ Code linting passed"

# Apply pending database migrations
migrate-up:
	@if ! command -v migrate >/dev/null 2>&1; then \
		echo "Error: migrate CLI is not installed"; \
		echo "Install it with: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest"; \
		exit 1; \
	fi
	@echo "Applying pending migrations..."
	@. ../infra/.env && \
	migrate -path db/migrations -database "postgresql://$${POSTGRES_USER}:$${POSTGRES_PASSWORD}@db/$${POSTGRES_DB}?sslmode=disable" up
	@echo "✓ Migrations applied"

# Rollback last migration
migrate-down:
	@if ! command -v migrate >/dev/null 2>&1; then \
		echo "Error: migrate CLI is not installed"; \
		exit 1; \
	fi
	@echo "Rolling back last migration..."
	@. ../infra/.env && \
	migrate -path db/migrations -database "postgresql://$${POSTGRES_USER}:$${POSTGRES_PASSWORD}@db/$${POSTGRES_DB}?sslmode=disable" down -steps 1
	@echo "✓ Migration rolled back"

# Check current migration status
migrate-status:
	@if ! command -v migrate >/dev/null 2>&1; then \
		echo "Error: migrate CLI is not installed"; \
		exit 1; \
	fi
	@. ../infra/.env && \
	migrate -path db/migrations -database "postgresql://$${POSTGRES_USER}:$${POSTGRES_PASSWORD}@db/$${POSTGRES_DB}?sslmode=disable" version

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f cmd/server/server coverage.out
	@echo "✓ Cleaned"

# Run service locally (requires migrations to be applied first)
run: build migrate-up
	@echo "Starting ledger service..."
	./cmd/server/server

# Generate type-safe Go queries from SQL using sqlc
sqlc-generate:
	@echo "Generating type-safe Go code with sqlc..."
	@if ! command -v sqlc >/dev/null 2>&1; then \
		echo "Error: sqlc CLI is not installed"; \
		echo "Install it with: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest"; \
		exit 1; \
	fi
	@sqlc generate
	@echo "✓ Generated Go code in db/sqlc/"

# Verify generated sqlc code is up to date
sqlc-verify:
	@echo "Verifying sqlc code is up to date..."
	@if ! command -v sqlc >/dev/null 2>&1; then \
		echo "Error: sqlc CLI is not installed"; \
		exit 1; \
	fi
	@output=$$(mktemp) && \
	sqlc generate && \
	if [ -d "db/sqlc" ]; then \
		git diff --quiet -- db/sqlc/ && echo "✓ sqlc generated code is up to date" || (echo "✗ sqlc generated code is out of date" && git diff db/sqlc/ && exit 1); \
	else \
		echo "✗ No sqlc output directory found"; \
		exit 1; \
	fi; \
	rm -f "$$output"
