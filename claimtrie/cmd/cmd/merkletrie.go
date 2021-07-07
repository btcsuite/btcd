package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/lbryio/lbcd/claimtrie/merkletrie"
	"github.com/lbryio/lbcd/claimtrie/merkletrie/merkletrierepo"
	"github.com/lbryio/lbcd/claimtrie/temporal/temporalrepo"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(NewTrieCommands())
}

func NewTrieCommands() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "trie",
		Short: "MerkleTrie related commands",
	}

	cmd.AddCommand(NewTrieNameCommand())

	return cmd
}

func NewTrieNameCommand() *cobra.Command {

	var height int32
	var name string

	cmd := &cobra.Command{
		Use:   "name",
		Short: "List the claim and child hashes at vertex name of block at height",
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

			state := chain.BestSnapshot()
			fmt.Printf("Block %7d: %s\n", state.Height, state.Hash.String())

			if height > state.Height {
				return errors.New("requested height is unavailable")
			}

			hash := state.Hash

			dbPath := filepath.Join(dataDir, netName, "claim_dbs", cfg.MerkleTrieRepoPebble.Path)
			log.Debugf("Open merkletrie repo: %q", dbPath)
			trieRepo, err := merkletrierepo.NewPebble(dbPath)
			if err != nil {
				return errors.Wrapf(err, "open merkle trie repo")
			}

			trie := merkletrie.NewPersistentTrie(trieRepo)
			defer trie.Close()

			trie.SetRoot(&hash)

			if len(name) > 1 {
				trie.Dump(name)
				return nil
			}

			dbPath = filepath.Join(dataDir, netName, "claim_dbs", cfg.TemporalRepoPebble.Path)
			log.Debugf("Open temporal repo: %q", dbPath)
			tmpRepo, err := temporalrepo.NewPebble(dbPath)
			if err != nil {
				return errors.Wrapf(err, "open temporal repo")
			}

			nodes, err := tmpRepo.NodesAt(height)
			if err != nil {
				return errors.Wrapf(err, "read temporal repo at %d", height)
			}

			for _, name := range nodes {
				fmt.Printf("Name: %s, ", string(name))
				trie.Dump(string(name))
			}

			return nil
		},
	}

	cmd.Flags().Int32Var(&height, "height", 0, "Height")
	cmd.Flags().StringVar(&name, "name", "", "Name")
	cmd.Flags().SortFlags = false

	return cmd
}
