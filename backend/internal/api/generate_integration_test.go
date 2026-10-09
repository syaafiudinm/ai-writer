package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ai-writer/internal/auth"
	"ai-writer/internal/config"
	"ai-writer/internal/db"
	"ai-writer/internal/gemini"
	"ai-writer/internal/ratelimit"
)

const secretFacts = "- Kutipan rahasia: \"Proyek ini rampung 2027\" kata Kadishub"

var beritaBody = `{"type":"berita","topic":"Pembukaan jalur bus listrik baru di Makassar",` +
	`"keywords":"bus listrik","facts":` + mustJSON(secretFacts) + `}`

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// fakeStreamer meniru Gemini: mengirim chunks, lalu mengembalikan err atau menggantung.
type fakeStreamer struct {
	chunks  []string
	delay   time.Duration
	usage   gemini.Usage
	err     error
	hang    bool          // tunggu sampai context selesai setelah chunks
	started chan struct{} // ditutup saat Stream dipanggil (opsional)

	mu    sync.Mutex
	calls int
	req   gemini.Request
}

func (f *fakeStreamer) Stream(ctx context.Context, req gemini.Request, onText func(string) error) (gemini.Usage, error) {
	f.mu.Lock()
	f.calls++
	f.req = req
	f.mu.Unlock()
	if f.started != nil {
		close(f.started)
	}
	for _, c := range f.chunks {
		if f.delay > 0 {
			select {
			case <-time.After(f.delay):
			case <-ctx.Done():
				return f.usage, ctx.Err()
			}
		}
		if err := onText(c); err != nil {
			return f.usage, err
		}
	}
	if f.hang {
		<-ctx.Done()
		return f.usage, ctx.Err()
	}
	return f.usage, f.err
}

func (f *fakeStreamer) snapshot() (int, gemini.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.req
}

type genEnv struct {
	c      testClient
	srv    *httptest.Server
	d      *db.DB
	cookie *http.Cookie
	logs   *syncBuffer
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func newGenEnv(t *testing.T, f *fakeStreamer, edit func(*config.Config, *Deps)) genEnv {
	t.Helper()
	d := testDB(t)
	logs := &syncBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	cfg := &config.Config{
		GeminiModel: "gemini-test", GeminiThinkingLevel: "minimal", SessionTTL: time.Hour,
		GenerateTimeout: 5 * time.Second,
	}
	deps := Deps{
		Config: cfg, DB: d, Logger: logger,
		Auth:            auth.NewService(d, cfg.SessionTTL, logger),
		LoginLimiter:    ratelimit.New(5, 15*time.Minute),
		GenerateLimiter: ratelimit.New(10, time.Minute),
		Streamer:        f,
	}
	if edit != nil {
		edit(cfg, &deps)
	}
	srv := httptest.NewServer(NewRouter(deps))
	t.Cleanup(srv.Close)

	c := testClient{t: t, srv: srv}
	createTestUser(t, d, "penulis@test.com", db.RoleUser, "password-penulis")
	ck, r := c.login("penulis@test.com", "password-penulis")
	c.expect(r, 200, "")
	return genEnv{c: c, srv: srv, d: d, cookie: ck, logs: logs}
}

type sseEvent struct {
	name string
	data map[string]any
}

func parseSSE(t *testing.T, raw string) []sseEvent {
	t.Helper()
	var events []sseEvent
	for _, block := range strings.Split(strings.TrimSpace(raw), "\n\n") {
		ev := sseEvent{name: "message"}
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev.data); err != nil {
					t.Fatalf("data bukan JSON: %q", line)
				}
			}
		}
		events = append(events, ev)
	}
	return events
}

type logRow struct {
	status, errorCode, topic, model string
	inTok, outTok                   int
	ttft                            *int
	duration                        int
}

