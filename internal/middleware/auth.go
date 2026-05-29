package middleware

import (
	"net/http"
	"strings"
	"wallet-service/internal/config"
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

func Auth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, apperrors.ErrorResponse{
				ErrorCode: "UNAUTHORIZED",
				Message:   "Authorization header is required.",
			})
			return
		}

		token := strings.TrimPrefix(header, "Bearer ")

		if token == cfg.OrderServiceToken {
			c.Set(CallerKey, CallerContext{Role: RoleOrderService, CustomerID: "order-service"})
			c.Next()
			return
		}

		if strings.HasPrefix(token, cfg.CustomerTokenPrefix) {
			customerID := strings.TrimPrefix(token, cfg.CustomerTokenPrefix)
			if customerID == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, apperrors.ErrorResponse{
					ErrorCode: "UNAUTHORIZED",
					Message:   "Invalid customer token.",
				})
				return
			}
			c.Set(CallerKey, CallerContext{Role: RoleCustomer, CustomerID: customerID})
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, apperrors.ErrorResponse{
			ErrorCode: "UNAUTHORIZED",
			Message:   "Invalid token.",
		})
	}
}

func GetCaller(c *gin.Context) CallerContext {
	val, _ := c.Get(CallerKey)
	caller, _ := val.(CallerContext)
	return caller
}

