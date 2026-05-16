package wallet

import (
	"context"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
)

// RunBetPlacedConsumer blocks on q.wallet.bet-placed.
func RunBetPlacedConsumer(ctx context.Context, svc *Service, ch *amqp.Channel, consumerTag string, inflight *sync.WaitGroup) error {
	return rmq.ConsumeWithWaitGroup(ctx, ch, rmq.QueueWalletBetPlaced, consumerTag, 0, inflight, func(ctx context.Context, d amqp.Delivery) error {
		return svc.ReserveBalance(ctx, d.Body)
	})
}

// RunBetSettledConsumer consumes q.wallet.bet-settled.
func RunBetSettledConsumer(ctx context.Context, svc *Service, ch *amqp.Channel, consumerTag string, inflight *sync.WaitGroup) error {
	return rmq.ConsumeWithWaitGroup(ctx, ch, rmq.QueueWalletBetSettled, consumerTag, 0, inflight, func(ctx context.Context, d amqp.Delivery) error {
		return svc.SettleBalance(ctx, d.Body)
	})
}
