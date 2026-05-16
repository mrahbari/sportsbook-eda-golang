package main

import (
	"context"
	"expvar"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"learning.local/sportsbook/internal/app/odds"
	"learning.local/sportsbook/internal/config"
	mysqlstore "learning.local/sportsbook/internal/infra/mysql"
	"learning.local/sportsbook/internal/infra/redis"
	"learning.local/sportsbook/internal/pkg/tracing"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.LoadOddsHTTP()
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

	var rdb *redis.Client
	if cfg.RedisAddr != "" {
		rdb = redis.NewClient(&redis.Options{
			Addr: cfg.RedisAddr,
		})
		if err := rdb.Ping(ctx).Err(); err != nil {
			log.Error("redis connect", "err", err)
			os.Exit(1)
		}
		defer rdb.Close()
	}

	// Initialize Repositories
	oddsRepo := &mysqlstore.MysqlOddsRepository{DB: db}
	var oddsCache *redisstore.RedisOddsCache
	if rdb != nil {
		oddsCache = &redisstore.RedisOddsCache{Client: rdb}
	}

	// Initialize Service
	oddsService := odds.NewService(oddsRepo, oddsCache, log)

	if err := oddsService.Seed(ctx); err != nil {
		log.Error("seeding", "err", err)
	}

	handler := &odds.Handler{
		Service: oddsService,
		Log:     log,
	}

	mux := http.NewServeMux()
	handler.RegisterHTTP(mux)

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

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           tracing.Middleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("http listening", "addr", cfg.HTTPAddr, "service", "odds-service")
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
