package postgres

import (
	"context"
	"errors"
	"wallet-service/internal/models"
	apperrors "wallet-service/internal/errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WalletRepo struct {
	db *pgxpool.Pool
}

func NewWalletRepo(db *pgxpool.Pool) *WalletRepo {
	return &WalletRepo{db: db}
}

func (r *WalletRepo) Create(ctx context.Context, w *models.Wallet) (*models.Wallet, error) {
	err := r.db.QueryRow(ctx,
		`INSERT INTO wallets (customer_id, balance)
		 VALUES ($1, $2)
		 RETURNING wallet_id, customer_id, balance, version, created_at`,
		w.CustomerID, w.Balance,
	).Scan(&w.WalletID, &w.CustomerID, &w.Balance, &w.Version, &w.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, apperrors.ErrDuplicateWallet
		}
		return nil, err
	}
	return w, nil
}

func (r *WalletRepo) FindByID(ctx context.Context, walletID string) (*models.Wallet, error) {
	w := &models.Wallet{}
	err := r.db.QueryRow(ctx,
		`SELECT wallet_id, customer_id, balance, version, created_at
		 FROM wallets WHERE wallet_id = $1`,
		walletID,
	).Scan(&w.WalletID, &w.CustomerID, &w.Balance, &w.Version, &w.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.ErrWalletNotFound
	}
	return w, err
}

func (r *WalletRepo) CreditBalance(ctx context.Context, tx pgx.Tx, walletID string, amount float64) (float64, error) {
	var newBalance float64
	err := tx.QueryRow(ctx,
		`UPDATE wallets
		 SET balance = balance + $1, version = version + 1
		 WHERE wallet_id = $2
		 RETURNING balance`,
		amount, walletID,
	).Scan(&newBalance)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, apperrors.ErrWalletNotFound
	}
	return newBalance, err
}

// DebitBalance atomically decreases balance only when sufficient funds exist.
func (r *WalletRepo) DebitBalance(ctx context.Context, tx pgx.Tx, walletID string, amount float64) (float64, error) {
	var newBalance float64
	err := tx.QueryRow(ctx,
		`UPDATE wallets
		 SET balance = balance - $1, version = version + 1
		 WHERE wallet_id = $2 AND balance >= $1
		 RETURNING balance`,
		amount, walletID,
	).Scan(&newBalance)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		_ = r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wallets WHERE wallet_id=$1)`, walletID).Scan(&exists)
		if !exists {
			return 0, apperrors.ErrWalletNotFound
		}
		return 0, apperrors.ErrInsufficientBalance
	}
	return newBalance, err
}
