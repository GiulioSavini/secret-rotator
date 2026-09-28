package cli

import (
	"fmt"

	"github.com/giulio/secret-rotator/internal/config"
	"github.com/spf13/cobra"
)

// Global state shared by subcommands, populated in PersistentPreRunE.
var (
	// AppConfig holds the loaded configuration.
	AppConfig *config.Config
	// dataDirFlag is the value of --data-dir, empty unless set explicitly.
	dataDirFlag string
	// verboseFlag reports whether --verbose was requested.
	verboseFlag bool
	// dryRunFlag reports whether --dry-run was requested.
	dryRunFlag bool
)

// NewRootCmd creates the root command with all subcommands and global flags.
func NewRootCmd() *cobra.Command {
	var cfgFile string

	rootCmd := &cobra.Command{
		Use:   "rotator",
		Short: "Secret rotation for self-hosted Docker environments",
		Long: `Rotator discovers, rotates, and manages secrets in Docker Compose environments.

It reads secret definitions from a rotator.yml configuration file, rotates
credentials in .env files, updates backing services, and restarts affected
containers in dependency order.

The configuration file is discovered automatically: ROTATOR_CONFIG, then
./rotator.yml, then /config/rotator.yml, then /etc/rotator/rotator.yml.
Run 'rotator init' to generate one from the current directory.`,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := config.LoadDiscovered(cfgFile)
			if err != nil {
				return err
			}
			AppConfig = cfg

			for _, w := range cfg.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}
			if verboseFlag && cfg.Path != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "using config: %s\n", cfg.Path)
				fmt.Fprintf(cmd.ErrOrStderr(), "using data dir: %s\n", cfg.DataDir(dataDirFlag))
			}
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"config file (default: auto-discovered, see ROTATOR_CONFIG)")
	rootCmd.PersistentFlags().StringVar(&dataDirFlag, "data-dir", "",
		"directory for rotation history (default: .rotator next to the config, see ROTATOR_DATA_DIR)")
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&dryRunFlag, "dry-run", false,
		"show what would be done without making changes")

	rootCmd.Version = version

	rootCmd.AddCommand(NewInitCmd())
	rootCmd.AddCommand(NewScanCmd())
	rootCmd.AddCommand(NewRotateCmd())
	rootCmd.AddCommand(NewStatusCmd())
	rootCmd.AddCommand(NewHistoryCmd())
	rootCmd.AddCommand(NewDaemonCmd())
	rootCmd.AddCommand(NewVersionCmd())

	return rootCmd
}

// historyPath returns the resolved path of the encrypted rotation history.
func historyPath() string {
	return AppConfig.HistoryPath(dataDirFlag)
}
