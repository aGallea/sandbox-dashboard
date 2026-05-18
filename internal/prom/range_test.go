package prom

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseRange_KnownValues(t *testing.T) {
	now := time.Now()
	cases := map[string]struct {
		duration time.Duration
		step     time.Duration
	}{
		"15m": {15 * time.Minute, 15 * time.Second},
		"1h":  {time.Hour, 30 * time.Second},
		"6h":  {6 * time.Hour, 2 * time.Minute},
		"24h": {24 * time.Hour, 5 * time.Minute},
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			r, err := ParseRange(in, now)
			require.NoError(t, err)
			require.WithinDuration(t, now, r.End, time.Second)
			require.WithinDuration(t, now.Add(-want.duration), r.Start, time.Second)
			require.Equal(t, want.step, r.Step)
		})
	}
}

func TestParseRange_DefaultsWhenEmpty(t *testing.T) {
	now := time.Now()
	r, err := ParseRange("", now)
	require.NoError(t, err)
	require.Equal(t, time.Hour, r.End.Sub(r.Start))
}

func TestParseRange_Rejects_Unknown(t *testing.T) {
	_, err := ParseRange("99m", time.Now())
	require.Error(t, err)
}
