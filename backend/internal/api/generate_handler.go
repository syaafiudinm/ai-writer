package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"ai-writer/internal/auth"
	"ai-writer/internal/db"
	"ai-writer/internal/gemini"
	"ai-writer/internal/generate"
	"ai-writer/internal/ratelimit"
)

// thinkingAllowance ditambahkan ke batas token jika thinking aktif, karena
// token thinking Gemini ikut memakan maxOutputTokens.
const thinkingAllowance = 8192

// errClientGone menandai penulisan ke klien gagal (koneksi putus).
var errClientGone = errors.New("klien terputus")

type generateHandler struct {
	streamer      gemini.Streamer
	model         string
	thinkingLevel string
	timeout       time.Duration
	limiter       *ratelimit.Limiter // generate per user
	db            *db.DB
	logger        *slog.Logger
}

type failure struct {
	status   string // status generation_logs
	httpCode int    // dipakai jika stream belum dimulai
	code     string
	msg      string
}

func (h *generateHandler) generate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := auth.UserFrom(ctx)

	var req generate.Request
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Normalize()
	if fields := req.Validate(); fields != nil {
		writeFieldErrors(w, fields)
		return
	}
	if ok, retry := h.limiter.Allow(user.ID); !ok {
		writeRateLimited(w, retry)
		return
	}

	prompt := generate.Build(req)
	maxTokens := prompt.MaxOutputTokens
	if h.thinkingLevel != "minimal" {
		maxTokens += thinkingAllowance
	}

	start := time.Now()
	var ttft *int
	sse := newSSEWriter(w)
	onText := func(text string) error {
		if !sse.started {
			ms := int(time.Since(start).Milliseconds())
			ttft = &ms
			if err := sse.event("meta", map[string]string{"model": h.model}); err != nil {
				return errClientGone
			}
		}
		if err := sse.event("", map[string]string{"text": text}); err != nil {
			return errClientGone
		}
		return nil
	}

	streamCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	usage, err := h.streamer.Stream(streamCtx, gemini.Request{
		System:          prompt.System,
		Prompt:          prompt.User,
		MaxOutputTokens: maxTokens,
		ThinkingLevel:   h.thinkingLevel,
	}, onText)

	if err == nil && !sse.started {
		err = &gemini.Error{Kind: gemini.KindUpstream, Message: "stream selesai tanpa teks"}
	}

	entry := db.GenerationLog{
		UserID: user.ID, Type: req.Type, Tone: req.Tone, Length: req.Length, Language: req.Language,
		Topic: req.Topic, Model: h.model, Status: db.GenStatusSuccess,
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, TTFTMs: ttft,
		IP: ClientIPFrom(ctx),
	}

	if err == nil {
		_ = sse.event("done", map[string]any{
			"stop_reason":   usage.StopReason,
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
		})
	} else {
		f := h.classify(ctx, streamCtx, err)
		entry.Status, entry.ErrorCode = f.status, f.code
		h.logger.WarnContext(ctx, "generate gagal",
			"request_id", RequestIDFrom(ctx), "user_id", user.ID,
			"status", f.status, "code", f.code, "error", err)

		switch {
		case f.status == db.GenStatusCancelled:
			// Klien sudah pergi; tidak ada yang perlu ditulis.
		case sse.started:
			_ = sse.event("error", map[string]string{"error": f.msg, "code": f.code})
		default:
			writeError(w, f.httpCode, f.code, f.msg)
		}
	}

	entry.DurationMs = int(time.Since(start).Milliseconds())
	// Tetap simpan log walau klien sudah membatalkan request.
	if err := h.db.InsertGenerationLog(context.WithoutCancel(ctx), entry); err != nil {
		h.logger.ErrorContext(ctx, "simpan generation_log gagal", "error", err)
	}
}

func (h *generateHandler) classify(reqCtx, streamCtx context.Context, err error) failure {
	switch {
	case errors.Is(err, errClientGone) || reqCtx.Err() != nil:
		return failure{db.GenStatusCancelled, 0, "cancelled", ""}
	case errors.Is(streamCtx.Err(), context.DeadlineExceeded):
		return failure{db.GenStatusTimeout, http.StatusGatewayTimeout, "timeout",
			"Waktu habis, coba panjang yang lebih pendek."}
	case gemini.IsKind(err, gemini.KindOverloaded):
		return failure{db.GenStatusError, http.StatusServiceUnavailable, "upstream_overloaded",
			"Layanan AI sedang sibuk, coba lagi sebentar."}
	case gemini.IsKind(err, gemini.KindBlocked):
		return failure{db.GenStatusError, http.StatusUnprocessableEntity, "content_blocked",
			"Permintaan ditolak filter keamanan layanan AI. Ubah topik atau fakta lalu coba lagi."}
	default:
		return failure{db.GenStatusError, http.StatusBadGateway, "upstream_error",
			"Gagal menghubungi layanan AI."}
	}
}
