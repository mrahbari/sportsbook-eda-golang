package domain

import "time"

// Bet represents the core betting entity.
type Bet struct {
	ID            string
	UserID        string
	StakeAmount   string
	Currency      string
	SelectionID   string
	MarketID      string
	Odds          float64
	OddsVersion   int
	Status        string
	CorrelationID string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// OutboxEvent represents an event staged for publishing.
type OutboxEvent struct {
	ID          int64
	EventType   string
	RoutingKey  string
	PayloadJSON string
	CreatedAt   time.Time
	PublishedAt *time.Time
	AggregateID string
}

// Wallet represents a user's balance.
type Wallet struct {
	UserID          string
	AvailableAmount string
	Currency        string
	Version         int
	UpdatedAt       time.Time
}

// MarketOdds represents the odds for a specific market.
type MarketOdds struct {
	MarketID    string      `json:"market_id"`
	OddsVersion int         `json:"odds_version"`
	Selections  []Selection `json:"selections"`
	Status      string      `json:"status"`
}

type Selection struct {
	SelectionID string  `json:"selection_id"`
	Price       float64 `json:"price"`
}
