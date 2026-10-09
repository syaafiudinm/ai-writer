package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ai-writer/internal/admin"
	"ai-writer/internal/auth"
	"ai-writer/internal/config"
	"ai-writer/internal/db"
	"ai-writer/internal/ratelimit"
)

type adminEnv struct {
	c      testClient
	d      *db.DB
	admin  *db.User
	cookie *http.Cookie
	loc    *time.Location
}

func newAdminEnv(t *testing.T) adminEnv {
	t.Helper()
	d := testDB(t)
	loc, _ := time.LoadLocation("Asia/Makassar")
	cfg := &config.Config{GeminiModel: "gemini-test", SessionTTL: time.Hour, Location: loc}
	srv := httptest.NewServer(NewRouter(Deps{
		Config: cfg, DB: d, Logger: discard,
		Auth:            auth.NewService(d, cfg.SessionTTL, discard),
		LoginLimiter:    ratelimit.New(100, time.Minute),
		GenerateLimiter: ratelimit.New(10, time.Minute),
		Streamer:        &fakeStreamer{},
		Admin:           admin.NewService(d, 1, 2, loc), // $1 input, $2 output per juta token
	}))
	t.Cleanup(srv.Close)
	c := testClient{t: t, srv: srv}
	a := createTestUser(t, d, "boss@test.com", db.RoleAdmin, "password-boss")
	ck, r := c.login("boss@test.com", "password-boss")
	c.expect(r, 200, "")
	return adminEnv{c: c, d: d, admin: a, cookie: ck, loc: loc}
}

func insertLog(t *testing.T, d *db.DB, userID string, at time.Time, typ, status string, in, out int, ttft *int, dur int) {
	t.Helper()
	_, err := d.Pool.Exec(context.Background(), `
		INSERT INTO generation_logs (user_id, type, tone, length, language, topic, model, status,
		  error_code, input_tokens, output_tokens, ttft_ms, duration_ms, created_at)
		VALUES ($1, $2, 'formal', 'sedang', 'id', 'Topik uji', 'gemini-test', $3,
		  CASE WHEN $3 = 'success' THEN NULL ELSE $3 END, $4, $5, $6, $7, $8)`,
		userID, typ, status, in, out, ttft, dur, at)
	if err != nil {
		t.Fatal(err)
	}
}

func ms(n int) *int { return &n }

func decode[T any](t *testing.T, r resp) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(r.raw), &v); err != nil {
		t.Fatalf("decode %T: %v (%s)", v, err, r.raw)
	}
	return v
}

