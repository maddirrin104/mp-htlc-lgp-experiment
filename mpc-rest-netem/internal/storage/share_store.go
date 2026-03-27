package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	kg "github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
)

type ShareEnvelope struct {
	PartyID string                `json:"party_id"`
	Share   kg.LocalPartySaveData `json:"share"`
}

func SaveShare(path, partyID string, share kg.LocalPartySaveData) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir for share path: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create share file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(ShareEnvelope{PartyID: partyID, Share: share}); err != nil {
		return fmt.Errorf("encode share: %w", err)
	}
	return nil
}

func LoadShare(path string) (ShareEnvelope, error) {
	f, err := os.Open(path)
	if err != nil {
		return ShareEnvelope{}, fmt.Errorf("open share file: %w", err)
	}
	defer f.Close()

	var env ShareEnvelope
	if err := json.NewDecoder(f).Decode(&env); err != nil {
		return ShareEnvelope{}, fmt.Errorf("decode share: %w", err)
	}
	return env, nil
}
