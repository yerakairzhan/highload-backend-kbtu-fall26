# Highload Backend — KBTU, Fall 2026

Course repository for **Highload Backend** at KBTU.

Each week's work lives on its own branch. `main` stays as an index; the code for a
given week is on that week's branch.

## Weeks

| Week | Branch | Topic |
|------|--------|-------|
| 1 | [`week-1`](../../tree/week-1) | Percentiles (p50/p95/p99), capacity estimation, load testing, Prometheus + Grafana |
| 2 | [`week-2`](../../tree/week-2) | HTTP/1.1 vs HTTP/2 vs HTTP/3 load testing: TLS handshake cost, connection pooling, netem impairment (Go + Caddy + k6 + quic-go) |

## How to use

```bash
# list branches
git branch -a

# check out a week's code
git checkout week-1
```

Each week's branch contains its own README with setup and run instructions.
