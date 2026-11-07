// Package http is the driving adapter: it exposes the ReadVault use cases
// over a JSON HTTP API.
package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// readinessTimeout bounds the database check behind /readyz.
const readinessTimeout = 2 * time.Second

// Pinger reports whether a backing service is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps holds the collaborators the handlers need.
type Deps struct {
	DB     Pinger
	Logger *slog.Logger
}

// NewHandler returns the API router wrapped in the common middleware.
func NewHandler(deps Deps) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.DiscardHandler)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /readyz", handleReadyz(deps.DB, deps.Logger))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
	})

	return withRecovery(deps.Logger, withRequestLog(deps.Logger, mux))
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleReadyz(db Pinger, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			logger.WarnContext(ctx, "readiness check failed", slog.String("error", err.Error()))
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
