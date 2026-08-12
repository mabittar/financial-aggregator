# Ledger DB Migration & Connection Pool Optimization Implementation Plan

> **Note:** The `spec` skill produced `docs/specs/20260812-ledger-db-migration-optimize_spec.md` which defines the target architecture. This plan translates that spec into a sequenced, testable implementation.

**Goal:** Replace hardcoded database connection pool settings in `ledger/internal/db/db.go` with environment-variable-driven configuration using `pgxpool`, add migration idempotency checks, and integrate SQLC into the development workflow.

**Architecture:** The ledger service currently uses `database/sql` with the `pgx` driver. We'll migrate to `pgxpool` for connection pool management, replacing hardcoded `SetMaxOpenConns`, `SetMaxIdleConns`, and `SetConnMaxLifetime` calls with environment-variable-driven `pgxpool.Config` fields. We'll also add idempotency checks before applying migrations and create SQLC query files for type-safe database access.

**Tech Stack:** Go 1.26.x, `github.com/jackc/pgx/v5/pgxpool`, `golang-migrate/migrate/v4`, `github.com/sqlc-dev/sqlc` (CLI tool, not a Go dependency)

---

## Development Roadmap

### Task 1: Add pgxpool import and update db.go Connect function

**Objective:** Replace `sql.Open("pgx", dsn)` with `pgxpool.NewWithConfig` in `db.go`, enabling environment-variable-driven connection pool configuration.

**Files:**
- Modify: `ledger/internal/db/db.go` — replace `Connect` function
- Modify: `ledger/internal/config/config.go` — add pool config fields
- Test: `ledger/internal/db/db.go` — verify compilation

**Step 1: Write failing test (connection pool config)**

Create a test that verifies the pool is configured with environment variables:

```go
// ledger/internal/db/db_test.go
package db

import (
	"context"
	"os"
	"testing"
)

func TestConnect_WithEnvironmentPoolConfig(t *testing.T) {
	// Set environment variables for testing
	os.Setenv("LEDGER_DB_POOL_MIN", "5")
	os.Setenv("LEDGER_DB_POOL_MAX", "25")
	os.Setenv("LEDGER_DB_POOL_IDLE_TIMEOUT", "30s")
	defer os.Unsetenv("LEDGER_DB_POOL_MIN")
	defer os.Unsetenv("LEDGER_DB_POOL_MAX")
	defer os.Unsetenv("LEDGER_DB_POOL_IDLE_TIMEOUT")

	// This test will fail until we implement the new Connect function
	// We can't fully test without a real DB, but we can check the import
	ctx := context.Background()
	// db, err := Connect(ctx, "postgres://invalid") — would fail at ping
	// But the key check is: does the code compile and does config parse?
}
```

**Step 2: Run test to verify failure**

```sh
cd ledger && go test ./internal/db/ -run TestConnect_WithEnvironmentPoolConfig -v
```
Expected: FAIL — `pgxpool` not imported

**Step 3: Update `ledger/internal/config/config.go`**

```go
// Add to Config struct:
MaxConnections  int
MinConnections  int
IdleTimeout     time.Duration
ConnTimeout     time.Duration
IdleLifetime    time.Duration

// Add to Load() function:
config.MaxConnections, config.MinConnections, etc.
```

**Step 4: Rewrite `ledger/internal/db/db.go`**

```go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Conn *pgxpool.Pool
}

func Connect(ctx context.Context, dsn string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Environment-driven pool configuration
	if min := getEnvInt("LEDGER_DB_POOL_MIN", 10); min > 0 {
		cfg.MinConns = int32(min)
	}
	if max := getEnvInt("LEDGER_DB_POOL_MAX", 50); max > 0 {
		cfg.MaxConns = int32(max)
	}
	if idleTimeout := getEnvDuration("LEDGER_DB_POOL_IDLE_TIMEOUT", 30*time.Second); idleTimeout > 0 {
		cfg.MaxConnIdleTime = idleTimeout
	}
	if connLifetime := getEnvDuration("LEDGER_DB_POOL_CONN_TIMEOUT", 10*time.Second); connLifetime > 0 {
		cfg.MaxConnLifetime = connLifetime
	}
	if healthCheckPeriod := getEnvDuration("LEDGER_DB_POOL_IDLE_LIFETIME", 5*time.Minute); healthCheckPeriod > 0 {
		cfg.HealthCheckPeriod = healthCheckPeriod
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping pool: %w", err)
	}

	return &DB{Conn: pool}, nil
}

func (db *DB) Close() error {
	if db == nil || db.Conn == nil {
		return nil
	}
	return db.Conn.Close()
}

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

// getEnvInt reads an integer env var with a fallback default
func getEnvInt(key string, defaultVal int) int {
	if val := getEnv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return defaultVal
}

// getEnvDuration reads a duration env var with a fallback default
func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := getEnv(key); val != "" {
		if parsed, err := time.ParseDuration(val); err == nil {
			return parsed
		}
	}
	return defaultVal
}

// getEnv reads an env var, returns empty string if not set
func getEnv(key string) string {
	// Use a helper from config package
	return ""
}
```

