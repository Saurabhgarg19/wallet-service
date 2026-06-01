package auth

import (
	"net/http"
	"strings"
	"wallet-service/internal/config"
	"wallet-service/internal/constants"
	apperrors "wallet-service/internal/errors"

	"github.com/gin-gonic/gin"
)

type CallerRole string

const (
	RoleCustomer     CallerRole = "CUSTOMER"
	RoleOrderService CallerRole = "ORDER_SERVICE"
)

type CallerContext struct {
	Role       CallerRole
	CustomerID string
}

const CallerKey = "caller"

func Middleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, constants.AuthBearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, apperrors.ErrorResponse{
				ErrorCode: constants.AuthErrorCodeUnauth,
				Message:   "Authorization header is required.",
			})
			return
		}

		token := strings.TrimPrefix(header, constants.AuthBearerPrefix)

		if token == cfg.Auth.OrderServiceToken {
			c.Set(CallerKey, CallerContext{Role: RoleOrderService, CustomerID: constants.AuthOrderServiceID})
			c.Next()
			return
		}

		if strings.HasPrefix(token, cfg.Auth.CustomerTokenPrefix) {
			customerID := strings.TrimPrefix(token, cfg.Auth.CustomerTokenPrefix)
			if customerID == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, apperrors.ErrorResponse{
					ErrorCode: constants.AuthErrorCodeUnauth,
					Message:   "Invalid customer token.",
				})
				return
			}
			c.Set(CallerKey, CallerContext{Role: RoleCustomer, CustomerID: customerID})
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, apperrors.ErrorResponse{
			ErrorCode: constants.AuthErrorCodeUnauth,
			Message:   "Invalid token.",
		})
	}
}

func GetCaller(c *gin.Context) CallerContext {
	val, _ := c.Get(CallerKey)
	caller, _ := val.(CallerContext)
	return caller
}

// AssertOwner returns ErrForbidden if a CUSTOMER caller does not own the wallet.
// ORDER_SERVICE callers are always allowed through.
func AssertOwner(c *gin.Context, walletCustomerID string) error {
	caller := GetCaller(c)
	if caller.Role == RoleCustomer && caller.CustomerID != walletCustomerID {
		return apperrors.ErrForbidden
	}
	return nil
}

