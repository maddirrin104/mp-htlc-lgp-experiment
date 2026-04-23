# MPC HTTP Service with Docker + netem

Bản này được dựng từ skeleton Go ban đầu và giữ nguyên logic mật mã cũ:

- **Digest dùng để ký là `Keccak256(message)`**
- **Không thêm EIP-191 prefix**
- Giá trị ký thực tế truyền vào `tss-lib` là `new(big.Int).SetBytes(digest)`

## Chạy

```bash
cd deploy
docker compose up --build -d
../scripts/apply-default-latency.sh
```

## Health

```bash
curl http://localhost:8081/health
curl http://localhost:8082/health
curl http://localhost:8083/health
```

## Keygen

```bash
curl -X POST http://localhost:8081/api/v1/keygen/start \
  -H 'Content-Type: application/json' \
  -d '{
    "session_id": "keygen-001",
    "protocol": "keygen",
    "participants": ["node1", "node2", "node3"],
    "threshold": 1
  }'
```

## Sign

```bash
NOW=$(date +%s)
TIMELOCK=$((NOW + 1800))

curl -X POST http://localhost:8081/api/v1/sign/start \
  -H 'Content-Type: application/json' \
  -d "{
    \"session_id\": \"sign-001\",
    \"protocol\": \"sign\",
    \"signers\": [\"node1\", \"node2\"],
    \"threshold\": 1,
    \"message\": \"claimWithSig:swap-001\",
    \"protocol_start_unix\": ${NOW},
    \"timelock_unix\": ${TIMELOCK},
    \"penalty_window_sec\": 600,
    \"estimated_mpc_ms\": 5000,
    \"strict_policy\": false
  }"
```

## Kiểm tra session

```bash
curl http://localhost:8081/api/v1/sessions/sign-001
```

Trong log sẽ có `T_sign_ms=...` để lấy số liệu biểu đồ.

## S5 - Network Chaos

Tăng delay node3 lên 4 giây:

```bash
../scripts/netem-control.sh set-delay mpc-node3 eth0 4000
```

Cắt mạng node3:

```bash
../scripts/netem-control.sh drop mpc-node3
```

Mạng chập chờn:

```bash
../scripts/netem-control.sh flaky mpc-node3 eth0 30 1200
```

Khôi phục:

```bash
../scripts/netem-control.sh clear mpc-node3
../scripts/netem-control.sh set-delay mpc-node3 eth0 250
```

## Timeout hiện tại

- `HTTP_TIMEOUT=45s`
- `SESSION_TIMEOUT=60s`
- `TSS_TIMEOUT=60s`
- `PREPARAM_TIMEOUT=45s`

Các giá trị này được đặt để tránh rớt nội bộ oan dưới WAN delay 150-250ms, nhưng vẫn cho phép tái hiện S5 khi đẩy delay/loss đủ cao.

## Section 6 - Đo lường và so sánh

Mục tiêu của section này là đặt MP-HTLC-LGP cạnh hai baseline trong cùng điều kiện mạng để nhìn rõ cả mặt mạnh lẫn đánh đổi của giao thức.

### Chỉ số đo

- `gas_used`: gas tiêu thụ cho đường `claim`.
- `capital_efficiency`: tỉ lệ `useful_locked_value / total_locked_capital`.
- `latency`: thời gian thực thi thực tế của phiên ký MPC, đo qua `T_sign_ms` trong log.

### Baseline so sánh

- Classic HTLC: chỉ có khóa tài sản, không có cơ chế phạt tuyến tính cho receiver collateral.
- Committee-verification baseline: đường xác thực phụ thuộc số thành viên, nên gas tăng nhanh khi N tăng.

### Kết quả hiện có trong project

- Latency benchmark đã được ghi nhận trong `plots/plot_metrics.py` và ảnh `plots/1.png`:
  - Baseline 250ms: `0.31s`
  - Flaky 30% drop: `33.13s`
  - High latency 4s: `55.60s`
- Gas benchmark của contract đã được bao phủ bởi test `test_S7_AggregatedClaimGas_RemainsSublinearForN20_50_100` và `test_S7_CommitteeVerificationUpperBoundGas_N20_50_100`.

### Cách vẽ biểu đồ

Chạy script này để xuất 3 ảnh trong cùng thư mục:

```bash
python plots/plot_metrics.py
```

Script sẽ tạo:

- `mpc_network_evaluation_final.png` cho latency.
- `gas_comparison_index.png` cho gas theo kích thước committee.
- `capital_efficiency_comparison.png` cho capital efficiency.

### Ghi chú phương pháp

Phần gas và capital efficiency hiện được biểu diễn theo cùng profile thực nghiệm của project để dễ so sánh xu hướng. Nếu bạn muốn số tuyệt đối để đưa vào báo cáo/paper, hãy thay các giá trị trong `plots/plot_metrics.py` bằng số đo trích từ `forge test -vvv` và log MPC thực tế.
