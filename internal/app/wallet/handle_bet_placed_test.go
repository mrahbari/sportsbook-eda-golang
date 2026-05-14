package wallet

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"learning.local/sportsbook/internal/domain"
)

func TestHandleBetPlaced_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %s", err)
	}
	defer db.Close()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	event := domain.BetPlacedV1{
		Metadata: domain.EventMetadata{
			EventID:       "evt-1",
			CorrelationID: "corr-1",
		},
		Payload: domain.BetPlacedV1Payload{
			BetID:    "bet-1",
			UserID:   "user-1",
			Amount:   "10.00",
			Currency: "USD",
		},
	}
	body, _ := json.Marshal(event)

	mock.ExpectBegin()
	// Idempotency check
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM processed_events").
		WithArgs(event.Metadata.EventID, ConsumerWalletReserve).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	
	// Load wallet
	mock.ExpectQuery("SELECT available_amount, currency, version FROM wallets").
		WithArgs(event.Payload.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"available_amount", "currency", "version"}).
			AddRow("100.00", "USD", 1))

	// Debit wallet
	mock.ExpectExec("UPDATE wallets").
		WithArgs("10.00", event.Payload.UserID, 1, "10.00").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Outbox insert
	mock.ExpectExec("INSERT INTO outbox_events").
		WithArgs(domain.EventTypeWalletReservedV1, domain.RoutingKeyWalletReservedV1, sqlmock.AnyArg(), event.Payload.BetID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Mark processed
	mock.ExpectExec("INSERT INTO processed_events").
		WithArgs(event.Metadata.EventID, ConsumerWalletReserve).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectCommit()

	err = HandleBetPlaced(context.Background(), db, body, log)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestHandleBetPlaced_InsufficientFunds(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %s", err)
	}
	defer db.Close()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	event := domain.BetPlacedV1{
		Metadata: domain.EventMetadata{EventID: "evt-1"},
		Payload: domain.BetPlacedV1Payload{
			BetID:    "bet-1",
			UserID:   "user-1",
			Amount:   "200.00", // More than available
			Currency: "USD",
		},
	}
	body, _ := json.Marshal(event)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM processed_events").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT available_amount, currency, version FROM wallets").
		WillReturnRows(sqlmock.NewRows([]string{"available_amount", "currency", "version"}).AddRow("100.00", "USD", 1))

	// Should reject bet
	mock.ExpectExec("UPDATE bets SET status = \\?, updated_at = CURRENT_TIMESTAMP\\(6\\) WHERE id = \\? AND status = \\?").
		WithArgs(domain.BetStatusRejected, event.Payload.BetID, domain.BetStatusPendingReserve).
		WillReturnResult(sqlmock.NewResult(1, 1))
	
	mock.ExpectExec("INSERT INTO processed_events").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = HandleBetPlaced(context.Background(), db, body, log)
	if err != nil {
		t.Errorf("expected no error (rejected is a business success), got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}
