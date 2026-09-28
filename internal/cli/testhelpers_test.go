package cli

import (
	"path/filepath"
	"testing"
)

// useDataDir points the history store at dir for the duration of a test.
// Subcommands read the data directory from the root command's persistent
// flag, which is not present when a subcommand is constructed in isolation.
func useDataDir(t *testing.T, dir string) {
	t.Helper()
	prev := dataDirFlag
	dataDirFlag = dir
	t.Cleanup(func() { dataDirFlag = prev })
}

// useHistoryIn points the history store at the ".rotator" subdirectory of
// dir, matching the layout the CLI creates at runtime.
func useHistoryIn(t *testing.T, dir string) {
	t.Helper()
	useDataDir(t, filepath.Join(dir, ".rotator"))
}
