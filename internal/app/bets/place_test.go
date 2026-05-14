package bets

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"learning.local/sportsbook/internal/domain"
)

func TestPlaceBet_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %s", err)
	}
	defer db.Close()

	cmd := PlaceBetCommand{
		UserID:      "user-1",
		StakeAmount: "10.00",
		Currency:    "USD",
		SelectionID: "sel-1",
		MarketID:    "mkt-1",
		Odds:        2.0,
		OddsVersion: 1,
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO bets").
		WithArgs(sqlmock.AnyArg(), cmd.UserID, cmd.StakeAmount, cmd.Currency, cmd.SelectionID, cmd.MarketID, cmd.Odds, cmd.OddsVersion, domain.BetStatusPendingReserve, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs(domain.EventTypeBetPlacedV1, domain.RoutingKeyBetPlacedV1, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	res, err := PlaceBet(context.Background(), db, log, cmd)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if res.BetID == "" {
		t.Error("expected BetID to be set")
	}
	if res.CorrelationID == "" {
		t.Error("expected CorrelationID to be set")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestPlaceBet_ValidationFailure(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	cmd := PlaceBetCommand{
		UserID: "", // Invalid
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	_, err := PlaceBet(context.Background(), db, log, cmd)
	if err == nil {
		t.Error("expected validation error, got nil")
	}
}
