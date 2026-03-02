package graphql

import (
	"context"
	"log/slog"

	"github.com/Zubimendi/chainwatch/internal/detector"
	"github.com/Zubimendi/chainwatch/internal/store"
)

// Resolver holds dependencies injected at startup.
type Resolver struct {
	store      *store.Store
	alertCh    <-chan detector.Alert // read-only view of the detector's alert channel
	logger     *slog.Logger
	// subscribers is a registry of active GraphQL subscription connections
	subscribers map[string]chan detector.Alert
}

func NewResolver(st *store.Store, alertCh <-chan detector.Alert, logger *slog.Logger) *Resolver {
	r := &Resolver{
		store:       st,
		alertCh:     alertCh,
		logger:      logger,
		subscribers: make(map[string]chan detector.Alert),
	}
	go r.broadcastAlerts()
	return r
}

// broadcastAlerts reads from the shared alert channel and fans out to all
// active GraphQL subscription connections.
func (r *Resolver) broadcastAlerts() {
	for alert := range r.alertCh {
		for id, ch := range r.subscribers {
			select {
			case ch <- alert:
			default:
				r.logger.Warn("GraphQL subscriber channel full", "subscriber_id", id)
			}
		}
	}
}

// Alerts resolves the Query.alerts field.
func (r *Resolver) Alerts(ctx context.Context, limit *int, severity *string) ([]store.AlertRow, error) {
	l := 50
	if limit != nil {
		l = *limit
	}
	sev := ""
	if severity != nil {
		sev = *severity
	}
	return r.store.ListAlerts(ctx, l, sev)
}

// WatchedAddresses resolves the Query.watchedAddresses field.
func (r *Resolver) WatchedAddresses(ctx context.Context) ([]store.WatchRow, error) {
	return r.store.ListWatches(ctx)
}

// AddWatch resolves the Mutation.addWatch field.
func (r *Resolver) AddWatch(ctx context.Context, address string, label *string) (bool, error) {
	l := ""
	if label != nil {
		l = *label
	}
	return true, r.store.AddWatch(ctx, address, l)
}

// AlertFired resolves the Subscription.alertFired field.
// Each connection gets its own channel, registered here and cleaned up on disconnect.
func (r *Resolver) AlertFired(ctx context.Context) (<-chan *detector.Alert, error) {
	ch := make(chan *detector.Alert, 100)
	id := generateSubID()
	internalCh := make(chan detector.Alert, 100)
	r.subscribers[id] = internalCh

	// Bridge internal channel to the typed channel expected by gqlgen
	go func() {
		defer func() {
			delete(r.subscribers, id)
			close(ch)
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case alert := <-internalCh:
				a := alert
				ch <- &a
			}
		}
	}()

	return ch, nil
}

func generateSubID() string {
	// simple incrementing ID is fine for a portfolio project
	return fmt.Sprintf("sub_%d", time.Now().UnixNano())
}