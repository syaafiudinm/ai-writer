package api

import (
	"errors"
	"log/slog"
	"net/http"

	"ai-writer/internal/auth"
	"ai-writer/internal/db"
	"ai-writer/internal/ratelimit"
)

type authHandler struct {
	svc          *auth.Service
	limiter      *ratelimit.Limiter // percobaan login gagal per IP + email
	cookieSecure bool
	logger       *slog.Logger
}

type userResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

func toUserResponse(u *db.User) userResponse {
	return userResponse{ID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role}
}

func (h *authHandler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	email := auth.NormalizeEmail(req.Email)
	fields := map[string]string{}
	if email == "" {
		fields["email"] = "Email wajib diisi."
	}
	if req.Password == "" {
		fields["password"] = "Password wajib diisi."
	}
	if len(fields) > 0 {
		writeFieldErrors(w, fields)
		return
	}

	ctx := r.Context()
	ip := ClientIPFrom(ctx)
	key := ip + "|" + email
	if ok, retry := h.limiter.Check(key); !ok {
		h.svc.Audit(ctx, db.AuditEntry{
			Action: db.AuditLoginRateLimited, IP: ip, Metadata: map[string]any{"email": email},
		})
		writeRateLimited(w, retry)
		return
	}

	res, err := h.svc.Login(ctx, email, req.Password, ip, r.UserAgent())
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		h.limiter.Add(key)
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Email atau password salah.")
		return
	case errors.Is(err, auth.ErrAccountDisabled):
		writeError(w, http.StatusForbidden, "forbidden", "Akun dinonaktifkan, hubungi admin.")
		return
	case err != nil:
		h.logger.ErrorContext(ctx, "login gagal", "error", err)
		writeInternalError(w)
		return
	}

	h.limiter.Reset(key)
	setLogUserID(ctx, res.User.ID)
	auth.SetSessionCookie(w, res.Token, res.ExpiresAt, h.cookieSecure)
	writeJSON(w, http.StatusOK, toUserResponse(res.User))
}

func (h *authHandler) logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := auth.UserFrom(ctx)
	if c, err := r.Cookie(auth.CookieName); err == nil {
		if err := h.svc.Logout(ctx, c.Value); err != nil {
			h.logger.ErrorContext(ctx, "hapus sesi gagal", "error", err)
			writeInternalError(w)
			return
		}
	}
	h.svc.Audit(ctx, db.AuditEntry{ActorID: u.ID, Action: db.AuditLogout, IP: ClientIPFrom(ctx)})
	auth.ClearSessionCookie(w, h.cookieSecure)
	w.WriteHeader(http.StatusNoContent)
}

func (h *authHandler) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, toUserResponse(auth.UserFrom(r.Context())))
}
