// Package config membaca dan memvalidasi konfigurasi aplikasi dari environment variable.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // database zona waktu ikut di binary (image distroless tidak punya)
)

// Config berisi seluruh konfigurasi server. Nilainya hanya berasal dari env.
type Config struct {
	GeminiAPIKey        string
	GeminiModel         string
	GeminiThinkingLevel string // minimal/low/medium/high; dukungan berbeda per model
	DatabaseURL         string

	PriceInputPerMTok  float64
	PriceOutputPerMTok float64

	Port             string
	SessionTTL       time.Duration
	CookieSecure     bool
	RateLimitPerMin  int
	GenerateTimeout  time.Duration
	LogRetentionDays int
	StaticDir        string
	AllowedOrigin    string
	TrustProxy       bool
	Timezone         string         // zona waktu pembagian hari di dashboard admin
	Location         *time.Location // hasil load Timezone
}

// Load membaca konfigurasi dari environment proses.
func Load() (*Config, error) {
	return load(os.Getenv)
}

// load menerima getenv agar bisa dites tanpa mengubah environment proses.
func load(getenv func(string) string) (*Config, error) {
	p := parser{getenv: getenv}

	cfg := &Config{
		GeminiAPIKey:        p.required("GEMINI_API_KEY"),
		GeminiModel:         p.str("GEMINI_MODEL", "gemini-3.5-flash"),
		GeminiThinkingLevel: p.str("GEMINI_THINKING_LEVEL", "minimal"),
		DatabaseURL:         p.required("DATABASE_URL"),
		PriceInputPerMTok:   p.float("PRICE_INPUT_PER_MTOK", 0),
		PriceOutputPerMTok:  p.float("PRICE_OUTPUT_PER_MTOK", 0),
		Port:                p.str("PORT", "8080"),
		SessionTTL:          p.duration("SESSION_TTL", 168*time.Hour),
		CookieSecure:        p.bool("COOKIE_SECURE", true),
		RateLimitPerMin:     p.int("RATE_LIMIT_PER_MIN", 10),
		GenerateTimeout:     p.duration("GENERATE_TIMEOUT", 180*time.Second),
		LogRetentionDays:    p.int("LOG_RETENTION_DAYS", 180),
		StaticDir:           p.str("STATIC_DIR", ""),
		AllowedOrigin:       p.str("ALLOWED_ORIGIN", ""),
		TrustProxy:          p.bool("TRUST_PROXY", false),
		Timezone:            p.str("TIMEZONE", "Asia/Makassar"),
	}

	loc, err := time.LoadLocation(cfg.Timezone)
	p.check(err == nil, "TIMEZONE tidak dikenal, contoh: Asia/Jakarta, Asia/Makassar, Asia/Jayapura")
	cfg.Location = loc

	p.check(validThinkingLevels[cfg.GeminiThinkingLevel],
		"GEMINI_THINKING_LEVEL harus salah satu dari: minimal, low, medium, high")
	p.check(cfg.PriceInputPerMTok >= 0, "PRICE_INPUT_PER_MTOK tidak boleh negatif")
	p.check(cfg.PriceOutputPerMTok >= 0, "PRICE_OUTPUT_PER_MTOK tidak boleh negatif")
	p.check(cfg.SessionTTL > 0, "SESSION_TTL harus lebih dari 0")
	p.check(cfg.RateLimitPerMin > 0, "RATE_LIMIT_PER_MIN harus lebih dari 0")
	p.check(cfg.GenerateTimeout > 0, "GENERATE_TIMEOUT harus lebih dari 0")
	p.check(cfg.LogRetentionDays > 0, "LOG_RETENTION_DAYS harus lebih dari 0")

	if err := errors.Join(p.errs...); err != nil {
		return nil, fmt.Errorf("konfigurasi tidak valid:\n%w", err)
	}
	return cfg, nil
}

var validThinkingLevels = map[string]bool{"minimal": true, "low": true, "medium": true, "high": true}

// String menyensor rahasia agar Config aman jika tidak sengaja di-log.
func (c Config) String() string {
	c.GeminiAPIKey = "[REDACTED]"
	c.DatabaseURL = "[REDACTED]"
	type plain Config // tipe tanpa method String, mencegah rekursi
	return fmt.Sprintf("%+v", plain(c))
}

// parser mengumpulkan semua error agar user melihat semua masalah env sekaligus.
type parser struct {
	getenv func(string) string
	errs   []error
}

func (p *parser) lookup(key string) (string, bool) {
	v := strings.TrimSpace(p.getenv(key))
	return v, v != ""
}

func (p *parser) check(ok bool, msg string) {
	if !ok {
		p.errs = append(p.errs, errors.New(msg))
	}
}

func (p *parser) required(key string) string {
	v, ok := p.lookup(key)
	p.check(ok, key+" wajib diisi")
	return v
}

func (p *parser) str(key, def string) string {
	if v, ok := p.lookup(key); ok {
		return v
	}
	return def
}

func (p *parser) int(key string, def int) int {
	v, ok := p.lookup(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		p.errs = append(p.errs, fmt.Errorf("%s harus berupa bilangan bulat, dapat %q", key, v))
		return def
	}
	return n
}

func (p *parser) float(key string, def float64) float64 {
	v, ok := p.lookup(key)
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		p.errs = append(p.errs, fmt.Errorf("%s harus berupa angka, dapat %q", key, v))
		return def
	}
	return f
}

func (p *parser) bool(key string, def bool) bool {
	v, ok := p.lookup(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		p.errs = append(p.errs, fmt.Errorf("%s harus true atau false, dapat %q", key, v))
		return def
	}
	return b
}

func (p *parser) duration(key string, def time.Duration) time.Duration {
	v, ok := p.lookup(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		p.errs = append(p.errs, fmt.Errorf("%s harus berupa durasi (mis. 180s, 168h), dapat %q", key, v))
		return def
	}
	return d
}
