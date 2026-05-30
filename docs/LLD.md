# Wallet Service — Low-Level Design (Go + PostgreSQL)

## 1. Project Layout

```text
wallet-service/
├── cmd/
│   └── server/
│       └── main.go                  # Entrypoint: config, DB init, migrate, wire, serve
├── internal/
│   ├── config/
│   │   └── config.go                # Env-based config (DB URL, auth tokens, port)
│   ├── domain/
│   │   ├── wallet.go                # Wallet, WalletTransaction, IdempotencyRecord structs
│   │   └── types.go                 # MoneyMovementType enum, DeductionOutcome enum
│   ├── errors/
│   │   └── errors.go                # Sentinel errors + ErrorResponse struct
│   ├── repository/
│   │   ├── interfaces.go            # WalletRepository, TransactionRepository, IdempotencyRepository
│   │   └── postgres/
│   │       ├── wallet_repo.go       # pgx-backed WalletRepository
│   │       ├── transaction_repo.go  # pgx-backed TransactionRepository
│   │       ├── idempotency_repo.go  # pgx-backed IdempotencyRepository
│   │       └── db.go                # pgxpool.Pool init helper
│   ├── service/
│   │   └── wallet_service.go        # WalletService: all business workflows
│   ├── handler/
│   │   ├── wallet_handler.go        # Gin handlers for all 6 endpoints + health
│   │   └── response.go              # Shared response/error helpers
│   ├── middleware/
│   │   └── auth.go                  # Bearer token parser, CallerContext injection
│   ├── metrics/
│   │   ├── port.go                  # MetricsPort interface
│   │   └── noop.go                  # NoOpMetricsPort implementation
│   └── events/
│       ├── publisher.go             # EventPublisher interface
│       └── noop.go                  # NoOpEventPublisher implementation
├── db/
│   └── migrations/
│       ├── 000001_wallets.up.sql
│       ├── 000001_wallets.down.sql
│       ├── 000002_wallet_transactions.up.sql
│       ├── 000002_wallet_transactions.down.sql
│       ├── 000003_deduction_idempotency.up.sql
│       └── 000003_deduction_idempotency.down.sql
├── scripts/
│   └── order_stub.go                # Runnable Order Service integration stub
├── docs/
│   ├── HLD.md
│   └── LLD.md
├── go.mod
├── go.sum
└── README.md
```

---

## 2. Configuration

`internal/config/config.go` — loaded from environment variables:

| Env Var | Description | Default |
|---------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | required |
| `PORT` | HTTP listen port | `8080` |
| `CUSTOMER_TOKEN_PREFIX` | Prefix for customer bearer tokens | `customer:` |
| `ORDER_SERVICE_TOKEN` | Static token for Order Service | required |
| `LOG_LEVEL` | `debug` / `info` / `warn` | `info` |

```go
type Config struct {
    DatabaseURL          string
    Port                 string
    CustomerTokenPrefix  string
    OrderServiceToken    string
    LogLevel             string
}
```

---

## 3. DB Schema

### 000001_wallets.up.sql
```sql
CREATE TABLE wallets (
    wallet_id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id TEXT        NOT NULL,
    balance     NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (balance >= 0),
    version     INT         NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_wallets_customer ON wallets(customer_id);
```

### 000002_wallet_transactions.up.sql
```sql
CREATE TYPE money_movement_type AS ENUM ('TOPUP', 'DEDUCT');

CREATE TABLE wallet_transactions (
    transaction_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id      UUID        NOT NULL REFERENCES wallets(wallet_id),
    type           money_movement_type NOT NULL,
    amount         NUMERIC(12,2) NOT NULL CHECK (amount > 0),
    reference_id   TEXT,
    idempotency_key TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_txn_wallet_id ON wallet_transactions(wallet_id);
```

### 000003_deduction_idempotency.up.sql
```sql
CREATE TYPE deduction_outcome AS ENUM ('SUCCESS', 'INSUFFICIENT_BALANCE');

CREATE TABLE deduction_idempotency (
    wallet_id        UUID        NOT NULL REFERENCES wallets(wallet_id),
    idempotency_key  TEXT        NOT NULL,
    requested_amount NUMERIC(12,2) NOT NULL,
    outcome          deduction_outcome NOT NULL,
    transaction_id   UUID,
    balance_after    NUMERIC(12,2),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (wallet_id, idempotency_key)
);
```

---

## 4. Domain Models

### `internal/domain/wallet.go`

