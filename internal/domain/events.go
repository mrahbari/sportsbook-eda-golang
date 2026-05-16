package domain

import "time"

// EventMetadata is the envelope header for all domain events (design §5).
type EventMetadata struct {
	EventID        string `json:"event_id"`
	CorrelationID  string `json:"correlation_id"`
	CausationID    string `json:"causation_id"`
	Timestamp      string `json:"timestamp"`
	Version        string `json:"version"`
	Producer       string `json:"producer"`
}

// BetPlacedV1Payload is the business payload for bet.placed.v1.
// Amounts use decimal strings in JSON to avoid float drift (educational choice).
type BetPlacedV1Payload struct {
	BetID       string  `json:"bet_id"`
	UserID      string  `json:"user_id"`
	Amount      string  `json:"amount"`
	Currency    string  `json:"currency"`
	SelectionID string  `json:"selection_id"`
	MarketID    string  `json:"market_id"`
	Odds        float64 `json:"odds"`
	OddsVersion int     `json:"odds_version"`
}

// BetPlacedV1 is the full envelope published to the broker.
type BetPlacedV1 struct {
	Metadata EventMetadata       `json:"metadata"`
	Payload  BetPlacedV1Payload `json:"payload"`
}

// BetStatus values stored in bets.status.
const (
	BetStatusPendingReserve = "PENDING_RESERVE"
	BetStatusOpen           = "OPEN"
	BetStatusRejected       = "REJECTED"
	BetStatusSettled        = "SETTLED"
)

// WalletReservedV1 follows design §5 (wallet.reserved.v1).
type WalletReservedV1 struct {
	Metadata EventMetadata           `json:"metadata"`
	Payload  WalletReservedV1Payload `json:"payload"`
}

type WalletReservedV1Payload struct {
	ReservationID string `json:"reservation_id"`
	UserID        string `json:"user_id"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	BetID         string `json:"bet_id"`
}

// BetAcceptedV1 is emitted after the bet moves to OPEN (optional downstream).
type BetAcceptedV1 struct {
	Metadata EventMetadata          `json:"metadata"`
	Payload  BetAcceptedV1Payload   `json:"payload"`
}

type BetAcceptedV1Payload struct {
	BetID string `json:"bet_id"`
}

// BetSettledV1 is emitted by settlement (design §3 payout leg).
type BetSettledV1 struct {
	Metadata EventMetadata         `json:"metadata"`
	Payload  BetSettledV1Payload   `json:"payload"`
}

type BetSettledV1Payload struct {
	BetID    string `json:"bet_id"`
	UserID   string `json:"user_id"`
	Payout   string `json:"payout"`
	Currency string `json:"currency"`
	Outcome  string `json:"outcome"` // WIN, LOSE, VOID
}

// WalletCreditedV1 is emitted after a payout is added back to a wallet.
type WalletCreditedV1 struct {
	Metadata EventMetadata           `json:"metadata"`
	Payload  WalletCreditedV1Payload `json:"payload"`
}

type WalletCreditedV1Payload struct {
	UserID   string `json:"user_id"`
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	BetID    string `json:"bet_id"`
	Reason   string `json:"reason"`
}

const (
	EventTypeBetPlacedV1   = "bet.placed.v1"
	RoutingKeyBetPlacedV1  = "bet.placed.v1"
	ProducerBetService     = "bet-service"

	EventTypeWalletReservedV1  = "wallet.reserved.v1"
	RoutingKeyWalletReservedV1 = "wallet.reserved.v1"
	ProducerWalletService      = "wallet-service"

	EventTypeBetAcceptedV1  = "bet.accepted.v1"
	RoutingKeyBetAcceptedV1 = "bet.accepted.v1"

	EventTypeBetSettledV1       = "bet.settled.v1"
	RoutingKeyBetSettledV1     = "bet.settled.v1"
	ProducerSettlementService  = "settlement-service"

	EventTypeWalletCreditedV1  = "wallet.credited.v1"
	RoutingKeyWalletCreditedV1 = "wallet.credited.v1"
)

// RFC3339NanoUTC formats t in UTC for event metadata.
func RFC3339NanoUTC(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
