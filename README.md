# Backend for Highloaded Environment — KBTU, Fall 2026

Weekly practice work for the **Backend for Highloaded Environment** course at KBTU.
Each week is a self-contained project on its own branch; `main` is the index.

| | |
|---|---|
| **Student** | Yerakairzhan — Telegram [@erakairzhan](https://t.me/erakairzhan) |
| **Tutor** | Nurov Arnur — Telegram [@zamimaru](https://t.me/zamimaru) |
| **Language** | Go (stdlib `net/http` first; external deps only where the protocol demands it) |

## Stack

| Layer | Tools |
|---|---|
| Services | **Go 1.26** (`net/http`, `encoding/json`), TLS 1.3, HTTP/1.1 + HTTP/2 via ALPN |
| Edge / HTTP/3 | Caddy 2.8 (h3 · h2 · http/1.1 termination, reverse proxy) |
| Load generation | k6 (constant-arrival-rate), custom Go clients (quic-go for HTTP/3), `curl` |
| Observability | Prometheus, Grafana (provisioned dashboards) |
| Network impairment | Linux `tc netem` (delay, jitter, loss) inside Docker |
| Runtime | Docker Compose — every week runs the same way: `docker compose up -d --build` |

## Weeks

| Week | Branch | Topic | Key deliverables |
|---|---|---|---|
| 1 | [`week-1`](../../tree/week-1) | **Percentiles & capacity.** p50/p95/p99 vs mean, Little's law, load generator, system-wide Prometheus histogram | Go service with 3 latency profiles · load tester · Prometheus + Grafana dashboard |
| 2 | [`week-2`](../../tree/week-2) | **HTTP/1.1 vs HTTP/2 vs HTTP/3.** Cost of the TLS handshake, connection pooling, multiplexing, head-of-line blocking under loss | Go service + Caddy h3 edge · k6 + quic-go load rig · netem scripts · `REPORT.md` with measured percentiles |

Each branch has its own `README.md` (how to run) and, where the task requires it, a `REPORT.md`
(measurements, environment, and mechanism-level explanations).

## How to use this repository

```bash
git clone https://github.com/yerakairzhan/highload-backend-kbtu-fall26.git
cd highload-backend-kbtu-fall26

git branch -r                 # list weeks
git checkout week-2           # pick one
cat README.md                 # follow that week's instructions
```

Prerequisites for every week: **Go 1.26+** and **Docker Desktop** (Compose v2). Everything else
(k6, Caddy, Prometheus, Grafana, netem) runs in containers — nothing to install on the host.

## Conventions

- **One branch per week**, branched from `main`; `main` never holds code, only this index.
- **Measurements over opinions.** Percentiles, never averages; environment and tool versions stated;
  the negotiated protocol proved, not assumed; invalid runs discarded and documented.
- **Reproducible by command line.** Each week has a single entry point (`go run .`,
  `docker compose up`, or `run-all.sh`).
- **AI usage is disclosed** in each week's README.
