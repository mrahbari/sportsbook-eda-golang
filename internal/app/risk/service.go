package risk

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
	"learning.local/sportsbook/internal/domain"
	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
	"learning.local/sportsbook/internal/pkg/tracing"
)

type Service struct {
	DB            *sql.DB
	ProcessedRepo domain.ProcessedEventRepository
	Log           *slog.Logger
}

func NewService(db *sql.DB, processedRepo domain.ProcessedEventRepository, log *slog.Logger) *Service {
	return &Service{
		DB:            db,
		ProcessedRepo: processedRepo,
		Log:           log,
	}
}

const ConsumerRiskBetPlaced = "risk-bet-placed"

func (s *Service) EvaluateRisk(ctx context.Context, body []byte) error {
	var env domain.BetPlacedV1
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("%w: json: %v", rmq.ErrPoison, err)
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	processed, err := s.ProcessedRepo.IsProcessed(ctx, tx, env.Metadata.EventID, ConsumerRiskBetPlaced)
	if err != nil {
		return err
	}
	if processed {
		return tx.Commit()
	}

	// Trivial async risk logic: log and mark processed.
	tracing.Log(ctx, s.Log).Info("risk evaluated OK",
		"bet_id", env.Payload.BetID,
		"market_id", env.Payload.MarketID,
		"stake", env.Payload.Amount,
	)

	if err := s.ProcessedRepo.MarkProcessed(ctx, tx, env.Metadata.EventID, ConsumerRiskBetPlaced); err != nil {
		return err
	}

	return tx.Commit()
}

func Run(ctx context.Context, svc *Service, ch *amqp.Channel, tag string, inflight *sync.WaitGroup) error {
	return rmq.ConsumeWithWaitGroup(ctx, ch, rmq.QueueRiskBetPlaced, tag, 0, inflight, func(ctx context.Context, d amqp.Delivery) error {
		return svc.EvaluateRisk(ctx, d.Body)
	})
}
