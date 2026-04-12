package hybrid

import "math"

// Normalizer rescales a set of scores while preserving their order.
type Normalizer func([]float64) []float64

// MinMaxNormalize scales scores into the inclusive range [0, 1].
//
// When all scores are equal, every result receives 1 so the backend still
// contributes uniformly during fusion.
func MinMaxNormalize(scores []float64) []float64 {
	if len(scores) == 0 {
		return nil
	}

	minScore := scores[0]
	maxScore := scores[0]
	for _, score := range scores[1:] {
		if score < minScore {
			minScore = score
		}
		if score > maxScore {
			maxScore = score
		}
	}

	out := make([]float64, len(scores))
	if maxScore == minScore {
		for i := range out {
			out[i] = 1
		}
		return out
	}

	scale := maxScore - minScore
	for i, score := range scores {
		out[i] = (score - minScore) / scale
	}
	return out
}

// ZScoreNormalize standardizes scores to mean 0 and standard deviation 1.
//
// When all scores are equal, the result is a slice of zeros.
func ZScoreNormalize(scores []float64) []float64 {
	if len(scores) == 0 {
		return nil
	}

	mean := 0.0
	for _, score := range scores {
		mean += score
	}
	mean /= float64(len(scores))

	variance := 0.0
	for _, score := range scores {
		delta := score - mean
		variance += delta * delta
	}
	variance /= float64(len(scores))

	stddev := math.Sqrt(variance)
	out := make([]float64, len(scores))
	if stddev == 0 {
		return out
	}

	for i, score := range scores {
		out[i] = (score - mean) / stddev
	}
	return out
}
