package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/Zubimendi/chainwatch/internal/detector"
)

// RedisProducer publishes alerts to a Redis Stream.
// Consumers (GraphQL subscriptions, external services) read from this stream.
type RedisProducer struct {
	client     *redis.Client
	streamName string
}

func NewRedisProducer(client *redis.Client, streamName string) *RedisProducer {
	return &RedisProducer{client: client, streamName: streamName}
}

// Publish adds an alert to the Redis Stream using XADD.
// Stream entries are capped at 10,000 entries (MAXLEN) to prevent unbounded growth.
func (r *RedisProducer) Publish(ctx context.Context, alert detector.Alert) error {
	payload, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("marshalling alert: %w", err)
	}

	// XADD chainwatch:alerts MAXLEN ~ 10000 * id <field> <value>
	// The '*' tells Redis to auto-generate the stream entry ID
	// MAXLEN ~ 10000 uses approximate trimming (faster, slightly imprecise)
	err = r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: r.streamName,
		MaxLen: 10000,
		Approx: true,
		ID:     "*",
		Values: map[string]interface{}{
			"alert_id": alert.ID,
			"type":     string(alert.Type),
			"severity": string(alert.Severity),
			"payload":  string(payload),
		},
	}).Err()

	if err != nil {
		return fmt.Errorf("XADD to stream %s: %w", r.streamName, err)
	}

	return nil
}