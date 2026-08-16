package downloader

import (
	"testing"
	"time"
)

func TestEstimateTimeout(t *testing.T) {
	cases := []struct {
		name     string
		duration time.Duration
		min, max time.Duration // acceptable range
	}{
		{"unknown duration falls back to the default", 0, defaultDownloadTimeout, defaultDownloadTimeout},
		{"very short video is clamped to the floor", 10 * time.Second, minDownloadTimeout, minDownloadTimeout},
		{"typical video scales with duration", 20 * time.Minute, 55 * time.Minute, 70 * time.Minute},
		{"very long video is clamped to the ceiling", 8 * time.Hour, maxDownloadTimeout, maxDownloadTimeout},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EstimateTimeout(tc.duration)
			if got < tc.min || got > tc.max {
				t.Errorf("EstimateTimeout(%v) = %v, want between %v and %v", tc.duration, got, tc.min, tc.max)
			}
		})
	}
}

func TestEstimateTimeoutNeverExceedsBounds(t *testing.T) {
	for d := time.Duration(0); d <= 24*time.Hour; d += 37 * time.Minute {
		got := EstimateTimeout(d)
		if got < minDownloadTimeout || got > maxDownloadTimeout {
			t.Fatalf("EstimateTimeout(%v) = %v, outside [%v, %v]", d, got, minDownloadTimeout, maxDownloadTimeout)
		}
	}
}
