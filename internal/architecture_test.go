// Package internal_test holds the architecture rules.
//
// The layering only survives if something checks it. Reviews miss a single
// added import; this does not.
package internal_test

import (
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const modulePath = "github.com/giulio/secret-rotator"

// layer describes what one layer of the module is allowed to import.
type layer struct {
	// dir is the layer's directory, relative to the module root.
	dir string
	// forbidden lists module-internal prefixes the layer must not import.
	forbidden []string
	// stdlibOnly, when set, additionally forbids every third-party import.
	stdlibOnly bool
	// why explains the rule in the failure message.
	why string
}

var layers = []layer{
	{
		dir: "internal/domain",
		forbidden: []string{
			modulePath + "/internal/application",
			modulePath + "/internal/infrastructure",
			modulePath + "/internal/interfaces",
		},
		stdlibOnly: true,
		why: "the domain is the innermost layer: it declares what it needs as interfaces (ports.go) " +
			"and lets the adapters implement them. An import here would make the model depend on a " +
			"driver, a file format or a CLI framework.",
	},
	{
		dir: "internal/application",
		forbidden: []string{
			modulePath + "/internal/infrastructure",
			modulePath + "/internal/interfaces",
		},
		stdlibOnly: true,
		why: "use cases orchestrate the domain through its ports. Reaching for a concrete adapter " +
			"here would make the use case untestable without Docker or a database.",
	},
	{
		dir:       "internal/infrastructure",
		forbidden: []string{modulePath + "/internal/interfaces"},
		why: "adapters are driven by the layers above them; an adapter that imports the CLI has " +
			"the dependency backwards.",
	},
}

func TestLayerDependencies(t *testing.T) {
	root := moduleRoot(t)

	for _, l := range layers {
		t.Run(l.dir, func(t *testing.T) {
			for _, pkgDir := range goPackagesUnder(t, filepath.Join(root, l.dir)) {
				pkg, err := build.ImportDir(pkgDir, 0)
				require.NoError(t, err, "parsing %s", pkgDir)

				rel, _ := filepath.Rel(root, pkgDir)

				// The layer rule covers test files too: a test that needs an
				// adapter is a sign the boundary has already moved.
				for _, imported := range append(append([]string{}, pkg.Imports...), pkg.TestImports...) {
					for _, banned := range l.forbidden {
						require.False(t, strings.HasPrefix(imported, banned),
							"%s imports %s\n\n%s", rel, imported, l.why)
					}
				}

				// The purity rule covers production code only. Assertion
				// libraries in _test.go files ship with nothing and constrain
				// nothing.
				if !l.stdlibOnly {
					continue
				}
				for _, imported := range pkg.Imports {
					if isThirdParty(imported) {
						require.Failf(t, "third-party import in a pure layer",
							"%s imports %s\n\n%s", rel, imported, l.why)
					}
				}
			}
		})
	}
}

// TestDomainDeclaresItsPorts guards the reason the layering exists: the domain
// has to declare the interfaces the adapters implement. If ports.go were
// deleted and the types inlined into the adapters, the compiler would stay
// happy and the architecture would quietly be gone.
func TestDomainDeclaresItsPorts(t *testing.T) {
	root := moduleRoot(t)

	body, err := os.ReadFile(filepath.Join(root, "internal", "domain", "ports.go"))
	require.NoError(t, err, "the domain must declare its ports")

	for _, port := range []string{
		"PasswordGenerator",
		"CredentialRotator",
		"RotatorRegistry",
		"EnvStore",
		"EnvDocument",
		"ContainerFleet",
		"AuditTrail",
		"Clock",
		"Reporter",
	} {
		require.Contains(t, string(body), "type "+port+" interface",
			"port %s must stay declared in the domain", port)
	}
}

// isThirdParty reports whether an import path points outside the standard
// library. Standard library paths have no dot in their first segment.
func isThirdParty(importPath string) bool {
	if strings.HasPrefix(importPath, modulePath) {
		return false
	}
	first, _, _ := strings.Cut(importPath, "/")
	return strings.Contains(first, ".")
}

// goPackagesUnder lists every directory under root that holds Go files.
func goPackagesUnder(t *testing.T, root string) []string {
	t.Helper()

	var dirs []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			return readErr
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
				dirs = append(dirs, path)
				break
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, dirs, "no Go packages found under %s", root)
	return dirs
}

// moduleRoot walks up from the test's directory to the directory holding
// go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "go.mod not found above %s", dir)
		dir = parent
	}
}
