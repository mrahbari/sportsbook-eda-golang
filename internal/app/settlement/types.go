package settlement

import "errors"

var ErrBetNotOpen = errors.New("bet not open or not found")

// SettleCommand is the input for the settlement usecase.
type SettleCommand struct {
	BetID         string
	UserID        string
	Payout        string
	Currency      string
	Outcome       string
	CorrelationID string
}
