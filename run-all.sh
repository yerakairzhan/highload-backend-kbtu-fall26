#!/usr/bin/env bash
# Runs every SIS #2 measurement in Docker: results/*.json + results/summary.md.
# ~10 minutes at the defaults; keep the machine awake.
#
#   REPS=3 RATE=2000 DURATION=20s WARMUP=5s ./run-all.sh
set -euo pipefail
cd "$(dirname "$0")"

REPS=${REPS:-3}
RATE=${RATE:-2000}
DURATION=${DURATION:-20s}
WARMUP=${WARMUP:-5s}
SCENARIOS=${SCENARIOS:-"get_post list_comments create_comment"}
TARGETS=(grpc json_h1)

lg() { docker compose exec -T loadgen ./loadgen "$@"; }

wait_ready() {
  until docker compose exec -T loadgen wget -q -O /dev/null http://blog-platform:8080/healthz 2>/dev/null; do sleep 1; done
}

mkdir -p results
rm -f results/*.json results/summary.md
docker compose up -d --build
wait_ready

{
  echo "date: $(date -u +%FT%TZ)"
  echo "host: $(sysctl -n machdep.cpu.brand_string 2>/dev/null || uname -m), $(sysctl -n hw.ncpu 2>/dev/null) cores"
  echo "docker: $(docker version --format '{{.Server.Version}}'), $(docker info --format '{{.NCPU}} CPUs, {{.MemTotal}} B')"
  echo "server: 2 CPUs | loadgen: 4 CPUs | ${RATE} RPS open loop | ${DURATION} measured after ${WARMUP} warm-up | ${REPS} reps"
} > results/environment.txt

lg sizes -out results/sizes.json

for rep in $(seq 1 "$REPS"); do
  for scenario in $SCENARIOS; do
    # A fresh server per (rep, scenario): both protocols start from the same store.
    docker compose restart blog-platform >/dev/null
    wait_ready
    for k in 0 1; do
      t=${TARGETS[$(( (k + rep) % 2 ))]} # alternate which protocol runs first
      lg bench -target "$t" -scenario "$scenario" -rate "$RATE" -duration "$DURATION" \
        -warmup "$WARMUP" -rep "$rep" -out "results/bench-$scenario-$t-r$rep.json"
    done
  done
done

lg report -dir results > results/summary.md
echo "done: results/summary.md"
