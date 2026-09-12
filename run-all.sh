#!/usr/bin/env bash
# Orchestrate the Week 2 runs and write results/ for REPORT.md.
#
#   1. three protocol runs on a clean link (h1.1, h2, h3)          — k6 + h3client
#   2. the handshake cost (h2 reused vs new-per-request)            — k6
#   3. the same on a bad network (netem 100 ms / 1 %, both ways)    — k6 + h3client
#   3b. controlled connection-model comparison under loss:
#       h2 vs h3 with ONE connection each, and h3 with 50 connections
#   4. (explanations are written by hand in REPORT.md)
#
# Every run uses the SAME endpoint, arrival rate (RATE) and duration
# (DURATION); all runs are open-loop. One variable changes at a time.
#
# Validity guards — a rig that silently measures the wrong thing is worse than
# no rig:
#   * The host must not sleep: the Docker VM pauses, k6 (which schedules on
#     realtime) then fires every missed iteration as a burst, and the tail is
#     garbage. We re-exec under `caffeinate` on macOS, AND we stamp each run
#     with HOST wall-clock time; a run whose host time exceeds the configured
#     duration by more than SLACK seconds (or whose k6 clock jumped) is
#     INVALID and is retried up to MAX_TRIES times.
#   * The impaired stage aborts unless netem applied on every frontend
#     interface and the measured RTT is ~2x the one-way delay.
set -uo pipefail
cd "$(dirname "$0")"

if [ -z "${CAFFEINATED:-}" ] && command -v caffeinate >/dev/null 2>&1; then
  echo "(re-exec under caffeinate so the host cannot sleep during the run)"
  # Go through bash: caffeinate execvp()s its target, and a bare "run-all.sh"
  # is looked up in PATH, not the cwd.
  CAFFEINATED=1 exec caffeinate -dims bash "$0" "$@"
fi

RATE="${RATE:-300}"            # requests/sec offered load, all protocols
DURATION="${DURATION:-60s}"    # steady duration per run, all protocols
DELAY="${DELAY:-100ms}"; JITTER="${JITTER:-10ms}"; LOSS="${LOSS:-1%}"
COLD_PREVUS="${COLD_PREVUS:-400}"; COLD_MAXVUS="${COLD_MAXVUS:-1000}" # cold run needs more VUs
SLACK="${SLACK:-20}"           # seconds of host time a run may exceed DURATION by
MAX_TRIES="${MAX_TRIES:-3}"

CFG_S=$(python3 -c "import re;print(int(sum(float(n)*{'ms':.001,'s':1,'m':60,'h':3600}[u] for n,u in re.findall(r'(\d+(?:\.\d+)?)(ms|s|m|h)','$DURATION'))))")

dc() { docker compose "$@"; }
now() { python3 -c 'import time; print(f"{time.time():.3f}")'; } # BSD date has no %N

# valid <name> <t0> <t1>: host wall within slack, and (k6 only) no clock jump.
valid() {
  local name="$1" wall; wall=$(echo "$3 - $2" | bc)
  local jump="False"
  jump=$(python3 -c "import json;print(json.load(open('results/${name}.summary.json')).get('k6_raw',{}).get('clock_jump_suspected',False))" 2>/dev/null || echo "n/a")
  if [ "$(echo "${wall} > ${CFG_S} + ${SLACK}" | bc)" = "1" ] || [ "${jump}" = "True" ]; then
    printf 'INVALID %-16s host_wall_s=%-8.2f k6_clock_jump=%s  (VM paused? host slept?)\n' "$name" "$wall" "$jump" | tee -a results/wallclock.txt
    return 1
  fi
  printf 'VALID   %-16s host_wall_s=%-8.2f k6_clock_jump=%s\n' "$name" "$wall" "$jump" | tee -a results/wallclock.txt
}

# retrying <name> <cmd...>: run up to MAX_TRIES until valid.
retrying() {
  local name="$1"; shift
  local try t0
  for try in $(seq 1 "${MAX_TRIES}"); do
    echo ">>> ${name} (attempt ${try}/${MAX_TRIES})"
    t0=$(now); "$@"
    valid "${name}" "${t0}" "$(now)" && return 0
    echo "    retrying ${name}..."
  done
  echo "FAILED  ${name}: no valid attempt in ${MAX_TRIES} tries" | tee -a results/wallclock.txt
  return 1
}

