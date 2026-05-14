package outboxrelay

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
	"learning.local/sportsbook/internal/metrics"
)

// Relay polls outbox_events and publishes to RabbitMQ (at-least-once).
// Duplicate publishes can occur if the process crashes after Publish but before COMMIT;
// consumers must be idempotent (design §6.2).
//
// Assumption: a single relay instance per deployment (no SKIP LOCKED multi-worker yet).
type Relay struct {
	DB  *sql.DB
	Pub *rmq.Publisher
	Log *slog.Logger
}

func (r *Relay) Run(ctx context.Context) {
	r.Log.Info("starting outbox relay poller")
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	failBackoff := time.Second
	for {
		select {
		case <-ctx.Done():
			r.Log.Info("outbox relay stopped", "reason", ctx.Err())
			return
		case <-t.C:
			if err := r.processOne(ctx); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					failBackoff = time.Second
					continue
				}
				r.Log.Error("outbox relay tick error", "err", err, "backoff", failBackoff.String())
				select {
				case <-ctx.Done():
					return
				case <-time.After(failBackoff):
				}
				if failBackoff < 30*time.Second {
					failBackoff *= 2
				}
				continue
			}
			failBackoff = time.Second
		}
	}
}

// processOne locks one unpublished row, publishes, marks published in the same DB transaction.
func (r *Relay) processOne(ctx context.Context) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var id int64
	var routingKey string
	var payload []byte
	err = tx.QueryRowContext(ctx, `
SELECT id, routing_key, payload_json
FROM outbox_events
WHERE published_at IS NULL
ORDER BY id ASC
LIMIT 1
FOR UPDATE`).Scan(&id, &routingKey, &payload)
	if err != nil {
		return err
	}

	r.Log.Debug("publishing event from outbox", "outbox_id", id, "routing_key", routingKey)

	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := r.Pub.PublishJSON(pubCtx, rmq.ExchangeBettingTopic, routingKey, payload); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE outbox_events SET published_at = CURRENT_TIMESTAMP(6) WHERE id = ?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	metrics.OutboxPublished.Add(1)
	r.Log.Info("outbox event published and marked sent", "outbox_id", id, "routing_key", routingKey)
	return nil
}
