package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestHealth(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
		wantBody   healthResponse
	}{
		{"db ok", nil, http.StatusOK, healthResponse{"ok", "ok", "claude-sonnet-5-5"}},
		{"db down", errors.New("conn refused"), http.StatusServiceUnavailable, healthResponse{"degraded", "unreachable", "claude-sonnet-5-5"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			healthHandler(fakePinger{tt.pingErr}, "claude-sonnet-5-5", discard).
				ServeHTTP(rec, httptest.NewRequest("GET", "/api/health", nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			var got healthResponse
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got != tt.wantBody {
				t.Errorf("body = %+v, want %+v", got, tt.wantBody)
			}
		})
	}
}

func TestMiddlewareChain(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /panic", func(http.ResponseWriter, *http.Request) { panic("boom") })
	mux.HandleFunc("GET /stream", func(w http.ResponseWriter, r *http.Request) {
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush lewat middleware gagal: %v", err)
		}
	})
	h := chain(mux, requestID, logRequests(discard, false), recoverPanic(discard))

	t.Run("request id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/stream", nil))
		if len(rec.Header().Get("X-Request-ID")) != 16 {
			t.Errorf("X-Request-ID = %q", rec.Header().Get("X-Request-ID"))
		}
		if !rec.Flushed {
			t.Error("respons tidak ter-flush")
		}
	})

	t.Run("panic jadi 500 JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/panic", nil))
		var body errorBody
		_ = json.NewDecoder(rec.Body).Decode(&body)
		if rec.Code != http.StatusInternalServerError || body.Code != "internal_error" {
			t.Errorf("status = %d, code = %q", rec.Code, body.Code)
		}
	})
}

func TestRemoteIP(t *testing.T) {
	tests := []struct {
		name  string
		trust bool
		hops  int
		xff   []string
		want  string
	}{
		{"tanpa proxy abaikan XFF", false, 1, []string{"9.9.9.9"}, "10.0.0.1"},
		{"1 hop: entri paling kanan", true, 1, []string{"6.6.6.6, 203.0.113.7"}, "203.0.113.7"},
		{"1 hop: beberapa header", true, 1, []string{"6.6.6.6", "203.0.113.8"}, "203.0.113.8"},
		{"Railway 2 hop: klien sebelum IP edge", true, 2, []string{"125.162.209.138, 152.233.68.98"}, "125.162.209.138"},
		{"Railway 2 hop: XFF palsu di kiri diabaikan", true, 2, []string{"6.6.6.6, 125.162.209.138, 152.233.15.123"}, "125.162.209.138"},
		{"2 hop tapi entri kurang → RemoteAddr", true, 2, []string{"6.6.6.6"}, "10.0.0.1"},
		{"proxy: XFF sampah", true, 1, []string{"bukan-ip"}, "10.0.0.1"},
		{"proxy: tanpa XFF", true, 1, nil, "10.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = "10.0.0.1:5555"
			for _, v := range tt.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := remoteIP(r, tt.trust, tt.hops); got != tt.want {
				t.Errorf("remoteIP = %q, want %q", got, tt.want)
			}
		})
	}
}