k6cmd() { # name url [extra -e ...]
  local name="$1" url="$2"; shift 2
  dc exec -T k6 k6 run -e URL="${url}" -e NAME="${name}" -e RATE="${RATE}" -e DURATION="${DURATION}" "$@" /load/load.js
}
gocmd() { # name proto conns url   (h3client, open-loop, same RATE/DURATION)
  local name="$1" proto="$2" conns="$3" url="$4"
  dc exec -T client h3client -mode load -proto "${proto}" -conns "${conns}" -url "${url}" \
    -rate "${RATE}" -duration "${DURATION}" -name "${name}" -out "/results/${name}.summary.json"
}
k6run() { retrying "$1" k6cmd "$@"; }
gorun() { retrying "$1" gocmd "$@"; }

impair() { # container -> abort the whole run if netem did not apply
  DELAY="$DELAY" JITTER="$JITTER" LOSS="$LOSS" dc exec -T "$1" bash /netem/impair.sh \
    || { echo "FATAL: netem failed to apply on $1 — refusing to produce half-impaired numbers"; clear_all; exit 1; }
}
clear_all() { for c in edge k6 client; do dc exec -T "$c" bash /netem/clear.sh; done; }

H1="https://edge:9444"; H="https://edge:9443"; Q="/api/quote/1"

echo "=== 0. build + up ==="
[ -f certs/cert.pem ] || bash certs/gen-cert.sh
dc up -d --build
echo "waiting for edge to answer..."
for i in $(seq 1 30); do
  if dc exec -T client curl -k -s -o /dev/null "${H}/healthz"; then echo "edge up"; break; fi
  sleep 1
done
: > results/wallclock.txt
echo "# rate=${RATE} duration=${DURATION} slack=${SLACK}s  started $(date -u +%FT%TZ)" >> results/wallclock.txt
clear_all >/dev/null 2>&1 || true   # never start on a poisoned link

echo "=== 1. prove versions ==="
{
  echo "# Version proof ($(date -u +%FT%TZ))"
  dc exec -T client h3client -mode prove -proto h1 -url "${H1}${Q}"
  dc exec -T client h3client -mode prove -proto h2 -url "${H}${Q}"
  dc exec -T client h3client -mode prove -proto h3 -url "${H}${Q}"
} | tee results/prove-versions.txt

echo "=== 2. clean-link protocol runs ==="
k6run h1_clean "${H1}"                     # k6, http/1.1-only listener
k6run h2_clean "${H}"                      # k6, negotiates h2 (one connection PER VU)
gorun h3_clean h3 1 "${H}${Q}"             # h3client, ONE QUIC connection
gorun h2_clean_1conn h2 1 "${H}${Q}"       # h3client, ONE TCP connection (same tool as h3)

echo "=== 3. handshake experiment (reused vs new-per-request) ==="
k6run h1_cold "${H1}" -e COLD=1 -e PREVUS="${COLD_PREVUS}" -e MAXVUS="${COLD_MAXVUS}"   # template row
k6run h2_cold "${H}"  -e COLD=1 -e PREVUS="${COLD_PREVUS}" -e MAXVUS="${COLD_MAXVUS}"

echo "=== 4. impaired-link runs (netem ${DELAY}/${JITTER}/${LOSS}, both directions) ==="
impair edge; impair k6; impair client
echo "RTT check (client->edge; expect ~2x ${DELAY}):"
rtt_avg=$(dc exec -T k6 ping -c 5 -q edge | sed -n 's#.*= [0-9.]*/\([0-9.]*\)/.*#\1#p')
echo "  avg RTT = ${rtt_avg} ms" | tee -a results/wallclock.txt
if [ -z "${rtt_avg}" ] || [ "$(echo "${rtt_avg} < 150" | bc)" = "1" ]; then
  echo "FATAL: RTT ${rtt_avg} ms is not ~2x delay — netem is not on both directions"; clear_all; exit 1
fi
echo "backend hop must stay clean (edge->service):"
dc exec -T edge ping -c 3 -q service | tail -1 | tee -a results/wallclock.txt

k6run h1_netem "${H1}"
k6run h2_netem "${H}"
gorun h3_netem h3 1 "${H}${Q}"             # ONE QUIC connection under loss
echo "=== 4b. controlled connection model under loss (same tool, same rate) ==="
gorun h2_netem_1conn h2 1 "${H}${Q}"       # ONE TCP connection under loss  <- Claim 3, fair fight
gorun h3_netem_c50 h3 50 "${H}${Q}"        # 50 QUIC connections: is the ceiling per-connection?

echo "clearing netem"
clear_all

echo "=== done. validity + results ==="
cat results/wallclock.txt
ls -1 results/
