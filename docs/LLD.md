# Wallet Service — Low-Level Design (Go + PostgreSQL)

## 1. Project Layout

```text
wallet-service/
├── cmd/
│   └── server/
│       └── main.go                  # Entrypoint: config, DB pool init, wire, serve
├── internal/
│   ├── api/
│   │   ├── router.go                # Gin engine + /health, mounts /api group
│   │   └── v1/
│   │       └── routes.go            # /api/v1 routes + auth middleware
│   ├── auth/
│   │   └── middleware.go            # Bearer token parser, CallerContext, AssertOwner
│   ├── business/
│   │   └── wallet_service.go        # WalletService: all business workflows + tests
│   ├── config/
│   │   └── config.go                # YAML-based config
│   ├── errors/
│   │   └── errors.go                # Sentinel errors + ErrorResponse struct
│   ├── events/
│   │   ├── events.go                # EventPublisher interface
│   │   └── (noop implementation inline)
│   ├── handler/
│   │   ├── wallet_handler.go        # Gin handlers for all 6 endpoints
│   │   ├── wallet_dto.go            # Request/response type definitions
│   │   └── response.go              # Shared response/error helpers
│   ├── metrics/
│   │   ├── metrics.go               # MetricsPort interface
│   │   └── (noop implementation inline)
│   ├── models/
│   │   └── models.go                # Wallet, WalletTransaction, IdempotencyRecord structs + enums
│   ├── repository/
│   │   ├── interfaces.go            # WalletRepository, TransactionRepository, IdempotencyRepository
│   │   └── postgres/
│   │       ├── wallet_repo.go       # pgx-backed WalletRepository
│   │       ├── transaction_repo.go  # pgx-backed TransactionRepository
│   │       └── idempotency_repo.go  # pgx-backed IdempotencyRepository
│   └── utils/
│       ├── db.go                    # pgxpool.Pool init helper
│       └── logger.go                # slog setup helper
├── resources/
│   └── config.yaml                  # Server, DB, auth configuration
├── scripts/
│   ├── setup_db.sql                 # Manual DB schema setup (run once)
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

`internal/config/config.go` — loaded from `resources/config.yaml`:

```yaml
server:
  port: "8080"

database:
  url: "postgres://wallet_user:wallet_pass@localhost:5432/wallet_db"

auth:
  customer_token_prefix: "customer:"
  order_service_token: "order-service-secret"

business:
  minimum_balance_reserve: 100.0
```

```go
type Config struct {
    Server   ServerConfig
    Database DatabaseConfig
    Auth     AuthConfig
    Business BusinessConfig
}

type ServerConfig struct {
    Port string
}

type DatabaseConfig struct {
    URL string
}

type AuthConfig struct {
    CustomerTokenPrefix string
    OrderServiceToken   string
}

type BusinessConfig struct {
    MinimumBalanceReserve float64 `yaml:"minimum_balance_reserve"`
}
```

Default: `MinimumBalanceReserve = 100.0` if not set in config.

---

## 3. DB Schema

### `scripts/setup_db.sql` (run once manually)

```sql
-- Wallets table — balance >= 0 is the DB safety net.
-- Minimum reserve is enforced by application via config (business.minimum_balance_reserve).
CREATE TABLE IF NOT EXISTS wallets (
    wallet_id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id TEXT        NOT NULL,
    balance     NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (balance >= 0),
    version     INT         NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_wallets_customer ON wallets(customer_id);

-- Money movement type enum
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'money_movement_type') THEN
        CREATE TYPE money_movement_type AS ENUM ('TOPUP', 'DEDUCT');
    END IF;
END $$;

-- Transactions ledger
CREATE TABLE IF NOT EXISTS wallet_transactions (
    transaction_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id      UUID        NOT NULL REFERENCES wallets(wallet_id),
    type           money_movement_type NOT NULL,
    amount         NUMERIC(12,2) NOT NULL CHECK (amount > 0),
    reference_id   TEXT,
    idempotency_key TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_txn_wallet_id ON wallet_transactions(wallet_id);

-- Deduction outcome enum
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'deduction_outcome') THEN
        CREATE TYPE deduction_outcome AS ENUM ('SUCCESS', 'INSUFFICIENT_BALANCE');
    END IF;
