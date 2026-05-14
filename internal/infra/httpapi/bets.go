package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"learning.local/sportsbook/internal/app/bets"
	mysqlstore "learning.local/sportsbook/internal/infra/mysql"
)

type placeBetRequest struct {
	UserID       string  `json:"user_id"`
	StakeAmount  string  `json:"stake_amount"`
	Currency     string  `json:"currency"`
	SelectionID  string  `json:"selection_id"`
	MarketID     string  `json:"market_id"`
	Odds         float64 `json:"odds"`
	OddsVersion  int     `json:"odds_version"`
}

type oddsResponse struct {
	MarketID    string `json:"market_id"`
	OddsVersion int    `json:"odds_version"`
	Status      string `json:"status"`
}

// HandlePlaceBet handles POST /bets (stdlib net/http only).
func HandlePlaceBet(db *sql.DB, log *slog.Logger, oddsServiceURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var req placeBetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}

		// 1. LIVE VALIDATION against Odds Service
		if oddsServiceURL != "" {
			url := fmt.Sprintf("%s/v1/markets/%s/odds", strings.TrimSuffix(oddsServiceURL, "/"), req.MarketID)
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()

			hreq, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			resp, err := http.DefaultClient.Do(hreq)
			if err != nil {
				log.Error("odds service unavailable", "err", err)
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "odds service unavailable"})
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var or oddsResponse
				if err := json.NewDecoder(resp.Body).Decode(&or); err == nil {
					if strings.ToUpper(or.Status) != "OPEN" {
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": "market is suspended"})
						return
					}
					if or.OddsVersion != req.OddsVersion {
						writeJSON(w, http.StatusBadRequest, map[string]string{
							"error":            "odds drift detected",
							"current_version": fmt.Sprint(or.OddsVersion),
						})
						return
					}
				}
			} else if resp.StatusCode == http.StatusNotFound {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "market not found"})
				return
			}
		}

		causation := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		cmd := bets.PlaceBetCommand{
			UserID:       strings.TrimSpace(req.UserID),
			StakeAmount:  strings.TrimSpace(req.StakeAmount),
			Currency:     strings.TrimSpace(req.Currency),
			SelectionID:  strings.TrimSpace(req.SelectionID),
			MarketID:     strings.TrimSpace(req.MarketID),
			Odds:         req.Odds,
			OddsVersion:  req.OddsVersion,
			CausationID:  causation,
		}

		res, err := bets.PlaceBet(r.Context(), db, log, cmd)
		if err != nil {
			if errors.Is(err, bets.ErrValidation) {
				log.Info("place bet validation", "err", err)
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if mysqlstore.IsFKViolation(err) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown user_id (run scripts/dev-seed.sql or insert users first)"})
				return
			}
			log.Error("place bet", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}

		log.Info("bet placed", "bet_id", res.BetID, "correlation_id", res.CorrelationID)
		writeJSON(w, http.StatusCreated, map[string]string{
			"bet_id":         res.BetID,
			"correlation_id": res.CorrelationID,
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
