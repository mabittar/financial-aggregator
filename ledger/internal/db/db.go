package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// DB exposes a Postgres-backed database connection.
type DB struct {
	Conn *sql.DB
}

// Connect initializes a Postgres connection pool and verifies connectivity.
func Connect(ctx context.Context, dsn string) (*DB, error) {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(15 * time.Minute)

	if err := conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return &DB{Conn: conn}, nil
}

// Close releases the underlying database connection.
func (db *DB) Close() error {
	if db == nil || db.Conn == nil {
		return nil
	}
	return db.Conn.Close()
}

// WithTransaction executes the provided callback in a transaction and binds the authenticated user.
func (db *DB) WithTransaction(ctx context.Context, userID string, fn func(ctx context.Context, tx *sql.Tx) error) error {
	tx, err := db.Conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	if _, err := tx.ExecContext(ctx, "SET LOCAL app.current_user_id = $1", userID); err != nil {
		tx.Rollback()
		return fmt.Errorf("bind user context: %w", err)
	}

	if err := fn(ctx, tx); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}
