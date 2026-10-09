// Package api berisi router, middleware, dan handler HTTP.
package api

import (
	"log/slog"
	"net/http"

	"ai-writer/internal/config"
	"ai-writer/internal/db"
)

// Deps adalah dependensi yang dibutuhkan handler.
type Deps struct {
	Config *config.Config
	DB     *db.DB
	Logger *slog.Logger
}

// NewRouter mendaftarkan semua route beserta middleware global.
func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /api/health", healthHandler(d.DB, d.Config.AnthropicModel, d.Logger))

	// Path /api/* yang tidak terdaftar dibalas JSON, bukan halaman 404 bawaan.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan.")
	})

	return chain(mux,
		requestID,
		logRequests(d.Logger),
		recoverPanic(d.Logger),
	)
}
