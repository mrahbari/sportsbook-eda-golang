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
)

// HandleDelivery logs accepted/settled events (tutorial stand-in for push / email / WS).
func HandleDelivery(ctx context.Context, d amqp.Delivery, log *slog.Logger) error {
	rk := d.RoutingKey
	log.Debug("received event for notification", "routing_key", rk)
	switch {
	case strings.Contains(rk, "bet.accepted"):
		var env domain.BetAcceptedV1
		if err := json.Unmarshal(d.Body, &env); err != nil {
			return fmt.Errorf("%w: json: %v", rmq.ErrPoison, err)
		}
		log.Info("NOTIFY: Bet Accepted",
			"correlation_id", env.Metadata.CorrelationID,
			"event_id", env.Metadata.EventID,
			"bet_id", env.Payload.BetID,
		)
	case strings.Contains(rk, "bet.settled"):
		var env domain.BetSettledV1
		if err := json.Unmarshal(d.Body, &env); err != nil {
			return fmt.Errorf("%w: json: %v", rmq.ErrPoison, err)
		}
		log.Info("NOTIFY: Bet Settled",
			"correlation_id", env.Metadata.CorrelationID,
			"event_id", env.Metadata.EventID,
			"bet_id", env.Payload.BetID,
			"outcome", env.Payload.Outcome,
			"payout", env.Payload.Payout,
		)
	default:
		log.Warn("notify unknown routing key pattern", "rk", rk)
	}
	return nil
}

// Run starts q.notify.bet-events consumer.
func Run(ctx context.Context, ch *amqp.Channel, log *slog.Logger, tag string, inflight *sync.WaitGroup) error {
	return rmq.ConsumeWithWaitGroup(ctx, ch, rmq.QueueNotifyBetEvents, tag, 0, inflight, func(ctx context.Context, d amqp.Delivery) error {
		return HandleDelivery(ctx, d, log)
	})
}
