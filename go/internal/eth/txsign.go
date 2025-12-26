package eth

import (
  "context"
  "crypto/ecdsa"
  "encoding/hex"
  "errors"
  "fmt"
  "math/big"
  "strings"
  "time"

  "github.com/ethereum/go-ethereum/accounts/abi"
  "github.com/ethereum/go-ethereum/common"
  "github.com/ethereum/go-ethereum/core/types"
  "github.com/ethereum/go-ethereum/crypto"
  "github.com/ethereum/go-ethereum/ethclient"
)

// BuildCallData packs ABI call.
func BuildCallData(abiJSON string, method string, args ...any) ([]byte, error) {
  a, err := abi.JSON(strings.NewReader(abiJSON))
  if err != nil { return nil, err }
  return a.Pack(method, args...)
}

func HexToBig(h string) (*big.Int, error) {
  h = strings.TrimPrefix(h, "0x")
  i := new(big.Int)
  _, ok := i.SetString(h, 16)
  if !ok { return nil, fmt.Errorf("bad hex int") }
  return i, nil
}

func MustAddr(s string) common.Address { return common.HexToAddress(s) }

// Compute address from uncompressed pubkey hex (0x04..).
func AddressFromUncompressed(pubHex string) (common.Address, error) {
  pubHex = strings.TrimPrefix(pubHex, "0x")
  b, err := hex.DecodeString(pubHex)
  if err != nil { return common.Address{}, err }
  pub, err := crypto.UnmarshalPubkey(b)
  if err != nil { return common.Address{}, err }
  return crypto.PubkeyToAddress(*pub), nil
}

// Find recovery id (0 or 1) by trying both and comparing recovered pubkey with expected address.
func FindRecID(hash32 []byte, r, s *big.Int, expected common.Address) (byte, error) {
  if len(hash32) != 32 { return 0, errors.New("hash must be 32 bytes") }
  sig := make([]byte, 65)
  rb := r.FillBytes(make([]byte, 32))
  sb := s.FillBytes(make([]byte, 32))
  copy(sig[0:32], rb)
  copy(sig[32:64], sb)
  for v := byte(0); v <= 1; v++ {
    sig[64] = v
    pub, err := crypto.SigToPub(hash32, sig)
    if err != nil { continue }
    addr := crypto.PubkeyToAddress(*pub)
    if addr == expected {
      return v, nil
    }
  }
  return 0, fmt.Errorf("cannot determine recid")
}

// Sign and send a dynamic fee tx via external (TSS) signer.
// The signer returns (r,s). We compute recId and let go-ethereum signer compute v per chain rules.
func SendTxWithExternalSig(
  ctx context.Context,
  ec *ethclient.Client,
  chainID *big.Int,
  from common.Address,
  to *common.Address,
  value *big.Int,
  data []byte,
  signerFn func(ctx context.Context, sighash []byte) (r, s *big.Int, took time.Duration, err error),
) (txHash common.Hash, gasUsed uint64, effGasPrice *big.Int, err error) {

  nonce, err := ec.PendingNonceAt(ctx, from)
  if err != nil { return common.Hash{}, 0, nil, err }

  tip, err := ec.SuggestGasTipCap(ctx)
  if err != nil { return common.Hash{}, 0, nil, err }
  fee, err := ec.SuggestGasPrice(ctx)
  if err != nil { return common.Hash{}, 0, nil, err }
  // Keep maxFee a bit above base fee
  maxFee := new(big.Int).Mul(fee, big.NewInt(2))

  // Estimate gas
  msg := ethereumCallMsg(from, to, value, data, tip, maxFee)
  gasLimit, err := ec.EstimateGas(ctx, msg)
  if err != nil { return common.Hash{}, 0, nil, fmt.Errorf("estimate gas: %w", err) }

  tx := types.NewTx(&types.DynamicFeeTx{
    ChainID: chainID,
    Nonce: nonce,
    GasTipCap: tip,
    GasFeeCap: maxFee,
    Gas: gasLimit,
    To: to,
    Value: value,
    Data: data,
  })

  ethSigner := types.LatestSignerForChainID(chainID)
  sighash := ethSigner.Hash(tx).Bytes() // 32 bytes

  r, s, _, err := signerFn(ctx, sighash)
  if err != nil { return common.Hash{}, 0, nil, err }

  recID, err := FindRecID(sighash, r, s, from)
  if err != nil { return common.Hash{}, 0, nil, err }

  sig := make([]byte, 65)
  copy(sig[0:32], r.FillBytes(make([]byte, 32)))
  copy(sig[32:64], s.FillBytes(make([]byte, 32)))
  sig[64] = recID

  signedTx, err := tx.WithSignature(ethSigner, sig)
  if err != nil { return common.Hash{}, 0, nil, err }

  if err := ec.SendTransaction(ctx, signedTx); err != nil { return common.Hash{}, 0, nil, err }

  // Wait for receipt
  receipt, err := waitReceipt(ctx, ec, signedTx.Hash())
  if err != nil { return signedTx.Hash(), 0, nil, err }

  // EffectiveGasPrice exists on receipt for EIP-1559
  eff := receipt.EffectiveGasPrice
  if eff == nil { eff = big.NewInt(0) }

  return signedTx.Hash(), receipt.GasUsed, eff, nil
}

// --- helpers (avoid importing bind) ---

type callMsg struct {
  From common.Address
  To *common.Address
  Gas uint64
  GasPrice *big.Int
  GasFeeCap *big.Int
  GasTipCap *big.Int
  Value *big.Int
  Data []byte
}

// We re-declare a struct compatible with ethclient.EstimateGas signature.
func ethereumCallMsg(from common.Address, to *common.Address, value *big.Int, data []byte, tip, fee *big.Int) interface{} {
  // ethclient expects ethereum.CallMsg, but importing "github.com/ethereum/go-ethereum" root is awkward.
  // We'll create it via reflection by importing the package in a small wrapper file.
  return newCallMsg(from, to, value, data, tip, fee)
}