func TestAdminStatsMatchesManualCount(t *testing.T) {
	env := newAdminEnv(t)
	a := createTestUser(t, env.d, "a@test.com", db.RoleUser, "password-a")
	b := createTestUser(t, env.d, "b@test.com", db.RoleUser, "password-b")
	wita := func(day, hour, min int) time.Time { return time.Date(2026, 10, day, hour, min, 0, 0, env.loc) }

	// Rentang 1–3 Okt WITA. Baris di luar rentang menguji batas tengah malam lokal.
	insertLog(t, env.d, a.ID, time.Date(2026, 9, 30, 23, 59, 0, 0, env.loc), "berita", "success", 1000, 1000, ms(1), 1)
	insertLog(t, env.d, a.ID, wita(1, 0, 0), "berita", "success", 100, 200, ms(1000), 5000)
	insertLog(t, env.d, b.ID, wita(1, 23, 30), "artikel", "error", 50, 0, nil, 1000)
	insertLog(t, env.d, a.ID, wita(2, 0, 30), "berita", "success", 300, 600, ms(3000), 7000)
	insertLog(t, env.d, a.ID, wita(2, 10, 0), "berita", "timeout", 10, 20, ms(500), 180000)
	insertLog(t, env.d, b.ID, wita(2, 11, 0), "artikel", "cancelled", 5, 5, ms(800), 2000)
	insertLog(t, env.d, a.ID, wita(4, 0, 0), "berita", "success", 1000, 1000, ms(1), 1)

	r := env.c.do("GET", "/api/admin/stats?from=2026-10-01&to=2026-10-03", "", env.cookie)
	env.c.expect(r, 200, "")
	st := decode[admin.Stats](t, r)

	if st.Range != (admin.RangeInfo{From: "2026-10-01", To: "2026-10-03", Timezone: "Asia/Makassar"}) ||
		!st.Pricing.Configured {
		t.Errorf("range/pricing = %+v %+v", st.Range, st.Pricing)
	}
	s := st.Summary
	want := admin.Summary{
		Generations: 5, ActiveUsers: 2, InputTokens: 465, OutputTokens: 825,
		EstimatedCostUSD: 0.002115, // 465/1e6*$1 + 825/1e6*$2
		ErrorRate:        0.4,      // (1 error + 1 timeout) / 5
		ByStatus:         admin.StatusCounts{Success: 2, Error: 1, Cancelled: 1, Timeout: 1},
	}
	if s.Generations != want.Generations || s.ActiveUsers != want.ActiveUsers ||
		s.InputTokens != want.InputTokens || s.OutputTokens != want.OutputTokens ||
		s.EstimatedCostUSD != want.EstimatedCostUSD || s.ErrorRate != want.ErrorRate ||
		s.ByStatus != want.ByStatus {
		t.Errorf("summary = %+v\nwant      %+v", s, want)
	}
	if s.AvgDurationMs == nil || *s.AvgDurationMs != 6000 || s.P50TTFTMs == nil || *s.P50TTFTMs != 2000 {
		t.Errorf("avg_duration=%v p50_ttft=%v, want 6000 dan 2000", s.AvgDurationMs, s.P50TTFTMs)
	}

	if len(st.Daily) != 3 {
		t.Fatalf("daily = %+v", st.Daily)
	}
	d1, d2, d3 := st.Daily[0], st.Daily[1], st.Daily[2]
	if d1.Date != "2026-10-01" || d1.Generations != 2 || d1.ActiveUsers != 2 || d1.InputTokens != 150 ||
		d1.OutputTokens != 200 || d1.ErrorRate != 0.5 || *d1.AvgDurationMs != 5000 || d1.EstimatedCostUSD != 0.00055 {
		t.Errorf("1 Okt = %+v", d1)
	}
	if d2.Date != "2026-10-02" || d2.Generations != 3 || d2.ActiveUsers != 2 || d2.InputTokens != 315 ||
		d2.OutputTokens != 625 || d2.ErrorRate != 0.3333 || *d2.AvgDurationMs != 7000 || d2.EstimatedCostUSD != 0.001565 {
		t.Errorf("2 Okt = %+v", d2)
	}
	if d3.Date != "2026-10-03" || d3.Generations != 0 || d3.AvgDurationMs != nil || d3.ErrorRate != 0 {
		t.Errorf("3 Okt (kosong) = %+v", d3)
	}

	// Silang-cek dengan hitungan SQL mentah memakai batas yang sama.
	n := countRows(t, env.d, `SELECT count(*) FROM generation_logs WHERE created_at >= $1 AND created_at < $2`,
		wita(1, 0, 0), wita(4, 0, 0))
	if n != s.Generations {
		t.Errorf("SQL manual = %d, stats = %d", n, s.Generations)
	}

	t.Run("default 30 hari", func(t *testing.T) {
		st := decode[admin.Stats](t, env.c.do("GET", "/api/admin/stats", "", env.cookie))
		if len(st.Daily) != 30 {
			t.Errorf("daily = %d hari, want 30", len(st.Daily))
		}
	})
	t.Run("rentang tidak valid", func(t *testing.T) {
		r := env.c.do("GET", "/api/admin/stats?from=2026-10-05&to=2026-10-01", "", env.cookie)
		env.c.expect(r, 400, "invalid_input")
		if r.body.Fields["from"] == "" {
			t.Errorf("fields = %v", r.body.Fields)
		}
	})
}

