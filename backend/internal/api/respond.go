package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"
)

// errorBody adalah bentuk standar semua error JSON (sebelum stream dimulai).
type errorBody struct {
	Error  string            `json:"error"`
	Code   string            `json:"code"`
	Fields map[string]string `json:"fields,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("tulis respons JSON gagal", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorBody{Error: msg, Code: code})
}

func writeInternalError(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, "internal_error", "Terjadi kesalahan pada server, coba lagi.")
}

// writeFieldErrors membalas 400 dengan detail per field. Jika hanya satu field
// yang salah, pesannya sekaligus dipakai sebagai pesan utama.
func writeFieldErrors(w http.ResponseWriter, fields map[string]string) {
	msg := "Periksa kembali isian yang ditandai."
	if len(fields) == 1 {
		for _, m := range fields {
			msg = m
		}
	}
	writeJSON(w, http.StatusBadRequest, errorBody{Error: msg, Code: "invalid_input", Fields: fields})
}

func writeRateLimited(w http.ResponseWriter, retryAfter time.Duration) {
	secs := max(1, int(math.Ceil(retryAfter.Seconds())))
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeError(w, http.StatusTooManyRequests, "rate_limited",
		fmt.Sprintf("Terlalu banyak permintaan, coba lagi dalam %d detik.", secs))
}

// decodeJSON membaca body ke dst. Jika gagal, respons error sudah ditulis dan hasilnya false.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	err := json.NewDecoder(r.Body).Decode(dst)
	if err == nil {
		return true
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Input terlalu panjang.")
	} else {
		writeError(w, http.StatusBadRequest, "invalid_input", "Format request tidak valid.")
	}
	return false
}
