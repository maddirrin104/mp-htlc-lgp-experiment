# MP-HTLC-LGP Testnet Starter (Sepolia) — Baseline + TSS Tx + Multi-party TSS + Netem

Starter kit để chạy thực nghiệm **MP-HTLC-LGP** trên **Sepolia testnet** (không dùng Anvil local), gồm 3 mức:

1) **Baseline**: sender ký tx bằng `DEPLOYER_PK`, claim ký EIP-712 (EOA)  
2) **TSS ký transaction**: sender (approve/lock/refund) do **ADDR_TSS** ký tx (external signer API)  
3) **TSS multi-party (N container)**: keygen/sign qua network + bật **tc netem** để đo **T_sign**

---

## 1) Yêu cầu môi trường

- Linux/WSL2  
- Docker + docker compose  
- Go (>= 1.21 khuyến nghị)  
- Foundry (forge/cast)  
- npm (>= 18)  

Khuyến nghị cài thêm:
- `jq` (đọc JSON dễ hơn)

---

## 2) Chuẩn bị ví Sepolia + faucet

Bạn cần 2 ví Sepolia:

- **DEPLOYER**: deploy contract, mint/transfer token, fund ETH cho receiver + fund ETH cho ADDR_TSS  
- **RECEIVER**: `confirmParticipation` + `claim`

Lấy ETH Sepolia từ faucet cho:
- DEPLOYER
- RECEIVER  
(ADDR_TSS sẽ xuất hiện sau keygen, bạn sẽ fund sau)

---

## 3) Cấu hình `.env`

Tạo file `.env` ở root project:

```bash
SEPOLIA_RPC_URL="https://..."
CHAIN_ID=11155111

DEPLOYER_PK="0x..."
RECEIVER_PK="0x..."

# (Baseline/mocked) có thể bỏ trống, bản multi-party không cần SIGNER_PK
SIGNER_PK=""

# Tham số thực nghiệm (khuyến nghị giảm để đỡ chờ trên testnet)
AMOUNT_TOKEN=100000000000000000000        # 100 MTK
TIMELOCK_SEC=600                           # 10 phút
PENALTY_WINDOW_SEC=180                     # 3 phút cuối
DEPOSIT_REQUIRED_WEI=10000000000000000     # 0.01 ETH
DEPOSIT_WINDOW_SEC=120                     # 2 phút

# Log
OUT_LOG=./logs/run.csv

# Deploy output (Foundry sẽ ghi file này)
DEPLOY_OUT=./broadcast_out/deployed.json

# External signer / TSS gateway (khi chạy TSS tx)
TSS_SIGNER_URL=http://localhost:9100
```

> Lưu ý: Foundry **không tự tạo** thư mục `broadcast_out`, bạn phải tạo trước (bước 4).

---

## 4) Deploy contracts lên Sepolia

### 4.1 Chuẩn bị quyền ghi file cho Foundry

Tạo thư mục output:

```bash
mkdir -p broadcast_out
```

Mở `foundry.toml` và đảm bảo có quyền ghi:

```toml
fs_permissions = [
  { access = "read-write", path = "./broadcast_out" }
]
```

### 4.2 Deploy

```bash
source .env

forge install OpenZeppelin/openzeppelin-contracts --no-commit
forge build

forge script script/Deploy.s.sol:Deploy   --rpc-url "$SEPOLIA_RPC_URL"   --private-key "$DEPLOYER_PK"   --broadcast -vvvv
```

Sau khi deploy xong:
- File được ghi tại: `./broadcast_out/deployed.json`

Để các script/runner đọc thống nhất, copy sang `configs/`:

```bash
mkdir -p configs
cp broadcast_out/deployed.json configs/deployed.json
cat configs/deployed.json
```

---

# PHẦN A — Baseline (sender = DEPLOYER, không TSS)

## 5) Cài deps JS (để ký EIP-712 claim)

```bash
npm i
```

## 6) Chạy kịch bản S1–S4 (baseline)

```bash
./scripts/run_scenario.sh S1
./scripts/run_scenario.sh S2
./scripts/run_scenario.sh S3
./scripts/run_scenario.sh S4
```

Kết quả log tại:
- `./logs/run.csv` (theo `OUT_LOG`)

---

# PHẦN B — TSS ký transaction (sender = ADDR_TSS)

Ở phần này:
- approve/lock/refund được ký **từ ADDR_TSS**
- chữ ký được lấy qua HTTP: `POST /signHash` (external signer)

## 7) Khởi động TSS multi-party network (N container)

Ví dụ chạy N=5, threshold t=2:

```bash
./tssnet/scripts/up.sh 5 2
```

Kiểm tra gateway sống:

```bash
curl http://localhost:9100/health
```

## 8) KeyGen để lấy ADDR_TSS

```bash
curl -X POST http://localhost:9100/keygen
```

Response sẽ có:
- `address` (chính là **ADDR_TSS**)
- `pubkey`
- `t_keygen_ms`

