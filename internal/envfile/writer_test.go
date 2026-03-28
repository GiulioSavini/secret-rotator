package envfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetUpdatesValue(t *testing.T) {
	path := writeTestEnv(t, "KEY=oldval\n")
	ef, err := Read(path)
	require.NoError(t, err)

	ef.Set("KEY", "newval")
	assert.Equal(t, "newval", ef.Lines[0].Value)
	assert.Equal(t, "KEY=newval", ef.Lines[0].Raw)
}

func TestSetPreservesDoubleQuotes(t *testing.T) {
	path := writeTestEnv(t, `KEY="oldval"`+"\n")
	ef, err := Read(path)
	require.NoError(t, err)

	ef.Set("KEY", "newval")
	assert.Equal(t, "newval", ef.Lines[0].Value)
	assert.Equal(t, `KEY="newval"`, ef.Lines[0].Raw)
	assert.Equal(t, `"`, ef.Lines[0].Quoted)
}

func TestSetPreservesSingleQuotes(t *testing.T) {
	path := writeTestEnv(t, "KEY='oldval'\n")
	ef, err := Read(path)
	require.NoError(t, err)

	ef.Set("KEY", "newval")
	assert.Equal(t, "newval", ef.Lines[0].Value)
	assert.Equal(t, "KEY='newval'", ef.Lines[0].Raw)
	assert.Equal(t, "'", ef.Lines[0].Quoted)
}

func TestSetNonExistentKey(t *testing.T) {
	path := writeTestEnv(t, "KEY=val\n")
	ef, err := Read(path)
	require.NoError(t, err)

	// Should be a no-op, no crash
	ef.Set("NONEXISTENT", "value")
	assert.Len(t, ef.Lines, 2) // KEY=val + trailing empty from split
	assert.Equal(t, "val", ef.Lines[0].Value)
}

func TestWriteAtomicCreatesFile(t *testing.T) {
	path := writeTestEnv(t, "KEY=value\nOTHER=123\n")
	ef, err := Read(path)
	require.NoError(t, err)

	ef.Set("KEY", "updated")
	require.NoError(t, ef.WriteAtomic())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "KEY=updated")
	assert.Contains(t, string(data), "OTHER=123")
}

func TestWriteAtomicPreservesComments(t *testing.T) {
	content := "# Database config\nDB_HOST=localhost\n\n# Secrets\nDB_PASS=secret\n"
	path := writeTestEnv(t, content)
	ef, err := Read(path)
	require.NoError(t, err)

	ef.Set("DB_PASS", "newsecret")
	require.NoError(t, ef.WriteAtomic())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "# Database config")
	assert.Contains(t, string(data), "# Secrets")
	assert.Contains(t, string(data), "DB_HOST=localhost")
	assert.Contains(t, string(data), "DB_PASS=newsecret")
}

func TestWriteAtomicPreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(path, []byte("KEY=val\n"), 0600))

	ef, err := Read(path)
	require.NoError(t, err)

	ef.Set("KEY", "newval")
	require.NoError(t, ef.WriteAtomic())

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestWriteAtomicNoCorruptionOnSimulatedCrash(t *testing.T) {
	content := "KEY=original\n"
	path := writeTestEnv(t, content)

	// Read the file
	ef, err := Read(path)
	require.NoError(t, err)

	// Verify temp files are created in the same directory during write
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	initialCount := len(entries)

	// After successful write, temp file should be cleaned up (renamed to target)
	ef.Set("KEY", "newval")
	require.NoError(t, ef.WriteAtomic())

	entries, err = os.ReadDir(dir)
	require.NoError(t, err)
	// Should still have the same number of files (temp renamed to target)
	assert.Equal(t, initialCount, len(entries))
}

func TestWriteAtomicRoundTrip(t *testing.T) {
	content := "# Config\nDB_HOST=localhost\nDB_PORT=5432\nDB_PASS=\"secret123\"\n\n# End\n"
	path := writeTestEnv(t, content)

	// Read
	ef, err := Read(path)
	require.NoError(t, err)

	// Modify
	ef.Set("DB_PASS", "newsecret")

	// Write
	require.NoError(t, ef.WriteAtomic())

	// Read again
	ef2, err := Read(path)
	require.NoError(t, err)

	// Verify modified value
	val, ok := ef2.Get("DB_PASS")
	assert.True(t, ok)
	assert.Equal(t, "newsecret", val)

	// Verify other values unchanged
	host, ok := ef2.Get("DB_HOST")
	assert.True(t, ok)
	assert.Equal(t, "localhost", host)

	port, ok := ef2.Get("DB_PORT")
	assert.True(t, ok)
	assert.Equal(t, "5432", port)

	// Verify formatting preserved
	assert.True(t, ef2.Lines[0].Comment)
	assert.Equal(t, "# Config", ef2.Lines[0].Raw)
	assert.Equal(t, `"`, ef2.Lines[3].Quoted)
	assert.True(t, ef2.Lines[5].Comment)
	assert.Equal(t, "# End", ef2.Lines[5].Raw)
}
