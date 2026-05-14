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

	betProxy := httputil.NewSingleHostReverseProxy(betURL)
	betProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Error("proxy error: bet-service", "err", err, "url", r.URL.Path)
		w.WriteHeader(http.StatusBadGateway)
	}

	oddsProxy := httputil.NewSingleHostReverseProxy(oddsURL)
	oddsProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Error("proxy error: odds-service", "err", err, "url", r.URL.Path)
		w.WriteHeader(http.StatusBadGateway)
	}

	settleURL, err := url.Parse(gc.SettlementServiceURL)
	if err != nil {
		log.Error("SETTLEMENT_SERVICE_URL", "err", err)
		os.Exit(1)
	}
	settleProxy := httputil.NewSingleHostReverseProxy(settleURL)
	settleProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Error("proxy error: settlement-service", "err", err, "url", r.URL.Path)
		w.WriteHeader(http.StatusBadGateway)
	}

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
		Handler:           mux,
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
