package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"ai-writer/internal/admin"
	"ai-writer/internal/auth"
	"ai-writer/internal/config"
	"ai-writer/internal/db"
	"ai-writer/internal/ratelimit"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// testDB membuka database test, menjalankan migrasi, dan mengosongkan semua tabel.
// Hanya jalan jika TEST_DATABASE_URL diset dan nama database berakhiran _test.
func testDB(t *testing.T) *db.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL tidak diset; lewati test integrasi")
	}
	pc, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(pc.ConnConfig.Database, "_test") {
		t.Fatalf("menolak mengosongkan database %q: nama harus berakhiran _test", pc.ConnConfig.Database)
	}

	ctx := context.Background()
	d, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx,
		`TRUNCATE users, sessions, generation_logs, audit_logs RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	return d
}

func createTestUser(t *testing.T, d *db.DB, email, role, password string) *db.User {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost) // cepat untuk test
	u, err := d.CreateUser(context.Background(), db.NewUser{
		Email: email, Name: "Test " + role, PasswordHash: string(hash), Role: role,
	})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

type testClient struct {
	t   *testing.T
	srv *httptest.Server
}

type resp struct {
	status int
	header http.Header
	body   errorBody
	raw    string
}

func (c testClient) do(method, path, body string, cookie *http.Cookie, headers ...string) resp {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	for i := 0; i+1 < len(headers); i += 2 {
		if headers[i+1] == "" {
			req.Header.Del(headers[i])
		} else {
			req.Header.Set(headers[i], headers[i+1])
		}
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res, err := c.srv.Client().Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	r := resp{status: res.StatusCode, header: res.Header, raw: string(raw)}
	_ = json.Unmarshal(raw, &r.body)
	return r
}

func (c testClient) login(email, password string) (*http.Cookie, resp) {
	c.t.Helper()
	r := c.do("POST", "/api/auth/login", `{"email":"`+email+`","password":"`+password+`"}`, nil)
	for _, ck := range (&http.Response{Header: r.header}).Cookies() {
		if ck.Name == auth.CookieName {
			return ck, r
		}
	}
	return nil, r
}

func (c testClient) expect(r resp, status int, code string) {
	c.t.Helper()
	if r.status != status || (code != "" && r.body.Code != code) {
		c.t.Fatalf("got %d %q, want %d %q (body: %s)", r.status, r.body.Code, status, code, r.raw)
	}
}

func countRows(t *testing.T, d *db.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := d.Pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAuthFlow(t *testing.T) {
	d := testDB(t)
	cfg := &config.Config{GeminiModel: "test-model", SessionTTL: time.Hour, CookieSecure: true, Location: time.UTC}
	srv := httptest.NewServer(NewRouter(Deps{
		Config:       cfg,
		DB:           d,
		Logger:       discard,
		Auth:         auth.NewService(d, cfg.SessionTTL, discard),
		LoginLimiter: ratelimit.New(5, 15*time.Minute),
		Admin:        admin.NewService(d, 0, 0, time.UTC),
	}))
	t.Cleanup(srv.Close)

	user := createTestUser(t, d, "user@test.com", db.RoleUser, "password-user")
	createTestUser(t, d, "admin@test.com", db.RoleAdmin, "password-admin")

	t.Run("AC: tanpa cookie → 401", func(t *testing.T) {
		c := testClient{t, srv}
		c.expect(c.do("GET", "/api/auth/me", "", nil), 401, "unauthenticated")
		c.expect(c.do("POST", "/api/generate", "{}", nil), 401, "unauthenticated")
		c.expect(c.do("GET", "/api/admin/stats", "", nil), 401, "unauthenticated")
	})

	t.Run("login salah: pesan sama untuk email & password", func(t *testing.T) {
		c := testClient{t, srv}
		_, r1 := c.login("user@test.com", "salah-password")
		_, r2 := c.login("tidak-ada@test.com", "salah-password")
		c.expect(r1, 401, "unauthenticated")
		c.expect(r2, 401, "unauthenticated")
		if r1.body.Error != r2.body.Error {
			t.Errorf("pesan berbeda: %q vs %q", r1.body.Error, r2.body.Error)
		}
	})

	t.Run("validasi field login", func(t *testing.T) {
		c := testClient{t, srv}
		r := c.do("POST", "/api/auth/login", `{"email":"","password":""}`, nil)
		c.expect(r, 400, "invalid_input")
		if r.body.Fields["email"] == "" || r.body.Fields["password"] == "" {
			t.Errorf("fields = %v", r.body.Fields)
		}
	})

	var userCookie *http.Cookie
	t.Run("login user sukses + atribut cookie", func(t *testing.T) {
		c := testClient{t, srv}
		ck, r := c.login("  USER@test.com ", "password-user")
		c.expect(r, 200, "")
		if ck == nil || !ck.HttpOnly || !ck.Secure || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/" {
			t.Fatalf("cookie = %+v", ck)
		}
		var body userResponse
		_ = json.Unmarshal([]byte(r.raw), &body)
		if body.ID != user.ID || body.Role != "user" {
			t.Errorf("body = %+v", body)
		}
		// DB hanya menyimpan hash, bukan token.
		if n := countRows(t, d, `SELECT count(*) FROM sessions WHERE token_hash = $1`, []byte(ck.Value)); n != 0 {
			t.Error("token asli tersimpan di DB")
		}
		userCookie = ck
	})

	t.Run("me, akses generate, dan AC: user ke admin → 403", func(t *testing.T) {
		c := testClient{t, srv}
		c.expect(c.do("GET", "/api/auth/me", "", userCookie), 200, "")
		c.expect(c.do("POST", "/api/generate", "{}", userCookie), 400, "invalid_input") // lolos auth, ditolak validasi
		c.expect(c.do("GET", "/api/admin/stats", "", userCookie), 403, "forbidden")
	})

	t.Run("admin lolos ke /api/admin", func(t *testing.T) {
		c := testClient{t, srv}
		ck, r := c.login("admin@test.com", "password-admin")
		c.expect(r, 200, "")
		c.expect(c.do("GET", "/api/admin/stats", "", ck), 200, "")
	})

	t.Run("CSRF: tanpa X-Requested-With → 403", func(t *testing.T) {
		c := testClient{t, srv}
		r := c.do("POST", "/api/auth/login", `{"email":"user@test.com","password":"password-user"}`, nil,
			"X-Requested-With", "")
		c.expect(r, 403, "forbidden")
	})

	t.Run("body > 16 KB → 413", func(t *testing.T) {
		c := testClient{t, srv}
		big := `{"email":"user@test.com","password":"` + strings.Repeat("a", 17<<10) + `"}`
		c.expect(c.do("POST", "/api/auth/login", big, nil), 413, "payload_too_large")
	})

	t.Run("logout mencabut sesi", func(t *testing.T) {
		c := testClient{t, srv}
		ck, _ := c.login("user@test.com", "password-user")
		r := c.do("POST", "/api/auth/logout", "", ck)
		if r.status != 204 {
			t.Fatalf("logout status = %d", r.status)
		}
		c.expect(c.do("GET", "/api/auth/me", "", ck), 401, "unauthenticated")
	})

	t.Run("AC: user dinonaktifkan → 403 lalu 401, sesi dicabut", func(t *testing.T) {
		c := testClient{t, srv}
		if _, err := d.Pool.Exec(context.Background(),
			`UPDATE users SET is_active = false WHERE id = $1`, user.ID); err != nil {
			t.Fatal(err)
		}
		c.expect(c.do("GET", "/api/auth/me", "", userCookie), 403, "forbidden")
		if n := countRows(t, d, `SELECT count(*) FROM sessions WHERE user_id = $1`, user.ID); n != 0 {
			t.Errorf("sesi tersisa %d", n)
		}
		c.expect(c.do("GET", "/api/auth/me", "", userCookie), 401, "unauthenticated")
		_, r := c.login("user@test.com", "password-user")
		c.expect(r, 403, "forbidden")
	})

	t.Run("AC: login gagal ke-6 dalam 15 menit → 429 + audit", func(t *testing.T) {
		c := testClient{t, srv}
		for i := range 5 {
			_, r := c.login("admin@test.com", "salah")
			if r.status != 401 {
				t.Fatalf("percobaan %d: status %d", i+1, r.status)
			}
		}
		_, r := c.login("admin@test.com", "password-admin") // password benar pun ditolak
		c.expect(r, 429, "rate_limited")
		if ra := r.header.Get("Retry-After"); ra == "" || ra == "0" {
			t.Errorf("Retry-After = %q", ra)
		}
		if n := countRows(t, d, `SELECT count(*) FROM audit_logs
			WHERE action = 'login_rate_limited' AND metadata->>'email' = 'admin@test.com'`); n != 1 {
			t.Errorf("audit login_rate_limited = %d, want 1", n)
		}
	})

	t.Run("audit login tercatat", func(t *testing.T) {
		for action, min := range map[string]int{"login_success": 3, "login_failed": 7, "logout": 1} {
			if n := countRows(t, d, `SELECT count(*) FROM audit_logs WHERE action = $1`, action); n < min {
				t.Errorf("audit %s = %d, want >= %d", action, n, min)
			}
		}
		if n := countRows(t, d, `SELECT count(*) FROM audit_logs
			WHERE action = 'login_failed' AND actor_id IS NULL AND metadata->>'reason' = 'unknown_email'`); n != 1 {
			t.Errorf("login_failed unknown_email = %d, want 1", n)
		}
	})
}
