#!/usr/bin/env bash
set -euo pipefail

DELAY="${NETEM_DELAY:-0ms}"
LOSS="${NETEM_LOSS:-0%}"
NODE="${NODE_NAME:-tss-node}"

echo "[${NODE}] applying netem: delay=${DELAY} loss=${LOSS}"

# Apply netem to container interface
tc qdisc add dev eth0 root netem delay "${DELAY}" loss "${LOSS}" || true

echo "[${NODE}] ready (stub). Replace this with a real tss-lib node."
sleep infinity
