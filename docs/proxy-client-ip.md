# IP Klien di Belakang Reverse Proxy

Catatan belajar dari masalah yang ditemukan saat deploy staging ke Railway (19 Okt 2026):
server mencatat **IP proxy Railway**, bukan IP pengguna. Dokumen ini menjelaskan kenapa itu
terjadi, cara membuktikannya, dan cara memperbaikinya dengan aman.

---

## 1. Ringkasan kasus

| | |
|---|---|
| **Gejala** | Semua request tercatat dengan IP `152.233.x.x` (Singapura), padahal pengguna di Makassar dengan IP Telkom `125.162.209.138`. |
| **Dampak** | Rate limit login "5 gagal / 15 menit per IP + email" berlaku untuk semua pengguna sekaligus, karena semua tampak berasal dari satu IP. IP di `audit_logs` dan `generation_logs` juga tidak berguna untuk investigasi. |
| **Penyebab** | Railway memakai **dua lapis proxy**. Server mengambil entri paling kanan `X-Forwarded-For`, yang ternyata IP edge Railway. |
| **Perbaikan** | Setting `TRUST_PROXY_HOPS`: IP klien diambil dari entri ke-N dari kanan. Railway = `2`, VPS dengan Caddy = `1`. |
| **Hal yang tidak bermasalah** | IP tidak pernah bisa dipalsukan pengguna, baik sebelum maupun sesudah perbaikan. |

---

## 2. Konsep dasar

### 2.1 `RemoteAddr`: siapa yang menelepon langsung

Setiap koneksi TCP punya alamat pengirim. Di Go, alamat ini ada di `r.RemoteAddr`
(misalnya `100.64.0.2:61510`). Nilainya **tidak bisa dipalsukan**, karena berasal dari
koneksi jaringan itu sendiri, bukan dari isi request.

Masalahnya, kalau ada **reverse proxy** di depan app, yang "menelepon" app adalah proxy,
bukan pengguna:

```
Pengguna ──TCP──▶ Proxy ──TCP──▶ App
                                  RemoteAddr = IP proxy  ❌ bukan IP pengguna
```

### 2.2 `X-Forwarded-For`: catatan perjalanan request

Supaya app tetap tahu IP asli pengguna, proxy menambahkan header `X-Forwarded-For` (XFF).
Aturannya: **setiap proxy menambahkan IP pihak yang menghubunginya di ujung kanan.**

```
X-Forwarded-For: <klien>, <proxy-1>, <proxy-2>
```

Contoh dengan dua proxy:

```
Pengguna (A) ──▶ Proxy 1 (B) ──▶ Proxy 2 (C) ──▶ App

Proxy 1 menerima dari A → XFF: A
Proxy 2 menerima dari B → XFF: A, B
App menerima dari C     → XFF: A, B     RemoteAddr: C
```

Catatan: IP proxy terakhir (C) **tidak** ada di XFF. IP itu muncul sebagai `RemoteAddr`.

### 2.3 Masalah utama: XFF bisa dipalsukan

XFF hanyalah header teks. Siapa pun bisa mengirim:

```sh
curl -H 'X-Forwarded-For: 6.6.6.6' https://app.example.com/api/auth/login
```

Proxy yang mengikuti aturan "tambahkan di kanan" akan meneruskannya menjadi:

```
X-Forwarded-For: 6.6.6.6, <IP asli pengguna>
                 ^^^^^^^   ^^^^^^^^^^^^^^^^^^^
                 palsu     ditulis proxy kita, bisa dipercaya
```

Jadi:

- ❌ **Entri paling kiri tidak boleh dipercaya.** Itu bisa tulisan pengguna sendiri.
  Kalau server memakainya, penyerang tinggal mengganti IP palsu di setiap percobaan,
  dan rate limit login per IP menjadi tidak berguna.
- ✅ **Yang bisa dipercaya hanya entri yang ditulis proxy milik kita**, dihitung dari kanan.

### 2.4 Konsep "hops" (jumlah proxy tepercaya)

Kalau kita tahu persis ada **N proxy tepercaya** di depan app, maka:

> **IP klien = entri ke-N dari kanan** pada `X-Forwarded-For`.

| Jumlah proxy | XFF yang diterima app | IP klien |
|---|---|---|
| 1 (Caddy) | `palsu, KLIEN` | entri ke-1 dari kanan |
| 2 (Railway) | `palsu, KLIEN, EDGE` | entri ke-2 dari kanan |

