package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/giulio/secret-rotator/internal/config"
	"github.com/giulio/secret-rotator/internal/discovery"
	"github.com/giulio/secret-rotator/internal/docker"
	"github.com/giulio/secret-rotator/internal/envfile"
	"github.com/spf13/cobra"
)

// NewInitCmd creates the init subcommand, which writes a ready-to-edit
// rotator.yml derived from the .env files and running containers it finds.
func NewInitCmd() *cobra.Command {
	var dir string
	var output string
	var force bool

	cmd := &cobra.Command{
		Use:   "init [directory]",
		Short: "Generate a rotator.yml from the current project",
		Long: `Init inspects the .env files in a directory and the containers running on
the Docker daemon, then writes a rotator.yml with one entry per discovered
secret, pre-filled with the provider settings it could infer.

Entries it cannot fully infer are written with TODO markers, and the command
prints exactly which fields still need a value. Secret values are never
written to the generated file.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				dir = args[0]
			}
			return runInit(cmd, dir, output, force)
		},
	}

	cmd.Flags().StringVar(&dir, "dir", ".", "directory to inspect for .env files")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output path (default: <dir>/rotator.yml)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing configuration file")

	return cmd
}

// serviceInfo describes a container discovered on the daemon.
type serviceInfo struct {
	// Ref is what the config should reference: the Compose service name when
	// available, otherwise the container name.
	Ref   string
	Image string
}

func runInit(cmd *cobra.Command, dir, output string, force bool) error {
	if output == "" {
		output = filepath.Join(dir, "rotator.yml")
	}

	if _, err := os.Stat(output); err == nil && !force {
		return fmt.Errorf("%s already exists; pass --force to overwrite it", output)
	}

	envFiles, err := collectEnvFiles(dir)
	if err != nil {
		return err
	}
	if len(envFiles) == 0 {
		return fmt.Errorf("no .env files found in %s; create one, or pass a different directory", dir)
	}

	secrets := discovery.NewScanner().ScanFiles(envFiles)
	if len(secrets) == 0 {
		return fmt.Errorf("no secrets discovered in %s; rotator recognises keys such as *_PASSWORD, *_TOKEN, *_API_KEY", dir)
	}

	services := discoverServices(cmd.Context(), cmd)

	rendered, todos := renderConfig(dir, secrets, services)

	if err := os.WriteFile(output, []byte(rendered), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", output, err)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Wrote %s (%d secrets discovered).\n", output, len(secrets))
	if len(todos) > 0 {
		fmt.Fprintf(out, "\nBefore the first rotation, fill in:\n")
		for _, t := range todos {
			fmt.Fprintf(out, "  - %s\n", t)
		}
	}
	fmt.Fprintf(out, "\nThen verify with:\n  rotator status\n  rotator rotate <name> --dry-run\n")
	return nil
}

// collectEnvFiles reads the .env files in dir, skipping templates that are
// meant to be committed rather than rotated.
func collectEnvFiles(dir string) ([]*envfile.EnvFile, error) {
	matches, err := filepath.Glob(filepath.Join(dir, ".env*"))
	if err != nil {
		return nil, fmt.Errorf("scanning %s: %w", dir, err)
	}
	sort.Strings(matches)

	var files []*envfile.EnvFile
	for _, path := range matches {
		base := filepath.Base(path)
		if strings.HasSuffix(base, ".example") || strings.HasSuffix(base, ".sample") ||
			strings.HasSuffix(base, ".template") || strings.HasSuffix(base, ".dist") {
			continue
		}
		ef, readErr := envfile.Read(path)
		if readErr != nil {
			continue
		}
		files = append(files, ef)
	}
	return files, nil
}

// discoverServices queries the Docker daemon for running containers. Failure
// is not fatal: init still produces a config, just with more TODOs.
func discoverServices(ctx context.Context, cmd *cobra.Command) []serviceInfo {
	mgr, err := docker.NewSDKClient()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "note: Docker is not reachable (%v); container names will need to be filled in manually\n", err)
		return nil
	}
	defer mgr.Close()

	listCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	containers, err := mgr.ListContainers(listCtx, docker.ContainerFilter{})
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "note: could not list containers (%v); container names will need to be filled in manually\n", err)
		return nil
	}

	seen := make(map[string]bool)
	var services []serviceInfo
	for _, c := range containers {
		ref := c.Labels[docker.ComposeServiceLabel]
		if ref == "" {
			ref = c.Name
		}
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		services = append(services, serviceInfo{Ref: ref, Image: c.Image})
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Ref < services[j].Ref })
	return services
}

// providerDefaults describes how a secret type maps onto a rotator provider.
type providerDefaults struct {
	kind         string // rotator provider type
	port         string
	imageHints   []string // substrings identifying the backing service image
	adminUser    string
	needsDB      bool
	defaultLen   int
	scheduleHint string
}

// providerCatalog maps env-key markers onto backing services. Order matters:
// the first marker found in the key wins.
var providerCatalog = []struct {
	marker string
	def    providerDefaults
}{
	{"MARIADB", providerDefaults{kind: "mysql", port: "3306", imageHints: []string{"mariadb", "mysql"}, adminUser: "root", defaultLen: 32, scheduleHint: "0 3 1 * *"}},
	{"MYSQL", providerDefaults{kind: "mysql", port: "3306", imageHints: []string{"mysql", "mariadb"}, adminUser: "root", defaultLen: 32, scheduleHint: "0 3 1 * *"}},
	{"POSTGRES", providerDefaults{kind: "postgres", port: "5432", imageHints: []string{"postgres"}, adminUser: "postgres", needsDB: true, defaultLen: 32, scheduleHint: "0 3 1 * *"}},
	{"PGSQL", providerDefaults{kind: "postgres", port: "5432", imageHints: []string{"postgres"}, adminUser: "postgres", needsDB: true, defaultLen: 32, scheduleHint: "0 3 1 * *"}},
	{"REDIS", providerDefaults{kind: "redis", port: "6379", imageHints: []string{"redis", "valkey"}, defaultLen: 32, scheduleHint: "0 4 1 * *"}},
}

// classify picks the provider defaults for an env key, defaulting to generic.
func classify(key string) providerDefaults {
	upper := strings.ToUpper(key)
	for _, entry := range providerCatalog {
		if strings.Contains(upper, entry.marker) {
			return entry.def
		}
	}
	return providerDefaults{kind: "generic", defaultLen: 32, scheduleHint: "0 3 1 * *"}
}

// matchService finds the container whose image looks like the backing service
// for a provider kind. Returns an empty string when there is no clear match.
func matchService(services []serviceInfo, hints []string) string {
	for _, hint := range hints {
		for _, s := range services {
			if strings.Contains(strings.ToLower(s.Image), hint) {
				return s.Ref
			}
		}
	}
	return ""
}

// secretNameFor derives a stable, lowercase config name from an env key.
func secretNameFor(key string, used map[string]bool) string {
	name := strings.ToLower(key)
	candidate := name
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%s_%d", name, i)
	}
	used[candidate] = true
	return candidate
}

// renderConfig builds the rotator.yml text and the list of fields the user
// still has to supply. It is rendered as text rather than marshalled so the
// generated file can carry explanatory comments.
func renderConfig(dir string, secrets []discovery.DiscoveredSecret, services []serviceInfo) (string, []string) {
	var b strings.Builder
	var todos []string
	used := make(map[string]bool)

	b.WriteString("# Generated by 'rotator init'. Review before the first rotation.\n")
	b.WriteString("# Reference: https://github.com/GiulioSavini/secret-rotator\n\n")
	b.WriteString("# Environment variable holding the passphrase that encrypts the rotation\n")
	b.WriteString("# history. Without it, rotations run but are not recorded.\n")
	b.WriteString("master_key_env: ROTATOR_MASTER_KEY\n\n")

	if composePath := findComposeFile(dir); composePath != "" {
		b.WriteString("# Used to restart containers in dependency order.\n")
		fmt.Fprintf(&b, "compose_file: %s\n\n", filepath.Base(composePath))
	}

	if len(services) > 0 {
		b.WriteString("# Containers visible on this host at generation time:\n")
		for _, s := range services {
			fmt.Fprintf(&b, "#   %s (%s)\n", s.Ref, s.Image)
		}
		b.WriteString("\n")
	}

	b.WriteString("secrets:\n")

	for _, s := range secrets {
		if s.FileReferenced {
			// *_FILE keys point at a mounted secret file, not a value to rotate.
			fmt.Fprintf(&b, "  # %s references a file (%s); rotate the file's content out of band.\n\n",
				s.Key, s.Source)
			continue
		}

		def := classify(s.Key)
		name := secretNameFor(s.Key, used)
		host := matchService(services, def.imageHints)

		fmt.Fprintf(&b, "  - name: %s\n", name)
		fmt.Fprintf(&b, "    type: %s\n", def.kind)
		fmt.Fprintf(&b, "    env_key: %s\n", s.Key)
		fmt.Fprintf(&b, "    env_file: %s\n", filepath.Base(s.Source))

		if host != "" {
			fmt.Fprintf(&b, "    containers:\n      - %s\n", host)
			b.WriteString("      # TODO add every service that reads this secret (app, worker, ...)\n")
			todos = append(todos, fmt.Sprintf("%s: list the application containers that read %s", name, s.Key))
		} else {
			b.WriteString("    containers: []   # TODO list the containers to restart after rotation\n")
			todos = append(todos, fmt.Sprintf("%s: set 'containers'", name))
		}

		if def.kind != "generic" {
			b.WriteString("    provider:\n")
			if host != "" {
				fmt.Fprintf(&b, "      host: %s\n", host)
			} else {
				fmt.Fprintf(&b, "      host: TODO   # hostname of the %s server as seen by rotator\n", def.kind)
				todos = append(todos, fmt.Sprintf("%s: set 'provider.host'", name))
			}
			fmt.Fprintf(&b, "      port: \"%s\"\n", def.port)

			if def.adminUser != "" {
				fmt.Fprintf(&b, "      username: %s\n", def.adminUser)
				fmt.Fprintf(&b, "      password_env: %s   # admin password, read from .env or the environment\n", s.Key)
				b.WriteString("      # target_user: app_user   # uncomment to rotate a non-admin account\n")
			}
			if def.needsDB {
				b.WriteString("      database: postgres\n")
			}
		}

		fmt.Fprintf(&b, "    schedule: \"%s\"   # monthly; see https://crontab.guru\n", def.scheduleHint)
		fmt.Fprintf(&b, "    length: %d\n\n", def.defaultLen)
	}

	b.WriteString("# Optional webhook targets.\n")
	b.WriteString("# notifications:\n")
	b.WriteString("#   - type: slack\n")
	b.WriteString("#     url: https://hooks.slack.com/services/xxx/yyy/zzz\n")

	return b.String(), todos
}

// findComposeFile returns the Compose file in dir, if one exists.
func findComposeFile(dir string) string {
	for _, name := range config.ComposeFileNames {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}
