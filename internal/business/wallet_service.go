package business

import (
	"context"
	"fmt"
	apperrors "wallet-service/internal/errors"
	"wallet-service/internal/events"
	"wallet-service/internal/metrics"
	"wallet-service/internal/models"
	"wallet-service/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
)

type WalletService struct {
	db      *pgxpool.Pool
	wallets repository.WalletRepository
	txns    repository.TransactionRepository
	idem    repository.IdempotencyRepository
	metrics metrics.MetricsPort
	events  events.EventPublisher
}

func NewWalletService(
	db *pgxpool.Pool,
	wallets repository.WalletRepository,
	txns repository.TransactionRepository,
	idem repository.IdempotencyRepository,
	m metrics.MetricsPort,
	e events.EventPublisher,
) *WalletService {
	return &WalletService{db: db, wallets: wallets, txns: txns, idem: idem, metrics: m, events: e}
}

func (s *WalletService) CreateWallet(ctx context.Context, customerID string, initialBalance float64) (*models.Wallet, error) {
	if initialBalance < 100 {
		return nil, fmt.Errorf("%w: initialBalance must be at least ₹100", apperrors.ErrInvalidRequest)
	}
	w, err := s.wallets.Create(ctx, &models.Wallet{CustomerID: customerID, Balance: initialBalance})
	if err != nil {
		return nil, err
	}
	s.metrics.RecordCreateWallet()
	s.events.PublishWalletCreated(w.WalletID, w.CustomerID)
	return w, nil
}

func (s *WalletService) GetWallet(ctx context.Context, walletID string) (*models.Wallet, error) {
	return s.wallets.FindByID(ctx, walletID)
}

type TopUpResult struct {
	WalletID      string
	Balance       float64
	TransactionID string
}

func (s *WalletService) TopUp(ctx context.Context, walletID string, amount float64, referenceID string) (*TopUpResult, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("%w: amount must be positive", apperrors.ErrInvalidRequest)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	newBalance, err := s.wallets.CreditBalance(ctx, tx, walletID, amount)
	if err != nil {
		return nil, err
	}

	txn, err := s.txns.Append(ctx, tx, &models.WalletTransaction{
		WalletID:    walletID,
		Type:        models.MovementTopUp,
		Amount:      amount,
		ReferenceID: referenceID,
	})
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	s.metrics.RecordTopupSuccess()
	s.events.PublishWalletToppedUp(walletID, amount)

	return &TopUpResult{WalletID: walletID, Balance: newBalance, TransactionID: txn.TransactionID}, nil
}

type DeductResult struct {
	WalletID                   string
	Balance                    float64
	TransactionID              string
	DeductedAmount             float64
	ServedFromIdempotencyCache bool
}

func (s *WalletService) Deduct(ctx context.Context, walletID, idempotencyKey string, amount float64, referenceID string) (*DeductResult, error) {
	if idempotencyKey == "" {
		return nil, fmt.Errorf("%w: idempotencyKey is required", apperrors.ErrInvalidRequest)
	}
	if amount <= 0 {
		return nil, fmt.Errorf("%w: amount must be positive", apperrors.ErrInvalidRequest)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	existing, err := s.idem.Find(ctx, tx, walletID, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.RequestedAmount != amount {
			return nil, apperrors.ErrIdempotencyConflict
		}
		tx.Rollback(ctx)
		s.metrics.RecordIdempotentReplay()
		return &DeductResult{
			WalletID:                   walletID,
			Balance:                    existing.BalanceAfter,
			TransactionID:              existing.TransactionID,
			DeductedAmount:             amount,
			ServedFromIdempotencyCache: true,
		}, nil
	}

	newBalance, err := s.wallets.DebitBalance(ctx, tx, walletID, amount)
	if err != nil {
		if err == apperrors.ErrInsufficientBalance {
			_ = s.idem.Save(ctx, tx, &models.IdempotencyRecord{
				WalletID:        walletID,
				IdempotencyKey:  idempotencyKey,
				RequestedAmount: amount,
				Outcome:         models.OutcomeInsufficientBalance,
			})
			_ = tx.Commit(ctx)
			s.metrics.RecordDeductRejected()
			s.events.PublishWalletDeductionRejected(walletID, "INSUFFICIENT_BALANCE")
		}
		return nil, err
	}

	txn, err := s.txns.Append(ctx, tx, &models.WalletTransaction{
		WalletID:       walletID,
		Type:           models.MovementDeduct,
		Amount:         amount,
		ReferenceID:    referenceID,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return nil, err
	}

	if err := s.idem.Save(ctx, tx, &models.IdempotencyRecord{
		WalletID:        walletID,
		IdempotencyKey:  idempotencyKey,
		RequestedAmount: amount,
		Outcome:         models.OutcomeSuccess,
		TransactionID:   txn.TransactionID,
		BalanceAfter:    newBalance,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	s.metrics.RecordDeductSuccess()
	s.events.PublishWalletDeducted(walletID, amount, txn.TransactionID)

	return &DeductResult{
		WalletID:       walletID,
		Balance:        newBalance,
		TransactionID:  txn.TransactionID,
		DeductedAmount: amount,
	}, nil
}

func (s *WalletService) GetBalance(ctx context.Context, walletID string) (*models.Wallet, error) {
	return s.wallets.FindByID(ctx, walletID)
}

func (s *WalletService) GetTransactions(ctx context.Context, walletID string) ([]*models.WalletTransaction, error) {
	if _, err := s.wallets.FindByID(ctx, walletID); err != nil {
		return nil, err
	}
	return s.txns.FindByWalletID(ctx, walletID)
}

