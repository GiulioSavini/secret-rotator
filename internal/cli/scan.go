package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/giulio/secret-rotator/internal/discovery"
	"github.com/giulio/secret-rotator/internal/envfile"
	"github.com/spf13/cobra"
)

// NewScanCmd creates the scan subcommand.
func NewScanCmd() *cobra.Command {
	var dir string

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Discover and audit secrets in .env files",
		Long:  `Scan discovers .env files, identifies secrets, and reports their strength and issues.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(cmd, dir)
		},
	}

	cmd.Flags().StringVar(&dir, "dir", ".", "directory to scan for .env files")

	return cmd
}

func runScan(cmd *cobra.Command, dir string) error {
	// Collect env file paths: glob .env* in the directory.
	pattern := filepath.Join(dir, ".env*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("scanning directory: %w", err)
	}

	// Also check config for explicit env_file/env_files paths.
	if AppConfig != nil {
		for _, sc := range AppConfig.Secrets {
			if sc.EnvFile != "" {
				matches = appendUnique(matches, sc.EnvFile)
			}
			for _, ef := range sc.EnvFiles {
				matches = appendUnique(matches, ef)
			}
		}
	}

	// Read all env files.
	var files []*envfile.EnvFile
	for _, path := range matches {
		ef, readErr := envfile.Read(path)
		if readErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not read %s: %v\n", path, readErr)
			continue
		}
		files = append(files, ef)
	}

	// Scan for secrets.
	scanner := discovery.NewScanner()
	secrets := scanner.ScanFiles(files)

	if len(secrets) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No secrets discovered.")
		return nil
	}

	// Output table.
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "SECRET\tTYPE\tSTRENGTH\tSOURCE\tISSUES")

	var weakCount, fairCount, goodCount, strongCount int

	for _, s := range secrets {
		typeStr := s.Type
		strengthStr := s.Strength.Score.String()
		issuesStr := "-"

		if s.FileReferenced {
			typeStr += " (file)"
			strengthStr = "n/a"
		} else {
			switch s.Strength.Score {
			case discovery.StrengthWeak:
				weakCount++
			case discovery.StrengthFair:
				fairCount++
			case discovery.StrengthGood:
				goodCount++
			case discovery.StrengthStrong:
				strongCount++
			}
		}

		if len(s.Strength.Issues) > 0 && !s.FileReferenced {
			issuesStr = strings.Join(s.Strength.Issues, ", ")
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Key, typeStr, strengthStr, s.Source, issuesStr)
	}
	w.Flush()

	total := weakCount + fairCount + goodCount + strongCount
	fmt.Fprintf(cmd.OutOrStdout(), "\nFound %d secrets (%d weak, %d fair, %d good, %d strong)\n",
		total, weakCount, fairCount, goodCount, strongCount)

	return nil
}

func appendUnique(slice []string, val string) []string {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}