END $$;

-- Idempotency tracking (stores even failures)
CREATE TABLE IF NOT EXISTS deduction_idempotency (
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

### `internal/models/models.go`

```go
type MoneyMovementType string

const (
    MovementTopUp  MoneyMovementType = "TOPUP"
    MovementDeduct MoneyMovementType = "DEDUCT"
)

type DeductionOutcome string

const (
    OutcomeSuccess             DeductionOutcome = "SUCCESS"
    OutcomeInsufficientBalance DeductionOutcome = "INSUFFICIENT_BALANCE"
)

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

Error-to-HTTP-status mapping (in `internal/handler/response.go`):

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
    Create(ctx context.Context, wallet *models.Wallet) (*models.Wallet, error)
    FindByID(ctx context.Context, walletID string) (*models.Wallet, error)
    DebitBalance(ctx context.Context, tx pgx.Tx, walletID string, amount float64, minReserve float64) (float64, error)
    CreditBalance(ctx context.Context, tx pgx.Tx, walletID string, amount float64) (float64, error)
}

type TransactionRepository interface {
    Append(ctx context.Context, tx pgx.Tx, t *models.WalletTransaction) (*models.WalletTransaction, error)
    FindByWalletID(ctx context.Context, walletID string) ([]*models.WalletTransaction, error)
}

type IdempotencyRepository interface {
    Find(ctx context.Context, tx pgx.Tx, walletID, key string) (*models.IdempotencyRecord, error)
    Save(ctx context.Context, tx pgx.Tx, record *models.IdempotencyRecord) error
}
```

---

## 7. Repository Implementations

### WalletRepository — Debit Strategy (configurable reserve)

```go
// DebitBalance atomically decreases balance while maintaining the configured minimum reserve.
// minReserve is passed as a SQL parameter ($3) — no SQL recompilation needed when config changes.
// Returns new balance. Returns ErrInsufficientBalance if 0 rows affected.
func (r *WalletRepo) DebitBalance(ctx context.Context, tx pgx.Tx, walletID string, amount float64, minReserve float64) (float64, error) {
    var newBalance float64
    err := tx.QueryRow(ctx,
        `UPDATE wallets
         SET balance = balance - $1, version = version + 1
         WHERE wallet_id = $2 AND balance >= $1::numeric + $3::numeric
         RETURNING balance`,
        amount, walletID, minReserve,
    ).Scan(&newBalance)
    if errors.Is(err, pgx.ErrNoRows) {
        var exists bool
        _ = r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wallets WHERE wallet_id=$1)`, walletID).Scan(&exists)
        if !exists {
            return 0, apperrors.ErrWalletNotFound
        }
        return 0, apperrors.ErrInsufficientBalance
    }
    return newBalance, err
}
```

> **pgx v5 note**: Explicit `::numeric` casts are required when multiple untyped numeric parameters appear in an arithmetic expression. Without them PostgreSQL raises `operator is not unique: unknown + unknown`.

### TransactionRepository — Reverse Chronological Order + ENUM cast

```go
func (r *TransactionRepo) Append(ctx context.Context, tx pgx.Tx, t *models.WalletTransaction) (*models.WalletTransaction, error) {
    err := tx.QueryRow(ctx,
        `INSERT INTO wallet_transactions (wallet_id, type, amount, reference_id, idempotency_key)
         VALUES ($1, $2::money_movement_type, $3, $4, $5)
         RETURNING transaction_id, created_at`,
        t.WalletID, string(t.Type), t.Amount, nullableString(t.ReferenceID), nullableString(t.IdempotencyKey),
    ).Scan(&t.TransactionID, &t.CreatedAt)
    return t, err
}