func TestAdminLogsFilterAndPagination(t *testing.T) {
	env := newAdminEnv(t)
	a := createTestUser(t, env.d, "a@test.com", db.RoleUser, "password-a")
	b := createTestUser(t, env.d, "b@test.com", db.RoleUser, "password-b")
	base := time.Date(2026, 10, 1, 8, 0, 0, 0, env.loc)
	for i := range 60 {
		insertLog(t, env.d, a.ID, base.Add(time.Duration(i)*time.Minute), "berita", "success", 1_000_000, 0, ms(900), 3000)
	}
	for i := range 5 {
		insertLog(t, env.d, b.ID, base.AddDate(0, 0, 2).Add(time.Duration(i)*time.Minute), "artikel", "error", 0, 0, nil, 100)
	}

	type page = admin.Page[admin.LogEntry]
	get := func(q string) page {
		r := env.c.do("GET", "/api/admin/logs"+q, "", env.cookie)
		env.c.expect(r, 200, "")
		return decode[page](t, r)
	}

	p1 := get("")
	if p1.Total != 65 || p1.TotalPages != 2 || len(p1.Items) != 50 || p1.PageSize != 50 {
		t.Fatalf("page 1: total=%d pages=%d items=%d", p1.Total, p1.TotalPages, len(p1.Items))
	}
	if first := p1.Items[0]; first.User.Email != "b@test.com" || first.Status != "error" ||
		first.ErrorCode == nil || *first.ErrorCode != "error" {
		t.Errorf("urutan harus terbaru dulu: %+v", first)
	}
	if p2 := get("?page=2"); len(p2.Items) != 15 || p2.Page != 2 {
		t.Errorf("page 2 items = %d", len(p2.Items))
	}
	if p := get("?page=3"); len(p.Items) != 0 || p.Items == nil {
		t.Errorf("halaman kosong harus [] bukan null: %+v", p)
	}

	if p := get("?user_id=" + b.ID); p.Total != 5 {
		t.Errorf("filter user_id total = %d", p.Total)
	}
	if p := get("?status=success&type=berita"); p.Total != 60 || p.Items[0].EstimatedCostUSD != 1 {
		t.Errorf("filter status+type total = %d cost=%v", p.Total, p.Items[0].EstimatedCostUSD)
	}
	if p := get("?from=2026-10-03&to=2026-10-03"); p.Total != 5 {
		t.Errorf("filter tanggal total = %d", p.Total)
	}
	if p := get("?to=2026-10-02"); p.Total != 60 {
		t.Errorf("filter to saja total = %d", p.Total)
	}

	r := env.c.do("GET", "/api/admin/logs?user_id=x&status=gagal&page=-1", "", env.cookie)
	env.c.expect(r, 400, "invalid_input")
	if len(r.body.Fields) != 3 {
		t.Errorf("fields = %v", r.body.Fields)
	}
}

