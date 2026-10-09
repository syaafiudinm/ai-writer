// Package ratelimit menyediakan rate limiter sliding window in-memory.
// Hanya akurat per instance; untuk >1 instance pindahkan ke Redis/PostgreSQL.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

type Limiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu   sync.Mutex
	hits map[string][]time.Time // urut dari yang terlama, maksimal limit entri
}

// New membuat limiter yang mengizinkan limit hit per window untuk setiap key.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, now: time.Now, hits: map[string][]time.Time{}}
}

// Allow mencatat satu hit jika key masih di bawah batas. Jika tidak, hit tidak
// dicatat dan retryAfter berisi sisa waktu sampai slot berikutnya terbuka.
func (l *Limiter) Allow(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if hs := l.prune(key, now); len(hs) >= l.limit {
		return false, l.retryAfter(hs, now)
	}
	l.record(key, now)
	return true, 0
}

// Check seperti Allow tetapi tidak mencatat hit.
func (l *Limiter) Check(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if hs := l.prune(key, now); len(hs) >= l.limit {
		return false, l.retryAfter(hs, now)
	}
	return true, 0
}

// Add mencatat hit tanpa memeriksa batas, mis. untuk login yang gagal.
func (l *Limiter) Add(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.prune(key, now)
	l.record(key, now)
}

// Reset menghapus semua hit milik key, mis. setelah login berhasil.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}

// Cleanup membuang key yang semua hit-nya sudah di luar jendela.
func (l *Limiter) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for key := range l.hits {
		l.prune(key, now)
	}
}

// RunCleanup menjalankan Cleanup berkala sampai ctx selesai.
func (l *Limiter) RunCleanup(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.Cleanup()
		}
	}
}

// prune membuang hit di luar jendela. Dipanggil dengan mu terkunci.
func (l *Limiter) prune(key string, now time.Time) []time.Time {
	hs := l.hits[key]
	cutoff := now.Add(-l.window)
	i := 0
	for i < len(hs) && !hs[i].After(cutoff) {
		i++
	}
	hs = hs[i:]
	if len(hs) == 0 {
		delete(l.hits, key)
		return nil
	}
	l.hits[key] = hs
	return hs
}

// record menambah hit dan hanya menyimpan limit entri terakhir agar memori terbatas.
func (l *Limiter) record(key string, now time.Time) {
	hs := append(l.hits[key], now)
	if len(hs) > l.limit {
		hs = hs[len(hs)-l.limit:]
	}
	l.hits[key] = hs
}

// retryAfter: slot terbuka saat hit ke-(len-limit) keluar dari jendela.
func (l *Limiter) retryAfter(hs []time.Time, now time.Time) time.Duration {
	return hs[len(hs)-l.limit].Add(l.window).Sub(now)
}
