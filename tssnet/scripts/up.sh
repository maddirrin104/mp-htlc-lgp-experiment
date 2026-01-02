#!/usr/bin/env bash
set -euo pipefail

N=${1:-3}
T=${2:-1}

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT_DIR=$(cd "$SCRIPT_DIR/../.." && pwd)
cd "$ROOT_DIR"

./tssnet/scripts/gen_compose.sh "$N" "$T"

docker compose -f tssnet/docker-compose.tssnet.yml up -d --build

echo "--- services ---"
docker compose -f tssnet/docker-compose.tssnet.yml ps
