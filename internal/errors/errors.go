package apperrors

import "errors"

var (
	ErrWalletNotFound      = errors.New("WALLET_NOT_FOUND")
	ErrInsufficientBalance = errors.New("INSUFFICIENT_BALANCE")
	ErrIdempotencyConflict = errors.New("IDEMPOTENCY_CONFLICT")
	ErrDuplicateWallet     = errors.New("DUPLICATE_WALLET")
	ErrUnauthorized        = errors.New("UNAUTHORIZED")
	ErrForbidden           = errors.New("FORBIDDEN")
	ErrInvalidRequest      = errors.New("INVALID_REQUEST")
)

type ErrorResponse struct {
	ErrorCode string `json:"errorCode"`
	Message   string `json:"message"`
}

