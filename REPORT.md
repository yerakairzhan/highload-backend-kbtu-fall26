# REPORT — Load Testing HTTP/1.1 vs HTTP/2 vs HTTP/3

Week 2 practice · Backend for Highloaded Environment · KBTU Fall 2026

> **Status: the rig is complete and verified; the measurement table is pending
> one attended run.** Three full runs of the suite were discarded because the
> host slept mid-run (see *Discarded datasets*). Everything below that does not
> depend on timing is measured and final. To fill the table:
> `RATE=300 DURATION=60s bash run-all.sh && python3 make-report.py` — with the
> laptop awake, lid open, ~13 minutes.

## Environment (state it or it is an opinion)

| | |
|---|---|
| Hardware | Apple M5, 10 cores, 24 GB RAM, macOS 26.5.2 |
| Container host | Docker Desktop 28.5.2 · VM: 10 CPUs, 8 GB, linuxkit 6.12.54, aarch64 |
| Service | Go 1.26.8 `net/http` (**not** Java 21 / Spring Boot — see Deviations) |
| Edge | Caddy v2.8.4 (h3 via quic-go), TLS 1.3, self-signed EC P-256 cert with SAN |
| Load tools | k6 v2.2.0 (HTTP/1.1, HTTP/2) · `h3client` = Go + quic-go v0.48.2 (HTTP/3, and controlled h2/h3 runs) |
| Impairment | `tc` iproute2 6.9.0 · `netem delay 100ms 10ms loss 1%` on **both** directions of the client↔edge link |
| Offered load | **300 req/s, open-loop (constant arrival rate), 60 s, every run** |
| Endpoint / payload | `GET /api/quote/1` → **37 B** JSON body (byte-identical across calls, md5-verified); `/api/quotes?n=200` → 7 893 B |
| Extra hop | Every protocol run crosses the same Caddy edge (one extra process); stated so it cancels out |

