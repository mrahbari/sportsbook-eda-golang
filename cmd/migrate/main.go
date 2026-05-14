package main

import (
	"context"
	"log/slog"
	"os"
	"strings"

	mysqlstore "learning.local/sportsbook/internal/infra/mysql"
	"learning.local/sportsbook/internal/migrate"
)

func main() {
	ctx := context.Background()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		log.Error("MYSQL_DSN is required")
		os.Exit(1)
	}

	db, err := mysqlstore.Open(ctx, dsn)
	if err != nil {
		log.Error("mysql", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := migrate.Apply(ctx, db); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}
	log.Info("migrations applied OK", "dsn_tail", redactDSN(dsn))
}

func redactDSN(dsn string) string {
	if i := strings.LastIndex(dsn, "@"); i >= 0 {
		return "…" + dsn[i:]
	}
	return dsn
}
