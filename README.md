# Wallet Service

A Go + PostgreSQL HTTP API service that manages customer wallet balances for a logistics platform.

---

## Quick Start

### 1. Start PostgreSQL

```bash
docker run --name postgres-wallet \
  -e POSTGRES_USER=wallet_user \
  -e POSTGRES_PASSWORD=wallet_pass \
  -e POSTGRES_DB=wallet_db \
  -p 5432:5432 \
  -d postgres:15
```

### 2. Run the service

```bash
export DATABASE_URL="postgres://wallet_user:wallet_pass@localhost:5432/wallet_db"
export ORDER_SERVICE_TOKEN="order-service-secret"
export CUSTOMER_TOKEN_PREFIX="customer:"
export PORT=8080

go run ./cmd/server
```

Migrations run automatically at startup.

### 3. Run the Order Service stub

```bash
go run ./scripts/order_stub.go
```

---

## API Endpoints

| Method | Path | Role | Purpose |
|--------|------|------|---------|
| `GET` | `/health` | — | Health check |
| `POST` | `/wallets` | CUSTOMER | Create wallet |
| `GET` | `/wallets/:id` | CUSTOMER / ORDER_SERVICE | Get wallet details |
| `POST` | `/wallets/:id/topup` | CUSTOMER | Add funds |
| `POST` | `/wallets/:id/deduct` | ORDER_SERVICE | Deduct (idempotent) |
| `GET` | `/wallets/:id/balance` | CUSTOMER / ORDER_SERVICE | Get balance |
| `GET` | `/wallets/:id/transactions` | CUSTOMER / ORDER_SERVICE | Transaction history |

### Authentication

All endpoints (except `/health`) require `Authorization: Bearer <token>`.

- Customer token: `customer:<customerId>` (e.g. `customer:cust-101`)
- Order Service token: value of `ORDER_SERVICE_TOKEN` env var

---

## Example Requests

```bash
# Create wallet
curl -X POST http://localhost:8080/wallets \
  -H "Authorization: Bearer customer:cust-101" \
  -H "Content-Type: application/json" \
  -d '{"initialBalance": 500}'

# Top up
curl -X POST http://localhost:8080/wallets/<id>/topup \
  -H "Authorization: Bearer customer:cust-101" \
  -H "Content-Type: application/json" \
  -d '{"amount": 300, "referenceId": "topup-001"}'

# Deduct (Order Service)
curl -X POST http://localhost:8080/wallets/<id>/deduct \
  -H "Authorization: Bearer order-service-secret" \
  -H "Content-Type: application/json" \
  -d '{"idempotencyKey": "order-9001", "amount": 100, "referenceId": "order-9001"}'

# Get balance
curl http://localhost:8080/wallets/<id>/balance \
  -H "Authorization: Bearer customer:cust-101"
```

---

## Running Tests

```bash
go test ./...
```

---

## Key Design Decisions

### Atomic conditional balance update
The debit path uses a single SQL statement:
```sql
UPDATE wallets SET balance = balance - $1, version = version + 1
WHERE wallet_id = $2 AND balance >= $1
RETURNING balance
```
Eliminates the read-then-write race condition with no application-level locking.

### Idempotent deductions
`deduction_idempotency` has `PRIMARY KEY (wallet_id, idempotency_key)`. On every deduct:
1. Check if record exists → replay stored outcome.
2. Amount differs → `IDEMPOTENCY_CONFLICT`.
3. New → debit + ledger + idempotency record in one transaction.

### Single DB transaction per mutation
Balance update, ledger append, and idempotency record committed together. No partial states possible.

### Layered architecture
`handler → service → repository`. Repositories are interfaces — swappable for mocks in tests or other backends in future.

### Static token auth
Customer tokens: `customer:<customerId>`. Order Service uses a single env-configured token. Simple and demonstrable; upgrade path is JWT or mTLS.

---

## Trade-offs

| Decision | Trade-off |
|----------|-----------|
| `balance` cached on wallet row | Fast reads; consistent because mutations write wallet + ledger in one transaction |
| Static bearer tokens | Easy to run locally; not production-grade security |
| No pagination on transactions | Acceptable for this scope; add cursor-based pagination for production |
| No-op metrics/events | Runs without external dependencies; replace interfaces for production observability |

---

## What I Would Do With More Time

- **Pagination** on `GET /wallets/:id/transactions`
- **JWT authentication** replacing static tokens
- **Prometheus metrics** via `MetricsPort`
- **Kafka event publishing** with transactional outbox pattern
- **Integration tests** using `testcontainers-go` against real Postgres
- **Concurrency tests** — 20 goroutines deducting concurrently, assert balance never negative
- **Rate limiting** per wallet
- **Graceful shutdown** with connection draining
- **Docker Compose** for one-command local setup

---

## Project Structure

```
wallet-service/
├── cmd/server/main.go          # Entrypoint: config, DB, migrate, wire, serve
├── internal/
│   ├── config/                 # Env config
│   ├── domain/                 # Domain models and types
│   ├── errors/                 # Sentinel errors + ErrorResponse
│   ├── handler/                # Gin HTTP handlers
│   ├── middleware/             # Auth middleware (CallerContext injection)
│   ├── metrics/                # MetricsPort interface + no-op
│   ├── events/                 # EventPublisher interface + no-op
│   ├── repository/             # Interfaces + PostgreSQL adapters (pgx)
│   └── service/                # Business logic (createWallet, topup, deduct...)
├── db/
│   ├── embed.go                # Embeds migration files
│   └── migrations/             # SQL migration files (up + down)
└── scripts/order_stub.go       # Order Service integration demo
```
