package auth

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewToken(t *testing.T) {
	tok1, hash1 := newToken()
	tok2, _ := newToken()
	if tok1 == tok2 {
		t.Fatal("token harus acak")
	}
	if len(tok1) != 43 { // 32 byte base64url tanpa padding
		t.Errorf("panjang token = %d, want 43", len(tok1))
	}
	if !bytes.Equal(hash1, hashToken(tok1)) || len(hash1) != 32 {
		t.Error("hash tidak konsisten dengan token")
	}
	if bytes.Contains(hash1, []byte(tok1)) {
		t.Error("hash tidak boleh memuat token asli")
	}
}

func TestValidateEmail(t *testing.T) {
	for _, tt := range []struct {
		in string
		ok bool
	}{
		{"admin@example.com", true},
		{NormalizeEmail("  Admin@Example.COM "), true},
		{"bukan-email", false},
		{"Nama <a@b.com>", false},
		{"", false},
	} {
		if err := ValidateEmail(tt.in); (err == nil) != tt.ok {
			t.Errorf("ValidateEmail(%q) = %v, want ok=%v", tt.in, err, tt.ok)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want error
	}{
		{"1234567", ErrPasswordTooShort},
		{"12345678", nil},
		{"rahasiañ", nil}, // 8 karakter walau 9 byte
		{strings.Repeat("a", 73), ErrPasswordTooLong},
	} {
		if err := ValidatePassword(tt.in); err != tt.want {
			t.Errorf("ValidatePassword(%q) = %v, want %v", tt.in, err, tt.want)
		}
	}
}
