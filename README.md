# MP-HTLC-LGP Testnet Starter (Sepolia)

Starter kit để chạy thực nghiệm MP-HTLC-LGP trên **testnet** (không dùng Anvil local).

## 1) Yêu cầu môi trường
- Linux/WSL2 hoặc macOS
- Docker + docker compose
- Go >= 1.22
- Foundry (forge/cast)

Cài Foundry:
```bash
curl -L https://foundry.paradigm.xyz | bash
foundryup
```

## 2) Chuẩn bị ví và faucet
Tạo 2 ví testnet:
- `DEPLOYER` (deploy contract + lock)
- `RECEIVER` (confirmParticipation + claim)

Lấy ETH Sepolia từ faucet cho cả 2 ví.

## 3) Cấu hình .env
Tạo file `.env`:
```bash
SEPOLIA_RPC_URL="https://..."
CHAIN_ID=11155111
DEPLOYER_PK="0x..."
RECEIVER_PK="0x..."
SIGNER_PK="0x..."

# SIGNER_PK la EOA gia lap ADDR_TSS de ky EIP-712 (ban se thay bang TSS that o buoc 6)

# Tham số thực nghiệm (khuyến nghị giảm để đỡ chờ)
AMOUNT_TOKEN=100000000000000000000   # 100 MTK
TIMELOCK_SEC=600                      # 10 phút
PENALTY_WINDOW_SEC=180                # 3 phút cuối
DEPOSIT_REQUIRED_WEI=10000000000000000 # 0.01 ETH
DEPOSIT_WINDOW_SEC=120                # 2 phút

# Orchestrator output
OUT_LOG=./logs/run.csv
```

## 4) Deploy contracts lên Sepolia
```bash
cd mp-htlc-lgp-testnet-starter
source .env

forge install OpenZeppelin/openzeppelin-contracts --no-commit
forge build

forge script script/Deploy.s.sol:Deploy \
  --rpc-url "$SEPOLIA_RPC_URL" \
  --private-key "$DEPLOYER_PK" \
  --broadcast \
  -vvvv
```
Sau khi chạy xong, script in ra `TOKEN` và `HTLC` address. Ghi lại vào `configs/deployed.json`.

## 5) Chạy kịch bản S1–S4 trên testnet (baseline, sender = DEPLOYER)

Baseline này giữ nguyên contract và luồng S1–S4, nhưng **tx phía sender** (approve/lock/refund) do `DEPLOYER_PK` ký.

Cài deps JS (để ký EIP-712 claim):
```bash
npm i
```

Chạy:
```bash
./scripts/run_scenario.sh S1
./scripts/run_scenario.sh S2
./scripts/run_scenario.sh S3
./scripts/run_scenario.sh S4
```

## 6) Network simulation cho TSS (phần nâng cấp)
Thư mục `docker/tss-node` là khung để bạn gắn tss-lib node vào và bật `tc netem`.

Ví dụ chạy 3 node latency 0/150/250ms:
```bash
cd docker
docker compose up -d --build
```

## 7) Log
Scripts ghi `logs/run.csv` gồm:
- txHash, gasUsed, effectiveGasPrice, fee
- timestamps, penalty, refund

---

## Thiết kế
- Contract dùng EIP-712 để bind chữ ký claim vào `(lockId, receiver, chainId, contract)`.


---

## (NEW) Bản TSS ký transaction (approve/lock/refund)

Starter kit đã bổ sung thư mục **go/** để chạy thực nghiệm theo đúng yêu cầu: **ADDR_TSS (AggPK) ký trực tiếp các tx phía sender** (approve ERC20, lock, refund) trên **testnet Sepolia**, không dùng Anvil.

### A) Chạy signer service (mock mode để test pipeline)

> Trong mock mode, signer dùng `SIGNER_PK` như “AggPK/ADDR_TSS” để bạn chạy end-to-end ngay. Sau đó bạn thay hàm `signHashMock()` trong `go/cmd/signer/main.go` bằng call sang TSS thật (tss-lib/CMP/GG18) để có đúng TSS.

```bash
# Tại root project
cd go
export SIGNER_PK=0x...        # địa chỉ này sẽ là ADDR_TSS
export LISTEN=:8787
go run ./cmd/signer
```

### B) Chạy experiment (S1–S4) với tx ký bởi external signer

```bash
# Tại root project
cd go
export SEPOLIA_RPC_URL=... 
export CHAIN_ID=11155111
export DEPLOYER_PK=0x...
export RECEIVER_PK=0x...
export TSS_SIGNER_URL=http://127.0.0.1:8787

# (khuyến nghị) fund ADDR_TSS ít ETH để trả gas cho approve/lock/refund
# set FUND_TSS_WEI=0 nếu bạn đã faucet trực tiếp cho ADDR_TSS
export FUND_TSS_WEI=20000000000000000

# các tham số timelock/penalty/deposit giống .env bạn đang dùng
export AMOUNT_TOKEN=100000000000000000000
export TIMELOCK_SEC=600
export PENALTY_WINDOW_SEC=180
export DEPOSIT_REQUIRED_WEI=10000000000000000
export DEPOSIT_WINDOW_SEC=120
export OUT_LOG=../logs/run_tss_tx.csv

go run ./cmd/experiment --scenario S1
go run ./cmd/experiment --scenario S2
go run ./cmd/experiment --scenario S3
go run ./cmd/experiment --scenario S4
```

Kết quả sẽ append vào `logs/run_tss_tx.csv` gồm txHash, gasUsed, effectiveGasPrice.

### C) Nối TSS thật

- Giữ nguyên API bề mặt:
  - `GET /pubkey` -> {pubkeyHex,address}
  - `POST /signHash` -> {r,s,tookMs}
- Bạn thay phần `signHashMock()` bằng pipeline TSS thật:
  - input: 32-byte sighash
  - output: r,s (big.Int)

Phần khó nhất là **recovery-id**; starter đã xử lý bằng cách thử v=0/1 và so sánh recovered address với `ADDR_TSS`.

