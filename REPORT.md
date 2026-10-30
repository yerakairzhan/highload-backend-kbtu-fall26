# SIS #2 — Report: gRPC vs REST/JSON

## Summary

- **Payload: gRPC sends fewer bytes in every scenario.** Per request, counting every byte on the TCP
  connection: −9 % for a post page, −32 % for a comment page, −47 % for creating a comment. Two
  mechanisms: HTTP/2 sends repeated headers (the 242-byte JWT among them) once per connection instead
  of once per request, and protobuf drops field names and encodes numbers in binary.
- **Latency: on a local network the JSON/HTTP/1.1 baseline was faster.** Median latency was
  0.09–0.18 ms lower for JSON in all 9 run pairs, and p99 was lower for post pages and writes. The one
  place gRPC won on latency was the far tail of the comment page: JSON's p99.9 hit 47–83 ms in 2 of 3
  runs, gRPC's never exceeded 5.2 ms.
- **For the SIS #1 design:** gRPC stays the right protocol between services — fewer bytes, one
  connection, a typed contract, deadlines that propagate — but its benefit is bandwidth and
  connections, not LAN latency. That is consistent with SIS #1 serving public reads to browsers as
  JSON through the CDN.

## Environment

| | |
|---|---|
| Host | Apple M5, 10 cores, 24 GB; Docker Desktop 28.5.2 (VM: 10 CPUs, 8 GB) |
| Server | `blog_platform`, Go 1.26, Connect-Go 1.20, **2 CPUs** (`cpus: 2`, so `GOMAXPROCS` = 2) |
| Load generator | `loadgen`, Go 1.26, 4 CPUs, separate container on the same Docker network |
| Object storage | RustFS 1.0.4 (idle during the benchmarks) |
| Data | 100 users, 1 000 posts of ~10 KB markdown, 50 comments of 300 B per post; deterministic seed |

Raw results: [`results/`](results) — one JSON file per run, [`summary.md`](results/summary.md),
[`environment.txt`](results/environment.txt).

## Method

**One variable.** Both targets call the same Connect handlers on the same port with the same data and
the same Go client code; only the wire protocol differs:

| target | protocol | encoding | HTTP | connections |
|---|---|---|---|---|
| `grpc` | gRPC | protobuf | HTTP/2 cleartext | 1, multiplexed |
| `json_h1` (baseline) | Connect | JSON (`protojson`) | HTTP/1.1 keep-alive | a pool; one request per connection at a time |

**Scenarios.** Every request is authenticated (`Authorization: Bearer <JWT>`, rotating over 10 logged-in
users), as a signed-in app's requests are.

| scenario | call | why |
|---|---|---|
| `get_post` | `GetPost` | the hottest read in SIS #1; ~10 KB response, almost all text |
| `list_comments` | `ListComments`, 20 per page | structure-heavy: 20 messages of ids, timestamps, short text |
| `create_comment` | `CreateComment` with an idempotency key | the write path |

**Load.** Open loop at a fixed **2 000 requests/s**: each request starts on schedule whether or not
earlier ones finished, so a slow server cannot slow the load down and hide its own latency
(coordinated omission). 5 s warm-up (discarded), then 20 s measured = 40 000 requests per run.
**3 repetitions**; the protocol that runs first alternates each repetition; the server is restarted
before every (repetition, scenario) group so both protocols start from the same store and heap.
Tables show the **median of the 3 runs** and the range of p99.

**Measured.** Latency from the call to the decoded response, successful calls only (there were no
failed or dropped requests in any run). Bytes sent and received per request on the TCP connection,
through a counting `net.Conn`: headers, framing, HPACK and HTTP/2 control frames included. Message
bodies encoded both ways from real responses.

**Validity guards** (a run aborts if one fails): the negotiated protocol is checked on a real response
(`grpc` must answer `HTTP/2.0` + `application/grpc`, `json_h1` `HTTP/1.1` + `application/json`);
compression is off on both sides and checked — Go's HTTP/1.1 client asks for gzip on its own unless
told not to, which would have compressed only the JSON side.