func (r *TransactionRepo) FindByWalletID(ctx context.Context, walletID string) ([]*models.WalletTransaction, error) {
    rows, err := r.db.Query(ctx,
        `SELECT transaction_id, wallet_id, type, amount,
                COALESCE(reference_id, ''), COALESCE(idempotency_key, ''), created_at
         FROM wallet_transactions
         WHERE wallet_id = $1
         ORDER BY created_at DESC`,  -- Newest first
        walletID,
    )
    // ...scan rows...
}
```

> **pgx v5 note**: Custom PostgreSQL ENUM types (`money_movement_type`, `deduction_outcome`) require explicit SQL casts (`$2::money_movement_type`). pgx v5 does not auto-register custom ENUM types, so passing the Go custom string type directly causes a `cannot find encode plan` error. The fix is to pass `string(t.Type)` with an explicit cast.

---

## 8. Service Layer

### `internal/business/wallet_service.go`

```go
type WalletService struct {
    db                *pgxpool.Pool
    wallets           repository.WalletRepository
    txns              repository.TransactionRepository
    idem              repository.IdempotencyRepository
    metrics           metrics.MetricsPort
    events            events.EventPublisher
    minBalanceReserve float64  // loaded from config: business.minimum_balance_reserve
}
```

### 8.1 CreateWallet (with configurable minimum reserve)

1. Validate `initialBalance >= minBalanceReserve` → reject with `ErrInvalidRequest` if not.
2. Build `models.Wallet{CustomerID: callerCustomerID, Balance: initialBalance}`.
3. Call `walletRepo.Create(ctx, wallet)`.
4. Emit `WalletCreated` event.
5. Return wallet.

### 8.3 Deduct (Full Workflow with failure storage)

1. Validate `idempotencyKey != ""` and `amount > 0`.
2. Begin DB transaction.
3. `idem.Find(ctx, tx, walletID, idempotencyKey)`.
   - **Record exists, amount matches** → rollback (no-op), return stored outcome with `ServedFromCache=true`.
   - **Record exists, amount differs** → rollback, return `ErrIdempotencyConflict`.
4. `wallets.DebitBalance(ctx, tx, walletID, amount, minBalanceReserve)`. (checks `balance >= amount + minBalanceReserve`)
   - **0 rows** → **store `INSUFFICIENT_BALANCE` idempotency record**, commit, return `ErrInsufficientBalance`.
5. `txns.Append(ctx, tx, &WalletTransaction{Type: DEDUCT, ...})`.
6. `idem.Save(ctx, tx, &IdempotencyRecord{Outcome: SUCCESS, ...})`.
7. Commit.
8. Record metrics, publish event.
9. Return success response.

---

## 9. Authentication and Authorization

### `internal/auth/middleware.go`

Token parsing rules:

| Token format | Role | CustomerID |
|---|---|---|
| `customer:<customerId>` | `CUSTOMER` | extracted from token |
| matches `order_service_token` from config | `ORDER_SERVICE` | `"order-service"` |
| anything else | — | → 401 |

`CallerContext` is injected into Gin context:

```go
type CallerRole string

const (
    RoleCustomer     CallerRole = "CUSTOMER"
    RoleOrderService CallerRole = "ORDER_SERVICE"
)

