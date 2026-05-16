package odds

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"learning.local/sportsbook/internal/domain"
)

type Handler struct {
	Service *Service
	Log     *slog.Logger
}

func (h *Handler) RegisterHTTP(mux *http.ServeMux) {
	mux.HandleFunc("/v1/markets/", h.handleMarkets)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"odds-service"}`))
	})
}

func (h *Handler) handleMarkets(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/markets/odds" {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		h.listOdds(w, r)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/markets/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "odds" {
		http.Error(w, `{"error":"use /v1/markets/{id}/odds"}`, http.StatusNotFound)
		return
	}
	mktID := parts[0]

	switch r.Method {
	case http.MethodGet:
		h.getOdds(w, r, mktID)
	case http.MethodPost:
		h.updateOdds(w, r, mktID)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *Handler) listOdds(w http.ResponseWriter, r *http.Request) {
	list, err := h.Service.ListOdds(r.Context())
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (h *Handler) getOdds(w http.ResponseWriter, r *http.Request, mktID string) {
	m, hit, err := h.Service.GetOdds(r.Context(), mktID)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if hit {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	_ = json.NewEncoder(w).Encode(m)
}

func (h *Handler) updateOdds(w http.ResponseWriter, r *http.Request, mktID string) {
	var req domain.MarketOdds
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	req.MarketID = mktID

	if err := h.Service.UpdateOdds(r.Context(), &req); err != nil {
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "updated", "market_id": mktID})
}
