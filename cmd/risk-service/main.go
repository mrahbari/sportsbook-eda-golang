package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"learning.local/sportsbook/internal/app/risk"
	"learning.local/sportsbook/internal/cmdutil"
	"learning.local/sportsbook/internal/config"
	mysqlstore "learning.local/sportsbook/internal/infra/mysql"
	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
	"learning.local/sportsbook/internal/migrate"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("risk-service starting")

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

	// Initialize Repositories
	processedRepo := &mysqlstore.MysqlProcessedEventRepository{}

	// Initialize Service
	riskService := risk.NewService(db, processedRepo, log)

	tag := cmdutil.ConsumerTag("risk")
	var inflight sync.WaitGroup
	if err := risk.Run(ctx, riskService, ch, tag, &inflight); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("risk consumer exited", "err", err)
	}
	log.Info("bye")
}