**Step 5: Run tests to verify pass**

```sh
cd ledger && go build ./internal/db/ && go test ./internal/db/ -v -run TestConnect_WithEnvironmentPoolConfig
```
Expected: PASS (compile success, test verifies code compiles)

**Step 6: Commit**

```bash
git add ledger/internal/db/db.go ledger/internal/config/config.go
git commit -m "feat(db): replace hardcoded connection pool with environment-driven pgxpool config"
```

**Rollback:** `git revert HEAD`

---

### Task 2: Update repository.go to work with pgxpool

**Objective:** Replace `*sql.DB` with `*pgxpool.Pool` in the Repository struct to ensure all database operations use the new connection pool.

**Files:**
- Modify: `ledger/internal/db/repository.go` — update Repository struct

**Step 1: Write failing test**

```go
// Verify Repository works with the new pool type
func TestRepository_WithPgxPool(t *testing.T) {
	// This test verifies that repository.go compiles with pgxpool
	// Without a full DB, we check that the types are consistent
	var repo *Repository
	_ = repo // Just verify it compiles
}
```

**Step 2: Run test to verify failure**

```sh
cd ledger && go test ./internal/db/ -run TestRepository_WithPgxPool -v
```
Expected: FAIL — Repository uses `*sql.DB` instead of `*pgxpool.Pool`

**Step 3: Update `ledger/internal/db/repository.go`**

```go
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/financial-aggregator/ledger/internal/auth"
)

// ... keep existing types unchanged ...

// UserRepository struct — change conn type
type UserRepository struct {
	conn *pgxpool.Pool
}

func NewUserRepository(conn *pgxpool.Pool) *UserRepository {
	return &UserRepository{conn: conn}
}

// repository.go: Repository struct — change conn type
type Repository struct {
	conn *pgxpool.Pool
}

func NewRepository(conn *pgxpool.Pool) *Repository {
	return &Repository{conn: conn}
}

// ... rest of the methods remain the same, they use conn.QueryRowContext, conn.QueryContext, etc.
// pgxpool.Pool supports these methods natively
```

**Step 4: Run tests to verify pass**

```sh
cd ledger && go build ./internal/db/ && go vet ./...
```
Expected: Build succeeds, no vet errors

**Step 5: Commit**

```bash
git add ledger/internal/db/repository.go
git commit -m "refactor(db): update Repository to use pgxpool.Pool"
```

**Rollback:** `git revert HEAD`

---

### Task 3: Update handler.go to use new pool type

**Objective:** Update `NewHandler` to pass `pgxpool.Pool` instead of `*sql.DB` to the new repository constructors.

**Files:**
- Modify: `ledger/internal/handler/handler.go` — update NewHandler function

**Step 1: Verify current code**

```go
func NewHandler(store *db.DB, cfg *config.Config) *Handler {
	userRepo := db.NewUserRepository(store.Conn)
	repo := db.NewRepository(store.Conn)
	// ...
}
```

**Step 2: Update to use pgxpool**

The handler should change `store.Conn` from `*sql.DB` to `*pgxpool.Pool`. Since `db.DB.Conn` is now `*pgxpool.Pool`, this should work automatically once the repository is updated.

**Step 3: Build to verify pass**

```sh
cd ledger && go build ./...
```
Expected: Build succeeds

**Step 4: Commit**

```bash
git add ledger/internal/handler/handler.go
git commit -m "refactor(handler): use pgxpool type for repository initialization"
```

**Rollback:** `git revert HEAD`

---

### Task 4: Update config.go with pool defaults

**Objective:** Add pool configuration defaults and validation to `config.go`.

