package blockchain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/gorilla/websocket"
)

const (
	maxReconnectDelay = 30 * time.Second
	baseReconnectDelay = 1 * time.Second
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 45 * time.Second
)

// Subscriber maintains a WebSocket connection to an Ethereum node and
// streams raw transactions and logs onto typed output channels.
type Subscriber struct {
	nodeURL string
	logger  *slog.Logger

	// Output channels — consumers read from these
	Transactions chan RawTransaction
	Logs         chan RawLog

	// addresses to watch for log subscriptions
	watchAddresses []string
}

// NewSubscriber creates a Subscriber. Call Start() to begin the connection.
func NewSubscriber(nodeURL string, watchAddresses []string, logger *slog.Logger) *Subscriber {
	return &Subscriber{
		nodeURL:        nodeURL,
		logger:         logger,
		Transactions:   make(chan RawTransaction, 1000), // buffered — don't block on slow consumers
		Logs:           make(chan RawLog, 1000),
		watchAddresses: watchAddresses,
	}
}

// Start connects to the node and begins streaming events. It blocks until ctx
// is cancelled. Reconnects automatically on connection loss with exponential backoff.
func (s *Subscriber) Start(ctx context.Context) error {
	delay := baseReconnectDelay

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("subscriber context cancelled — shutting down")
			return nil
		default:
		}

		s.logger.Info("connecting to Ethereum node", "url", s.nodeURL)
		err := s.connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // context cancelled — clean exit
			}
			// Add jitter to prevent thundering herd on reconnect
			jitter := time.Duration(rand.Int63n(int64(delay) / 2))
			sleepFor := delay + jitter
			s.logger.Warn("connection lost — reconnecting",
				"error", err,
				"retry_in", sleepFor,
			)

			select {
			case <-ctx.Done():
				return nil
			case <-time.After(sleepFor):
			}

			// Exponential backoff capped at maxReconnectDelay
			delay = min(delay*2, maxReconnectDelay)
			continue
		}

		// Successful connection — reset backoff
		delay = baseReconnectDelay
	}
}

// connect establishes one WebSocket session and streams messages until the
// connection drops or ctx is cancelled. Returns the error that caused disconnect.
func (s *Subscriber) connect(ctx context.Context) error {
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, s.nodeURL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	s.logger.Info("connected to Ethereum node")

	// Subscribe to pending transactions
	if err := s.subscribe(conn, "newPendingTransactions"); err != nil {
		return fmt.Errorf("pending tx subscription: %w", err)
	}

	// Subscribe to logs for watched addresses
	if len(s.watchAddresses) > 0 {
		if err := s.subscribeToLogs(conn); err != nil {
			return fmt.Errorf("logs subscription: %w", err)
		}
	}

	// Start ping/pong to keep connection alive
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	go s.pingLoop(ctx, conn)

	// Read loop
	for {
		select {
		case <-ctx.Done():
			// Send a clean close frame
			conn.WriteMessage(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			)
			return nil
		default:
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		if err := s.handleMessage(msg); err != nil {
			s.logger.Warn("failed to handle message", "error", err)
			// Non-fatal — keep reading
		}
	}
}

func (s *Subscriber) subscribe(conn *websocket.Conn, method string) error {
	req := SubscriptionRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "eth_subscribe",
		Params:  []interface{}{method},
	}
	return conn.WriteJSON(req)
}

func (s *Subscriber) subscribeToLogs(conn *websocket.Conn) error {
	// Build address list for log filter
	addresses := make([]string, len(s.watchAddresses))
	copy(addresses, s.watchAddresses)

	req := SubscriptionRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "eth_subscribe",
		Params: []interface{}{
			"logs",
			map[string]interface{}{
				"address": addresses,
			},
		},
	}
	return conn.WriteJSON(req)
}

func (s *Subscriber) pingLoop(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *Subscriber) handleMessage(raw []byte) error {
	// First decode the envelope to determine what type of message this is
	var envelope WSMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}

	// Subscription confirmation messages have no params
	if envelope.Method == "" {
		return nil
	}

	if envelope.Method != "eth_subscription" {
		return nil
	}

	// Try to decode result as a transaction first
	resultBytes, err := json.Marshal(envelope.Params.Result)
	if err != nil {
		return fmt.Errorf("re-marshal result: %w", err)
	}

	// Transactions are objects with a "hash" field
	// Pending tx subscriptions return just the hash string
	var hashOnly string
	if err := json.Unmarshal(resultBytes, &hashOnly); err == nil && len(hashOnly) == 66 {
		// Just a hash — this happens with newPendingTransactions
		// We'd need to fetch full tx details via eth_getTransactionByHash
		// For now, emit a minimal RawTransaction with just the hash
		select {
		case s.Transactions <- RawTransaction{Hash: hashOnly}:
		default:
			s.logger.Warn("transaction channel full — dropping", "hash", hashOnly)
		}
		return nil
	}

	// Try as a full transaction object
	var tx RawTransaction
	if err := json.Unmarshal(resultBytes, &tx); err == nil && tx.Hash != "" {
		select {
		case s.Transactions <- tx:
		default:
			s.logger.Warn("transaction channel full — dropping", "hash", tx.Hash)
		}
		return nil
	}

	// Try as a log
	var log RawLog
	if err := json.Unmarshal(resultBytes, &log); err == nil && log.TransactionHash != "" {
		select {
		case s.Logs <- log:
		default:
			s.logger.Warn("log channel full — dropping", "tx", log.TransactionHash)
		}
		return nil
	}

	return nil
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}