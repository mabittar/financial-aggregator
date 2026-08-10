# Plan: Complete and Verify Ledger S0/S1 Bootstrap

**TL;DR**: The ledger service is ~95% complete. Gap analysis revealed three critical items:

1. **JWT_SIGNING_KEY** already added to `.env` ✅
2. **Config validation** needed — missing env vars must panic with clear error before startup
3. **Tests missing** (spec requires `go test ./...` passes)

The plan adds config validation, uses golang-migrate CLI for migrations (external tool, not embedded), adds integration tests with testcontainers, and verifies all Definition of Done criteria are met.

---

## Steps

### Phase 1: Environment & Configuration _(Independent / can start immediately)_

1. Update `infra/.env.example` to document all required variables:
   - Add `JWT_SIGNING_KEY=<example-key-128-chars>` (required, already in .env)
   - Add `JWT_ISSUER=financial-aggregator`
   - Add `JWT_EXPIRATION_MINUTES=60`
   - Add `LEDGER_PORT=8080`
   - Add `LEDGER_DB_URL` (optional; falls back to construct from POSTGRES\_\* vars)
   - Mark required vs. optional

2. Add validation to [ledger/internal/config/config.go](ledger/internal/config/config.go):
   - Create `Validate()` method that checks all required env vars are present
   - Required vars: `JWT_SIGNING_KEY`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`
   - If any required var is missing, panic with clear error message listing missing vars
   - Call `cfg.Validate()` at start of `main.go` before DB connection
   - Prevents app startup with incomplete configuration

---

### Phase 2: Configure golang-migrate CLI _(Depends on Phase 1; can run in parallel with Phase 3)_

3. Install golang-migrate CLI tool (if not already installed):
   - Option A: `brew install golang-migrate` (macOS)
   - Option B: `go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest` (any platform)
   - Verify installation: `migrate --version`

4. Verify migration files are correct in `ledger/db/migrations/`:
   - `000001_create_users_table.up.sql` — schema matches auth.User struct
   - `000002_create_monthly_statements.up.sql` — has composite unique constraint on (user_id, portfolio_name, reference_date, ingest_key)
   - `000003_create_idempotency_keys.up.sql` — has correct PKs/FKs
   - All `.down.sql` files present for rollback

5. Document migration command in `ledger/Makefile`:
   - Add `make migrate-up` target: `migrate -path db/migrations -database "$DATABASE_URL" up`
   - Add `make migrate-down` target: `migrate -path db/migrations -database "$DATABASE_URL" down`
   - Add `make migrate-status` target: `migrate -path db/migrations -database "$DATABASE_URL" version`
   - Document in README: migrations must be run manually before starting service or via CI/CD pipeline

6. Create `scripts/migrate.sh` (optional convenience script):
   - Loads DATABASE_URL from `infra/.env`
   - Runs `migrate up` with proper error handling
   - Can be called before service startup

---

### Phase 3: Test Suite _(Parallel with Phase 2)_

7. Create test infrastructure:
   - Add `github.com/testcontainers/testcontainers-go` to `go.mod`
   - Create `ledger/tests/fixtures.go` (new dir):
     - `NewTestDB(t *testing.T) (*sql.DB, func())` — starts Postgres container, runs migrations, returns cleanup function
     - Reusable across all test files
   - Create `ledger/tests/config.go`:
     - Test config loader with mock env vars

8. Write unit tests for auth service (`ledger/internal/auth/service_test.go`):
   - `TestRegisterUser_Success` — happy path
   - `TestRegisterUser_DuplicateEmail` — conflict handling
   - `TestAuthenticateUser_ValidCredentials` — password verification
   - `TestAuthenticateUser_InvalidCredentials` — wrong password
   - `TestGenerateToken_ClaimsIncluded` — JWT contains user_id, email, role
   - `TestValidateToken_ValidSignature` — token accepted
   - `TestValidateToken_ExpiredToken` — token rejected
   - `TestValidateToken_InvalidSignature` — malformed token rejected

9. Write integration tests for database layer (`ledger/internal/db/repository_test.go`):
   - Use testcontainers Postgres for each test
   - `TestUserRepository_Create_And_FindByEmail` — round-trip user storage
   - `TestIdempotencyKey_Upsert_Idempotent` — verify ON CONFLICT behavior
   - `TestMonthlyStatement_Insert_And_List` — statement storage and retrieval
   - `TestMonthlyStatement_UniqueConstraint` — composite unique (user_id, portfolio_name, reference_date, ingest_key)

10. Write integration tests for HTTP handlers (`ledger/internal/handler/handler_test.go`):
    - `TestHealthEndpoint_ReturnsOK` — basic connectivity
    - `TestRegisterEndpoint_Success` — POST /api/v1/auth/register with valid JSON
    - `TestRegisterEndpoint_InvalidJSON` — malformed request
    - `TestLoginEndpoint_Success` — POST /api/v1/auth/login returns JWT
    - `TestLoginEndpoint_InvalidCredentials` — wrong password returns 401
    - `TestProtectedRoute_WithoutToken_Returns401` — missing Bearer token
    - `TestProtectedRoute_WithValidToken_AllowsAccess` — token validation middleware works

11. Create `ledger/Makefile` for test automation:
    - `make test` — run all tests
    - `make test-unit` — unit tests only (fast)
    - `make test-integration` — integration tests with containers (slower)
    - `make build` — compile ledger binary
    - `make lint` — run go vet and golangci-lint if available

---

### Phase 4: Compilation & Verification _(Depends on Phase 2)_

12. Test compilation:
    - Run `cd ledger && go build ./cmd/server` in workspace
    - Verify no errors or warnings
    - Binary should be created at `ledger/cmd/server/server` (or platform-specific)

13. Run full test suite:
    - Execute `cd ledger && go test ./...` (from Definition of Done)
    - All tests must pass
    - Code coverage should be >60% for auth and db packages (or document why lower)

14. Manual smoke test (local):
    - Start containers: `docker compose up -d`
    - Wait for Postgres healthcheck to pass
    - Run ledger service: `cd ledger && go run ./cmd/server`
    - Verify startup logs show:
      - Config loaded successfully
      - DB connection established
      - Migrations completed
      - HTTP server listening on :8080
    - Test health endpoint: `curl http://localhost:8080/health`
    - Register a user: `curl -X POST http://localhost:8080/api/v1/auth/register -d '{"email":"test@example.com","password":"password123"}'`
    - Login and receive JWT
    - Use JWT to call protected endpoint

