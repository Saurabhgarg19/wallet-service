package repository

import (
	"context"
	"wallet-service/internal/domain"

	"github.com/jackc/pgx/v5"
)

type WalletRepository interface {
	Create(ctx context.Context, wallet *domain.Wallet) (*domain.Wallet, error)
	FindByID(ctx context.Context, walletID string) (*domain.Wallet, error)
	CreditBalance(ctx context.Context, tx pgx.Tx, walletID string, amount float64) (float64, error)
	DebitBalance(ctx context.Context, tx pgx.Tx, walletID string, amount float64) (float64, error)
}

type TransactionRepository interface {
	Append(ctx context.Context, tx pgx.Tx, t *domain.WalletTransaction) (*domain.WalletTransaction, error)
	FindByWalletID(ctx context.Context, walletID string) ([]*domain.WalletTransaction, error)
}

type IdempotencyRepository interface {
	Find(ctx context.Context, tx pgx.Tx, walletID, key string) (*domain.IdempotencyRecord, error)
	Save(ctx context.Context, tx pgx.Tx, record *domain.IdempotencyRecord) error
}

