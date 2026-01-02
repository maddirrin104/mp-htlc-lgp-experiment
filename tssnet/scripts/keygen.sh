#!/usr/bin/env bash
set -euo pipefail

GATEWAY=${GATEWAY_URL:-http://localhost:9100}

curl -sS -X POST "$GATEWAY/keygen" | tee /dev/stderr | jq -r '.address // .AddrHex // empty' || true