```go
type Wallet struct {
    WalletID   string
    CustomerID string
    Balance    float64
    Version    int
    CreatedAt  time.Time
}

type WalletTransaction struct {
    TransactionID  string
    WalletID       string
    Type           MoneyMovementType
    Amount         float64
    ReferenceID    string
    IdempotencyKey string
    CreatedAt      time.Time
}

type IdempotencyRecord struct {
    WalletID        string
    IdempotencyKey  string
    RequestedAmount float64
    Outcome         DeductionOutcome
    TransactionID   string
    BalanceAfter    float64
    CreatedAt       time.Time
}
```

### `internal/domain/types.go`

```go
type MoneyMovementType string

const (
    TopUp  MoneyMovementType = "TOPUP"
    Deduct MoneyMovementType = "DEDUCT"
)

type DeductionOutcome string

const (
    OutcomeSuccess            DeductionOutcome = "SUCCESS"
    OutcomeInsufficientBalance DeductionOutcome = "INSUFFICIENT_BALANCE"
)
```

---

## 5. Error Model

### `internal/errors/errors.go`

```go
var (
    ErrWalletNotFound       = errors.New("WALLET_NOT_FOUND")
    ErrInsufficientBalance  = errors.New("INSUFFICIENT_BALANCE")
    ErrIdempotencyConflict  = errors.New("IDEMPOTENCY_CONFLICT")
    ErrDuplicateWallet      = errors.New("DUPLICATE_WALLET")
    ErrUnauthorized         = errors.New("UNAUTHORIZED")
    ErrForbidden            = errors.New("FORBIDDEN")
    ErrInvalidRequest       = errors.New("INVALID_REQUEST")
)

type ErrorResponse struct {
    ErrorCode string `json:"errorCode"`
    Message   string `json:"message"`
}
```

Error-to-HTTP-status mapping (in handler):

| Sentinel Error | HTTP Status |
|----------------|-------------|
| `ErrInvalidRequest` | 400 |
| `ErrUnauthorized` | 401 |
| `ErrForbidden` | 403 |
| `ErrWalletNotFound` | 404 |
| `ErrInsufficientBalance` | 409 |
| `ErrIdempotencyConflict` | 409 |
| `ErrDuplicateWallet` | 409 |
| Any other | 500 |

---

## 6. Repository Interfaces

### `internal/repository/interfaces.go`

```go
type WalletRepository interface {
    Create(ctx context.Context, wallet *domain.Wallet) (*domain.Wallet, error)
    FindByID(ctx context.Context, walletID string) (*domain.Wallet, error)
    DebitBalance(ctx context.Context, tx pgx.Tx, walletID string, amount float64) (float64, error)
    CreditBalance(ctx context.Context, tx pgx.Tx, walletID string, amount float64) (float64, error)
}

type TransactionRepository interface {
    Append(ctx context.Context, tx pgx.Tx, t *domain.WalletTransaction) (*domain.WalletTransaction, error)
    FindByWalletID(ctx context.Context, walletID string) ([]*domain.WalletTransaction, error)
}

type IdempotencyRepository interface {
    Find(ctx context.Context, tx pgx.Tx, walletID, key string) (*domain.IdempotencyRecord, error)
    Save(ctx context.Context, tx pgx.Tx, record *domain.IdempotencyRecord) error
}
```

---

## 7. Repository Implementations

### WalletRepository — Debit Strategy

```go
// DebitBalance uses atomic conditional UPDATE — no separate balance read needed.
// Returns new balance. Returns ErrInsufficientBalance if 0 rows affected.
func (r *pgWalletRepo) DebitBalance(ctx context.Context, tx pgx.Tx, walletID string, amount float64) (float64, error) {
    var newBalance float64
    err := tx.QueryRow(ctx,
        `UPDATE wallets
         SET balance = balance - $1, version = version + 1
         WHERE wallet_id = $2 AND balance >= $1
         RETURNING balance`,
        amount, walletID,
    ).Scan(&newBalance)
    if errors.Is(err, pgx.ErrNoRows) {
        return 0, apperrors.ErrInsufficientBalance
    }
    return newBalance, err
}
```

### WalletRepository — Credit Strategy (TopUp)

```go
func (r *pgWalletRepo) CreditBalance(ctx context.Context, tx pgx.Tx, walletID string, amount float64) (float64, error) {
    var newBalance float64
    err := tx.QueryRow(ctx,
        `UPDATE wallets
         SET balance = balance + $1, version = version + 1
         WHERE wallet_id = $2
         RETURNING balance`,
        amount, walletID,
    ).Scan(&newBalance)
    if errors.Is(err, pgx.ErrNoRows) {
        return 0, apperrors.ErrWalletNotFound
    }
    return newBalance, err
}
```

---

## 8. Service Layer

### `internal/service/wallet_service.go`

