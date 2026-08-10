## Plan: Ledger Auth & Ingest Implementation

TL;DR: Bootstrap the `ledger/` Go service and implement the approved backend scope: JWT auth, user context binding into Postgres for RLS, portfolio-scoped ingest endpoints with idempotency semantics, and SQL migrations managed by `golang-migrate`.

**Steps**

1. Confirm current infra is aligned.
   - Verify `docker-compose.yml` and `infra/.env.example` exist and define `postgres:18.4` + `redis:8.10.0` with healthchecks.
   - This is already present; no new infra work beyond using it in implementation.

2. Initialize the Go module and service bootstrap.
   - Create `ledger/go.mod` and `ledger/cmd/server/main.go`.
   - Add a basic server startup using `go-chi/chi` and environment-based configuration.
   - Ensure `ledger/` can compile independently.

3. Build config and DB connection support.
   - Add `ledger/internal/config` for env-driven settings: `LEDGER_DB_URL`, `JWT_SIGNING_KEY`, `JWT_ISSUER`, `JWT_EXPIRATION_MINUTES`, etc.
   - Add `ledger/internal/db` with Postgres connection pooling, transaction helper, and `set_config('app.current_user_id', user_id, true)` helper.

4. Create DB migrations.
   - Add `ledger/db/migrations/000001_create_users_table.up.sql` and `.down.sql`.
   - Add `ledger/db/migrations/000002_create_monthly_statements.up.sql` and `.down.sql`.
   - Add `ledger/db/migrations/000003_create_idempotency_keys.up.sql` and `.down.sql`.
   - Add optional migration for RLS policies and default roles.
   - Use `uuidv7()` and required constraints from the spec.

5. Implement auth domain logic.
   - Create `ledger/internal/auth` with `CustomClaims`, token generation, token validation, and secure password hashing (bcrypt or argon2).
   - Add `AuthenticateRequest` middleware that verifies JWT, validates claims, and injects auth info into request context.
   - Add a DB-backed `users` repository for registration and credential validation.

6. Implement portfolio ingest domain logic.
   - Add `ledger/internal/handler` for auth endpoints and portfolio ingest endpoints.
   - Implement `POST /api/v1/portfolios/{portfolio_name}/{YYYYMMDD}/assets` and `POST /api/v1/portfolios/{portfolio_name}/{YYYYMMDD}/mov` to validate canonical JSON, enforce portfolio scope, compute payload hash, check idempotency, and insert/update `monthly_statements` and `idempotency_keys` inside a transaction.
   - Implement `GET /api/v1/portfolios/{portfolio_name}/{YYYYMMDD}/reconciliation` for reconciliation state.
   - Implement `POST /api/v1/portfolios/{portfolio_name}/{YYYYMMDD}/confirm` to finalize portfolio ingest for the date.
   - Add portfolio management endpoints: `GET /api/v1/portfolios`, `POST /api/v1/portfolios`, `GET /api/v1/holdings`, `GET /api/v1/holdings/{id}/transactions`.
   - Implement duplicate handling: `409 Conflict` on duplicate idempotency key unless `force_update=true`, `200 OK` on force update.

7. Wire routing and middleware.
   - Define routes in `ledger/cmd/server/main.go` or `ledger/internal/router`.
   - Protect ingest and user routes with JWT middleware.
   - Add health endpoint for the ledger service.

8. Add schema validation and payload models.
   - Define minimal Go structs for the canonical ingest payload.
   - Use JSON decoding and validation checks for required fields and types.
   - Add clear error responses for validation failures.

9. Add tests.
   - Unit test auth middleware and token validation.
   - Unit test user registration/login and password hashing.
   - Unit test ingest idempotency conflict and force update behavior.
   - Add integration tests that use a temporary Postgres test database or Docker container.

10. Update documentation / README.

- Document ledger startup, required env variables, and migration commands.
- Add a `ledger/README.md` or update root README with service-specific usage.

**Relevant files**

- `docs/specs/20260807-ledger-auth-ingest_spec.md` — approved spec source.
- `ledger/cmd/server/main.go` — service bootstrap.
- `ledger/internal/config` — env config.
- `ledger/internal/db` — DB connection + RLS binding.
- `ledger/internal/auth` — JWT and password domain.
- `ledger/internal/handler` — API handlers.
- `ledger/db/migrations/` — SQL migrations.
- `docker-compose.yml` — infra for Postgres/Redis.
- `infra/.env.example` — example environment variables.

**Verification**

1. Confirm `docker compose up -d` starts Postgres and Redis with healthy status.
2. Ensure `go test ./ledger/...` passes once code is implemented.
3. Validate auth and ingest behavior against the spec acceptance criteria.
4. Confirm migrations can be applied with `golang-migrate`.
5. Verify docs are updated with environment and startup instructions.

**Decisions / Assumptions**

- Use `go-chi/chi` and `golang-jwt/jwt/v5` for the ledger service.
- Keep auth state in JWT only; no refresh token flow.
- Use session-scoped Postgres `SET LOCAL` binding for `app.current_user_id` and RLS.
- Treat Redis as optional for future caching/orchestration, but not required for MVP implementation.
- Exclude worker and frontend implementation from this plan.
