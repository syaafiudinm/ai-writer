// Package auth menangani login, sesi, dan middleware otorisasi.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"sync"
	"time"

	"ai-writer/internal/db"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("auth: email atau password salah")
	ErrAccountDisabled    = errors.New("auth: akun nonaktif")
	ErrNoSession          = errors.New("auth: sesi tidak valid")
)

// dummyHash dibandingkan saat email tidak terdaftar agar waktu respons login
// tidak membocorkan apakah email ada.
var dummyHash = sync.OnceValue(func() []byte {
	h, _ := bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), BcryptCost)
	return h
})

type Service struct {
	db     *db.DB
	ttl    time.Duration
	logger *slog.Logger
}

func NewService(d *db.DB, ttl time.Duration, logger *slog.Logger) *Service {
	return &Service{db: d, ttl: ttl, logger: logger}
}

type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	User      *db.User
}

// Login memverifikasi kredensial, membuat sesi, dan mencatat audit.
// Email harus sudah dinormalisasi.
func (s *Service) Login(ctx context.Context, email, password, ip, userAgent string) (*LoginResult, error) {
	failed := func(actorID, reason string) {
		s.Audit(ctx, db.AuditEntry{
			ActorID:  actorID,
			Action:   db.AuditLoginFailed,
			IP:       ip,
			Metadata: map[string]any{"email": email, "reason": reason},
		})
	}

	u, err := s.db.UserByEmail(ctx, email)
	if errors.Is(err, db.ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
		failed("", "unknown_email")
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		failed(u.ID, "wrong_password")
		return nil, ErrInvalidCredentials
	}
	if !u.IsActive {
		failed(u.ID, "inactive")
		return nil, ErrAccountDisabled
	}

	token, hash := newToken()
	expires := time.Now().Add(s.ttl)
	err = s.db.CreateSession(ctx, db.NewSession{
		UserID: u.ID, TokenHash: hash, ExpiresAt: expires, IP: ip, UserAgent: userAgent,
	})
	if err != nil {
		return nil, err
	}
	if err := s.db.TouchLastLogin(ctx, u.ID); err != nil {
		s.logger.WarnContext(ctx, "update last_login_at gagal", "user_id", u.ID, "error", err)
	}
	s.Audit(ctx, db.AuditEntry{ActorID: u.ID, Action: db.AuditLoginSuccess, IP: ip})

	return &LoginResult{Token: token, ExpiresAt: expires, User: u}, nil
}

// Authenticate mengembalikan pemilik token sesi. Akun nonaktif ditolak dan
// semua sesinya dicabut.
func (s *Service) Authenticate(ctx context.Context, token string) (*db.User, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	u, err := s.db.SessionUser(ctx, hashToken(token))
	if errors.Is(err, db.ErrNotFound) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}
	if !u.IsActive {
		if err := s.db.DeleteUserSessions(ctx, u.ID); err != nil {
			s.logger.ErrorContext(ctx, "cabut sesi user nonaktif gagal", "user_id", u.ID, "error", err)
		}
		return nil, ErrAccountDisabled
	}
	return u, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.db.DeleteSession(ctx, hashToken(token))
}

// Audit mencatat jejak audit. Kegagalan hanya di-log agar tidak menggagalkan request.
func (s *Service) Audit(ctx context.Context, e db.AuditEntry) {
	if err := s.db.InsertAudit(ctx, e); err != nil {
		s.logger.ErrorContext(ctx, "tulis audit log gagal", "action", e.Action, "error", err)
	}
}

// newToken membuat token sesi acak 32 byte beserta hash yang disimpan di DB.
func newToken() (token string, hash []byte) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read tidak pernah gagal sejak Go 1.24
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token)
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
