#!/usr/bin/env bash
set -euo pipefail
if [[ $# -lt 2 ]]; then
  echo "Usage: $0 <set-delay|drop|flaky|clear> <container> [iface] [args...]"
  exit 1
fi
ACTION="$1"
CONTAINER="$2"
IFACE="${3:-eth0}"
run_tc() { docker exec "$CONTAINER" sh -lc "$1"; }
case "$ACTION" in
  set-delay) DELAY_MS="${4:?missing delay ms}"; run_tc "tc qdisc replace dev $IFACE root netem delay ${DELAY_MS}ms" ;;
  drop) run_tc "tc qdisc replace dev $IFACE root netem loss 100%" ;;
  flaky) LOSS_PCT="${4:?missing loss pct}"; DELAY_MS="${5:?missing delay ms}"; run_tc "tc qdisc replace dev $IFACE root netem delay ${DELAY_MS}ms loss ${LOSS_PCT}%" ;;
  clear) run_tc "tc qdisc del dev $IFACE root || true" ;;
  *) echo "Unknown action: $ACTION"; exit 1 ;;
esac
