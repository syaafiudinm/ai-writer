package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"ai-writer/internal/db"
)

const CookieName = "session"

// ErrorWriter menulis respons error dalam format API (disediakan paket api).
type ErrorWriter func(w http.ResponseWriter, status int, code, msg string)

type Middleware struct {
	Service      *Service
	CookieSecure bool
	WriteError   ErrorWriter
	// OnUser dipanggil setelah user terautentikasi, mis. untuk mencatat user id di log.
	OnUser func(ctx context.Context, u *db.User)
}

type ctxKey struct{}

// UserFrom mengambil user yang sedang login; nil jika route tidak melewati RequireAuth.
func UserFrom(ctx context.Context) *db.User {
	u, _ := ctx.Value(ctxKey{}).(*db.User)
	return u
}

func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil {
			m.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sesi berakhir, silakan login lagi.")
			return
		}

		u, err := m.Service.Authenticate(r.Context(), c.Value)
		switch {
		case errors.Is(err, ErrNoSession):
			ClearSessionCookie(w, m.CookieSecure)
			m.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sesi berakhir, silakan login lagi.")
			return
		case errors.Is(err, ErrAccountDisabled):
			ClearSessionCookie(w, m.CookieSecure)
			m.WriteError(w, http.StatusForbidden, "forbidden", "Akun dinonaktifkan, hubungi admin.")
			return
		case err != nil:
			m.Service.logger.ErrorContext(r.Context(), "autentikasi sesi gagal", "error", err)
			m.WriteError(w, http.StatusInternalServerError, "internal_error", "Terjadi kesalahan pada server, coba lagi.")
			return
		}

		if m.OnUser != nil {
			m.OnUser(r.Context(), u)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

// RequireRole harus dipasang di dalam RequireAuth.
func (m *Middleware) RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFrom(r.Context())
			if u == nil {
				m.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sesi berakhir, silakan login lagi.")
				return
			}
			if u.Role != role {
				m.WriteError(w, http.StatusForbidden, "forbidden", "Akun tidak punya akses ke halaman ini.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func SetSessionCookie(w http.ResponseWriter, token string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
