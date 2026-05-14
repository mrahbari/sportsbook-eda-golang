package rabbitmq

import (
	"context"
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher wraps a channel with a mutex — channels are not goroutine-safe (design §7.3).
type Publisher struct {
	mu sync.Mutex
	ch *amqp.Channel
}

func NewPublisher(ch *amqp.Channel) *Publisher {
	return &Publisher{ch: ch}
}

// PublishJSON publishes a persistent JSON message (mandatory=false; publisher confirms out of scope).
func (p *Publisher) PublishJSON(ctx context.Context, exchange, routingKey string, body []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.ch.PublishWithContext(ctx,
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
}

// PublishJSONDefault uses ExchangeBettingTopic.
func (p *Publisher) PublishJSONDefault(ctx context.Context, routingKey string, body []byte) error {
	if err := p.PublishJSON(ctx, ExchangeBettingTopic, routingKey, body); err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	return nil
}
