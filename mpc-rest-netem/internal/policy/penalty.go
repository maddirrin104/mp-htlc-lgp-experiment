package policy

import (
	"errors"
	"fmt"
	"time"

	"mphtlc-lgp-mpc/internal/config"
)

type Decision string

const (
	DecisionAllow  Decision = "allow"
	DecisionWarn   Decision = "warn"
	DecisionReject Decision = "reject"
)

type Evaluation struct {
	Decision           Decision      `json:"decision"`
	Message            string        `json:"message"`
	NowOffset          time.Duration `json:"now_offset"`
	PenaltyStartOffset time.Duration `json:"penalty_start_offset"`
	TimeLockOffset     time.Duration `json:"time_lock_offset"`
	ProjectedFinish    time.Duration `json:"projected_finish"`
	ProjectedPenalty   float64       `json:"projected_penalty_ratio"`
}

func EvaluatePenaltyWindow(cfg config.PenaltyConfig, protocolStart time.Time, now time.Time) (Evaluation, error) {
	if cfg.TimeLockSeconds <= 0 {
		return Evaluation{}, errors.New("timeLockSeconds must be > 0")
	}
	if cfg.PenaltyWindowSeconds <= 0 || cfg.PenaltyWindowSeconds > cfg.TimeLockSeconds {
		return Evaluation{}, errors.New("penaltyWindowSeconds must be in (0, timeLockSeconds]")
	}

	penaltyStart := protocolStart.Add(time.Duration(cfg.TimeLockSeconds-cfg.PenaltyWindowSeconds) * time.Second)
	timeLock := protocolStart.Add(time.Duration(cfg.TimeLockSeconds) * time.Second)
	projectedFinish := now.Add(cfg.ExpectedSignLatency + cfg.SafetyMargin)

	eval := Evaluation{
		Decision:           DecisionAllow,
		NowOffset:          now.Sub(protocolStart),
		PenaltyStartOffset: penaltyStart.Sub(protocolStart),
		TimeLockOffset:     timeLock.Sub(protocolStart),
		ProjectedFinish:    projectedFinish.Sub(protocolStart),
	}

	switch {
	case !now.Before(timeLock):
		eval.Decision = DecisionReject
		eval.ProjectedPenalty = 1.0
		eval.Message = "refuse to sign: timelock has passed; on-chain refund/slash path is already active"
		return eval, nil
	case !projectedFinish.Before(timeLock):
		eval.Decision = DecisionReject
		eval.ProjectedPenalty = 1.0
		eval.Message = "refuse to sign: projected MPC completion exceeds timelock"
		return eval, nil
	case !now.Before(penaltyStart):
		eval.ProjectedPenalty = clampPenaltyRatio(projectedFinish.Sub(penaltyStart), time.Duration(cfg.PenaltyWindowSeconds)*time.Second)
		if cfg.WarnOnly {
			eval.Decision = DecisionWarn
			eval.Message = fmt.Sprintf("signing is inside penalty window; projected penalty ratio %.4f", eval.ProjectedPenalty)
		} else {
			eval.Decision = DecisionReject
			eval.Message = fmt.Sprintf("refuse to sign inside penalty window; projected penalty ratio %.4f", eval.ProjectedPenalty)
		}
		return eval, nil
	case !projectedFinish.Before(penaltyStart):
		eval.ProjectedPenalty = clampPenaltyRatio(projectedFinish.Sub(penaltyStart), time.Duration(cfg.PenaltyWindowSeconds)*time.Second)
		eval.Decision = DecisionWarn
		eval.Message = fmt.Sprintf("projected signing completion may cross into penalty window; projected penalty ratio %.4f", eval.ProjectedPenalty)
		return eval, nil
	default:
		eval.Message = "safe to sign before penalty window"
		return eval, nil
	}
}

func clampPenaltyRatio(elapsed, window time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	if elapsed >= window {
		return 1
	}
	return float64(elapsed) / float64(window)
}
