// Package admin berisi query agregat dan daftar untuk dashboard admin.
package admin

import (
	"context"
	"math"
	"time"

	"ai-writer/internal/db"
)

const (
	PageSize       = 50
	defaultDays    = 30
	maxRangeDays   = 366
	maxPage        = 10000
	dateLayout     = "2006-01-02"
	msgDateFormat  = "Format tanggal harus YYYY-MM-DD."
	msgRangeOrder  = "Tanggal awal harus sebelum atau sama dengan tanggal akhir."
	msgRangeLength = "Rentang maksimal 366 hari."
)

type Service struct {
	db       *db.DB
	priceIn  float64 // USD per juta token input
	priceOut float64 // USD per juta token output
	loc      *time.Location
	now      func() time.Time
}

func NewService(d *db.DB, priceIn, priceOut float64, loc *time.Location) *Service {
	return &Service{db: d, priceIn: priceIn, priceOut: priceOut, loc: loc, now: time.Now}
}

// DateRange adalah rentang hari lokal inklusif [From, To].
type DateRange struct {
	From, To time.Time // tengah malam waktu lokal
}

func (r DateRange) start() time.Time { return r.From }
func (r DateRange) end() time.Time   { return r.To.AddDate(0, 0, 1) } // eksklusif

func (s *Service) day(t time.Time) time.Time {
	t = t.In(s.loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, s.loc)
}

func (s *Service) parseDate(v string) (time.Time, bool) {
	t, err := time.ParseInLocation(dateLayout, v, s.loc)
	return t, err == nil
}

// StatsRange membaca ?from=&to=. Default: 30 hari terakhir sampai hari ini.
func (s *Service) StatsRange(fromStr, toStr string) (DateRange, map[string]string) {
	errs := map[string]string{}
	to := s.day(s.now())
	if toStr != "" {
		t, ok := s.parseDate(toStr)
		if !ok {
			errs["to"] = msgDateFormat
		}
		to = t
	}
	from := to.AddDate(0, 0, -(defaultDays - 1))
	if fromStr != "" {
		t, ok := s.parseDate(fromStr)
		if !ok {
			errs["from"] = msgDateFormat
		}
		from = t
	}
	if len(errs) == 0 {
		if from.After(to) {
			errs["from"] = msgRangeOrder
		} else if to.Sub(from) >= maxRangeDays*24*time.Hour {
			errs["to"] = msgRangeLength
		}
	}
	if len(errs) > 0 {
		return DateRange{}, errs
	}
	return DateRange{From: from, To: to}, nil
}

type Pricing struct {
	InputPerMTok  float64 `json:"input_per_mtok"`
	OutputPerMTok float64 `json:"output_per_mtok"`
	Configured    bool    `json:"configured"`
}

type RangeInfo struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Timezone string `json:"timezone"`
}

type StatusCounts struct {
	Success   int `json:"success"`
	Error     int `json:"error"`
	Cancelled int `json:"cancelled"`
	Timeout   int `json:"timeout"`
}

type Summary struct {
	Generations      int          `json:"generations"`
	ActiveUsers      int          `json:"active_users"`
	InputTokens      int64        `json:"input_tokens"`
	OutputTokens     int64        `json:"output_tokens"`
	EstimatedCostUSD float64      `json:"estimated_cost_usd"`
	ErrorRate        float64      `json:"error_rate"` // (error + timeout) / generations
	AvgDurationMs    *int         `json:"avg_duration_ms"`
	P50TTFTMs        *int         `json:"p50_ttft_ms"`
	ByStatus         StatusCounts `json:"by_status"`
}

