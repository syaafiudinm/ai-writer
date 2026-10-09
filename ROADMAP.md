# Roadmap Backend — AI Article Generator (Fase 1)

Checklist kerja per modul, diturunkan dari PRD Backend. Urutan mengikuti milestone M1–M5;
satu modul = satu unit kerja yang bisa dites terpisah.

## M1 — Fondasi

### 1. Config — `internal/config/config.go`
- [x] Baca semua env + default (`PORT`, `SESSION_TTL`, `GENERATE_TIMEOUT`, dst.)
- [x] Parse durasi, angka, bool, dan harga per MTok
- [x] Gagal start jika `ANTHROPIC_API_KEY` atau `DATABASE_URL` kosong
- [x] API key tidak pernah ikut tercetak (`String()` disensor)
- [x] Unit test (`config_test.go`)
- [x] `.env.example` berisi semua variabel

### 2. DB + migrasi — `internal/db/db.go`, `migrations/`
- [x] pgx pool + ping saat start
- [x] `00001_init.sql`: `users`, `sessions`, `generation_logs`, `audit_logs` + index
- [x] File SQL di-embed ke binary, goose jalan otomatis saat server start
- [ ] `docker-compose.yml` dengan Postgres lokal (file sudah ada, belum dicoba: Docker daemon mati)
- [x] Migrasi up/down dites ke Postgres lokal (Postgres 16 Homebrew)

### 3. Kerangka server — `cmd/server/main.go`, `internal/api/`
- [x] `log/slog` JSON ke stdout
- [x] Middleware: request id, request logging (method, path, status, durasi), recover panic
- [x] Helper `respond.go`: JSON sukses + error `{error, code, fields}`
- [x] `GET /api/health` → status server, database, model aktif
- [x] `http.Server` tanpa `WriteTimeout` global
- [x] Graceful shutdown: tangkap SIGTERM/SIGINT, tunggu 30 detik

## M2 — Autentikasi

### 4. Query auth — `internal/db/users.go`, `sessions.go`, `audit_logs.go`
- [ ] Users: get by email/id, create, update role/is_active/password, update `last_login_at`
- [ ] Sessions: create, get by token hash (join user), delete, delete semua milik user
- [ ] Audit logs: insert

### 5. Auth service — `internal/auth/service.go`
- [ ] bcrypt cost 12, password minimal 8 karakter
- [ ] Token sesi acak 32 byte; yang disimpan hanya hash SHA-256
- [ ] Login: pesan gagal tidak membedakan email vs password salah
- [ ] Logout / cabut sesi

### 6. Middleware — `internal/auth/middleware.go`, `internal/api/middleware.go`
- [ ] `RequireAuth`: sesi kedaluwarsa → 401; akun nonaktif → ditolak + sesi dicabut
- [ ] `RequireRole("admin")` → 403
- [ ] Cek header `X-Requested-With: fetch` untuk request yang mengubah data
- [ ] Batas body 16 KB (`http.MaxBytesReader`) → 413
- [ ] IP klien dari `X-Forwarded-For` hanya jika `TRUST_PROXY=true`
- [ ] User id ikut tercatat di log request

### 7. Rate limiter — `internal/ratelimit/limiter.go`
- [ ] Sliding window in-memory
- [ ] Login: 5 gagal / 15 menit per IP + email
- [ ] Generate: `RATE_LIMIT_PER_MIN` per user
- [ ] Lewat batas → 429 + header `Retry-After`
- [ ] Unit test

### 8. Handler auth + seed — `internal/api/auth_handler.go`, `cmd/seed/main.go`
- [ ] `POST /api/auth/login` → set cookie (`httpOnly`, `Secure`, `SameSite=Lax`), balas `{id, name, role}`
- [ ] `POST /api/auth/logout`, `GET /api/auth/me`
- [ ] Audit `login_success` / `login_failed`
- [ ] Perintah seed membuat admin pertama
- [ ] AC: tanpa cookie → 401; user biasa ke `/api/admin/*` → 403
- [ ] AC: user dinonaktifkan → request berikutnya 401/403
- [ ] AC: login gagal ke-6 dalam 15 menit → 429 dan tercatat di `audit_logs`

