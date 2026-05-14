package wallet

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	"learning.local/sportsbook/internal/domain"
	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
)

const ConsumerWalletSettled = "wallet-settled"

// HandleBetSettled credits wallet on bet.settled.v1 (tutorial: single ledger row).
func HandleBetSettled(ctx context.Context, db *sql.DB, body []byte, log *slog.Logger) error {
	var env domain.BetSettledV1
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("%w: decode bet.settled json: %v", rmq.ErrPoison, err)
	}
	log = log.With(
		"correlation_id", env.Metadata.CorrelationID,
		"bet_settled_event_id", env.Metadata.EventID,
		"bet_id", env.Payload.BetID,
	)
	log.Debug("handling bet.settled event")

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
		env.Metadata.EventID, ConsumerWalletSettled,
	).Scan(&already); err != nil {
		return err
	}
	if already > 0 {
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
		log.Info("wallet settle skip duplicate (already processed)")
		return nil
	}

	payoutStr := strings.TrimSpace(env.Payload.Payout)
	payout, err := strconv.ParseFloat(payoutStr, 64)
	if err != nil {
		return fmt.Errorf("%w: payout: %v", rmq.ErrPoison, err)
	}
	if payout < 0 {
		return fmt.Errorf("%w: negative payout", rmq.ErrPoison)
	}

	if payout == 0 && strings.EqualFold(env.Payload.Outcome, "LOSE") {
		log.Info("wallet settle noop lose outcome", "bet_id", env.Payload.BetID)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO processed_events (event_id, consumer_name) VALUES (?, ?)`,
			env.Metadata.EventID, ConsumerWalletSettled,
		); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
		return nil
	}

	var wVersion int
	err = tx.QueryRowContext(ctx,
		`SELECT version FROM wallets WHERE user_id = ? FOR UPDATE`,
		env.Payload.UserID,
	).Scan(&wVersion)
	if err == sql.ErrNoRows {
		log.Error("wallet not found for user", "user_id", env.Payload.UserID)
		return fmt.Errorf("%w: no wallet for user", rmq.ErrPoison)
	}
	if err != nil {
		return err
	}

	log.Debug("crediting wallet", "user_id", env.Payload.UserID, "payout", payoutStr)
	res, err := tx.ExecContext(ctx, `
UPDATE wallets
SET available_amount = available_amount + CAST(? AS DECIMAL(19,4)),
    version = version + 1
WHERE user_id = ? AND version = ?`,
		payoutStr, env.Payload.UserID, wVersion,
	)
	if err != nil {
		return fmt.Errorf("wallet credit: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("%w: wallet credit rows=%d (likely version mismatch)", rmq.ErrTransient, n)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO processed_events (event_id, consumer_name) VALUES (?, ?)`,
		env.Metadata.EventID, ConsumerWalletSettled,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	log.Info("wallet credited settlement", "user_id", env.Payload.UserID, "payout", payoutStr, "outcome", env.Payload.Outcome)
	return nil
}

// RunBetSettledConsumer consumes q.wallet.bet-settled.
func RunBetSettledConsumer(ctx context.Context, db *sql.DB, ch *amqp.Channel, log *slog.Logger, consumerTag string, inflight *sync.WaitGroup) error {
	return rmq.ConsumeWithWaitGroup(ctx, ch, rmq.QueueWalletBetSettled, consumerTag, 0, inflight, func(ctx context.Context, d amqp.Delivery) error {
		return HandleBetSettled(ctx, db, d.Body, log)
	})
}
