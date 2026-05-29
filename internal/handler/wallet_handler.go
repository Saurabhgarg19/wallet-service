package handler

import (
	"errors"
	"net/http"
	"time"
	apperrors "wallet-service/internal/errors"
	"wallet-service/internal/middleware"
	"wallet-service/internal/service"

	"github.com/gin-gonic/gin"
)

type WalletHandler struct {
	svc *service.WalletService
}

func NewWalletHandler(svc *service.WalletService) *WalletHandler {
	return &WalletHandler{svc: svc}
}

func (h *WalletHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.POST("/wallets", h.createWallet)
	r.GET("/wallets/:id", h.getWallet)
	r.POST("/wallets/:id/topup", h.topUp)
	r.POST("/wallets/:id/deduct", h.deduct)
	r.GET("/wallets/:id/balance", h.getBalance)
	r.GET("/wallets/:id/transactions", h.getTransactions)
}

// --- Request / Response types ---

type createWalletRequest struct {
	InitialBalance float64 `json:"initialBalance" binding:"min=0"`
}

type walletResponse struct {
	WalletID   string    `json:"walletId"`
	CustomerID string    `json:"customerId"`
	Balance    float64   `json:"balance"`
	CreatedAt  time.Time `json:"createdAt"`
}

type topUpRequest struct {
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	ReferenceID string  `json:"referenceId"`
}

type topUpResponse struct {
	WalletID      string  `json:"walletId"`
	Balance       float64 `json:"balance"`
	TransactionID string  `json:"transactionId"`
	Status        string  `json:"status"`
}

type deductRequest struct {
	IdempotencyKey string  `json:"idempotencyKey" binding:"required"`
	Amount         float64 `json:"amount" binding:"required,gt=0"`
	ReferenceID    string  `json:"referenceId"`
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

// --- Handlers ---

func (h *WalletHandler) createWallet(c *gin.Context) {
	caller := middleware.GetCaller(c)
	if caller.Role != middleware.RoleCustomer {
		respondForbidden(c)
		return
	}

	var req createWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperrors.ErrInvalidRequest, err.Error())
		return
	}

	w, err := h.svc.CreateWallet(c.Request.Context(), caller.CustomerID, req.InitialBalance)
	if err != nil {
		respondErr(c, err)
		return
	}

	c.JSON(http.StatusCreated, walletResponse{
		WalletID:   w.WalletID,
		CustomerID: w.CustomerID,
		Balance:    w.Balance,
		CreatedAt:  w.CreatedAt,
	})
}

func (h *WalletHandler) getWallet(c *gin.Context) {
	w, err := h.svc.GetWallet(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondErr(c, err)
		return
	}
	if err := assertOwner(c, w.CustomerID); err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, walletResponse{
		WalletID:   w.WalletID,
		CustomerID: w.CustomerID,
		Balance:    w.Balance,
		CreatedAt:  w.CreatedAt,
	})
}

func (h *WalletHandler) topUp(c *gin.Context) {
	caller := middleware.GetCaller(c)
	if caller.Role != middleware.RoleCustomer {
		respondForbidden(c)
		return
	}

	walletID := c.Param("id")

	// Ownership check before mutation.
	w, err := h.svc.GetWallet(c.Request.Context(), walletID)
	if err != nil {
		respondErr(c, err)
		return
	}
	if w.CustomerID != caller.CustomerID {
		respondForbidden(c)
		return
	}

	var req topUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperrors.ErrInvalidRequest, err.Error())
		return
	}

	result, err := h.svc.TopUp(c.Request.Context(), walletID, req.Amount, req.ReferenceID)
	if err != nil {
		respondErr(c, err)
		return
	}

	c.JSON(http.StatusOK, topUpResponse{
		WalletID:      result.WalletID,
		Balance:       result.Balance,
		TransactionID: result.TransactionID,
		Status:        "SUCCESS",
	})
}