Apa pun yang pengguna tulis di XFF selalu berada **di kiri** entri-entri tepercaya,
sehingga tidak akan pernah terpilih.

### 2.5 Header lain yang perlu diketahui

| Header | Keterangan |
|---|---|
| `X-Real-IP` | Satu IP saja, diisi proxy (umum di Nginx). Hanya aman kalau proxy **selalu menimpanya**. |
| `Forwarded` | Versi standar resmi (RFC 7239): `Forwarded: for=125.162.209.138;proto=https`. Lebih rapi, tapi jarang dipakai. |
| `CF-Connecting-IP` | Khusus Cloudflare. Hanya aman kalau app menolak koneksi yang bukan dari IP Cloudflare. |

---

## 3. Arsitektur Railway (hasil pengamatan)

```
                 Internet                       │            Jaringan internal Railway
                                                │
 Pengguna ─────────▶ Edge Railway ──────────────┼──▶ Proxy internal ──────▶ App (Go)
 125.162.209.138     152.233.68.105             │    100.64.0.x
 (Telkom, Makassar)  (Datacamp, Singapura)      │    (rentang CGNAT)
                     · menimpa XFF & X-Real-IP  │
                       dari klien               │
                     · XFF = <klien>            │    · menambah IP edge
                                                │    · XFF = <klien>, <edge>
```

Yang sampai ke app:

```
RemoteAddr      = 100.64.0.2:61510                     ← proxy internal
X-Forwarded-For = 125.162.209.138, 152.233.68.105      ← klien, edge
X-Real-IP       = 125.162.209.138
```

Kode awal mengasumsikan hanya ada **satu** proxy, jadi mengambil entri paling kanan,
yaitu `152.233.68.105` (IP edge). Itu sebabnya semua pengguna tercatat dengan IP Singapura.

---

## 4. Bukti eksperimen

### 4.1 Cara menguji

1. Aktifkan `LOG_PROXY_HEADERS=true` supaya log mencatat `remote_addr`, `x_forwarded_for`,
   dan `x_real_ip` mentah.
2. Kirim beberapa request dari komputer yang IP publiknya diketahui, termasuk yang membawa
   header palsu. Setiap respons membawa `X-Request-ID`, jadi barisnya mudah dicari di log.

```sh
URL=https://ai-writer-production-f3cf.up.railway.app/api/health
rid() { curl -s -o /dev/null -D - "$@" $URL | grep -i '^x-request-id' | awk '{print $2}'; }

curl -s https://api.ipify.org                                        # IP publik sendiri
rid                                                                  # A: normal
rid -H 'X-Forwarded-For: 6.6.6.6'                                    # B: XFF palsu
rid -H 'X-Real-IP: 7.7.7.7'                                          # C: X-Real-IP palsu
rid -H 'X-Forwarded-For: 8.8.8.8, 9.9.9.9' -H 'X-Real-IP: 7.7.7.7'   # D: keduanya palsu
```

### 4.2 Hasil (IP publik pengujian: `125.162.209.138`)

**Sebelum perbaikan** (hops = 1):

| Uji | `ip` tercatat |
|---|---|
| A normal | `152.233.68.98` ❌ IP edge |
| B XFF palsu | `152.233.15.123` ❌ IP edge (palsu tidak terpakai) |

**Sesudah perbaikan** (`TRUST_PROXY_HOPS=2`):

| Uji | `x_forwarded_for` | `x_real_ip` | `ip` tercatat |
|---|---|---|---|
| A normal | `125.162.209.138, 152.233.68.105` | `125.162.209.138` | `125.162.209.138` ✅ |
| B XFF palsu `6.6.6.6` | `125.162.209.138, 152.233.68.105` | `125.162.209.138` | `125.162.209.138` ✅ |
| C X-Real-IP palsu `7.7.7.7` | `125.162.209.138, 152.233.68.105` | `125.162.209.138` | `125.162.209.138` ✅ |
| D keduanya palsu | `125.162.209.138, 152.233.68.105` | `125.162.209.138` | `125.162.209.138` ✅ |

### 4.3 Kesimpulan dari data

1. **Edge Railway membuang XFF dan X-Real-IP kiriman klien.** Uji B, C, dan D memberi header
   yang identik dengan uji A. Jadi di Railway, saat ini, header palsu bahkan tidak sampai ke app.
