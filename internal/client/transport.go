// Package client builds the clients the load generator uses: the same
// Connect-Go stubs speak gRPC (HTTP/2) or Connect/JSON (HTTP/1.1).
package client

import (
	"context"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type HTTPVersion string

const (
	HTTP1 HTTPVersion = "HTTP/1.1"
	H2C   HTTPVersion = "HTTP/2.0"
)

// WireCounter counts every byte the client's TCP connections send and
// receive, headers and framing included.
type WireCounter struct {
	Sent, Received atomic.Int64
}

type countingConn struct {
	net.Conn
	c *WireCounter
}

func (c countingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.c.Received.Add(int64(n))
	return n, err
}

func (c countingConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.c.Sent.Add(int64(n))
	return n, err
}

func (w *WireCounter) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	return countingConn{conn, w}, nil
}

// ProtoProbe records what the first response actually negotiated, so a run
// fails loudly instead of silently measuring the wrong protocol.
type ProtoProbe struct {
	once        sync.Once
	Proto       string
	ContentType string
	Encoding    string
}

type probeTransport struct {
	next  http.RoundTripper
	probe *ProtoProbe
}

func (t probeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := t.next.RoundTrip(req)
	if err == nil {
		t.probe.once.Do(func() {
			t.probe.Proto = res.Proto
			t.probe.ContentType = res.Header.Get("Content-Type")
			t.probe.Encoding = res.Header.Get("Content-Encoding") + res.Header.Get("Grpc-Encoding")
		})
	}
	return res, err
}

// NewHTTPClient returns a client for one HTTP version. Compression is off on
// both: Go's HTTP/1.1 transport would otherwise ask for gzip by itself and the
// JSON baseline would quietly be compressed while protobuf was not.
func NewHTTPClient(version HTTPVersion, wire *WireCounter, probe *ProtoProbe) *http.Client {
	var protocols http.Protocols
	if version == H2C {
		protocols.SetUnencryptedHTTP2(true) // prior knowledge: no HTTP/1.1 upgrade dance
	} else {
		protocols.SetHTTP1(true)
	}
	rt := &http.Transport{
		Protocols:           &protocols,
		DialContext:         wire.dial,
		DisableCompression:  true,
		MaxIdleConns:        1024,
		MaxIdleConnsPerHost: 1024,
		IdleConnTimeout:     90 * time.Second,
	}
	return &http.Client{Transport: probeTransport{rt, probe}}
}
