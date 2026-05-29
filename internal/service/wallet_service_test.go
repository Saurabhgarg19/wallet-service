package service_test

import (
	"context"
	"errors"
	"testing"
	"wallet-service/internal/domain"
	apperrors "wallet-service/internal/errors"
	"wallet-service/internal/events"
	"wallet-service/internal/metrics"
	"wallet-service/internal/service"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mock: WalletRepository ---

type mockWalletRepo struct{ mock.Mock }

func (m *mockWalletRepo) Create(ctx context.Context, w *domain.Wallet) (*domain.Wallet, error) {
	args := m.Called(ctx, w)
	return args.Get(0).(*domain.Wallet), args.Error(1)
}
func (m *mockWalletRepo) FindByID(ctx context.Context, id string) (*domain.Wallet, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Wallet), args.Error(1)
}
func (m *mockWalletRepo) CreditBalance(ctx context.Context, tx pgx.Tx, id string, amount float64) (float64, error) {
	args := m.Called(ctx, tx, id, amount)
	return args.Get(0).(float64), args.Error(1)
}
func (m *mockWalletRepo) DebitBalance(ctx context.Context, tx pgx.Tx, id string, amount float64) (float64, error) {
	args := m.Called(ctx, tx, id, amount)
	return args.Get(0).(float64), args.Error(1)
}

// --- Mock: TransactionRepository ---

type mockTxnRepo struct{ mock.Mock }

func (m *mockTxnRepo) Append(ctx context.Context, tx pgx.Tx, t *domain.WalletTransaction) (*domain.WalletTransaction, error) {
	args := m.Called(ctx, tx, t)
	return args.Get(0).(*domain.WalletTransaction), args.Error(1)
}
func (m *mockTxnRepo) FindByWalletID(ctx context.Context, id string) ([]*domain.WalletTransaction, error) {
	args := m.Called(ctx, id)
	return args.Get(0).([]*domain.WalletTransaction), args.Error(1)
}

// --- Mock: IdempotencyRepository ---

type mockIdemRepo struct{ mock.Mock }

func (m *mockIdemRepo) Find(ctx context.Context, tx pgx.Tx, walletID, key string) (*domain.IdempotencyRecord, error) {
	args := m.Called(ctx, tx, walletID, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.IdempotencyRecord), args.Error(1)
}
func (m *mockIdemRepo) Save(ctx context.Context, tx pgx.Tx, rec *domain.IdempotencyRecord) error {
	return m.Called(ctx, tx, rec).Error(0)
}

// --- Helpers ---

func newTestService(wRepo *mockWalletRepo, tRepo *mockTxnRepo, iRepo *mockIdemRepo) *service.WalletService {
	return service.NewWalletService(nil, wRepo, tRepo, iRepo, metrics.NoOpMetricsPort{}, events.NoOpEventPublisher{})
}

// --- Tests ---

func TestCreateWallet_Success(t *testing.T) {
	wRepo := &mockWalletRepo{}
	svc := newTestService(wRepo, nil, nil)

	wRepo.On("Create", mock.Anything, mock.MatchedBy(func(w *domain.Wallet) bool {
		return w.CustomerID == "cust-1" && w.Balance == 500
	})).Return(&domain.Wallet{WalletID: "wal-1", CustomerID: "cust-1", Balance: 500}, nil)

	w, err := svc.CreateWallet(context.Background(), "cust-1", 500)
	assert.NoError(t, err)
	assert.Equal(t, "wal-1", w.WalletID)
	wRepo.AssertExpectations(t)
}

func TestCreateWallet_NegativeBalance(t *testing.T) {
	svc := newTestService(&mockWalletRepo{}, nil, nil)
	_, err := svc.CreateWallet(context.Background(), "cust-1", -10)
	assert.True(t, errors.Is(err, apperrors.ErrInvalidRequest))
}

func TestCreateWallet_DuplicateWallet(t *testing.T) {
	wRepo := &mockWalletRepo{}
	svc := newTestService(wRepo, nil, nil)
	wRepo.On("Create", mock.Anything, mock.Anything).Return(&domain.Wallet{}, apperrors.ErrDuplicateWallet)
	_, err := svc.CreateWallet(context.Background(), "cust-1", 0)
	assert.True(t, errors.Is(err, apperrors.ErrDuplicateWallet))
}

func TestGetWallet_NotFound(t *testing.T) {
	wRepo := &mockWalletRepo{}
	svc := newTestService(wRepo, nil, nil)
	wRepo.On("FindByID", mock.Anything, "wal-x").Return(nil, apperrors.ErrWalletNotFound)
	_, err := svc.GetWallet(context.Background(), "wal-x")
	assert.True(t, errors.Is(err, apperrors.ErrWalletNotFound))
}

func TestGetTransactions_Success(t *testing.T) {
	wRepo := &mockWalletRepo{}
	tRepo := &mockTxnRepo{}
	svc := newTestService(wRepo, tRepo, nil)

	wRepo.On("FindByID", mock.Anything, "wal-1").Return(&domain.Wallet{WalletID: "wal-1"}, nil)
	tRepo.On("FindByWalletID", mock.Anything, "wal-1").Return([]*domain.WalletTransaction{
		{TransactionID: "txn-1", Type: domain.MovementTopUp, Amount: 200},
	}, nil)

	txns, err := svc.GetTransactions(context.Background(), "wal-1")
	assert.NoError(t, err)
	assert.Len(t, txns, 1)
}

