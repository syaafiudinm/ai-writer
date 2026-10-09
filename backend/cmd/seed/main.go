// Perintah seed membuat akun admin pertama.
//
//	go run ./cmd/seed -email admin@contoh.com -name "Admin"
//
// Password diminta lewat terminal (tidak tampil), atau dibaca dari stdin jika
// non-interaktif: echo "$ADMIN_PASSWORD" | go run ./cmd/seed -email ...
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"ai-writer/internal/auth"
	"ai-writer/internal/db"

	"golang.org/x/term"
)

func main() {
	email := flag.String("email", "", "email admin (wajib)")
	name := flag.String("name", "Admin", "nama admin")
	flag.Parse()

	if err := run(*email, *name); err != nil {
		fmt.Fprintln(os.Stderr, "seed gagal:", err)
		os.Exit(1)
	}
}

func run(email, name string) error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL wajib diisi")
	}
	email = auth.NormalizeEmail(email)
	if err := auth.ValidateEmail(email); err != nil {
		return fmt.Errorf("-email: %w", err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("-name tidak boleh kosong")
	}

	password, err := readPassword()
	if err != nil {
		return err
	}
	if err := auth.ValidatePassword(password); err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	database, err := db.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer database.Close()
	if _, err := database.Migrate(ctx); err != nil {
		return err
	}

	u, err := database.CreateUser(ctx, db.NewUser{
		Email: email, Name: name, PasswordHash: hash, Role: db.RoleAdmin,
	})
	if errors.Is(err, db.ErrEmailTaken) {
		return fmt.Errorf("email %s sudah terdaftar", email)
	}
	if err != nil {
		return err
	}
	if err := database.InsertAudit(ctx, db.AuditEntry{
		Action: db.AuditUserCreated, TargetID: u.ID,
		Metadata: map[string]any{"role": u.Role, "via": "seed"},
	}); err != nil {
		return err
	}

	fmt.Printf("admin dibuat: %s (%s)\n", u.Email, u.ID)
	return nil
}

func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	fmt.Fprint(os.Stderr, "Password admin: ")
	p1, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Ulangi password: ")
	p2, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(p1) != string(p2) {
		return "", errors.New("password tidak sama")
	}
	return string(p1), nil
}