// lastLog menunggu sampai baris generation_logs muncul (log ditulis setelah respons selesai).
func lastLog(t *testing.T, d *db.DB) logRow {
	t.Helper()
	var r logRow
	deadline := time.Now().Add(3 * time.Second)
	for {
		var ec *string
		err := d.Pool.QueryRow(context.Background(),
			`SELECT status, error_code, topic, model, input_tokens, output_tokens, ttft_ms, duration_ms
			 FROM generation_logs ORDER BY id DESC LIMIT 1`).
			Scan(&r.status, &ec, &r.topic, &r.model, &r.inTok, &r.outTok, &r.ttft, &r.duration)
		if err == nil {
			if ec != nil {
				r.errorCode = *ec
			}
			return r
		}
		if time.Now().After(deadline) {
			t.Fatalf("generation_logs kosong: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestGenerateSuccess(t *testing.T) {
	f := &fakeStreamer{
		chunks: []string{"# Bus Listrik ", "Rute Losari–Bandara\n\n"},
		usage:  gemini.Usage{InputTokens: 612, OutputTokens: 1180, StopReason: "stop"},
	}
	env := newGenEnv(t, f, nil)

	r := env.c.do("POST", "/api/generate", beritaBody, env.cookie)
	if r.status != 200 || !strings.HasPrefix(r.header.Get("Content-Type"), "text/event-stream") ||
		r.header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("status=%d headers=%v body=%s", r.status, r.header, r.raw)
	}
	events := parseSSE(t, r.raw)
	if len(events) != 4 || events[0].name != "meta" || events[0].data["model"] != "gemini-test" ||
		events[1].data["text"] != "# Bus Listrik " || events[2].data["text"] != "Rute Losari–Bandara\n\n" ||
		events[3].name != "done" {
		t.Fatalf("events = %+v", events)
	}
	done := events[3].data
	if done["stop_reason"] != "stop" || done["input_tokens"] != 612.0 || done["output_tokens"] != 1180.0 {
		t.Errorf("done = %v", done)
	}

	_, req := f.snapshot()
	if req.MaxOutputTokens != 2400 || req.ThinkingLevel != "minimal" ||
		!strings.Contains(req.System, "editor berita") || !strings.Contains(req.Prompt, secretFacts) {
		t.Errorf("request ke Gemini = %+v", req)
	}

	row := lastLog(t, env.d)
	if row.status != "success" || row.errorCode != "" || row.model != "gemini-test" ||
		row.topic != "Pembukaan jalur bus listrik baru di Makassar" ||
		row.inTok != 612 || row.outTok != 1180 || row.ttft == nil {
		t.Errorf("log = %+v", row)
	}
	if strings.Contains(env.logs.String(), "Kadishub") {
		t.Error("isi facts bocor ke log aplikasi")
	}
}

func TestGenerateThinkingAddsAllowance(t *testing.T) {
	f := &fakeStreamer{chunks: []string{"x"}, usage: gemini.Usage{StopReason: "stop"}}
	env := newGenEnv(t, f, func(c *config.Config, _ *Deps) { c.GeminiThinkingLevel = "low" })
	env.c.do("POST", "/api/generate", beritaBody, env.cookie)
	if _, req := f.snapshot(); req.MaxOutputTokens != 2400+thinkingAllowance || req.ThinkingLevel != "low" {
		t.Errorf("req = %+v", req)
	}
}

func TestGenerateInvalidInputSkipsGemini(t *testing.T) {
	f := &fakeStreamer{}
	env := newGenEnv(t, f, nil)
	r := env.c.do("POST", "/api/generate", `{"type":"opini","topic":"abc","language":"jv"}`, env.cookie)
	env.c.expect(r, 400, "invalid_input")
	for _, field := range []string{"type", "topic", "language"} {
		if r.body.Fields[field] == "" {
			t.Errorf("fields tidak memuat %s: %v", field, r.body.Fields)
		}
	}
	if calls, _ := f.snapshot(); calls != 0 {
		t.Errorf("Gemini dipanggil %d kali", calls)
	}
	if n := countRows(t, env.d, `SELECT count(*) FROM generation_logs`); n != 0 {
		t.Errorf("input tidak valid tidak boleh dicatat, dapat %d", n)
	}
}

func TestGenerateRateLimit(t *testing.T) {
	f := &fakeStreamer{chunks: []string{"x"}, usage: gemini.Usage{StopReason: "stop"}}
	env := newGenEnv(t, f, func(_ *config.Config, d *Deps) { d.GenerateLimiter = ratelimit.New(2, time.Minute) })
	for i := range 2 {
		if r := env.c.do("POST", "/api/generate", beritaBody, env.cookie); r.status != 200 {
			t.Fatalf("request %d: %d", i+1, r.status)
		}
	}
	r := env.c.do("POST", "/api/generate", beritaBody, env.cookie)
	env.c.expect(r, 429, "rate_limited")
	if r.header.Get("Retry-After") == "" {
		t.Error("Retry-After kosong")
	}
	if calls, _ := f.snapshot(); calls != 2 {
		t.Errorf("Gemini dipanggil %d kali, want 2", calls)
	}
}

func TestGenerateErrorsBeforeStreamAreJSON(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"overloaded", &gemini.Error{Kind: gemini.KindOverloaded, Status: 503, Message: "UNAVAILABLE: pesan-mentah-gemini"},
			503, "upstream_overloaded"},
		{"upstream", &gemini.Error{Kind: gemini.KindUpstream, Status: 400, Message: "INVALID_ARGUMENT: pesan-mentah-gemini"},
			502, "upstream_error"},
		{"blocked", &gemini.Error{Kind: gemini.KindBlocked, Message: "prompt diblokir: SAFETY"},
			422, "content_blocked"},
		{"stream kosong", nil, 502, "upstream_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newGenEnv(t, &fakeStreamer{err: tt.err}, nil)
			r := env.c.do("POST", "/api/generate", beritaBody, env.cookie)
			env.c.expect(r, tt.status, tt.code)
			if strings.Contains(r.raw, "pesan-mentah-gemini") {
				t.Error("pesan mentah Gemini bocor ke klien")
			}
			if row := lastLog(t, env.d); row.status != "error" || row.errorCode != tt.code || row.ttft != nil {
				t.Errorf("log = %+v", row)
			}
		})
	}
}

