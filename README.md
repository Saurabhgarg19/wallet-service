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
- Order Service token: configured in `resources/config.yaml`

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

### Test Methodology

| Layer | What's Tested | Approach |
|-------|--------------|----------|
| Business logic | `WalletService` | Unit tests with mock repositories |
| Correctness | Balance constraint, idempotency | Tests verify `ErrInsufficientBalance`, duplicate key returns cached result |
| Edge cases | Negative balance, invalid inputs | Sentinel error assertions |

**Hard cases covered:**
- Create wallet with negative balance → `ErrInvalidRequest`
- Duplicate wallet for same customer → `ErrDuplicateWallet`
- Wallet not found → `ErrWalletNotFound`
- Deduction with same idempotency key → returns cached result
- Deduction with same key but different amount → `ErrIdempotencyConflict`

---

## Business Rules

- **Single currency**: All amounts in INR (₹)
- **One wallet per customer**: Enforced by `UNIQUE INDEX` on `customer_id`
- **Minimum balance reserve**: ₹100 — balance can never drop below this threshold
- **Idempotency**: Only `/deduct` endpoint requires idempotency key
- **Transaction history**: Returned in reverse chronological order (newest first)

---

## Key Design Decisions

### Atomic conditional balance update
The debit path uses a single SQL statement that enforces the ₹100 minimum reserve:
```sql
UPDATE wallets SET balance = balance - $1, version = version + 1
WHERE wallet_id = $2 AND balance >= $1 + 100
RETURNING balance
```
Ensures balance never drops below ₹100. Race-condition-free with no application-level locking.

### Idempotent deductions
`deduction_idempotency` has `PRIMARY KEY (wallet_id, idempotency_key)`. On every deduct:
1. Check if record exists → replay stored outcome.
2. Amount differs → `IDEMPOTENCY_CONFLICT`.
3. New → debit + ledger + idempotency record in one transaction.

### Single DB transaction per mutation
Balance update, ledger append, and idempotency record committed together. No partial states possible.

### Layered architecture
`handler → business → repository`. Repositories are interfaces — swappable for mocks in tests or other backends in future.

### Static token auth
Customer tokens: `customer:<customerId>`. Order Service uses a config file token. Simple and demonstrable; upgrade path is JWT or mTLS.

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
├── cmd/server/main.go          # Entrypoint: config, DB, wire, serve
├── internal/
│   ├── api/                    # Router + versioned route groups
│   │   ├── router.go
│   │   └── v1/routes.go
│   ├── auth/                   # Auth middleware (CallerContext injection)
│   ├── business/               # Business logic (createWallet, topup, deduct...)
│   ├── config/                 # YAML config loading
│   ├── errors/                 # Sentinel errors + ErrorResponse
│   ├── events/                 # EventPublisher interface + no-op
│   ├── handler/                # Gin HTTP handlers + DTOs
│   ├── metrics/                # MetricsPort interface + no-op
│   ├── models/                 # Domain models and types
│   ├── repository/             # Interfaces + PostgreSQL adapters (pgx)
│   └── utils/                  # DB pool + logger initialization
├── resources/
│   └── config.yaml             # Server, DB, auth configuration
├── scripts/
│   ├── setup_db.sql            # DB schema (run once before first start)
│   └── order_stub.go           # Order Service integration demo
└── docs/
    ├── HLD.md
    └── LLD.md
```
