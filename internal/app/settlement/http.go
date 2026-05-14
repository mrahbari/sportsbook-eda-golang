package settlement

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"learning.local/sportsbook/internal/domain"
	"learning.local/sportsbook/internal/pkg/uuid"
)

type settleRequest struct {
	BetID          string `json:"bet_id"`
	UserID         string `json:"user_id"`
	Payout         string `json:"payout"`
	Currency       string `json:"currency"`
	Outcome        string `json:"outcome"`
	CorrelationID  string `json:"correlation_id"`
}

// RegisterHTTP registers POST /v1/settlements (internal tutorial API).
func RegisterHTTP(mux *http.ServeMux, db *sql.DB, log *slog.Logger) {
	mux.HandleFunc("/v1/settlements", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var req settleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		req.BetID = strings.TrimSpace(req.BetID)
		req.UserID = strings.TrimSpace(req.UserID)
		if req.BetID == "" || req.UserID == "" {
			http.Error(w, `{"error":"bet_id and user_id required"}`, http.StatusBadRequest)
			return
		}
		ctx := r.Context()
		corr := strings.TrimSpace(req.CorrelationID)
		if corr == "" {
			var err error
			corr, err = uuid.New()
			if err != nil {
				http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
				return
			}
		}

		evID, err := uuid.New()
		if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
			return
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback()
			}
		}()

		res, err := tx.ExecContext(ctx, `
UPDATE bets SET status = ?, updated_at = CURRENT_TIMESTAMP(6)
WHERE id = ? AND user_id = ? AND status = ?`,
			domain.BetStatusSettled, req.BetID, req.UserID, domain.BetStatusOpen,
		)
		if err != nil {
			log.Error("settlement update bet", "err", err)
			http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
			return
		}
		ra, _ := res.RowsAffected()
		if ra == 0 {
			http.Error(w, `{"error":"bet not open or not found"}`, http.StatusConflict)
			return
		}

		env := domain.BetSettledV1{
			Metadata: domain.EventMetadata{
				EventID:        evID,
				CorrelationID:  corr,
				CausationID:    req.BetID,
				Timestamp:      domain.RFC3339NanoUTC(time.Now().UTC()),
				Version:        "v1",
				Producer:       domain.ProducerSettlementService,
			},
			Payload: domain.BetSettledV1Payload{
				BetID:    req.BetID,
				UserID:   req.UserID,
				Payout:   strings.TrimSpace(req.Payout),
				Currency: strings.ToUpper(strings.TrimSpace(req.Currency)),
				Outcome:  strings.ToUpper(strings.TrimSpace(req.Outcome)),
			},
		}
		if env.Payload.Currency == "" {
			env.Payload.Currency = "USD"
		}
		raw, err := json.Marshal(env)
		if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO outbox_events (event_type, routing_key, payload_json, aggregate_id)
VALUES (?, ?, ?, ?)`,
			domain.EventTypeBetSettledV1, domain.RoutingKeyBetSettledV1, string(raw), req.BetID,
		); err != nil {
			log.Error("settlement outbox", "err", err)
			http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			log.Error("settlement commit", "err", err)
			http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
			return
		}
		committed = true

		log.Info("settlement recorded", "bet_id", req.BetID, "correlation_id", corr, "event_id", evID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":   "accepted",
			"event_id": evID,
		})
	})
}
