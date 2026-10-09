// Package db membungkus pool PostgreSQL dan query per tabel.
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ai-writer/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// QueryTimeout adalah batas waktu satu query DB.
const QueryTimeout = 5 * time.Second

// ErrNotFound dikembalikan jika baris yang dicari tidak ada.
var ErrNotFound = errors.New("db: data tidak ditemukan")

// withTimeout membatasi satu query dengan QueryTimeout.
func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, QueryTimeout)
}

// nullable mengubah string kosong menjadi NULL.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// DB adalah titik masuk semua query. Method per tabel ada di file terpisah.
type DB struct {
	Pool *pgxpool.Pool
}

// Open membuat pool dan memastikan database bisa dihubungi.
func Open(ctx context.Context, url string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("buat pool: %w", err)
	}

	d := &DB{Pool: pool}
	if err := d.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return d, nil
}

// Close menutup semua koneksi pool.
func (d *DB) Close() {
	d.Pool.Close()
}

// Ping mengecek koneksi dengan batas QueryTimeout.
func (d *DB) Ping(ctx context.Context) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	return d.Pool.Ping(ctx)
}

// Migrate menjalankan semua migrasi goose yang belum diterapkan.
func (d *DB) Migrate(ctx context.Context) ([]*goose.MigrationResult, error) {
	sqlDB := stdlib.OpenDBFromPool(d.Pool)
	defer sqlDB.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		return nil, fmt.Errorf("buat goose provider: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("jalankan migrasi: %w", err)
	}
	return results, nil
}
