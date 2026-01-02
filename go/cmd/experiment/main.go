package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"mp-htlc-lgp/experiment/internal/config"
	"mp-htlc-lgp/experiment/internal/eth"
	"mp-htlc-lgp/experiment/internal/tssnet"
)

func must(b *big.Int, err error) *big.Int {
	if err != nil { panic(err) }
	return b
}

func main() {
	scenario := flag.String("scenario", "S1", "S1|S2|S3|S4")
	flag.Parse()

	projectRoot, _ := os.Getwd()
	// If running from /go, go up to repo root.
	if strings.HasSuffix(projectRoot, string(filepath.Separator)+"go") {
		projectRoot = filepath.Dir(projectRoot)
	}
	env := config.Load()
	d, err := env.LoadDeployed(projectRoot)
	if err != nil {
		log.Fatalf("load deployed: %v", err)
	}

	ctx := context.Background()
	cli, err := eth.Dial(env.RPCURL)
	if err != nil { log.Fatalf("dial: %v", err) }
	chainID := big.NewInt(env.ChainID)

	deployerKey, deployerAddr, err := eth.PrivKeyFromHex(env.DeployerPK)
	if err != nil { log.Fatalf("deployer pk: %v", err) }
	receiverKey, receiverAddr, err := eth.PrivKeyFromHex(env.ReceiverPK)
	if err != nil { log.Fatalf("receiver pk: %v", err) }

	signerAPI := tssapi.New(env.SignerURL)
	signerAddrHex, _, err := signerAPI.GetAddress()
	if err != nil { log.Fatalf("signer /address: %v", err) }
	signerAddr := eth.MustAddress(signerAddrHex)

	token := eth.MustAddress(d.Token)
	htlc := eth.MustAddress(d.HTLC)

	amountToken := must(eth.BigFromDec(env.AmountToken))
	depositRequired := must(eth.BigFromDec(env.DepositRequiredWei))
	fundTSS := must(eth.BigFromDec(env.FundTSSWei))
	depositWindow := big.NewInt(env.DepositWindowSec)
	penaltyWindow := big.NewInt(env.PenaltyWindowSec)

	log.Printf("scenario=%s\nchainID=%d\nHTLC=%s\nToken=%s\nADDR_TSS=%s\nReceiver=%s\n",
		*scenario, env.ChainID, htlc.Hex(), token.Hex(), signerAddr.Hex(), receiverAddr.Hex())

	// Fund ADDR_TSS with some ETH for gas (testnet). You can set FUND_TSS_WEI=0 to skip.
	if fundTSS.Sign() > 0 {
		fundTx := sendEOATx(ctx, cli.RPC, chainID, deployerKey, deployerAddr, &signerAddr, nil, fundTSS)
		fundRcpt := mustReceipt(ctx, cli.RPC, fundTx)
		writeLog(env.OutLog, *scenario, "fund_TSS", fundTx, fundRcpt)
	}

	// 0) Mint token to ADDR_TSS (deployer is minter in MockToken)
	mintData, _ := eth.PackERC20("mint", signerAddr, amountToken)
	mintTx := sendEOATx(ctx, cli.RPC, chainID, deployerKey, deployerAddr, &token, mintData, big.NewInt(0))
	mintRcpt := mustReceipt(ctx, cli.RPC, mintTx)
	writeLog(env.OutLog, *scenario, "mint", mintTx, mintRcpt)

	// 1) Approve HTLC from ADDR_TSS (signed by TSS)
	approveData, _ := eth.PackERC20("approve", htlc, amountToken)
	approveTxUnsigned, err := eth.BuildDynamicTx(ctx, cli.RPC, chainID, signerAddr, &token, approveData, big.NewInt(0))
	if err != nil { log.Fatalf("build approve tx: %v", err) }
	approveTx, err := eth.SignAndSendDynamicTx(ctx, cli.RPC, chainID, signerAddr, signerAPI, approveTxUnsigned)
	if err != nil { log.Fatalf("send approve: %v", err) }
	approveRcpt := mustReceipt(ctx, cli.RPC, approveTx)
	writeLog(env.OutLog, *scenario, "approve", approveTx, approveRcpt)

	// 2) Create lock params
	preimage := rand32()
	hashlock := crypto.Keccak256Hash(preimage[:])
	rnd := rand32()
	lockId := crypto.Keccak256Hash(append([]byte("lock-"), rnd[:]...))
	// timelock is absolute timestamp
	nowTs := latestTs(ctx, cli.RPC)
	timelock := new(big.Int).SetUint64(uint64(nowTs + env.TimelockSec))

	lockData, _ := eth.PackMPHTLC("lock",
		lockId, token, receiverAddr, signerAddr, amountToken, hashlock,
		timelock, penaltyWindow, depositRequired, depositWindow,
	)
	lockTxUnsigned, err := eth.BuildDynamicTx(ctx, cli.RPC, chainID, signerAddr, &htlc, lockData, big.NewInt(0))
	if err != nil { log.Fatalf("build lock tx: %v", err) }
	lockTx, err := eth.SignAndSendDynamicTx(ctx, cli.RPC, chainID, signerAddr, signerAPI, lockTxUnsigned)
	if err != nil { log.Fatalf("send lock: %v", err) }
	lockRcpt := mustReceipt(ctx, cli.RPC, lockTx)
	writeLog(env.OutLog, *scenario, "lock", lockTx, lockRcpt)

	createdAt := latestTs(ctx, cli.RPC)
	penaltyStart := int64(0)
	if env.PenaltyWindowSec > 0 {
		penaltyStart = (nowTs + env.TimelockSec) - env.PenaltyWindowSec
	}
	log.Printf("lockId=%s\nhashlock=%s\npreimage=0x%s\ncreatedAt~%d\ntimelock=%s (ts=%d)\npenaltyStart=%d\n",
		lockId.Hex(), hashlock.Hex(), hex.EncodeToString(preimage[:]), createdAt, timelock.String(), nowTs+env.TimelockSec, penaltyStart)

	switch strings.ToUpper(*scenario) {
	case "S1":
		// deposit then claim BEFORE penaltyStart
		confirm := confirmParticipation(ctx, cli.RPC, chainID, receiverKey, receiverAddr, htlc, lockId, depositRequired)
		confirmRcpt := mustReceipt(ctx, cli.RPC, confirm)
		writeLog(env.OutLog, *scenario, "confirmParticipation", confirm, confirmRcpt)

		// wait until (penaltyStart - 10s) but not negative
		target := penaltyStart - 10
		if target < latestTs(ctx, cli.RPC)+1 {
			target = latestTs(ctx, cli.RPC) + 1
		}
		waitUntil(ctx, cli.RPC, target)

			sig := buildClaimSig(ctx, signerAPI, chainID, htlc, lockId, receiverAddr, signerAddr)
		claimTx := claimWithSig(ctx, cli.RPC, chainID, receiverKey, receiverAddr, htlc, lockId, preimage, sig)
		claimRcpt := mustReceipt(ctx, cli.RPC, claimTx)
		writeLog(env.OutLog, *scenario, "claimWithSig", claimTx, claimRcpt)

	case "S2":
		// deposit then claim IN penalty window (mid)
		confirm := confirmParticipation(ctx, cli.RPC, chainID, receiverKey, receiverAddr, htlc, lockId, depositRequired)
		confirmRcpt := mustReceipt(ctx, cli.RPC, confirm)
		writeLog(env.OutLog, *scenario, "confirmParticipation", confirm, confirmRcpt)

		mid := penaltyStart + env.PenaltyWindowSec/2
		if mid < latestTs(ctx, cli.RPC)+1 {
			mid = latestTs(ctx, cli.RPC) + 1
		}
		waitUntil(ctx, cli.RPC, mid)

			sig := buildClaimSig(ctx, signerAPI, chainID, htlc, lockId, receiverAddr, signerAddr)
		claimTx := claimWithSig(ctx, cli.RPC, chainID, receiverKey, receiverAddr, htlc, lockId, preimage, sig)
		claimRcpt := mustReceipt(ctx, cli.RPC, claimTx)
		writeLog(env.OutLog, *scenario, "claimWithSig", claimTx, claimRcpt)

	case "S3":
		// deposit then DO NOT claim; refund after timelock
		confirm := confirmParticipation(ctx, cli.RPC, chainID, receiverKey, receiverAddr, htlc, lockId, depositRequired)
		confirmRcpt := mustReceipt(ctx, cli.RPC, confirm)
		writeLog(env.OutLog, *scenario, "confirmParticipation", confirm, confirmRcpt)

		waitUntil(ctx, cli.RPC, nowTs+env.TimelockSec+5)

		refundData, _ := eth.PackMPHTLC("refund", lockId)
		refundUnsigned, err := eth.BuildDynamicTx(ctx, cli.RPC, chainID, signerAddr, &htlc, refundData, big.NewInt(0))
		if err != nil { log.Fatalf("build refund: %v", err) }
		refundTx, err := eth.SignAndSendDynamicTx(ctx, cli.RPC, chainID, signerAddr, signerAPI, refundUnsigned)
		if err != nil { log.Fatalf("send refund: %v", err) }
		refundRcpt := mustReceipt(ctx, cli.RPC, refundTx)
		writeLog(env.OutLog, *scenario, "refund", refundTx, refundRcpt)

	case "S4":
		// no deposit; refund after depositWindow
		waitUntil(ctx, cli.RPC, int64(createdAt)+env.DepositWindowSec+5)
		refundData, _ := eth.PackMPHTLC("refund", lockId)
		refundUnsigned, err := eth.BuildDynamicTx(ctx, cli.RPC, chainID, signerAddr, &htlc, refundData, big.NewInt(0))
		if err != nil { log.Fatalf("build refund: %v", err) }
		refundTx, err := eth.SignAndSendDynamicTx(ctx, cli.RPC, chainID, signerAddr, signerAPI, refundUnsigned)
		if err != nil { log.Fatalf("send refund: %v", err) }
		refundRcpt := mustReceipt(ctx, cli.RPC, refundTx)
		writeLog(env.OutLog, *scenario, "refund", refundTx, refundRcpt)

	default:
		log.Fatalf("unknown scenario %s", *scenario)
	}

	log.Printf("done %s -> log at %s", *scenario, env.OutLog)
}

