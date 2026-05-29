package metrics

import "time"

type MetricsPort interface {
	RecordCreateWallet()
	RecordTopupSuccess()
	RecordDeductSuccess()
	RecordDeductRejected()
	RecordIdempotentReplay()
	RecordLatency(operation string, duration time.Duration)
}

type NoOpMetricsPort struct{}

func (NoOpMetricsPort) RecordCreateWallet()                                    {}
func (NoOpMetricsPort) RecordTopupSuccess()                                    {}
func (NoOpMetricsPort) RecordDeductSuccess()                                   {}
func (NoOpMetricsPort) RecordDeductRejected()                                  {}
func (NoOpMetricsPort) RecordIdempotentReplay()                                {}
func (NoOpMetricsPort) RecordLatency(operation string, duration time.Duration) {}