**Server limits (the Go analogue of Tomcat's `threads.max / max-connections / accept-count`).**
Go runs one goroutine per request, so there is no fixed thread cap. The ceilings are:
HTTP/2 `MaxConcurrentStreams = 250` (advertised as `SETTINGS_MAX_CONCURRENT_STREAMS`),
`ReadHeaderTimeout 2 s`, `ReadTimeout 5 s`, `WriteTimeout 10 s`, `IdleTimeout 60 s`, TLS 1.3 minimum.
Client timeouts: `h3client` request timeout 5 s; k6 defaults. Caddy edge at defaults.

## Proof of the negotiated version (do not assume — assert)

Single requests through the edge, negotiated protocol read from the response, body length checked
(an empty 200 would mean the edge answered without proxying):

```
asked=h1  negotiated=HTTP/1.1  status=200 bytes=37   connect=0.4ms  tls=1.3ms  ttfb=2.3ms   https://edge:9444/api/quote/1
asked=h2  negotiated=HTTP/2.0  status=200 bytes=37   connect=0.4ms  tls=0.9ms  ttfb=1.5ms   https://edge:9443/api/quote/1
asked=h3  negotiated=HTTP/3.0  status=200 bytes=37   connect=n/a    tls=n/a    ttfb=n/a     https://edge:9443/api/quote/1
```

- `:9444` offers **only** `http/1.1` in ALPN — asking it for h2 returns `HTTP/1.1`. This is how the
  HTTP/1.1 run is forced honestly (k6 cannot pin a version per request).
- QUIC has no separate TCP/TLS phases, hence `n/a` for h3; the handshake is inside the QUIC connect.
- Reproduce: `docker compose exec client h3client -mode prove -proto h1|h2|h3 -url …`

## Results

<!-- RESULTS:BEGIN -->
_Not yet generated — run the suite, then `python3 make-report.py`._
<!-- RESULTS:END -->

**How the numbers are produced.** p50/p95/p99 are `http_req_duration` (k6) or per-request total time
(`h3client`); never the mean. Achieved RPS is iterations ÷ configured duration (see *Discarded
datasets* for why k6's own `http_reqs.rate` is not used). The handshake cost is
`(cold − warm)` on `http_req_connecting + http_req_tls_handshaking`, the two k6 metrics that isolate TCP
and TLS. Rows marked INVALID were rejected by the host wall-clock guard and are struck through.

## Three explanations (to be written from the VALID run)

Each must name a mechanism: handshake round trips, initial congestion window, multiplexing,
head-of-line blocking, HPACK/QPACK. The candidate mechanisms below are **hypotheses to confirm or
refute against the table — not conclusions**; two earlier "obvious" explanations turned out to be
clock artefacts, which is exactly why the table is generated only from validated runs.

1. **Clean link, one request at a time (Claim 2).** Expect h1 ≈ h2 ≈ h3 at p50: at 300 req/s with
   sub-millisecond service time, Little's law gives L = λW ≈ 300 × 0.0005 ≈ 0.15 requests in flight —
   effectively concurrency 1, so multiplexing has nothing to multiplex. Check whether h2's tail is
   worse than h1's; if so, the mechanism to test is *serialisation on a single TCP connection*
   (h2_clean_1conn vs h2_clean, which is one connection per k6 VU).

2. **Cold vs warm (Claim 1).** The handshake cost in ms comes straight from the second table. On the
   clean link it should be small (connect ≈ 0.1 ms + TLS ≈ 1 ms measured on single requests); the
   point is that it is *one TCP RTT plus one TLS 1.3 RTT*, so on the 200 ms link it becomes the
   dominant term — compare with `h1_netem`/`h2_netem` p50 (≈ one RTT ≈ 200 ms).

3. **Loss (Claim 3): HTTP/2 vs HTTP/3 on ONE connection each.** The fair fight is `h2_netem_1conn`
   vs `h3_netem`: one congestion controller each. In the (invalid, so illustrative only) runs, a
   single QUIC connection saturated at ≈160 req/s under 1 % loss — the loss-based throughput bound
   MSS/RTT · 1.22/√p ≈ 73 KB/s ≈ 160 req/s for ~450 B on the wire per exchange. Whether h2-on-one-
   connection does the same or worse (TCP head-of-line blocking stalls *all* streams on one lost
   segment; QUIC recovers per stream) is read off p99. `h3_netem_c50` then tests whether the ceiling
   is per-connection: 50 congestion windows should lift it. The k6 h2 row uses hundreds of
   connections and is therefore *not* the lecture's "one connection, many streams".

## Discarded datasets (an honest limitation costs nothing)

Three complete runs were rejected. The cause was found from independent evidence, not guessed:

| Run | Symptom | Evidence |
|---|---|---|
| 1 | netem RTT 94 ms instead of ~200 ms; only one direction impaired | `tc` rejected the edge interface name `eth0@if328`; fixed by stripping the veth suffix |
| 1–3 | k6 reported a 60 s scenario as 58 min; single "max" latencies of 100–600 s; inflated p95/p99 | `state.testRunDurationMs` vs iteration count (exactly 300 × 60); host wall-clock stamps of 126–1184 s per 60 s run; `pmset -g log` sleep entries |
| 3 | all 30 attempts INVALID despite `caffeinate` | closed-lid sleep overrides idle-sleep assertions; brief wakes let each run complete in fragments |

Mechanism: when the host sleeps the Docker VM pauses; k6 schedules on the *realtime* clock, so on
resume it fires every missed iteration as a burst — a tail that looks like protocol behaviour and is
not. `h3client` uses Go's monotonic clock and does not burst, but its runs span the same pauses.
The guards in `run-all.sh` (host wall-clock stamp, k6 clock-jump flag, netem/RTT assertions,
automatic retry) exist because of this; `results/wallclock.txt` carries the verdict for every run.

## Deviations from the task sheet

1. Go service instead of Java 21 / Spring Boot (consistent with Week 1; mechanisms are protocol-level).
2. `h3client` (quic-go) instead of `curl --http3`/`h2load`: no arm64 HTTP/3 curl image exists; the
   negotiated version comes from `res.Proto`, the same truth as `%{http_version}`.
3. Payloads 37 B / 7.9 KB rather than ~90 B / ~18 KB (compact JSON).
4. HTTP/3 load is open-loop at the same 300 req/s as k6, not closed-loop "as fast as possible".
5. Connection model made explicit (`-conns`), because k6's h2 is many connections.

## Reproduce

```bash
bash certs/gen-cert.sh
docker compose up -d --build
docker compose exec client h3client -mode prove -proto h3 -url https://edge:9443/api/quote/1
RATE=300 DURATION=60s bash run-all.sh      # keep the machine awake, lid open
python3 make-report.py                     # fills the Results section above
```

AI usage: see README.md.