func rand32() [32]byte {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return b
}

func latestTs(ctx context.Context, rpc *ethclient.Client) int64 {
	h, err := rpc.HeaderByNumber(ctx, nil)
	if err != nil {
		panic(err)
	}
	return int64(h.Time)
}

func waitUntil(ctx context.Context, rpc *ethclient.Client, targetTs int64) {
	for {
		now := latestTs(ctx, rpc)
		if now >= targetTs {
			return
		}
		log.Printf("waiting: now=%d target=%d (sleep 4s)", now, targetTs)
		select {
		case <-ctx.Done():
			panic(ctx.Err())
		case <-time.After(4 * time.Second):
		}
	}
}

func mustReceipt(ctx context.Context, rpc *ethclient.Client, tx *types.Transaction) *types.Receipt {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	r, err := eth.WaitMined(cctx, rpc, tx)
	if err != nil {
		panic(err)
	}
	return r
}

func sendEOATx(ctx context.Context, rpc *ethclient.Client, chainID *big.Int, key *ecdsa.PrivateKey, from common.Address, to *common.Address, data []byte, value *big.Int) *types.Transaction {
	unsigned, err := eth.BuildDynamicTx(ctx, rpc, chainID, from, to, data, value)
	if err != nil { panic(err) }
	signer := types.LatestSignerForChainID(chainID)
	h := signer.Hash(unsigned)
	sig, err := crypto.Sign(h.Bytes(), key)
	if err != nil { panic(err) }
	signed, err := unsigned.WithSignature(signer, sig)
	if err != nil { panic(err) }
	if err := rpc.SendTransaction(ctx, signed); err != nil { panic(err) }
	return signed
}

