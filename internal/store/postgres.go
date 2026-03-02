package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yourusername/chainwatch/internal/detector"
)

// Store wraps the Postgres connection pool and exposes
// typed query methods. All SQL lives in this package.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a Store connected to the given DSN.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Close shuts down the connection pool.
func (s *Store) Close() {
	s.pool.Close()
}

// SaveAlert persists an alert to the database.
func (s *Store) SaveAlert(ctx context.Context, a detector.Alert) error {
	metadata, err := json.Marshal(a.RuleMetadata)
	if err != nil {
		metadata = []byte("{}")
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO alerts (
			id, type, severity, title, description,
			transaction_hash, from_address, to_address,
			value_eth, gas_price_gwei, block_number,
			triggered_rule, rule_metadata, detected_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10, $11,
			$12, $13, $14
		) ON CONFLICT (id) DO NOTHING`,
		a.ID, string(a.Type), string(a.Severity), a.Title, a.Description,
		a.TransactionHash, a.FromAddress, a.ToAddress,
		a.ValueETH, a.GasPriceGwei, a.BlockNumber,
		a.TriggeredRule, metadata, a.DetectedAt,
	)
	return err
}

// ListAlerts fetches recent alerts with optional severity filter.
func (s *Store) ListAlerts(ctx context.Context, limit int, severity string) ([]AlertRow, error) {
	query := `
		SELECT id, type, severity, title, description,
			   transaction_hash, from_address, to_address,
			   value_eth, gas_price_gwei, triggered_rule, detected_at
		FROM alerts
	`
	args := []interface{}{}

	if severity != "" {
		query += " WHERE severity = $1"
		args = append(args, severity)
	}

	query += " ORDER BY detected_at DESC LIMIT $" + fmt.Sprintf("%d", len(args)+1)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying alerts: %w", err)
	}
	defer rows.Close()

	var alerts []AlertRow
	for rows.Next() {
		var a AlertRow
		if err := rows.Scan(
			&a.ID, &a.Type, &a.Severity, &a.Title, &a.Description,
			&a.TransactionHash, &a.FromAddress, &a.ToAddress,
			&a.ValueETH, &a.GasPriceGwei, &a.TriggeredRule, &a.DetectedAt,
		); err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

// ListWebhooks returns all active webhooks.
func (s *Store) ListWebhooks(ctx context.Context) ([]WebhookRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, url, secret, min_severity
		FROM webhooks WHERE active = true
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hooks []WebhookRow
	for rows.Next() {
		var h WebhookRow
		if err := rows.Scan(&h.ID, &h.URL, &h.Secret, &h.MinSeverity); err != nil {
			return nil, err
		}
		hooks = append(hooks, h)
	}
	return hooks, rows.Err()
}

// AddWatch inserts a watched address.
func (s *Store) AddWatch(ctx context.Context, address, label string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO watched_addresses (address, label)
		 VALUES ($1, $2) ON CONFLICT (address) DO UPDATE SET active = true, label = $2`,
		address, label,
	)
	return err
}

// ListWatches returns all active watched addresses.
func (s *Store) ListWatches(ctx context.Context) ([]WatchRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT address, label, added_at FROM watched_addresses WHERE active = true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var watches []WatchRow
	for rows.Next() {
		var w WatchRow
		if err := rows.Scan(&w.Address, &w.Label, &w.AddedAt); err != nil {
			return nil, err
		}
		watches = append(watches, w)
	}
	return watches, rows.Err()
}

// RunMigrations executes all migration SQL files in order.
// In production you'd use golang-migrate; this is fine for a portfolio project.
func (s *Store) RunMigrations(ctx context.Context, migrations []string) error {
	for _, sql := range migrations {
		if _, err := s.pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, sql[:min(len(sql), 100)])
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}