package bets

import (
	"errors"
)

// PlaceBetCommand is the application input for placing a bet.
type PlaceBetCommand struct {
	UserID      string
	StakeAmount string // decimal string, e.g. "10.50"
	Currency    string
	SelectionID string
	MarketID    string
	Odds        float64
	OddsVersion int
	CausationID string // optional; e.g. X-Request-Id from HTTP layer
}

// Result is the application output for a successful bet placement.
type Result struct {
	BetID         string
	CorrelationID string
}

var (
	ErrValidation      = errors.New("validation error")
	ErrMarketSuspended = errors.New("market is suspended")
	ErrOddsDrift       = errors.New("odds version mismatch (drift)")
)
