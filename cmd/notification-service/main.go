package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"learning.local/sportsbook/internal/app/notify"
	"learning.local/sportsbook/internal/cmdutil"
	"learning.local/sportsbook/internal/config"
	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("notification-service starting")

	cfg, err := config.LoadNotify()
	if err != nil {
		log.Error("config", "err", err)
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

	// Initialize Service
	notifyService := notify.NewService(log)

	tag := cmdutil.ConsumerTag("notify")
	var inflight sync.WaitGroup
	if err := notify.Run(ctx, notifyService, ch, tag, &inflight); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("notify consumer exited", "err", err)
	}
	log.Info("bye")
}
