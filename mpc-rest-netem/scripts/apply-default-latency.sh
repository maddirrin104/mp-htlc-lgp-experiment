#!/usr/bin/env bash
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
"$DIR/netem-control.sh" clear mpc-node1 eth0 || true
"$DIR/netem-control.sh" clear mpc-node2 eth0 || true
"$DIR/netem-control.sh" clear mpc-node3 eth0 || true
"$DIR/netem-control.sh" set-delay mpc-node1 eth0 0
"$DIR/netem-control.sh" set-delay mpc-node2 eth0 150
"$DIR/netem-control.sh" set-delay mpc-node3 eth0 250
