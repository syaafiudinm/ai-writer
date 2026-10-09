package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// sseWriter menulis event Server-Sent Events dan flush setiap event agar teks
// langsung sampai ke browser. Header baru dikirim saat event pertama, sehingga
// error sebelum itu masih bisa dibalas sebagai JSON biasa.
type sseWriter struct {
	w       http.ResponseWriter
	rc      *http.ResponseController
	started bool
}

func newSSEWriter(w http.ResponseWriter) *sseWriter {
	return &sseWriter{w: w, rc: http.NewResponseController(w)}
}

func (s *sseWriter) start() {
	if s.started {
		return
	}
	s.started = true
	h := s.w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // matikan buffering Nginx
	s.w.WriteHeader(http.StatusOK)
}

// event menulis satu event; name kosong = event default ("message").
func (s *sseWriter) event(name string, data any) error {
	s.start()
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if name != "" {
		if _, err := fmt.Fprintf(s.w, "event: %s\n", name); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", payload); err != nil {
		return err
	}
	return s.rc.Flush()
}
