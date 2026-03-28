package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionCmd_DefaultValues(t *testing.T) {
	cmd := NewVersionCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "rotator version dev")
	assert.Contains(t, output, "commit: none")
	assert.Contains(t, output, "built: unknown")
}

func TestVersionCmd_OutputFormat(t *testing.T) {
	cmd := NewVersionCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()
	require.NoError(t, err)

	expected := "rotator version dev (commit: none, built: unknown)\n"
	assert.Equal(t, expected, buf.String())
}
