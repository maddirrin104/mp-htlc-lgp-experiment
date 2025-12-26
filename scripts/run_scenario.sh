#!/usr/bin/env bash
set -euo pipefail

# Usage: ./scripts/run_scenario.sh S1|S2|S3|S4
SCENARIO=${1:-S1}

if [ ! -f .env ]; then
  echo "Missing .env" >&2
  exit 1
fi
# shellcheck disable=SC1091
source .env

: "${SEPOLIA_RPC_URL:?Missing SEPOLIA_RPC_URL}"
: "${CHAIN_ID:?Missing CHAIN_ID}"
: "${DEPLOYER_PK:?Missing DEPLOYER_PK}"
: "${RECEIVER_PK:?Missing RECEIVER_PK}"
: "${SIGNER_PK:?Missing SIGNER_PK}"

: "${AMOUNT_TOKEN:?Missing AMOUNT_TOKEN}"
: "${TIMELOCK_SEC:?Missing TIMELOCK_SEC}"
: "${PENALTY_WINDOW_SEC:?Missing PENALTY_WINDOW_SEC}"
: "${DEPOSIT_REQUIRED_WEI:?Missing DEPOSIT_REQUIRED_WEI}"
: "${DEPOSIT_WINDOW_SEC:?Missing DEPOSIT_WINDOW_SEC}"

TOKEN=$(jq -r .token configs/deployed.json)
HTLC=$(jq -r .htlc configs/deployed.json)

DEPLOYER_ADDR=$(cast wallet address --private-key "$DEPLOYER_PK")
RECEIVER_ADDR=$(cast wallet address --private-key "$RECEIVER_PK")
SIGNER_ADDR=$(cast wallet address --private-key "$SIGNER_PK")

now_ts=$(date +%s)
timelock=$((now_ts + TIMELOCK_SEC))

# Random preimage/hashlock
preimage="0x$(openssl rand -hex 32)"
hashlock=$(cast keccak "$preimage")

# Use a random lockId (bytes32)
lockId="0x$(openssl rand -hex 32)"

echo "Scenario  : $SCENARIO"
echo "TOKEN     : $TOKEN"
echo "HTLC      : $HTLC"
echo "lockId    : $lockId"
echo "preimage  : $preimage"
echo "hashlock  : $hashlock"
echo "timelock  : $timelock (now=$now_ts)"
echo "deployer  : $DEPLOYER_ADDR"
echo "receiver  : $RECEIVER_ADDR"
echo "signer    : $SIGNER_ADDR"
echo

csv=${OUT_LOG:-"./logs/run.csv"}
mkdir -p "$(dirname "$csv")"
if [ ! -f "$csv" ]; then
  echo "scenario,step,txHash,gasUsed,effectiveGasPrice,feeWei" > "$csv"
fi

append_receipt () {
  local scenario="$1"
  local step="$2"
  local tx="$3"
  local gasUsed
  local egp
  local fee
  gasUsed=$(cast receipt "$tx" --rpc-url "$SEPOLIA_RPC_URL" --json | jq -r .gasUsed)
  egp=$(cast receipt "$tx" --rpc-url "$SEPOLIA_RPC_URL" --json | jq -r .effectiveGasPrice)
  fee=$(python - <<PY
gas=int("$gasUsed")
egp=int("$egp")
print(gas*egp)
PY
)
  echo "${scenario},${step},${tx},${gasUsed},${egp},${fee}" >> "$csv"
}

echo "[1/6] Mint MTK to deployer"
tx_mint=$(cast send "$TOKEN" "mint(address,uint256)" "$DEPLOYER_ADDR" "$AMOUNT_TOKEN" \
  --rpc-url "$SEPOLIA_RPC_URL" --private-key "$DEPLOYER_PK" --json | jq -r .transactionHash)
append_receipt "$SCENARIO" "mint" "$tx_mint"

echo "[2/6] Approve HTLC to spend MTK"
tx_app=$(cast send "$TOKEN" "approve(address,uint256)" "$HTLC" "$AMOUNT_TOKEN" \
  --rpc-url "$SEPOLIA_RPC_URL" --private-key "$DEPLOYER_PK" --json | jq -r .transactionHash)