func TestAdminUserManagement(t *testing.T) {
	env := newAdminEnv(t)
	c := env.c

	// --- create ---
	r := c.do("POST", "/api/admin/users",
		`{"email":" Penulis@Test.com ","name":"Penulis Satu","password":"password-awal"}`, env.cookie)
	c.expect(r, 201, "")
	created := decode[adminUser](t, r)
	if created.Email != "penulis@test.com" || created.Role != "user" || !created.IsActive {
		t.Fatalf("created = %+v", created)
	}
	c.expect(c.do("POST", "/api/admin/users",
		`{"email":"penulis@test.com","name":"X","password":"password-awal"}`, env.cookie), 409, "email_taken")
	r = c.do("POST", "/api/admin/users", `{"email":"bukan","name":"","password":"pendek","role":"editor"}`, env.cookie)
	c.expect(r, 400, "invalid_input")
	if len(r.body.Fields) != 4 {
		t.Errorf("fields = %v", r.body.Fields)
	}

	list := decode[struct{ Items []adminUser }](t, c.do("GET", "/api/admin/users", "", env.cookie))
	if len(list.Items) != 2 {
		t.Errorf("users = %+v", list.Items)
	}

	// --- user biasa tidak boleh ke semua endpoint admin ---
	userCookie, lr := c.login("penulis@test.com", "password-awal")
	c.expect(lr, 200, "")
	for _, ep := range []struct{ method, path string }{
		{"GET", "/api/admin/stats"}, {"GET", "/api/admin/logs"}, {"GET", "/api/admin/audit"},
		{"GET", "/api/admin/users"}, {"POST", "/api/admin/users"}, {"PATCH", "/api/admin/users/" + created.ID},
		{"GET", "/api/admin/tidak-ada"},
	} {
		c.expect(c.do(ep.method, ep.path, "{}", userCookie), 403, "forbidden")
	}

	patch := func(id, body string) resp { return c.do("PATCH", "/api/admin/users/"+id, body, env.cookie) }

	// --- role ---
	r = patch(created.ID, `{"role":"admin"}`)
	c.expect(r, 200, "")
	if decode[adminUser](t, r).Role != "admin" {
		t.Error("role tidak berubah")
	}
	c.expect(c.do("GET", "/api/admin/stats", "", userCookie), 200, "") // berlaku tanpa login ulang
	c.expect(patch(created.ID, `{"role":"user"}`), 200, "")

	// --- nonaktif: sesi langsung dicabut, login ditolak ---
	r = patch(created.ID, `{"is_active":false}`)
	c.expect(r, 200, "")
	c.expect(c.do("GET", "/api/auth/me", "", userCookie), 401, "unauthenticated")
	_, lr = c.login("penulis@test.com", "password-awal")
	c.expect(lr, 403, "forbidden")

	c.expect(patch(created.ID, `{"is_active":true}`), 200, "")
	userCookie, lr = c.login("penulis@test.com", "password-awal")
	c.expect(lr, 200, "")

	// --- reset password: sesi lama dicabut, password lama tidak berlaku ---
	c.expect(patch(created.ID, `{"password":"password-baru"}`), 200, "")
	c.expect(c.do("GET", "/api/auth/me", "", userCookie), 401, "unauthenticated")
	_, lr = c.login("penulis@test.com", "password-awal")
	c.expect(lr, 401, "unauthenticated")
	_, lr = c.login("penulis@test.com", "password-baru")
	c.expect(lr, 200, "")

	// --- validasi & pengaman ---
	c.expect(patch(env.admin.ID, `{"role":"user"}`), 409, "self_lockout")
	c.expect(patch(env.admin.ID, `{"is_active":false}`), 409, "self_lockout")
	c.expect(patch(env.admin.ID, `{"password":"password-boss-2"}`), 200, "") // ganti password sendiri boleh
	adminCookie, lr := c.login("boss@test.com", "password-boss-2")
	c.expect(lr, 200, "")
	patch = func(id, body string) resp { return c.do("PATCH", "/api/admin/users/"+id, body, adminCookie) }

	c.expect(patch(created.ID, `{}`), 400, "invalid_input")
	r = patch(created.ID, `{"role":"editor","password":"x"}`)
	c.expect(r, 400, "invalid_input")
	if len(r.body.Fields) != 2 {
		t.Errorf("fields = %v", r.body.Fields)
	}
	c.expect(patch("6efb4624-28e9-4c11-9e6b-8d71829c2582", `{"role":"user"}`), 404, "not_found")
	c.expect(patch("bukan-uuid", `{"role":"user"}`), 404, "not_found")

	// --- audit ---
	audit := decode[admin.Page[admin.AuditEntry]](t, c.do("GET", "/api/admin/audit?action=user_disabled", "", adminCookie))
	if audit.Total != 1 || audit.Items[0].Actor == nil || audit.Items[0].Actor.Email != "boss@test.com" ||
		audit.Items[0].Target == nil || audit.Items[0].Target.Email != "penulis@test.com" {
		t.Errorf("audit user_disabled = %+v", audit)
	}
	for action, want := range map[string]int{
		"user_created": 1, "user_role_changed": 2, "user_disabled": 1, "user_enabled": 1, "user_password_reset": 2,
	} {
		if n := countRows(t, env.d, `SELECT count(*) FROM audit_logs WHERE action = $1`, action); n != want {
			t.Errorf("audit %s = %d, want %d", action, n, want)
		}
	}
	all := decode[admin.Page[admin.AuditEntry]](t, c.do("GET", "/api/admin/audit", "", adminCookie))
	if _, offset := all.Items[0].CreatedAt.Zone(); all.Total < 10 || offset != 8*3600 {
		t.Errorf("audit total = %d, offset created_at = %d (want WITA +08:00)", all.Total, offset)
	}
	reasons := map[any]bool{}
	for _, e := range all.Items {
		if e.Action == "login_failed" {
			reasons[e.Metadata["reason"]] = true
		}
	}
	if !reasons["wrong_password"] || !reasons["inactive"] {
		t.Errorf("metadata reason login_failed = %v, want wrong_password & inactive", reasons)
	}
}

