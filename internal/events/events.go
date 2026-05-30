package events

type EventPublisher interface {
	PublishWalletCreated(walletID, customerID string)
	PublishWalletToppedUp(walletID string, amount float64)
	PublishWalletDeducted(walletID string, amount float64, txnID string)
	PublishWalletDeductionRejected(walletID, reason string)
}

type NoOpEventPublisher struct{}

func (NoOpEventPublisher) PublishWalletCreated(walletID, customerID string)              {}
func (NoOpEventPublisher) PublishWalletToppedUp(walletID string, amount float64)         {}
func (NoOpEventPublisher) PublishWalletDeducted(walletID string, amount float64, txnID string) {}
func (NoOpEventPublisher) PublishWalletDeductionRejected(walletID, reason string)        {}