## Results

### Latency and bytes per request (median of 3 runs)

| scenario | target | p50 ms | p90 ms | p99 ms | p99 range ms | p99.9 ms | sent B/req | received B/req |
|---|---|--:|--:|--:|--:|--:|--:|--:|
| get_post | grpc | 0.642 | 1.404 | 3.091 | 2.926–3.590 | 4.868 | **83** | **10 271** |
| get_post | json_h1 | **0.533** | **1.258** | **2.603** | 2.542–3.236 | **4.051** | 469 | 10 873 |
| list_comments | grpc | 0.623 | 1.368 | 2.854 | 2.627–2.967 | **5.182** | **73** | **7 384** |
| list_comments | json_h1 | **0.497** | **0.887** | **2.598** | 2.460–4.250 | 47.622 | 494 | 10 424 |
| create_comment | grpc | 0.546 | 1.072 | 1.990 | 1.894–2.024 | 4.338 | **400** | **399** |
| create_comment | json_h1 | **0.427** | **0.659** | **1.205** | 1.162–1.299 | **2.687** | 871 | 627 |

### Message bodies: protobuf vs JSON (no headers)

| message | protobuf B | JSON B | JSON / protobuf |
|---|--:|--:|--:|
| `IdRequest` (GetPost request) | 10 | 27 | 2.70× |
| `Post` (GetPost response) | 10 258 | 10 787 | 1.05× |
| `CommentListRequest` | 12 | 44 | 3.67× |
| `CommentList` (20 comments) | 7 350 | 10 325 | 1.40× |
| `CommentCreate` | 351 | 401 | 1.14× |
| `Comment` | 360 | 495 | 1.38× |

### Whole request + response on the wire

| scenario | grpc B | json_h1 B | gRPC saves |
|---|--:|--:|--:|
| get_post | 10 354 | 11 342 | 9 % |
| list_comments | 7 457 | 10 918 | 32 % |
| create_comment | 799 | 1 498 | 47 % |

## Why — mechanisms

**1. Request bytes: headers sent once vs every time.** An HTTP/1.1 request repeats every header as
text. Here that is ~440 B per request, and more than half of it is one header:
`Authorization: Bearer <JWT>` is 242 B. HTTP/2 compresses headers with HPACK: the first request on a
connection puts each header in a table shared by both ends, and every later request refers to it with
an index of a byte or two. That is why a gRPC request is 73–83 B *in total*: frame headers, a few
HPACK indexes, the 5-byte gRPC length prefix and a 10–12 B protobuf body. The more headers a request
carries — auth, tracing, request ids — the bigger this gap gets.

**2. Response bytes: what protobuf saves, and what it doesn't.** Protobuf replaces each field name with
a one-byte tag and writes numbers in binary. It cannot shrink text:

- A post is ~10 KB of markdown, so protobuf saves only **5 %** — the quotes and field names around the
  text.
- A comment page is 20 small messages: 8 field names repeated 20 times, 64-bit ids that JSON sends as
  quoted 18-digit strings (`"109547507150753792"`, 20 B) where protobuf uses an 8–9 B varint, and
  timestamps as ~30-byte RFC 3339 strings where protobuf uses ~12 B. Hence **1.40×**.
- Response headers add the rest: 13–39 B of HPACK indexes and gRPC trailers per gRPC response against
  86–132 B of text headers and chunked framing per HTTP/1.1 response.

**3. Median latency: bytes do not matter on a LAN, per-request work does.** The JSON baseline was
0.09–0.18 ms faster at the median in every pair. Three observations narrow down why:

- The gap is the **same size for a 10 KB response and a 0.4 KB one** (0.11 ms for `get_post`, 0.12 ms
  for `create_comment`). Moving 600 extra bytes over a Docker bridge costs microseconds, so it is not
  the bytes.
- Protobuf is **cheaper** to encode and decode than `protojson`, so encoding would favour gRPC, not
  JSON.
