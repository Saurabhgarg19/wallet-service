package api

import (
	"net/http"
	v1 "wallet-service/internal/api/v1"
	"wallet-service/internal/config"
	"wallet-service/internal/handler"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(cfg *config.Config, pool *pgxpool.Pool, h *handler.WalletHandler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	r.GET("/health", func(c *gin.Context) {
		if err := pool.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// All API routes are versioned under /api
	api := r.Group("/api")
	v1.Register(api, cfg, h)

	return r
}
