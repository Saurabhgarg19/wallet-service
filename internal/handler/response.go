package handler

import (
	"errors"
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

