# Wallet Service

A Go + PostgreSQL HTTP API service that manages customer wallet balances for a logistics platform.

---

## Quick Start

### 1. Start PostgreSQL

**Option A — Docker (recommended)**
```bash
docker run --name postgres-wallet \
  -e POSTGRES_USER=wallet_user \
  -e POSTGRES_PASSWORD=wallet_pass \
  -e POSTGRES_DB=wallet_db \
  -p 5432:5432 \
  -d postgres:15
```

**Option B — Postgres.app (macOS)**

Download and open [Postgres.app](https://postgresapp.com/), then add `psql` to your PATH:
```bash
echo 'export PATH="/Applications/Postgres.app/Contents/Versions/latest/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```
Create the DB and user:
```bash
psql -c "CREATE USER wallet_user WITH PASSWORD 'wallet_pass';"
psql -c "CREATE DATABASE wallet_db OWNER wallet_user;"
```

### 2. Set up the DB schema (first time only)

```bash
psql postgres://wallet_user:wallet_pass@localhost:5432/wallet_db -f scripts/setup_db.sql
```

### 3. Run the service

```bash
go run ./cmd/server
```

### 4. Run the Order Service stub

```bash
go run ./scripts/order_stub.go
```

---

## API Endpoints

All routes (except `/health`) are under `/api/v1` and require authentication.

| Method | Path | Role | Purpose |
|--------|------|------|---------|
| `GET` | `/health` | — | Health check |
| `POST` | `/api/v1/wallets` | CUSTOMER | Create wallet |
| `GET` | `/api/v1/wallets/:id` | CUSTOMER / ORDER_SERVICE | Get wallet details |
| `POST` | `/api/v1/wallets/:id/topup` | CUSTOMER | Add funds |
| `POST` | `/api/v1/wallets/:id/deduct` | ORDER_SERVICE | Deduct (idempotent) |
| `GET` | `/api/v1/wallets/:id/balance` | CUSTOMER / ORDER_SERVICE | Get balance |
| `GET` | `/api/v1/wallets/:id/transactions` | CUSTOMER / ORDER_SERVICE | Transaction history |

### Authentication

All `/api/v1` endpoints require `Authorization: Bearer <token>`.

- Customer token: `customer:<customerId>` (e.g. `customer:cust-101`)
- Order Service token: configured in `resources/config.yaml` under `auth.order_service_token`

---

## Example Requests

```bash
# Create wallet
curl -X POST http://localhost:8080/api/v1/wallets \
  -H "Authorization: Bearer customer:cust-101" \
  -H "Content-Type: application/json" \
  -d '{"initialBalance": 500}'

# Top up
curl -X POST http://localhost:8080/api/v1/wallets/<id>/topup \
  -H "Authorization: Bearer customer:cust-101" \
  -H "Content-Type: application/json" \
  -d '{"amount": 300, "referenceId": "topup-001"}'

# Deduct (Order Service)
curl -X POST http://localhost:8080/api/v1/wallets/<id>/deduct \
  -H "Authorization: Bearer order-service-secret" \
  -H "Content-Type: application/json" \
  -d '{"idempotencyKey": "order-9001", "amount": 100, "referenceId": "order-9001"}'

# Get balance
curl http://localhost:8080/api/v1/wallets/<id>/balance \
  -H "Authorization: Bearer customer:cust-101"

# Get transactions
curl http://localhost:8080/api/v1/wallets/<id>/transactions \
  -H "Authorization: Bearer customer:cust-101"
```

---

## Running Tests

```bash
go test ./...
```

### Test Coverage

| Layer | What's Tested | Approach |
|-------|--------------|----------|
| Business logic | `WalletService` | Unit tests with mock repositories |
| Validation | Input constraints | Required fields, positive amounts, minimum balance |
| Edge cases | Error paths | Sentinel error assertions for not found, duplicate, invalid input |

**Unit tests (10 tests):**
- ✅ Create wallet with valid balance (≥ configured minimum reserve)
- ✅ Create wallet with below-minimum balance → `ErrInvalidRequest`
- ✅ Create wallet with negative balance → `ErrInvalidRequest`
- ✅ Duplicate wallet for same customer → `ErrDuplicateWallet`
- ✅ Get wallet not found → `ErrWalletNotFound`
- ✅ Get transactions success
- ✅ Deduct without idempotency key → `ErrInvalidRequest`
- ✅ Deduct with zero/negative amount → `ErrInvalidRequest`
- ✅ Top-up success
- ✅ Top-up with zero/negative amount → `ErrInvalidRequest`

**Integration demonstration (`scripts/order_stub.go`):**
- ✅ End-to-end idempotent deduction — retry returns cached result, **prevents double-debit** (`cached=true`)
- ✅ Minimum balance reserve enforcement — deductions fail cleanly when balance would drop below ₹100
- ✅ Transaction history ordering — newest first

**Why idempotency isn't unit-tested:**
Testing idempotent replay requires mocking `pgxpool.Pool.Begin()` to control transaction behavior, adding significant complexity (mocking `pgx.Tx`, `pgx.Rows`, connection pools) with marginal value. The order stub proves idempotency works correctly end-to-end against real PostgreSQL, which is a stronger guarantee than mocked unit tests. The business logic (check idempotency record → return cached result → never call `DebitBalance` again) is clearly readable in `wallet_service.go`.

---

## Business Rules

- **Single currency**: All amounts in INR (₹)
- **One wallet per customer**: Enforced by `UNIQUE INDEX` on `customer_id`
- **Minimum balance reserve**: Configurable (default ₹100) — balance can never drop below this threshold
- **Idempotency**: Only the `/deduct` endpoint requires an idempotency key
- **Transaction history**: Returned in reverse chronological order (newest first)

---

## Key Design Decisions

### Configurable minimum balance reserve
The reserve is driven by `resources/config.yaml`:
```yaml
business:
  minimum_balance_reserve: 100.0
```
This means the threshold can be changed without a DB migration. The DB only has `CHECK (balance >= 0)` as an absolute safety net.

### Atomic conditional balance update
The debit path uses a single SQL statement that enforces the minimum reserve atomically:
```sql
UPDATE wallets SET balance = balance - $1, version = version + 1
WHERE wallet_id = $2 AND balance >= $1::numeric + $3::numeric
RETURNING balance
```
`$3` is the configured reserve passed at runtime. Race-condition-free — no application-level locking required.

### Idempotent deductions
`deduction_idempotency` has `PRIMARY KEY (wallet_id, idempotency_key)`. On every deduct:
1. Check if record exists → replay stored outcome.
2. Amount differs → `IDEMPOTENCY_CONFLICT`.
3. New → debit + ledger + idempotency record in one DB transaction.
4. **Even `INSUFFICIENT_BALANCE` failures are stored** — retries replay the failure consistently.

### Single DB transaction per mutation
Balance update, ledger append, and idempotency record are committed together. No partial states possible.

### Layered architecture
`handler → business → repository`. Repositories are interfaces — swappable for mocks in tests or alternative backends in future.

### Static token auth
Customer tokens: `customer:<customerId>`. Order Service uses a config-file token. Simple and demonstrable; upgrade path is JWT or mTLS.

---

## Trade-offs

| Decision | Trade-off |
|----------|-----------|
| `balance` cached on wallet row | Fast reads; consistent because mutations write wallet + ledger in one transaction |
| Static bearer tokens | Easy to run locally; not production-grade security |
| No pagination on transactions | Acceptable for this scope; add cursor-based pagination for production |
| No-op metrics/events | Runs without external dependencies; replace interfaces for production observability |
| Manual DB setup via `psql` script | Avoids migration tooling complexity; one-time command. Production: automate via CI/CD |
| `float64` for money | Simple for this scope; upgrade to `shopspring/decimal` for paise-level precision in production |

---

## Testing Philosophy

This implementation prioritises **correctness verification over coverage percentage**:

1. **Critical business logic** (CreateWallet, validation, input constraints) has unit tests with mocks.
2. **Complex transactional flows** (Deduct with idempotency, TopUp) are demonstrated via the order stub against real Postgres.
3. **Why?** — Mocking `pgx.Tx` and transaction boundaries adds complexity that obscures intent. The order stub proves the system works end-to-end.

For production: add integration tests using `testcontainers-go` to spin up real Postgres instances and test full Deduct flows with proper transaction semantics.

---

## What I Would Do With More Time

- **Pagination** on `GET /wallets/:id/transactions`
- **JWT authentication** replacing static tokens
- **Prometheus metrics** via `MetricsPort`
- **Kafka event publishing** with transactional outbox pattern
- **Integration tests** using `testcontainers-go` against real Postgres
- **Concurrency tests** — 20 goroutines deducting concurrently, assert balance never drops below reserve
- **Rate limiting** per wallet
- **Graceful shutdown** with connection draining
- **Docker Compose** for one-command local setup

---

## Project Structure

```
wallet-service/
├── cmd/server/main.go          # Entrypoint: config, DB, wire, serve
├── internal/
│   ├── api/                    # Router + versioned route groups
│   │   ├── router.go           # Gin engine, /health endpoint, /api group
│   │   └── v1/routes.go        # /api/v1 routes with auth middleware
│   ├── auth/                   # Bearer token parser, CallerContext, AssertOwner
│   ├── business/               # WalletService: createWallet, topup, deduct, get...
│   ├── config/                 # YAML config loading (server, db, auth, business)
│   ├── errors/                 # Sentinel errors + ErrorResponse struct
│   ├── events/                 # EventPublisher interface + no-op implementation
│   ├── handler/                # Gin HTTP handlers, DTOs, response helpers
│   │   ├── wallet_handler.go   # Route handlers
│   │   ├── wallet_dto.go       # Request/response type definitions
│   │   └── response.go         # Shared error mapping and JSON helpers
│   ├── metrics/                # MetricsPort interface + no-op implementation
│   ├── models/                 # Domain structs and enums (Wallet, WalletTransaction...)
│   ├── repository/             # Interfaces + PostgreSQL adapters (pgx v5)
│   │   ├── interfaces.go
│   │   └── postgres/
│   │       ├── wallet_repo.go
│   │       ├── transaction_repo.go
│   │       └── idempotency_repo.go
│   └── utils/                  # DB pool init, slog logger setup
├── resources/
│   └── config.yaml             # Server, DB, auth, business configuration
├── scripts/
│   ├── setup_db.sql            # DB schema (run once before first start)
│   └── order_stub.go           # Order Service integration demo
└── docs/
    ├── HLD.md
    └── LLD.md
```
