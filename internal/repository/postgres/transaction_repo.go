package postgres

import (
	"context"
	"wallet-service/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TransactionRepo struct {
	db *pgxpool.Pool
}

func NewTransactionRepo(db *pgxpool.Pool) *TransactionRepo {
	return &TransactionRepo{db: db}
}

func (r *TransactionRepo) Append(ctx context.Context, tx pgx.Tx, t *domain.WalletTransaction) (*domain.WalletTransaction, error) {
	err := tx.QueryRow(ctx,
		`INSERT INTO wallet_transactions (wallet_id, type, amount, reference_id, idempotency_key)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING transaction_id, created_at`,
		t.WalletID, t.Type, t.Amount, nullableString(t.ReferenceID), nullableString(t.IdempotencyKey),
	).Scan(&t.TransactionID, &t.CreatedAt)
	return t, err
}

func (r *TransactionRepo) FindByWalletID(ctx context.Context, walletID string) ([]*domain.WalletTransaction, error) {
	rows, err := r.db.Query(ctx,
		`SELECT transaction_id, wallet_id, type, amount,
		        COALESCE(reference_id, ''), COALESCE(idempotency_key, ''), created_at
		 FROM wallet_transactions
		 WHERE wallet_id = $1
		 ORDER BY created_at ASC`,
		walletID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txns []*domain.WalletTransaction
	for rows.Next() {
		t := &domain.WalletTransaction{}
		if err := rows.Scan(&t.TransactionID, &t.WalletID, &t.Type, &t.Amount,
			&t.ReferenceID, &t.IdempotencyKey, &t.CreatedAt); err != nil {
			return nil, err
		}
		txns = append(txns, t)
	}
	return txns, rows.Err()
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

