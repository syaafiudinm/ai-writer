package config

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"GEMINI_API_KEY": "AIza-test",
		"DATABASE_URL":   "postgres://localhost/db",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	checks := []struct {
		name      string
		got, want any
	}{
		{"GeminiModel", cfg.GeminiModel, "gemini-3.5-flash"},
		{"GeminiThinkingLevel", cfg.GeminiThinkingLevel, "minimal"},
		{"Port", cfg.Port, "8080"},
		{"SessionTTL", cfg.SessionTTL, 168 * time.Hour},
		{"CookieSecure", cfg.CookieSecure, true},
		{"RateLimitPerMin", cfg.RateLimitPerMin, 10},
		{"GenerateTimeout", cfg.GenerateTimeout, 180 * time.Second},
		{"LogRetentionDays", cfg.LogRetentionDays, 180},
		{"StaticDir", cfg.StaticDir, ""},
		{"AllowedOrigin", cfg.AllowedOrigin, ""},
		{"TrustProxy", cfg.TrustProxy, false},
		{"PriceInputPerMTok", cfg.PriceInputPerMTok, 0.0},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"GEMINI_API_KEY":        "AIza-test",
		"DATABASE_URL":          "postgres://localhost/db",
		"GEMINI_MODEL":          "gemini-3.8-flash",
		"GEMINI_THINKING_LEVEL": "low",
		"PRICE_INPUT_PER_MTOK":  "3",
		"PRICE_OUTPUT_PER_MTOK": "15.5",
		"PORT":                  "9000",
		"SESSION_TTL":           "24h",
		"COOKIE_SECURE":         "false",
		"RATE_LIMIT_PER_MIN":    "20",
		"GENERATE_TIMEOUT":      "90s",
		"TRUST_PROXY":           "true",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.GeminiModel != "gemini-3.8-flash" || cfg.GeminiThinkingLevel != "low" || cfg.Port != "9000" ||
		cfg.SessionTTL != 24*time.Hour || cfg.CookieSecure || cfg.RateLimitPerMin != 20 ||
		cfg.GenerateTimeout != 90*time.Second || !cfg.TrustProxy ||
		cfg.PriceInputPerMTok != 3 || cfg.PriceOutputPerMTok != 15.5 {
		t.Errorf("override tidak terbaca: %+v", cfg)
	}
}

func TestLoadCollectsAllErrors(t *testing.T) {
	_, err := load(env(map[string]string{
		"SESSION_TTL":           "seminggu",
		"RATE_LIMIT_PER_MIN":    "0",
		"COOKIE_SECURE":         "ya",
		"GEMINI_THINKING_LEVEL": "tinggi",
	}))
	if err == nil {
		t.Fatal("harusnya error")
	}
	for _, want := range []string{
		"GEMINI_API_KEY wajib diisi",
		"GEMINI_THINKING_LEVEL harus salah satu dari",
		"DATABASE_URL wajib diisi",
		"SESSION_TTL harus berupa durasi",
		"RATE_LIMIT_PER_MIN harus lebih dari 0",
		"COOKIE_SECURE harus true atau false",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error tidak memuat %q:\n%v", want, err)
		}
	}
}

func TestStringRedactsSecrets(t *testing.T) {
	cfg := Config{GeminiAPIKey: "AIza-rahasia", DatabaseURL: "postgres://app:pass@host/db"}
	s := cfg.String()
	if strings.Contains(s, "AIza-rahasia") || strings.Contains(s, "pass@host") {
		t.Errorf("rahasia bocor: %s", s)
	}
}
