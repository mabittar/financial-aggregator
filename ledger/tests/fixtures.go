package tests

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

const templateDB = "aggregator"

func NewTestDB(t *testing.T) (*pgxpool.Pool, string, func()) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := NewTestConfig(t)

	// Get the root connection
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("failed to open root connection: %v", err)
	}
	dbRoot, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err := dbRoot.Ping(ctx); err != nil {
		dbRoot.Close()
		t.Fatalf("failed to ping Postgres root at %q: %v", cfg.DatabaseURL, err)
	}

	var templateExists int
	if err := dbRoot.QueryRow(ctx,
		"SELECT 1 FROM pg_database WHERE datname = $1", templateDB,
	).Scan(&templateExists); err == sql.ErrNoRows {
		dbRoot.Close()
		t.Fatalf("template database %q does not exist", templateDB)
	} else if err != nil {
		dbRoot.Close()
		t.Fatalf("failed to check template database: %v", err)
	}

	dbName := strings.ToLower(fmt.Sprintf("test_%s_%d", sanitizeDBName(t.Name()), rand.Intn(100000)))

	createQuery := fmt.Sprintf("CREATE DATABASE %s WITH TEMPLATE %s;", dbName, templateDB)
	if _, err := dbRoot.Exec(ctx, createQuery); err != nil {
		dbRoot.Close()
		t.Fatalf("failed to create test database %q from template %q: %v", dbName, templateDB, err)
	}

	var dbExists int
	if err := dbRoot.QueryRow(ctx,
		"SELECT 1 FROM pg_database WHERE datname = $1", dbName,
	).Scan(&dbExists); err != nil {
		dbRoot.Close()
		t.Fatalf("database %q creation reported success but it does not exist: %v", dbName, err)
	}

	newConnStr := replaceDBName(t, cfg.DatabaseURL, dbName)
	newPoolConfig, err := pgxpool.ParseConfig(newConnStr)
	if err != nil {
		dbRoot.Close()
		t.Fatalf("failed to open test database %q: %v", dbName, err)
	}
	db, err := pgxpool.NewWithConfig(ctx, newPoolConfig)

	var pingErr error
	for i := 0; i < 5; i++ {
		pingErr = db.Ping(ctx)
		if pingErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if pingErr != nil {
		db.Close()
		dbRoot.Close()
		t.Fatalf("failed to ping test database %q after retries: %v", dbName, pingErr)
	}

	if err := runMigrations(ctx, db); err != nil {
		db.Close()
		dbRoot.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}

	cleanup := func() {
		db.Close()

		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		termQuery := fmt.Sprintf(`
			SELECT pg_terminate_backend(pid)
			FROM pg_stat_activity
			WHERE datname = '%s' AND pid <> pg_backend_pid()`, dbName)
		_, _ = dbRoot.Exec(cleanupCtx, termQuery)

		dropQuery := fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName)
		if _, err := dbRoot.Exec(cleanupCtx, dropQuery); err != nil {
			t.Logf("warning: could not drop test database %q: %v", dbName, err)
		}

		dbRoot.Close()
	}

	return db, newConnStr, cleanup
}

func runMigrations(ctx context.Context, db *pgxpool.Pool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	sqlDB := stdlib.OpenDB(*db.Config().ConnConfig)
	defer sqlDB.Close()

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("error creating migration driver: %w", err)
	}

	mInstance, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath(),
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("error creating migration instance: %w", err)
	}

	if err := mInstance.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("error running migrations: %w", err)
	}

	return nil
}

func migrationsPath() string {
	_, b, _, _ := runtime.Caller(0)
	basePath := filepath.Dir(b)
	return filepath.Join(basePath, "..", "db", "migrations")
}

func sanitizeDBName(name string) string {
	r := strings.NewReplacer("-", "_", "/", "_", " ", "_", ".", "_")
	return r.Replace(name)
}

func replaceDBName(t *testing.T, rawURL string, newDBName string) string {
	t.Helper()

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("error parsing DATABASE_URL: %v", err)
	}

	parsedURL.Path = "/" + strings.TrimPrefix(newDBName, "/")
	return parsedURL.String()
}

// runMigrationsIfNotApplied checks whether a migration has already been applied before attempting to apply it again.
// Returns nil if the migration is idempotent (already applied or no pending migrations).
// Returns an error if the migration fails for a non-idempotent reason.
func runMigrationsIfNotApplied(ctx context.Context, db *sql.DB) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("error creating migration driver: %w", err)
	}

	mInstance, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath(),
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("error creating migration instance: %w", err)
	}

	// Check current migration version
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("error checking migration version: %w", err)
	}

	// If already at the latest version, this migration is idempotent — skip it.
	if err == migrate.ErrNoChange {
		return nil
	}

	// Apply pending migrations.
	if err := mInstance.Up(); err != nil {
		return fmt.Errorf("error running migrations: %w", err)
	}

	return nil
}
