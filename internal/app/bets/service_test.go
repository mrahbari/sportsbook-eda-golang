package bets

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"learning.local/sportsbook/internal/domain"
)

type mockBetRepo struct {
	saveFn func(ctx context.Context, tx *sql.Tx, bet *domain.Bet) error
}

func (m *mockBetRepo) Save(ctx context.Context, tx *sql.Tx, bet *domain.Bet) error {
	return m.saveFn(ctx, tx, bet)
}
func (m *mockBetRepo) GetByID(ctx context.Context, id string) (*domain.Bet, error) { return nil, nil }
func (m *mockBetRepo) UpdateStatus(ctx context.Context, tx *sql.Tx, id string, status string) error {
	return nil
}
func (m *mockBetRepo) UpdateStatusSelective(ctx context.Context, tx *sql.Tx, id string, newStatus string, expectedStatus string) (int64, error) {
	return 0, nil
}
func (m *mockBetRepo) Settle(ctx context.Context, tx *sql.Tx, id string, userID string, status string) (int64, error) {
	return 0, nil
}

type mockOutboxRepo struct {
	saveFn func(ctx context.Context, tx *sql.Tx, event *domain.OutboxEvent) error
}

func (m *mockOutboxRepo) Save(ctx context.Context, tx *sql.Tx, event *domain.OutboxEvent) error {
	return m.saveFn(ctx, tx, event)
}

func TestService_PlaceBet_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()

	betRepo := &mockBetRepo{
		saveFn: func(ctx context.Context, tx *sql.Tx, bet *domain.Bet) error {
			return nil
		},
	}
	outboxRepo := &mockOutboxRepo{
		saveFn: func(ctx context.Context, tx *sql.Tx, event *domain.OutboxEvent) error {
			return nil
		},
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	svc := NewService(db, betRepo, outboxRepo, nil, log)

	cmd := PlaceBetCommand{
		UserID:      "user-1",
		StakeAmount: "10.00",
		Currency:    "USD",
		SelectionID: "sel-1",
		MarketID:    "mkt-1",
		Odds:        2.0,
		OddsVersion: 1,
	}

	res, err := svc.PlaceBet(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.BetID == "" {
		t.Error("expected betID")
	}
}
