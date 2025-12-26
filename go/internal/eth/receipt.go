package eth

import (
  "context"
  "fmt"
  "time"

  "github.com/ethereum/go-ethereum/common"
  "github.com/ethereum/go-ethereum/core/types"
  "github.com/ethereum/go-ethereum/ethclient"
)

func waitReceipt(ctx context.Context, ec *ethclient.Client, h common.Hash) (*types.Receipt, error) {
  ticker := time.NewTicker(2 * time.Second)
  defer ticker.Stop()
  deadline := time.NewTimer(8 * time.Minute)
  defer deadline.Stop()

  for {
    receipt, err := ec.TransactionReceipt(ctx, h)
    if receipt != nil {
      return receipt, nil
    }
    if err != nil {
      // keep polling until mined
    }

    select {
    case <-ctx.Done():
      return nil, ctx.Err()
    case <-deadline.C:
      return nil, fmt.Errorf("timeout waiting receipt: %s", h.Hex())
    case <-ticker.C:
    }
  }
}
