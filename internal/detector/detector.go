package detector

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/Zubimendi/chainwatch/internal/blockchain"
	"github.com/Zubimendi/chainwatch/internal/config"
)

// Rule is implemented by every anomaly detection rule for transactions.
type Rule interface {
	Name() string
	Evaluate(tx *blockchain.DecodedTransaction) (*Alert, bool)
}

// LogRule is implemented by rules that evaluate event logs rather than transactions.
type LogRule interface {
	Name() string
	EvaluateLog(log *blockchain.DecodedLog) (*Alert, bool)
}

// Detector runs all registered rules against every decoded event and
// emits alerts onto the Alerts channel.
type Detector struct {
	txRules  []Rule
	logRules []LogRule
	logger   *slog.Logger
	Alerts   chan Alert
}

// New creates a Detector with rules configured from cfg.
func New(cfg *config.Config, logger *slog.Logger) *Detector {
	return &Detector{
		logger: logger,
		Alerts: make(chan Alert, 500),
	}
}

// RegisterTxRule adds a transaction anomaly rule.
func (d *Detector) RegisterTxRule(r Rule) {
	d.txRules = append(d.txRules, r)
}

// RegisterLogRule adds an event log anomaly rule.
func (d *Detector) RegisterLogRule(r LogRule) {
	d.logRules = append(d.logRules, r)
}

// RunTransactions reads from txCh, evaluates all tx rules,
// and emits any fired alerts to d.Alerts.
// Blocks until ctx is cancelled.
func (d *Detector) RunTransactions(ctx context.Context, txCh <-chan *blockchain.DecodedTransaction) {
	for {
		select {
		case <-ctx.Done():
			return
		case tx, ok := <-txCh:
			if !ok {
				return
			}
			d.evaluateTx(tx)
		}
	}
}

// RunLogs reads from logCh and evaluates all log rules.
func (d *Detector) RunLogs(ctx context.Context, logCh <-chan *blockchain.DecodedLog) {
	for {
		select {
		case <-ctx.Done():
			return
		case log, ok := <-logCh:
			if !ok {
				return
			}
			d.evaluateLog(log)
		}
	}
}

func (d *Detector) evaluateTx(tx *blockchain.DecodedTransaction) {
	for _, rule := range d.txRules {
		alert, fired := rule.Evaluate(tx)
		if !fired {
			continue
		}

		alert.ID = generateID()
		alert.DetectedAt = time.Now().UTC()

		d.logger.Info("alert fired",
			"rule", rule.Name(),
			"severity", alert.Severity,
			"tx", tx.Hash,
		)

		select {
		case d.Alerts <- *alert:
		default:
			d.logger.Warn("alert channel full — dropping alert", "rule", rule.Name())
		}
	}
}

func (d *Detector) evaluateLog(log *blockchain.DecodedLog) {
	for _, rule := range d.logRules {
		alert, fired := rule.EvaluateLog(log)
		if !fired {
			continue
		}

		alert.ID = generateID()
		alert.DetectedAt = time.Now().UTC()

		d.logger.Info("log alert fired",
			"rule", rule.Name(),
			"severity", alert.Severity,
			"tx", log.TransactionHash,
		)

		select {
		case d.Alerts <- *alert:
		default:
			d.logger.Warn("alert channel full — dropping log alert", "rule", rule.Name())
		}
	}
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("alert_%s_%d", hex.EncodeToString(b), time.Now().UnixNano())
}