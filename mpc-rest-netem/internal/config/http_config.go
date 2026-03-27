package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type PeerInfo struct {
	ID      string `json:"id"`
	BaseURL string `json:"base_url"`
}

type HTTPConfig struct {
	NodeID              string        `json:"node_id"`
	ListenAddr          string        `json:"listen_addr"`
	BaseURL             string        `json:"base_url"`
	DataDir             string        `json:"data_dir"`
	SharePath           string        `json:"share_path"`
	Threshold           int           `json:"threshold"`
	PreParamTimeout     time.Duration `json:"pre_param_timeout"`
	PreParamConcurrency int           `json:"pre_param_concurrency"`
	HTTPTimeout         time.Duration `json:"http_timeout"`
	SessionTimeout      time.Duration `json:"session_timeout"`
	TssTimeout          time.Duration `json:"tss_timeout"`
	Peers               []PeerInfo    `json:"peers"`
}

func LoadHTTPConfigFromEnv() (HTTPConfig, error) {
	dataDir := envOr("DATA_DIR", "./data")
	nodeID := envOr("NODE_ID", "node1")

	cfg := HTTPConfig{
		NodeID:              nodeID,
		ListenAddr:          envOr("LISTEN_ADDR", ":8080"),
		BaseURL:             envOr("BASE_URL", "http://127.0.0.1:8080"),
		DataDir:             dataDir,
		SharePath:           envOr("SHARE_PATH", filepath.Join(dataDir, nodeID+"-share.json")),
		Threshold:           mustParseInt(envOr("THRESHOLD", "1")),
		PreParamConcurrency: mustParseInt(envOr("PREPARAM_CONCURRENCY", "4")),
		PreParamTimeout:     mustParseDuration(envOr("PREPARAM_TIMEOUT", "45s")),
		HTTPTimeout:         mustParseDuration(envOr("HTTP_TIMEOUT", "45s")),
		SessionTimeout:      mustParseDuration(envOr("SESSION_TIMEOUT", "60s")),
		TssTimeout:          mustParseDuration(envOr("TSS_TIMEOUT", "60s")),
	}

	peersJSON := envOr("PEERS_JSON", "[]")
	if err := json.Unmarshal([]byte(peersJSON), &cfg.Peers); err != nil {
		return HTTPConfig{}, fmt.Errorf("parse PEERS_JSON: %w", err)
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustParseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		panic(err)
	}
	return d
}

func mustParseInt(s string) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return v
}