- What remains is the transport path. gRPC puts every request on **one HTTP/2 connection**. In Go's
  HTTP/2 server each stream's handler runs in its own goroutine and every frame passes through the
  connection's single serving loop (plus HPACK and flow-control bookkeeping), while an HTTP/1.1
  request is read, handled and written by one goroutine on its own connection. At sub-millisecond
  latencies those extra hand-offs per request are visible.

This last point is the inferred mechanism, not one this run isolates. Adding a JSON-over-HTTP/2
target would separate it: if JSON over HTTP/2 is as slow as gRPC, the cost is the HTTP/2 path, not
the encoding.

**4. The far tail: one connection vs a pool.** On the comment page, JSON's p99.9 reached 47 and 83 ms
in 2 of 3 runs (max 131 ms); gRPC's never exceeded 5.2 ms. HTTP/1.1 can carry one request per
connection at a time, so when an open-loop burst finds every pooled connection busy, the client
opens new TCP connections, each paying a handshake, a cold congestion window and an accept on the
server. gRPC multiplexes all requests on the connection it already has. (Inferred from the protocol
behaviour; this run did not count connections.)

## What it means for the SIS #1 design

- **Keep gRPC between the gateway and the services.** The savings are real at SIS #1 scale: the
  comment page alone runs at ~3.5 K RPS at the origin, and 3.5 K × 3.5 KB saved ≈ 12 MB/s ≈
  100 Mbit/s less traffic, plus one connection per service pair instead of a pool.
- **Do not justify gRPC by LAN latency.** On a fast network the median is decided by per-request work,
  and here Go's HTTP/1.1 path was cheaper. Where gRPC's byte savings turn into time is a slow link: on
  a 20 Mbit/s mobile connection the 0.7 KB saved per `create_comment` is ~0.3 ms of transfer.
- **Public reads stay JSON**, as SIS #1 planned: browsers need JSON, and only GETs are cached by a CDN.

## Idempotency, timeouts and retries

Implemented in the service and the client (see [README](README.md#idempotency-timeouts-retries));
proved by tests that run over **both** protocols (`go test ./...`, all passing):

| test | what it shows |
|---|---|
| `TestHandlers_IdempotencyKeyMakesRetrySafe` | the server loses every response *after* committing; a create with an idempotency key is retried 3 times and stores **exactly one** row; a create without a key is never retried |
| `TestHandlers_PerAttemptTimeoutStopsWaiting` | the server stalls 500 ms; a 50 ms per-attempt deadline (sent to the server as `grpc-timeout` / `Connect-Timeout-Ms`) ends each attempt, the read is retried once, and the call returns in well under 300 ms |
| `TestPostService_CreatePost_IdempotencyKeyReplays`, `…KeyReusedWithDifferentBody`, `…KeyIsScopedToTheUser` | a replay returns the first post; the same key with another payload is rejected; keys are per user |
| `TestPostService_LikePost_IsIdempotent` | liking twice leaves one like |
| `TestHandlers_AccessToken`, `TestAccessToken_Rejected`, `TestAuthService_RefreshTokenRotates` | bad, expired, foreign-key and `alg: none` tokens are rejected; a reused refresh token revokes the login |

## Limitations

- **One machine, a Docker bridge network.** There is no real latency, bandwidth limit or packet loss,
  so this measures per-request overhead, not wide-area behaviour (week 2 measured that).
- **The baseline uses `protojson`** (Connect's JSON codec), which is slower than a hand-written
  `encoding/json` server. That biases against JSON — and JSON still had the lower median, so the
  latency conclusion holds in that direction.
- **One rate (2 000 RPS) and 2 server CPUs.** At a rate closer to saturation the encoding cost would
  matter more and the ranking could change.
- **In-memory storage** keeps database time out of the comparison on purpose; with PostgreSQL (SIS #3)
  the protocol differences become a smaller share of the total.

## Reproduce

```bash
./run-all.sh   # ~10 min: rebuilds, runs everything, writes results/ and results/summary.md
```
