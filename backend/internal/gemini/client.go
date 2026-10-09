// Package gemini adalah client HTTP minimal untuk streaming Gemini API
// (streamGenerateContent dengan alt=sse).
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const DefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Request adalah satu permintaan generate satu giliran.
type Request struct {
	System          string
	Prompt          string
	MaxOutputTokens int    // termasuk token thinking
	ThinkingLevel   string // kosong = default model
}

// Usage diambil dari usageMetadata terakhir di stream.
type Usage struct {
	InputTokens  int
	OutputTokens int    // teks + thinking, karena keduanya ditagih sebagai output
	StopReason   string // finishReason Gemini dalam huruf kecil, mis. "stop", "max_tokens"
}

// Streamer memungkinkan handler dites dengan mock tanpa memanggil API asli.
// onText dipanggil untuk setiap potongan teks; error darinya menghentikan stream.
// Usage tetap dikembalikan (sebagian) saat terjadi error.
type Streamer interface {
	Stream(ctx context.Context, req Request, onText func(string) error) (Usage, error)
}

type ErrorKind int

const (
	// KindUpstream: request ditolak atau stream rusak (4xx, respons tak terduga).
	KindUpstream ErrorKind = iota
	// KindOverloaded: layanan sibuk atau kuota habis (429, 5xx); aman dicoba lagi.
	KindOverloaded
	// KindBlocked: prompt atau jawaban diblokir filter keamanan Gemini.
	KindBlocked
)

// Error membawa pesan mentah dari Gemini. Message hanya untuk log server,
// jangan dikirim ke klien.
type Error struct {
	Kind    ErrorKind
	Status  int // status HTTP dari Gemini; 0 jika error terjadi di dalam stream
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("gemini: status %d: %s", e.Status, e.Message)
}

type Client struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

type Option func(*Client)

func WithBaseURL(u string) Option          { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

func New(apiKey, model string, opts ...Option) *Client {
	// Tanpa http.Client.Timeout: durasi stream dibatasi lewat context (GENERATE_TIMEOUT).
	c := &Client{apiKey: apiKey, model: model, baseURL: DefaultBaseURL, http: &http.Client{}}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) Model() string { return c.model }

type part struct {
	Text string `json:"text"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type thinkingConfig struct {
	ThinkingLevel string `json:"thinkingLevel,omitempty"`
}

type generationConfig struct {
	MaxOutputTokens int             `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *thinkingConfig `json:"thinkingConfig,omitempty"`
}

type requestBody struct {
	SystemInstruction *content         `json:"systemInstruction,omitempty"`
	Contents          []content        `json:"contents"`
	GenerationConfig  generationConfig `json:"generationConfig"`
}

func (c *Client) Stream(ctx context.Context, req Request, onText func(string) error) (Usage, error) {
	body := requestBody{
		Contents:         []content{{Role: "user", Parts: []part{{Text: req.Prompt}}}},
		GenerationConfig: generationConfig{MaxOutputTokens: req.MaxOutputTokens},
	}
	if req.System != "" {
		body.SystemInstruction = &content{Parts: []part{{Text: req.System}}}
	}
	if req.ThinkingLevel != "" {
		body.GenerationConfig.ThinkingConfig = &thinkingConfig{ThinkingLevel: req.ThinkingLevel}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Usage{}, err
	}

	url := c.baseURL + "/models/" + c.model + ":streamGenerateContent?alt=sse"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return Usage{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	// Key lewat header, bukan ?key= di URL, agar tidak ikut tercatat di log.
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	res, err := c.http.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return Usage{}, ctx.Err()
		}
		return Usage{}, &Error{Kind: KindOverloaded, Message: "koneksi gagal: " + err.Error()}
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Usage{}, errorFromResponse(res)
	}
	usage, err := readStream(res.Body, onText)
	if err != nil && ctx.Err() != nil {
		return usage, ctx.Err()
	}
	return usage, err
}

// apiError adalah bentuk error JSON Google: {"error":{"code","message","status"}}.
type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

func errorFromResponse(res *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	var body struct {
		Error apiError `json:"error"`
	}
	msg := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &body) == nil && body.Error.Message != "" {
		msg = body.Error.Status + ": " + body.Error.Message
	}
	return &Error{Kind: kindForStatus(res.StatusCode), Status: res.StatusCode, Message: msg}
}

func kindForStatus(status int) ErrorKind {
	if status == http.StatusTooManyRequests || status >= 500 {
		return KindOverloaded
	}
	return KindUpstream
}

var _ Streamer = (*Client)(nil)

// IsKind mengecek jenis error Gemini.
func IsKind(err error, kind ErrorKind) bool {
	var e *Error
	return errors.As(err, &e) && e.Kind == kind
}
