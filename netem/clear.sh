#!/usr/bin/env bash
# Remove netem from the frontend interface. Always clean up before the next run;
# a leftover qdisc silently poisons every measurement after it.
set -euo pipefail

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
tc qdisc del dev "${DEV}" root 2>/dev/null || true
echo "netem cleared on ${DEV}"
tc qdisc show dev "${DEV}"
