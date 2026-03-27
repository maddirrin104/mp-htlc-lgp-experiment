package policy

import (
	"testing"
	"time"

	"mphtlc-lgp-mpc/internal/config"
)

func TestEvaluatePenaltyWindow(t *testing.T) {
	cfg := config.PenaltyConfig{
		TimeLockSeconds:      1800,
		PenaltyWindowSeconds: 600,
		ExpectedSignLatency:  4 * time.Second,
		SafetyMargin:         2 * time.Second,
		WarnOnly:             true,
	}
	start := time.Unix(0, 0)

	t.Run("allow before penalty window", func(t *testing.T) {
		eval, err := EvaluatePenaltyWindow(cfg, start, start.Add(10*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if eval.Decision != DecisionAllow {
			t.Fatalf("expected allow, got %s", eval.Decision)
		}
	})

	t.Run("warn inside penalty window", func(t *testing.T) {
		eval, err := EvaluatePenaltyWindow(cfg, start, start.Add(21*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if eval.Decision != DecisionWarn {
			t.Fatalf("expected warn, got %s", eval.Decision)
		}
		if eval.ProjectedPenalty <= 0 {
			t.Fatalf("expected positive penalty ratio")
		}
	})

	t.Run("reject after timelock", func(t *testing.T) {
		eval, err := EvaluatePenaltyWindow(cfg, start, start.Add(31*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if eval.Decision != DecisionReject {
			t.Fatalf("expected reject, got %s", eval.Decision)
		}
		if eval.ProjectedPenalty != 1 {
			t.Fatalf("expected full penalty ratio, got %f", eval.ProjectedPenalty)
		}
	})
}