---

## Relevant Files

- `ledger/internal/config/config.go` — add Validate() method to check required env vars
- `ledger/internal/db/db.go` — already complete; no changes needed
- `ledger/internal/db/repository.go` — already complete; all methods present
- `ledger/internal/auth/service.go` — already complete; no changes needed
- `ledger/cmd/server/main.go` — call cfg.Validate() at startup
- `ledger/internal/auth/service_test.go` (NEW) — unit tests for JWT/auth
- `ledger/internal/db/repository_test.go` (NEW) — integration tests for DB layer
- `ledger/internal/handler/handler_test.go` (NEW) — HTTP handler tests
- `ledger/tests/fixtures.go` (NEW) — testcontainers setup
- `ledger/tests/config.go` (NEW) — test config helpers
- `ledger/Makefile` (NEW) — build, test, and migration automation
- `scripts/migrate.sh` (NEW, optional) — convenience script for running migrations
- `infra/.env` — already has JWT_SIGNING_KEY
- `infra/.env.example` — document all required/optional variables
- `ledger/db/migrations/*.sql` — verify schema correctness

---

## Verification

1. **Environment**: `infra/.env` contains JWT_SIGNING_KEY and config.go validates all required vars
2. **Compilation**: `go build ./cmd/server` succeeds with no errors
3. **Migrations**: golang-migrate CLI installed; `make migrate-up` successfully applies pending migrations
4. **Tests**: `go test ./...` passes all unit and integration tests
5. **Code Coverage**: Auth and DB packages report >60% coverage
6. **Smoke Test**:
   - Health endpoint returns 200 OK
   - User registration succeeds and stores hashed password
   - Login returns valid JWT with correct claims
   - Protected routes reject requests without Bearer token
   - Protected routes accept valid JWTs

---

## Decisions

- **Migration Strategy**: Use golang-migrate CLI tool (external, not embedded in code). Developers install `migrate` locally or in CI/CD; migrations run manually via `make migrate-up` or convenience script before service startup.
- **Config Validation**: Add Validate() method to config.go that panics on missing required env vars (JWT_SIGNING_KEY, POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB). Prevents silent misconfiguration.
- **Test Containers**: Use testcontainers-go for integration tests (Postgres instance per test, auto-cleanup).
- **Test Scope**: Integration tests verify actual DB operations; unit tests verify auth logic in isolation.
- **Definition of Done**: Verified against spec checklist — all items including "documentation updated" in README for migration setup.

---

## Further Considerations

1. **Migration Execution**: Developers must run `make migrate-up` before starting the ledger service for the first time. This can be automated in CI/CD, Docker entrypoints, or Makefile targets.

2. **RLS Session Binding**: The plan verifies basic auth; RLS policies (SET LOCAL app.current_user_id) are already in `db.go` but not tested in S0/S1. Consider a flag to enable RLS validation in future stages.

3. **Refresh Tokens**: The spec mentions refresh token flow but S0/S1 only requires access tokens (JWT). Refresh tokens can be added in S2 (User CRUD + Session Management).
