package admin

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ai-writer/internal/db"

	"github.com/jackc/pgx/v5"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-([0-9a-fA-F]{4}-){3}[0-9a-fA-F]{12}$`)

// IsUUID mengecek format UUID agar id yang salah dibalas 404/400, bukan error DB.
func IsUUID(s string) bool { return uuidPattern.MatchString(s) }

// Page adalah satu halaman hasil daftar.
type Page[T any] struct {
	Items      []T `json:"items"`
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

func newPage[T any](items []T, page, total int) Page[T] {
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, Page: page, PageSize: PageSize, Total: total,
		TotalPages: (total + PageSize - 1) / PageSize}
}

func parsePage(v string, errs map[string]string) int {
	if v == "" {
		return 1
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > maxPage {
		errs["page"] = "Halaman harus bilangan bulat mulai dari 1."
		return 1
	}
	return n
}

// ---------- generation_logs ----------

type LogFilter struct {
	UserID string
	Status string
	Type   string
	Since  *time.Time // inklusif
	Until  *time.Time // eksklusif
	Page   int
}

func (s *Service) ParseLogFilter(q url.Values) (LogFilter, map[string]string) {
	errs := map[string]string{}
	f := LogFilter{
		UserID: strings.TrimSpace(q.Get("user_id")),
		Status: strings.TrimSpace(q.Get("status")),
		Type:   strings.TrimSpace(q.Get("type")),
		Page:   parsePage(q.Get("page"), errs),
	}
	if f.UserID != "" && !IsUUID(f.UserID) {
		errs["user_id"] = "user_id harus berupa UUID."
	}
	switch f.Status {
	case "", db.GenStatusSuccess, db.GenStatusError, db.GenStatusCancelled, db.GenStatusTimeout:
	default:
		errs["status"] = "Status harus salah satu dari: success, error, cancelled, timeout."
	}
	switch f.Type {
	case "", "berita", "artikel":
	default:
		errs["type"] = "Jenis harus berita atau artikel."
	}
	if v := q.Get("from"); v != "" {
		if t, ok := s.parseDate(v); ok {
			f.Since = &t
		} else {
			errs["from"] = msgDateFormat
		}
	}
	if v := q.Get("to"); v != "" {
		if t, ok := s.parseDate(v); ok {
			end := t.AddDate(0, 0, 1)
			f.Until = &end
		} else {
			errs["to"] = msgDateFormat
		}
	}
	if f.Since != nil && f.Until != nil && !f.Since.Before(*f.Until) {
		errs["from"] = msgRangeOrder
	}
	if len(errs) > 0 {
		return LogFilter{}, errs
	}
	return f, nil
}

type UserRef struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type LogEntry struct {
	ID               int64     `json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	User             UserRef   `json:"user"`
	Type             string    `json:"type"`
	Tone             string    `json:"tone"`
	Length           string    `json:"length"`
	Language         string    `json:"language"`
	Topic            string    `json:"topic"`
	Model            string    `json:"model"`
	Status           string    `json:"status"`
	ErrorCode        *string   `json:"error_code"`
	InputTokens      int       `json:"input_tokens"`
	OutputTokens     int       `json:"output_tokens"`
	EstimatedCostUSD float64   `json:"estimated_cost_usd"`
	TTFTMs           *int      `json:"ttft_ms"`
	DurationMs       int       `json:"duration_ms"`
	IP               *string   `json:"ip"`
}

// where menyusun klausa WHERE dinamis dengan placeholder bernomor.
type where struct {
	conds []string
	args  []any
}

func (w *where) add(cond string, arg any) {
	w.args = append(w.args, arg)
	w.conds = append(w.conds, fmt.Sprintf(cond, len(w.args)))
}

func (w *where) sql() string {
	if len(w.conds) == 0 {
		return ""
	}
	return "WHERE " + strings.Join(w.conds, " AND ")
}

