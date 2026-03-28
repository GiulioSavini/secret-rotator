package envfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestEnv(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	return path
}

func TestReadSimpleKeyValue(t *testing.T) {
	path := writeTestEnv(t, "KEY=value\nANOTHER=123\n")
	ef, err := Read(path)
	require.NoError(t, err)

	assert.Equal(t, "KEY", ef.Lines[0].Key)
	assert.Equal(t, "value", ef.Lines[0].Value)
	assert.Equal(t, "ANOTHER", ef.Lines[1].Key)
	assert.Equal(t, "123", ef.Lines[1].Value)
}

func TestReadDoubleQuoted(t *testing.T) {
	path := writeTestEnv(t, `KEY="value with spaces"`+"\n")
	ef, err := Read(path)
	require.NoError(t, err)

	assert.Equal(t, "KEY", ef.Lines[0].Key)
	assert.Equal(t, "value with spaces", ef.Lines[0].Value)
	assert.Equal(t, `"`, ef.Lines[0].Quoted)
}

func TestReadSingleQuoted(t *testing.T) {
	path := writeTestEnv(t, "KEY='value'\n")
	ef, err := Read(path)
	require.NoError(t, err)

	assert.Equal(t, "KEY", ef.Lines[0].Key)
	assert.Equal(t, "value", ef.Lines[0].Value)
	assert.Equal(t, "'", ef.Lines[0].Quoted)
}

func TestReadComments(t *testing.T) {
	path := writeTestEnv(t, "# This is a comment\nKEY=val\n")
	ef, err := Read(path)
	require.NoError(t, err)

	assert.True(t, ef.Lines[0].Comment)
	assert.Equal(t, "", ef.Lines[0].Key)
	assert.Equal(t, "# This is a comment", ef.Lines[0].Raw)
}

func TestReadBlankLines(t *testing.T) {
	path := writeTestEnv(t, "KEY=val\n\nKEY2=val2\n")
	ef, err := Read(path)
	require.NoError(t, err)

	assert.Equal(t, "", ef.Lines[1].Key)
	assert.Equal(t, "", ef.Lines[1].Raw)
	assert.False(t, ef.Lines[1].Comment)
}

func TestReadMalformed(t *testing.T) {
	path := writeTestEnv(t, "this is not a valid line\nKEY=val\n")
	ef, err := Read(path)
	require.NoError(t, err)

	assert.Equal(t, "", ef.Lines[0].Key)
	assert.Equal(t, "this is not a valid line", ef.Lines[0].Raw)
}

func TestReadInlineComment(t *testing.T) {
	// Go .env convention: no inline comments unless quoted
	path := writeTestEnv(t, "KEY=value # comment\n")
	ef, err := Read(path)
	require.NoError(t, err)

	assert.Equal(t, "KEY", ef.Lines[0].Key)
	assert.Equal(t, "value # comment", ef.Lines[0].Value)
}

func TestReadExportPrefix(t *testing.T) {
	path := writeTestEnv(t, "export KEY=value\n")
	ef, err := Read(path)
	require.NoError(t, err)

	assert.Equal(t, "KEY", ef.Lines[0].Key)
	assert.Equal(t, "value", ef.Lines[0].Value)
}

func TestReadMultipleFiles(t *testing.T) {
	path1 := writeTestEnv(t, "A=1\n")

	dir2 := t.TempDir()
	path2 := filepath.Join(dir2, ".env")
	require.NoError(t, os.WriteFile(path2, []byte("B=2\n"), 0644))

	ef1, err := Read(path1)
	require.NoError(t, err)
	ef2, err := Read(path2)
	require.NoError(t, err)

	assert.Equal(t, "A", ef1.Lines[0].Key)
	assert.Equal(t, "B", ef2.Lines[0].Key)
	// Independent -- different paths
	assert.NotEqual(t, ef1.Path, ef2.Path)
}

func TestGet(t *testing.T) {
	path := writeTestEnv(t, "KEY=value\n")
	ef, err := Read(path)
	require.NoError(t, err)

	val, ok := ef.Get("KEY")
	assert.True(t, ok)
	assert.Equal(t, "value", val)
}

func TestGetMissing(t *testing.T) {
	path := writeTestEnv(t, "KEY=value\n")
	ef, err := Read(path)
	require.NoError(t, err)

	val, ok := ef.Get("NONEXISTENT")
	assert.False(t, ok)
	assert.Equal(t, "", val)
}

func TestReadPreservesOriginalLines(t *testing.T) {
	content := "# comment\nKEY=value\n  SPACED = val  \n\nexport FOO=bar\n"
	path := writeTestEnv(t, content)
	ef, err := Read(path)
	require.NoError(t, err)

	// Reconstruct from Raw lines
	var reconstructed string
	for i, line := range ef.Lines {
		reconstructed += line.Raw
		if i < len(ef.Lines)-1 {
			reconstructed += "\n"
		}
	}
	assert.Equal(t, content, reconstructed)
}

func TestReadHashInQuotedValue(t *testing.T) {
	path := writeTestEnv(t, `KEY="pass#word"`+"\n")
	ef, err := Read(path)
	require.NoError(t, err)

	assert.Equal(t, "KEY", ef.Lines[0].Key)
	assert.Equal(t, "pass#word", ef.Lines[0].Value)
	assert.Equal(t, `"`, ef.Lines[0].Quoted)
}
