package main

import (
	"log"
	"net/http"
	"time"
)

// Server wiring only. Handlers and latency profiles live in handlers.go.
func main() {
	mux := http.NewServeMux()

	// All endpoints feed the same system-wide latency histogram via measure().
	mux.HandleFunc("GET /fast", measure(handle(fastProfile)))
	mux.HandleFunc("GET /average", measure(handle(averageProfile)))
	mux.HandleFunc("GET /slow", measure(handle(slowProfile)))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	// Prometheus scrape target -> Grafana reads from here.
	mux.Handle("GET /metrics", metrics)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      logging(mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	log.Println("listening on :8080  (endpoints: /fast /average /slow /health)")
	log.Fatal(srv.ListenAndServe())
}
