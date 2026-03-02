package rules

import (
	"fmt"
	"sync"
	"time"

	"github.com/Zubimendi/chainwatch/internal/blockchain"
	"github.com/Zubimendi/chainwatch/internal/detector"
)

// RapidTransactionsRule fires when the same address sends too many
// transactions within a sliding time window. Classic bot/exploit pattern.
type RapidTransactionsRule struct {
	WindowDuration time.Duration
	Threshold      int

	mu      sync.Mutex
	// address → list of tx timestamps within the current window
	txTimes map[string][]time.Time
}

func NewRapidTransactionsRule(window time.Duration, threshold int) *RapidTransactionsRule {
	return &RapidTransactionsRule{
		WindowDuration: window,
		Threshold:      threshold,
		txTimes:        make(map[string][]time.Time),
	}
}

func (r *RapidTransactionsRule) Name() string { return "rapid-transactions" }

func (r *RapidTransactionsRule) Evaluate(tx *blockchain.DecodedTransaction) (*detector.Alert, bool) {
	if tx.From == "" {
		return nil, false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.WindowDuration)

	// Append current tx time
	r.txTimes[tx.From] = append(r.txTimes[tx.From], now)

	// Evict entries outside the window
	times := r.txTimes[tx.From]
	windowStart := 0
	for windowStart < len(times) && times[windowStart].Before(cutoff) {
		windowStart++
	}
	r.txTimes[tx.From] = times[windowStart:]

	count := len(r.txTimes[tx.From])
	if count < r.Threshold {
		return nil, false
	}

	return &detector.Alert{
		Type:     detector.AlertRapidTransactions,
		Severity: detector.SeverityHigh,
		Title:    fmt.Sprintf("Rapid Transactions from %s", tx.From),
		Description: fmt.Sprintf(
			"Address %s sent %d transactions within %s (threshold: %d). Possible bot activity or exploit.",
			tx.From, count, r.WindowDuration, r.Threshold,
		),
		TransactionHash: tx.Hash,
		FromAddress:     tx.From,
		ToAddress:       tx.To,
		ValueETH:        tx.ValueETH,
		GasPriceGwei:    tx.GasPriceGwei,
		BlockNumber:     tx.BlockNumber,
		TriggeredRule:   r.Name(),
		RuleMetadata: map[string]interface{}{
			"tx_count":        count,
			"window_seconds":  r.WindowDuration.Seconds(),
			"threshold":       r.Threshold,
		},
	}, true
}