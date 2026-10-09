package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// ErrEmailTaken dikembalikan saat membuat user dengan email yang sudah ada.
var ErrEmailTaken = errors.New("db: email sudah terdaftar")

type User struct {
	ID           string
	Email        string
	Name         string
	PasswordHash string
	Role         string
	IsActive     bool
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

type NewUser struct {
	Email        string // harus sudah huruf kecil
	Name         string
	PasswordHash string
	Role         string
}

// userColumns menghasilkan daftar kolom users dengan prefix alias, mis. "u.".
func userColumns(alias string) string {
	a := alias
	return a + "id, " + a + "email, " + a + "name, " + a + "password_hash, " + a + "role, " +
		a + "is_active, " + a + "created_at, " + a + "last_login_at"
}

func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.IsActive, &u.CreatedAt, &u.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) UserByEmail(ctx context.Context, email string) (*User, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return scanUser(d.Pool.QueryRow(ctx,
		`SELECT `+userColumns("")+` FROM users WHERE email = $1`, email))
}

func (d *DB) UserByID(ctx context.Context, id string) (*User, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return scanUser(d.Pool.QueryRow(ctx,
		`SELECT `+userColumns("")+` FROM users WHERE id = $1`, id))
}

func (d *DB) CreateUser(ctx context.Context, n NewUser) (*User, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	u, err := scanUser(d.Pool.QueryRow(ctx,
		`INSERT INTO users (email, name, password_hash, role)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+userColumns(""),
		n.Email, n.Name, n.PasswordHash, n.Role))

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return nil, ErrEmailTaken
	}
	return u, err
}

func (d *DB) TouchLastLogin(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := d.Pool.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, id)
	return err
}