append_receipt "$SCENARIO" "approve" "$tx_app"

echo "[3/6] Lock"
tx_lock=$(cast send "$HTLC" \
  "lock(bytes32,address,address,address,uint256,bytes32,uint256,uint256,uint256,uint256)" \
  "$lockId" "$TOKEN" "$RECEIVER_ADDR" "$SIGNER_ADDR" "$AMOUNT_TOKEN" "$hashlock" "$timelock" "$PENALTY_WINDOW_SEC" \
  "$DEPOSIT_REQUIRED_WEI" "$DEPOSIT_WINDOW_SEC" \
  --rpc-url "$SEPOLIA_RPC_URL" --private-key "$DEPLOYER_PK" --json | jq -r .transactionHash)
append_receipt "$SCENARIO" "lock" "$tx_lock"

case "$SCENARIO" in
  S4)
    echo "[4/6] Skip deposit (S4)"
    ;;
  *)
    echo "[4/6] confirmParticipation (deposit)"
    tx_dep=$(cast send "$HTLC" "confirmParticipation(bytes32)" "$lockId" \
      --value "$DEPOSIT_REQUIRED_WEI" \
      --rpc-url "$SEPOLIA_RPC_URL" --private-key "$RECEIVER_PK" --json | jq -r .transactionHash)
    append_receipt "$SCENARIO" "deposit" "$tx_dep"
    ;;
esac

penalty_start=$((timelock - PENALTY_WINDOW_SEC))

sign_and_claim () {
  local tx_sig
  local sig
  sig=$(node ./scripts/sign_claim.js --lockId "$lockId" --receiver "$RECEIVER_ADDR" --chainId "$CHAIN_ID" --contract "$HTLC")
  tx_sig=$(cast send "$HTLC" "claimWithSig(bytes32,bytes32,bytes)" "$lockId" "$preimage" "$sig" \
    --rpc-url "$SEPOLIA_RPC_URL" --private-key "$RECEIVER_PK" --json | jq -r .transactionHash)
  append_receipt "$SCENARIO" "claim" "$tx_sig"
  echo "Claim tx: $tx_sig"
}

case "$SCENARIO" in
  S1)
    echo "[5/6] Claim early (before penalty window)"
    sign_and_claim
    ;;
  S2)
    echo "[5/6] Wait to middle of penalty window, then claim"
    target=$((penalty_start + PENALTY_WINDOW_SEC/2))
    now=$(date +%s)
    if [ "$now" -lt "$target" ]; then
      sleep $((target - now))
    fi
    sign_and_claim
    ;;
  S3)
    echo "[5/6] No claim; wait until timelock, then refund"
    now=$(date +%s)
    if [ "$now" -lt "$timelock" ]; then
      sleep $((timelock - now + 5))
    fi
    tx_ref=$(cast send "$HTLC" "refund(bytes32)" "$lockId" \
      --rpc-url "$SEPOLIA_RPC_URL" --private-key "$DEPLOYER_PK" --json | jq -r .transactionHash)
    append_receipt "$SCENARIO" "refund" "$tx_ref"
    echo "Refund tx: $tx_ref"
    ;;
  S4)
    echo "[5/6] No deposit; wait depositWindow then refund"
    created=$(( $(date +%s) )) # approx; depositWindow is short; good enough for script
    sleep $((DEPOSIT_WINDOW_SEC + 5))
    tx_ref=$(cast send "$HTLC" "refund(bytes32)" "$lockId" \
      --rpc-url "$SEPOLIA_RPC_URL" --private-key "$DEPLOYER_PK" --json | jq -r .transactionHash)
    append_receipt "$SCENARIO" "refund" "$tx_ref"
    echo "Refund tx: $tx_ref"
    ;;
  *)
    echo "Unknown scenario: $SCENARIO" >&2
    exit 1
    ;;
esac

echo "[6/6] Done. Log appended to: $csv"
