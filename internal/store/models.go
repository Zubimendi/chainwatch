package store

import "time"

type AlertRow struct {
	ID              string
	Type            string
	Severity        string
	Title           string
	Description     string
	TransactionHash string
	FromAddress     string
	ToAddress       string
	ValueETH        float64
	GasPriceGwei    float64
	TriggeredRule   string
	DetectedAt      time.Time
}

type WebhookRow struct {
	ID          int64
	URL         string
	Secret      string
	MinSeverity string
}

type WatchRow struct {
	Address string
	Label   string
	AddedAt time.Time
}