# Roadmap Backend — AI Article Generator (Fase 1)

Checklist kerja per modul, diturunkan dari PRD Backend. Urutan mengikuti milestone M1–M5;
satu modul = satu unit kerja yang bisa dites terpisah.

## Perbedaan dari PRD

| Area | PRD | Implementasi | Alasan |
|---|---|---|---|
| Provider AI | Anthropic Messages API | Gemini API (Google AI Studio) | Keputusan user. Env: `GEMINI_API_KEY`, `GEMINI_MODEL`, `GEMINI_THINKING_LEVEL` |
| Model default | `claude-sonnet-5-5` | `gemini-3.5-flash` + thinking `minimal` | `gemini-3.8-flash` sering 503 dan TTFT 4–20 dtk saat dites; thinking memakan jatah `maxOutputTokens` |
| Interface | `anthropic.Request` | `gemini.Request` | Bentuk `Streamer` sama |
| Kode error | `upstream_overloaded` saat 529 | 429 dan 5xx dari Gemini | Free tier cepat kena 429 |
| Kode error baru | — | `content_blocked` (422 / event SSE) | Filter keamanan Gemini (`SAFETY`, dll.) |
| `stop_reason` | `end_turn` | `stop`, `max_tokens`, … (finishReason Gemini, huruf kecil) | |
| `output_tokens` | token output | teks + thinking | Thinking ditagih sebagai output |
| Penanda fakta (en) | `[perlu konfirmasi]` | `[needs confirmation: ...]` untuk draf bahasa Inggris | Mengikuti bahasa draf |
| Audit | login, aksi admin | + `login_rate_limited`, `logout` | |
| Respons login/me | `{id, name, role}` | + `email` | |
| Env baru | — | `TIMEZONE` (default `Asia/Makassar`) | Batas "hari" di statistik harian |
| Env baru | `TRUST_PROXY` saja | + `TRUST_PROXY_HOPS` (Caddy = 1, Railway = 2), `LOG_PROXY_HEADERS` (diagnosis sementara) | Railway memakai 2 lapis proxy; tanpa ini IP yang tercatat adalah IP edge Railway |
| Kode error baru | — | 409 `email_taken`, `self_lockout`, `last_admin`; 404 `not_found` | Kelola user |
| Audit baru | `user_created`, `user_disabled`, … | + `user_enabled`, `user_role_changed`, `user_password_reset` | |
| Statistik | jumlah, user aktif, token, biaya, error rate, durasi | + `p50_ttft_ms`, `by_status`; error rate = (error + timeout) / total | Metrik keberhasilan PRD (TTFT p50, ≥ 98% tanpa error) |

Catatan privasi: free tier Google AI Studio boleh memakai input untuk melatih model. Aktifkan billing sebelum dipakai dengan `facts` asli.

## M1 — Fondasi

### 1. Config — `internal/config/config.go`
- [x] Baca semua env + default (`PORT`, `SESSION_TTL`, `GENERATE_TIMEOUT`, dst.)
- [x] Parse durasi, angka, bool, dan harga per MTok
- [x] Gagal start jika `GEMINI_API_KEY` atau `DATABASE_URL` kosong
- [x] API key tidak pernah ikut tercetak (`String()` disensor)
- [x] Unit test (`config_test.go`)
- [x] `.env.example` berisi semua variabel

### 2. DB + migrasi — `internal/db/db.go`, `migrations/`
- [x] pgx pool + ping saat start
- [x] `00001_init.sql`: `users`, `sessions`, `generation_logs`, `audit_logs` + index
- [x] File SQL di-embed ke binary, goose jalan otomatis saat server start
- [x] `docker-compose.yml` dengan Postgres 17 lokal (port host 5433)
- [x] Migrasi up/down dites ke Postgres lokal (Docker Postgres 17)

### 3. Kerangka server — `cmd/server/main.go`, `internal/api/`
- [x] `log/slog` JSON ke stdout
- [x] Middleware: request id, request logging (method, path, status, durasi), recover panic
- [x] Helper `respond.go`: JSON sukses + error `{error, code, fields}`
- [x] `GET /api/health` → status server, database, model aktif
- [x] `http.Server` tanpa `WriteTimeout` global
- [x] Graceful shutdown: tangkap SIGTERM/SIGINT, tunggu 30 detik

