package admin

import (
	"net/url"
	"testing"
	"time"
)

func testService() *Service {
	loc, _ := time.LoadLocation("Asia/Makassar")
	s := NewService(nil, 1, 2, loc)
	// 2026-10-10 01:00 WITA = 2026-10-09 17:00 UTC: "hari ini" harus 10 Okt, bukan 9 Okt.
	s.now = func() time.Time { return time.Date(2026, 10, 9, 17, 0, 0, 0, time.UTC) }
	return s
}

func TestStatsRange(t *testing.T) {
	s := testService()
	tests := []struct {
		name, from, to   string
		wantFrom, wantTo string
		errField         string
	}{
		{"default 30 hari, hari ini menurut zona lokal", "", "", "2026-09-11", "2026-10-10", ""},
		{"hanya to", "", "2026-10-05", "2026-09-06", "2026-10-05", ""},
		{"lengkap", "2026-10-01", "2026-10-03", "2026-10-01", "2026-10-03", ""},
		{"satu hari", "2026-10-01", "2026-10-01", "2026-10-01", "2026-10-01", ""},
		{"366 hari", "2025-10-10", "2026-10-10", "2025-10-10", "2026-10-10", ""},
		{"format salah", "01-10-2026", "", "", "", "from"},
		{"terbalik", "2026-10-05", "2026-10-01", "", "", "from"},
		{"terlalu panjang", "2025-10-09", "2026-10-10", "", "", "to"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, errs := s.StatsRange(tt.from, tt.to)
			if tt.errField != "" {
				if _, ok := errs[tt.errField]; !ok {
					t.Fatalf("errs = %v, want field %s", errs, tt.errField)
				}
				return
			}
			if errs != nil {
				t.Fatalf("errs = %v", errs)
			}
			if got := r.From.Format(dateLayout); got != tt.wantFrom {
				t.Errorf("from = %s, want %s", got, tt.wantFrom)
			}
			if got := r.To.Format(dateLayout); got != tt.wantTo {
				t.Errorf("to = %s, want %s", got, tt.wantTo)
			}
			if r.end().Sub(r.start()) <= 0 || r.From.Location().String() != "Asia/Makassar" {
				t.Error("rentang harus memakai zona lokal dan end eksklusif")
			}
		})
	}
}

func TestParseLogFilter(t *testing.T) {
	s := testService()
	f, errs := s.ParseLogFilter(url.Values{
		"user_id": {"6efb4624-28e9-4c11-9e6b-8d71829c2582"}, "status": {"error"}, "type": {"berita"},
		"from": {"2026-10-01"}, "to": {"2026-10-01"}, "page": {"3"},
	})
	if errs != nil {
		t.Fatal(errs)
	}
	if f.Page != 3 || f.Status != "error" || f.Until.Sub(*f.Since) != 24*time.Hour {
		t.Errorf("filter = %+v", f)
	}

	_, errs = s.ParseLogFilter(url.Values{
		"user_id": {"bukan-uuid"}, "status": {"gagal"}, "type": {"opini"}, "from": {"kemarin"}, "page": {"0"},
	})
	for _, field := range []string{"user_id", "status", "type", "from", "page"} {
		if errs[field] == "" {
			t.Errorf("errs tidak memuat %s: %v", field, errs)
		}
	}
}

func TestCostAndRate(t *testing.T) {
	s := testService() // $1 input, $2 output per juta token
	if c := s.cost(1_500_000, 250_000); c != 2.0 {
		t.Errorf("cost = %v, want 2.0", c)
	}
	if r := rate(1, 3); r != 0.3333 {
		t.Errorf("rate = %v", r)
	}
	if r := rate(0, 0); r != 0 {
		t.Errorf("rate kosong = %v", r)
	}
}
