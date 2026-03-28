package cli

import (
	"github.com/giulio/secret-rotator/internal/config"
	"github.com/spf13/cobra"
)

// AppConfig holds the loaded configuration, accessible to subcommands.
var AppConfig *config.Config

// NewRootCmd creates the root command with all subcommands and global flags.
func NewRootCmd() *cobra.Command {
	var cfgFile string
	var verbose bool
	var dryRun bool

	rootCmd := &cobra.Command{
		Use:   "rotator",
		Short: "Secret rotation for self-hosted Docker environments",
		Long: `Rotator discovers, rotates, and manages secrets in Docker Compose environments.

It reads secret definitions from a rotator.yml configuration file, rotates
credentials in .env files, updates backing services, and restarts affected
containers in dependency order.`,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return err
			}
			AppConfig = cfg
			_ = verbose // will be used by subcommands
			_ = dryRun  // will be used by subcommands
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: none, zero-config mode)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "show what would be done without making changes")

	rootCmd.AddCommand(NewScanCmd())
	rootCmd.AddCommand(NewRotateCmd())
	rootCmd.AddCommand(NewStatusCmd())
	rootCmd.AddCommand(NewHistoryCmd())
	rootCmd.AddCommand(NewDaemonCmd())

	return rootCmd
}
