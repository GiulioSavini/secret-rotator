package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeComposeFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestLoadDependencyOrderSimple(t *testing.T) {
	// A depends_on B => order should be [B, A]
	path := writeComposeFile(t, `
services:
  a:
    image: scratch
    depends_on:
      - b
  b:
    image: scratch
`)

	order, err := LoadDependencyOrder(path)
	require.NoError(t, err)
	require.Len(t, order, 2)
	// B must come before A
	bIdx := indexOf(order, "b")
	aIdx := indexOf(order, "a")
	assert.True(t, bIdx < aIdx, "b should come before a, got order: %v", order)
}

func TestLoadDependencyOrderChain(t *testing.T) {
	// A -> B -> C => [C, B, A]
	path := writeComposeFile(t, `
services:
  a:
    image: scratch
    depends_on:
      - b
  b:
    image: scratch
    depends_on:
      - c
  c:
    image: scratch
`)

	order, err := LoadDependencyOrder(path)
	require.NoError(t, err)
	require.Len(t, order, 3)
	cIdx := indexOf(order, "c")
	bIdx := indexOf(order, "b")
	aIdx := indexOf(order, "a")
	assert.True(t, cIdx < bIdx, "c should come before b, got: %v", order)
	assert.True(t, bIdx < aIdx, "b should come before a, got: %v", order)
}

func TestLoadDependencyOrderDiamond(t *testing.T) {
	// A->B, A->C, B->D, C->D => D first, A last
	path := writeComposeFile(t, `
services:
  a:
    image: scratch
    depends_on:
      - b
      - c
  b:
    image: scratch
    depends_on:
      - d
  c:
    image: scratch
    depends_on:
      - d
  d:
    image: scratch
`)

	order, err := LoadDependencyOrder(path)
	require.NoError(t, err)
	require.Len(t, order, 4)
	dIdx := indexOf(order, "d")
	aIdx := indexOf(order, "a")
	assert.Equal(t, 0, dIdx, "d should be first, got: %v", order)
	assert.Equal(t, 3, aIdx, "a should be last, got: %v", order)
}

func TestLoadDependencyOrderNoDeps(t *testing.T) {
	// No depends_on -- services returned in any order
	path := writeComposeFile(t, `
services:
  x:
    image: scratch
  y:
    image: scratch
  z:
    image: scratch
`)

	order, err := LoadDependencyOrder(path)
	require.NoError(t, err)
	require.Len(t, order, 3)
	assert.Contains(t, order, "x")
	assert.Contains(t, order, "y")
	assert.Contains(t, order, "z")
}

func TestLoadDependencyOrderCycle(t *testing.T) {
	// A->B->C->A => circular => error
	path := writeComposeFile(t, `
services:
  a:
    image: scratch
    depends_on:
      - b
  b:
    image: scratch
    depends_on:
      - c
  c:
    image: scratch
    depends_on:
      - a
`)

	_, err := LoadDependencyOrder(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestLoadDependencyOrderFilterServices(t *testing.T) {
	// Full order: D, B, C, A
	// Filter to only [a, d] => [d, a] (d before a since a depends on b depends on d)
	path := writeComposeFile(t, `
services:
  a:
    image: scratch
    depends_on:
      - b
      - c
  b:
    image: scratch
    depends_on:
      - d
  c:
    image: scratch
    depends_on:
      - d
  d:
    image: scratch
`)

	allOrder, err := LoadDependencyOrder(path)
	require.NoError(t, err)

	filtered := FilterDependencyOrder(allOrder, []string{"a", "d"})
	require.Len(t, filtered, 2)
	assert.Equal(t, "d", filtered[0])
	assert.Equal(t, "a", filtered[1])
}

func indexOf(slice []string, item string) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}
	return -1
}
