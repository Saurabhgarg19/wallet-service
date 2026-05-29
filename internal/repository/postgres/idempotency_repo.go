package postgres

import (
	"context"
	"errors"
	"wallet-service/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IdempotencyRepo struct {
	db *pgxpool.Pool
}

func NewIdempotencyRepo(db *pgxpool.Pool) *IdempotencyRepo {
	return &IdempotencyRepo{db: db}
}

func (r *IdempotencyRepo) Find(ctx context.Context, tx pgx.Tx, walletID, key string) (*domain.IdempotencyRecord, error) {
	rec := &domain.IdempotencyRecord{}
	err := tx.QueryRow(ctx,
		`SELECT wallet_id, idempotency_key, requested_amount, outcome,
		        COALESCE(transaction_id::text, ''), COALESCE(balance_after, 0), created_at
		 FROM deduction_idempotency
		 WHERE wallet_id = $1 AND idempotency_key = $2`,
		walletID, key,
	).Scan(&rec.WalletID, &rec.IdempotencyKey, &rec.RequestedAmount,
		&rec.Outcome, &rec.TransactionID, &rec.BalanceAfter, &rec.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return rec, err
}

func (r *IdempotencyRepo) Save(ctx context.Context, tx pgx.Tx, rec *domain.IdempotencyRecord) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO deduction_idempotency
		    (wallet_id, idempotency_key, requested_amount, outcome, transaction_id, balance_after)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		rec.WalletID, rec.IdempotencyKey, rec.RequestedAmount, rec.Outcome,
		nullableString(rec.TransactionID), rec.BalanceAfter,
	)
	return err
}

