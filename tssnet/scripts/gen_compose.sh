#!/usr/bin/env bash
set -euo pipefail

N=${1:-3}
T=${2:-1}
SESSION=${SESSION:-cluster}
OUT=${OUT:-tssnet/docker-compose.tssnet.yml}

if [ "$T" -lt 1 ] || [ "$T" -ge "$N" ]; then
  echo "Threshold T must satisfy 1 <= T < N" >&2
  exit 1
fi

PARTIES=$(seq -s, -f 'P%g' 1 $N)

cat > "$OUT" <<YAML
services:
  tss-coordinator:
    build:
      context: ..
      dockerfile: tssnet/docker/Dockerfile
      args:
        CMD: tss-coordinator
    ports:
      - "9000:9000"

  tss-gateway:
    build:
      context: ..
      dockerfile: tssnet/docker/Dockerfile
      args:
        CMD: tss-gateway
    depends_on:
      - tss-coordinator
    ports:
      - "9100:9100"
    command:
      - "-coordinator=ws://tss-coordinator:9000/ws"
      - "-session=$SESSION"
      - "-party=G"
      - "-parties=$PARTIES"
      - "-threshold=$T"
      - "-listen=:9100"
YAML

for i in $(seq 1 $N); do
  PARTY="P$i"          # party ID (giữ uppercase)
  SVC="p$i"            # service name (lowercase để Docker hợp lệ)

  cat >> "$OUT" <<YAML

  tss-node-$SVC:
    build:
      context: ..
      dockerfile: tssnet/docker/Dockerfile
      args:
        CMD: tss-node
    depends_on:
      - tss-coordinator
    cap_add:
      - NET_ADMIN
    entrypoint:
      - /usr/local/bin/node_entrypoint.sh
    volumes:
      - ./data/$PARTY:/data
    environment:
      # set any of these to enable network emulation inside the container
      # LATENCY_MS: "80"
      # JITTER_MS: "20"
      # LOSS_PCT: "0.5"
      # TC_IFACE: "eth0"
      - LATENCY_MS=0
      - JITTER_MS=0
      - LOSS_PCT=0
    command:
      - "-coordinator=ws://tss-coordinator:9000/ws"
      - "-session=$SESSION"
      - "-party=$PARTY"
      - "-data=/data"
      - "-gateway=G"
YAML
done

echo "wrote $OUT (N=$N, T=$T, parties=$PARTIES, session=$SESSION)"
