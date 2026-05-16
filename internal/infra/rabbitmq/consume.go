package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"learning.local/sportsbook/internal/metrics"
	"learning.local/sportsbook/internal/pkg/tracing"
)

// Consume runs a blocking consumer loop (no in-flight WaitGroup tracking).
func Consume(ctx context.Context, ch *amqp.Channel, queue, consumerTag string, prefetch int, handler func(context.Context, amqp.Delivery) error) error {
	return ConsumeWithWaitGroup(ctx, ch, queue, consumerTag, prefetch, nil, handler)
}

// ConsumeWithWaitGroup is like Consume but tracks each handler invocation with wg (Add before, Done after).
// On ctx cancellation: sends consumer Cancel, drains deliveries for up to shutdownDrainTimeout,
// then waits up to shutdownHandlerWait for in-flight handlers.
const (
	shutdownDrainTimeout   = 25 * time.Second
	shutdownHandlerWait    = 8 * time.Second
	preNackBackoffInitial  = 200 * time.Millisecond
	preNackBackoffRedeliver = 2 * time.Second
)

func ConsumeWithWaitGroup(ctx context.Context, ch *amqp.Channel, queue, consumerTag string, prefetch int, inflight *sync.WaitGroup, handler func(context.Context, amqp.Delivery) error) error {
	if prefetch <= 0 {
		prefetch = PrefetchDefault
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		return fmt.Errorf("qos: %w", err)
	}

	deliveries, err := ch.Consume(queue, consumerTag, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", queue, err)
	}

	wrap := func(runCtx context.Context, d amqp.Delivery) error {
		if inflight != nil {
			inflight.Add(1)
			defer inflight.Done()
		}

		// Inject Correlation ID from message headers if present
		corrID, _ := d.Headers[tracing.HeaderXCorrelationID].(string)
		if corrID == "" {
			corrID = d.CorrelationId
		}
		if corrID != "" {
			runCtx = tracing.NewContext(runCtx, corrID)
		}

		return handler(runCtx, d)
	}

	for {
		select {
		case <-ctx.Done():
			return shutdownConsumer(ch, consumerTag, deliveries, inflight, wrap, ctx)
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("deliveries channel closed")
			}
			if err := wrap(ctx, d); err != nil {
				metrics.ConsumerHandlerErrors.Add(1)
				preNackBackoff(ctx, d, err)
				_ = d.Nack(false, false)
				continue
			}
			if err := d.Ack(false); err != nil {
				return fmt.Errorf("ack: %w", err)
			}
		}
	}
}

func shutdownConsumer(
	ch *amqp.Channel,
	consumerTag string,
	deliveries <-chan amqp.Delivery,
	inflight *sync.WaitGroup,
	wrap func(context.Context, amqp.Delivery) error,
	ctx context.Context,
) error {
	// Stop new deliveries from the broker (design §7.5).
	if err := ch.Cancel(consumerTag, false); err != nil {
		return fmt.Errorf("cancel consumer %q: %w", consumerTag, err)
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), shutdownDrainTimeout)
	defer cancel()

	for {
		select {
		case d, ok := <-deliveries:
			if !ok {
				waitInflightOrTimeout(inflight, shutdownHandlerWait)
				err := context.Cause(ctx)
				if err == nil {
					err = ctx.Err()
				}
				return err
			}
			if err := wrap(drainCtx, d); err != nil {
				metrics.ConsumerHandlerErrors.Add(1)
				_ = d.Nack(false, false)
			} else {
				_ = d.Ack(false)
			}
		case <-drainCtx.Done():
			waitInflightOrTimeout(inflight, shutdownHandlerWait)
			err := context.Cause(ctx)
			if err == nil {
				err = ctx.Err()
			}
			return fmt.Errorf("shutdown: drain timeout: %w", err)
		}
	}
}

func waitInflightOrTimeout(inflight *sync.WaitGroup, maxWait time.Duration) {
	if inflight == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(maxWait):
	}
}

func preNackBackoff(ctx context.Context, d amqp.Delivery, err error) {
	if errors.Is(err, ErrPoison) {
		return
	}
	delay := preNackBackoffInitial
	if d.Redelivered {
		delay = preNackBackoffRedeliver
	}
	// Weaker than broker TTL retry chains (§6.3): single sleep before NACK to reduce hot loops on DB contention.
	select {
	case <-ctx.Done():
	case <-time.After(delay):
	}
}
