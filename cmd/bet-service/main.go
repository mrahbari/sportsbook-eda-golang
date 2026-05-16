package main

import (
	"context"
	"encoding/json"
	"expvar"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"learning.local/sportsbook/internal/app/bets"
	"learning.local/sportsbook/internal/config"
	"learning.local/sportsbook/internal/infra/httpapi"
	mysqlstore "learning.local/sportsbook/internal/infra/mysql"
	rmq "learning.local/sportsbook/internal/infra/rabbitmq"
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
	log.Info("migrations applied")

	conn, ch, err := rmq.Connect(cfg.RabbitMQURL)
	if err != nil {
		log.Error("rabbitmq", "err", err)
		os.Exit(1)
	}
	defer conn.Close()
	defer ch.Close()
	log.Info("rabbitmq connection and channel OK")

	if err := rmq.DeclareBettingTopology(ch); err != nil {
		log.Error("rabbitmq topology", "err", err)
		os.Exit(1)
	}
	log.Info("rabbitmq topology declared")

	pub := rmq.NewPublisher(ch)

	// Initialize Repositories
	betRepo := &mysqlstore.MysqlBetRepository{DB: db}
	outboxRepo := &mysqlstore.MysqlOutboxRepository{}
	processedRepo := &mysqlstore.MysqlProcessedEventRepository{}

	// Initialize Service
	betService := bets.NewService(db, betRepo, outboxRepo, processedRepo, log)

	var metricsSrv *http.Server
	if cfg.MetricsAddr != "" {
		mx := http.NewServeMux()
		mx.Handle("/debug/vars", expvar.Handler())
		metricsSrv = &http.Server{
			Addr:              cfg.MetricsAddr,
			Handler:           mx,
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			log.Info("metrics listening", "addr", cfg.MetricsAddr)
			if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error("metrics server", "err", err)
			}
		}()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "bet-service"})
	})
	mux.HandleFunc("/bets", httpapi.HandlePlaceBet(betService, cfg.OddsServiceURL))

	if cfg.DevMode {
		log.Warn("DEV_MODE enabled: POST /dev/publish-test-bet-placed is exposed")
		mux.HandleFunc("/dev/publish-test-bet-placed", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var body json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			pctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			if err := pub.PublishJSONDefault(pctx, "bet.placed.v1", body); err != nil {
				log.Error("dev publish", "err", err)
				http.Error(w, "publish failed", http.StatusBadGateway)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		})
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           tracing.Middleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("http listening", "addr", cfg.HTTPAddr, "service", "bet-service")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if metricsSrv != nil {
		_ = metricsSrv.Shutdown(shutdownCtx)
	}
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown", "err", err)
	}
	log.Info("bye")
}
