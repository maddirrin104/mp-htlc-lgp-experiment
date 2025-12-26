# Multi-party TSS (N containers) + tc netem

Thư mục này bổ sung **TSS thật multi-party** (nhiều container) giao tiếp qua network (WebSocket relay), để bạn có thể bật **tc netem** và đo `T_sign` đúng ý tưởng trong `HTLC.pdf`.

## 1) Kiến trúc

- `tss-coordinator` (port `9000`): WebSocket relay (router) cho thông điệp TSS.
- `tss-node-Pi`: mỗi node là một party TSS, lưu share key ở `./tssnet/data/Pi/keygen.json`.
- `tss-gateway` (port `9100`): HTTP API để bạn gọi `keygen`/`sign` và đo thời gian.

Giao thức TSS dùng `github.com/bnb-chain/tss-lib/v2` (ECDSA, secp256k1), message được `WireBytes()` và chuyển qua mạng qua coordinator, phía nhận gọi `UpdateFromBytes()`.

## 2) Chạy N party

Tại root repo:

```bash
# N=5, threshold t=2
./tssnet/scripts/up.sh 5 2
```

Mặc định sẽ sinh file `tssnet/docker-compose.tssnet.yml` và `docker compose up -d --build`.

> **Yêu cầu:** Docker Desktop/Linux docker + docker-compose v2.

## 3) Bật tc netem để đo T_sign

Mở file `tssnet/docker-compose.tssnet.yml`, chỉnh env cho node mà bạn muốn:

```yaml
environment:
  - LATENCY_MS=80
  - JITTER_MS=20
  - LOSS_PCT=0.5
```

Sau đó:

```bash
docker compose -f tssnet/docker-compose.tssnet.yml up -d
```

Mỗi node có `cap_add: NET_ADMIN` và entrypoint sẽ tự áp dụng `tc qdisc netem` trên `eth0`.

## 4) API

### Keygen

```bash
curl -X POST http://localhost:9100/keygen
```

Trả về:
- `address`, `pubkey`
- `t_keygen_ms`

Hoặc dùng:

```bash
./tssnet/scripts/keygen.sh
```

### Sign hash (đo T_sign)

```bash
curl -X POST http://localhost:9100/signHash \
  -H 'Content-Type: application/json' \
  -d '{"hash_hex":"0x<32-byte-hash>"}'
```

Trả về:
- `r`, `s` (hex)
- `t_sign_ms`

Benchmark nhiều lần:

```bash
./tssnet/scripts/signbench.sh 0x<32-byte-hash> 20
```

> Script `keygen.sh` và `signbench.sh` dùng `jq`. Nếu máy bạn chưa có `jq`, có thể đọc JSON thủ công hoặc cài thêm.

## 5) Gắn vào pipeline ký tx của repo

Trong bản `TSS ký tx` trước đó, bạn chỉ cần trỏ `TSS_SIGNER_URL` sang gateway:

- `TSS_SIGNER_URL=http://localhost:9100`

Rồi flow ký giao dịch sẽ gọi `POST /signHash` để lấy `(r,s)`.

## 6) Gợi ý đo đạc

- Đo `T_sign` ở gateway (`t_sign_ms`) => đo end-to-end qua network.
- Đồng thời lưu log của từng node để đối chiếu round nào bị chậm khi tăng delay/loss:

```bash
docker compose -f tssnet/docker-compose.tssnet.yml logs -f tss-node-P1
```
