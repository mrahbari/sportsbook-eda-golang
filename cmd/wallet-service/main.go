package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"learning.local/sportsbook/internal/app/wallet"
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
	log.Info("wallet-service starting")

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

	chReserve, err := conn.Channel()
	if err != nil {
		log.Error("rabbitmq reserve channel", "err", err)
		os.Exit(1)
	}
	chSettled, err := conn.Channel()
	if err != nil {
		log.Error("rabbitmq settled channel", "err", err)
		os.Exit(1)
	}
	defer chReserve.Close()
	defer chSettled.Close()

	if err := rmq.DeclareBettingTopology(chReserve); err != nil {
		log.Error("rabbitmq topology (reserve)", "err", err)
		os.Exit(1)
	}
	if err := rmq.DeclareBettingTopology(chSettled); err != nil {
		log.Error("rabbitmq topology (settled)", "err", err)
		os.Exit(1)
	}

	tagR := cmdutil.ConsumerTag("wallet-reserve")
	tagS := cmdutil.ConsumerTag("wallet-settled")
	var inflightR, inflightS sync.WaitGroup

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := wallet.RunBetPlacedConsumer(ctx, db, chReserve, log, tagR, &inflightR); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("wallet reserve consumer exited", "err", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := wallet.RunBetSettledConsumer(ctx, db, chSettled, log, tagS, &inflightS); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("wallet settled consumer exited", "err", err)
		}
	}()

	<-ctx.Done()
	log.Info("shutdown signal", "reason", ctx.Err())
	wg.Wait()
	log.Info("bye")
}
