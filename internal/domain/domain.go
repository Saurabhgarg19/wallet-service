package domain

import "time"

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