func (s *Service) Logs(ctx context.Context, f LogFilter) (Page[LogEntry], error) {
	ctx, cancel := context.WithTimeout(ctx, db.QueryTimeout)
	defer cancel()

	var w where
	if f.UserID != "" {
		w.add("g.user_id = $%d", f.UserID)
	}
	if f.Status != "" {
		w.add("g.status = $%d", f.Status)
	}
	if f.Type != "" {
		w.add("g.type = $%d", f.Type)
	}
	if f.Since != nil {
		w.add("g.created_at >= $%d", *f.Since)
	}
	if f.Until != nil {
		w.add("g.created_at < $%d", *f.Until)
	}

	var total int
	if err := s.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM generation_logs g `+w.sql(), w.args...).Scan(&total); err != nil {
		return Page[LogEntry]{}, err
	}

	args := append(w.args, (f.Page-1)*PageSize)
	rows, err := s.db.Pool.Query(ctx, `
		SELECT g.id, g.created_at, u.id, u.email, u.name, g.type, g.tone, g.length, g.language,
		       g.topic, g.model, g.status, g.error_code, g.input_tokens, g.output_tokens,
		       g.ttft_ms, g.duration_ms, g.ip
		FROM generation_logs g JOIN users u ON u.id = g.user_id
		`+w.sql()+`
		ORDER BY g.created_at DESC, g.id DESC
		LIMIT `+strconv.Itoa(PageSize)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return Page[LogEntry]{}, err
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (LogEntry, error) {
		var e LogEntry
		err := row.Scan(&e.ID, &e.CreatedAt, &e.User.ID, &e.User.Email, &e.User.Name, &e.Type, &e.Tone,
			&e.Length, &e.Language, &e.Topic, &e.Model, &e.Status, &e.ErrorCode, &e.InputTokens,
			&e.OutputTokens, &e.TTFTMs, &e.DurationMs, &e.IP)
		e.CreatedAt = e.CreatedAt.In(s.loc)
		e.EstimatedCostUSD = s.cost(int64(e.InputTokens), int64(e.OutputTokens))
		return e, err
	})
	if err != nil {
		return Page[LogEntry]{}, err
	}
	return newPage(items, f.Page, total), nil
}

// ---------- audit_logs ----------

type AuditFilter struct {
	Action string
	Page   int
}

func ParseAuditFilter(q url.Values) (AuditFilter, map[string]string) {
	errs := map[string]string{}
	f := AuditFilter{Action: strings.TrimSpace(q.Get("action")), Page: parsePage(q.Get("page"), errs)}
	if len(f.Action) > 64 {
		errs["action"] = "Action maksimal 64 karakter."
	}
	if len(errs) > 0 {
		return AuditFilter{}, errs
	}
	return f, nil
}

type AuditEntry struct {
	ID        int64          `json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	Action    string         `json:"action"`
	Actor     *UserRef       `json:"actor"`  // nil: mis. login gagal dengan email tak terdaftar
	Target    *UserRef       `json:"target"` // nil: aksi tanpa target
	Metadata  map[string]any `json:"metadata"`
	IP        *string        `json:"ip"`
}

func (s *Service) Audit(ctx context.Context, f AuditFilter) (Page[AuditEntry], error) {
	ctx, cancel := context.WithTimeout(ctx, db.QueryTimeout)
	defer cancel()

	var w where
	if f.Action != "" {
		w.add("a.action = $%d", f.Action)
	}
	var total int
	if err := s.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_logs a `+w.sql(), w.args...).Scan(&total); err != nil {
		return Page[AuditEntry]{}, err
	}

	args := append(w.args, (f.Page-1)*PageSize)
	rows, err := s.db.Pool.Query(ctx, `
		SELECT a.id, a.created_at, a.action,
		       ua.id, ua.email, ua.name, ut.id, ut.email, ut.name,
		       a.metadata, a.ip
		FROM audit_logs a
		LEFT JOIN users ua ON ua.id = a.actor_id
		LEFT JOIN users ut ON ut.id = a.target_id
		`+w.sql()+`
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT `+strconv.Itoa(PageSize)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return Page[AuditEntry]{}, err
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AuditEntry, error) {
		var e AuditEntry
		var actorID, actorEmail, actorName, targetID, targetEmail, targetName *string
		err := row.Scan(&e.ID, &e.CreatedAt, &e.Action, &actorID, &actorEmail, &actorName,
			&targetID, &targetEmail, &targetName, &e.Metadata, &e.IP)
		e.CreatedAt = e.CreatedAt.In(s.loc)
		if actorID != nil {
			e.Actor = &UserRef{ID: *actorID, Email: *actorEmail, Name: *actorName}
		}
		if targetID != nil {
			e.Target = &UserRef{ID: *targetID, Email: *targetEmail, Name: *targetName}
		}
		return e, err
	})
	if err != nil {
		return Page[AuditEntry]{}, err
	}
	return newPage(items, f.Page, total), nil
}
