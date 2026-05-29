package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"wallet-service/db"
	"wallet-service/internal/config"
	"wallet-service/internal/events"
	"wallet-service/internal/handler"
	"wallet-service/internal/metrics"
	"wallet-service/internal/middleware"
	pgRepo "wallet-service/internal/repository/postgres"
	"wallet-service/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)


func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("db connect error", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("db ping failed", "err", err)
		os.Exit(1)
	}

	if err := runMigrations(cfg.DatabaseURL); err != nil {
		slog.Error("migration error", "err", err)
		os.Exit(1)
	}

	slog.Info("migrations applied")

	walletRepo := pgRepo.NewWalletRepo(pool)
	txnRepo := pgRepo.NewTransactionRepo(pool)
	idemRepo := pgRepo.NewIdempotencyRepo(pool)

	svc := service.NewWalletService(
		pool,
		walletRepo,
		txnRepo,
		idemRepo,
		metrics.NoOpMetricsPort{},
		events.NoOpEventPublisher{},
	)

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

	api := r.Group("/", middleware.Auth(cfg))
	handler.NewWalletHandler(svc).RegisterRoutes(api)

	slog.Info("starting wallet service", "port", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func runMigrations(databaseURL string) error {
	d, err := iofs.New(db.MigrationsFS, "migrations")
	if err != nil {
		return err
	}

	// golang-migrate pgx/v5 driver uses pgx5:// scheme
	pgxURL := "pgx5://" + databaseURL[len("postgres://"):]
	if len(databaseURL) >= len("postgresql://") && databaseURL[:len("postgresql://")] == "postgresql://" {
		pgxURL = "pgx5://" + databaseURL[len("postgresql://"):]
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, pgxURL)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