## M2 — Autentikasi

### 4. Query auth — `internal/db/users.go`, `sessions.go`, `audit_logs.go`
- [x] Users: get by email/id, create, update `last_login_at` (update role/is_active/password → M4)
- [x] Sessions: create, get by token hash (join user), delete, delete semua milik user
- [x] Audit logs: insert

### 5. Auth service — `internal/auth/service.go`
- [x] bcrypt cost 12, password minimal 8 karakter
- [x] Token sesi acak 32 byte; yang disimpan hanya hash SHA-256
- [x] Login: pesan gagal tidak membedakan email vs password salah
- [x] Logout / cabut sesi

### 6. Middleware — `internal/auth/middleware.go`, `internal/api/middleware.go`
- [x] `RequireAuth`: sesi kedaluwarsa → 401; akun nonaktif → ditolak + sesi dicabut
- [x] `RequireRole("admin")` → 403
- [x] Cek header `X-Requested-With: fetch` untuk request yang mengubah data
- [x] Batas body 16 KB (`http.MaxBytesReader`) → 413
- [x] IP klien dari `X-Forwarded-For` hanya jika `TRUST_PROXY=true`
- [x] User id ikut tercatat di log request

### 7. Rate limiter — `internal/ratelimit/limiter.go`
- [x] Sliding window in-memory
- [x] Login: 5 gagal / 15 menit per IP + email
- [x] Generate: `RATE_LIMIT_PER_MIN` per user (`Allow` siap; dipasang di handler M3)
- [x] Lewat batas → 429 + header `Retry-After`
- [x] Unit test
- [x] Test integrasi semua AC M2 (`TEST_DATABASE_URL`, DB `aiwriter_test`)

### 8. Handler auth + seed — `internal/api/auth_handler.go`, `cmd/seed/main.go`
- [x] `POST /api/auth/login` → set cookie (`httpOnly`, `Secure`, `SameSite=Lax`), balas `{id, name, role}`
- [x] `POST /api/auth/logout`, `GET /api/auth/me`
- [x] Audit `login_success` / `login_failed`
- [x] Perintah seed membuat admin pertama
- [x] AC: tanpa cookie → 401; user biasa ke `/api/admin/*` → 403
- [x] AC: user dinonaktifkan → request berikutnya 401/403
- [x] AC: login gagal ke-6 dalam 15 menit → 429 dan tercatat di `audit_logs`

## M3 — Gemini client & generate

### 9. Request + validasi — `internal/generate/request.go`
- [x] Struct brief + default (`formal`, `sedang`, `id`)
- [x] Validasi wajib, enum, batas panjang; error per field
- [x] Table test

### 10. Prompt builder — `internal/generate/prompt.go`
- [x] System prompt berita vs artikel sesuai aturan konten
- [x] Input user dibungkus `<topik>`, `<kata_kunci>`, `<fakta>`; tag palsu di input dinetralkan
- [x] `max_tokens` per panjang (1.200 / 2.400 / 4.000) + ruang thinking jika aktif
- [x] Snapshot test (`go test ./internal/generate -update` untuk memperbarui)
- [ ] Tuning lanjutan saat QA M5: model masih sesekali menambah tujuan/opini di judul dan catatan "belum ada informasi" di badan berita

### 11. Gemini client — `internal/gemini/`
- [x] Interface `Streamer`
- [x] `streamGenerateContent?alt=sse`, key lewat header `x-goog-api-key`
- [x] Parser SSE (teks, usage kumulatif, `finishReason`, `promptFeedback`, error di tengah stream)
- [x] Pemetaan error: 429/5xx → `upstream_overloaded`; 4xx lain → `upstream_error`; filter keamanan → `content_blocked`
- [x] Test dengan `httptest` server memakai bentuk chunk asli

