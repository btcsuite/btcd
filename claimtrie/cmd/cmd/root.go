package cmd

import (
	"os"

	"github.com/lbryio/lbcd/claimtrie/config"
	"github.com/lbryio/lbcd/claimtrie/param"
	"github.com/lbryio/lbcd/limits"
	"github.com/lbryio/lbcd/wire"

	"github.com/btcsuite/btclog"
	"github.com/spf13/cobra"
)

var (
	log        = btclog.NewBackend(os.Stdout).Logger("CMDL")
	cfg        = config.DefaultConfig
	netName    string
	dataDir    string
	debugLevel string
)

var rootCmd = NewRootCommand()

func NewRootCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:          "claimtrie",
		Short:        "ClaimTrie Command Line Interface",
		SilenceUsage: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			level, _ := btclog.LevelFromString(debugLevel)
			log.SetLevel(level)

			switch netName {
			case "mainnet":
				param.SetNetwork(wire.MainNet)
			case "testnet":
				param.SetNetwork(wire.TestNet3)
			case "regtest":
				param.SetNetwork(wire.TestNet)
			}
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			os.Stdout.Sync()
		},
	}

	cmd.PersistentFlags().StringVar(&netName, "netname", "mainnet", "Net name")
	cmd.PersistentFlags().StringVarP(&dataDir, "datadir", "b", cfg.DataDir, "Data dir")
	cmd.PersistentFlags().StringVarP(&debugLevel, "debuglevel", "d", cfg.DebugLevel, "Debug level")

	return cmd
}

func Execute() {

	// Up some limits.
	if err := limits.SetLimits(); err != nil {
		log.Errorf("failed to set limits: %v\n", err)
	}

	rootCmd.Execute() // nolint : errchk
}
