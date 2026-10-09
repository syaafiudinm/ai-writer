package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// maxBodyBytes adalah batas body setiap request (PRD: 16 KB).
const maxBodyBytes = 16 << 10

type ctxKey int

const (
	requestIDKey ctxKey = iota
	clientIPKey
	reqInfoKey
)

// RequestIDFrom mengambil request id dari context, kosong jika tidak ada.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// ClientIPFrom mengambil IP klien yang sudah ditentukan middleware clientIP.
func ClientIPFrom(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey).(string)
	return ip
}

// reqInfo diisi handler di dalam rantai dan dibaca logRequests setelah selesai.
type reqInfo struct {
	userID string
}

func setLogUserID(ctx context.Context, id string) {
	if info, ok := ctx.Value(reqInfoKey).(*reqInfo); ok {
		info.userID = id
	}
}

func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		id := hex.EncodeToString(b)

		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

func clientIP(trustProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := remoteIP(r, trustProxy)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey, ip)))
		})
	}
}

// remoteIP memakai X-Forwarded-For hanya jika TRUST_PROXY aktif. Entri paling
// kanan yang diambil karena itu ditambahkan proxy kita sendiri (asumsi satu
// hop: Nginx atau Cloud Run); entri di kirinya bisa dipalsukan klien.
func remoteIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if vals := r.Header.Values("X-Forwarded-For"); len(vals) > 0 {
			parts := strings.Split(vals[len(vals)-1], ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// statusRecorder mencatat status dan ukuran respons. Unwrap dan Flush wajib ada
// agar streaming SSE tetap bisa flush melewati middleware ini.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

func (s *statusRecorder) Flush() {
	_ = http.NewResponseController(s.ResponseWriter).Flush()
}

func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

func logRequests(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			info := &reqInfo{}
			next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), reqInfoKey, info)))

			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			level := slog.LevelInfo
			if rec.status >= 500 {
				level = slog.LevelError
			}
			attrs := []slog.Attr{
				slog.String("request_id", RequestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int("bytes", rec.bytes),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.String("ip", ClientIPFrom(r.Context())),
			}
			if info.userID != "" {
				attrs = append(attrs, slog.String("user_id", info.userID))
			}
			logger.LogAttrs(r.Context(), level, "request", attrs...)
		})
	}
}

func recoverPanic(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if v == http.ErrAbortHandler {
					panic(v)
				}
				logger.ErrorContext(r.Context(), "panic di handler",
					"request_id", RequestIDFrom(r.Context()),
					"panic", v,
					"stack", string(debug.Stack()),
				)
				// Jika header sudah terkirim (mis. stream berjalan), respons tidak bisa diganti.
				if rec, ok := w.(*statusRecorder); ok && rec.status != 0 {
					return
				}
				writeInternalError(w)
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// requireFetchHeader adalah perlindungan CSRF ringan: request yang mengubah data
// wajib membawa X-Requested-With: fetch, yang tidak bisa dikirim form lintas situs.
func requireFetchHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get("X-Requested-With") != "fetch" {
				writeError(w, http.StatusForbidden, "forbidden", "Permintaan ditolak.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}
