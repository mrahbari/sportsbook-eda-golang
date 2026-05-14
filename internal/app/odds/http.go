package odds

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

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

var (
	mu   sync.RWMutex
	data = map[string]MarketOdds{
		"mkt_demo_1": {
			MarketID:    "mkt_demo_1",
			OddsVersion: 99,
			Selections: []Selection{
				{SelectionID: "sel_demo_1", Price: 1.95},
			},
			Status: "OPEN",
		},
	}
)

// RegisterHTTP serves minimal read-only odds (tutorial substitute for Redis + provider feed).
func RegisterHTTP(mux *http.ServeMux) {
	mux.HandleFunc("/v1/markets/", func(w http.ResponseWriter, r *http.Request) {
		// path: /v1/markets/{id}/odds
		path := strings.TrimPrefix(r.URL.Path, "/v1/markets/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 || parts[1] != "odds" {
			http.Error(w, `{"error":"use /v1/markets/{id}/odds"}`, http.StatusNotFound)
			return
		}
		mktID := parts[0]
		if mktID == "" {
			http.Error(w, `{"error":"missing market id"}`, http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodGet:
			mu.RLock()
			m, ok := data[mktID]
			mu.RUnlock()

			if !ok {
				http.Error(w, `{"error":"market not found"}`, http.StatusNotFound)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(m)

		case http.MethodPost:
			var req MarketOdds
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
				return
			}
			req.MarketID = mktID

			mu.Lock()
			data[mktID] = req
			mu.Unlock()

			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "updated", "market_id": mktID})

		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"odds-service"}`))
	})
}
