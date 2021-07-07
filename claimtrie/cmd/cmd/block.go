package cmd

import (
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(NewBlocCommands())
}

func NewBlocCommands() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "block",
		Short: "Block related commands",
	}

	cmd.AddCommand(NewBlockBestCommand())
	cmd.AddCommand(NewBlockListCommand())

	return cmd
}

func NewBlockBestCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "best",
		Short: "Show the height and hash of the best block",
		RunE: func(cmd *cobra.Command, args []string) error {

			db, err := loadBlocksDB()
			if err != nil {
				return errors.Wrapf(err, "load blocks database")
			}
			defer db.Close()

			chain, err := loadChain(db)
			if err != nil {
				return errors.Wrapf(err, "load chain")
			}

			state := chain.BestSnapshot()
			fmt.Printf("Block %7d: %s\n", state.Height, state.Hash.String())

			return nil
		},
	}

	return cmd
}

func NewBlockListCommand() *cobra.Command {

	var fromHeight int32
	var toHeight int32

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List merkle hash of blocks between <from_height> <to_height>",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {

			db, err := loadBlocksDB()
			if err != nil {
				return errors.Wrapf(err, "load blocks database")
			}
			defer db.Close()

			chain, err := loadChain(db)
			if err != nil {
				return errors.Wrapf(err, "load chain")
			}

			if toHeight > chain.BestSnapshot().Height {
				toHeight = chain.BestSnapshot().Height
			}

			for ht := fromHeight; ht <= toHeight; ht++ {
				hash, err := chain.BlockHashByHeight(ht)
				if err != nil {
					return errors.Wrapf(err, "load hash for %d", ht)
				}
				fmt.Printf("Block %7d: %s\n", ht, hash.String())
			}

			return nil
		},
	}

	cmd.Flags().Int32Var(&fromHeight, "from", 0, "From height (inclusive)")
	cmd.Flags().Int32Var(&toHeight, "to", 0, "To height (inclusive)")
	cmd.Flags().SortFlags = false

	return cmd
}
