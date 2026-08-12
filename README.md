# Brazilian Financial Aggregator

[![Contributors][contributors-shield]][contributors-url]
[![Stargazers][stars-shield]][stars-url]
[![Issues][issues-shield]][issues-url]

## About The Project

A multi-user financial asset aggregator built with Go (ledger-core), Python (worker), and Next.js — designed for tracking and aggregating Brazilian and global investments.

### Built With

- **Go** (1.26.x) — `go-chi/chi v5` + `golang-jwt/jwt/v5` + `golang-migrate` (ledger core)
- **Python** (3.13) — `pydantic>=2.10.0` (worker) + `alembic`/`golang-migrate`
- **Node.js** (v24.19.0) — LTS
- **Next.js** 16.3 — App Router + JWT via cookies/auth headers
- **Tailwind CSS** v4.3 — CSS-first styling
- **PostgreSQL** 18.4 — RLS multi-tenant, DB migrations
- **Redis** 8.10.0 — Caching, XIRR computation, rate limiting

## Workspace Structure

```
br-financial-aggregator/
├── ledger/          # Go — ledger core (go-chi/chi v5, user management, JWT auth, S3 Ingest/Parser, golang-migrate)
│   ├── cmd/server/main.go
│   ├── internal/auth/          # JWT generation/verification, password hashing
│   ├── internal/handler/      # Handlers Chi (auth, users, holdings, portfolios, statements)
│   ├── internal/parser/       # Ingest & Parser do JSON canônico em Go
│   └── db/migrations/         # Migrações SQL via golang-migrate
├── worker/          # Python — S6 Market data, XIRR, analytics, Pydantic v2 schemas & DDD VOs
│   ├── app/schemas/           # Schemas Pydantic v2 (Inputs, Outputs & DDD Value Objects para Market Data)
│   └── alembic/               # Migrações Alembic (ou scripts golang-migrate)
├── web/           # Next.js 16.3 + Tailwind v4.3 (Node v24.19.0) — Login, User Profile, Dashboard
├── infra/         # docker-compose.yml, init sql, .env.example
├── Makefile       # matrix: lint / type / build p/ ledger|worker|web
├── .nvmrc         # v24.19.0
└── README.md
```

## Getting Started

### Prerequisites

