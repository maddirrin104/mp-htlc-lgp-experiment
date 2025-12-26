package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Deployed struct {
	Token string `json:"token"`
	HTLC  string `json:"htlc"`
}

type Env struct {
	RPCURL              string
	ChainID             int64
	DeployerPK          string
	ReceiverPK          string
	SignerURL           string
	AmountToken         string // uint256 as decimal
	TimelockSec         int64
	PenaltyWindowSec    int64
	DepositRequiredWei  string // uint256 decimal
	DepositWindowSec    int64
	FundTSSWei          string // optional: send ETH to ADDR_TSS for gas
	OutLog              string
	DeployedJSONPath    string
}

func mustGet(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		panic(fmt.Sprintf("missing env %s", key))
	}
	return v
}

func getDefault(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func parseI64(key string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("bad %s: %v", key, err))
	}
	return i
}

func Load() Env {
	chainID, err := strconv.ParseInt(getDefault("CHAIN_ID", "11155111"), 10, 64)
	if err != nil {
		panic(err)
	}
	return Env{
		RPCURL:             mustGet("SEPOLIA_RPC_URL"),
		ChainID:            chainID,
		DeployerPK:         mustGet("DEPLOYER_PK"),
		ReceiverPK:         mustGet("RECEIVER_PK"),
		SignerURL:          getDefault("TSS_SIGNER_URL", "http://127.0.0.1:8080"),
		AmountToken:        getDefault("AMOUNT_TOKEN", "100000000000000000000"),
		TimelockSec:        parseI64("TIMELOCK_SEC", 600),
		PenaltyWindowSec:   parseI64("PENALTY_WINDOW_SEC", 180),
		DepositRequiredWei: getDefault("DEPOSIT_REQUIRED_WEI", "10000000000000000"),
		DepositWindowSec:   parseI64("DEPOSIT_WINDOW_SEC", 120),
		FundTSSWei:         getDefault("FUND_TSS_WEI", "20000000000000000"),
		OutLog:             getDefault("OUT_LOG", "./logs/run.csv"),
		DeployedJSONPath:   getDefault("DEPLOYED_JSON", "./configs/deployed.json"),
	}
}

func (e Env) LoadDeployed(projectRoot string) (Deployed, error) {
	path := e.DeployedJSONPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(projectRoot, path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Deployed{}, err
	}
	var d Deployed
	if err := json.Unmarshal(b, &d); err != nil {
		return Deployed{}, err
	}
	if d.Token == "" || d.HTLC == "" {
		return Deployed{}, errors.New("deployed.json missing token/htlc")
	}
	return d, nil
}
