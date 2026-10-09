// Package generate berisi brief dari user, validasinya, dan prompt builder.
package generate

import (
	"strings"
	"unicode/utf8"
)

const (
	TypeBerita  = "berita"
	TypeArtikel = "artikel"
)

var (
	validTypes     = []string{TypeBerita, TypeArtikel}
	validTones     = []string{"formal", "santai", "persuasif", "edukatif"}
	validLengths   = []string{"pendek", "sedang", "panjang"}
	validLanguages = []string{"id", "en"}
)

// Request adalah brief generate dari user.
type Request struct {
	Type     string `json:"type"`
	Topic    string `json:"topic"`
	Keywords string `json:"keywords"`
	Tone     string `json:"tone"`
	Length   string `json:"length"`
	Language string `json:"language"`
	Facts    string `json:"facts"`
}

// Normalize merapikan spasi, menyeragamkan enum ke huruf kecil, dan mengisi default.
func (r *Request) Normalize() {
	r.Type = strings.ToLower(strings.TrimSpace(r.Type))
	r.Topic = strings.TrimSpace(r.Topic)
	r.Keywords = strings.TrimSpace(r.Keywords)
	r.Tone = strings.ToLower(strings.TrimSpace(r.Tone))
	r.Length = strings.ToLower(strings.TrimSpace(r.Length))
	r.Language = strings.ToLower(strings.TrimSpace(r.Language))
	r.Facts = strings.TrimSpace(r.Facts)

	if r.Tone == "" {
		r.Tone = "formal"
	}
	if r.Length == "" {
		r.Length = "sedang"
	}
	if r.Language == "" {
		r.Language = "id"
	}
}

// Validate mengembalikan pesan error per field, atau nil jika valid.
// Panggil Normalize terlebih dulu.
func (r Request) Validate() map[string]string {
	errs := map[string]string{}
	if !oneOf(r.Type, validTypes) {
		errs["type"] = "Jenis wajib dipilih: berita atau artikel."
	}
	if n := utf8.RuneCountInString(r.Topic); n < 5 || n > 500 {
		errs["topic"] = "Topik wajib diisi (5–500 karakter)."
	}
	if utf8.RuneCountInString(r.Keywords) > 200 {
		errs["keywords"] = "Kata kunci maksimal 200 karakter."
	}
	if !oneOf(r.Tone, validTones) {
		errs["tone"] = "Gaya bahasa harus salah satu dari: formal, santai, persuasif, edukatif."
	}
	if !oneOf(r.Length, validLengths) {
		errs["length"] = "Panjang harus salah satu dari: pendek, sedang, panjang."
	}
	if !oneOf(r.Language, validLanguages) {
		errs["language"] = "Bahasa harus id atau en."
	}
	if utf8.RuneCountInString(r.Facts) > 4000 {
		errs["facts"] = "Fakta maksimal 4.000 karakter."
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func oneOf(v string, allowed []string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
