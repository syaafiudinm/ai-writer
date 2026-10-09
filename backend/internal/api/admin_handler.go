package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"ai-writer/internal/admin"
	"ai-writer/internal/auth"
	"ai-writer/internal/db"
)

type adminHandler struct {
	svc    *admin.Service
	auth   *auth.Service
	db     *db.DB
	loc    *time.Location
	logger *slog.Logger
}

func (h *adminHandler) internalError(w http.ResponseWriter, r *http.Request, what string, err error) {
	h.logger.ErrorContext(r.Context(), what, "request_id", RequestIDFrom(r.Context()), "error", err)
	writeInternalError(w)
}

func (h *adminHandler) stats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rng, errs := h.svc.StatsRange(q.Get("from"), q.Get("to"))
	if errs != nil {
		writeFieldErrors(w, errs)
		return
	}
	st, err := h.svc.Stats(r.Context(), rng)
	if err != nil {
		h.internalError(w, r, "query statistik gagal", err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *adminHandler) logs(w http.ResponseWriter, r *http.Request) {
	f, errs := h.svc.ParseLogFilter(r.URL.Query())
	if errs != nil {
		writeFieldErrors(w, errs)
		return
	}
	page, err := h.svc.Logs(r.Context(), f)
	if err != nil {
		h.internalError(w, r, "query generation_logs gagal", err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *adminHandler) audit(w http.ResponseWriter, r *http.Request) {
	f, errs := admin.ParseAuditFilter(r.URL.Query())
	if errs != nil {
		writeFieldErrors(w, errs)
		return
	}
	page, err := h.svc.Audit(r.Context(), f)
	if err != nil {
		h.internalError(w, r, "query audit_logs gagal", err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

type adminUser struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Role        string     `json:"role"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

func (h *adminHandler) toAdminUser(u *db.User) adminUser {
	au := adminUser{ID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role, IsActive: u.IsActive,
		CreatedAt: u.CreatedAt.In(h.loc)}
	if u.LastLoginAt != nil {
		t := u.LastLoginAt.In(h.loc)
		au.LastLoginAt = &t
	}
	return au
}

func (h *adminHandler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.db.ListUsers(r.Context())
	if err != nil {
		h.internalError(w, r, "daftar user gagal", err)
		return
	}
	items := make([]adminUser, len(users))
	for i := range users {
		items[i] = h.toAdminUser(&users[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func validRole(role string) bool { return role == db.RoleAdmin || role == db.RoleUser }

const (
	msgRole     = "Peran harus admin atau user."
	msgPassword = "Password minimal 8 karakter (maksimal 72 byte)."
)

func (h *adminHandler) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	email := auth.NormalizeEmail(req.Email)
	name := strings.TrimSpace(req.Name)
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = db.RoleUser
	}

	fields := map[string]string{}
	if auth.ValidateEmail(email) != nil {
		fields["email"] = "Format email tidak valid."
	}
	if n := utf8.RuneCountInString(name); n < 1 || n > 100 {
		fields["name"] = "Nama wajib diisi (maksimal 100 karakter)."
	}
	if auth.ValidatePassword(req.Password) != nil {
		fields["password"] = msgPassword
	}
	if !validRole(role) {
		fields["role"] = msgRole
	}
	if len(fields) > 0 {
		writeFieldErrors(w, fields)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		h.internalError(w, r, "hash password gagal", err)
		return
	}
	u, err := h.db.CreateUser(r.Context(), db.NewUser{Email: email, Name: name, PasswordHash: hash, Role: role})
	if errors.Is(err, db.ErrEmailTaken) {
		writeError(w, http.StatusConflict, "email_taken", "Email sudah terdaftar.")
		return
	}
	if err != nil {
		h.internalError(w, r, "buat user gagal", err)
		return
	}

	actor := auth.UserFrom(r.Context())
	h.auth.Audit(r.Context(), db.AuditEntry{
		ActorID: actor.ID, Action: db.AuditUserCreated, TargetID: u.ID, IP: ClientIPFrom(r.Context()),
		Metadata: map[string]any{"email": u.Email, "role": u.Role},
	})
	writeJSON(w, http.StatusCreated, h.toAdminUser(u))
}

func (h *adminHandler) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !admin.IsUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "User tidak ditemukan.")
		return
	}
	var req struct {
		Role     *string `json:"role"`
		IsActive *bool   `json:"is_active"`
		Password *string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Role == nil && req.IsActive == nil && req.Password == nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "Isi minimal satu dari: role, is_active, password.")
		return
	}
	fields := map[string]string{}
	if req.Role != nil && !validRole(*req.Role) {
		fields["role"] = msgRole
	}
	if req.Password != nil && auth.ValidatePassword(*req.Password) != nil {
		fields["password"] = msgPassword
	}
	if len(fields) > 0 {
		writeFieldErrors(w, fields)
		return
	}

	ctx := r.Context()
	actor := auth.UserFrom(ctx)
	if id == actor.ID && ((req.Role != nil && *req.Role != db.RoleAdmin) || (req.IsActive != nil && !*req.IsActive)) {
		writeError(w, http.StatusConflict, "self_lockout",
			"Tidak bisa menonaktifkan atau menurunkan peran akun sendiri.")
		return
	}

	upd := db.UserUpdate{Role: req.Role, IsActive: req.IsActive}
	if req.Password != nil {
		hash, err := auth.HashPassword(*req.Password)
		if err != nil {
			h.internalError(w, r, "hash password gagal", err)
			return
		}
		upd.PasswordHash = &hash
	}

	before, after, err := h.db.UpdateUser(ctx, id, upd)
	switch {
	case errors.Is(err, db.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "User tidak ditemukan.")
		return
	case errors.Is(err, db.ErrLastAdmin):
		writeError(w, http.StatusConflict, "last_admin", "Harus ada minimal satu admin aktif.")
		return
	case err != nil:
		h.internalError(w, r, "ubah user gagal", err)
		return
	}

	audit := func(action string, meta map[string]any) {
		h.auth.Audit(ctx, db.AuditEntry{
			ActorID: actor.ID, Action: action, TargetID: id, IP: ClientIPFrom(ctx), Metadata: meta,
		})
	}
	if before.Role != after.Role {
		audit(db.AuditUserRoleChanged, map[string]any{"from": before.Role, "to": after.Role})
	}
	if before.IsActive && !after.IsActive {
		audit(db.AuditUserDisabled, nil)
	}
	if !before.IsActive && after.IsActive {
		audit(db.AuditUserEnabled, nil)
	}
	if req.Password != nil {
		audit(db.AuditUserPasswordSet, nil)
	}
	writeJSON(w, http.StatusOK, h.toAdminUser(after))
}
