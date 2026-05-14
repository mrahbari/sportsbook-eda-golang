package bets

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"learning.local/sportsbook/internal/domain"
	"learning.local/sportsbook/internal/pkg/uuid"
)

// PlaceBetCommand is the application input for placing a bet.
type PlaceBetCommand struct {
	UserID       string
	StakeAmount  string // decimal string, e.g. "10.50"
	Currency     string
	SelectionID  string
	MarketID     string
	Odds         float64
	OddsVersion  int
	CausationID  string // optional; e.g. X-Request-Id from HTTP layer
}

type Result struct {
	BetID          string
	CorrelationID  string
}

// PlaceBet inserts a bet and matching outbox row in one transaction (no Rabbit publish).
func PlaceBet(ctx context.Context, db *sql.DB, log *slog.Logger, cmd PlaceBetCommand) (Result, error) {
	var zero Result
	if err := validate(cmd); err != nil {
		return zero, err
	}

	betID, err := uuid.New()
	if err != nil {
		return zero, err
	}
	eventID, err := uuid.New()
	if err != nil {
		return zero, err
	}
	correlationID, err := uuid.New()
	if err != nil {
		return zero, err
	}

	causationID := strings.TrimSpace(cmd.CausationID)
	if causationID == "" {
		causationID = eventID
	}

	log = log.With("bet_id", betID, "correlation_id", correlationID)
	log.Debug("starting bet placement transaction")

	now := time.Now().UTC()
	env := domain.BetPlacedV1{
		Metadata: domain.EventMetadata{
			EventID:       eventID,
			CorrelationID: correlationID,
			CausationID:   causationID,
			Timestamp:     domain.RFC3339NanoUTC(now),
			Version:       "v1",
			Producer:      domain.ProducerBetService,
		},
		Payload: domain.BetPlacedV1Payload{
			BetID:       betID,
			UserID:      cmd.UserID,
			Amount:      cmd.StakeAmount,
			Currency:    strings.ToUpper(strings.TrimSpace(cmd.Currency)),
			SelectionID: cmd.SelectionID,
			MarketID:    cmd.MarketID,
			Odds:        cmd.Odds,
			OddsVersion: cmd.OddsVersion,
		},
	}

	payloadJSON, err := json.Marshal(env)
	if err != nil {
		return zero, fmt.Errorf("marshal envelope: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return zero, fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	_, err = tx.ExecContext(ctx, `
INSERT INTO bets (
  id, user_id, stake_amount, currency, selection_id, market_id, odds, odds_version, status, correlation_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		betID, cmd.UserID, cmd.StakeAmount, strings.ToUpper(strings.TrimSpace(cmd.Currency)),
		cmd.SelectionID, cmd.MarketID, cmd.Odds, cmd.OddsVersion,
		domain.BetStatusPendingReserve, correlationID,
	)
	if err != nil {
		return zero, fmt.Errorf("insert bet: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO outbox_events (event_type, routing_key, payload_json, aggregate_id)
VALUES (?, ?, ?, ?)`,
		domain.EventTypeBetPlacedV1, domain.RoutingKeyBetPlacedV1, string(payloadJSON), betID,
	)
	if err != nil {
		return zero, fmt.Errorf("insert outbox: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return zero, fmt.Errorf("commit: %w", err)
	}
	committed = true

	log.Info("bet and outbox event persisted")
	return Result{BetID: betID, CorrelationID: correlationID}, nil
}

func validate(cmd PlaceBetCommand) error {
	if strings.TrimSpace(cmd.UserID) == "" {
		return fmt.Errorf("user_id: %w", ErrValidation)
	}
	stake, err := strconv.ParseFloat(strings.TrimSpace(cmd.StakeAmount), 64)
	if err != nil || stake <= 0 {
		return fmt.Errorf("stake_amount must be a positive number: %w", ErrValidation)
	}
	if len(strings.TrimSpace(cmd.Currency)) != 3 {
		return fmt.Errorf("currency must be 3 letters: %w", ErrValidation)
	}
	if strings.TrimSpace(cmd.SelectionID) == "" || strings.TrimSpace(cmd.MarketID) == "" {
		return fmt.Errorf("selection_id and market_id: %w", ErrValidation)
	}
	if cmd.Odds <= 0 {
		return fmt.Errorf("odds must be positive: %w", ErrValidation)
	}
	if cmd.OddsVersion < 0 {
		return fmt.Errorf("odds_version: %w", ErrValidation)
	}
	return nil
}

var (
	ErrValidation      = errors.New("validation error")
	ErrMarketSuspended = errors.New("market is suspended")
	ErrOddsDrift       = errors.New("odds version mismatch (drift)")
)
