package handler

import (
	"errors"
	"log/slog"
	"net/http"
	apperrors "wallet-service/internal/errors"

	"github.com/gin-gonic/gin"
)


func respondForbidden(c *gin.Context) {
	c.JSON(http.StatusForbidden, apperrors.ErrorResponse{ErrorCode: "FORBIDDEN", Message: "Access denied."})
}

func respondError(c *gin.Context, sentinel error, msg string) {
	status, code := statusFor(sentinel)
	c.JSON(status, apperrors.ErrorResponse{ErrorCode: code, Message: msg})
}

func respondErr(c *gin.Context, err error) {
	status, code, msg := statusAndMessageFor(err)
	if status == http.StatusInternalServerError {
		slog.Error("unhandled error", "path", c.FullPath(), "err", err)
	}
	c.JSON(status, apperrors.ErrorResponse{ErrorCode: code, Message: msg})
}

func statusFor(err error) (int, string) {
	status, code, _ := statusAndMessageFor(err)
	return status, code
}

func statusAndMessageFor(err error) (int, string, string) {
	switch {
	case errors.Is(err, apperrors.ErrInvalidRequest):
		return http.StatusBadRequest, "INVALID_REQUEST", err.Error()
	case errors.Is(err, apperrors.ErrUnauthorized):
		return http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required."
	case errors.Is(err, apperrors.ErrForbidden):
		return http.StatusForbidden, "FORBIDDEN", "Access denied."
	case errors.Is(err, apperrors.ErrWalletNotFound):
		return http.StatusNotFound, "WALLET_NOT_FOUND", "Wallet not found."
	case errors.Is(err, apperrors.ErrInsufficientBalance):
		return http.StatusConflict, "INSUFFICIENT_BALANCE", "Insufficient balance. Wallet must maintain the minimum reserve after deduction."
	case errors.Is(err, apperrors.ErrIdempotencyConflict):
		return http.StatusConflict, "IDEMPOTENCY_CONFLICT", "The same idempotency key cannot be reused with a different amount."
	case errors.Is(err, apperrors.ErrDuplicateWallet):
		return http.StatusConflict, "DUPLICATE_WALLET", "A wallet already exists for this customer."
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred."
	}
}
