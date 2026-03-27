package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	gethcommon "github.com/ethereum/go-ethereum/common"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"

	"mphtlc-lgp-mpc/internal/config"
	"mphtlc-lgp-mpc/internal/policy"
	"mphtlc-lgp-mpc/internal/tssengine"
)

func main() {
	var (
		mode              = flag.String("mode", "demo", "keygen | sign | policy | demo")
		dataDir           = flag.String("data-dir", "./data", "directory for persisted party shares")
		digestHex         = flag.String("digest", "", "32-byte digest hex for signing; if empty in demo/sign mode, a digest is derived from the message")
		message           = flag.String("message", "claimWithSig demo payload", "message used to derive a digest when --digest is empty")
		signersCSV        = flag.String("signers", "node-1,node-2", "comma-separated signer IDs")
		protocolStartUnix = flag.Int64("protocol-start-unix", time.Now().Add(-300*time.Second).Unix(), "swap creation timestamp (unix seconds)")
		warnOnly          = flag.Bool("warn-only", true, "when inside penalty window, warn instead of reject")
	)
	flag.Parse()

	absDataDir, err := filepath.Abs(*dataDir)
	if err != nil {
		log.Fatalf("resolve data dir: %v", err)
	}
	cfg := config.DefaultDemoConfig(absDataDir)
	cfg.Penalty.WarnOnly = *warnOnly
	cluster := tssengine.NewCluster(cfg)
	ctx := context.Background()

	switch *mode {
	case "keygen":
		res, err := cluster.RunKeygen(ctx)
		if err != nil {
			log.Fatalf("keygen failed: %v", err)
		}
		fmt.Printf("keygen ok\naddress=%s\npubkey=%s\nparticipants=%s\n", res.EthereumAddress.Hex(), res.PublicKeyHex, strings.Join(res.ParticipantIDs, ","))
	case "policy":
		start := time.Unix(*protocolStartUnix, 0)
		eval, err := policy.EvaluatePenaltyWindow(cfg.Penalty, start, time.Now())
		if err != nil {
			log.Fatalf("policy eval failed: %v", err)
		}
		fmt.Printf("decision=%s\nmessage=%s\nnow_offset=%s\nprojected_finish=%s\nprojected_penalty_ratio=%.4f\n",
			eval.Decision, eval.Message, eval.NowOffset, eval.ProjectedFinish, eval.ProjectedPenalty)
	case "sign":
		digest := mustDigest(*digestHex, *message)
		start := time.Unix(*protocolStartUnix, 0)
		eval, err := policy.EvaluatePenaltyWindow(cfg.Penalty, start, time.Now())
		if err != nil {
			log.Fatalf("policy eval failed: %v", err)
		}
		fmt.Printf("policy decision=%s message=%s projected_penalty_ratio=%.4f\n", eval.Decision, eval.Message, eval.ProjectedPenalty)
		if eval.Decision == policy.DecisionReject {
			os.Exit(2)
		}
		res, err := cluster.RunSigning(ctx, digest, parseCSV(*signersCSV))
		if err != nil {
			log.Fatalf("sign failed: %v", err)
		}
		fmt.Printf("signature=%s\nv=%d\nrecovered=%s\ndigest=%s\nsigners=%s\n",
			res.SignatureHex, res.V, res.Recovered.Hex(), res.DigestHex, strings.Join(res.SignerIDs, ","))
	case "demo":
		keyRes, err := cluster.RunKeygen(ctx)
		if err != nil {
			log.Fatalf("demo keygen failed: %v", err)
		}
		fmt.Printf("[1] keygen ok address=%s\n", keyRes.EthereumAddress.Hex())
		start := time.Unix(*protocolStartUnix, 0)
		eval, err := policy.EvaluatePenaltyWindow(cfg.Penalty, start, time.Now())
		if err != nil {
			log.Fatalf("demo policy failed: %v", err)
		}
		fmt.Printf("[2] policy decision=%s message=%s projected_penalty_ratio=%.4f\n", eval.Decision, eval.Message, eval.ProjectedPenalty)
		if eval.Decision == policy.DecisionReject {
			log.Fatalf("demo stopped by policy")
		}
		digest := mustDigest(*digestHex, *message)
		sigRes, err := cluster.RunSigning(ctx, digest, parseCSV(*signersCSV))
		if err != nil {
			log.Fatalf("demo signing failed: %v", err)
		}
		fmt.Printf("[3] signing ok signature=%s recovered=%s\n", sigRes.SignatureHex, sigRes.Recovered.Hex())
	default:
		log.Fatalf("unknown mode %q", *mode)
	}
}

func parseCSV(in string) []string {
	parts := strings.Split(in, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func mustDigest(digestHex, message string) []byte {
	if digestHex != "" {
		clean := strings.TrimPrefix(digestHex, "0x")
		bz, err := hex.DecodeString(clean)
		if err != nil {
			log.Fatalf("decode digest hex: %v", err)
		}
		if len(bz) != 32 {
			log.Fatalf("digest must be exactly 32 bytes, got %d", len(bz))
		}
		return bz
	}
	h := gethcrypto.Keccak256Hash([]byte(message))
	return h.Bytes()
}

var _ = gethcommon.Address{}
