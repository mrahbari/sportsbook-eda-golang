package bets

import (
	"fmt"
	"strconv"
	"strings"
)

func validate(cmd PlaceBetCommand) error {
	if strings.TrimSpace(cmd.UserID) == "" {
		return fmt.Errorf("user_id: %w", ErrValidation)
	}
	stake, err := strconv.ParseFloat(strings.TrimSpace(cmd.StakeAmount), 64)
	if err != nil || stake <= 0 {
		return fmt.Errorf("stake_amount must be a positive number: %w", ErrValidation)
	}
	if len(strings.TrimSpace(cmd.Currency)) != 3 {
		return fmt.Errorf("currency must be 3 letters: %w", ErrValidation)
	}
	if strings.TrimSpace(cmd.SelectionID) == "" || strings.TrimSpace(cmd.MarketID) == "" {
		return fmt.Errorf("selection_id and market_id: %w", ErrValidation)
	}
	if cmd.Odds <= 0 {
		return fmt.Errorf("odds must be positive: %w", ErrValidation)
	}
	if cmd.OddsVersion < 0 {
		return fmt.Errorf("odds_version: %w", ErrValidation)
	}
	return nil
}
