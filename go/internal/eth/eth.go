package eth

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Client struct {
	RPC *ethclient.Client
}

func Dial(rpc string) (*Client, error) {
	rpc = strings.TrimSpace(rpc)
	c, err := ethclient.Dial(rpc)
	if err != nil {
		return nil, err
	}
	return &Client{RPC: c}, nil
}

func MustAddress(s string) common.Address {
	if !common.IsHexAddress(s) {
		panic(fmt.Sprintf("bad address: %s", s))
	}
	return common.HexToAddress(s)
}

func MustHex32(s string) [32]byte {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		panic(fmt.Sprintf("bad bytes32: %s", s))
	}
	var out [32]byte
	copy(out[:], b)
	return out
}

func PrivKeyFromHex(pk string) (*ecdsa.PrivateKey, common.Address, error) {
	pk = strings.TrimSpace(pk)
	pk = strings.TrimPrefix(pk, "0x")
	k, err := crypto.HexToECDSA(pk)
	if err != nil {
		return nil, common.Address{}, err
	}
	addr := crypto.PubkeyToAddress(k.PublicKey)
	return k, addr, nil
}

func BigFromDec(dec string) (*big.Int, error) {
	i := new(big.Int)
	_, ok := i.SetString(strings.TrimSpace(dec), 10)
	if !ok {
		return nil, fmt.Errorf("bad decimal: %s", dec)
	}
	return i, nil
}

// BuildDynamicTx builds an EIP-1559 tx with sane defaults.
func BuildDynamicTx(ctx context.Context, c *ethclient.Client, chainID *big.Int, from common.Address, to *common.Address, data []byte, value *big.Int) (*types.Transaction, error) {
	nonce, err := c.PendingNonceAt(ctx, from)
	if err != nil {
		return nil, err
	}
	tip, err := c.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, err
	}
	header, err := c.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, err
	}
	baseFee := header.BaseFee
	if baseFee == nil {
		baseFee = big.NewInt(0)
	}
	maxFee := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), tip)
	// Estimate gas
	msg := ethereum.CallMsg{From: from}
	if to != nil {
		msg.To = to
	}
	msg.Value = value
	msg.Data = data
	gas, err := c.EstimateGas(ctx, msg)
	if err != nil {
		return nil, err
	}
	// Add a bit of headroom
	gas = gas + gas/5

	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		GasTipCap: tip,
		GasFeeCap: maxFee,
		Gas:       gas,
		To:        to,
		Value:     value,
		Data:      data,
	})
	return tx, nil
}

func WaitMined(ctx context.Context, c *ethclient.Client, tx *types.Transaction) (*types.Receipt, error) {
	for {
		r, err := c.TransactionReceipt(ctx, tx.Hash())
		if err == nil {
			return r, nil
		}
		// keep polling
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(4 * time.Second):
		}
	}
}

func ParseABI(jsonABI string) abi.ABI {
	a, err := abi.JSON(strings.NewReader(jsonABI))
	if err != nil {
		panic(err)
	}
	return a
}