func TestGenerateErrorMidStreamIsSSE(t *testing.T) {
	f := &fakeStreamer{
		chunks: []string{"# Judul\n"},
		usage:  gemini.Usage{InputTokens: 50, OutputTokens: 10},
		err:    &gemini.Error{Kind: gemini.KindOverloaded, Message: "UNAVAILABLE"},
	}
	env := newGenEnv(t, f, nil)
	r := env.c.do("POST", "/api/generate", beritaBody, env.cookie)
	events := parseSSE(t, r.raw)
	last := events[len(events)-1]
	if r.status != 200 || last.name != "error" || last.data["code"] != "upstream_overloaded" ||
		last.data["error"] != "Layanan AI sedang sibuk, coba lagi sebentar." {
		t.Fatalf("status=%d events=%+v", r.status, events)
	}
	if row := lastLog(t, env.d); row.status != "error" || row.inTok != 50 || row.outTok != 10 || row.ttft == nil {
		t.Errorf("log = %+v", row)
	}
}

func TestGenerateTimeout(t *testing.T) {
	short := func(c *config.Config, _ *Deps) { c.GenerateTimeout = 200 * time.Millisecond }

	t.Run("sebelum teks pertama → 504 JSON", func(t *testing.T) {
		env := newGenEnv(t, &fakeStreamer{hang: true}, short)
		env.c.expect(env.c.do("POST", "/api/generate", beritaBody, env.cookie), 504, "timeout")
		if row := lastLog(t, env.d); row.status != "timeout" || row.errorCode != "timeout" {
			t.Errorf("log = %+v", row)
		}
	})
	t.Run("di tengah stream → event error", func(t *testing.T) {
		env := newGenEnv(t, &fakeStreamer{chunks: []string{"awal"}, hang: true}, short)
		events := parseSSE(t, env.c.do("POST", "/api/generate", beritaBody, env.cookie).raw)
		if last := events[len(events)-1]; last.name != "error" || last.data["code"] != "timeout" {
			t.Errorf("events = %+v", events)
		}
		if row := lastLog(t, env.d); row.status != "timeout" {
			t.Errorf("log = %+v", row)
		}
	})
}

func TestGenerateClientCancel(t *testing.T) {
	f := &fakeStreamer{chunks: []string{"satu", "dua", "tiga", "empat"}, delay: 300 * time.Millisecond,
		usage: gemini.Usage{InputTokens: 7}}
	env := newGenEnv(t, f, nil)

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "POST", env.srv.URL+"/api/generate", strings.NewReader(beritaBody))
	req.Header.Set("X-Requested-With", "fetch")
	req.AddCookie(env.cookie)
	res, err := env.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	// Baca sampai teks pertama muncul, lalu putuskan koneksi seperti tombol Stop.
	br := bufio.NewReader(res.Body)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(line, `"text"`) {
			break
		}
	}
	cancel()
	res.Body.Close()

	if row := lastLog(t, env.d); row.status != "cancelled" || row.errorCode != "cancelled" || row.inTok != 7 {
		t.Errorf("log = %+v", row)
	}
}

func TestGracefulShutdownFinishesActiveStream(t *testing.T) {
	f := &fakeStreamer{
		chunks:  []string{"a", "b", "c"},
		delay:   300 * time.Millisecond,
		usage:   gemini.Usage{StopReason: "stop"},
		started: make(chan struct{}),
	}
	env := newGenEnv(t, f, nil)

	type result struct {
		r resp
	}
	done := make(chan result)
	go func() { done <- result{env.c.do("POST", "/api/generate", beritaBody, env.cookie)} }()

	<-f.started
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := env.srv.Config.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	res := <-done
	events := parseSSE(t, res.r.raw)
	if last := events[len(events)-1]; last.name != "done" {
		t.Fatalf("stream tidak selesai: %+v", events)
	}
	if row := lastLog(t, env.d); row.status != "success" {
		t.Errorf("log = %+v", row)
	}
}