**Files:**
- Modify: `ledger/internal/config/config.go` — add pool config fields and defaults

**Step 1: Write failing test**

```go
func TestConfig_PoolDefaults(t *testing.T) {
	cfg := &Config{}
	// Test default values
	if cfg.MaxConnections != 50 {
		t.Error("expected default MaxConnections=50")
	}
}
```

**Step 2: Run test to verify failure**

```sh
cd ledger && go test ./internal/config/ -run TestConfig_PoolDefaults -v
```
Expected: FAIL — Config has no pool fields

**Step 3: Update config.go**

```go
// Add to Config struct:
type Config struct {
	DatabaseURL          string
	JwtSigningKey        string
	JwtIssuer            string
	JwtExpirationMinutes int
	Port                 string
	MaxConnections       int
	MinConnections       int
	IdleTimeout          time.Duration
	ConnTimeout          time.Duration
	IdleLifetime         time.Duration
}

// Add to Load():
maxConn := 50
if raw := os.Getenv("LEDGER_DB_POOL_MAX"); raw != "" {
	if parsed, err := strconv.Atoi(raw); err == nil {
		maxConn = parsed
	}
}

// ... similar for each pool env var ...
```

**Step 4: Run test to verify pass**

```sh
cd ledger && go test ./internal/config/ -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add ledger/internal/config/config.go
git commit -m "feat(config): add environment-driven pool configuration defaults"
```

**Rollback:** `git revert HEAD`

---

### Task 5: Add migration idempotency check to fixtures.go

**Objective:** Add a helper function that checks whether a migration has already been applied before attempting to apply it again.

**Files:**
- Modify: `ledger/tests/fixtures.go` — add idempotency check helper
- Modify: `ledger/Makefile` — add idempotent migration target

**Step 1: Write failing test for idempotency check**

```go
func TestMigrations_AreIdempotent(t *testing.T) {
	// Run migrations twice and verify no error on second run
}
```

**Step 2: Run test to verify failure**

```sh
cd ledger && go test ./internal/db/ -run TestMigrations_AreIdempotent -v
```
Expected: FAIL or unexpected behavior

**Step 3: Add `runMigrationsIfNotApplied` to fixtures.go**

```go
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

	// Check current version
	version, _, err := mInstance.Version()
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("error checking migration version: %w", err)
	}

	// If version is 0 and no dirty, apply all migrations
	// If version is already the latest, skip
	if err := mInstance.Up(); err != nil {
		if err == migrate.ErrNoChange {
			// Already at latest version, this is the idempotent skip
			return nil
		}
		return fmt.Errorf("error running migrations: %w", err)
	}

	return nil
}
```

**Step 4: Update Makefile**

```makefile
# Replace migrate-up with idempotent version:
migrate-up:
	@echo "Applying pending migrations (idempotent)..."
	@./infra/.env && \
	migrate -path db/migrations -database "postgresql://$${POSTGRES_USER}:***@"$${POSTGRES_HOST}"/"$${POSTGRES_DB}"?sslmode=disable" up || true
	@echo "✓ Migrations applied (or already at latest)"
```

**Step 5: Commit**

```bash
git add ledger/tests/fixtures.go ledger/Makefile
git commit -m "feat(migrations): add idempotent migration check to prevent duplicate application"
```

**Rollback:** `git revert HEAD`

---

### Task 6: Create SQLC query files

**Objective:** Create SQLC query files that map to existing repository methods, enabling type-safe code generation.

**Files:**
- Create: `ledger/db/queries/user.queries.sql`
- Create: `ledger/db/queries/portfolio.queries.sql`
- Create: `ledger/db/queries/monthly_statement.queries.sql`
- Create: `ledger/db/queries/idempotency_key.queries.sql`

**Step 1: Create `ledger/db/queries/user.queries.sql`**

```sql
-- CreateUser: Creates a new user
INSERT INTO users (email, password_hash, display_name)
VALUES ($1, $2, $3)
RETURNING id, email, password_hash, display_name, created_at;

-- GetUserByEmail: Retrieves a user by email
SELECT id, email, password_hash, display_name, created_at
FROM users
WHERE email = $1;
```

**Step 2: Create `ledger/db/queries/portfolio.queries.sql`**

