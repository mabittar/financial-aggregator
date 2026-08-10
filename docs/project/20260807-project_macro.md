# [Project Macro Definitions]

**Date**: 07/08/2026  
**Last Update**: 07/08/2026  
**Version**: 1.0  
**Priority**: 🔴 HIGH

**Changelog v1.0**:

- Initial version — scaffolded and initial analysis written from provided spec

## Business Objective

Plataforma multi-usuário para agregação, reconciliação e análise de investimentos (ativos brasileiros e internacionais), com autenticação JWT, ingestão de extratos em JSON canônico, cálculo de XIRR e visualização via dashboard.

## Technical Stack (as specified)

- Backend (ledger core): Go 1.26.x, go-chi/chi v5, golang-jwt/jwt v5, golang-migrate
- Worker (analytics/market data): Python 3.13, Pydantic v2 (>=2.10.0), Alembic (or golang-migrate compatibility)
- Frontend: Next.js 16.3, Node v24.19.0, Tailwind CSS v4.3
- Data stores: PostgreSQL 18.4 (with RLS), Redis 8.10.0
- Infra: docker-compose for local/dev; Makefile targets for lint/type/build

## Project Type

Monorepo with three primary services/components: `ledger/` (Go API & ingest), `worker/` (Python analytics), and `web/` (Next.js frontend). Deployable as separate services/containers.

## Dependencies

- `golang` toolchain (1.26.x), `go-chi/chi v5`, `golang-jwt/jwt v5`, `golang-migrate`
- `python` 3.13, `pydantic >= 2.10.0`, `alembic`, `pyxirr` or numeric fallback libraries
- `node` v24.19.0, `next` 16.3, `tailwindcss` v4.3
- `postgres:18.4`, `redis:8.10.0`

## Architecture Pattern

Monorepo / modular microservices pattern: small, well-scoped services per responsibility (API/ingest in Go; heavy analytics in Python; UI in Next.js). Communication via REST HTTP endpoints and Redis for caching/coordination. DB is shared PostgreSQL with tenant isolation via `user_id` and RLS policies.

## 1. Analysis of Alternatives

| Approach                                 | Pros                                                                               | Cons                                                                                             |
| :--------------------------------------- | :--------------------------------------------------------------------------------- | :----------------------------------------------------------------------------------------------- |
| Single service monolith (all in Go)      | Simpler ops, single deployment artifact                                            | Harder to use Python ecosystem (Pydantic, pyxirr), heavier binary, slower iteration on analytics |
| Monorepo with separate services (chosen) | Clear separation of concerns; use best language per task; independent deploy/scale | More infra complexity; cross-language migrations/versioning                                      |
| Serverless functions for worker          | Scales on demand, lower infra cost for bursty jobs                                 | Cold-starts for Python, complexity in stateful tasks (XIRR, Monte Carlo) and orchestration       |

**Chosen**: Monorepo with separate services | **Justification**: Best trade-off between developer productivity (use Python for analytics) and production stability (Go for API/ingest).

## 2. Solution Design (Mermaid Diagram)

```mermaid
flowchart TD
   Client[Web / CLI / Scheduler] -->|Auth| Auth[JWT Auth (Go Ledger)]
   Client -->|Upload assets.json / movements.json| Ledger[Ledger Core (Go)]
   Ledger --> Staging[(monthly_statements staging)]
   Ledger -->|Confirm| Consolidation[Consolidation & Reconciliation (Go)]
   Consolidation -->|Trigger| Worker[Worker (Python) — XIRR / Market Data]
   Worker -->|Cache/Store| Redis((Redis))
   Worker -->|Write analytics| AnalyticsDB[(Postgres)]
   Ledger -->|Read/Write| Postgres[(Postgres)]
   Web[Next.js Dashboard] -->|API calls| Ledger
   Web -->|Auth| Auth
```

## 3. Data Architecture

- Core tables (recommended): `users` (id UUID), `portfolios` (user_id, portfolio_name), `holdings` (user_id, portfolio_id, ticker_or_code, asset_class, position_model, quantity, gross_value), `transactions` (holding_id, type, date, amount, fees), `monthly_statements` (raw_payload, parsed_payload, reference_date, status)
- Migrations: use `golang-migrate` for `ledger/` SQL migrations; `worker/` may use Alembic but align with same Postgres schema or use dedicated schemas/namespaces
- RLS: Row-level security policies tied to `user_id` extraction from JWT/session for tenant isolation

## Weak Points / Clarifications Needed

- [R1] LLM extractor reliability: spec assumes an external LLM produces valid JSON — need validation rules and fallback flow for inconsistent/malformed JSON. (Mitigation: strict schema validation, reject with clear error, human review queue).
- [R2] Token strategy: refresh token flow is mentioned but details missing (refresh token rotation, storage, revocation). Clarify refresh lifecycle and cookie vs storage strategy.
- [R3] Currency conversion and FX timing: how PTAX updates are sourced and applied; conversion rules for mixed-currency portfolios not fully specified.
- [R4] Migrations cross-language: choose single source of truth for migrations (prefer `golang-migrate` for DB schema; worker should avoid conflicting Alembic-managed DDLs or coordinate schema ownership).
- [R5] RLS session binding: how `user_id` is bound into DB session (SET LOCAL, application_role, or claim-based connection?). Need precise implementation pattern for secure RLS.
- [R6] Error handling / idempotency of ingest endpoints: confirm strategies for dedup, replay, idempotency keys.
- [R7] Scaling & hosting: spec pins container images but does not specify target environment (Kubernetes, ECS, or managed VMs). Clarify CI/CD and deployment target.
- [R8] Testing & verification: E2E tests and benchmarking approach for XIRR accuracy vs Excel — specify test fixtures and tolerances.
- [SUPOSIÇÃO] Assumed: Users are identified solely via JWT `user_id` claim and no additional multi-tenant mapping is required.

## Immediate Recommendations (short)

- Add a clear contract and retry/fallback for the LLM extractor; add a human-in-the-loop review queue for rejected payloads.
- Define refresh-token lifecycle and storage (httpOnly cookie + rotation) in the Auth spec.
- Choose a single DB migration owner (recommend `ledger/` with `golang-migrate`) and document how `worker/` will evolve schema (SQL files submitted to same pipeline).
- Specify deployment target (k8s vs docker-compose for production) to finalize infra decisions.

## Next steps

1. Resolve clarifications above (LLM contract edge cases, refresh tokens, FX rules, migration ownership, RLS binding).
2. Optionally run web research to verify recommended libraries/versions and alternatives before finalizing the macro document.
3. Update this document to v1.1 with decisions and expanded diagrams.

---

Generated from the provided spec `brazilian-financial-aggregator-v3.0.html` on 07/08/2026.
