package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"learning.local/sportsbook/internal/app/settlement"
	"learning.local/sportsbook/internal/config"
	mysqlstore "learning.local/sportsbook/internal/infra/mysql"
	"learning.local/sportsbook/internal/migrate"
	"learning.local/sportsbook/internal/pkg/tracing"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	db, err := mysqlstore.Open(ctx, cfg.MySQLDSN)
	if err != nil {
		log.Error("mysql connect", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := migrate.Apply(ctx, db); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	// Initialize Repositories
	betRepo := &mysqlstore.MysqlBetRepository{DB: db}
	outboxRepo := &mysqlstore.MysqlOutboxRepository{}

	// Initialize Service
	settlementService := settlement.NewService(db, betRepo, outboxRepo, log)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "settlement-service"})
	})
	settlement.RegisterHTTP(mux, settlementService)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           tracing.Middleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("http listening", "addr", cfg.HTTPAddr, "service", "settlement-service")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown", "err", err)
	}
	log.Info("bye")
}