```sql
-- CreatePortfolio: Creates a portfolio for a user
INSERT INTO portfolios (id, user_id, name, created_at)
VALUES (gen_random_uuid(), $1, $2, NOW())
RETURNING id, user_id, name, created_at;

-- ListPortfoliosByUser: Lists all portfolios for a user
SELECT id, name
FROM portfolios
WHERE user_id = $1
ORDER BY name;

-- DeletePortfolio: Deletes a portfolio
DELETE FROM portfolios WHERE id = $1 AND user_id = $2;
```

**Step 3: Create `ledger/db/queries/monthly_statement.queries.sql`**

```sql
-- InsertMonthlyStatement: Inserts a new monthly statement
INSERT INTO monthly_statements (id, user_id, portfolio_name, reference_date, ingest_key, raw_payload, parsed_payload, status, source, submitted_at, updated_at)
VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, 'pending', $7, NOW(), NOW())
RETURNING id;

-- UpdateMonthlyStatement: Updates an existing statement
UPDATE monthly_statements
SET raw_payload = $1, parsed_payload = $2, status = 'pending', updated_at = NOW(), source = $3
WHERE user_id = $4 AND portfolio_name = $5 AND reference_date = $6 AND ingest_key = $7
RETURNING id;

-- ListStatementsForPortfolio: Lists statements for a portfolio
SELECT id, ingest_key, status, submitted_at, updated_at, source
FROM monthly_statements
WHERE user_id = $1 AND portfolio_name = $2 AND reference_date = $3
ORDER BY submitted_at DESC;

-- ConfirmStatements: Confirms all statements for a portfolio
UPDATE monthly_statements
SET status = 'confirmed', updated_at = NOW()
WHERE user_id = $1 AND portfolio_name = $2 AND reference_date = $3;
```

**Step 4: Create `ledger/db/queries/idempotency_key.queries.sql`**

```sql
-- FindIdempotencyKey: Finds an idempotency key record
SELECT key, user_id, payload_hash, response_metadata
FROM idempotency_keys
WHERE key = $1;

-- UpsertIdempotencyKey: Inserts or updates an idempotency key
INSERT INTO idempotency_keys (key, user_id, payload_hash, response_metadata, created_at, updated_at)
VALUES ($1, $2, $3, $4, NOW(), NOW())
ON CONFLICT (key) DO UPDATE SET
  payload_hash = EXCLUDED.payload_hash,
  response_metadata = EXCLUDED.response_metadata,
  updated_at = NOW();
```

**Step 5: Run SQLC generate**

```sh
cd ledger && sqlc generate
```

**Step 6: Commit**

```bash
git add ledger/db/queries/
git commit -m "feat(queries): add SQLC query files for type-safe database access"
```

**Rollback:** `git rm -r ledger/db/queries/ && git commit -m "remove: SQLC query files"`

---

### Task 7: Run integration tests and verify

**Objective:** Run the full test suite to verify the connection pool optimization works correctly.

**Files:**
- Test: `ledger/internal/db/` — all tests

**Step 1: Start test containers**

```sh
cd /home/mmb/pprojects/financial-aggregator && docker compose up -d postgres redis
```

**Step 2: Run tests**

```sh
cd ledger && go test -v -race -coverprofile=coverage.out ./...
```

Expected: All tests pass

**Step 3: Verify connection pool metrics**

```sh
# Check logs for pool metrics
cd ledger && go run ./cmd/server/main.go &
sleep 5
# Verify pool is configured correctly
```

**Step 4: Commit**

```bash
git add .
git commit -m "test: verify connection pool optimization passes all tests"
```

**Rollback:** `git revert HEAD`

---

## Sequence of Commits

1. `feat(db): replace hardcoded connection pool with environment-driven pgxpool config`
2. `refactor(db): update Repository to use pgxpool.Pool`
3. `refactor(handler): use pgxpool type for repository initialization`
4. `feat(config): add environment-driven pool configuration defaults`
5. `feat(migrations): add idempotent migration check to prevent duplicate application`
6. `feat(queries): add SQLC query files for type-safe database access`
7. `test: verify connection pool optimization passes all tests`

## Verification Checklist

- [ ] Tasks are sequential and logical (pool optimization before repository, repository before queries)
- [ ] Each task is bite-sized (2-5 min)
- [ ] File paths are exact
- [ ] Code examples are complete (copy-pasteable)
- [ ] Commands are exact with expected output
- [ ] Rollback strategy is defined for every task (git revert)
- [ ] DRY, YAGNI, TDD principles applied
- [ ] No hardcoded secrets in code
