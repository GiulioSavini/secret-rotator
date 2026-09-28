package application

import (
	"fmt"

	"github.com/giulio/secret-rotator/internal/domain"
)

// ScanSecrets audits the credentials found in a set of .env files.
type ScanSecrets struct {
	env     domain.EnvStore
	scanner *domain.Scanner
}

// NewScanSecrets builds the scanning use case.
func NewScanSecrets(env domain.EnvStore) *ScanSecrets {
	return &ScanSecrets{env: env, scanner: domain.NewScanner()}
}

// ScanReport is the outcome of a scan: the secrets found, a count per strength
// level, and the files that could not be read.
type ScanReport struct {
	Secrets    []domain.DiscoveredSecret
	Unreadable []UnreadableFile
	Counts     map[domain.Strength]int
}

// UnreadableFile records a file that was listed but could not be parsed.
type UnreadableFile struct {
	Path   domain.FilePath
	Reason string
}

// Execute reads every path and audits what it finds. A file that cannot be
// read is reported rather than failing the scan, because scanning is a
// discovery tool run against directories the user may not fully control.
func (uc *ScanSecrets) Execute(paths []domain.FilePath) (*ScanReport, error) {
	report := &ScanReport{Counts: map[domain.Strength]int{}}

	docs := make([]domain.EnvDocument, 0, len(paths))
	for _, path := range paths {
		doc, err := uc.env.Open(path)
		if err != nil {
			report.Unreadable = append(report.Unreadable, UnreadableFile{
				Path:   path,
				Reason: err.Error(),
			})
			continue
		}
		docs = append(docs, doc)
	}

	report.Secrets = uc.scanner.ScanDocuments(docs)
	for _, s := range report.Secrets {
		if s.FileReferenced {
			continue
		}
		report.Counts[s.Strength.Score]++
	}

	if len(docs) == 0 && len(paths) > 0 {
		return report, fmt.Errorf("none of the %d candidate files could be read", len(paths))
	}
	return report, nil
}

// Total returns the number of audited (non file-referenced) secrets.
func (r *ScanReport) Total() int {
	total := 0
	for _, n := range r.Counts {
		total += n
	}
	return total
}
