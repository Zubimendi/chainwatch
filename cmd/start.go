package cmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/Zubimendi/chainwatch/internal/blockchain"
	"github.com/Zubimendi/chainwatch/internal/config"
	"github.com/Zubimendi/chainwatch/internal/detector"
	"github.com/Zubimendi/chainwatch/internal/detector/rules"
	"github.com/Zubimendi/chainwatch/internal/dispatcher"
	"github.com/Zubimendi/chainwatch/internal/store"

	"github.com/redis/go-redis/v9"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the chainwatch monitoring daemon",
	Long: `Start monitoring the blockchain for anomalous transactions.

Connects to the configured Ethereum node via WebSocket,
runs all detection rules, and dispatches alerts to Redis, Postgres, and webhooks.

Examples:
  chainwatch start
  CHAINWATCH_NODE_WS_URL=wss://rpc.ankr.com/eth/ws chainwatch start`,
	RunE: runStart,
}

func init() { rootCmd.AddCommand(startCmd) }

func runStart(cmd *cobra.Command, args []string) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// ─── Load config ──────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		logger.Error("config error", "error", err)
		return err
	}
	logger.Info("configuration loaded",
		"node", cfg.NodeWSURL,
		"large_transfer_threshold_eth", cfg.LargeTransferThresholdETH,
	)

	// ─── Context with graceful shutdown ───────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for Ctrl+C / SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutdown signal received — stopping gracefully")
		cancel()
	}()

	// ─── Database ─────────────────────────────────────────────────
	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	// Run migrations
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	if err := st.RunMigrations(ctx, migrations); err != nil {
		return err
	}
	logger.Info("database migrations applied")

	// ─── Redis ────────────────────────────────────────────────────
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return err
	}
	redisClient := redis.NewClient(redisOpts)
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return err
	}
	logger.Info("redis connected")
	redisProducer := dispatcher.NewRedisProducer(redisClient, cfg.AlertStreamName)

	// ─── Load watched addresses ───────────────────────────────────
	watches, err := st.ListWatches(ctx)
	if err != nil {
		return err
	}
	addresses := make([]string, len(watches))
	for i, w := range watches {
		addresses[i] = w.Address
	}
	logger.Info("watching addresses", "count", len(addresses))

	// ─── Subscriber ───────────────────────────────────────────────
	sub := blockchain.NewSubscriber(cfg.NodeWSURL, addresses, logger)

	// ─── Decoder ──────────────────────────────────────────────────
	erc20ABI, err := blockchain.LoadERC20ABI()
	if err != nil {
		return err
	}
	dec := blockchain.NewDecoder(erc20ABI)

	// Channels: subscriber → decoder → detector
	decodedTxCh := make(chan *blockchain.DecodedTransaction, 1000)
	decodedLogCh := make(chan *blockchain.DecodedLog, 1000)

	// ─── Detector with rules ──────────────────────────────────────
	det := detector.New(cfg, logger)
	det.RegisterTxRule(&rules.LargeTransferRule{
		ThresholdETH: cfg.LargeTransferThresholdETH,
	})
	det.RegisterTxRule(rules.NewRapidTransactionsRule(
		time.Duration(cfg.RapidTxWindowSeconds)*time.Second,
		cfg.RapidTxThreshold,
	))
	det.RegisterTxRule(&rules.GasSpikeRule{
		Multiplier: cfg.GasSpikeMultiplier,
	})
	det.RegisterLogRule(&rules.FlashLoanRule{})

	// ─── Dispatcher ───────────────────────────────────────────────
	webhookSender := dispatcher.NewWebhookSender(st, cfg.WebhookTimeout)
	disp := dispatcher.New(st, redisProducer, webhookSender, logger)

	// ─── Wire everything together with errgroup ───────────────────
	// errgroup cancels the context if any goroutine returns an error
	g, gctx := errgroup.WithContext(ctx)

	// 1. WebSocket subscriber
	g.Go(func() error {
		return sub.Start(gctx)
	})

	// 2. Decode transactions from subscriber
	g.Go(func() error {
		defer close(decodedTxCh)
		for {
			select {
			case <-gctx.Done():
				return nil
			case raw, ok := <-sub.Transactions:
				if !ok {
					return nil
				}
				decoded, err := dec.DecodeTransaction(raw)
				if err != nil {
					logger.Warn("tx decode failed", "hash", raw.Hash, "error", err)
					continue
				}
				select {
				case decodedTxCh <- decoded:
				case <-gctx.Done():
					return nil
				}
			}
		}
	})

	// 3. Decode logs from subscriber
	g.Go(func() error {
		defer close(decodedLogCh)
		for {
			select {
			case <-gctx.Done():
				return nil
			case raw, ok := <-sub.Logs:
				if !ok {
					return nil
				}
				decoded, err := dec.DecodeLog(raw)
				if err != nil {
					continue
				}
				select {
				case decodedLogCh <- decoded:
				case <-gctx.Done():
					return nil
				}
			}
		}
	})

	// 4. Detect anomalies in transactions
	g.Go(func() error {
		det.RunTransactions(gctx, decodedTxCh)
		return nil
	})

	// 5. Detect anomalies in logs
	g.Go(func() error {
		det.RunLogs(gctx, decodedLogCh)
		return nil
	})

	// 6. Dispatch alerts
	g.Go(func() error {
		disp.Run(gctx, det.Alerts)
		return nil
	})

	logger.Info("chainwatch started — monitoring blockchain",
		"node", cfg.NodeWSURL,
		"rules", 4,
	)

	return g.Wait()
}