func confirmParticipation(ctx context.Context, rpc *ethclient.Client, chainID *big.Int, key *ecdsa.PrivateKey, from common.Address, htlc common.Address, lockId common.Hash, depositRequired *big.Int) *types.Transaction {
	data, _ := eth.PackMPHTLC("confirmParticipation", lockId)
	return sendEOATx(ctx, rpc, chainID, key, from, &htlc, data, depositRequired)
}

func claimWithSig(ctx context.Context, rpc *ethclient.Client, chainID *big.Int, key *ecdsa.PrivateKey, from common.Address, htlc common.Address, lockId common.Hash, preimage [32]byte, sig65 []byte) *types.Transaction {
	data, _ := eth.PackMPHTLC("claimWithSig", lockId, preimage, sig65)
	return sendEOATx(ctx, rpc, chainID, key, from, &htlc, data, big.NewInt(0))
}

func buildClaimSig(ctx context.Context, signerAPI *tssapi.Client, chainID *big.Int, verifyingContract common.Address, lockId common.Hash, receiver common.Address, expectedSigner common.Address) []byte {
	lid := [32]byte{}
	copy(lid[:], lockId.Bytes())
	digest := eth.ClaimDigest(chainID, verifyingContract, lid, receiver)
	r, s, err := signerAPI.SignHash(digest.Bytes())
	if err != nil { panic(err) }
	// derive v (0/1) then return 65 bytes with v=27/28 for OZ ECDSA.recover
	var sig65 [65]byte
	copy(sig65[0:32], r)
	copy(sig65[32:64], s)
	for v := byte(0); v < 2; v++ {
		sig65[64] = v
		pub, err := crypto.SigToPub(digest.Bytes(), sig65[:])
		if err != nil { continue }
		addr := crypto.PubkeyToAddress(*pub)
		if addr != expectedSigner {
			continue
		}
		sig65[64] = v + 27
		return sig65[:]
	}
	panic("cannot compute v for claim signature")
}

func writeLog(path string, scenario string, step string, tx *types.Transaction, r *types.Receipt) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	newFile := false
	if _, err := os.Stat(path); err != nil {
		newFile = true
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil { panic(err) }
	defer f.Close()
	w := csv.NewWriter(f)
	if newFile {
		_ = w.Write([]string{"timestamp", "scenario", "step", "txHash", "status", "gasUsed", "effectiveGasPriceWei"})
	}
	_ = w.Write([]string{
		time.Now().Format(time.RFC3339),
		scenario,
		step,
		tx.Hash().Hex(),
		fmt.Sprintf("%d", r.Status),
		fmt.Sprintf("%d", r.GasUsed),
		r.EffectiveGasPrice.String(),
	})
	w.Flush()
}
