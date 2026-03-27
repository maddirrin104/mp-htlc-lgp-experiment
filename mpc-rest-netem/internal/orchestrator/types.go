package orchestrator

import "time"

type Protocol string

const (
	ProtocolKeygen Protocol = "keygen"
	ProtocolSign   Protocol = "sign"
)

type SessionStatus string

const (
	StatusPending   SessionStatus = "pending"
	StatusRunning   SessionStatus = "running"
	StatusCompleted SessionStatus = "completed"
	StatusFailed    SessionStatus = "failed"
)

type KeygenStartRequest struct {
	SessionID    string   `json:"session_id"`
	Protocol     string   `json:"protocol"`
	Participants []string `json:"participants"`
	Threshold    int      `json:"threshold"`
}

type SignStartRequest struct {
	SessionID         string   `json:"session_id"`
	Protocol          string   `json:"protocol"`
	Signers           []string `json:"signers"`
	Threshold         int      `json:"threshold"`
	Message           string   `json:"message"`
	ProtocolStartUnix int64    `json:"protocol_start_unix"`
	TimeLockUnix      int64    `json:"timelock_unix"`
	PenaltyWindowSec  int64    `json:"penalty_window_sec"`
	EstimatedMPCMs    int64    `json:"estimated_mpc_ms"`
	StrictPolicy      bool     `json:"strict_policy"`
}

type SessionInitRequest struct {
	SessionID       string   `json:"session_id"`
	Protocol        Protocol `json:"protocol"`
	Participants    []string `json:"participants"`
	Threshold       int      `json:"threshold"`
	DigestHex       string   `json:"digest_hex,omitempty"`
	InitiatorNodeID string   `json:"initiator_node_id"`
}

type TSSMessageRequest struct {
	SessionID string   `json:"session_id"`
	Protocol  Protocol `json:"protocol"`
	WireHex   string   `json:"wire_hex"`
	FromID    string   `json:"from_id"`
	ToIDs     []string `json:"to_ids"`
	Broadcast bool     `json:"broadcast"`
}

type SessionResponse struct {
	SessionID        string        `json:"session_id"`
	Protocol         Protocol      `json:"protocol"`
	Status           SessionStatus `json:"status"`
	Error            string        `json:"error,omitempty"`
	StartedAt        time.Time     `json:"started_at"`
	CompletedAt      *time.Time    `json:"completed_at,omitempty"`
	ExecutionTimeMS  int64         `json:"execution_time_ms,omitempty"`
	SignatureHex     string        `json:"signature_hex,omitempty"`
	DigestHex        string        `json:"digest_hex,omitempty"`
	RecoveredAddress string        `json:"recovered_address,omitempty"`
	PublicKeyHex     string        `json:"public_key_hex,omitempty"`
	EthereumAddress  string        `json:"ethereum_address,omitempty"`
}
