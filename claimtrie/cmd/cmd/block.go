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

	var showHash bool
	var showHeight bool

	cmd := &cobra.Command{
		Use:   "best",
		Short: "Show the block hash and height of the best block",
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

			switch {
			case showHeight && showHash:
				fmt.Printf("%s:%d\n", state.Hash, state.Height)
			case !showHeight && showHash:
				fmt.Printf("%s\n", state.Hash)
			case showHeight && !showHash:
				fmt.Printf("%d\n", state.Height)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&showHeight, "showheight", true, "Display block height")
	cmd.Flags().BoolVar(&showHash, "showhash", true, "Display block hash")

	return cmd
}

func NewBlockListCommand() *cobra.Command {

	var fromHeight int32
	var toHeight int32
	var showHash bool
	var showHeight bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List block hash and height between blocks <from_height> <to_height>",
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
				switch {
				case showHeight && showHash:
					fmt.Printf("%s:%d\n", hash, ht)
				case !showHeight && showHash:
					fmt.Printf("%s\n", hash)
				case showHeight && !showHash:
					fmt.Printf("%d\n", ht)
				}
			}

			return nil
		},
	}

	cmd.Flags().Int32Var(&fromHeight, "from", 0, "From height (inclusive)")
	cmd.Flags().Int32Var(&toHeight, "to", 0, "To height (inclusive)")
	cmd.Flags().BoolVar(&showHeight, "showheight", true, "Display block height")
	cmd.Flags().BoolVar(&showHash, "showhash", true, "Display block hash")
	cmd.Flags().SortFlags = false

	return cmd
}
