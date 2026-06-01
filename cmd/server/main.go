package main

import (
	"context"
	"log/slog"
	"os"
	"wallet-service/internal/api"
	"wallet-service/internal/business"
	"wallet-service/internal/config"
	"wallet-service/internal/events"
	"wallet-service/internal/handler"
	"wallet-service/internal/metrics"
	pgRepo "wallet-service/internal/repository/postgres"
	"wallet-service/internal/utils"
)

func main() {
	slog.SetDefault(utils.NewLogger())

	cfg, err := config.Load("resources/config.yaml")
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	pool, err := utils.NewDBPool(context.Background(), cfg.Database.URL)
	if err != nil {
		slog.Error("db error", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	svc := business.NewWalletService(
		pool,
		pgRepo.NewWalletRepo(pool),
		pgRepo.NewTransactionRepo(pool),
		pgRepo.NewIdempotencyRepo(pool),
		metrics.NoOpMetricsPort{},
		events.NoOpEventPublisher{},
		cfg.Business.MinimumBalanceReserve,
	)

	r := api.NewRouter(cfg, pool, handler.NewWalletHandler(svc))

	slog.Info("starting wallet service", "port", cfg.Server.Port)
	if err := r.Run(":" + cfg.Server.Port); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
