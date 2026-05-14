package bets

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"learning.local/sportsbook/internal/domain"
	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
	"learning.local/sportsbook/internal/pkg/uuid"
)

// ConsumerBetAccept is the idempotency key for wallet.reserved → bet OPEN (prompt 07).
const ConsumerBetAccept = "bet-accept"

// HandleWalletReserved consumes wallet.reserved.v1: OPEN the bet and enqueue bet.accepted.v1 outbox.
func HandleWalletReserved(ctx context.Context, db *sql.DB, body []byte, log *slog.Logger) error {
	var env domain.WalletReservedV1
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("%w: decode wallet.reserved json: %v", rmq.ErrPoison, err)
	}
	log = log.With(
		"correlation_id", env.Metadata.CorrelationID,
		"wallet_reserved_event_id", env.Metadata.EventID,
		"bet_id", env.Payload.BetID,
	)
	log.Debug("handling wallet.reserved event")

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var already int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM processed_events WHERE event_id = ? AND consumer_name = ?`,
		env.Metadata.EventID, ConsumerBetAccept,
	).Scan(&already); err != nil {
		return fmt.Errorf("dedup check: %w", err)
	}
	if already > 0 {
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
		log.Info("bet-accept skip duplicate (already processed)")
		return nil
	}

	betID := strings.TrimSpace(env.Payload.BetID)
	log.Debug("opening bet", "bet_id", betID)
	res, err := tx.ExecContext(ctx, `
UPDATE bets
SET status = ?, updated_at = CURRENT_TIMESTAMP(6)
WHERE id = ? AND status = ?`,
		domain.BetStatusOpen, betID, domain.BetStatusPendingReserve,
	)
	if err != nil {
		return fmt.Errorf("update bet: %w", err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if ra == 0 {
		var st string
		err := tx.QueryRowContext(ctx, `SELECT status FROM bets WHERE id = ?`, betID).Scan(&st)
		if err == sql.ErrNoRows {
			log.Error("bet not found", "bet_id", betID)
			return fmt.Errorf("bet not found: %s", betID)
		}
		if err != nil {
			return err
		}
		if st == domain.BetStatusOpen {
			log.Info("bet already OPEN; idempotent ack")
			if _, err := tx.ExecContext(ctx,
				`INSERT IGNORE INTO processed_events (event_id, consumer_name) VALUES (?, ?)`,
				env.Metadata.EventID, ConsumerBetAccept,
			); err != nil {
				return fmt.Errorf("insert processed idempotent: %w", err)
			}
			if err := tx.Commit(); err != nil {
				return err
			}
			committed = true
			return nil
		}
		log.Warn("bet in unexpected status", "bet_id", betID, "status", st)
		return fmt.Errorf("bet %s in unexpected status %s", betID, st)
	}

	acceptEventID, err := uuid.New()
	if err != nil {
		return err
	}
	accepted := domain.BetAcceptedV1{
		Metadata: domain.EventMetadata{
			EventID:        acceptEventID,
			CorrelationID:  env.Metadata.CorrelationID,
			CausationID:    env.Metadata.EventID,
			Timestamp:      domain.RFC3339NanoUTC(time.Now().UTC()),
			Version:        "v1",
			Producer:       domain.ProducerBetService,
		},
		Payload: domain.BetAcceptedV1Payload{
			BetID: betID,
		},
	}
	acceptJSON, err := json.Marshal(accepted)
	if err != nil {
		return fmt.Errorf("marshal bet.accepted: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO outbox_events (event_type, routing_key, payload_json, aggregate_id)
VALUES (?, ?, ?, ?)`,
		domain.EventTypeBetAcceptedV1, domain.RoutingKeyBetAcceptedV1, string(acceptJSON), betID,
	); err != nil {
		return fmt.Errorf("insert outbox bet.accepted: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO processed_events (event_id, consumer_name) VALUES (?, ?)`,
		env.Metadata.EventID, ConsumerBetAccept,
	); err != nil {
		return fmt.Errorf("insert processed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true
	log.Info("bet accepted and OPENED", "bet_id", betID, "bet_accepted_event_id", acceptEventID)
	return nil
}
