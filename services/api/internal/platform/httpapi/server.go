// Package httpapi is what every module's HTTP adapter shares: the router
// with its health checks and middleware, the JSON error shape and uploads.
package httpapi

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

// Routes is a module's set of endpoints, mounted by NewHandler.
type Routes interface {
	Register(mux *http.ServeMux)
}

// Deps holds the collaborators the router needs.
type Deps struct {
	DB     Pinger
	Logger *slog.Logger
	// Modules are the endpoints of each module (library, importing, content).
	Modules []Routes
}

// NewHandler returns the API router wrapped in the common middleware: the
// health checks, every module's endpoints and a JSON 404 for the rest.
func NewHandler(deps Deps) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.DiscardHandler)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /readyz", handleReadyz(deps.DB, deps.Logger))
	for _, m := range deps.Modules {
		m.Register(mux)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, http.StatusNotFound, "not_found", "route not found")
	})

	return withRecovery(deps.Logger, withRequestLog(deps.Logger, mux))
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleReadyz(db Pinger, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			logger.WarnContext(ctx, "readiness check failed", slog.String("error", err.Error()))
			WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
