package generate

import (
	"strings"
	"testing"
)

func valid() Request {
	return Request{Type: "berita", Topic: "Pembukaan jalur bus listrik"}
}

func TestNormalizeDefaults(t *testing.T) {
	r := Request{Type: " Berita ", Topic: "  Topik panjang  ", Tone: "", Language: "EN"}
	r.Normalize()
	if r.Type != "berita" || r.Topic != "Topik panjang" || r.Tone != "formal" ||
		r.Length != "sedang" || r.Language != "en" {
		t.Errorf("normalize = %+v", r)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(*Request)
		field string // kosong = valid
	}{
		{"valid minimal", func(r *Request) {}, ""},
		{"valid lengkap", func(r *Request) {
			r.Type, r.Tone, r.Length, r.Language = "artikel", "edukatif", "panjang", "en"
			r.Keywords, r.Facts = strings.Repeat("k", 200), strings.Repeat("f", 4000)
		}, ""},
		{"type kosong", func(r *Request) { r.Type = "" }, "type"},
		{"type asing", func(r *Request) { r.Type = "opini" }, "type"},
		{"topik pendek", func(r *Request) { r.Topic = "bus" }, "topic"},
		{"topik 5 karakter multibyte", func(r *Request) { r.Topic = "ñññññ" }, ""},
		{"topik panjang", func(r *Request) { r.Topic = strings.Repeat("a", 501) }, "topic"},
		{"keywords panjang", func(r *Request) { r.Keywords = strings.Repeat("k", 201) }, "keywords"},
		{"tone asing", func(r *Request) { r.Tone = "lucu" }, "tone"},
		{"length asing", func(r *Request) { r.Length = "sangat panjang" }, "length"},
		{"bahasa asing", func(r *Request) { r.Language = "jv" }, "language"},
		{"facts panjang", func(r *Request) { r.Facts = strings.Repeat("f", 4001) }, "facts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := valid()
			tt.edit(&r)
			r.Normalize()
			errs := r.Validate()
			if tt.field == "" {
				if errs != nil {
					t.Errorf("harus valid, dapat %v", errs)
				}
				return
			}
			if _, ok := errs[tt.field]; !ok || len(errs) != 1 {
				t.Errorf("errs = %v, want hanya %q", errs, tt.field)
			}
		})
	}
}

func TestValidateReportsAllFields(t *testing.T) {
	r := Request{Type: "x", Topic: "", Tone: "x", Length: "x", Language: "x"}
	if errs := r.Validate(); len(errs) != 5 {
		t.Errorf("errs = %v", errs)
	}
}
