package gemini

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// maxEventBytes membatasi satu event SSE; chunk Gemini bisa memuat thoughtSignature panjang.
const maxEventBytes = 1 << 20

type chunk struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text    string `json:"text"`
				Thought bool   `json:"thought"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	UsageMetadata *struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
	Error *apiError `json:"error"`
}

// finishReason yang berarti jawaban dihentikan oleh filter, bukan selesai normal.
var blockedFinish = map[string]bool{
	"SAFETY": true, "RECITATION": true, "BLOCKLIST": true, "PROHIBITED_CONTENT": true,
	"SPII": true, "IMAGE_SAFETY": true,
}

// readStream membaca event SSE dari Gemini sampai selesai.
func readStream(r io.Reader, onText func(string) error) (Usage, error) {
	var usage Usage
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxEventBytes)

	var data strings.Builder
	dispatch := func() error {
		if data.Len() == 0 {
			return nil
		}
		raw := data.String()
		data.Reset()
		return handleChunk(raw, &usage, onText)
	}

	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if err := dispatch(); err != nil {
				return usage, err
			}
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
		// Baris lain (event:, id:, komentar) tidak dipakai Gemini.
	}
	if err := sc.Err(); err != nil {
		return usage, &Error{Kind: KindUpstream, Message: "baca stream: " + err.Error()}
	}
	if err := dispatch(); err != nil { // event terakhir tanpa baris kosong penutup
		return usage, err
	}
	if usage.StopReason == "" {
		return usage, &Error{Kind: KindUpstream, Message: "stream berakhir tanpa finishReason"}
	}
	return usage, nil
}

func handleChunk(raw string, usage *Usage, onText func(string) error) error {
	var c chunk
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return &Error{Kind: KindUpstream, Message: "chunk bukan JSON valid: " + err.Error()}
	}
	if c.Error != nil {
		return &Error{Kind: kindForStatus(c.Error.Code), Message: c.Error.Status + ": " + c.Error.Message}
	}
	if m := c.UsageMetadata; m != nil {
		// Nilai usage bersifat kumulatif; yang terakhir adalah total.
		usage.InputTokens = m.PromptTokenCount
		usage.OutputTokens = m.CandidatesTokenCount + m.ThoughtsTokenCount
	}
	if fb := c.PromptFeedback; fb != nil && fb.BlockReason != "" {
		usage.StopReason = "blocked"
		return &Error{Kind: KindBlocked, Message: "prompt diblokir: " + fb.BlockReason}
	}
	for _, cand := range c.Candidates {
		for _, p := range cand.Content.Parts {
			if p.Thought || p.Text == "" {
				continue
			}
			if err := onText(p.Text); err != nil {
				return err
			}
		}
		if fr := cand.FinishReason; fr != "" {
			usage.StopReason = strings.ToLower(fr)
			if blockedFinish[fr] {
				return &Error{Kind: KindBlocked, Message: "jawaban dihentikan: " + fr}
			}
		}
	}
	return nil
}
