package settlement

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type settleRequest struct {
	BetID         string `json:"bet_id"`
	UserID        string `json:"user_id"`
	Payout        string `json:"payout"`
	Currency      string `json:"currency"`
	Outcome       string `json:"outcome"`
	CorrelationID string `json:"correlation_id"`
}

// RegisterHTTP registers POST /v1/settlements (internal tutorial API).
func RegisterHTTP(mux *http.ServeMux, svc *Service) {
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

		cmd := SettleCommand{
			BetID:         strings.TrimSpace(req.BetID),
			UserID:        strings.TrimSpace(req.UserID),
			Payout:        strings.TrimSpace(req.Payout),
			Currency:      strings.TrimSpace(req.Currency),
			Outcome:       strings.TrimSpace(req.Outcome),
			CorrelationID: strings.TrimSpace(req.CorrelationID),
		}

		if cmd.BetID == "" || cmd.UserID == "" {
			http.Error(w, `{"error":"bet_id and user_id required"}`, http.StatusBadRequest)
			return
		}

		eventID, err := svc.Settle(r.Context(), cmd)
		if err != nil {
			if errors.Is(err, ErrBetNotOpen) {
				http.Error(w, `{"error":"bet not open or not found"}`, http.StatusConflict)
				return
			}
			svc.Log.Error("settle failed", "err", err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}

		svc.Log.Info("settlement recorded", "bet_id", cmd.BetID, "event_id", eventID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":   "accepted",
			"event_id": eventID,
		})
	})
}
