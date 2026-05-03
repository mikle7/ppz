package desktop

import (
	"bytes"
	"embed"
	"net/http"
)

//go:embed web/index.html
var webFS embed.FS

// ServeHTTP starts a tiny HTTP server that:
//   - GET /            → serves the embedded HTML page
//   - GET /api/state   → returns the same JSON snapshot --dump-state produces
//                        (so the page can poll it on a 1 s interval)
//
// Blocks until the server errors out.
func ServeHTTP(addr, ipcSock string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// Only serve the page at exactly "/" — anything else is 404 so the
		// state endpoint stays unambiguous.
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, err := webFS.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		if err := DumpState(ipcSock, &buf); err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(buf.Bytes())
	})

	srv := &http.Server{Addr: addr, Handler: mux}
	return srv.ListenAndServe()
}
