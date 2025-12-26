package eth

import (
  "math/big"
  "github.com/ethereum/go-ethereum"
  "github.com/ethereum/go-ethereum/common"
)

func newCallMsg(from common.Address, to *common.Address, value *big.Int, data []byte, tip, fee *big.Int) ethereum.CallMsg {
  return ethereum.CallMsg{From: from, To: to, Value: value, Data: data, GasTipCap: tip, GasFeeCap: fee}
}
