package db

import "context"

// Nilai kolom generation_logs.status.
const (
	GenStatusSuccess   = "success"
	GenStatusError     = "error"
	GenStatusCancelled = "cancelled"
	GenStatusTimeout   = "timeout"
)

// GenerationLog hanya berisi metadata; facts dan isi draf sengaja tidak ada.
type GenerationLog struct {
	UserID       string
	Type         string
	Tone         string
	Length       string
	Language     string
	Topic        string
	Model        string
	Status       string
	ErrorCode    string
	InputTokens  int
	OutputTokens int
	TTFTMs       *int // nil jika teks pertama tidak pernah muncul
	DurationMs   int
	IP           string
}

func (d *DB) InsertGenerationLog(ctx context.Context, l GenerationLog) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := d.Pool.Exec(ctx,
		`INSERT INTO generation_logs (user_id, type, tone, length, language, topic, model, status,
		   error_code, input_tokens, output_tokens, ttft_ms, duration_ms, ip)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		l.UserID, l.Type, l.Tone, l.Length, l.Language, l.Topic, l.Model, l.Status,
		nullable(l.ErrorCode), l.InputTokens, l.OutputTokens, l.TTFTMs, l.DurationMs, nullable(l.IP))
	return err
}
