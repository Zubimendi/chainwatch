package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for chainwatch.
type Config struct {
	// Ethereum node WebSocket endpoint
	// Free options: Ankr (https://rpc.ankr.com/eth/ws), local Hardhat
	NodeWSURL string

	// Redis connection
	RedisURL string // e.g. redis://localhost:6379

	// Postgres connection
	DatabaseURL string // e.g. postgres://user:pass@localhost:5432/chainwatch

	// GraphQL server
	GraphQLPort int // default 8080

	// Webhook
	WebhookWorkers int           // number of parallel webhook senders
	WebhookTimeout time.Duration // per-request timeout

	// Detector tuning
	LargeTransferThresholdETH float64 // alert if transfer > X ETH
	RapidTxWindowSeconds      int     // window for rapid tx detection
	RapidTxThreshold          int     // number of txs in window to trigger alert
	GasSpikeMultiplier        float64 // alert if gas > X * baseline

	// Redis stream name
	AlertStreamName string
}

// Load reads config from environment variables.
// All variables are prefixed with CHAINWATCH_.
func Load() (*Config, error) {
	cfg := &Config{
		NodeWSURL:                 getEnv("CHAINWATCH_NODE_WS_URL", "ws://localhost:8545"),
		RedisURL:                  getEnv("CHAINWATCH_REDIS_URL", "redis://localhost:6379"),
		DatabaseURL:               getEnv("CHAINWATCH_DATABASE_URL", "postgres://chainwatch:chainwatch@localhost:5432/chainwatch?sslmode=disable"),
		GraphQLPort:               getEnvInt("CHAINWATCH_GRAPHQL_PORT", 8080),
		WebhookWorkers:            getEnvInt("CHAINWATCH_WEBHOOK_WORKERS", 5),
		WebhookTimeout:            time.Duration(getEnvInt("CHAINWATCH_WEBHOOK_TIMEOUT_SECONDS", 10)) * time.Second,
		LargeTransferThresholdETH: getEnvFloat("CHAINWATCH_LARGE_TRANSFER_ETH", 10.0),
		RapidTxWindowSeconds:      getEnvInt("CHAINWATCH_RAPID_TX_WINDOW_SECONDS", 60),
		RapidTxThreshold:          getEnvInt("CHAINWATCH_RAPID_TX_THRESHOLD", 5),
		GasSpikeMultiplier:        getEnvFloat("CHAINWATCH_GAS_SPIKE_MULTIPLIER", 3.0),
		AlertStreamName:           getEnv("CHAINWATCH_ALERT_STREAM", "chainwatch:alerts"),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.NodeWSURL == "" {
		return fmt.Errorf("CHAINWATCH_NODE_WS_URL is required")
	}
	if !strings.HasPrefix(c.NodeWSURL, "ws://") && !strings.HasPrefix(c.NodeWSURL, "wss://") {
		return fmt.Errorf("CHAINWATCH_NODE_WS_URL must start with ws:// or wss://")
	}
	if c.DatabaseURL == "" {
		return fmt.Errorf("CHAINWATCH_DATABASE_URL is required")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}