package tracing

import (
	"context"
	"log/slog"
	"net/http"

	"learning.local/sportsbook/internal/pkg/uuid"
)

type contextKey string

const (
	correlationIDKey contextKey = "correlation_id"
	HeaderXCorrelationID        = "X-Correlation-ID"
)

// FromContext extracts the correlation ID from a context.
func FromContext(ctx context.Context) string {
	if v, ok := ctx.Value(correlationIDKey).(string); ok {
		return v
	}
	return ""
}

// NewContext adds a correlation ID to a context.
func NewContext(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationIDKey, correlationID)
}

// Middleware is an HTTP middleware that extracts or generates a correlation ID.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get(HeaderXCorrelationID)
		if corrID == "" {
			corrID, _ = uuid.New()
		}
		
		// Set header in response so client can track it
		w.Header().Set(HeaderXCorrelationID, corrID)

		ctx := NewContext(r.Context(), corrID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Log adds the correlation ID to the logger's attributes.
func Log(ctx context.Context, logger *slog.Logger) *slog.Logger {
	if id := FromContext(ctx); id != "" {
		return logger.With("correlation_id", id)
	}
	return logger
}
