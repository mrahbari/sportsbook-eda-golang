package domain

import (
	"context"
	"database/sql"
)

// BetRepository defines the data access contract for Bets.
type BetRepository interface {
	Save(ctx context.Context, tx *sql.Tx, bet *Bet) error
	GetByID(ctx context.Context, id string) (*Bet, error)
	UpdateStatus(ctx context.Context, tx *sql.Tx, id string, status string) error
	UpdateStatusSelective(ctx context.Context, tx *sql.Tx, id string, newStatus string, expectedStatus string) (int64, error)
	Settle(ctx context.Context, tx *sql.Tx, id string, userID string, status string) (int64, error)
}

// OutboxRepository defines the data access contract for the Transactional Outbox.
type OutboxRepository interface {
	Save(ctx context.Context, tx *sql.Tx, event *OutboxEvent) error
}

// WalletRepository defines the data access contract for Wallets.
type WalletRepository interface {
	GetByUserID(ctx context.Context, tx *sql.Tx, userID string, lock bool) (*Wallet, error)
	UpdateBalance(ctx context.Context, tx *sql.Tx, userID string, amount string, version int) error
}

// ProcessedEventRepository handles idempotency checks.
type ProcessedEventRepository interface {
	IsProcessed(ctx context.Context, tx *sql.Tx, eventID string, consumerName string) (bool, error)
	MarkProcessed(ctx context.Context, tx *sql.Tx, eventID string, consumerName string) error
}

// OddsRepository defines the data access contract for Market Odds.
type OddsRepository interface {
	Save(ctx context.Context, odds *MarketOdds) error
	GetByID(ctx context.Context, marketID string) (*MarketOdds, error)
	ListAll(ctx context.Context) ([]MarketOdds, error)
}

// OddsCache defines the caching contract for Market Odds.
type OddsCache interface {
	Set(ctx context.Context, odds *MarketOdds) error
	Get(ctx context.Context, marketID string) (*MarketOdds, error)
}
