// Package fuzzy implements the fzf fuzzy matching algorithm.
//
// It provides two algorithms:
//   - V1: O(n) greedy forward/backward scan. Fast, may miss optimal alignment.
//   - V2: O(nm) modified Smith-Waterman. Finds highest-scoring alignment.
//
// Scoring favors matches at word boundaries, camelCase transitions,
// and consecutive character runs. The first pattern character gets
// extra weight at special positions.
//
// Reference: https://github.com/junegunn/fzf/blob/master/src/algo/algo.go
package fuzzy

import (
	"strings"
	"unicode"
)

// Result holds the outcome of a fuzzy match.
type Result struct {
	Start     int   // start index in text (inclusive)
	End       int   // end index in text (exclusive)
	Score     int   // match quality score (higher is better)
	Positions []int // matched character positions (optional)
}

// Scheme controls boundary bonus tuning for different use cases.
type Scheme int

const (
	SchemeDefault Scheme = iota
	SchemePath           // boost path separators
	SchemeHistory        // equal weight to all boundaries
)

// Scoring constants from fzf.
const (
	ScoreMatch        = 16
	ScoreGapStart     = -3
	ScoreGapExtension = -1

	BonusBoundary            = ScoreMatch / 2
	BonusNonWord             = ScoreMatch / 2
	BonusCamel123            = BonusBoundary + ScoreGapExtension
	BonusConsecutive         = -(ScoreGapStart + ScoreGapExtension)
	BonusFirstCharMultiplier = 2
)

// charClass categorizes characters for bonus calculation.
type charClass int

const (
	charWhite charClass = iota
	charNonWord
	charDelimiter
	charLower
	charUpper
	charLetter
	charNumber
)

const delimiterChars = "/,:;|"
const whiteChars = " \t\n\v\f\r"

func classifyChar(r rune) charClass {
	if r >= 'a' && r <= 'z' {
		return charLower
	}
	if r >= 'A' && r <= 'Z' {
		return charUpper
	}
	if r >= '0' && r <= '9' {
		return charNumber
	}
	if strings.ContainsRune(whiteChars, r) {
		return charWhite
	}
	if strings.ContainsRune(delimiterChars, r) {
		return charDelimiter
	}
	if unicode.IsLower(r) {
		return charLower
	}
	if unicode.IsUpper(r) {
		return charUpper
	}
	if unicode.IsNumber(r) {
		return charNumber
	}
	if unicode.IsLetter(r) {
		return charLetter
	}
	if unicode.IsSpace(r) {
		return charWhite
	}
	return charNonWord
}

func bonusFor(prev, cur charClass) int {
	if cur > charNonWord {
		switch prev {
		case charWhite:
			return BonusBoundary + 2
		case charDelimiter:
			return BonusBoundary + 1
		case charNonWord:
			return BonusBoundary
		}
	}
	if prev == charLower && cur == charUpper {
		return BonusCamel123
	}
	if prev != charNumber && cur == charNumber {
		return BonusCamel123
	}
	if cur == charNonWord || cur == charDelimiter {
		return BonusNonWord
	}
	if cur == charWhite {
		return BonusBoundary + 2
	}
	return 0
}

func toLower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + 32
	}
	if r > unicode.MaxASCII {
		return unicode.ToLower(r)
	}
	return r
}
