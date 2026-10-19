// Package api berisi router, middleware, dan handler HTTP.
package api

import (
	"context"
	"log/slog"
	"net/http"

	"ai-writer/internal/admin"
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
	Admin           *admin.Service
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

	adm := &adminHandler{svc: d.Admin, auth: d.Auth, db: d.DB, loc: d.Config.Location, logger: d.Logger}

	mux := http.NewServeMux()
	mux.Handle("GET /api/health", healthHandler(d.DB, d.Config.GeminiModel, d.Logger))

	mux.HandleFunc("POST /api/auth/login", ah.login)
	mux.Handle("POST /api/auth/logout", requireUser(http.HandlerFunc(ah.logout)))
	mux.Handle("GET /api/auth/me", requireUser(http.HandlerFunc(ah.me)))

	mux.Handle("POST /api/generate", requireUser(http.HandlerFunc(gh.generate)))

	mux.Handle("GET /api/admin/stats", requireAdmin(http.HandlerFunc(adm.stats)))
	mux.Handle("GET /api/admin/logs", requireAdmin(http.HandlerFunc(adm.logs)))
	mux.Handle("GET /api/admin/audit", requireAdmin(http.HandlerFunc(adm.audit)))
	mux.Handle("GET /api/admin/users", requireAdmin(http.HandlerFunc(adm.listUsers)))
	mux.Handle("POST /api/admin/users", requireAdmin(http.HandlerFunc(adm.createUser)))
	mux.Handle("PATCH /api/admin/users/{id}", requireAdmin(http.HandlerFunc(adm.updateUser)))
	// Path admin lain tetap lewat cek admin dulu agar keberadaan endpoint tidak bocor.
	mux.Handle("/api/admin/", requireAdmin(http.HandlerFunc(notFound)))

	// Path /api/* yang tidak terdaftar dibalas JSON, bukan halaman 404 bawaan.
	mux.HandleFunc("/api/", notFound)

	return chain(mux,
		requestID,
		clientIP(d.Config.TrustProxy, d.Config.TrustProxyHops),
		logRequests(d.Logger, d.Config.LogProxyHeaders),
		recoverPanic(d.Logger),
		requireFetchHeader,
		limitBody,
	)
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "Endpoint tidak ditemukan.")
}
