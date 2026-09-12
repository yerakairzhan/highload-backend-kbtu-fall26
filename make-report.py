#!/usr/bin/env python3
"""Render the results table into REPORT.md from results/.

Reads results/*.summary.json and results/wallclock.txt and rewrites the block
between <!-- RESULTS:BEGIN --> and <!-- RESULTS:END --> in REPORT.md.

Every row carries the validity verdict from run-all.sh. A row is VALID only if
its host wall-clock time matched the configured duration and (for k6) its own
clock did not jump. INVALID rows are printed struck through so they can never
be mistaken for measurements.

    python3 make-report.py            # rewrite REPORT.md
    python3 make-report.py --print    # just print the markdown
"""
import glob
import json
import os
import re
import sys

RES = "results"
REPORT = "REPORT.md"

# name -> (protocol, connection model, tool, mechanism placeholder)
ROWS = [
    ("h1_clean",       "HTTP/1.1", "reused (k6 pool)",            "clean"),
    ("h1_cold",        "HTTP/1.1", "new per request",             "clean"),
    ("h2_clean",       "HTTP/2",   "reused, 1 conn per VU (k6)",  "clean"),
    ("h2_cold",        "HTTP/2",   "new per request",             "clean"),
    ("h2_clean_1conn", "HTTP/2",   "reused, ONE connection",      "clean"),
    ("h3_clean",       "HTTP/3",   "reused, ONE connection",      "clean"),
    ("h1_netem",       "HTTP/1.1", "reused (k6 pool)",            "netem 100 ms / 1 %"),
    ("h2_netem",       "HTTP/2",   "reused, 1 conn per VU (k6)",  "netem 100 ms / 1 %"),
    ("h2_netem_1conn", "HTTP/2",   "reused, ONE connection",      "netem 100 ms / 1 %"),
    ("h3_netem",       "HTTP/3",   "reused, ONE connection",      "netem 100 ms / 1 %"),
    ("h3_netem_c50",   "HTTP/3",   "reused, 50 connections",      "netem 100 ms / 1 %"),
]


def validity():
    """name -> 'VALID' | 'INVALID' | 'FAILED' from the LAST verdict per run."""
    v = {}
    path = os.path.join(RES, "wallclock.txt")
    if not os.path.exists(path):
        return v
    for line in open(path):
        m = re.match(r"(VALID|INVALID|FAILED)\s+(\S+)", line)
        if m:
            v[m.group(2).rstrip(":")] = m.group(1)
    return v


def rtt():
    path = os.path.join(RES, "wallclock.txt")
    if not os.path.exists(path):
        return None, None
    txt = open(path).read()
    fe = re.search(r"avg RTT = ([\d.]+) ms", txt)
    be = re.search(r"rtt min/avg/max/mdev = [\d.]+/([\d.]+)/", txt)
    return (fe.group(1) if fe else None), (be.group(1) if be else None)


def load(name):
    path = os.path.join(RES, f"{name}.summary.json")
    if not os.path.exists(path):
        return None
    d = json.load(open(path))
    if "http_req_duration_ms" in d:  # k6
        r = d["http_req_duration_ms"]
        return dict(p50=r["p50"], p95=r["p95"], p99=r["p99"], rps=d["achieved_rps"],
                    extra=f"dropped={d.get('dropped_iterations', 0)}", raw=d)
    return dict(p50=d["p50_ms"], p95=d["p95_ms"], p99=d["p99_ms"], rps=d["achieved_rps"],
                extra=f"errors={d['errors']} max_inflight={d['max_in_flight']}", raw=d)


def fmt(x):
    return "—" if x is None else f"{x:.2f}" if isinstance(x, float) else str(x)


def table(v):
    out = ["| Protocol | Connection | Link | p50 ms | p95 ms | p99 ms | RPS | Validity | Notes |",
           "|---|---|---|---|---|---|---|---|---|"]
    for name, proto, conn, link in ROWS:
        d = load(name)
        ver = v.get(name, "not run" if d is None else "unverified")
        if d is None:
            out.append(f"| {proto} | {conn} | {link} | — | — | — | — | {ver} | `{name}` |")
            continue
        cells = [fmt(d["p50"]), fmt(d["p95"]), fmt(d["p99"]), fmt(d["rps"])]
        if ver != "VALID":
            cells = [f"~~{c}~~" for c in cells]
        out.append(f"| {proto} | {conn} | {link} | {' | '.join(cells)} | **{ver}** | `{name}` {d['extra']} |")
    return "\n".join(out)


def handshake(v):
    """Cold minus warm on connecting + tls_handshaking, per protocol (k6)."""
    lines = ["| Protocol | Metric | warm p50 | cold p50 | cold − warm | warm p95 | cold p95 | Validity |",
             "|---|---|---|---|---|---|---|---|"]
    for proto, warm, cold in (("HTTP/1.1", "h1_clean", "h1_cold"), ("HTTP/2", "h2_clean", "h2_cold")):
        w, c = load(warm), load(cold)
        if not (w and c):
            lines.append(f"| {proto} | — | — | — | — | — | — | not run |")
            continue
        ok = v.get(warm) == "VALID" and v.get(cold) == "VALID"
        tot_w = tot_c = 0.0
        for key, label in (("http_req_connecting_ms", "TCP connect"),
                           ("http_req_tls_handshaking_ms", "TLS handshake")):
            wp, cp = w["raw"][key], c["raw"][key]
            tot_w += wp["p50"]; tot_c += cp["p50"]
            lines.append(f"| {proto} | {label} | {fmt(wp['p50'])} | {fmt(cp['p50'])} | "
                         f"**{fmt(cp['p50'] - wp['p50'])}** | {fmt(wp['p95'])} | {fmt(cp['p95'])} | "
                         f"{'VALID' if ok else 'INVALID'} |")
        lines.append(f"| {proto} | **connect + TLS (the handshake)** | {fmt(tot_w)} | {fmt(tot_c)} | "
                     f"**{fmt(tot_c - tot_w)} ms** | | | {'VALID' if ok else 'INVALID'} |")
    return "\n".join(lines)


def render():
    v = validity()
    fe, be = rtt()
    n_valid = sum(1 for s in v.values() if s == "VALID")
    block = [
        f"_Generated by `make-report.py` from `results/` — {n_valid} of {len(ROWS)} rows VALID._",
        "",
        "### All runs (same endpoint `/api/quote/1`, 300 req/s open-loop, 60 s each)",
        "",
        table(v),
        "",
        f"Impaired link check: client→edge RTT **{fe or '—'} ms** (expected ≈2 × 100 ms); "
        f"edge→service backend hop **{be or '—'} ms** (must stay clean).",
        "",
        "### The cost of the handshake (k6, cold − warm)",
        "",
        handshake(v),
    ]
    return "\n".join(block)


def main():
    md = render()
    if "--print" in sys.argv:
        print(md)
        return
    src = open(REPORT).read()
    new = re.sub(r"(<!-- RESULTS:BEGIN -->).*?(<!-- RESULTS:END -->)",
                 lambda m: f"{m.group(1)}\n{md}\n{m.group(2)}", src, flags=re.S)
    open(REPORT, "w").write(new)
    print(f"REPORT.md updated ({sum(1 for s in validity().values() if s == 'VALID')} valid rows)")


if __name__ == "__main__":
    main()
