package eth

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

func PackERC20(method string, args ...interface{}) ([]byte, error) {
	a := ERC20ABI()
	data, err := a.Pack(method, args...)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func PackMPHTLC(method string, args ...interface{}) ([]byte, error) {
	a := MPHTLCABI()
	data, err := a.Pack(method, args...)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func CallBalanceOf(ctx context.Context, rpc *ethclient.Client, token common.Address, account common.Address) (*big.Int, error) {
	data, err := PackERC20("balanceOf", account)
	if err != nil {
		return nil, err
	}
	res, err := rpc.CallContract(ctx, ethereum.CallMsg{To: &token, Data: data}, nil)
	if err != nil {
		return nil, err
	}
	out, err := ERC20ABI().Unpack("balanceOf", res)
	if err != nil {
		return nil, err
	}
	if len(out) != 1 {
		return nil, fmt.Errorf("unexpected unpack len %d", len(out))
	}
	return out[0].(*big.Int), nil
}
