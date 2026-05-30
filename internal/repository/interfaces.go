package repository

import (
	"context"
	"wallet-service/internal/models"

	"github.com/jackc/pgx/v5"
)

type WalletRepository interface {
	Create(ctx context.Context, wallet *models.Wallet) (*models.Wallet, error)
	FindByID(ctx context.Context, walletID string) (*models.Wallet, error)
	CreditBalance(ctx context.Context, tx pgx.Tx, walletID string, amount float64) (float64, error)
	DebitBalance(ctx context.Context, tx pgx.Tx, walletID string, amount float64) (float64, error)
}

type TransactionRepository interface {
	Append(ctx context.Context, tx pgx.Tx, t *models.WalletTransaction) (*models.WalletTransaction, error)
	FindByWalletID(ctx context.Context, walletID string) ([]*models.WalletTransaction, error)
}

type IdempotencyRepository interface {
	Find(ctx context.Context, tx pgx.Tx, walletID, key string) (*models.IdempotencyRecord, error)
	Save(ctx context.Context, tx pgx.Tx, record *models.IdempotencyRecord) error
}
