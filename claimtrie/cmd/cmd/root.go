package cmd

import (
	"os"

	"github.com/btcsuite/btclog"
	"github.com/lbryio/lbcd/claimtrie/config"
	"github.com/lbryio/lbcd/claimtrie/param"
	"github.com/lbryio/lbcd/limits"
	"github.com/lbryio/lbcd/wire"

	"github.com/spf13/cobra"
)

var (
	log     btclog.Logger
	cfg     = config.DefaultConfig
	netName string
	dataDir string
)

var rootCmd = NewRootCommand()

func NewRootCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:          "claimtrie",
		Short:        "ClaimTrie Command Line Interface",
		SilenceUsage: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			switch netName {
			case "mainnet":
				param.SetNetwork(wire.MainNet)
			case "testnet":
				param.SetNetwork(wire.TestNet3)
			case "regtest":
				param.SetNetwork(wire.TestNet)
			}
		},
	}

	cmd.PersistentFlags().StringVar(&netName, "netname", "mainnet", "Net name")
	cmd.PersistentFlags().StringVarP(&dataDir, "datadir", "b", cfg.DataDir, "Data dir")

	return cmd
}

func Execute() {

	backendLogger := btclog.NewBackend(os.Stdout)
	defer os.Stdout.Sync()
	log = backendLogger.Logger("CMDL")
	log.SetLevel(btclog.LevelDebug)

	// Up some limits.
	if err := limits.SetLimits(); err != nil {
		log.Errorf("failed to set limits: %v\n", err)
	}

	rootCmd.Execute() // nolint : errchk
}
