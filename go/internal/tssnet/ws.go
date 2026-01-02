package tssnet

import (
	"encoding/json"
	"time"
)

// WSMessage là schema dùng chung giữa gateway/coordinator/node qua WebSocket.
type WSMessage struct {
	// basic routing/session
	Type    string `json:"type,omitempty"`    // hello | cmd | send | pong | ...
	Session string `json:"session,omitempty"` // cluster session id
	Party   string `json:"party,omitempty"`   // P1..Pn hoặc gateway party id
	Role    string `json:"role,omitempty"`    // "gateway" | "node" | "coordinator"

	// command (gateway -> nodes/coordinator)
	Cmd       string   `json:"cmd,omitempty"`       // keygen | sign | keygen_result | sign_result ...
	Parties   []string `json:"parties,omitempty"`   // danh sách parties trong phiên / hoặc target list
	Threshold int      `json:"threshold,omitempty"` // t trong (t,n) nếu có
	HashHex   string   `json:"hash_hex,omitempty"`  // 0x... (nếu có)

	// response/result (nodes/coordinator -> gateway)
	Ok        bool   `json:"ok,omitempty"`
	Err       string `json:"err,omitempty"`
	PubKeyHex string `json:"pubkey_hex,omitempty"` // 0x...
	AddrHex   string `json:"addr_hex,omitempty"`   // 0x...
	RHex      string `json:"r_hex,omitempty"`      // 0x...
	SHex      string `json:"s_hex,omitempty"`      // 0x...

	// p2p relay (node <-> coordinator <-> node) hoặc routing chung
	From       string   `json:"from,omitempty"`        // sender party
	To         []string `json:"to,omitempty"`          // receivers
	Bcast      bool     `json:"bcast,omitempty"`       // broadcast flag
	PayloadB64 string   `json:"payload_b64,omitempty"` // wire message base64

	// optional trace
	MsgID string `json:"msg_id,omitempty"`
	TsMs  int64  `json:"ts_ms,omitempty"`

	// optional generic payload (để mở rộng về sau)
	Payload json.RawMessage `json:"payload,omitempty"`
}

func MustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func NowMs() int64 { return time.Now().UnixNano() / int64(time.Millisecond) }
