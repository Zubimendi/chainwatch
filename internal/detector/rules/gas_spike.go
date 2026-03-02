package rules

import (
	"fmt"
	"sync"

	"github.com/Zubimendi/chainwatch/internal/blockchain"
	"github.com/Zubimendi/chainwatch/internal/detector"
)

// GasSpikeRule fires when a transaction's gas price is significantly above
// the rolling average. Sudden gas spikes indicate urgency — a hallmark of
// front-running bots and exploit transactions trying to get in a block fast.
type GasSpikeRule struct {
	Multiplier float64 // alert if gasPrice > baseline * multiplier

	mu          sync.Mutex
	sampleCount int
	rollingSum  float64
}

func (r *GasSpikeRule) Name() string { return "gas-spike" }

func (r *GasSpikeRule) Evaluate(tx *blockchain.DecodedTransaction) (*detector.Alert, bool) {
	if tx.GasPriceGwei == 0 {
		return nil, false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Update rolling average (exponential moving average — no memory growth)
	if r.sampleCount < 100 {
		r.sampleCount++
		r.rollingSum += tx.GasPriceGwei
	} else {
		// EMA: weight new sample at 1% — smooths out noise
		r.rollingSum = r.rollingSum*0.99 + tx.GasPriceGwei*0.01
	}

	if r.sampleCount < 10 {
		// Not enough samples to establish a baseline yet
		return nil, false
	}

	baseline := r.rollingSum / float64(min64(r.sampleCount, 100))
	threshold := baseline * r.Multiplier

	if tx.GasPriceGwei <= threshold {
		return nil, false
	}

	return &detector.Alert{
		Type:     detector.AlertGasSpike,
		Severity: detector.SeverityMedium,
		Title:    fmt.Sprintf("Gas Spike: %.1f Gwei (%.1fx baseline)", tx.GasPriceGwei, tx.GasPriceGwei/baseline),
		Description: fmt.Sprintf(
			"Transaction %s used %.1f Gwei — %.1fx above the %.1f Gwei baseline. "+
				"High gas price may indicate front-running or exploit urgency.",
			tx.Hash, tx.GasPriceGwei, tx.GasPriceGwei/baseline, baseline,
		),
		TransactionHash: tx.Hash,
		FromAddress:     tx.From,
		ToAddress:       tx.To,
		ValueETH:        tx.ValueETH,
		GasPriceGwei:    tx.GasPriceGwei,
		BlockNumber:     tx.BlockNumber,
		TriggeredRule:   r.Name(),
		RuleMetadata: map[string]interface{}{
			"gas_gwei":        tx.GasPriceGwei,
			"baseline_gwei":   baseline,
			"multiplier":      r.Multiplier,
			"spike_factor":    tx.GasPriceGwei / baseline,
		},
	}, true
}

func min64(a, b int) int {
	if a < b {
		return a
	}
	return b
}