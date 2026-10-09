package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Chunk diambil dari bentuk respons asli gemini-3.5-flash (thoughtSignature dipendekkan).
const (
	chunk1 = `{"candidates": [{"content": {"parts": [{"text": "# Judul\n\nParagraf "}],"role": "model"},"index": 0}],"usageMetadata": {"promptTokenCount": 20,"candidatesTokenCount": 14,"totalTokenCount": 34}}`
	chunk2 = `{"candidates": [{"content": {"parts": [{"text": "pertama."}],"role": "model"},"index": 0}],"usageMetadata": {"promptTokenCount": 20,"candidatesTokenCount": 19,"totalTokenCount": 245,"thoughtsTokenCount": 206}}`
	final  = `{"candidates": [{"content": {"parts": [{"text": "","thoughtSignature": "EuoGCucG"}],"role": "model"},"finishReason": "STOP","index": 0}],"usageMetadata": {"promptTokenCount": 20,"candidatesTokenCount": 19,"totalTokenCount": 245,"thoughtsTokenCount": 206}}`
)

func sse(chunks ...string) string {
	var b strings.Builder
	for _, c := range chunks {
		fmt.Fprintf(&b, "data: %s\r\n\r\n", c)
	}
	return b.String()
}

type captured struct {
	path, query, apiKey string
	body                requestBody
}

func newServer(t *testing.T, status int, body string) (*Client, *captured) {
	t.Helper()
	got := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path, got.query, got.apiKey = r.URL.Path, r.URL.RawQuery, r.Header.Get("x-goog-api-key")
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return New("AIza-test", "gemini-test", WithBaseURL(srv.URL)), got
}

func collect(c *Client, req Request) (string, Usage, error) {
	var text strings.Builder
	u, err := c.Stream(context.Background(), req, func(s string) error {
		text.WriteString(s)
		return nil
	})
	return text.String(), u, err
}

func TestStreamSuccess(t *testing.T) {
	c, got := newServer(t, 200, sse(chunk1, chunk2, final))
	text, u, err := collect(c, Request{System: "sys", Prompt: "halo", MaxOutputTokens: 1200, ThinkingLevel: "minimal"})
	if err != nil {
		t.Fatal(err)
	}
	if text != "# Judul\n\nParagraf pertama." {
		t.Errorf("text = %q", text)
	}
	if u != (Usage{InputTokens: 20, OutputTokens: 19 + 206, StopReason: "stop"}) {
		t.Errorf("usage = %+v", u)
	}

	if got.path != "/models/gemini-test:streamGenerateContent" || got.query != "alt=sse" {
		t.Errorf("url = %s?%s", got.path, got.query)
	}
	if got.apiKey != "AIza-test" || strings.Contains(got.query, "AIza") {
		t.Error("API key harus lewat header, bukan query")
	}
	b := got.body
	if b.SystemInstruction == nil || b.SystemInstruction.Parts[0].Text != "sys" ||
		b.Contents[0].Role != "user" || b.Contents[0].Parts[0].Text != "halo" ||
		b.GenerationConfig.MaxOutputTokens != 1200 || b.GenerationConfig.ThinkingConfig.ThinkingLevel != "minimal" {
		t.Errorf("body = %+v", b)
	}
}

func TestStreamMaxTokensIsNotError(t *testing.T) {
	c, _ := newServer(t, 200, sse(chunk1,
		`{"candidates":[{"content":{"parts":[{"text":"x"}]},"finishReason":"MAX_TOKENS"}]}`))
	_, u, err := collect(c, Request{Prompt: "p"})
	if err != nil || u.StopReason != "max_tokens" {
		t.Errorf("err=%v stop=%q", err, u.StopReason)
	}
}

func TestStreamErrors(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		kind     ErrorKind
		wantText string
		msg      string
	}{
		{"503 sebelum stream", 503,
			`{"error":{"code":503,"message":"This model is currently experiencing high demand.","status":"UNAVAILABLE"}}`,
			KindOverloaded, "", "UNAVAILABLE: This model"},
		{"429 kuota", 429,
			`{"error":{"code":429,"message":"Quota exceeded","status":"RESOURCE_EXHAUSTED"}}`,
			KindOverloaded, "", "RESOURCE_EXHAUSTED"},
		{"400 argumen salah", 400,
			`{"error":{"code":400,"message":"Thinking level MINIMAL is not supported","status":"INVALID_ARGUMENT"}}`,
			KindUpstream, "", "INVALID_ARGUMENT"},
		{"404 body html kosong", 404, ``, KindUpstream, "", ""},
		{"prompt diblokir", 200,
			sse(`{"promptFeedback":{"blockReason":"SAFETY"},"usageMetadata":{"promptTokenCount":9}}`),
			KindBlocked, "", "prompt diblokir: SAFETY"},
		{"jawaban dihentikan SAFETY", 200,
			sse(chunk1, `{"candidates":[{"content":{"parts":[{"text":""}]},"finishReason":"SAFETY"}]}`),
			KindBlocked, "# Judul\n\nParagraf ", "SAFETY"},
		{"error di tengah stream", 200,
			sse(chunk1, `{"error":{"code":503,"message":"overloaded","status":"UNAVAILABLE"}}`),
			KindOverloaded, "# Judul\n\nParagraf ", "UNAVAILABLE"},
		{"stream terputus tanpa finishReason", 200, sse(chunk1, chunk2),
			KindUpstream, "# Judul\n\nParagraf pertama.", "tanpa finishReason"},
		{"chunk rusak", 200, sse(`{"candidates": [`), KindUpstream, "", "bukan JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newServer(t, tt.status, tt.body)
			text, _, err := collect(c, Request{Prompt: "p"})
			var gerr *Error
			if !errors.As(err, &gerr) || gerr.Kind != tt.kind {
				t.Fatalf("err = %#v, want kind %d", err, tt.kind)
			}
			if tt.msg != "" && !strings.Contains(gerr.Message, tt.msg) {
				t.Errorf("message = %q, want contains %q", gerr.Message, tt.msg)
			}
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
		})
	}
}

func TestStreamPartialUsageOnError(t *testing.T) {
	c, _ := newServer(t, 200, sse(chunk1, chunk2))
	_, u, err := collect(c, Request{Prompt: "p"})
	if err == nil || u.InputTokens != 20 || u.OutputTokens != 225 {
		t.Errorf("err=%v usage=%+v", err, u)
	}
}

func TestStreamOnTextErrorStops(t *testing.T) {
	c, _ := newServer(t, 200, sse(chunk1, chunk2, final))
	stop := errors.New("klien putus")
	calls := 0
	_, err := c.Stream(context.Background(), Request{Prompt: "p"}, func(string) error {
		calls++
		return stop
	})
	if !errors.Is(err, stop) || calls != 1 {
		t.Errorf("err=%v calls=%d", err, calls)
	}
}

func TestStreamContextCancel(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprintf(w, "data: %s\n\n", chunk1)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done() // menggantung seperti Gemini yang lambat
	}))
	t.Cleanup(srv.Close)
	c := New("k", "m", WithBaseURL(srv.URL))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := c.Stream(ctx, Request{Prompt: "p"}, func(string) error { return nil })
	<-started
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
}

func TestStreamConnectionRefused(t *testing.T) {
	c := New("k", "m", WithBaseURL("http://127.0.0.1:1"))
	_, err := c.Stream(context.Background(), Request{Prompt: "p"}, func(string) error { return nil })
	if !IsKind(err, KindOverloaded) {
		t.Errorf("err = %v", err)
	}
}
