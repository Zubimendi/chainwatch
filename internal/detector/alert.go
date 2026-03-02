// internal/detector/alert.go
package detector

import "time"

// AlertType categorizes the kind of anomaly detected.
type AlertType string

const (
	AlertLargeTransfer    AlertType = "LARGE_TRANSFER"
	AlertRapidTransactions AlertType = "RAPID_TRANSACTIONS"
	AlertGasSpike         AlertType = "GAS_SPIKE"
	AlertFlashLoan        AlertType = "FLASH_LOAN"
	AlertContractDeploy   AlertType = "CONTRACT_DEPLOY"
	AlertUnknownCaller    AlertType = "UNKNOWN_CALLER"
)

// Severity levels for alerts.
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
	SeverityInfo     Severity = "INFO"
)

// Alert is produced by the detector when an anomaly rule fires.
// It is the main event that flows through the rest of the system.
type Alert struct {
	ID              string    `json:"id"`
	Type            AlertType `json:"type"`
	Severity        Severity  `json:"severity"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	TransactionHash string    `json:"transaction_hash"`
	FromAddress     string    `json:"from_address"`
	ToAddress       string    `json:"to_address"`
	ValueETH        float64   `json:"value_eth"`
	GasPriceGwei    float64   `json:"gas_price_gwei"`
	BlockNumber     uint64    `json:"block_number"`
	TriggeredRule   string    `json:"triggered_rule"`
	RuleMetadata    map[string]interface{} `json:"rule_metadata"`
	DetectedAt      time.Time `json:"detected_at"`
}