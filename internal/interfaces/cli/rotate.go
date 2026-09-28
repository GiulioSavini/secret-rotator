package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/giulio/secret-rotator/internal/domain"
	"github.com/giulio/secret-rotator/internal/infrastructure/configfile"
	"github.com/giulio/secret-rotator/internal/infrastructure/container"
)

// NewRotateCmd creates the rotate subcommand.
func NewRotateCmd() *cobra.Command {
	var passphrase string

	cmd := &cobra.Command{
		Use:   "rotate SECRET_NAME",
		Short: "Rotate a secret",
		Long: `Rotate generates a new value for the named secret, applies it to the backing
service, updates every .env file that carries it, and restarts the affected
containers in dependency order.

Any failure after the first change is compensated: the files are restored, the
containers are restarted with the old values, and the backing service is put
back. Use --dry-run first to see the plan without touching anything.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRotate(cmd, args[0], passphrase)
		},
	}

	cmd.Flags().StringVar(&passphrase, "passphrase", "", "master passphrase for history encryption")

	return cmd
}

func runRotate(cmd *cobra.Command, name string, passphrase string) error {
	if err := AppConfig.RequireSecrets(); err != nil {
		return err
	}

	secret, found := AppConfig.Find(name)
	if !found {
		where := "the configuration"
		if AppConfig.Path != "" {
			where = AppConfig.Path
		}
		return fmt.Errorf("secret '%s' not found in %s (defined: %s)",
			name, where, strings.Join(AppConfig.SecretNames(), ", "))
	}

	secret, err := applyDependencyOrder(cmd, AppConfig, secret)
	if err != nil {
		return err
	}

	rt, err := newRuntime(cmd, runtimeOptions{
		needsDocker: true,
		passphrase:  resolvePassphrase(passphrase),
		quiet:       dryRunFlag,
	})
	if err != nil {
		return err
	}
	defer rt.close()

	if dryRunFlag {
		return printPlan(cmd, rt, secret)
	}

	if err := rt.rotateUseCase().Execute(cmd.Context(), secret); err != nil {
		return err
	}

	containers := "none"
	if refs := secret.Containers(); len(refs) > 0 {
		names := make([]string, 0, len(refs))
		for _, r := range refs {
			names = append(names, r.String())
		}
		containers = strings.Join(names, ", ")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Rotated %s (type: %s, containers restarted: %s)\n",
		secret.Name(), secret.Kind(), containers)
	return nil
}

// printPlan renders what a rotation would do, without doing it.
func printPlan(cmd *cobra.Command, rt *runtime, secret *domain.Secret) error {
	plan, err := rt.planUseCase().Build(cmd.Context(), secret)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Dry run for %s (type: %s). Nothing has been changed.\n\n",
		plan.Secret.Name(), plan.Secret.Kind())
	for i, action := range plan.Actions {
		fmt.Fprintf(out, "  %d. %s\n", i+1, action.Description)
	}
	fmt.Fprintf(out, "\nRun without --dry-run to apply.\n")
	return nil
}

// applyDependencyOrder returns the secret with its containers reordered so
// that dependencies restart before their dependents, using the project's
// Compose file. A missing or unparsable Compose file leaves the configured
// order untouched.
func applyDependencyOrder(
	cmd *cobra.Command, cfg *configfile.Config, secret *domain.Secret,
) (*domain.Secret, error) {
	refs := secret.Containers()
	if len(refs) < 2 {
		return secret, nil
	}

	composePath, err := cfg.ResolveComposeFile()
	if err != nil {
		return nil, err
	}
	if composePath == "" {
		return secret, nil
	}

	order, err := container.LoadDependencyOrder(composePath)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: could not derive restart order from %s (%v); using the configured order\n",
			composePath, err)
		return secret, nil
	}

	names := make([]string, 0, len(refs))
	for _, r := range refs {
		names = append(names, r.String())
	}

	ordered := container.FilterDependencyOrder(order, names)
	// Keep any entry the Compose file does not know about -- a plain container
	// name, for instance -- at the end rather than dropping it.
	known := make(map[string]bool, len(ordered))
	for _, n := range ordered {
		known[n] = true
	}
	for _, n := range names {
		if !known[n] {
			ordered = append(ordered, n)
		}
	}

	if verboseFlag {
		fmt.Fprintf(cmd.ErrOrStderr(), "restart order from %s: %s\n",
			composePath, strings.Join(ordered, " -> "))
	}

	out := make([]domain.ContainerRef, 0, len(ordered))
	for _, n := range ordered {
		out = append(out, domain.ContainerRef(n))
	}
	return secret.WithContainers(out), nil
}
