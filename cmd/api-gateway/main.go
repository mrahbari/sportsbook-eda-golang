package main

import (
	"context"
	"encoding/json"
	"expvar"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"learning.local/sportsbook/internal/config"
	"learning.local/sportsbook/internal/pkg/tracing"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	gc, err := config.LoadGateway()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	betURL, err := url.Parse(gc.BetServiceURL)
	if err != nil {
		log.Error("BET_SERVICE_URL", "err", err)
		os.Exit(1)
	}

	oddsURL, err := url.Parse(gc.OddsServiceURL)
	if err != nil {
		log.Error("ODDS_SERVICE_URL", "err", err)
		os.Exit(1)
	}

	proxyErrorHandler := func(serviceName string) func(http.ResponseWriter, *http.Request, error) {
		return func(w http.ResponseWriter, r *http.Request, err error) {
			log.Error("proxy error", "service", serviceName, "err", err, "url", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "upstream service unavailable",
				"service": serviceName,
				"reason":  err.Error(),
			})
		}
	}

	betProxy := httputil.NewSingleHostReverseProxy(betURL)
	betProxy.ErrorHandler = proxyErrorHandler("bet-service")

	oddsProxy := httputil.NewSingleHostReverseProxy(oddsURL)
	oddsProxy.ErrorHandler = proxyErrorHandler("odds-service")

	settleURL, err := url.Parse(gc.SettlementServiceURL)
	if err != nil {
		log.Error("SETTLEMENT_SERVICE_URL", "err", err)
		os.Exit(1)
	}

	settleProxy := httputil.NewSingleHostReverseProxy(settleURL)
	settleProxy.ErrorHandler = proxyErrorHandler("settlement-service")

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "api-gateway"})
	})

	mux.Handle("/v1/settlements", settleProxy)
	mux.Handle("/v1/markets/", oddsProxy)
	mux.Handle("/", betProxy)

	var metricsSrv *http.Server
	if gc.MetricsAddr != "" {
		mx := http.NewServeMux()
		mx.Handle("/debug/vars", expvar.Handler())
		metricsSrv = &http.Server{
			Addr:              gc.MetricsAddr,
			Handler:           mx,
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			log.Info("metrics listening", "addr", gc.MetricsAddr)
			if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error("metrics server", "err", err)
			}
		}()
	}

	srv := &http.Server{
		Addr:              gc.HTTPAddr,
		Handler:           tracing.Middleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("http listening", "addr", gc.HTTPAddr, "service", "api-gateway")
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
