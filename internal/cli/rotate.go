package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/giulio/secret-rotator/internal/config"
	"github.com/giulio/secret-rotator/internal/docker"
	"github.com/giulio/secret-rotator/internal/engine"
	"github.com/giulio/secret-rotator/internal/history"
	"github.com/giulio/secret-rotator/internal/provider"
	"github.com/spf13/cobra"
)

// defaultStepTimeout bounds each container restart and health check.
const defaultStepTimeout = 30 * time.Second

// NewRotateCmd creates the rotate subcommand.
func NewRotateCmd() *cobra.Command {
	var passphrase string

	cmd := &cobra.Command{
		Use:   "rotate SECRET_NAME",
		Short: "Rotate a secret",
		Long:  `Rotate generates a new value for the named secret, updates .env files, and restarts affected containers.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRotate(cmd, args[0], passphrase)
		},
	}

	cmd.Flags().StringVar(&passphrase, "passphrase", "", "master passphrase for history encryption")

	return cmd
}

func runRotate(cmd *cobra.Command, secretName string, passphrase string) error {
	if err := AppConfig.RequireSecrets(); err != nil {
		return err
	}

	secretCfg, err := findSecret(AppConfig, secretName)
	if err != nil {
		return err
	}

	prov, err := newRegistry().Get(secretCfg.Type)
	if err != nil {
		return fmt.Errorf("resolving provider for type %q: %w", secretCfg.Type, err)
	}

	dockerMgr, err := docker.NewSDKClient()
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer dockerMgr.Close()

	if err := applyDependencyOrder(cmd, AppConfig, &secretCfg); err != nil {
		return err
	}

	histStore := openHistoryStore(cmd, passphrase)

	eng := engine.NewEngine(prov, dockerMgr, histStore, defaultStepTimeout, dryRunFlag)

	if err := eng.Execute(cmd.Context(), secretCfg); err != nil {
		return fmt.Errorf("rotation failed for %s: %w", secretName, err)
	}

	containers := "none"
	if len(secretCfg.Containers) > 0 {
		containers = strings.Join(secretCfg.Containers, ", ")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Successfully rotated %s (provider: %s, containers restarted: [%s])\n",
		secretName, secretCfg.Type, containers)

	return nil
}

// findSecret looks up a secret by name, listing the known names on failure.
func findSecret(cfg *config.Config, name string) (config.SecretConfig, error) {
	known := make([]string, 0, len(cfg.Secrets))
	for _, s := range cfg.Secrets {
		if s.Name == name {
			return s, nil
		}
		known = append(known, s.Name)
	}
	where := "configuration"
	if cfg.Path != "" {
		where = cfg.Path
	}
	return config.SecretConfig{}, fmt.Errorf(
		"secret '%s' not found in %s (defined: %s)", name, where, strings.Join(known, ", "))
}

// newRegistry builds the provider registry with every supported backend.
func newRegistry() *provider.Registry {
	registry := provider.NewRegistry()
	registry.Register(&provider.GenericProvider{})
	registry.Register(&provider.MySQLProvider{})
	registry.Register(&provider.PostgresProvider{})
	registry.Register(&provider.RedisProvider{})
	return registry
}

// openHistoryStore returns the encrypted history store, or nil when no
// passphrase is available. A missing passphrase disables the audit log rather
// than blocking the rotation, so it is reported on stderr.
func openHistoryStore(cmd *cobra.Command, passphrase string) *history.Store {
	pp := resolvePassphrase(passphrase)
	if pp == "" {
		fmt.Fprintln(cmd.ErrOrStderr(),
			"warning: no master passphrase (ROTATOR_MASTER_KEY); this rotation will not be recorded in the history")
		return nil
	}
	return history.NewStore(historyPath(), []byte(pp))
}

// applyDependencyOrder reorders a secret's containers so that dependencies
// restart before their dependents, using the project's Compose file.
// A missing or unparsable Compose file leaves the configured order untouched.
func applyDependencyOrder(cmd *cobra.Command, cfg *config.Config, secretCfg *config.SecretConfig) error {
	if len(secretCfg.Containers) < 2 {
		return nil
	}

	composePath, err := cfg.ResolveComposeFile()
	if err != nil {
		return err
	}
	if composePath == "" {
		return nil
	}

	order, err := docker.LoadDependencyOrder(composePath)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: could not derive restart order from %s (%v); using the configured order\n", composePath, err)
		return nil
	}

	ordered := docker.FilterDependencyOrder(order, secretCfg.Containers)
	// Keep any entry the Compose file does not know about (a plain container
	// name, for instance) at the end rather than dropping it.
	known := make(map[string]bool, len(ordered))
	for _, name := range ordered {
		known[name] = true
	}
	for _, name := range secretCfg.Containers {
		if !known[name] {
			ordered = append(ordered, name)
		}
	}

	if verboseFlag {
		fmt.Fprintf(cmd.ErrOrStderr(), "restart order from %s: %s\n",
			composePath, strings.Join(ordered, " -> "))
	}
	secretCfg.Containers = ordered
	return nil
}
