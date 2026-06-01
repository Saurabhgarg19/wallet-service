package constants

// Config defaults
const (
	DefaultServerPort            = "8080"
	DefaultCustomerTokenPrefix   = "customer:"
	DefaultMinimumBalanceReserve = 100.0
)

// Auth
const (
	AuthBearerPrefix      = "Bearer "
	AuthOrderServiceID    = "order-service"
	AuthErrorCodeUnauth   = "UNAUTHORIZED"
)

// Transaction / operation statuses
const (
	StatusSuccess            = "SUCCESS"
	StatusInsufficientBalance = "INSUFFICIENT_BALANCE"
)
