package eth

import (
	_ "embed"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

//go:embed abi/erc20.json
var erc20ABIJSON string

//go:embed abi/mphtlc_lgp.json
var mphtlcABIJSON string

func ERC20ABI() abi.ABI {
	a, err := abi.JSON(strings.NewReader(erc20ABIJSON))
	if err != nil {
		panic(err)
	}
	return a
}

func MPHTLCABI() abi.ABI {
	a, err := abi.JSON(strings.NewReader(mphtlcABIJSON))
	if err != nil {
		panic(err)
	}
	return a
}
