package wallet

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"learning.local/sportsbook/internal/domain"
	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
	"learning.local/sportsbook/internal/pkg/uuid"
)

// ConsumerWalletReserve is stored in processed_events.consumer_name for idempotency (design §6.2).
const ConsumerWalletReserve = "wallet-reserve"

// HandleBetPlaced applies bet.placed.v1: reserve stake, mark processed, then enqueue wallet.reserved.v1 in outbox.
//
// Insufficient funds or currency mismatch: bet is set to REJECTED, processed_events is written for this
// bet.placed event_id so the message is not retried indefinitely; we ACK (no DLQ spam for MVP).
func HandleBetPlaced(ctx context.Context, db *sql.DB, body []byte, log *slog.Logger) error {
	var env domain.BetPlacedV1
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("%w: decode bet.placed json: %v", rmq.ErrPoison, err)
	}
	log = log.With(
		"correlation_id", env.Metadata.CorrelationID,
		"bet_placed_event_id", env.Metadata.EventID,
		"bet_id", env.Payload.BetID,
	)
	log.Debug("handling bet.placed event")

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
		env.Metadata.EventID, ConsumerWalletReserve,
	).Scan(&already); err != nil {
		return fmt.Errorf("dedup check: %w", err)
	}
	if already > 0 {
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
		log.Info("wallet skip duplicate bet.placed (already processed)")
		return nil
	}

	var availStr string
	var wCurrency string
	var wVersion int
	err = tx.QueryRowContext(ctx,
		`SELECT available_amount, currency, version FROM wallets WHERE user_id = ? FOR UPDATE`,
		env.Payload.UserID,
	).Scan(&availStr, &wCurrency, &wVersion)
	if err == sql.ErrNoRows {
		log.Warn("wallet not found for user", "user_id", env.Payload.UserID)
		if err := markBetRejectedAndProcessed(ctx, tx, &env, log, "no wallet row"); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("load wallet: %w", err)
	}

	stakeStr := strings.TrimSpace(env.Payload.Amount)
	stake, err := strconv.ParseFloat(stakeStr, 64)
	if err != nil {
		return fmt.Errorf("parse stake: %w", err)
	}
	avail, err := strconv.ParseFloat(availStr, 64)
	if err != nil {
		return fmt.Errorf("parse available: %w", err)
	}

	if strings.ToUpper(strings.TrimSpace(wCurrency)) != strings.ToUpper(strings.TrimSpace(env.Payload.Currency)) {
		log.Warn("currency mismatch", "wallet_currency", wCurrency, "event_currency", env.Payload.Currency)
		if err := markBetRejectedAndProcessed(ctx, tx, &env, log, "currency mismatch"); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
		return nil
	}

	if avail < stake {
		log.Warn("insufficient funds", "available", avail, "stake", stake)
		if err := markBetRejectedAndProcessed(ctx, tx, &env, log, "insufficient funds"); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
		return nil
	}

	log.Debug("debiting wallet", "user_id", env.Payload.UserID, "amount", stakeStr)
	res, err := tx.ExecContext(ctx, `
UPDATE wallets
SET available_amount = available_amount - CAST(? AS DECIMAL(19,4)),
    version = version + 1
WHERE user_id = ? AND version = ? AND available_amount >= CAST(? AS DECIMAL(19,4))`,
		stakeStr, env.Payload.UserID, wVersion, stakeStr,
	)
	if err != nil {
		return fmt.Errorf("wallet debit: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("%w: wallet debit rows=%d (likely version mismatch)", rmq.ErrTransient, n)
	}

	reservationID, err := uuid.New()
	if err != nil {
		return fmt.Errorf("reservation id: %w", err)
	}
	walletEventID, err := uuid.New()
	if err != nil {
		return fmt.Errorf("wallet event id: %w", err)
	}

	out := domain.WalletReservedV1{
		Metadata: domain.EventMetadata{
			EventID:        walletEventID,
			CorrelationID:  env.Metadata.CorrelationID,
			CausationID:    env.Metadata.EventID,
			Timestamp:      domain.RFC3339NanoUTC(time.Now().UTC()),
			Version:        "v1",
			Producer:       domain.ProducerWalletService,
		},
		Payload: domain.WalletReservedV1Payload{
			ReservationID: reservationID,
			UserID:          env.Payload.UserID,
			Amount:          stakeStr,
			Currency:        strings.ToUpper(strings.TrimSpace(env.Payload.Currency)),
			BetID:           env.Payload.BetID,
		},
	}
	payload, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshal wallet.reserved: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO outbox_events (event_type, routing_key, payload_json, aggregate_id)
VALUES (?, ?, ?, ?)`,
		domain.EventTypeWalletReservedV1, domain.RoutingKeyWalletReservedV1, string(payload), env.Payload.BetID,
	); err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO processed_events (event_id, consumer_name) VALUES (?, ?)`,
		env.Metadata.EventID, ConsumerWalletReserve,
	); err != nil {
		return fmt.Errorf("insert processed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true

	log.Info("wallet reserved, outbox event recorded", "wallet_event_id", walletEventID)
	return nil
}

func markBetRejectedAndProcessed(ctx context.Context, tx *sql.Tx, env *domain.BetPlacedV1, log *slog.Logger, reason string) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE bets SET status = ?, updated_at = CURRENT_TIMESTAMP(6) WHERE id = ? AND status = ?`,
		domain.BetStatusRejected, env.Payload.BetID, domain.BetStatusPendingReserve,
	); err != nil {
		return fmt.Errorf("reject bet: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO processed_events (event_id, consumer_name) VALUES (?, ?)`,
		env.Metadata.EventID, ConsumerWalletReserve,
	); err != nil {
		return fmt.Errorf("insert processed (reject path): %w", err)
	}
	log.Warn("wallet rejected bet", "reason", reason)
	return nil
}
