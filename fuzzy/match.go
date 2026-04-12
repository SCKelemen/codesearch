package fuzzy

// Options controls matching behavior.
type Options struct {
	CaseSensitive bool
	WithPositions bool
}

// MatchV1 performs O(n) greedy fuzzy match.
// Forward scan finds the first occurrence, backward scan shortens it.
func MatchV1(text []rune, pattern []rune, opts Options) Result {
	if len(pattern) == 0 {
		return Result{Score: 0}
	}
	n := len(text)
	m := len(pattern)

	// Forward pass: find first fuzzy occurrence
	pidx := 0
	sidx := -1
	eidx := -1
	for i := 0; i < n; i++ {
		tc := text[i]
		pc := pattern[pidx]
		if !opts.CaseSensitive {
			tc = toLower(tc)
			pc = toLower(pc)
		}
		if tc == pc {
			if sidx < 0 {
				sidx = i
			}
			pidx++
			if pidx == m {
				eidx = i + 1
				break
			}
		}
	}
	if eidx < 0 {
		return Result{Start: -1, End: -1}
	}

	// Backward pass: shorten the match
	pidx = m - 1
	for i := eidx - 1; i >= sidx; i-- {
		tc := text[i]
		pc := pattern[pidx]
		if !opts.CaseSensitive {
			tc = toLower(tc)
			pc = toLower(pc)
		}
		if tc == pc {
			pidx--
			if pidx < 0 {
				sidx = i
				break
			}
		}
	}

	score, pos := calculateScore(text, pattern, sidx, eidx, opts)
	return Result{Start: sidx, End: eidx, Score: score, Positions: pos}
}

// MatchV2 performs O(nm) modified Smith-Waterman fuzzy match.
// Examines all occurrences to find the highest-scoring alignment.
func MatchV2(text []rune, pattern []rune, opts Options) Result {
	m := len(pattern)
	if m == 0 {
		return Result{Score: 0}
	}
	n := len(text)
	if m > n {
		return Result{Start: -1, End: -1}
	}

	// Phase 1: Check feasibility and compute bonuses
	T := make([]rune, n)
	B := make([]int, n)
	F := make([]int, m) // first occurrence of each pattern char
	pidx := 0
	lastIdx := 0
	prevClass := charWhite
	for i := 0; i < n; i++ {
		char := text[i]
		if !opts.CaseSensitive {
			char = toLower(char)
		}
		T[i] = char
		class := classifyChar(text[i])
		B[i] = bonusFor(prevClass, class)
		prevClass = class

		if pidx < m {
			pc := pattern[pidx]
			if !opts.CaseSensitive {
				pc = toLower(pc)
			}
			if char == pc {
				F[pidx] = i
				pidx++
			}
		}
		lastChar := pattern[m-1]
		var prevMatchChar rune
		if pidx > 0 {
			prevMatchChar = pattern[pidx-1]
		}
		if !opts.CaseSensitive {
			lastChar = toLower(lastChar)
			prevMatchChar = toLower(prevMatchChar)
		}
		if char == lastChar || (pidx > 0 && char == prevMatchChar) {
			lastIdx = i
		}
	}
	if pidx != m {
		return Result{Start: -1, End: -1}
	}

	// Phase 2: Fill score matrix H
	f0 := F[0]
	width := lastIdx - f0 + 1
	H := make([]int, width*m)
	C := make([]int, width*m) // consecutive count

	// First row
	prevH := 0
	inGap := false
	pchar0 := pattern[0]
	if !opts.CaseSensitive {
		pchar0 = toLower(pchar0)
	}
	maxScore, maxScorePos := 0, 0
	for j := 0; j < width; j++ {
		col := f0 + j
		char := T[col]
		if char == pchar0 {
			score := ScoreMatch + B[col]*BonusFirstCharMultiplier
			H[j] = score
			C[j] = 1
			if m == 1 && score > maxScore {
				maxScore, maxScorePos = score, col
			}
			inGap = false
			prevH = score
		} else {
			if inGap {
				H[j] = max(prevH+ScoreGapExtension, 0)
			} else {
				H[j] = max(prevH+ScoreGapStart, 0)
			}
			C[j] = 0
			inGap = true
			prevH = H[j]
		}
	}

	// Subsequent rows
	for i := 1; i < m; i++ {
		row := i * width
		pchar := pattern[i]
		if !opts.CaseSensitive {
			pchar = toLower(pchar)
		}
		fi := F[i]
		inGap = false
		for j := 0; j < width; j++ {
			col := f0 + j
			if col < fi {
				H[row+j] = 0
				C[row+j] = 0
				continue
			}

			char := T[col]
			var s1, s2 int
			var consecutive int

			// Gap score (from left)
			if j > 0 {
				if inGap {
					s2 = H[row+j-1] + ScoreGapExtension
				} else {
					s2 = H[row+j-1] + ScoreGapStart
				}
			}

			// Match score (from diagonal)
			if char == pchar && j > 0 {
				s1 = H[row-width+j-1] + ScoreMatch
				b := B[col]
				consecutive = C[row-width+j-1] + 1
				if consecutive > 1 {
					fb := B[col-consecutive+1]
					if b >= BonusBoundary && b > fb {
						consecutive = 1
					} else {
						b = max(b, BonusConsecutive, fb)
					}
				}
				if s1+b < s2 {
					s1 += B[col]
					consecutive = 0
				} else {
					s1 += b
				}
			}

			C[row+j] = consecutive
			inGap = s1 < s2
			score := max(s1, s2, 0)
			if i == m-1 && score > maxScore {
				maxScore, maxScorePos = score, col
			}
			H[row+j] = score
		}
	}

	if maxScore == 0 {
		return Result{Start: -1, End: -1}
	}

	// Phase 3: Backtrace for positions
	var positions []int
	startPos := f0
	if opts.WithPositions {
		positions = make([]int, 0, m)
		i := m - 1
		j := maxScorePos
		preferMatch := true
		for i >= 0 && j >= f0 {
			row := i * width
			jj := j - f0
			s := H[row+jj]
			var s1, s2 int
			if i > 0 && jj > 0 {
				s1 = H[row-width+jj-1]
			}
			if jj > 0 {
				s2 = H[row+jj-1]
			}
			if s > s1 && (s > s2 || (s == s2 && preferMatch)) {
				positions = append(positions, j)
				if i == 0 {
					startPos = j
					break
				}
				i--
			}
			preferMatch = C[row+jj] > 1 || (row+width+jj+1 < len(C) && C[row+width+jj+1] > 0)
			j--
		}
		// Reverse positions
		for l, r := 0, len(positions)-1; l < r; l, r = l+1, r-1 {
			positions[l], positions[r] = positions[r], positions[l]
		}
	}

	return Result{Start: startPos, End: maxScorePos + 1, Score: maxScore, Positions: positions}
}

