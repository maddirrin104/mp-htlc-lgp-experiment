package eth

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	domainTypeHash = crypto.Keccak256Hash([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))
	claimTypeHash  = crypto.Keccak256Hash([]byte("Claim(bytes32 lockId,address receiver)"))
	nameHash       = crypto.Keccak256Hash([]byte("MPHTLC_LGP"))
	versionHash    = crypto.Keccak256Hash([]byte("1"))
)

func leftPad32(b []byte) []byte {
	if len(b) >= 32 {
		out := make([]byte, 32)
		copy(out, b[len(b)-32:])
		return out
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}

func addrTo32(a common.Address) []byte {
	return leftPad32(a.Bytes())
}

// DomainSeparator builds OZ EIP712 domain separator.
func DomainSeparator(chainID *big.Int, verifyingContract common.Address) common.Hash {
	chainB := leftPad32(chainID.Bytes())
	enc := make([]byte, 0, 32*5)
	enc = append(enc, domainTypeHash.Bytes()...)
	enc = append(enc, nameHash.Bytes()...)
	enc = append(enc, versionHash.Bytes()...)
	enc = append(enc, chainB...)
	enc = append(enc, addrTo32(verifyingContract)...)
	return crypto.Keccak256Hash(enc)
}

// ClaimDigest computes EIP-712 digest for Claim(lockId, receiver) used by MPHTLC_LGP.claimWithSig.
func ClaimDigest(chainID *big.Int, verifyingContract common.Address, lockId [32]byte, receiver common.Address) common.Hash {
	// structHash = keccak256(abi.encode(CLAIM_TYPEHASH, lockId, receiver))
	enc := make([]byte, 0, 32*3)
	enc = append(enc, claimTypeHash.Bytes()...)
	enc = append(enc, lockId[:]...)
	enc = append(enc, addrTo32(receiver)...)
	structHash := crypto.Keccak256Hash(enc)
	ds := DomainSeparator(chainID, verifyingContract)
	// digest = keccak256("\x19\x01" || ds || structHash)
	buf := make([]byte, 0, 2+32+32)
	buf = append(buf, 0x19, 0x01)
	buf = append(buf, ds.Bytes()...)
	buf = append(buf, structHash.Bytes()...)
	return crypto.Keccak256Hash(buf)
}
