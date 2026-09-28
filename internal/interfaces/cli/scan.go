package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/giulio/secret-rotator/internal/application"
	"github.com/giulio/secret-rotator/internal/domain"
	"github.com/giulio/secret-rotator/internal/infrastructure/envstore"
)

// NewScanCmd creates the scan subcommand.
func NewScanCmd() *cobra.Command {
	var dir string

	cmd := &cobra.Command{
		Use:   "scan [directory]",
		Short: "Discover and audit secrets in .env files",
		Long: `Scan finds the .env files in a directory, identifies which of their entries
are credentials, and reports how strong each one is.

It changes nothing and needs no configuration, so it is safe to point at an
unfamiliar project.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && dir == "." {
				dir = args[0]
			}
			return runScan(cmd, dir)
		},
	}

	cmd.Flags().StringVar(&dir, "dir", ".", "directory to scan for .env files")

	return cmd
}

func runScan(cmd *cobra.Command, dir string) error {
	paths, err := candidateEnvFiles(dir)
	if err != nil {
		return err
	}

	// Include the files the configuration names, which may live outside dir.
	for _, secret := range AppConfig.Secrets() {
		for _, p := range secret.EnvFiles() {
			paths = appendUniquePath(paths, p)
		}
	}

	if len(paths) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No .env files found in %s.\n", dir)
		return nil
	}

	report, err := application.NewScanSecrets(envstore.NewStore()).Execute(paths)
	if err != nil {
		return err
	}

	for _, u := range report.Unreadable {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not read %s: %s\n", u.Path, u.Reason)
	}

	if len(report.Secrets) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No secrets discovered.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "SECRET\tTYPE\tSTRENGTH\tSOURCE\tISSUES")

	for _, s := range report.Secrets {
		category := s.Category
		strength := s.Strength.Score.String()
		issues := "-"

		if s.FileReferenced {
			category += " (file)"
			strength = "n/a"
		} else if len(s.Strength.Issues) > 0 {
			issues = strings.Join(s.Strength.Issues, ", ")
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Key, category, strength, s.Source, issues)
	}
	w.Flush()

	fmt.Fprintf(cmd.OutOrStdout(), "\nFound %d secrets (%d weak, %d fair, %d good, %d strong)\n",
		report.Total(),
		report.Counts[domain.StrengthWeak],
		report.Counts[domain.StrengthFair],
		report.Counts[domain.StrengthGood],
		report.Counts[domain.StrengthStrong],
	)
	return nil
}

// candidateEnvFiles lists the .env files in a directory, in a stable order.
func candidateEnvFiles(dir string) ([]domain.FilePath, error) {
	matches, err := filepath.Glob(filepath.Join(dir, ".env*"))
	if err != nil {
		return nil, fmt.Errorf("scanning %s: %w", dir, err)
	}
	sort.Strings(matches)

	paths := make([]domain.FilePath, 0, len(matches))
	for _, m := range matches {
		paths = append(paths, domain.FilePath(m))
	}
	return paths, nil
}

// appendUniquePath adds a path if it is not already present.
func appendUniquePath(paths []domain.FilePath, p domain.FilePath) []domain.FilePath {
	for _, existing := range paths {
		if existing == p {
			return paths
		}
	}
	return append(paths, p)
}
