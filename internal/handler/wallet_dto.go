package handler

import "time"

// --- Request types ---

type createWalletRequest struct {
	InitialBalance float64 `json:"initialBalance" binding:"min=0"`
}

type topUpRequest struct {
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	ReferenceID string  `json:"referenceId"`
}

type deductRequest struct {
	IdempotencyKey string  `json:"idempotencyKey" binding:"required"`
	Amount         float64 `json:"amount" binding:"required,gt=0"`
	ReferenceID    string  `json:"referenceId"`
}

// --- Response types ---

type walletResponse struct {
	WalletID   string    `json:"walletId"`
	CustomerID string    `json:"customerId"`
	Balance    float64   `json:"balance"`
	CreatedAt  time.Time `json:"createdAt"`
}

type topUpResponse struct {
	WalletID      string  `json:"walletId"`
	Balance       float64 `json:"balance"`
	TransactionID string  `json:"transactionId"`
	Status        string  `json:"status"`
}

type deductResponse struct {
	WalletID                   string  `json:"walletId"`
	Balance                    float64 `json:"balance"`
	TransactionID              string  `json:"transactionId"`
	Status                     string  `json:"status"`
	DeductedAmount             float64 `json:"deductedAmount"`
	ServedFromIdempotencyCache bool    `json:"servedFromIdempotencyCache"`
}

type balanceResponse struct {
	WalletID string  `json:"walletId"`
	Balance  float64 `json:"balance"`
}

type transactionItem struct {
	TransactionID  string    `json:"transactionId"`
	Type           string    `json:"type"`
	Amount         float64   `json:"amount"`
	ReferenceID    *string   `json:"referenceId"`
	IdempotencyKey *string   `json:"idempotencyKey"`
	CreatedAt      time.Time `json:"createdAt"`
}

