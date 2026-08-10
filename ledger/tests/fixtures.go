package tests

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestDBConfig holds database configuration for tests
type TestDBConfig struct {
	User     string
	Password string
	Database string
	Host     string
	Port     int
}

// NewTestDB starts a PostgreSQL container, runs migrations, and returns a database connection.
// The cleanup function should be deferred to ensure the container is stopped.
func NewTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create PostgreSQL container
	req := testcontainers.ContainerRequest{
		Image:        "postgres:18.4",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "testuser",
			"POSTGRES_PASSWORD": "testpass",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	// Get the container host and port
	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to get container port: %v", err)
	}

	// Construct connection string
	dsn := fmt.Sprintf("postgresql://testuser:testpass@%s:%s/testdb?sslmode=disable",
		host, port.Port())

	// Connect to the database
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to connect to test database: %v", err)
	}

	// Verify the connection
	if err := db.PingContext(ctx); err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to ping test database: %v", err)
	}

	// Run migrations
	if err := runMigrations(ctx, db); err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Cleanup function
	cleanup := func() {
		db.Close()
		container.Terminate(context.Background())
	}

	return db, cleanup
}

// runMigrations applies all database migrations for testing
func runMigrations(ctx context.Context, db *sql.DB) error {
	migrations := []string{
		`CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`,
		// 000001_create_users_table
		`CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT uuidv7(),
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			display_name TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);`,
		// 000002_create_monthly_statements
		`CREATE TABLE IF NOT EXISTS monthly_statements (
			id UUID PRIMARY KEY DEFAULT uuidv7(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			portfolio_name TEXT NOT NULL,
			reference_date DATE NOT NULL,
			ingest_key TEXT NOT NULL,
			raw_payload JSONB NOT NULL,
			parsed_payload JSONB NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			source TEXT NOT NULL,
			submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (user_id, portfolio_name, reference_date, ingest_key)
		);`,
		// 000003_create_idempotency_keys
		`CREATE TABLE IF NOT EXISTS idempotency_keys (
			key TEXT PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			payload_hash TEXT NOT NULL,
			response_metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);`,
	}

	for _, migration := range migrations {
		if _, err := db.ExecContext(ctx, migration); err != nil {
			return fmt.Errorf("failed to execute migration: %w", err)
		}
	}

	return nil
}
