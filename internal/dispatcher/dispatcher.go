package dispatcher

import (
	"context"
	"log/slog"

	"github.com/Zubimendi/chainwatch/internal/detector"
	"github.com/Zubimendi/chainwatch/internal/store"
)

// Dispatcher reads from the alert channel and fans out to every consumer:
// Postgres (persistence), Redis Stream (pub/sub), and webhooks (HTTP).
type Dispatcher struct {
	store   *store.Store
	redis   *RedisProducer
	webhook *WebhookSender
	logger  *slog.Logger
}

func New(st *store.Store, redis *RedisProducer, webhook *WebhookSender, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		store:   st,
		redis:   redis,
		webhook: webhook,
		logger:  logger,
	}
}

// Run reads alerts from alertCh and dispatches to all consumers concurrently.
// Each consumer runs in its own goroutine so a slow webhook doesn't block Postgres writes.
func (d *Dispatcher) Run(ctx context.Context, alertCh <-chan detector.Alert) {
	for {
		select {
		case <-ctx.Done():
			return
		case alert, ok := <-alertCh:
			if !ok {
				return
			}
			// Fan out — each consumer is independent
			go d.persistAlert(ctx, alert)
			go d.publishToRedis(ctx, alert)
			go d.sendWebhooks(ctx, alert)
		}
	}
}

func (d *Dispatcher) persistAlert(ctx context.Context, alert detector.Alert) {
	if err := d.store.SaveAlert(ctx, alert); err != nil {
		d.logger.Error("failed to persist alert", "error", err, "alert_id", alert.ID)
	}
}

func (d *Dispatcher) publishToRedis(ctx context.Context, alert detector.Alert) {
	if d.redis == nil {
		return
	}
	if err := d.redis.Publish(ctx, alert); err != nil {
		d.logger.Error("failed to publish alert to Redis", "error", err, "alert_id", alert.ID)
	}
}

func (d *Dispatcher) sendWebhooks(ctx context.Context, alert detector.Alert) {
	if d.webhook == nil {
		return
	}
	if err := d.webhook.Send(ctx, alert); err != nil {
		d.logger.Error("webhook dispatch failed", "error", err, "alert_id", alert.ID)
	}
}