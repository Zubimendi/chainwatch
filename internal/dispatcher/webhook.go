package dispatcher

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Zubimendi/chainwatch/internal/detector"
	"github.com/Zubimendi/chainwatch/internal/store"
)

// WebhookSender delivers alerts to configured HTTP endpoints.
// Each webhook can have a secret for HMAC-SHA256 request signing,
// so the receiver can verify the request came from chainwatch.
type WebhookSender struct {
	store      *store.Store
	httpClient *http.Client
}

func NewWebhookSender(st *store.Store, timeout time.Duration) *WebhookSender {
	return &WebhookSender{
		store: st,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Send fetches all active webhooks and delivers the alert to each one.
func (w *WebhookSender) Send(ctx context.Context, alert detector.Alert) error {
	hooks, err := w.store.ListWebhooks(ctx)
	if err != nil {
		return fmt.Errorf("fetching webhooks: %w", err)
	}

	payload, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("marshalling alert: %w", err)
	}

	for _, hook := range hooks {
		// Check severity filter
		if !shouldSend(alert.Severity, detector.Severity(hook.MinSeverity)) {
			continue
		}
		go w.deliver(ctx, hook, payload, alert.ID)
	}

	return nil
}

func (w *WebhookSender) deliver(ctx context.Context, hook store.WebhookRow, payload []byte, alertID string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(payload))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ChainWatch-Alert-ID", alertID)

	// HMAC signature — receiver can verify with the shared secret
	if hook.Secret != "" {
		sig := hmacSHA256(payload, hook.Secret)
		req.Header.Set("X-ChainWatch-Signature", "sha256="+sig)
	}

	resp, err := w.httpClient.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	if err != nil {
		return
	}
}

// hmacSHA256 produces a hex-encoded HMAC-SHA256 of payload with key.
func hmacSHA256(payload []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// shouldSend returns true if the alert severity meets or exceeds the webhook's minimum.
func shouldSend(alertSev, minSev detector.Severity) bool {
	rank := map[detector.Severity]int{
		detector.SeverityCritical: 0,
		detector.SeverityHigh:     1,
		detector.SeverityMedium:   2,
		detector.SeverityLow:      3,
		detector.SeverityInfo:     4,
	}
	return rank[alertSev] <= rank[minSev]
}