func (h *WalletHandler) deduct(c *gin.Context) {
	caller := middleware.GetCaller(c)
	if caller.Role != middleware.RoleOrderService {
		respondForbidden(c)
		return
	}

	var req deductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperrors.ErrInvalidRequest, err.Error())
		return
	}

	result, err := h.svc.Deduct(c.Request.Context(), c.Param("id"), req.IdempotencyKey, req.Amount, req.ReferenceID)
	if err != nil {
		respondErr(c, err)
		return
	}

	c.JSON(http.StatusOK, deductResponse{
		WalletID:                   result.WalletID,
		Balance:                    result.Balance,
		TransactionID:              result.TransactionID,
		Status:                     "SUCCESS",
		DeductedAmount:             result.DeductedAmount,
		ServedFromIdempotencyCache: result.ServedFromIdempotencyCache,
	})
}

func (h *WalletHandler) getBalance(c *gin.Context) {
	w, err := h.svc.GetBalance(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondErr(c, err)
		return
	}
	if err := assertOwner(c, w.CustomerID); err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, balanceResponse{WalletID: w.WalletID, Balance: w.Balance})
}

func (h *WalletHandler) getTransactions(c *gin.Context) {
	walletID := c.Param("id")
	w, err := h.svc.GetWallet(c.Request.Context(), walletID)
	if err != nil {
		respondErr(c, err)
		return
	}
	if err := assertOwner(c, w.CustomerID); err != nil {
		respondErr(c, err)
		return
	}

	txns, err := h.svc.GetTransactions(c.Request.Context(), walletID)
	if err != nil {
		respondErr(c, err)
		return
	}

	type txnItem struct {
		TransactionID  string    `json:"transactionId"`
		Type           string    `json:"type"`
		Amount         float64   `json:"amount"`
		ReferenceID    *string   `json:"referenceId"`
		IdempotencyKey *string   `json:"idempotencyKey"`
		CreatedAt      time.Time `json:"createdAt"`
	}

	items := make([]txnItem, len(txns))
	for i, t := range txns {
		item := txnItem{
			TransactionID: t.TransactionID,
			Type:          string(t.Type),
			Amount:        t.Amount,
			CreatedAt:     t.CreatedAt,
		}
		if t.ReferenceID != "" {
			item.ReferenceID = &t.ReferenceID
		}
		if t.IdempotencyKey != "" {
			item.IdempotencyKey = &t.IdempotencyKey
		}
		items[i] = item
	}
	c.JSON(http.StatusOK, items)
}

// --- Helpers ---

func assertOwner(c *gin.Context, walletCustomerID string) error {
	caller := middleware.GetCaller(c)
	if caller.Role == middleware.RoleCustomer && caller.CustomerID != walletCustomerID {
		return apperrors.ErrForbidden
	}
	return nil
}

func respondForbidden(c *gin.Context) {
	c.JSON(http.StatusForbidden, apperrors.ErrorResponse{ErrorCode: "FORBIDDEN", Message: "Access denied."})
}

func respondError(c *gin.Context, sentinel error, msg string) {
	status, code := statusFor(sentinel)
	c.JSON(status, apperrors.ErrorResponse{ErrorCode: code, Message: msg})
}

func respondErr(c *gin.Context, err error) {
	status, code := statusFor(err)
	c.JSON(status, apperrors.ErrorResponse{ErrorCode: code, Message: err.Error()})
}

func statusFor(err error) (int, string) {
	switch {
	case errors.Is(err, apperrors.ErrInvalidRequest):
		return http.StatusBadRequest, "INVALID_REQUEST"
	case errors.Is(err, apperrors.ErrUnauthorized):
		return http.StatusUnauthorized, "UNAUTHORIZED"
	case errors.Is(err, apperrors.ErrForbidden):
		return http.StatusForbidden, "FORBIDDEN"
	case errors.Is(err, apperrors.ErrWalletNotFound):
		return http.StatusNotFound, "WALLET_NOT_FOUND"
	case errors.Is(err, apperrors.ErrInsufficientBalance):
		return http.StatusConflict, "INSUFFICIENT_BALANCE"
	case errors.Is(err, apperrors.ErrIdempotencyConflict):
		return http.StatusConflict, "IDEMPOTENCY_CONFLICT"
	case errors.Is(err, apperrors.ErrDuplicateWallet):
		return http.StatusConflict, "DUPLICATE_WALLET"
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR"
	}
}