## M3 — Anthropic client & generate

### 9. Request + validasi — `internal/generate/request.go`
- [ ] Struct brief + default (`formal`, `sedang`, `id`)
- [ ] Validasi wajib, enum, batas panjang; error per field
- [ ] Table test

### 10. Prompt builder — `internal/generate/prompt.go`
- [ ] System prompt berita vs artikel sesuai aturan konten
- [ ] Input user dibungkus `<topik>`, `<kata_kunci>`, `<fakta>`
- [ ] `max_tokens` per panjang (1.200 / 2.400 / 4.000)
- [ ] Snapshot test

### 11. Anthropic client — `internal/anthropic/`
- [ ] Interface `Streamer`
- [ ] Request ke Messages API dengan `stream: true`
- [ ] Parser event SSE (`text_delta`, usage, `message_stop`, `error`)
- [ ] Pemetaan error: 4xx/5xx sebelum stream → `upstream_error`; 529/`overloaded_error` → `upstream_overloaded`
- [ ] Test dengan `httptest` server

### 12. SSE + handler generate — `internal/api/sse.go`, `generate_handler.go`, `internal/db/generation_logs.go`
- [ ] Header SSE + `X-Accel-Buffering: no`, flush per chunk
- [ ] Event `meta`, teks, `done`, `error`
- [ ] Error sebelum stream → JSON; sesudahnya → event SSE
- [ ] Timeout `GENERATE_TIMEOUT`; klien putus → context dibatalkan
- [ ] Catat TTFT, durasi, token, status ke `generation_logs` (sukses/error/cancelled/timeout)
- [ ] `facts` dan isi draf tidak disimpan maupun di-log
- [ ] AC: input tidak valid → 400 tanpa memanggil Anthropic
- [ ] AC: `curl -N` menampilkan teks bertahap, diakhiri `event: done`
- [ ] Graceful shutdown dites dengan stream aktif (stream selesai + log tersimpan)

## M4 — API admin

### 13. Statistik — `internal/admin/stats.go`
- [ ] Ringkasan + deret harian: generate, user aktif, token in/out, estimasi biaya, error rate, rata-rata durasi
- [ ] Biaya dihitung saat query dari `PRICE_*_PER_MTOK`
- [ ] AC: angka 30 hari cocok dengan hitungan manual

### 14. Handler admin — `internal/api/admin_handler.go`
- [ ] `GET /api/admin/stats?from=&to=`
- [ ] `GET /api/admin/logs` dengan filter + paginasi 50
- [ ] `GET /api/admin/audit?action=&page=`
- [ ] `GET` / `POST /api/admin/users`
- [ ] `PATCH /api/admin/users/{id}`: role, is_active, password baru (cabut sesi saat nonaktif/reset)
- [ ] Semua aksi admin tercatat di `audit_logs`

## M5 — Siap deploy

### 15. Retensi log — `internal/jobs/retention.go`
- [ ] Job harian hapus `generation_logs` > `LOG_RETENTION_DAYS`

### 16. Static + CORS
- [ ] Sajikan `STATIC_DIR` dengan fallback SPA ke `index.html`
- [ ] CORS hanya untuk `ALLOWED_ORIGIN` dengan credentials

### 17. Paket & dokumentasi
- [ ] `Dockerfile` multi-stage: build React + Go → distroless
- [ ] Service `app` di `docker-compose.yml`
- [ ] `README.md`: setup lokal, env, seed, deploy (Cloud Run / Nginx `proxy_buffering off`)
- [ ] Uji manual 10 brief berita + 10 artikel
- [ ] AC: API key, password, token sesi, dan `facts` tidak muncul di log maupun response
