package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/giulio/secret-rotator/internal/docker"
	"github.com/giulio/secret-rotator/internal/engine"
	"github.com/giulio/secret-rotator/internal/history"
	"github.com/giulio/secret-rotator/internal/provider"
	"github.com/spf13/cobra"
)

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
	// Validate config is loaded
	if AppConfig == nil || len(AppConfig.Secrets) == 0 {
		return fmt.Errorf("configuration required: use --config flag to specify rotator.yml")
	}

	// Find the secret in config
	var found bool
	var secretIdx int
	for i, s := range AppConfig.Secrets {
		if s.Name == secretName {
			found = true
			secretIdx = i
			break
		}
	}
	if !found {
		return fmt.Errorf("secret '%s' not found in configuration", secretName)
	}

	secretCfg := AppConfig.Secrets[secretIdx]

	// Create provider registry and resolve provider
	registry := provider.NewRegistry()
	registry.Register(&provider.GenericProvider{})
	registry.Register(&provider.MySQLProvider{})
	registry.Register(&provider.PostgresProvider{})
	registry.Register(&provider.RedisProvider{})

	prov, err := registry.Get(secretCfg.Type)
	if err != nil {
		return fmt.Errorf("resolving provider for type %q: %w", secretCfg.Type, err)
	}

	// Create Docker manager
	dockerMgr, err := docker.NewSDKClient()
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer dockerMgr.Close()

	// Resolve passphrase for history store
	pp := resolvePassphrase(passphrase)
	var histStore *history.Store
	if pp != "" {
		histStore = history.NewStore(".rotator/history.json", []byte(pp))
	}

	// Get dry-run flag from root persistent flags
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if cmd.Parent() != nil {
		if dr, err := cmd.Parent().PersistentFlags().GetBool("dry-run"); err == nil {
			dryRun = dr
		}
	}

	// Create and execute the engine
	eng := engine.NewEngine(prov, dockerMgr, histStore, 30*time.Second, dryRun)

	if err := eng.Execute(cmd.Context(), secretCfg); err != nil {
		return fmt.Errorf("rotation failed for %s: %w", secretName, err)
	}

	// Print success summary
	containers := "none"
	if len(secretCfg.Containers) > 0 {
		containers = strings.Join(secretCfg.Containers, ", ")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Successfully rotated %s (provider: %s, containers restarted: [%s])\n",
		secretName, secretCfg.Type, containers)

	return nil
}