2. **Ada tepat dua entri di XFF:** klien dan edge, sehingga `TRUST_PROXY_HOPS=2`.
3. **IP edge berganti-ganti** (`.68.98`, `.15.123`, `.68.105`), dan `remote_addr` juga
   (`100.64.0.2` sampai `.5`). Jangan pernah meng-hardcode IP proxy.

> Perilaku poin 1 adalah milik Railway dan bisa berubah. Di forum Railway, jawaban staf
> mereka sendiri saling bertentangan soal ini. Pendekatan hops tetap aman walaupun suatu
> saat edge berhenti membuang header palsu, karena entri palsu selalu berada di kiri.

---

## 5. Implementasi di kode

File: [`backend/internal/api/middleware.go`](../backend/internal/api/middleware.go), fungsi `remoteIP`.

```go
func remoteIP(r *http.Request, trustProxy bool, hops int) string {
	if trustProxy && hops > 0 {
		// Gabungkan semua header XFF (bisa lebih dari satu baris) menjadi satu daftar.
		var parts []string
		for _, v := range r.Header.Values("X-Forwarded-For") {
			for _, p := range strings.Split(v, ",") {
				if p = strings.TrimSpace(p); p != "" {
					parts = append(parts, p)
				}
			}
		}
		// Ambil entri ke-hops dari kanan.
		if len(parts) >= hops {
			if ip := parts[len(parts)-hops]; net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	// Tidak di belakang proxy, XFF terlalu pendek, atau isinya bukan IP → pakai koneksi langsung.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
```

Alasan setiap keputusan:

| Keputusan | Alasan |
|---|---|
| Hanya membaca XFF kalau `TRUST_PROXY=true` | Kalau app diakses langsung tanpa proxy, XFF sepenuhnya buatan klien. |
| Menggabungkan semua baris header XFF | HTTP boleh mengirim header yang sama berkali-kali; proxy bisa menambah baris baru alih-alih menyambung. |
| Kalau entri **lebih sedikit** dari `hops`, pakai `RemoteAddr` | Itu tanda request tidak lewat jalur proxy yang diharapkan. Mengambil entri paling kiri justru membuka celah pemalsuan. |
| `net.ParseIP` sebelum dipakai | Menolak isi sampah seperti `unknown` atau `bukan-ip`. |
| Tidak memakai `X-Real-IP` | Satu sumber kebenaran lebih mudah diaudit. Di forum Railway ada laporan `X-Real-IP` sesekali berisi IP infrastruktur. |

Test-nya ada di [`backend/internal/api/router_test.go`](../backend/internal/api/router_test.go)
(`TestRemoteIP`), termasuk kasus Railway dan XFF palsu.

IP hasil fungsi ini disimpan di context oleh middleware `clientIP`, lalu dipakai oleh:

- request log (`"ip"`),
- rate limit login (kunci `IP|email`),
- `sessions.ip`, `audit_logs.ip`, `generation_logs.ip`.

---

## 6. Konfigurasi per lingkungan

| Lingkungan | Jalur request | `TRUST_PROXY` | `TRUST_PROXY_HOPS` |
|---|---|---|---|
| Lokal (`go run`, tanpa proxy) | Klien → App | `false` | — |
| **Railway** | Klien → Edge → Proxy internal → App | `true` | **`2`** |
| **VPS + Caddy** | Klien → Caddy → App | `true` | **`1`** |
| VPS + Nginx | Klien → Nginx → App | `true` | `1` |
| Cloudflare → Caddy → App | Klien → Cloudflare → Caddy → App | `true` | `2`, **dan** VPS hanya boleh menerima koneksi dari IP Cloudflare |

Catatan per proxy:

- **Caddy**: secara default Caddy mengabaikan XFF kiriman klien yang tidak tepercaya dan
  menulis ulang XFF berisi IP klien saja. Jadi app menerima `XFF: <klien>`, dan hops = 1.
- **Nginx**: gunakan `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`
  (menambah IP klien di kanan). Hops = 1. Jangan lupa `proxy_buffering off` untuk SSE.
- **Cloudflare di depan**: jumlah proxy bertambah satu. Kalau VPS juga bisa diakses langsung
  tanpa lewat Cloudflare, penyerang bisa melewati Cloudflare dan memalsukan satu entri.
  Karena itu firewall VPS wajib membatasi akses hanya dari rentang IP Cloudflare.

