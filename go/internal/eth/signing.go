package eth

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"mp-htlc-lgp/experiment/internal/tssnet"
)

// SignAndSendDynamicTx asks the TSS signer for (r,s), derives the recovery id v,
// attaches the signature to the tx, and broadcasts it.
func SignAndSendDynamicTx(ctx context.Context, rpc *ethclient.Client, chainID *big.Int, from common.Address, signerAPI *tssapi.Client, tx *types.Transaction) (*types.Transaction, error) {
	signer := types.LatestSignerForChainID(chainID)
	h := signer.Hash(tx)
	r, s, err := signerAPI.SignHash(h.Bytes())
	if err != nil {
		return nil, err
	}

	// Try v = 0/1, recover pubkey and match to expected 'from' address.
	var sig65 [65]byte
	copy(sig65[0:32], r)
	copy(sig65[32:64], s)
	found := false
	for v := byte(0); v < 2; v++ {
		sig65[64] = v
		pub, err := crypto.SigToPub(h.Bytes(), sig65[:])
		if err != nil {
			continue
		}
		addr := crypto.PubkeyToAddress(*pub)
		if addr == from {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("cannot derive recovery id: signature does not match from=%s", from.Hex())
	}

	signedTx, err := tx.WithSignature(signer, sig65[:])
	if err != nil {
		return nil, err
	}
	if err := rpc.SendTransaction(ctx, signedTx); err != nil {
		return nil, err
	}
	return signedTx, nil
}
