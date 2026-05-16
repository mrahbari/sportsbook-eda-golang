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
	"learning.local/sportsbook/internal/pkg/tracing"
	"learning.local/sportsbook/internal/pkg/uuid"
)

type Service struct {
	DB            *sql.DB
	BetRepo       domain.BetRepository
	OutboxRepo    domain.OutboxRepository
	ProcessedRepo domain.ProcessedEventRepository
	Log           *slog.Logger
}

func NewService(db *sql.DB, betRepo domain.BetRepository, outboxRepo domain.OutboxRepository, processedRepo domain.ProcessedEventRepository, log *slog.Logger) *Service {
	return &Service{
		DB:            db,
		BetRepo:       betRepo,
		OutboxRepo:    outboxRepo,
		ProcessedRepo: processedRepo,
		Log:           log,
	}
}

const ConsumerBetAccept = "bet-accept"

// AcceptBet handles the wallet.reserved event to move a bet from PENDING to OPEN.
func (s *Service) AcceptBet(ctx context.Context, body []byte) error {
	var env domain.WalletReservedV1
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("%w: decode wallet.reserved json: %v", rmq.ErrPoison, err)
	}

	logger := tracing.Log(ctx, s.Log).With(
		"correlation_id", env.Metadata.CorrelationID,
		"bet_id", env.Payload.BetID,
	)
	logger.Debug("handling wallet.reserved event")

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	processed, err := s.ProcessedRepo.IsProcessed(ctx, tx, env.Metadata.EventID, ConsumerBetAccept)
	if err != nil {
		return err
	}
	if processed {
		return tx.Commit()
	}

	betID := strings.TrimSpace(env.Payload.BetID)
	rows, err := s.BetRepo.UpdateStatusSelective(ctx, tx, betID, domain.BetStatusOpen, domain.BetStatusPendingReserve)
	if err != nil {
		return err
	}

	if rows == 0 {
		bet, err := s.BetRepo.GetByID(ctx, betID)
		if err == sql.ErrNoRows {
			return fmt.Errorf("bet not found: %s", betID)
		}
		if err != nil {
			return err
		}
		if bet.Status == domain.BetStatusOpen {
			if err := s.ProcessedRepo.MarkProcessed(ctx, tx, env.Metadata.EventID, ConsumerBetAccept); err != nil {
				return err
			}
			return tx.Commit()
		}
		return fmt.Errorf("bet %s in unexpected status %s", betID, bet.Status)
	}

	acceptEventID, _ := uuid.New()
	accepted := domain.BetAcceptedV1{
		Metadata: domain.EventMetadata{
			EventID:       acceptEventID,
			CorrelationID: env.Metadata.CorrelationID,
			CausationID:   env.Metadata.EventID,
			Timestamp:     domain.RFC3339NanoUTC(time.Now().UTC()),
			Version:       "v1",
			Producer:      domain.ProducerBetService,
		},
		Payload: domain.BetAcceptedV1Payload{
			BetID: betID,
		},
	}
	acceptJSON, _ := json.Marshal(accepted)

	if err := s.OutboxRepo.Save(ctx, tx, &domain.OutboxEvent{
		EventType:   domain.EventTypeBetAcceptedV1,
		RoutingKey:  domain.RoutingKeyBetAcceptedV1,
		PayloadJSON: string(acceptJSON),
		AggregateID: betID,
	}); err != nil {
		return err
	}

	if err := s.ProcessedRepo.MarkProcessed(ctx, tx, env.Metadata.EventID, ConsumerBetAccept); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	logger.Info("bet accepted and OPENED", "bet_accepted_event_id", acceptEventID)
	return nil
}

// PlaceBet validates the input and records a new bet in a PENDING state within a transaction.
func (s *Service) PlaceBet(ctx context.Context, cmd PlaceBetCommand) (Result, error) {
	logger := tracing.Log(ctx, s.Log)
	var zero Result
	if err := validate(cmd); err != nil {
		return zero, err
	}

	betID, _ := uuid.New()
	eventID, _ := uuid.New()
	
	correlationID := tracing.FromContext(ctx)
	if correlationID == "" {
		correlationID, _ = uuid.New()
	}

	causationID := strings.TrimSpace(cmd.CausationID)
	if causationID == "" {
		causationID = eventID
	}

	bet := &domain.Bet{
		ID:            betID,
		UserID:        cmd.UserID,
		StakeAmount:   cmd.StakeAmount,
		Currency:      strings.ToUpper(strings.TrimSpace(cmd.Currency)),
		SelectionID:   cmd.SelectionID,
		MarketID:      cmd.MarketID,
		Odds:          cmd.Odds,
		OddsVersion:   cmd.OddsVersion,
		Status:        domain.BetStatusPendingReserve,
		CorrelationID: correlationID,
	}

	event := domain.BetPlacedV1{
		Metadata: domain.EventMetadata{
			EventID:       eventID,
			CorrelationID: correlationID,
			CausationID:   causationID,
			Timestamp:     domain.RFC3339NanoUTC(time.Now().UTC()),
			Version:       "v1",
			Producer:      domain.ProducerBetService,
		},
		Payload: domain.BetPlacedV1Payload{
			BetID:       bet.ID,
			UserID:      bet.UserID,
			Amount:      bet.StakeAmount,
			Currency:    bet.Currency,
			SelectionID: bet.SelectionID,
			MarketID:    bet.MarketID,
			Odds:        bet.Odds,
			OddsVersion: bet.OddsVersion,
		},
	}

	payloadJSON, err := json.Marshal(event)
	if err != nil {
		return zero, err
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback()

	if err := s.BetRepo.Save(ctx, tx, bet); err != nil {
		return zero, fmt.Errorf("repo save bet: %w", err)
	}

	outboxEvent := &domain.OutboxEvent{
		EventType:   domain.EventTypeBetPlacedV1,
		RoutingKey:  domain.RoutingKeyBetPlacedV1,
		PayloadJSON: string(payloadJSON),
		AggregateID: bet.ID,
	}

	if err := s.OutboxRepo.Save(ctx, tx, outboxEvent); err != nil {
		return zero, fmt.Errorf("repo save outbox: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return zero, err
	}

	logger.Info("bet placed", "bet_id", bet.ID)
	return Result{BetID: bet.ID, CorrelationID: correlationID}, nil
}