### Kalau salah setting

| Kesalahan | Akibat |
|---|---|
| `TRUST_PROXY_HOPS` **terlalu kecil** (Railway diisi 1) | Semua pengguna tercatat dengan IP proxy. Rate limit per IP berlaku bersama-sama. **Aman, tapi tidak akurat.** Ini kasus kita sebelum diperbaiki. |
| `TRUST_PROXY_HOPS` **terlalu besar** (VPS diisi 2) | App mengambil entri yang ditulis klien. **IP bisa dipalsukan**, dan rate limit login bisa diakali. **Berbahaya.** |
| `TRUST_PROXY=true` tanpa proxy di depan | Sama berbahayanya: seluruh XFF ditulis klien. |

> Prinsipnya: kalau ragu, **lebih baik terlalu kecil daripada terlalu besar**.
> Terlalu kecil hanya membuat data kurang akurat; terlalu besar membuka celah keamanan.

---

## 7. Prosedur verifikasi (ulangi setiap ganti platform atau proxy)

1. Deploy dengan `LOG_PROXY_HEADERS=true`.
2. Jalankan uji A–D di bagian 4.1 dari komputer yang IP publiknya diketahui.
3. Cari keempat `request_id` di log, lalu periksa:
   - `x_forwarded_for`: berapa entri, dan di posisi mana IP-mu berada?
   - `ip`: harus IP publikmu di **keempat** uji.
4. Set `TRUST_PROXY_HOPS` sesuai posisi IP-mu dihitung dari kanan.
5. Kembalikan `LOG_PROXY_HEADERS=false`.

Kapan perlu diulang:
- Pindah dari Railway ke VPS (hops 2 → 1).
- Menambahkan Cloudflare atau CDN di depan.
- Railway mengumumkan perubahan jaringan edge, atau `"ip"` di log tiba-tiba berisi
  `152.233.x.x` / `100.64.x.x` lagi.

---

## 8. Pertanyaan untuk pendalaman

1. Kenapa IP proxy terakhir (yang langsung menghubungi app) tidak pernah muncul di XFF?
   Di mana kita bisa melihatnya?
2. Misalkan Railway menambah satu lapis proxy lagi tanpa pemberitahuan, padahal kita masih
   `TRUST_PROXY_HOPS=2`. Apa yang akan tercatat sebagai `ip`, dan apakah itu berbahaya?
3. Kenapa rentang `100.64.0.0/10` disebut "CGNAT", dan kenapa Railway tidak menyarankan
   kita mengandalkan rentang itu?
4. Apa bedanya pendekatan "hitung hops dari kanan" dengan "daftar IP proxy tepercaya"
   (yang dipakai `trusted_proxies` di Caddy atau `set_real_ip_from` di Nginx)?
   Kapan masing-masing lebih cocok?
5. Rate limit kita memakai kunci `IP|email`. Kalau ada 50 penulis di satu kantor dengan
   IP publik yang sama (NAT kantor), apa dampaknya? Apakah kunci itu masih masuk akal?

---

## 9. Referensi

- MDN — [X-Forwarded-For](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/X-Forwarded-For)
  (bagian *Security and privacy concerns* dan *Selecting an IP address*)
- RFC 7239 — [Forwarded HTTP Extension](https://www.rfc-editor.org/rfc/rfc7239)
- Caddy — [reverse_proxy: defaults & trusted_proxies](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy#defaults)
- Nginx — [ngx_http_realip_module](https://nginx.org/en/docs/http/ngx_http_realip_module.html)
- Diskusi Railway (jawaban staf saling bertentangan, sebabnya kita menguji sendiri):
  - [Which header should I rely on for real client IP?](https://station.railway.com/questions/which-header-should-i-rely-on-for-real-c-d78a6f96)
  - [Does the edge strip client X-Forwarded-For? Two staff answers contradict](https://station.railway.com/questions/does-the-edge-strip-client-x-forwarded-f-613e5cb6)
  - [Edge/CDN layer leaking its own IP as client IP in X-Forwarded-For](https://station.railway.com/questions/edge-cdn-layer-leaking-its-own-ip-as-cli-1ce29b05)
  - [Which client-address headers does the Railway edge set?](https://station.railway.com/questions/which-client-address-headers-does-the-ra-fb9e36d7)
