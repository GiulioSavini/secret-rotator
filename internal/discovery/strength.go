package discovery

import (
	"math"
	"strings"
	"unicode"
)

// Strength represents a password strength level.
type Strength int

const (
	StrengthWeak   Strength = 0
	StrengthFair   Strength = 1
	StrengthGood   Strength = 2
	StrengthStrong Strength = 3
)

// String returns the human-readable strength label.
func (s Strength) String() string {
	switch s {
	case StrengthWeak:
		return "weak"
	case StrengthFair:
		return "fair"
	case StrengthGood:
		return "good"
	case StrengthStrong:
		return "strong"
	default:
		return "unknown"
	}
}

// StrengthResult holds the detailed result of a password strength audit.
type StrengthResult struct {
	Score   Strength
	Entropy float64
	Length  int
	Issues  []string
}

// AuditStrength analyzes a password value and returns its strength assessment.
func AuditStrength(value string) StrengthResult {
	r := StrengthResult{
		Length: len(value),
	}

	// Empty password is always weak.
	if value == "" {
		r.Score = StrengthWeak
		r.Issues = append(r.Issues, "empty password")
		return r
	}

	// Check for known defaults (case-insensitive).
	if CommonDefaults[strings.ToLower(value)] {
		r.Score = StrengthWeak
		r.Issues = append(r.Issues, "common default password")
		return r
	}

	// Count character classes.
	var hasLower, hasUpper, hasDigit, hasSymbol bool
	for _, ch := range value {
		switch {
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsDigit(ch):
			hasDigit = true
		default:
			hasSymbol = true
		}
	}

	classCount := 0
	charsetSize := 0
	if hasLower {
		classCount++
		charsetSize += 26
	}
	if hasUpper {
		classCount++
		charsetSize += 26
	}
	if hasDigit {
		classCount++
		charsetSize += 10
	}
	if hasSymbol {
		classCount++
		charsetSize += 32
	}

	// Calculate entropy: length * log2(charset_size).
	if charsetSize > 0 {
		r.Entropy = float64(r.Length) * math.Log2(float64(charsetSize))
	}

	// Length-based score.
	var lengthScore Strength
	switch {
	case r.Length < 8:
		lengthScore = StrengthWeak
	case r.Length < 16:
		lengthScore = StrengthFair
	case r.Length < 32:
		lengthScore = StrengthGood
	default:
		lengthScore = StrengthStrong
	}

	// Entropy-based score.
	var entropyScore Strength
	switch {
	case r.Entropy < 28:
		entropyScore = StrengthWeak
	case r.Entropy < 36:
		entropyScore = StrengthFair
	case r.Entropy < 60:
		entropyScore = StrengthGood
	default:
		entropyScore = StrengthStrong
	}

	// Final score is the minimum of length and entropy scores.
	r.Score = lengthScore
	if entropyScore < r.Score {
		r.Score = entropyScore
	}

	// Boost: if 4 character classes and both scores >= Good, allow Strong.
	// Penalty: if < 2 classes, cap at Fair and note issue.
	if classCount < 2 {
		r.Issues = append(r.Issues, "limited character variety")
		if r.Score > StrengthFair {
			r.Score = StrengthFair
		}
	}

	// Length issues.
	if r.Length < 8 {
		r.Issues = append(r.Issues, "length < 8")
	}

	return r
}
