package db

import "context"

// Nilai kolom audit_logs.action.
const (
	AuditLoginSuccess     = "login_success"
	AuditLoginFailed      = "login_failed"
	AuditLoginRateLimited = "login_rate_limited"
	AuditLogout           = "logout"
	AuditUserCreated      = "user_created"
)

type AuditEntry struct {
	ActorID  string // kosong = NULL, mis. login gagal dengan email tak terdaftar
	Action   string
	TargetID string
	Metadata map[string]any
	IP       string
}

func (d *DB) InsertAudit(ctx context.Context, e AuditEntry) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	meta := e.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	_, err := d.Pool.Exec(ctx,
		`INSERT INTO audit_logs (actor_id, action, target_id, metadata, ip)
		 VALUES ($1, $2, $3, $4, $5)`,
		nullable(e.ActorID), e.Action, nullable(e.TargetID), meta, nullable(e.IP))
	return err
}
