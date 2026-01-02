#!/usr/bin/env bash
set -euo pipefail

GATEWAY=${GATEWAY_URL:-http://localhost:9100}
HASH=${1:-}
N=${2:-10}

if [ -z "$HASH" ]; then
  echo "Usage: $0 <hash_hex_32_bytes> [count]" >&2
  exit 1
fi

for i in $(seq 1 $N); do
  resp=$(curl -sS -X POST "$GATEWAY/signHash" -H 'Content-Type: application/json' -d "{\"hash_hex\":\"$HASH\"}")
  t=$(echo "$resp" | jq -r '.t_sign_ms // empty')
  r=$(echo "$resp" | jq -r '.r // empty')
  s=$(echo "$resp" | jq -r '.s // empty')
  echo "#${i} t_sign_ms=${t} r=${r} s=${s}"
done
