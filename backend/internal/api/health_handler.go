package api

import (
	"context"
	"log/slog"
	"net/http"
)

type pinger interface {
	Ping(ctx context.Context) error
}

type healthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Model    string `json:"model"`
}

func healthHandler(db pinger, model string, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()); err != nil {
			logger.ErrorContext(r.Context(), "health: database tidak terjangkau", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, healthResponse{
				Status: "degraded", Database: "unreachable", Model: model,
			})
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Database: "ok", Model: model})
	}
}