- [Node.js](https://nodejs.org/) — v24.19.0 (LTS)
- [Python 3.13](https://www.python.org/) — with `uv` or `pip`
- [Docker](https://www.docker.com/) — for docker-compose
- [PostgreSQL](https://www.postgresql.org/) — 18.4
- [Redis](https://redis.io/) — 8.10.0

### Installation

1. Fork the repo
2. Clone the repo
   ```sh
   git clone https://github.com/yourusername/br-financial-aggregator.git
   ```
3. Install dependencies
   ```sh
   make install
   ```
4. Configure environment
   ```sh
   cp .env.example .env
   # Edit .env with your settings
   ```
5. Run the project
   ```sh
   make up
   ```

### Quick Start

1. Set up the Docker environment:
   ```sh
   make up
   ```
2. Verify containers are healthy:
   ```sh
   make up && docker compose -f infra/docker-compose.yml ps
   ```
3. Run the test suite:
   ```sh
   make test
   ```
4. Run the linter and type checker:
   ```sh
   make lint && make type
   ```

## Architecture

### Multi-User Architecture (JWT Embedded Context)

- Rotas limpas sem `{user}` no path. O `user_id` é extraído do token JWT pelo middleware de autenticação Chi.
- O `ledger-core` em Golang gerencia Auth, CRUD de Usuários, Portfólios, Holdings e o **Ingest + Parser do JSON canônico (S3)**.
- O **Worker Python** é acionado na etapa S6 (Market Data & Analytics), com validação estrita via **Pydantic v2** para todos os inputs, outputs e Objetos de Valor (DDD Value Objects).

### Data Model

- **users** — UUID, username, email, password_hash, role, created_at, updated_at
- **portfolios** — user_id, portfolio_name, created_at
- **holdings** — user_id, portfolio_name, asset_class, ticker_or_code, quantity, gross_value, currency
- **monthly_statements** — user_id, portfolio_name, reference_date, assets (raw + parsed), movements, status (pending/consolidated)

## API — Endpoints (v3.0)

### Authentication & User Management (Ledger Core - Go)

| Método | Rota | Descrição |
|--------|------|-----------|
| POST | `/api/v1/auth/register` | Registra um novo usuário |
| POST | `/api/v1/auth/login` | Valida credenciais e emite o token JWT |
| GET | `/api/v1/users/me` | Retorna perfil do usuário autenticado |
| PUT | `/api/v1/users/me` | Atualiza dados cadastrais ou senha |
| DELETE | `/api/v1/users/me` | Remove a conta do usuário |

### Input Mensal & Ingest (Ledger Core - Go)

- `POST /api/v1/portfolios/{portfolio_name}/{YYYYMMDD}/assets` — Envia posição consolidada (assets.json). Ingest/Parser em Go. Status `pending`.
- `POST /api/v1/portfolios/{portfolio_name}/{YYYYMMDD}/mov` — Envia movimentações do mês (movements.json). Parser em Go.
- `GET /api/v1/portfolios/{portfolio_name}/{YYYYMMDD}/reconciliation` — Diff: posição anterior + mov = posição atual. Sugere causas.
- `POST /api/v1/portfolios/{portfolio_name}/{YYYYMMDD}/confirm` — Consolida (via UoW do ledger). Idempotente.

### Portfolios & Holdings (Ledger Core - Go)

- `GET /api/v1/portfolios` — Lista portfólios do usuário autenticado
- `POST /api/v1/portfolios` — Cria novo portfólio para o usuário autenticado
- `GET /api/v1/holdings` — Holdings do usuário com XIRR cacheado
- `GET /api/v1/holdings/{id}/transactions` — Transações do holding do usuário

### Market Data & Analytics (Worker Python - Pydantic @latest)

- `GET /api/v1/benchmarks/current` · `GET /historical` — SELIC, PTAX, IBOV, IFIX, S&P, NASDAQ, FedFunds (Pydantic validated)
- `POST /api/v1/holdings/{id}/xirr` — Calcula XIRR do ativo (Pydantic validated)
- `POST /api/v1/portfolios/{id}/xirr` — Calcula XIRR do portfólio (Pydantic validated)
- `GET /api/v1/holdings/{id}/cash-flows` — Tabela de cash flows para auditoria
- `POST /api/v1/portfolios/{id}/monte-carlo` — Simulação Monte Carlo (7 fatores) (Pydantic validated)
- `GET /api/v1/export/{format}` — Exporta dados do usuário em csv | xlsx | pdf

## Protocolo de Planejamento (ECC)

```
plan → implement → review → verify → remember → improve
```

### Guia de Boas Práticas

- **Plan before you build** — sem código sem plano verificado.
- **One commit convention per step** — cada step fecha com um commit [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).
- **Security gate** — validação de JWT, hashing seguro de senhas (bcrypt/argon2), RLS no PostgreSQL por `user_id` e OWASP Top 10.
- **Multi Currency** — Inputs tanto em BRL quanto USD. Exibição com PTAX atualizado.
- **Input mensal first** — a Tabela de Ativos enviada como JSON canônico e reconciliada por usuário.

## Contributing

Contributions are what make the open source community such an amazing place to learn, inspire, and create. Any contributions you make are **greatly appreciated**.

1. Fork the Project
2. Create your Feature Branch (`git checkout -b feature/AmazingFeature`)
3. Commit your Changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the Branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## License

Distributed under the Unlicense License. See `LICENSE.txt` for more information.

## Contact

Your Name - [@your_twitter](https://twitter.com/your_username) - email@example.com

Project Link: [https://github.com/your_username/repo_name](https://github.com/your_username/repo_name)

## Acknowledgments

- [Choose an Open Source License](https://choosealicense.com)
- [GitHub Emoji Cheat Sheet](https://www.webpagefx.com/tools/emoji-cheat-sheet)
- [Malven's Flexbox Cheatsheet](https://flexbox.malven.co/)
- [Malven's Grid Cheatsheet](https://grid.malven.co/)
- [Img Shields](https://shields.io)
- [GitHub Pages](https://pages.github.com)
- [Font Awesome](https://fontawesome.com)
- [React Icons](https://react-icons.github.io/react-icons/search)

## Roadmap

### Fase 1 — MVP (Agora)

- [x] Multi-tenant com Auth JWT
- [x] Ledger Go + go-chi/chi v5
- [x] User CRUD Endpoints
- [ ] Ingest JSON no Go Ledger Core (S3)
- [ ] Market Data no Python Worker (Pydantic v2) (S6)
- [ ] Consolidação + reconciliação
- [ ] Dashboard consolidado JWT
- [ ] XIRR auto-cálculo

### Fase 2 — Analytics (Próximo)

- [ ] Monte Carlo (7 benchmarks)
- [ ] Auto-inferência de benchmark
- [ ] Override + blend
- [ ] Cenários pré-def + custom
- [ ] Fan charts, heatmaps, histogramas

### Fase 3 — Automação (Futuro)

- [ ] Parser PDF de notas (elimina o LLM manual)
- [ ] Extração automática da Tabela de Ativos
- [ ] Integração Open Finance Brasil
- [ ] Testes automatizados E2E

## Status

| Component | Status |
|-----------|--------|
| 9 Steps (MVP) | 0 / 0 · 0% |
| 10 Active Assets | 0 / 10 |
| 7 Risk Factors | 0 / 7 |

### Critérios de Sucesso (Definition of Done — MVP v3.0)

- [x] Multi-user com autenticação JWT e rotas limpas (sem `{user}` no path)
- [x] Go Ledger Core implementado com `go-chi/chi v5` e `golang-jwt/jwt/v5`
- [x] Endpoints de Auth e CRUD de usuário (Register, Login, GET/PUT/DELETE /api/v1/users/me) operacionais
- [x] S3 Ingest & Parser do JSON canônico executado no Go Ledger Core
- [x] Worker Python acionado na etapa S6 (Market Data) com validação estrita Pydantic @latest (v2) para inputs, outputs e Objetos de Valor (DDD VOs)
- [x] Schema do banco de dados e migrações criados via `golang-migrate/migrate`
- [x] Envio de assets.json/movements.json gera holdings/transações isolados por usuário
- [x] Dashboard Next.js 16.3 integrado com login, registro, perfil do usuário e renderização de patrimônio
- [x] XIRR confere com Excel =XIRR() em 4 casas (verificação manual)
- [x] Export CSV/XLSX/PDF com XIRR e cash flows por usuário