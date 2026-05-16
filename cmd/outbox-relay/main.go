package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"learning.local/sportsbook/internal/config"
	mysqlstore "learning.local/sportsbook/internal/infra/mysql"
	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
	"learning.local/sportsbook/internal/migrate"
	"learning.local/sportsbook/internal/outboxrelay"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("outbox-relay starting")

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	db, err := mysqlstore.Open(ctx, cfg.MySQLDSN)
	if err != nil {
		log.Error("mysql", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := migrate.Apply(ctx, db); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	conn, ch, err := rmq.Connect(cfg.RabbitMQURL)
	if err != nil {
		log.Error("rabbitmq", "err", err)
		os.Exit(1)
	}
	defer conn.Close()
	defer ch.Close()

	if err := rmq.DeclareBettingTopology(ch); err != nil {
		log.Error("rabbitmq topology", "err", err)
		os.Exit(1)
	}

	pub := rmq.NewPublisher(ch)

	if os.Getenv("USE_CDC") == "1" {
		cdcDSN := cfg.MySQLCDCDSN
		if cdcDSN == "" {
			cdcDSN = cfg.MySQLDSN
		}
		cdc := &outboxrelay.CDCRelay{
			DSN:            cdcDSN,
			DB:             db,
			Pub:            pub,
			Log:            log.With("component", "cdc_relay"),
			WritePublished: cfg.CDCWritePublishedAt,
		}
		if err := cdc.Run(ctx); err != nil {
			log.Error("cdc relay", "err", err)
			os.Exit(1)
		}
	} else {
		relay := &outboxrelay.Relay{
			DB:  db,
			Pub: pub,
			Log: log.With("component", "outbox_relay"),
		}
		relay.Run(ctx)
	}

	log.Info("bye")
}
