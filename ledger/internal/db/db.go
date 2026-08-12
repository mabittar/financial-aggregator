package db

import (
	"context"
	"fmt"

	"github.com/financial-aggregator/ledger/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is an alias for pgxpool.Pool to keep import names clean.
type Pool = pgxpool.Pool

// Tx is an alias for pgx.Tx, the transaction interface used across the repository layer.
type Tx = pgx.Tx

// DB exposes a PostgreSQL connection pool.
type DB struct {
	Conn *pgxpool.Pool
}

// Connect initializes a PostgreSQL connection pool using environment-variable-driven configuration
// and verifies connectivity with a ping. Pool defaults (min=10, max=50, idleTimeout=30s, connTimeout=10s,
// idleConnLifetime=5m) are applied when environment variables are not set.
func Connect(ctx context.Context, cfg *config.Config) (*DB, error) {
	// Parse the database URL into a pgxpool config
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Apply pool configuration from config
	if cfg.MinConnections > 0 {
		poolCfg.MinConns = int32(cfg.MinConnections)
	}
	if cfg.MaxConnections > 0 {
		poolCfg.MaxConns = int32(cfg.MaxConnections)
	}
	if cfg.IdleTimeout > 0 {
		poolCfg.MaxConnIdleTime = cfg.IdleTimeout
	}
	if cfg.ConnTimeout > 0 {
		poolCfg.ConnConfig.ConnectTimeout = cfg.ConnTimeout
	}
	if cfg.IdleLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.IdleLifetime
	}

	// Create the connection pool
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// Verify connectivity
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &DB{Conn: pool}, nil
}

// Close releases the connection pool resources.
func (d *DB) Close() {
	if d.Conn != nil {
		d.Conn.Close()
	}
}

// WithTransaction begins a transaction, invokes the callback, and commits or rolls back
// based on the returned error. This enables the handler layer to write transaction-scoped
// business logic without leaking pgx.Tx types into the domain.
func (d *DB) WithTransaction(ctx context.Context, userID string, fn func(ctx context.Context, tx Tx) error) error {
	opts := pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadWrite,
	}
	tx, err := d.Conn.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	// Helper to safely roll back in the background
	rollback := func() {
		_ = tx.Rollback(ctx)
	}

	if err := fn(ctx, tx); err != nil {
		rollback()
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
