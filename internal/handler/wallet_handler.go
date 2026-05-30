package handler

import (
	"net/http"
	"wallet-service/internal/auth"
	"wallet-service/internal/business"
	apperrors "wallet-service/internal/errors"

	"github.com/gin-gonic/gin"
)

type WalletHandler struct {
	svc *business.WalletService
}

func NewWalletHandler(svc *business.WalletService) *WalletHandler {
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


func (h *WalletHandler) createWallet(c *gin.Context) {
	caller := auth.GetCaller(c)
	if caller.Role != auth.RoleCustomer {
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
	if err := auth.AssertOwner(c, w.CustomerID); err != nil {
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
	caller := auth.GetCaller(c)
	if caller.Role != auth.RoleCustomer {
		respondForbidden(c)
		return
	}

	walletID := c.Param("id")
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
	caller := auth.GetCaller(c)
	if caller.Role != auth.RoleOrderService {
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
	if err := auth.AssertOwner(c, w.CustomerID); err != nil {
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
	if err := auth.AssertOwner(c, w.CustomerID); err != nil {
		respondErr(c, err)
		return
	}

	txns, err := h.svc.GetTransactions(c.Request.Context(), walletID)
	if err != nil {
		respondErr(c, err)
		return
	}

	items := make([]transactionItem, len(txns))
	for i, t := range txns {
		item := transactionItem{
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

