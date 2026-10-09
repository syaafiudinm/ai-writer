package ratelimit

import (
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newTestLimiter(limit int, window time.Duration) (*Limiter, *clock) {
	c := &clock{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	l := New(limit, window)
	l.now = c.now
	return l, c
}

func TestAllowSlidingWindow(t *testing.T) {
	l, c := newTestLimiter(3, time.Minute)

	for i := range 3 {
		if ok, _ := l.Allow("u1"); !ok {
			t.Fatalf("hit ke-%d harus lolos", i+1)
		}
		c.advance(10 * time.Second)
	}
	// t=30s, hit di 0s/10s/20s
	ok, retry := l.Allow("u1")
	if ok || retry != 30*time.Second {
		t.Fatalf("hit ke-4: ok=%v retry=%v, want false 30s", ok, retry)
	}
	if ok, _ := l.Allow("u2"); !ok {
		t.Fatal("key lain tidak boleh terpengaruh")
	}

	c.advance(30 * time.Second) // t=60s, hit 0s keluar jendela
	if ok, _ := l.Allow("u1"); !ok {
		t.Fatal("setelah slot terlama kedaluwarsa harus lolos lagi")
	}
	if ok, retry := l.Allow("u1"); ok || retry != 10*time.Second {
		t.Fatalf("ok=%v retry=%v, want false 10s", ok, retry)
	}
}

func TestCheckAddResetForLogin(t *testing.T) {
	l, c := newTestLimiter(5, 15*time.Minute)
	key := "1.2.3.4|a@b.com"

	for i := range 5 {
		if ok, _ := l.Check(key); !ok {
			t.Fatalf("percobaan ke-%d harus diizinkan", i+1)
		}
		l.Add(key) // login gagal
		c.advance(time.Minute)
	}
	ok, retry := l.Check(key)
	if ok || retry != 10*time.Minute {
		t.Fatalf("percobaan ke-6: ok=%v retry=%v, want false 10m", ok, retry)
	}

	// Check tidak mencatat: memanggil berkali-kali tidak memperpanjang blokir.
	l.Check(key)
	if _, retry2 := l.Check(key); retry2 != retry {
		t.Errorf("Check mengubah state: %v → %v", retry, retry2)
	}

	l.Reset(key)
	if ok, _ := l.Check(key); !ok {
		t.Error("setelah Reset harus diizinkan")
	}
}

func TestCleanupDropsExpiredKeys(t *testing.T) {
	l, c := newTestLimiter(2, time.Minute)
	l.Allow("a")
	l.Add("b")
	c.advance(2 * time.Minute)
	l.Cleanup()
	if len(l.hits) != 0 {
		t.Errorf("hits tersisa: %v", l.hits)
	}
}

func TestMemoryBounded(t *testing.T) {
	l, _ := newTestLimiter(5, time.Hour)
	for range 100 {
		l.Add("k")
	}
	if n := len(l.hits["k"]); n != 5 {
		t.Errorf("entri tersimpan = %d, want 5", n)
	}
}
