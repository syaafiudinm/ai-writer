package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
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
