package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds runtime settings loaded from the environment.
type Config struct {
	MySQLDSN       string
	RabbitMQURL    string
	HTTPAddr       string
	OddsServiceURL string // added for live validation
	DevMode        bool
	MetricsAddr    string
}

// Load reads required environment variables. Missing vars return an error.
func Load() (Config, error) {
	c := Config{
		MySQLDSN:       os.Getenv("MYSQL_DSN"),
		RabbitMQURL:    os.Getenv("RABBITMQ_URL"),
		HTTPAddr:       os.Getenv("HTTP_ADDR"),
		OddsServiceURL: os.Getenv("ODDS_SERVICE_URL"),
		DevMode:        os.Getenv("DEV_MODE") == "1" || os.Getenv("DEV_MODE") == "true",
		MetricsAddr:    strings.TrimSpace(os.Getenv("METRICS_ADDR")),
	}
	if c.MySQLDSN == "" {
		return Config{}, fmt.Errorf("MYSQL_DSN is required")
	}
	if c.RabbitMQURL == "" {
		return Config{}, fmt.Errorf("RABBITMQ_URL is required")
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = ":8080"
	}
	return c, nil
}

// GatewayConfig is for cmd/api-gateway (no database).
type GatewayConfig struct {
	HTTPAddr             string
	BetServiceURL        string
	OddsServiceURL       string
	SettlementServiceURL string // optional; if set, proxies POST /v1/settlements
	MetricsAddr          string
}

// LoadGateway reads env for the edge reverse-proxy.
func LoadGateway() (GatewayConfig, error) {
	g := GatewayConfig{
		HTTPAddr:             strings.TrimSpace(os.Getenv("HTTP_ADDR")),
		BetServiceURL:        strings.TrimSpace(os.Getenv("BET_SERVICE_URL")),
		OddsServiceURL:       strings.TrimSpace(os.Getenv("ODDS_SERVICE_URL")),
		SettlementServiceURL: strings.TrimSpace(os.Getenv("SETTLEMENT_SERVICE_URL")),
		MetricsAddr:          strings.TrimSpace(os.Getenv("METRICS_ADDR")),
	}
	if g.HTTPAddr == "" {
		g.HTTPAddr = ":8080"
	}
	if g.BetServiceURL == "" || g.OddsServiceURL == "" {
		return GatewayConfig{}, fmt.Errorf("BET_SERVICE_URL and ODDS_SERVICE_URL are required for api-gateway")
	}
	if g.SettlementServiceURL == "" {
		return GatewayConfig{}, fmt.Errorf("SETTLEMENT_SERVICE_URL is required for api-gateway (settlement HTTP behind gateway)")
	}
	return g, nil
}

// OddsHTTPConfig is for cmd/odds-service (no database, no broker).
type OddsHTTPConfig struct {
	HTTPAddr    string
	MetricsAddr string
}

// LoadOddsHTTP reads env for the odds read API.
func LoadOddsHTTP() (OddsHTTPConfig, error) {
	o := OddsHTTPConfig{
		HTTPAddr:    strings.TrimSpace(os.Getenv("HTTP_ADDR")),
		MetricsAddr: strings.TrimSpace(os.Getenv("METRICS_ADDR")),
	}
	if o.HTTPAddr == "" {
		o.HTTPAddr = ":8082"
	}
	return o, nil
}

// NotifyConfig is for cmd/notification-service (RabbitMQ only).
type NotifyConfig struct {
	RabbitMQURL string
}

// LoadNotify reads env for the notification consumer.
func LoadNotify() (NotifyConfig, error) {
	n := NotifyConfig{RabbitMQURL: strings.TrimSpace(os.Getenv("RABBITMQ_URL"))}
	if n.RabbitMQURL == "" {
		return NotifyConfig{}, fmt.Errorf("RABBITMQ_URL is required")
	}
	return n, nil
}