type Daily struct {
	Date             string  `json:"date"`
	Generations      int     `json:"generations"`
	ActiveUsers      int     `json:"active_users"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	ErrorRate        float64 `json:"error_rate"`
	AvgDurationMs    *int    `json:"avg_duration_ms"`
}

type Stats struct {
	Range   RangeInfo `json:"range"`
	Pricing Pricing   `json:"pricing"`
	Summary Summary   `json:"summary"`
	Daily   []Daily   `json:"daily"`
}

// Stats menghitung ringkasan dan deret harian. Biaya dihitung saat query dari
// harga di env, jadi perubahan harga tidak mengubah data lama.
func (s *Service) Stats(ctx context.Context, r DateRange) (*Stats, error) {
	ctx, cancel := context.WithTimeout(ctx, db.QueryTimeout)
	defer cancel()

	st := &Stats{
		Range: RangeInfo{From: r.From.Format(dateLayout), To: r.To.Format(dateLayout), Timezone: s.loc.String()},
		Pricing: Pricing{InputPerMTok: s.priceIn, OutputPerMTok: s.priceOut,
			Configured: s.priceIn > 0 || s.priceOut > 0},
	}

	var avgDur, p50 *float64
	sm := &st.Summary
	err := s.db.Pool.QueryRow(ctx, `
		SELECT count(*),
		       count(DISTINCT user_id),
		       coalesce(sum(input_tokens), 0),
		       coalesce(sum(output_tokens), 0),
		       count(*) FILTER (WHERE status = 'success'),
		       count(*) FILTER (WHERE status = 'error'),
		       count(*) FILTER (WHERE status = 'cancelled'),
		       count(*) FILTER (WHERE status = 'timeout'),
		       avg(duration_ms) FILTER (WHERE status = 'success'),
		       percentile_cont(0.5) WITHIN GROUP (ORDER BY ttft_ms)
		         FILTER (WHERE status = 'success' AND ttft_ms IS NOT NULL)
		FROM generation_logs
		WHERE created_at >= $1 AND created_at < $2`,
		r.start(), r.end(),
	).Scan(&sm.Generations, &sm.ActiveUsers, &sm.InputTokens, &sm.OutputTokens,
		&sm.ByStatus.Success, &sm.ByStatus.Error, &sm.ByStatus.Cancelled, &sm.ByStatus.Timeout,
		&avgDur, &p50)
	if err != nil {
		return nil, err
	}
	sm.EstimatedCostUSD = s.cost(sm.InputTokens, sm.OutputTokens)
	sm.ErrorRate = rate(sm.ByStatus.Error+sm.ByStatus.Timeout, sm.Generations)
	sm.AvgDurationMs, sm.P50TTFTMs = roundPtr(avgDur), roundPtr(p50)

	rows, err := s.db.Pool.Query(ctx, `
		WITH agg AS (
		  SELECT (created_at AT TIME ZONE $5)::date AS day,
		         count(*) AS generations,
		         count(DISTINCT user_id) AS active_users,
		         sum(input_tokens) AS input_tokens,
		         sum(output_tokens) AS output_tokens,
		         count(*) FILTER (WHERE status IN ('error', 'timeout')) AS errors,
		         avg(duration_ms) FILTER (WHERE status = 'success') AS avg_duration
		  FROM generation_logs
		  WHERE created_at >= $1 AND created_at < $2
		  GROUP BY 1
		)
		SELECT to_char(d, 'YYYY-MM-DD'),
		       coalesce(agg.generations, 0), coalesce(agg.active_users, 0),
		       coalesce(agg.input_tokens, 0), coalesce(agg.output_tokens, 0),
		       coalesce(agg.errors, 0), agg.avg_duration
		FROM generate_series($3::date, $4::date, interval '1 day') AS d
		LEFT JOIN agg ON agg.day = d::date
		ORDER BY d`,
		r.start(), r.end(), st.Range.From, st.Range.To, s.loc.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d Daily
		var errors int
		var avg *float64
		if err := rows.Scan(&d.Date, &d.Generations, &d.ActiveUsers, &d.InputTokens, &d.OutputTokens,
			&errors, &avg); err != nil {
			return nil, err
		}
		d.EstimatedCostUSD = s.cost(d.InputTokens, d.OutputTokens)
		d.ErrorRate = rate(errors, d.Generations)
		d.AvgDurationMs = roundPtr(avg)
		st.Daily = append(st.Daily, d)
	}
	return st, rows.Err()
}

func (s *Service) cost(in, out int64) float64 {
	c := float64(in)/1e6*s.priceIn + float64(out)/1e6*s.priceOut
	return math.Round(c*1e6) / 1e6
}

func rate(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return math.Round(float64(n)/float64(total)*1e4) / 1e4
}

func roundPtr(f *float64) *int {
	if f == nil {
		return nil
	}
	n := int(math.Round(*f))
	return &n
}
