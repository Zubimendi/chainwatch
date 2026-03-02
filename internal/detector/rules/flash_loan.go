package rules

import (
	"fmt"

	"github.com/Zubimendi/chainwatch/internal/blockchain"
	"github.com/Zubimendi/chainwatch/internal/detector"
)

// FlashLoanRule fires when a decoded log matches a known flash loan event signature.
// Flash loans themselves are not malicious — but they are the first step in
// 90%+ of DeFi price manipulation attacks.
type FlashLoanRule struct{}

func (r *FlashLoanRule) Name() string { return "flash-loan-detected" }

// EvaluateLog checks a decoded log (not a transaction) for flash loan events.
func (r *FlashLoanRule) EvaluateLog(log *blockchain.DecodedLog) (*detector.Alert, bool) {
	if log.EventName != "FlashLoan" {
		return nil, false
	}

	return &detector.Alert{
		Type:     detector.AlertFlashLoan,
		Severity: detector.SeverityHigh,
		Title:    fmt.Sprintf("Flash Loan Detected at %s", log.Address),
		Description: fmt.Sprintf(
			"A flash loan event was emitted by contract %s in transaction %s. "+
				"Flash loans are commonly the first step in price manipulation attacks. "+
				"Monitor subsequent transactions from the same address for suspicious activity.",
			log.Address, log.TransactionHash,
		),
		TransactionHash: log.TransactionHash,
		FromAddress:     log.Address,
		BlockNumber:     log.BlockNumber,
		TriggeredRule:   r.Name(),
		RuleMetadata: map[string]interface{}{
			"contract":    log.Address,
			"event":       log.EventName,
			"log_args":    log.Args,
		},
	}, true
}