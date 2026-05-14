package risk

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
	"learning.local/sportsbook/internal/domain"
	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
)

const ConsumerRiskBetPlaced = "risk-bet-placed"

// HandleBetPlaced performs a trivial async risk check (tutorial: persist idempotency + log).
func HandleBetPlaced(ctx context.Context, db *sql.DB, body []byte, log *slog.Logger) error {
	var env domain.BetPlacedV1
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("%w: json: %v", rmq.ErrPoison, err)
	}

	log = log.With(
		"correlation_id", env.Metadata.CorrelationID,
		"event_id", env.Metadata.EventID,
		"bet_id", env.Payload.BetID,
	)
	log.Debug("risk evaluating bet.placed")

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM processed_events WHERE event_id = ? AND consumer_name = ?`,
		env.Metadata.EventID, ConsumerRiskBetPlaced,
	).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
		log.Info("risk skip duplicate (already processed)")
		return nil
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO processed_events (event_id, consumer_name) VALUES (?, ?)`,
		env.Metadata.EventID, ConsumerRiskBetPlaced,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true

	log.Info("risk evaluated OK",
		"market_id", strings.TrimSpace(env.Payload.MarketID),
		"stake", env.Payload.Amount,
	)
	return nil
}

// Run consumes q.risk.bet-placed.
func Run(ctx context.Context, db *sql.DB, ch *amqp.Channel, log *slog.Logger, tag string, inflight *sync.WaitGroup) error {
	return rmq.ConsumeWithWaitGroup(ctx, ch, rmq.QueueRiskBetPlaced, tag, 0, inflight, func(ctx context.Context, d amqp.Delivery) error {
		return HandleBetPlaced(ctx, db, d.Body, log)
	})
}
