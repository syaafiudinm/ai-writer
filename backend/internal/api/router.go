// Package api berisi router, middleware, dan handler HTTP.
package api

import (
	"context"
	"log/slog"
	"net/http"

	"ai-writer/internal/auth"
	"ai-writer/internal/config"
	"ai-writer/internal/db"
	"ai-writer/internal/gemini"
	"ai-writer/internal/ratelimit"
)

// Deps adalah dependensi yang dibutuhkan handler.
type Deps struct {
	Config          *config.Config
	DB              *db.DB
	Logger          *slog.Logger
	Auth            *auth.Service
	LoginLimiter    *ratelimit.Limiter
	GenerateLimiter *ratelimit.Limiter
	Streamer        gemini.Streamer
}

// NewRouter mendaftarkan semua route beserta middleware global.
func NewRouter(d Deps) http.Handler {
	mw := &auth.Middleware{
		Service:      d.Auth,
		CookieSecure: d.Config.CookieSecure,
		WriteError:   writeError,
		OnUser:       func(ctx context.Context, u *db.User) { setLogUserID(ctx, u.ID) },
	}
	requireUser := mw.RequireAuth
	requireAdmin := func(h http.Handler) http.Handler {
		return mw.RequireAuth(mw.RequireRole(db.RoleAdmin)(h))
	}

	ah := &authHandler{
		svc: d.Auth, limiter: d.LoginLimiter, cookieSecure: d.Config.CookieSecure, logger: d.Logger,
	}

	gh := &generateHandler{
		streamer:      d.Streamer,
		model:         d.Config.GeminiModel,
		thinkingLevel: d.Config.GeminiThinkingLevel,
		timeout:       d.Config.GenerateTimeout,
		limiter:       d.GenerateLimiter,
		db:            d.DB,
		logger:        d.Logger,
	}

	mux := http.NewServeMux()
	mux.Handle("GET /api/health", healthHandler(d.DB, d.Config.GeminiModel, d.Logger))

	mux.HandleFunc("POST /api/auth/login", ah.login)
	mux.Handle("POST /api/auth/logout", requireUser(http.HandlerFunc(ah.logout)))
	mux.Handle("GET /api/auth/me", requireUser(http.HandlerFunc(ah.me)))

	mux.Handle("POST /api/generate", requireUser(http.HandlerFunc(gh.generate)))

	// Placeholder sampai M4; sudah dilindungi auth agar aturan aksesnya bisa dites.
	mux.Handle("/api/admin/", requireAdmin(http.HandlerFunc(notImplemented)))

	// Path /api/* yang tidak terdaftar dibalas JSON, bukan halaman 404 bawaan.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan.")
	})

	return chain(mux,
		requestID,
		clientIP(d.Config.TrustProxy),
		logRequests(d.Logger),
		recoverPanic(d.Logger),
		requireFetchHeader,
		limitBody,
	)
}

func notImplemented(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "Fitur belum tersedia.")
}
