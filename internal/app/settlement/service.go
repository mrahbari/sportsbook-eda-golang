package settlement

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"learning.local/sportsbook/internal/domain"
	"learning.local/sportsbook/internal/pkg/tracing"
	"learning.local/sportsbook/internal/pkg/uuid"
)

type Service struct {
	DB         *sql.DB
	BetRepo    domain.BetRepository
	OutboxRepo domain.OutboxRepository
	Log        *slog.Logger
}

func NewService(db *sql.DB, betRepo domain.BetRepository, outboxRepo domain.OutboxRepository, log *slog.Logger) *Service {
	return &Service{
		DB:         db,
		BetRepo:    betRepo,
		OutboxRepo: outboxRepo,
		Log:        log,
	}
}

func (s *Service) Settle(ctx context.Context, req SettleCommand) (string, error) {
	corr := strings.TrimSpace(req.CorrelationID)
	if corr == "" {
		corr, _ = uuid.New()
	}

	eventID, _ := uuid.New()

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	rows, err := s.BetRepo.Settle(ctx, tx, req.BetID, req.UserID, domain.BetStatusOpen)
	if err != nil {
		return "", err
	}
	if rows == 0 {
		return "", ErrBetNotOpen
	}

	event := domain.BetSettledV1{
		Metadata: domain.EventMetadata{
			EventID:       eventID,
			CorrelationID: corr,
			CausationID:   req.BetID,
			Timestamp:     domain.RFC3339NanoUTC(time.Now().UTC()),
			Version:       "v1",
			Producer:      domain.ProducerSettlementService,
		},
		Payload: domain.BetSettledV1Payload{
			BetID:    req.BetID,
			UserID:   req.UserID,
			Payout:   strings.TrimSpace(req.Payout),
			Currency: strings.ToUpper(strings.TrimSpace(req.Currency)),
			Outcome:  strings.ToUpper(strings.TrimSpace(req.Outcome)),
		},
	}
	if event.Payload.Currency == "" {
		event.Payload.Currency = "USD"
	}

	payloadJSON, _ := json.Marshal(event)

	if err := s.OutboxRepo.Save(ctx, tx, &domain.OutboxEvent{
		EventType:   domain.EventTypeBetSettledV1,
		RoutingKey:  domain.RoutingKeyBetSettledV1,
		PayloadJSON: string(payloadJSON),
		AggregateID: req.BetID,
	}); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	tracing.Log(ctx, s.Log).Info("settlement recorded", "bet_id", req.BetID, "event_id", eventID)
	return eventID, nil
}
