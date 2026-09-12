// Command service is the Week 2 endpoint under test.
//
// It serves ONE JSON endpoint over TLS on :8443 and advertises both
// h2 and http/1.1 via ALPN. The client (through the Caddy edge) picks the
// version, which is exactly why the protocol is a variable in the load test
// and not a deployment decision.
//
// Go's net/http speaks HTTP/1.1 and HTTP/2, but NOT HTTP/3. That is why the
// rig puts Caddy in front: Caddy terminates h3/h2/http1.1 and reverse-proxies
// here. See edge/Caddyfile and REPORT.md.
package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/quote/{id}", oneHandler)
	mux.HandleFunc("GET /api/quotes", manyHandler)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{
		Addr:    ":8443",
		Handler: mux,

		// Documented server limits (the Go analogue of Tomcat's
		// threads.max / max-connections / accept-count). Go uses one
		// goroutine per request rather than a fixed thread pool, so the
		// real ceiling on HTTP/2 is the stream limit below, plus these
		// timeouts. State these in REPORT.md as "our limits".
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,

		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13, // TLS 1.3 only, like the lab spec
		},
	}

	// HTTP/2 concurrency ceiling, advertised to peers as
	// SETTINGS_MAX_CONCURRENT_STREAMS. This is the h2 equivalent of a
	// connection pool size: one connection, many streams, capped here.
	srv.HTTP2 = &http.HTTP2Config{MaxConcurrentStreams: 250}

	log.Printf("service listening on :8443 (TLS 1.3, ALPN h2 + http/1.1, max_concurrent_streams=250)")
	// ListenAndServeTLS + a TLS config makes Go negotiate h2 via ALPN
	// automatically and fall back to http/1.1 when the client asks for it.
	if err := srv.ListenAndServeTLS("/certs/cert.pem", "/certs/key.pem"); err != nil {
		log.Fatalf("server: %v", err)
	}
}
