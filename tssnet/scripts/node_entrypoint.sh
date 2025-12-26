#!/bin/sh
set -e

IFACE=${TC_IFACE:-eth0}
LAT=${LATENCY_MS:-0}
JIT=${JITTER_MS:-0}
LOSS=${LOSS_PCT:-0}

if [ "$LAT" != "0" ] || [ "$LOSS" != "0" ]; then
  echo "[node_entrypoint] applying tc netem: iface=$IFACE latency=${LAT}ms jitter=${JIT}ms loss=${LOSS}%"
  tc qdisc del dev "$IFACE" root 2>/dev/null || true

  NETEM_ARGS=""
  if [ "$LAT" != "0" ]; then
    if [ "$JIT" != "0" ]; then
      NETEM_ARGS="$NETEM_ARGS delay ${LAT}ms ${JIT}ms"
    else
      NETEM_ARGS="$NETEM_ARGS delay ${LAT}ms"
    fi
  fi
  if [ "$LOSS" != "0" ]; then
    NETEM_ARGS="$NETEM_ARGS loss ${LOSS}%"
  fi

  if [ -n "$NETEM_ARGS" ]; then
    tc qdisc add dev "$IFACE" root netem $NETEM_ARGS
  fi
fi

exec /usr/local/bin/app "$@"
