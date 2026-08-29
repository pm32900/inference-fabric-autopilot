package recommender

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultThresholdsAreValid(t *testing.T) {
	if err := DefaultThresholds().Validate(); err != nil {
		t.Fatalf("the shipped defaults do not validate: %v", err)
	}
}

func TestThresholdValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Thresholds)
		wantErr string
	}{
		{
			// With the bands inverted, a workload can be simultaneously "idle"
			// and "saturated" and both queue rules fire on it, which is worse
			// than either firing alone.
			name:    "GPU bands inverted",
			mutate:  func(th *Thresholds) { th.GPUUtilLowPct, th.GPUUtilHighPct = 90, 30 },
			wantErr: "gpu_util_low_pct",
		},
		{
			name:    "GPU bands equal",
			mutate:  func(th *Thresholds) { th.GPUUtilLowPct, th.GPUUtilHighPct = 50, 50 },
			wantErr: "gpu_util_low_pct",
		},
		{
			name:    "percentage above 100",
			mutate:  func(th *Thresholds) { th.KVCacheHighPct = 150 },
			wantErr: "kv_cache_high_pct",
		},
		{
			name:    "negative percentage",
			mutate:  func(th *Thresholds) { th.ErrorRatePct = -1 },
			wantErr: "error_rate_pct",
		},
		{
			name:    "negative latency threshold",
			mutate:  func(th *Thresholds) { th.TTFTP95Ms = -5 },
			wantErr: "ttft_p95_ms",
		},
		{
			// p99 cannot be below p95, so a ratio under 1 describes an
			// impossible condition and the rule could never fire.
			name:    "tail ratio below one",
			mutate:  func(th *Thresholds) { th.TailRatioP99P95 = 0.5 },
			wantErr: "tail_ratio_p99_p95",
		},
		{
			name:    "stale_after not set",
			mutate:  func(th *Thresholds) { th.StaleAfter = 0 },
			wantErr: "stale_after",
		},
		{
			name:    "negative sustain window",
			mutate:  func(th *Thresholds) { th.SustainFor = -time.Second },
			wantErr: "sustain_for",
		},
		{
			// Zero is legal and means "evaluate the latest sample only".
			name:   "zero sustain window is allowed",
			mutate: func(th *Thresholds) { th.SustainFor = 0 },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			th := DefaultThresholds()
			tc.mutate(&th)
			err := th.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error mentioning %q, got none", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not mention the offending field %q", err, tc.wantErr)
			}
		})
	}
}
