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

func clientIP(trustProxy bool, hops int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := remoteIP(r, trustProxy, hops)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey, ip)))
		})
	}
}

// remoteIP memakai X-Forwarded-For hanya jika TRUST_PROXY aktif. Setiap proxy
// menambahkan alamat pengirimnya di kanan, jadi IP klien ada di entri ke-hops
// dari kanan (hops = jumlah proxy tepercaya; Caddy/Nginx = 1, Railway = 2).
// Entri di kirinya bisa dipalsukan klien. Jika entri lebih sedikit dari hops,
// request tidak lewat jalur proxy yang diharapkan, jadi RemoteAddr yang dipakai.
func remoteIP(r *http.Request, trustProxy bool, hops int) string {
	if trustProxy && hops > 0 {
		var parts []string
		for _, v := range r.Header.Values("X-Forwarded-For") {
			for _, p := range strings.Split(v, ",") {
				if p = strings.TrimSpace(p); p != "" {
					parts = append(parts, p)
				}
			}
		}
		if len(parts) >= hops {
			if ip := parts[len(parts)-hops]; net.ParseIP(ip) != nil {
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

// logRequests mencatat setiap request. logProxyHeaders (sementara, untuk
// diagnosis TRUST_PROXY_HOPS) ikut mencatat header proxy mentah.
func logRequests(logger *slog.Logger, logProxyHeaders bool) func(http.Handler) http.Handler {
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
			if logProxyHeaders {
				attrs = append(attrs,
					slog.String("remote_addr", r.RemoteAddr),
					slog.String("x_forwarded_for", strings.Join(r.Header.Values("X-Forwarded-For"), " | ")),
					slog.String("x_real_ip", r.Header.Get("X-Real-IP")))
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
