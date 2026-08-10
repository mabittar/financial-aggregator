# Lavish‑Axi Style Playbook — Initial Artifact

**Date**: 07/08/2026
**Context**: Short operational/playbook artifact derived from project macro and spec (first artifact copy).

- **Purpose**: Capture concise infra/implementation decisions and developer-facing snippets for onboarding and early implementation.
- **Owner**: `ledger/` service (DB migrations + RLS binding), `worker/` for analytics code and numeric verification, `web/` for UI/UX.

**Quick Decisions**

- Migrations: `golang-migrate` owned by `ledger/db/migrations` (see migration files).
- Auth: JWT access tokens only; no refresh tokens by spec constraint. Enforce strict `alg` checks and `user_id` claim.
- RLS binding: Use per-transaction GUC via `set_config('app.current_user_id', user_id::text, true)` and `SET LOCAL` inside transaction scope.
- Idempotency: Endpoints accept `idempotency_key` and return `409 Conflict` on duplicates; allow `{ "force_update": true }` to overwrite.

**Dev Snippets**

- RLS session bind (Postgres):

```sql
-- set in the connection/transaction after verifying JWT
SELECT set_config('app.current_user_id', '<<user_id>>', true);
-- transaction-scoped: SET LOCAL used inside server code per request
```

- JWT claims (Go example): `user_id`, `email`, `role`, `exp`, `iat`.

**Migration files**

- Place SQLs in `ledger/db/migrations` following `golang-migrate` naming.

**Idempotency contract (ingest)**

- Request includes `idempotency_key` + payload.
- Server stores `idempotency_key` with hash of payload and response metadata.
- On conflict: return existing resource metadata unless `force_update=true` is present; in that case, apply update and return updated resource.

**Validation / LLM extractor**

- Always validate LLM-extracted JSON against strict Pydantic/Go schema; reject with HTTP 422 and route to a human review queue if invalid.

**Next steps**

- Implement `ledger/` bootstrap: DB migrations, basic `users` table, JWT middleware, RLS policy templates.
- Add example ingest endpoint with idempotency store and unit tests for replay.

---

Generated for repository documentation.
