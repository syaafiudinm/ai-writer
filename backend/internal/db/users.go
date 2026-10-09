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

// ErrLastAdmin dikembalikan jika perubahan akan menyisakan nol admin aktif.
var ErrLastAdmin = errors.New("db: harus ada minimal satu admin aktif")

func (d *DB) ListUsers(ctx context.Context) ([]User, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	rows, err := d.Pool.Query(ctx, `SELECT `+userColumns("")+` FROM users ORDER BY created_at, email`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (User, error) {
		u, err := scanUser(row)
		if err != nil {
			return User{}, err
		}
		return *u, nil
	})
}

// UserUpdate berisi field yang diubah; nil = tidak diubah.
type UserUpdate struct {
	Role         *string
	IsActive     *bool
	PasswordHash *string
}

// UpdateUser mengubah user dalam satu transaksi dan mengembalikan kondisi
// sebelum dan sesudahnya. Sesi user dicabut jika dinonaktifkan atau password
// diganti. Menolak perubahan yang menyisakan nol admin aktif.
func (d *DB) UpdateUser(ctx context.Context, id string, upd UserUpdate) (before, after *User, err error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	before, err = scanUser(tx.QueryRow(ctx,
		`SELECT `+userColumns("")+` FROM users WHERE id = $1 FOR UPDATE`, id))
	if err != nil {
		return nil, nil, err
	}

	stillAdmin := (upd.Role == nil || *upd.Role == RoleAdmin) && (upd.IsActive == nil || *upd.IsActive)
	if before.Role == RoleAdmin && before.IsActive && !stillAdmin {
		// Kunci baris admin lain agar dua admin yang saling menurunkan tidak lolos bersamaan.
		rows, err := tx.Query(ctx,
			`SELECT id FROM users WHERE role = 'admin' AND is_active AND id <> $1 FOR UPDATE`, id)
		if err != nil {
			return nil, nil, err
		}
		others, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return nil, nil, err
		}
		if len(others) == 0 {
			return nil, nil, ErrLastAdmin
		}
	}

	after, err = scanUser(tx.QueryRow(ctx,
		`UPDATE users SET
		   role = coalesce($2, role),
		   is_active = coalesce($3, is_active),
		   password_hash = coalesce($4, password_hash)
		 WHERE id = $1
		 RETURNING `+userColumns(""),
		id, upd.Role, upd.IsActive, upd.PasswordHash))
	if err != nil {
		return nil, nil, err
	}

	if (before.IsActive && !after.IsActive) || upd.PasswordHash != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id); err != nil {
			return nil, nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return before, after, nil
}
