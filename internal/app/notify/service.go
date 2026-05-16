package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	"learning.local/sportsbook/internal/domain"
	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
	"learning.local/sportsbook/internal/pkg/tracing"
)

type Service struct {
	Log *slog.Logger
}

func NewService(log *slog.Logger) *Service {
	return &Service{Log: log}
}

func (s *Service) Notify(ctx context.Context, d amqp.Delivery) error {
	rk := d.RoutingKey
	logger := tracing.Log(ctx, s.Log)
	switch {
	case strings.Contains(rk, "bet.accepted"):
		var env domain.BetAcceptedV1
		if err := json.Unmarshal(d.Body, &env); err != nil {
			return fmt.Errorf("%w: json: %v", rmq.ErrPoison, err)
		}
		logger.Info("NOTIFY: Bet Accepted",
			"correlation_id", env.Metadata.CorrelationID,
			"bet_id", env.Payload.BetID,
		)
	case strings.Contains(rk, "bet.settled"):
		var env domain.BetSettledV1
		if err := json.Unmarshal(d.Body, &env); err != nil {
			return fmt.Errorf("%w: json: %v", rmq.ErrPoison, err)
		}
		logger.Info("NOTIFY: Bet Settled",
			"correlation_id", env.Metadata.CorrelationID,
			"bet_id", env.Payload.BetID,
			"outcome", env.Payload.Outcome,
			"payout", env.Payload.Payout,
		)
	}
	return nil
}

func Run(ctx context.Context, svc *Service, ch *amqp.Channel, tag string, inflight *sync.WaitGroup) error {
	return rmq.ConsumeWithWaitGroup(ctx, ch, rmq.QueueNotifyBetEvents, tag, 0, inflight, func(ctx context.Context, d amqp.Delivery) error {
		return svc.Notify(ctx, d)
	})
}