```go
type WalletService struct {
    db           *pgxpool.Pool
    walletRepo   repository.WalletRepository
    txnRepo      repository.TransactionRepository
    idemRepo     repository.IdempotencyRepository
    metrics      metrics.MetricsPort
    events       events.EventPublisher
}
```

### 8.1 CreateWallet

1. Validate `initialBalance >= 0`.
2. Build `domain.Wallet{CustomerID: callerCustomerID, Balance: initialBalance}`.
3. Call `walletRepo.Create(ctx, wallet)`.
4. Emit `WalletCreated` event.
5. Return wallet.

### 8.2 TopUp

1. Validate `amount > 0`.
2. Begin DB transaction.
3. `walletRepo.CreditBalance(ctx, tx, walletID, amount)`.
4. `txnRepo.Append(ctx, tx, &WalletTransaction{Type: TOPUP, ...})`.
5. Commit.
6. Record metrics, publish event.
7. Return updated balance + transaction ID.

### 8.3 Deduct (Full Workflow)

1. Validate `idempotencyKey != ""` and `amount > 0`.
2. Begin DB transaction.
3. `idemRepo.Find(ctx, tx, walletID, idempotencyKey)`.
   - **Record exists, amount matches** → commit (no-op), return stored outcome with `ServedFromCache=true`.
   - **Record exists, amount differs** → rollback, return `ErrIdempotencyConflict`.
4. `walletRepo.DebitBalance(ctx, tx, walletID, amount)`.
   - **0 rows** → rollback, store `INSUFFICIENT_BALANCE` record, return `ErrInsufficientBalance`.
5. `txnRepo.Append(ctx, tx, &WalletTransaction{Type: DEDUCT, ...})`.
6. `idemRepo.Save(ctx, tx, &IdempotencyRecord{Outcome: SUCCESS, ...})`.
7. Commit.
8. Record metrics, publish event.
9. Return success response.

### 8.4 GetBalance

1. `walletRepo.FindByID(ctx, walletID)`.
2. Verify caller can access wallet.
3. Return `{ walletId, balance }`.

### 8.5 GetTransactions

1. `walletRepo.FindByID(ctx, walletID)` (ownership check).
2. `txnRepo.FindByWalletID(ctx, walletID)`.
3. Return list.

---

## 9. Authentication and Authorization

### `internal/middleware/auth.go`

Token parsing rules:

| Token format | Role | CustomerID |
|---|---|---|
| `customer:<customerId>` | `CUSTOMER` | extracted from token |
| matches `ORDER_SERVICE_TOKEN` env | `ORDER_SERVICE` | `"order-service"` |
| anything else | — | → 401 |

`CallerContext` is injected into Gin context:

```go
type CallerContext struct {
    Role       CallerRole
    CustomerID string
}
```

### Authorization Matrix

| Endpoint | CUSTOMER | ORDER_SERVICE |
|---|---|---|
| `POST /wallets` | ✅ | ❌ |
| `GET /wallets/:id` | ✅ (own wallet) | ✅ |
| `POST /wallets/:id/topup` | ✅ (own wallet) | ❌ |
| `POST /wallets/:id/deduct` | ❌ | ✅ |
| `GET /wallets/:id/balance` | ✅ (own wallet) | ✅ |
| `GET /wallets/:id/transactions` | ✅ (own wallet) | ✅ |

Ownership check helper in handler layer:

```go
func assertOwner(ctx CallerContext, wallet *domain.Wallet) error {
    if ctx.Role == CUSTOMER && ctx.CustomerID != wallet.CustomerID {
        return ErrForbidden
    }
    return nil
}
```

---

## 10. API Contracts

### POST /wallets
**Request:**
```json
{ "initialBalance": 500 }
```
**Response 201:**
```json
{
  "walletId": "550e8400-e29b-41d4-a716-446655440000",
  "customerId": "cust-101",
  "balance": 500,
  "createdAt": "2026-05-29T10:00:00Z"
}
```

### POST /wallets/:id/topup
**Request:**
```json
{ "amount": 300, "referenceId": "topup-001" }
```
**Response 200:**
```json
{
  "walletId": "...",
  "balance": 800,
  "transactionId": "...",
  "status": "SUCCESS"
}
```

### POST /wallets/:id/deduct
**Request:**
```json
{ "idempotencyKey": "order-9001", "amount": 100, "referenceId": "order-9001" }
```
**Response 200 (success):**
```json
{
  "walletId": "...",
  "balance": 700,
  "transactionId": "...",
  "status": "SUCCESS",
  "deductedAmount": 100,
  "servedFromIdempotencyCache": false
}
```
**Response 409 (insufficient funds):**
```json
{ "errorCode": "INSUFFICIENT_BALANCE", "message": "Wallet balance is lower than the deduction amount." }
```
**Response 409 (conflict):**
```json
{ "errorCode": "IDEMPOTENCY_CONFLICT", "message": "The same idempotency key cannot be reused with a different amount." }
```