### 12. SSE + handler generate — `internal/api/sse.go`, `generate_handler.go`, `internal/db/generation_logs.go`
- [x] Pasang rate limit generate per user (`ratelimit.Limiter.Allow`)
- [x] Header SSE + `X-Accel-Buffering: no`, flush per chunk
- [x] Event `meta`, teks, `done`, `error`
- [x] Error sebelum teks pertama → JSON; sesudahnya → event SSE
- [x] Timeout `GENERATE_TIMEOUT`; klien putus → context dibatalkan
- [x] Catat TTFT, durasi, token, status ke `generation_logs` (sukses/error/cancelled/timeout)
- [x] `facts` dan isi draf tidak disimpan maupun di-log
- [x] AC: input tidak valid → 400 tanpa memanggil Gemini
- [x] AC: `curl -N` menampilkan teks bertahap, diakhiri `event: done` (dites ke Gemini asli)
- [x] Graceful shutdown dites dengan stream aktif (stream selesai + log tersimpan)

## M4 — API admin

### 13. Statistik — `internal/admin/stats.go`
- [x] Ringkasan + deret harian: generate, user aktif, token in/out, estimasi biaya, error rate, rata-rata durasi, p50 TTFT
- [x] Biaya dihitung saat query dari `PRICE_*_PER_MTOK`
- [x] Hari dibagi menurut `TIMEZONE`; hari tanpa data tetap muncul (nilai 0)
- [x] AC: angka 30 hari cocok dengan hitungan manual (test dengan data di batas tengah malam WITA)
- [x] PRD performa: 60.000 log (30 hari) → stats 61 ms, logs 19 ms (target ≤ 1 detik)

### 14. Handler admin — `internal/api/admin_handler.go`, `internal/admin/logs.go`
- [x] `GET /api/admin/stats?from=&to=`
- [x] `GET /api/admin/logs` dengan filter (`user_id`, `status`, `type`, `from`, `to`) + paginasi 50
- [x] `GET /api/admin/audit?action=&page=`
- [x] `GET` / `POST /api/admin/users`
- [x] Query DB: list users, update role/is_active/password_hash dalam satu transaksi
- [x] `PATCH /api/admin/users/{id}`: role, is_active, password baru (cabut sesi saat nonaktif/reset)
- [x] Pengaman: tidak bisa menurunkan/menonaktifkan diri sendiri; minimal satu admin aktif
- [x] Semua aksi admin tercatat di `audit_logs`

## M5 — Siap deploy

Target deploy: **satu VPS, satu domain**. Docker Compose berisi Caddy (HTTPS otomatis) → app Go (API + build React) → Postgres.
Frontend dan API satu origin, jadi tidak perlu CORS dan cookie `SameSite=Lax` langsung bekerja.

### 15. Retensi log — `internal/jobs/retention.go`
- [ ] Job harian hapus `generation_logs` > `LOG_RETENTION_DAYS`
- [ ] Sekalian hapus `sessions` yang sudah kedaluwarsa

### 16. Static + CORS
- [x] Sajikan `STATIC_DIR` dengan fallback SPA ke `index.html` (`internal/api/static.go`; asset ber-hash di-cache immutable, file hilang → 404)
- [ ] CORS hanya untuk `ALLOWED_ORIGIN` dengan credentials (opsional, untuk jaga-jaga jika FE dipindah ke Cloudflare Pages)

### 17. Paket, deploy & dokumentasi
- [x] `Dockerfile` multi-stage: build React + Go → distroless (`STATIC_DIR=/app/web`)
- [ ] `docker-compose` produksi: Caddy + app + Postgres (Postgres tidak dibuka ke internet)
- [ ] `Caddyfile`: HTTPS otomatis untuk domain, streaming SSE tanpa buffer, `TRUST_PROXY=true`
- [ ] Backup Postgres harian (`pg_dump`) ke luar VPS
- [ ] `README.md`: setup lokal, env, seed, deploy ke VPS, backup & restore
- [ ] Uji manual 10 brief berita + 10 artikel
- [ ] AC: API key, password, token sesi, dan `facts` tidak muncul di log maupun response