// Match runs V2 (optimal) by default. Use MatchV1 for speed over quality.
func Match(text string, pattern string, opts Options) Result {
	return MatchV2([]rune(text), []rune(pattern), opts)
}

func calculateScore(text, pattern []rune, sidx, eidx int, opts Options) (int, []int) {
	pidx := 0
	score := 0
	inGap := false
	consecutive := 0
	firstBonus := 0
	prevClass := charWhite
	if sidx > 0 {
		prevClass = classifyChar(text[sidx-1])
	}
	var positions []int
	if opts.WithPositions {
		positions = make([]int, 0, len(pattern))
	}

	for i := sidx; i < eidx; i++ {
		char := text[i]
		class := classifyChar(char)
		if !opts.CaseSensitive {
			char = toLower(char)
		}
		pc := pattern[pidx]
		if !opts.CaseSensitive {
			pc = toLower(pc)
		}
		if char == pc {
			if opts.WithPositions {
				positions = append(positions, i)
			}
			score += ScoreMatch
			bonus := bonusFor(prevClass, class)
			if consecutive == 0 {
				firstBonus = bonus
			} else {
				if bonus >= BonusBoundary && bonus > firstBonus {
					firstBonus = bonus
				}
				bonus = max(bonus, BonusConsecutive, firstBonus)
			}
			if pidx == 0 {
				score += bonus * BonusFirstCharMultiplier
			} else {
				score += bonus
			}
			inGap = false
			consecutive++
			pidx++
		} else {
			if inGap {
				score += ScoreGapExtension
			} else {
				score += ScoreGapStart
			}
			inGap = true
			consecutive = 0
			firstBonus = 0
		}
		prevClass = class
	}
	return score, positions
}