func TestUpdateUserKeepsLastAdmin(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	only := createTestUser(t, d, "only@test.com", db.RoleAdmin, "password-only")
	user, inactive := db.RoleUser, false

	if _, _, err := d.UpdateUser(ctx, only.ID, db.UserUpdate{Role: &user}); !errors.Is(err, db.ErrLastAdmin) {
		t.Errorf("turunkan admin terakhir: err = %v", err)
	}
	if _, _, err := d.UpdateUser(ctx, only.ID, db.UserUpdate{IsActive: &inactive}); !errors.Is(err, db.ErrLastAdmin) {
		t.Errorf("nonaktifkan admin terakhir: err = %v", err)
	}
	createTestUser(t, d, "second@test.com", db.RoleAdmin, "password-second")
	if _, after, err := d.UpdateUser(ctx, only.ID, db.UserUpdate{Role: &user}); err != nil || after.Role != "user" {
		t.Errorf("dengan admin lain: after=%+v err=%v", after, err)
	}
}

// PRD: halaman dashboard admin ≤ 1 detik untuk 30 hari data.
func TestAdminStatsPerformance30Days(t *testing.T) {
	if testing.Short() {
		t.Skip("lewati di -short")
	}
	env := newAdminEnv(t)
	users := make([]string, 20)
	for i := range users {
		users[i] = createTestUser(t, env.d, fmt.Sprintf("u%d@test.com", i), db.RoleUser, "password-u").ID
	}
	const perDay = 2000
	_, err := env.d.Pool.Exec(context.Background(), `
		INSERT INTO generation_logs (user_id, type, tone, length, language, topic, model, status,
		  input_tokens, output_tokens, ttft_ms, duration_ms, created_at)
		SELECT ($1::uuid[])[1 + i % 20], CASE WHEN i % 3 = 0 THEN 'artikel' ELSE 'berita' END,
		       'formal', 'sedang', 'id', 'Topik ' || i, 'gemini-test',
		       CASE WHEN i % 50 = 0 THEN 'error' ELSE 'success' END,
		       400 + i % 300, 900 + i % 700, 800 + i % 4000, 3000 + i % 9000,
		       now() - (i * interval '30 days' / $2)
		FROM generate_series(1, $2) AS i`, users, perDay*30)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.d.Pool.Exec(context.Background(), `ANALYZE generation_logs`); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/api/admin/stats", "/api/admin/logs?status=success&page=10"} {
		start := time.Now()
		r := env.c.do("GET", path, "", env.cookie)
		elapsed := time.Since(start)
		env.c.expect(r, 200, "")
		t.Logf("%s: %v untuk %d baris", path, elapsed, perDay*30)
		if elapsed > time.Second {
			t.Errorf("%s butuh %v, target ≤ 1 detik", path, elapsed)
		}
	}
}
