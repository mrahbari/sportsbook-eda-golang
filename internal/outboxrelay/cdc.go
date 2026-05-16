package outboxrelay

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-sql-driver/mysql"
	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
	"learning.local/sportsbook/internal/pkg/tracing"
)

type CDCRelay struct {
	DSN            string
	DB             *sql.DB
	Pub            *rmq.Publisher
	Log            *slog.Logger
	Canal          *canal.Canal
	WritePublished bool
}

func (c *CDCRelay) Run(ctx context.Context) error {
	c.Log.Info("starting CDC outbox relay (binlog listener)", "write_published_at", c.WritePublished)

	mysqlCfg, err := mysql.ParseDSN(c.DSN)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}

	cfg := canal.NewDefaultConfig()
	cfg.Addr = mysqlCfg.Addr
	cfg.User = mysqlCfg.User
	cfg.Password = mysqlCfg.Passwd
	cfg.Dump.ExecutionPath = ""
	cfg.IncludeTableRegex = []string{fmt.Sprintf("%s\\.outbox_events", mysqlCfg.DBName)}

	can, err := canal.NewCanal(cfg)
	if err != nil {
		return fmt.Errorf("canal create: %w", err)
	}

	c.Canal = can
	can.SetEventHandler(&outboxEventHandler{
		db:             c.DB,
		pub:            c.Pub,
		log:            c.Log,
		writePublished: c.WritePublished,
	})

	// Start canal in background
	go func() {
		if err := can.Run(); err != nil {
			c.Log.Error("canal run error", "err", err)
		}
	}()

	<-ctx.Done()
	c.Log.Info("stopping CDC outbox relay")
	can.Close()
	return nil
}

type outboxEventHandler struct {
	canal.DummyEventHandler
	db             *sql.DB
	pub            *rmq.Publisher
	log            *slog.Logger
	writePublished bool
}

func (h *outboxEventHandler) OnRow(e *canal.RowsEvent) error {
	if e.Action != canal.InsertAction {
		return nil
	}

	// outbox_events schema:
	// id (0), event_type (1), routing_key (2), payload_json (3), created_at (4), published_at (5), aggregate_id (6)
	
	for _, row := range e.Rows {
		id, _ := row[0].(int64)
		routingKey, ok := row[2].(string)
		if !ok {
			h.log.Warn("CDC: routing_key not a string", "val", row[2])
			continue
		}
		
		var payload []byte
		switch v := row[3].(type) {
		case []byte:
			payload = v
		case string:
			payload = []byte(v)
		default:
			h.log.Warn("CDC: payload_json unexpected type", "type", fmt.Sprintf("%T", v))
			continue
		}

		h.log.Debug("CDC: publishing event", "routing_key", routingKey, "outbox_id", id)
		
		// 5 second timeout for publishing
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		// Maintain trace
		var envelope struct {
			Metadata struct {
				CorrelationID string `json:"correlation_id"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(payload, &envelope); err == nil && envelope.Metadata.CorrelationID != "" {
			ctx = tracing.NewContext(ctx, envelope.Metadata.CorrelationID)
		}

		if err := h.pub.PublishJSON(ctx, rmq.ExchangeBettingTopic, routingKey, payload); err != nil {
			h.log.Error("CDC: publish failed", "err", err, "routing_key", routingKey)
			cancel()
			return err
		}

		if h.writePublished && h.db != nil {
			if _, err := h.db.ExecContext(ctx, `UPDATE outbox_events SET published_at = CURRENT_TIMESTAMP(6) WHERE id = ?`, id); err != nil {
				h.log.Error("CDC: write-published-at failed", "err", err, "outbox_id", id)
				// We don't return error here to avoid blocking the relay if only the write fails
			}
		}

		cancel()
		h.log.Info("CDC: outbox event published", "routing_key", routingKey, "outbox_id", id)
	}

	return nil
}

func (h *outboxEventHandler) String() string {
	return "OutboxEventHandler"
}
