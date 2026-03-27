package config

import "time"

type NodeConfig struct {
	ID        string `json:"id"`
	Moniker   string `json:"moniker"`
	Key       int64  `json:"key"`
	SharePath string `json:"share_path"`
}

type PenaltyConfig struct {
	TimeLockSeconds      int64         `json:"time_lock_seconds"`
	PenaltyWindowSeconds int64         `json:"penalty_window_seconds"`
	ExpectedSignLatency  time.Duration `json:"expected_sign_latency"`
	SafetyMargin         time.Duration `json:"safety_margin"`
	WarnOnly             bool          `json:"warn_only"`
}

type ClusterConfig struct {
	Parties             []NodeConfig  `json:"parties"`
	Threshold           int           `json:"threshold"`
	PreParamTimeout     time.Duration `json:"pre_param_timeout"`
	PreParamConcurrency int           `json:"pre_param_concurrency"`
	MessageTimeout      time.Duration `json:"message_timeout"`
	Penalty             PenaltyConfig `json:"penalty"`
}

func DefaultDemoConfig(baseDir string) ClusterConfig {
	return ClusterConfig{
		Threshold:           1, // 2-of-3
		PreParamTimeout:     5 * time.Minute,
		PreParamConcurrency: 4,
		MessageTimeout:      45 * time.Second,
		Penalty: PenaltyConfig{
			TimeLockSeconds:      1800,
			PenaltyWindowSeconds: 600,
			ExpectedSignLatency:  4 * time.Second,
			SafetyMargin:         2 * time.Second,
			WarnOnly:             true,
		},
		Parties: []NodeConfig{
			{ID: "node-1", Moniker: "Node-1", Key: 101, SharePath: baseDir + "/node-1-share.json"},
			{ID: "node-2", Moniker: "Node-2", Key: 102, SharePath: baseDir + "/node-2-share.json"},
			{ID: "node-3", Moniker: "Node-3", Key: 103, SharePath: baseDir + "/node-3-share.json"},
		},
	}
}
