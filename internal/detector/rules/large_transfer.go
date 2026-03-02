package rules

import (
	"fmt"

	"github.com/Zubimendi/chainwatch/internal/blockchain"
	"github.com/Zubimendi/chainwatch/internal/detector"
)

// LargeTransferRule fires when an ETH transfer exceeds a threshold.
type LargeTransferRule struct {
	ThresholdETH float64
}

func (r *LargeTransferRule) Name() string { return "large-transfer" }

func (r *LargeTransferRule) Evaluate(tx *blockchain.DecodedTransaction) (*detector.Alert, bool) {
	if tx.ValueETH < r.ThresholdETH {
		return nil, false
	}

	severity := detector.SeverityMedium
	if tx.ValueETH >= r.ThresholdETH*10 {
		severity = detector.SeverityCritical
	} else if tx.ValueETH >= r.ThresholdETH*3 {
		severity = detector.SeverityHigh
	}

	return &detector.Alert{
		Type:            detector.AlertLargeTransfer,
		Severity:        severity,
		Title:           fmt.Sprintf("Large Transfer: %.4f ETH", tx.ValueETH),
		Description:     fmt.Sprintf("Transfer of %.4f ETH from %s to %s exceeds threshold of %.2f ETH", tx.ValueETH, tx.From, tx.To, r.ThresholdETH),
		TransactionHash: tx.Hash,
		FromAddress:     tx.From,
		ToAddress:       tx.To,
		ValueETH:        tx.ValueETH,
		GasPriceGwei:    tx.GasPriceGwei,
		BlockNumber:     tx.BlockNumber,
		TriggeredRule:   r.Name(),
		RuleMetadata: map[string]interface{}{
			"threshold_eth": r.ThresholdETH,
			"actual_eth":    tx.ValueETH,
		},
	}, true
}