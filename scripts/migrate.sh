#!/bin/bash
# scripts/migrate.sh - Convenience script for running golang-migrate
# 
# Usage:
#   ./scripts/migrate.sh up      # Apply all pending migrations
#   ./scripts/migrate.sh down    # Rollback last migration
#   ./scripts/migrate.sh version # Show current migration version

set -e

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Load environment variables from infra/.env
if [ -f "$PROJECT_ROOT/infra/.env" ]; then
    export $(cat "$PROJECT_ROOT/infra/.env" | grep -v '^#' | xargs)
else
    echo "Error: infra/.env file not found at $PROJECT_ROOT/infra/.env"
    exit 1
fi

# Check if migrate CLI is installed
if ! command -v migrate &> /dev/null; then
    echo "Error: golang-migrate CLI is not installed"
    echo ""
    echo "Install it with one of these commands:"
    echo "  macOS:     brew install golang-migrate"
    echo "  Any OS:    go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest"
    exit 1
fi

# Construct database URL
DATABASE_URL="postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@db/${POSTGRES_DB}?sslmode=disable"

# Get the migration command (default to 'up' if not specified)
COMMAND="${1:-up}"

# Execute migration
case "$COMMAND" in
    up)
        echo "Applying pending migrations..."
        migrate -path "$PROJECT_ROOT/ledger/db/migrations" -database "$DATABASE_URL" up
        echo "✓ Migrations applied"
        ;;
    down)
        echo "Rolling back last migration..."
        migrate -path "$PROJECT_ROOT/ledger/db/migrations" -database "$DATABASE_URL" down -steps 1
        echo "✓ Migration rolled back"
        ;;
    version)
        echo "Current migration version:"
        migrate -path "$PROJECT_ROOT/ledger/db/migrations" -database "$DATABASE_URL" version
        ;;
    *)
        echo "Usage: $0 {up|down|version}"
        echo ""
        echo "Commands:"
        echo "  up       - Apply all pending migrations"
        echo "  down     - Rollback last migration (single step)"
        echo "  version  - Show current migration version"
        exit 1
        ;;
esac
