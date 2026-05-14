package bets

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
)

// RunBetAcceptConsumer blocks on q.bet.wallet-reserved.
func RunBetAcceptConsumer(ctx context.Context, db *sql.DB, ch *amqp.Channel, log *slog.Logger, consumerTag string, inflight *sync.WaitGroup) error {
	return rmq.ConsumeWithWaitGroup(ctx, ch, rmq.QueueBetWalletReserved, consumerTag, 0, inflight, func(ctx context.Context, d amqp.Delivery) error {
		return HandleWalletReserved(ctx, db, d.Body, log)
	})
}
