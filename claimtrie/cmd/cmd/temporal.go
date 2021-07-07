package cmd

import (
	"path/filepath"

	"github.com/lbryio/lbcd/claimtrie/temporal/temporalrepo"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(NewTemporalCommand())
}

func NewTemporalCommand() *cobra.Command {

	var fromHeight int32
	var toHeight int32

	cmd := &cobra.Command{
		Use:   "temporal",
		Short: "List which nodes are update in a range of heights",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {

			dbPath := filepath.Join(dataDir, netName, "claim_dbs", cfg.TemporalRepoPebble.Path)
			log.Debugf("Open temporal repo: %s", dbPath)
			repo, err := temporalrepo.NewPebble(dbPath)
			if err != nil {
				return errors.Wrapf(err, "open temporal repo")
			}

			if toHeight <= 0 {
				toHeight = fromHeight
			}

			for ht := fromHeight; ht <= toHeight; ht++ {
				names, err := repo.NodesAt(ht)
				if err != nil {
					return errors.Wrapf(err, "get node names from temporal")
				}

				if len(names) == 0 {
					continue
				}

				showTemporalNames(ht, names)
			}

			return nil
		},
	}

	cmd.Flags().Int32Var(&fromHeight, "from", 0, "From height (inclusive)")
	cmd.Flags().Int32Var(&toHeight, "to", 0, "To height (inclusive)")
	cmd.Flags().SortFlags = false

	return cmd
}
