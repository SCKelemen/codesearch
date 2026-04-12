package hybrid

import (
	"math"
	"testing"
)

func TestMinMaxNormalize(t *testing.T) {
	t.Parallel()

	got := MinMaxNormalize([]float64{10, 20, 30})
	want := []float64{0, 0.5, 1}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-12 {
			t.Fatalf("got[%d] = %f, want %f", i, got[i], want[i])
		}
	}
}

func TestMinMaxNormalizeAllEqual(t *testing.T) {
	t.Parallel()

	got := MinMaxNormalize([]float64{5, 5, 5})
	want := []float64{1, 1, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %f, want %f", i, got[i], want[i])
		}
	}
}

func TestZScoreNormalize(t *testing.T) {
	t.Parallel()

	got := ZScoreNormalize([]float64{1, 2, 3})
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}

	mean := 0.0
	for _, score := range got {
		mean += score
	}
	mean /= float64(len(got))
	if math.Abs(mean) > 1e-12 {
		t.Fatalf("mean = %f, want 0", mean)
	}

	variance := 0.0
	for _, score := range got {
		variance += score * score
	}
	variance /= float64(len(got))
	if math.Abs(variance-1) > 1e-12 {
		t.Fatalf("variance = %f, want 1", variance)
	}
}
