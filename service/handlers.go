package main

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// Quote is the payload. Values are fixed on purpose: the body must be
// byte-identical between runs, or a changing length reads as protocol
// behaviour. No timestamps, no random values.
type Quote struct {
	ID   int64   `json:"id"`
	Pair string  `json:"pair"`
	Bid  float64 `json:"bid"`
}

// oneHandler returns a single quote (~90 B). With a body this small, headers
// are a large share of the bytes on the wire, so HPACK (h2) shows up.
//
// No database, no sleep, no per-request logging: we are measuring the
// protocol, and anything the handler does is noise on top of the signal.
func oneHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	writeJSON(w, Quote{ID: id, Pair: "USDKZT", Bid: 533.14})
}

// manyHandler returns n quotes (n=200 → ~18 KB). At this size the initial
// congestion window starts to matter for time-to-last-byte.
func manyHandler(w http.ResponseWriter, r *http.Request) {
	n := 1
	if v := r.URL.Query().Get("n"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			n = parsed
		}
	}
	quotes := make([]Quote, 0, n)
	for i := int64(1); i <= int64(n); i++ {
		quotes = append(quotes, Quote{ID: i, Pair: "USDKZT", Bid: 533.14})
	}
	writeJSON(w, quotes)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	// Marshal first so a serialization error never leaves a half-written body.
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(b)
}
