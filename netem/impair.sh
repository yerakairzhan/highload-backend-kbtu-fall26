#!/usr/bin/env bash
# Apply netem to the FRONTEND interface (client<->edge link).
#
# Run it inside the edge container AND inside the active load container so both
# directions of the client<->edge path are impaired (delay each way -> full RTT,
# loss each way). The edge<->service hop is on a different interface and stays
# clean on purpose.
#
# Env: DELAY (default 100ms), JITTER (default 10ms), LOSS (default 1%).
set -euo pipefail

DELAY="${DELAY:-100ms}"
JITTER="${JITTER:-10ms}"
LOSS="${LOSS:-1%}"

# Pick the frontend interface. On the edge, 'service' resolves over the backend
# network; the interface toward it is the backend iface, so the OTHER one is
# frontend. On a client container 'service' does not resolve, so we fall back
# to the default-route interface (its single eth).
pick_dev() {
  local backend_ip backend_if dev
  backend_ip="$(getent hosts service 2>/dev/null | awk '{print $1; exit}' || true)"
  if [ -n "${backend_ip}" ]; then
    backend_if="$(ip -o route get "${backend_ip}" 2>/dev/null | sed -n 's/.* dev \([^ ]*\).*/\1/p')"
    # veth names print as "eth0@if328"; tc wants the bare name, so strip "@...".
    dev="$(ip -o link show | awk -F': ' '{print $2}' | cut -d@ -f1 | grep -E '^eth' | grep -v "^${backend_if}$" | head -1)"
  fi
  [ -z "${dev:-}" ] && dev="$(ip route | awk '/default/{print $5; exit}')"
  [ -z "${dev:-}" ] && dev="eth0"
  echo "${dev}"
}

DEV="${DEV:-$(pick_dev)}"

# Clear any existing qdisc first (idempotent), then apply.
tc qdisc del dev "${DEV}" root 2>/dev/null || true
tc qdisc add dev "${DEV}" root netem delay "${DELAY}" "${JITTER}" loss "${LOSS}"

echo "netem applied on ${DEV}: delay ${DELAY} ${JITTER}, loss ${LOSS}"
tc qdisc show dev "${DEV}"
