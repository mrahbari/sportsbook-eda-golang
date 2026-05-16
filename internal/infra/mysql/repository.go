package mysqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"learning.local/sportsbook/internal/domain"
)

type MysqlBetRepository struct {
	DB *sql.DB
}

func (r *MysqlBetRepository) Save(ctx context.Context, tx *sql.Tx, bet *domain.Bet) error {
	query := `
INSERT INTO bets (
  id, user_id, stake_amount, currency, selection_id, market_id, odds, odds_version, status, correlation_id
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  status = VALUES(status),
  updated_at = CURRENT_TIMESTAMP(6)`

	_, err := tx.ExecContext(ctx, query,
		bet.ID, bet.UserID, bet.StakeAmount, bet.Currency,
		bet.SelectionID, bet.MarketID, bet.Odds, bet.OddsVersion,
		bet.Status, bet.CorrelationID,
	)
	return err
}

func (r *MysqlBetRepository) GetByID(ctx context.Context, id string) (*domain.Bet, error) {
	var b domain.Bet
	err := r.DB.QueryRowContext(ctx, `
SELECT id, user_id, stake_amount, currency, selection_id, market_id, odds, odds_version, status, correlation_id
FROM bets WHERE id = ?`, id).Scan(
		&b.ID, &b.UserID, &b.StakeAmount, &b.Currency,
		&b.SelectionID, &b.MarketID, &b.Odds, &b.OddsVersion,
		&b.Status, &b.CorrelationID,
	)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *MysqlBetRepository) UpdateStatus(ctx context.Context, tx *sql.Tx, id string, status string) error {
	_, err := tx.ExecContext(ctx, `UPDATE bets SET status = ?, updated_at = CURRENT_TIMESTAMP(6) WHERE id = ?`, status, id)
	return err
}

func (r *MysqlBetRepository) UpdateStatusSelective(ctx context.Context, tx *sql.Tx, id string, newStatus string, expectedStatus string) (int64, error) {
	res, err := tx.ExecContext(ctx, `
UPDATE bets SET status = ?, updated_at = CURRENT_TIMESTAMP(6)
WHERE id = ? AND status = ?`,
		newStatus, id, expectedStatus,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *MysqlBetRepository) Settle(ctx context.Context, tx *sql.Tx, id string, userID string, status string) (int64, error) {
	res, err := tx.ExecContext(ctx, `
UPDATE bets SET status = ?, updated_at = CURRENT_TIMESTAMP(6)
WHERE id = ? AND user_id = ? AND status = ?`,
		domain.BetStatusSettled, id, userID, status,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type MysqlOutboxRepository struct{}

func (r *MysqlOutboxRepository) Save(ctx context.Context, tx *sql.Tx, event *domain.OutboxEvent) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO outbox_events (event_type, routing_key, payload_json, aggregate_id)
VALUES (?, ?, ?, ?)`,
		event.EventType, event.RoutingKey, event.PayloadJSON, event.AggregateID,
	)
	return err
}

type MysqlWalletRepository struct {
	DB *sql.DB
}

func (r *MysqlWalletRepository) GetByUserID(ctx context.Context, tx *sql.Tx, userID string, lock bool) (*domain.Wallet, error) {
	query := `SELECT user_id, available_amount, currency, version FROM wallets WHERE user_id = ?`
	if lock {
		query += " FOR UPDATE"
	}

	var w domain.Wallet
	err := tx.QueryRowContext(ctx, query, userID).Scan(
		&w.UserID, &w.AvailableAmount, &w.Currency, &w.Version,
	)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *MysqlWalletRepository) UpdateBalance(ctx context.Context, tx *sql.Tx, userID string, amount string, version int) error {
	// Note: amount can be positive (credit) or negative (debit) if we use this generally,
	// but here we follow the existing logic which expects a string for precise DECIMAL casting.
	res, err := tx.ExecContext(ctx, `
UPDATE wallets
SET available_amount = available_amount + CAST(? AS DECIMAL(19,4)),
    version = version + 1
WHERE user_id = ? AND version = ?`,
		amount, userID, version,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("wallet update version mismatch")
	}
	return nil
}

type MysqlProcessedEventRepository struct{}

func (r *MysqlProcessedEventRepository) IsProcessed(ctx context.Context, tx *sql.Tx, eventID string, consumerName string) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM processed_events WHERE event_id = ? AND consumer_name = ?`,
		eventID, consumerName).Scan(&count)
	return count > 0, err
}

func (r *MysqlProcessedEventRepository) MarkProcessed(ctx context.Context, tx *sql.Tx, eventID string, consumerName string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO processed_events (event_id, consumer_name) VALUES (?, ?)`,
		eventID, consumerName)
	return err
}

type MysqlOddsRepository struct {
	DB *sql.DB
}

func (r *MysqlOddsRepository) Save(ctx context.Context, odds *domain.MarketOdds) error {
	payload, err := json.Marshal(odds)
	if err != nil {
		return err
	}

	_, err = r.DB.ExecContext(ctx, `
INSERT INTO market_odds (market_id, odds_version, status, payload_json)
VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  odds_version = VALUES(odds_version),
  status = VALUES(status),
  payload_json = VALUES(payload_json)`,
		odds.MarketID, odds.OddsVersion, odds.Status, payload)
	return err
}

func (r *MysqlOddsRepository) GetByID(ctx context.Context, marketID string) (*domain.MarketOdds, error) {
	var payload []byte
	err := r.DB.QueryRowContext(ctx, "SELECT payload_json FROM market_odds WHERE market_id = ?", marketID).Scan(&payload)
	if err != nil {
		return nil, err
	}

	var m domain.MarketOdds
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *MysqlOddsRepository) ListAll(ctx context.Context) ([]domain.MarketOdds, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT payload_json FROM market_odds ORDER BY market_id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.MarketOdds
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			continue
		}
		var m domain.MarketOdds
		if err := json.Unmarshal(payload, &m); err == nil {
			list = append(list, m)
		}
	}
	return list, nil
}
