package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSPAHandler(t *testing.T) {
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, "assets"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>app</title>"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "assets", "index-abc.js"), []byte("console.log(1)"), 0o644))
	must(CheckStaticDir(dir))
	if CheckStaticDir(t.TempDir()) == nil {
		t.Error("CheckStaticDir tanpa index.html harus error")
	}

	// Sama seperti router: /api/ lebih spesifik daripada fallback SPA.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", notFound)
	mux.Handle("/", spaHandler(dir))

	tests := []struct {
		method, path string
		wantStatus   int
		wantBody     string
		wantCache    string
	}{
		{"GET", "/", 200, "<title>app</title>", "no-cache"},
		{"GET", "/admin/logs", 200, "<title>app</title>", "no-cache"},
		{"HEAD", "/login", 200, "", "no-cache"},
		{"GET", "/assets/index-abc.js", 200, "console.log", "immutable"},
		{"GET", "/assets/hilang.js", 404, "", ""},
		{"GET", "/api/tidak-ada", 404, `"code":"not_found"`, ""},
		{"POST", "/admin", 405, "method_not_allowed", ""},
	}
	t.Run("traversal tetap di dalam dir", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.URL.Path = "/../../etc/passwd" // lolos dari pembersihan ServeMux
		rec := httptest.NewRecorder()
		spaHandler(dir).ServeHTTP(rec, req)
		// http.ServeFile menolak path berisi ".." sebelum membuka file apa pun.
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, body = %q; want 400", rec.Code, rec.Body.String())
		}
	})

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want berisi %q", rec.Body.String(), tt.wantBody)
			}
			if !strings.Contains(rec.Header().Get("Cache-Control"), tt.wantCache) {
				t.Errorf("Cache-Control = %q, want berisi %q", rec.Header().Get("Cache-Control"), tt.wantCache)
			}
		})
	}
}
