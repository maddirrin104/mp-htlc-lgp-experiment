package tssengine

import (
	"crypto/ecdsa"
	"fmt"

	tsscommon "github.com/bnb-chain/tss-lib/v2/common"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
)

type SignatureCompat struct {
	*tsscommon.SignatureData
}

func (s *SignatureCompat) Recovery() byte {
	if s == nil || s.SignatureData == nil || len(s.SignatureRecovery) == 0 {
		return 0
	}
	return s.SignatureRecovery[0]
}

func (s *SignatureCompat) RecoverPubkey(digest []byte) (*ecdsa.PublicKey, error) {
	if s == nil || s.SignatureData == nil {
		return nil, fmt.Errorf("nil signature")
	}
	sig := append(append(append([]byte{}, s.R...), s.S...), s.Recovery())
	return gethcrypto.SigToPub(digest, sig)
}
