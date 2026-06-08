package db

import (
	"testing"
	"time"
)

func TestNextDelay(t *testing.T) {
	cases := []struct {
		cur, maxDelay, want time.Duration
	}{
		{1 * time.Second, 8 * time.Second, 2 * time.Second},
		{2 * time.Second, 8 * time.Second, 4 * time.Second},
		{4 * time.Second, 8 * time.Second, 8 * time.Second},
		{8 * time.Second, 8 * time.Second, 8 * time.Second},  // capped
		{10 * time.Second, 8 * time.Second, 8 * time.Second}, // never exceeds cap
	}
	for _, c := range cases {
		if got := nextDelay(c.cur, c.maxDelay); got != c.want {
			t.Errorf("nextDelay(%s, %s) = %s, want %s", c.cur, c.maxDelay, got, c.want)
		}
	}
}

func TestWithJitter_Bounds(t *testing.T) {
	d := 1 * time.Second
	ratio := 0.2
	min := time.Duration(float64(d) * (1 - ratio))
	maxD := time.Duration(float64(d) * (1 + ratio))
	// Run a bunch of trials and assert the distribution stays inside the band.
	for range 1000 {
		got := withJitter(d, ratio)
		if got < min || got > maxD {
			t.Fatalf("withJitter(%s, %.2f) = %s, want in [%s, %s]", d, ratio, got, min, maxD)
		}
	}
}

func TestWithJitter_ZeroRatio(t *testing.T) {
	d := 1 * time.Second
	if got := withJitter(d, 0); got != d {
		t.Errorf("zero ratio should be identity; got %s, want %s", got, d)
	}
}

func TestRetryOptions_Defaults(t *testing.T) {
	var o RetryOptions
	o.setDefaults()
	if o.MaxAttempts != 4 {
		t.Errorf("MaxAttempts = %d, want 4", o.MaxAttempts)
	}
	if o.InitialDelay != 1*time.Second {
		t.Errorf("InitialDelay = %s, want 1s", o.InitialDelay)
	}
	if o.MaxDelay != 8*time.Second {
		t.Errorf("MaxDelay = %s, want 8s", o.MaxDelay)
	}
	if o.JitterRatio != 0.2 {
		t.Errorf("JitterRatio = %f, want 0.2", o.JitterRatio)
	}
}

func TestRetryOptions_RespectsCallerValues(t *testing.T) {
	o := RetryOptions{
		MaxAttempts:  7,
		InitialDelay: 250 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		JitterRatio:  0.05,
	}
	o.setDefaults()
	if o.MaxAttempts != 7 || o.InitialDelay != 250*time.Millisecond ||
		o.MaxDelay != 30*time.Second || o.JitterRatio != 0.05 {
		t.Errorf("setDefaults overwrote caller values: %+v", o)
	}
}
