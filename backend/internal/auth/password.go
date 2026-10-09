package auth

import (
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	BcryptCost     = 12
	MinPasswordLen = 8
	// bcrypt hanya memakai 72 byte pertama; lebih dari itu ditolak agar tidak menyesatkan.
	maxPasswordBytes = 72
)

var (
	ErrInvalidEmail     = errors.New("format email tidak valid")
	ErrPasswordTooShort = errors.New("password minimal 8 karakter")
	ErrPasswordTooLong  = errors.New("password maksimal 72 byte")
)

// NormalizeEmail menyeragamkan email sebelum disimpan atau dicari.
func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// ValidateEmail menerima email yang sudah dinormalisasi.
func ValidateEmail(email string) error {
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return ErrInvalidEmail
	}
	return nil
}

func ValidatePassword(pw string) error {
	if utf8.RuneCountInString(pw) < MinPasswordLen {
		return ErrPasswordTooShort
	}
	if len(pw) > maxPasswordBytes {
		return ErrPasswordTooLong
	}
	return nil
}

func HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), BcryptCost)
	return string(h), err
}
