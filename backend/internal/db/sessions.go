package db

import (
	"context"
	"time"
)

type NewSession struct {
	UserID    string
	TokenHash []byte // SHA-256 token; token asli tidak pernah disimpan
	ExpiresAt time.Time
	IP        string
	UserAgent string
}

const maxUserAgentLen = 512

func (d *DB) CreateSession(ctx context.Context, s NewSession) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	ua := s.UserAgent
	if len(ua) > maxUserAgentLen {
		ua = ua[:maxUserAgentLen]
	}
	_, err := d.Pool.Exec(ctx,
		`INSERT INTO sessions (user_id, token_hash, expires_at, ip, user_agent)
		 VALUES ($1, $2, $3, $4, $5)`,
		s.UserID, s.TokenHash, s.ExpiresAt, nullable(s.IP), nullable(ua))
	return err
}

// SessionUser mengembalikan pemilik sesi yang belum kedaluwarsa, atau ErrNotFound.
func (d *DB) SessionUser(ctx context.Context, tokenHash []byte) (*User, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return scanUser(d.Pool.QueryRow(ctx,
		`SELECT `+userColumns("u.")+`
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.expires_at > now()`, tokenHash))
}

func (d *DB) DeleteSession(ctx context.Context, tokenHash []byte) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := d.Pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

// DeleteUserSessions mencabut semua sesi milik user (nonaktif / reset password).
func (d *DB) DeleteUserSessions(ctx context.Context, userID string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := d.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}
