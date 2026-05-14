package rabbitmq

import (
	"fmt"
	"net"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Connect opens a connection and a channel. Caller must Close() the channel and connection when done.
// Topology declaration is deferred to later prompts; here we only prove the broker is reachable.
func Connect(url string) (*amqp.Connection, *amqp.Channel, error) {
	dialer := net.Dialer{Timeout: 10 * time.Second}
	cfg := amqp.Config{
		Properties: amqp.Table{"connection_name": "sportsbook"},
		Dial:       dialer.Dial,
	}
	conn, err := amqp.DialConfig(url, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("amqp dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("amqp channel: %w", err)
	}
	return conn, ch, nil
}
