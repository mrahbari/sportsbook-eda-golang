package wallet

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
	WalletRepo    domain.WalletRepository
	BetRepo       domain.BetRepository
	OutboxRepo    domain.OutboxRepository
	ProcessedRepo domain.ProcessedEventRepository
	Log           *slog.Logger
}

func NewService(
	db *sql.DB,
	walletRepo domain.WalletRepository,
	betRepo domain.BetRepository,
	outboxRepo domain.OutboxRepository,
	processedRepo domain.ProcessedEventRepository,
	log *slog.Logger,
) *Service {
	return &Service{
		DB:            db,
		WalletRepo:    walletRepo,
		BetRepo:       betRepo,
		OutboxRepo:    outboxRepo,
		ProcessedRepo: processedRepo,
		Log:           log,
	}
}

func (s *Service) ReserveBalance(ctx context.Context, body []byte) error {
	var env domain.BetPlacedV1
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("%w: decode bet.placed json: %v", rmq.ErrPoison, err)
	}

	logger := tracing.Log(ctx, s.Log).With(
		"correlation_id", env.Metadata.CorrelationID,
		"bet_id", env.Payload.BetID,
	)
	logger.Debug("handling bet.placed event")

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	processed, err := s.ProcessedRepo.IsProcessed(ctx, tx, env.Metadata.EventID, ConsumerWalletReserve)
	if err != nil {
		return err
	}
	if processed {
		return tx.Commit()
	}

	wallet, err := s.WalletRepo.GetByUserID(ctx, tx, env.Payload.UserID, true)
	if err == sql.ErrNoRows {
		if err := s.rejectBet(ctx, tx, &env, "no wallet"); err != nil {
			return err
		}
		return tx.Commit()
	}
	if err != nil {
		return err
	}

	if strings.ToUpper(wallet.Currency) != strings.ToUpper(env.Payload.Currency) {
		if err := s.rejectBet(ctx, tx, &env, "currency mismatch"); err != nil {
			return err
		}
		return tx.Commit()
	}

	debitAmount := "-" + env.Payload.Amount
	if err := s.WalletRepo.UpdateBalance(ctx, tx, wallet.UserID, debitAmount, wallet.Version); err != nil {
		return err
	}

	// Create Outbox Event
	walletEventID, _ := uuid.New()
	reservationID, _ := uuid.New()
	out := domain.WalletReservedV1{
		Metadata: domain.EventMetadata{
			EventID:       walletEventID,
			CorrelationID: env.Metadata.CorrelationID,
			CausationID:   env.Metadata.EventID,
			Timestamp:     domain.RFC3339NanoUTC(time.Now().UTC()),
			Version:       "v1",
			Producer:      domain.ProducerWalletService,
		},
		Payload: domain.WalletReservedV1Payload{
			ReservationID: reservationID,
			UserID:        env.Payload.UserID,
			Amount:        env.Payload.Amount,
			Currency:      wallet.Currency,
			BetID:         env.Payload.BetID,
		},
	}
	payload, _ := json.Marshal(out)

	if err := s.OutboxRepo.Save(ctx, tx, &domain.OutboxEvent{
		EventType:   domain.EventTypeWalletReservedV1,
		RoutingKey:  domain.RoutingKeyWalletReservedV1,
		PayloadJSON: string(payload),
		AggregateID: env.Payload.BetID,
	}); err != nil {
		return err
	}

	if err := s.ProcessedRepo.MarkProcessed(ctx, tx, env.Metadata.EventID, ConsumerWalletReserve); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	logger.Info("wallet reserved, outbox event recorded")
	return nil
}

func (s *Service) SettleBalance(ctx context.Context, body []byte) error {
	var env domain.BetSettledV1
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("%w: decode bet.settled json: %v", rmq.ErrPoison, err)
	}

	logger := tracing.Log(ctx, s.Log).With(
		"correlation_id", env.Metadata.CorrelationID,
		"bet_id", env.Payload.BetID,
	)
	logger.Debug("handling bet.settled event")

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	processed, err := s.ProcessedRepo.IsProcessed(ctx, tx, env.Metadata.EventID, ConsumerWalletSettled)
	if err != nil {
		return err
	}
	if processed {
		return tx.Commit()
	}

	payoutStr := strings.TrimSpace(env.Payload.Payout)
	// Simplified check for LOSE outcome with 0 payout
	if payoutStr == "0" || payoutStr == "0.00" || payoutStr == "0.0000" {
		if strings.EqualFold(env.Payload.Outcome, "LOSE") {
			if err := s.ProcessedRepo.MarkProcessed(ctx, tx, env.Metadata.EventID, ConsumerWalletSettled); err != nil {
				return err
			}
			return tx.Commit()
		}
	}

	wallet, err := s.WalletRepo.GetByUserID(ctx, tx, env.Payload.UserID, true)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: no wallet for user", rmq.ErrPoison)
	}
	if err != nil {
		return err
	}

	if err := s.WalletRepo.UpdateBalance(ctx, tx, wallet.UserID, payoutStr, wallet.Version); err != nil {
		return err
	}

	// Produce wallet.credited event for settlement
	creditedEventID, _ := uuid.New()
	credited := domain.WalletCreditedV1{
		Metadata: domain.EventMetadata{
			EventID:       creditedEventID,
			CorrelationID: env.Metadata.CorrelationID,
			CausationID:   env.Metadata.EventID,
			Timestamp:     domain.RFC3339NanoUTC(time.Now().UTC()),
			Version:       "v1",
			Producer:      domain.ProducerWalletService,
		},
		Payload: domain.WalletCreditedV1Payload{
			UserID:   env.Payload.UserID,
			Amount:   payoutStr,
			Currency: wallet.Currency,
			BetID:    env.Payload.BetID,
			Reason:   "BET_SETTLEMENT",
		},
	}
	payload, _ := json.Marshal(credited)

	if err := s.OutboxRepo.Save(ctx, tx, &domain.OutboxEvent{
		EventType:   domain.EventTypeWalletCreditedV1,
		RoutingKey:  domain.RoutingKeyWalletCreditedV1,
		PayloadJSON: string(payload),
		AggregateID: env.Payload.UserID,
	}); err != nil {
		return err
	}

	if err := s.ProcessedRepo.MarkProcessed(ctx, tx, env.Metadata.EventID, ConsumerWalletSettled); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	logger.Info("wallet credited settlement", "payout", payoutStr, "outcome", env.Payload.Outcome)
	return nil
}

func (s *Service) rejectBet(ctx context.Context, tx *sql.Tx, env *domain.BetPlacedV1, reason string) error {
	if err := s.BetRepo.UpdateStatus(ctx, tx, env.Payload.BetID, domain.BetStatusRejected); err != nil {
		return err
	}
	return s.ProcessedRepo.MarkProcessed(ctx, tx, env.Metadata.EventID, ConsumerWalletReserve)
}
