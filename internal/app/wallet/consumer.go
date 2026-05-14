package wallet

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
)

// RunBetPlacedConsumer blocks on q.wallet.bet-placed (single goroutine per channel; design §7.3).
// inflight, if non-nil, counts in-flight handlers for graceful shutdown (prompt 08).
func RunBetPlacedConsumer(ctx context.Context, db *sql.DB, ch *amqp.Channel, log *slog.Logger, consumerTag string, inflight *sync.WaitGroup) error {
	return rmq.ConsumeWithWaitGroup(ctx, ch, rmq.QueueWalletBetPlaced, consumerTag, 0, inflight, func(ctx context.Context, d amqp.Delivery) error {
		return HandleBetPlaced(ctx, db, d.Body, log)
	})
}
