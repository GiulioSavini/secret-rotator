package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreAppendAndList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	passphrase := []byte("test-passphrase")

	store := NewStore(path, passphrase)

	entries := []HistoryEntry{
		{SecretName: "DB_PASSWORD", RotatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC), OldValue: "old1", NewHash: "hash1", Status: "success", Details: "rotated via mysql"},
		{SecretName: "REDIS_PASS", RotatedAt: time.Date(2026, 3, 28, 10, 1, 0, 0, time.UTC), OldValue: "old2", NewHash: "hash2", Status: "success", Details: "rotated via redis"},
		{SecretName: "API_KEY", RotatedAt: time.Date(2026, 3, 28, 10, 2, 0, 0, time.UTC), OldValue: "old3", NewHash: "hash3", Status: "failed", Details: "connection refused"},
	}

	for _, e := range entries {
		require.NoError(t, store.Append(e))
	}

	listed, err := store.List()
	require.NoError(t, err)
	require.Len(t, listed, 3)

	assert.Equal(t, "DB_PASSWORD", listed[0].SecretName)
	assert.Equal(t, "REDIS_PASS", listed[1].SecretName)
	assert.Equal(t, "API_KEY", listed[2].SecretName)
	assert.Equal(t, "failed", listed[2].Status)
	assert.Equal(t, "old1", listed[0].OldValue)
}

func TestStoreEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	passphrase := []byte("test-passphrase")

	store := NewStore(path, passphrase)

	entries, err := store.List()
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestStoreCorruptedEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	passphrase := []byte("test-passphrase")

	store := NewStore(path, passphrase)

	// Add a valid entry
	require.NoError(t, store.Append(HistoryEntry{
		SecretName: "GOOD_SECRET",
		RotatedAt:  time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		Status:     "success",
	}))

	// Verify file was written
	_, err = os.ReadFile(path)
	require.NoError(t, err)

	// Create second store instance to test append
	store2 := NewStore(path, passphrase)

	// Add another valid entry
	require.NoError(t, store2.Append(HistoryEntry{
		SecretName: "ANOTHER_GOOD",
		RotatedAt:  time.Date(2026, 3, 28, 11, 0, 0, 0, time.UTC),
		Status:     "success",
	}))

	// Now corrupt the file by replacing the first entry's data with garbage
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	// Replace base64 data of first entry with invalid base64
	corrupted := replaceFirstEntryData(data)
	require.NoError(t, os.WriteFile(path, corrupted, 0o600))

	// List should return partial results (the non-corrupted entry)
	store3 := NewStore(path, passphrase)
	entries, err := store3.List()
	// Should not error entirely -- returns what it can decrypt
	require.NoError(t, err)
	// At least 1 entry should be readable
	assert.GreaterOrEqual(t, len(entries), 1)

	_ = data // suppress unused warning
}

// replaceFirstEntryData replaces the base64 data of the first encrypted entry with garbage.
func replaceFirstEntryData(data []byte) []byte {
	// Find the first "data":" and replace its value
	s := string(data)
	idx := 0
	target := `"data":"`
	pos := indexOf(s, target, idx)
	if pos < 0 {
		return data
	}
	start := pos + len(target)
	end := indexOf(s, `"`, start)
	if end < 0 {
		return data
	}
	return []byte(s[:start] + "CORRUPTED_GARBAGE_DATA" + s[end:])
}

func indexOf(s, substr string, from int) int {
	sub := s[from:]
	i := 0
	for i < len(sub) {
		j := 0
		for j < len(substr) && i+j < len(sub) && sub[i+j] == substr[j] {
			j++
		}
		if j == len(substr) {
			return from + i
		}
		i++
	}
	return -1
}

func TestStoreKeyCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	passphrase := []byte("test-passphrase")

	store := NewStore(path, passphrase)

	// Append multiple entries
	for i := 0; i < 5; i++ {
		require.NoError(t, store.Append(HistoryEntry{
			SecretName: "SECRET",
			RotatedAt:  time.Now(),
			Status:     "success",
		}))
	}

	// Create fresh store for listing to verify key caching
	listStore := NewStore(path, passphrase)
	entries, err := listStore.List()
	require.NoError(t, err)
	assert.Len(t, entries, 5)

	// Verify derived key is cached (non-nil after List call)
	assert.NotNil(t, listStore.derivedKey, "derivedKey should be cached after List")
	assert.NotNil(t, listStore.salt, "salt should be cached after List")
}