### GET /wallets/:id/balance
**Response 200:**
```json
{ "walletId": "...", "balance": 700 }
```

### GET /wallets/:id/transactions
**Response 200:**
```json
[
  {
    "transactionId": "...",
    "type": "TOPUP",
    "amount": 300,
    "referenceId": "topup-001",
    "idempotencyKey": null,
    "createdAt": "2026-05-29T10:05:00Z"
  }
]
```

---

## 11. Metrics and Events

### `internal/metrics/port.go`
```go
type MetricsPort interface {
    RecordCreateWallet()
    RecordTopupSuccess()
    RecordDeductSuccess()
    RecordDeductRejected()
    RecordIdempotentReplay()
    RecordLatency(operation string, duration time.Duration)
}
```

### `internal/events/publisher.go`
```go
type EventPublisher interface {
    PublishWalletCreated(walletID, customerID string)
    PublishWalletToppedUp(walletID string, amount float64)
    PublishWalletDeducted(walletID string, amount float64, txnID string)
    PublishWalletDeductionRejected(walletID, reason string)
}
```

Both have no-op default implementations (`NoOpMetricsPort`, `NoOpEventPublisher`).

---

## 12. Testing Plan

### Unit Tests — `internal/service/wallet_service_test.go`

Mock repository interfaces using `testify/mock` or hand-written stubs.

| Test Case | Assertion |
|-----------|-----------|
| `CreateWallet` success | wallet returned, balance set |
| `TopUp` success | balance increased, transaction appended |
| `Deduct` success | balance decreased, idempotency record saved |
| `Deduct` insufficient balance | `ErrInsufficientBalance` returned, balance unchanged |
| `Deduct` idempotent replay (same amount) | stored outcome returned, no DB mutation |
| `Deduct` idempotency conflict (different amount) | `ErrIdempotencyConflict` returned |
| `Deduct` wallet not found | `ErrWalletNotFound` returned |
| `CreateWallet` invalid initial balance | `ErrInvalidRequest` |

### Handler / Integration Tests — `internal/handler/wallet_handler_test.go`

Use `httptest.NewRecorder()` + real Postgres (testcontainers-go or local Docker).

| Test Case | Expected Status |
|-----------|----------------|
| `POST /wallets` with valid customer token | 201 |
| `POST /wallets` with order-service token | 403 |
| `POST /wallets/:id/deduct` with customer token | 403 |
| `POST /wallets/:id/deduct` with order-service token, sufficient balance | 200 |
| `POST /wallets/:id/deduct` with insufficient balance | 409 |
| Repeated deduct with same idempotency key | 200, `servedFromIdempotencyCache: true` |
| `GET /wallets/:id/balance` with wrong customer token | 403 |
| No auth header | 401 |

### Concurrency Tests

```go
// Fire 20 concurrent deduct goroutines against a wallet with ₹1000.
// Each deducts ₹100. Assert exactly 10 succeed, 10 fail.
// Assert final balance == 0, never negative.
```

---

## 13. Order Service Stub

`scripts/order_stub.go` — runnable with `go run ./scripts/order_stub.go`:

1. `POST /wallets` → create wallet with ₹500 initial balance.
2. `POST /wallets/:id/topup` → add ₹300.
3. Fire `POST /wallets/:id/deduct` with `idempotencyKey=order-001` → succeeds (₹700 → ₹600).
4. Retry same request with `idempotencyKey=order-001` → idempotent replay, balance unchanged.
5. Fire `POST /wallets/:id/deduct` with different key → succeeds.
6. Fire deducts until balance < ₹100 → gets `INSUFFICIENT_BALANCE`.
7. Print summary of all responses.

---

## 14. Implementation Notes

- Money stored as `NUMERIC(12,2)` (supports paise-level precision if needed later). Go side uses `float64` for simplicity in this exercise; upgrade to `github.com/shopspring/decimal` for production.
- `walletId` is UUID generated by PostgreSQL (`gen_random_uuid()`). Go uses string UUIDs everywhere.
- State mutation ordering: DB commit first, then metrics + events (best-effort side effects).
- `golang-migrate` runs `db/migrations/*.up.sql` at service startup via embedded FS.
- Structured JSON logging via `log/slog` (Go 1.21+).
- DB pool managed by `pgxpool.Pool`; connection string from `DATABASE_URL` env var.

