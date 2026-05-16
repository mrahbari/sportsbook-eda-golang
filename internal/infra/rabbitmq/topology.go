package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Naming: one topic exchange for the tutorial (see sportsbook design §4).
// DLX fanout receives dead-lettered messages from main queues (NACK requeue=false).
const (
	ExchangeBettingTopic = "xc.betting.topic"
	ExchangeBettingDLX   = "xc.betting.dlx"

	QueueWalletBetPlaced      = "q.wallet.bet-placed"
	QueueWalletBetPlacedDLQ   = "q.wallet.bet-placed.dlq"
	QueueBetWalletReserved    = "q.bet.wallet-reserved"
	QueueBetWalletReservedDLQ = "q.bet.wallet-reserved.dlq"

	// QueueRiskBetPlaced Risk / analytics (design diagram): async check on bet placement.
	QueueRiskBetPlaced    = "q.risk.bet-placed"
	QueueRiskBetPlacedDLQ = "q.risk.bet-placed.dlq"

	// QueueWalletBetSettled Wallet settlement leg (payout after bet.settled).
	QueueWalletBetSettled    = "q.wallet.bet-settled"
	QueueWalletBetSettledDLQ = "q.wallet.bet-settled.dlq"

	// QueueNotifyBetEvents Notification fan-in: accepted + settled (simple tutorial queue).
	QueueNotifyBetEvents    = "q.notify.bet-events"
	QueueNotifyBetEventsDLQ = "q.notify.bet-events.dlq"

	// PrefetchDefault PrefetchCount caps unacked deliveries per consumer (design §7.2).
	PrefetchDefault = 30
)

// DeclareBettingTopology creates exchanges, DLQs, main queues, and bindings for all tutorial services.
// Safe to call on every process start (idempotent parameters).
func DeclareBettingTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(
		ExchangeBettingDLX,
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("declare dlx: %w", err)
	}

	dlqQueues := []string{
		QueueWalletBetPlacedDLQ,
		QueueBetWalletReservedDLQ,
		QueueRiskBetPlacedDLQ,
		QueueWalletBetSettledDLQ,
		QueueNotifyBetEventsDLQ,
	}
	for _, q := range dlqQueues {
		if _, err := ch.QueueDeclare(q, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare dlq %s: %w", q, err)
		}
		if err := ch.QueueBind(q, "", ExchangeBettingDLX, false, nil); err != nil {
			return fmt.Errorf("bind dlq %s: %w", q, err)
		}
	}

	if err := ch.ExchangeDeclare(
		ExchangeBettingTopic,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("declare topic exchange: %w", err)
	}

	dlxArgs := amqp.Table{
		"x-dead-letter-exchange": ExchangeBettingDLX,
	}

	declareBind := func(name, routingKey string) error {
		if _, err := ch.QueueDeclare(name, true, false, false, false, dlxArgs); err != nil {
			return fmt.Errorf("declare queue %s: %w", name, err)
		}
		if err := ch.QueueBind(name, routingKey, ExchangeBettingTopic, false, nil); err != nil {
			return fmt.Errorf("bind %s -> %s: %w", name, routingKey, err)
		}
		return nil
	}

	if err := declareBind(QueueWalletBetPlaced, "bet.placed.*"); err != nil {
		return err
	}
	if err := declareBind(QueueRiskBetPlaced, "bet.placed.*"); err != nil {
		return err
	}
	if err := declareBind(QueueBetWalletReserved, "wallet.reserved.*"); err != nil {
		return err
	}
	if err := declareBind(QueueWalletBetSettled, "bet.settled.*"); err != nil {
		return err
	}
	if err := declareBind(QueueNotifyBetEvents, "bet.accepted.*"); err != nil {
		return err
	}
	if err := ch.QueueBind(QueueNotifyBetEvents, "bet.settled.*", ExchangeBettingTopic, false, nil); err != nil {
		return fmt.Errorf("bind notify second pattern: %w", err)
	}

	return nil
}