func loadMigrations() ([]string, error) {
	// In a real project, embed with //go:embed migrations/*.sql
	// For simplicity, return inline SQL or read from files
	return []string{
		migrationCreateEvents,
		migrationCreateAlerts,
		migrationCreateWatches,
		migrationCreateWebhooks,
	}, nil
}

// Migration SQL constants (copy from your migrations/ files)
const migrationCreateEvents = `CREATE TABLE IF NOT EXISTS decoded_transactions (
    id BIGSERIAL PRIMARY KEY, hash VARCHAR(66) NOT NULL UNIQUE,
    from_address VARCHAR(42) NOT NULL, to_address VARCHAR(42),
    value_wei NUMERIC(78,0) NOT NULL DEFAULT 0, value_eth NUMERIC(30,18) NOT NULL DEFAULT 0,
    gas_limit BIGINT, gas_price_gwei NUMERIC(20,9), nonce BIGINT, block_number BIGINT,
    method_name VARCHAR(128), is_deployment BOOLEAN NOT NULL DEFAULT FALSE,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE INDEX IF NOT EXISTS idx_tx_from ON decoded_transactions(from_address);
CREATE INDEX IF NOT EXISTS idx_tx_received ON decoded_transactions(received_at DESC);`

const migrationCreateAlerts = `CREATE TABLE IF NOT EXISTS alerts (
    id VARCHAR(64) PRIMARY KEY, type VARCHAR(64) NOT NULL, severity VARCHAR(16) NOT NULL,
    title TEXT NOT NULL, description TEXT NOT NULL, transaction_hash VARCHAR(66),
    from_address VARCHAR(42), to_address VARCHAR(42), value_eth NUMERIC(30,18),
    gas_price_gwei NUMERIC(20,9), block_number BIGINT, triggered_rule VARCHAR(128) NOT NULL,
    rule_metadata JSONB, detected_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE INDEX IF NOT EXISTS idx_alerts_severity ON alerts(severity);
CREATE INDEX IF NOT EXISTS idx_alerts_detected ON alerts(detected_at DESC);`

const migrationCreateWatches = `CREATE TABLE IF NOT EXISTS watched_addresses (
    id BIGSERIAL PRIMARY KEY, address VARCHAR(42) NOT NULL UNIQUE,
    label VARCHAR(128), notes TEXT, active BOOLEAN NOT NULL DEFAULT TRUE,
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW());`

const migrationCreateWebhooks = `CREATE TABLE IF NOT EXISTS webhooks (
    id BIGSERIAL PRIMARY KEY, url TEXT NOT NULL, secret VARCHAR(128),
    active BOOLEAN NOT NULL DEFAULT TRUE, min_severity VARCHAR(16) NOT NULL DEFAULT 'LOW',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());`