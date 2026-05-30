package v1

import (
	"wallet-service/internal/auth"
	"wallet-service/internal/config"
	"wallet-service/internal/handler"

	"github.com/gin-gonic/gin"
)

// Register mounts all v1 wallet routes under /api/v1 with auth middleware.
func Register(rg *gin.RouterGroup, cfg *config.Config, h *handler.WalletHandler) {
	v1 := rg.Group("/v1", auth.Middleware(cfg))
	h.RegisterRoutes(v1)
}

