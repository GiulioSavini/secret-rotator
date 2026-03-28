package discovery

import "github.com/giulio/secret-rotator/internal/envfile"

// Scanner identifies secrets in environment files by key naming patterns.
type Scanner struct{}

// DiscoveredSecret represents a secret found during scanning.
type DiscoveredSecret struct {
	Key            string
	Type           string
	Source         string
	Value          string
	Strength       StrengthResult
	FileReferenced bool
}

// NewScanner creates a Scanner with default patterns.
func NewScanner() *Scanner {
	return &Scanner{}
}

func (s *Scanner) classifyKey(key string) (string, bool) {
	return "", false
}

// ScanFile scans an EnvFile and returns discovered secrets.
func (s *Scanner) ScanFile(ef *envfile.EnvFile) []DiscoveredSecret {
	return nil
}

// ScanFiles scans multiple EnvFiles and returns all discovered secrets.
func (s *Scanner) ScanFiles(files []*envfile.EnvFile) []DiscoveredSecret {
	return nil
}