Export địa chỉ:

```bash
export ADDR_TSS=0x...   # lấy từ output keygen
```

## 9) Fund ETH Sepolia cho ADDR_TSS (BẮT BUỘC)

Vì sender tx là ADDR_TSS nên nó cần ETH trả gas:

```bash
source .env
cast send $ADDR_TSS --value 0.05ether   --rpc-url "$SEPOLIA_RPC_URL"   --private-key "$DEPLOYER_PK"
```

Check balance:

```bash
cast balance $ADDR_TSS --rpc-url "$SEPOLIA_RPC_URL"
```

## 10) Cấp token cho ADDR_TSS

Lấy `token` từ file deploy:

```bash
export TOKEN=$(jq -r .token configs/deployed.json)
```

Cấp token (tuỳ MockToken hỗ trợ):

**Cách A — mint thẳng cho ADDR_TSS**

```bash
cast send $TOKEN "mint(address,uint256)" $ADDR_TSS $AMOUNT_TOKEN   --rpc-url "$SEPOLIA_RPC_URL"   --private-key "$DEPLOYER_PK"
```

**Cách B — transfer sang ADDR_TSS**

```bash
cast send $TOKEN "transfer(address,uint256)" $ADDR_TSS $AMOUNT_TOKEN   --rpc-url "$SEPOLIA_RPC_URL"   --private-key "$DEPLOYER_PK"
```

Check token balance:

```bash
cast call $TOKEN "balanceOf(address)(uint256)" $ADDR_TSS --rpc-url "$SEPOLIA_RPC_URL"
```

## 11) Smoke test TSS sign (đo T_sign)

```bash
curl -s -X POST http://localhost:9100/signHash   -H 'Content-Type: application/json'   -d '{"hash_hex":"0x1111111111111111111111111111111111111111111111111111111111111111"}'
```

Response gồm `r,s` và `t_sign_ms`.

## 12) Chạy kịch bản S1–S4 với tx ký bởi TSS

```bash
./scripts/run_scenario_tss.sh S1
./scripts/run_scenario_tss.sh S2
./scripts/run_scenario_tss.sh S3
./scripts/run_scenario_tss.sh S4
```

Log:
- `./logs/run.csv` hoặc file theo `OUT_LOG`

---

# PHẦN C — Netem WAN simulation + đo T_sign theo latency/loss

Mục tiêu: giả lập WAN (0/150/250ms, loss…) rồi benchmark `T_sign` và chạy lại S2 để quan sát penalty thay đổi.

## 13) Bật tc netem cho các node TSS

Mở `tssnet/docker-compose.tssnet.yml` và set env cho từng node (ví dụ):

```yaml
environment:
  - LATENCY_MS=150
  - JITTER_MS=20
  - LOSS_PCT=0.5
```

Apply:

```bash
docker compose -f tssnet/docker-compose.tssnet.yml up -d
```

## 14) Benchmark ký nhiều lần để lấy thống kê T_sign

```bash
./tssnet/scripts/signbench.sh 0x1111111111111111111111111111111111111111111111111111111111111111 20
```

Bạn nên chạy theo nhiều profile:
- 0ms / 150ms / 250ms
- loss 0% / 0.5% / 1%

## 15) Chạy lại S2 dưới từng profile netem

Ví dụ:
1) set netem = 150ms
2) chạy `signbench`
3) chạy S2:

```bash
./scripts/run_scenario_tss.sh S2
```

4) đổi netem = 250ms và lặp lại

---

## 16) Output & log

Các scripts sẽ ghi CSV gồm (tuỳ runner):
- `scenario`, `step`, `txHash`, `gasUsed`, `effectiveGasPrice`, `fee`
- `t_sign_ms` (từ gateway)
- `penalty`, `refund`, `timestamps`

---

## Troubleshooting nhanh

### A) `vm.writeJson ... path not allowed`

- Thêm `fs_permissions` trong `foundry.toml`
- Đảm bảo output nằm trong path được cho phép

### B) `vm.writeJson ... No such file or directory`

- Tạo thư mục trước: `mkdir -p broadcast_out`

### C) S1 fail vì “insufficient funds” / “nonce too low”

- Check ETH của `ADDR_TSS`:

```bash
cast balance $ADDR_TSS --rpc-url "$SEPOLIA_RPC_URL"
```

- Nếu chạy nhiều lần, có thể cần đợi tx confirm hoặc tăng fee.

### D) lock fail vì “insufficient token allowance/balance”

- Check token balance + allowance của `ADDR_TSS`
- Mint/transfer token trước khi lock

---

## Thiết kế chữ ký

- Claim sử dụng chữ ký ECDSA từ `ADDR_TSS` (AggPK) và được bind vào các trường an toàn (EIP-712), tránh copy/preimage front-run.  
- Tx sender-side được ký bằng external signer API `/signHash`, client tự suy ra recovery-id (v) bằng cách thử v=0/1 và so địa chỉ recovered với `ADDR_TSS`.
