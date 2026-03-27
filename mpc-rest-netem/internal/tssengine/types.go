package tssengine

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"

	"mphtlc-lgp-mpc/internal/config"
)

type KeygenResult struct {
	PublicKey       *ecdsa.PublicKey `json:"-"`
	EthereumAddress common.Address   `json:"ethereum_address"`
	PublicKeyHex    string           `json:"public_key_hex"`
	ParticipantIDs  []string         `json:"participant_ids"`
}

type SignatureResult struct {
	R            string         `json:"r"`
	S            string         `json:"s"`
	V            byte           `json:"v"`
	SignatureHex string         `json:"signature_hex"`
	DigestHex    string         `json:"digest_hex"`
	Recovered    common.Address `json:"recovered_address"`
	SignerIDs    []string       `json:"signer_ids"`
}

func participantIDs(cfgs []config.NodeConfig) []string {
	ids := make([]string, 0, len(cfgs))
	for _, c := range cfgs {
		ids = append(ids, c.ID)
	}
	sort.Strings(ids)
	return ids
}

func marshalPubKey(pub *ecdsa.PublicKey) (common.Address, string, error) {
	if pub == nil {
		return common.Address{}, "", fmt.Errorf("nil public key")
	}
	uncompressed := gethcrypto.FromECDSAPub(pub)
	return gethcrypto.PubkeyToAddress(*pub), hex.EncodeToString(uncompressed), nil
}

func toBigInt32(digest []byte) *big.Int {
	return new(big.Int).SetBytes(digest)
}