type CallerContext struct {
    Role       CallerRole
    CustomerID string
}
```

### Authorization Helper

```go
// AssertOwner returns ErrForbidden if a CUSTOMER caller does not own the wallet.
// ORDER_SERVICE callers are always allowed through.
func AssertOwner(c *gin.Context, walletCustomerID string) error {
    caller := GetCaller(c)
    if caller.Role == RoleCustomer && caller.CustomerID != walletCustomerID {
        return apperrors.ErrForbidden
    }
    return nil
}
```

### Authorization Matrix

| Endpoint | CUSTOMER | ORDER_SERVICE |
|---|---|---|
| `POST /api/v1/wallets` | ✅ | ❌ |
| `GET /api/v1/wallets/:id` | ✅ (own wallet) | ✅ |
| `POST /api/v1/wallets/:id/topup` | ✅ (own wallet) | ❌ |
| `POST /api/v1/wallets/:id/deduct` | ❌ | ✅ |
| `GET /api/v1/wallets/:id/balance` | ✅ (own wallet) | ✅ |
| `GET /api/v1/wallets/:id/transactions` | ✅ (own wallet) | ✅ |

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

### Unit Tests — `internal/business/wallet_service_test.go` (10 tests)

Mock repository interfaces using `testify/mock`.

| Test Case | Assertion |
|-----------|-----------|
| `CreateWallet` success (≥ ₹100) | wallet returned, balance set |
| `CreateWallet` below minimum (< ₹100) | `ErrInvalidRequest` returned |
| `CreateWallet` negative balance | `ErrInvalidRequest` returned |
| `CreateWallet` duplicate customer | `ErrDuplicateWallet` returned |
| `Deduct` missing idempotency key | `ErrInvalidRequest` returned |
| `Deduct` invalid amount (≤ 0) | `ErrInvalidRequest` returned |
| `TopUp` invalid amount (≤ 0) | `ErrInvalidRequest` returned |
| `GetWallet` not found | `ErrWalletNotFound` returned |
| `GetTransactions` success | transactions list returned |

**Note:** Full transaction-based tests (TopUp, Deduct with success/failure/idempotency) require mocking `pgxpool.Pool.Begin()` which is complex. These flows are demonstrated end-to-end via the order stub.

### Integration Demonstration — `scripts/order_stub.go`

| Scenario | Verification |
|----------|-------------|
| Create wallet with ≥ ₹100 | 201 Created |
| Top-up | Balance increases |
| First deduct | Success, balance decreased, `cached=false` |
| Retry deduct (same key) | Success, same balance, `cached=true` |
| Deduct until balance < ₹100 + amount | `INSUFFICIENT_BALANCE` |
| Transaction history | Newest first |

### Concurrency Tests (Proposed)

```go
// Fire 20 concurrent deduct goroutines against a wallet with ₹1200.
// Each deducts ₹100.
// Assert exactly 10 succeed (leaving ₹200 = 10 deductions, ₹100 reserve).
// Assert final balance == 100, never drops below 100.
```

---

## 13. Order Service Stub

`scripts/order_stub.go` — runnable with `go run ./scripts/order_stub.go`:

1. `POST /api/v1/wallets` → create wallet with ₹500 initial balance.
2. `POST /api/v1/wallets/:id/topup` → add ₹300 (total ₹800).
3. Fire `POST /api/v1/wallets/:id/deduct` with `idempotencyKey=order-001`, amount=110 → succeeds (₹800 → ₹690).
4. Retry same request with `idempotencyKey=order-001` → idempotent replay, `cached=true`, balance unchanged.
5. Fire deducts with different keys until balance would drop below ₹100 + amount → gets `INSUFFICIENT_BALANCE`.
6. Print summary of all responses.

---

## 14. Implementation Notes

- **Money precision**: Stored as `NUMERIC(12,2)` (supports paise-level precision). Go uses `float64` for simplicity; upgrade to `github.com/shopspring/decimal` for production.
- **UUIDs**: Generated by PostgreSQL (`gen_random_uuid()`). Go uses string UUIDs everywhere.
- **Config loading**: `resources/config.yaml` parsed via `gopkg.in/yaml.v3`. Includes `business.minimum_balance_reserve` (default 100.0).
- **DB pool**: Managed by `pgxpool.Pool` from `github.com/jackc/pgx/v5`.
- **Logging**: Structured JSON logging via `log/slog` (Go 1.21+).
- **HTTP framework**: Gin (`github.com/gin-gonic/gin`).
- **Versioned API**: All routes under `/api/v1` for future extensibility.
- **Minimum balance reserve**: Passed as SQL parameter `$3` in debit query — reserve changes require only a config update, not a DB migration or SQL rewrite.
- **ENUM casts**: pgx v5 requires `$2::money_movement_type` and `$4::deduction_outcome` explicit casts; Go values passed as `string(enumVal)`.
- **Transaction ordering**: `ORDER BY created_at DESC` for reverse chronological (newest first).
- **psql on macOS**: Use [Postgres.app](https://postgresapp.com/) and add `/Applications/Postgres.app/Contents/Versions/latest/bin` to `PATH`.

