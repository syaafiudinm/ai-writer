package api

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
)

// spaHandler menyajikan build React dari dir. Path tanpa file yang cocok jatuh
// ke index.html agar route sisi klien (mis. /admin/logs) tetap jalan saat di-refresh.
func spaHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Metode tidak diizinkan.")
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")

		p := path.Clean("/" + r.URL.Path)
		if p != "/" {
			if fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err == nil && !fi.IsDir() {
				// Nama file di /assets/ berisi hash konten, jadi aman di-cache selamanya.
				if path.Dir(p) == "/assets" {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
			// File statis yang hilang (mis. chunk lama setelah deploy) → 404, bukan HTML.
			if path.Ext(p) != "" {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}

// CheckStaticDir memastikan STATIC_DIR berisi build React sebelum server jalan.
func CheckStaticDir(dir string) error {
	_, err := os.Stat(filepath.Join(dir, "index.html"))
	return err
}
