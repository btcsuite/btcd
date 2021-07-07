package cmd

import (
	"encoding/hex"
	"fmt"
	"math"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/lbryio/lbcd/claimtrie/change"
	"github.com/lbryio/lbcd/claimtrie/node"
	"github.com/lbryio/lbcd/claimtrie/node/noderepo"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(NewNodeCommands())
}

func NewNodeCommands() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "node",
		Short: "Replay the application of changes on a node up to certain height",
	}

	cmd.AddCommand(NewNodeDumpCommand())
	cmd.AddCommand(NewNodeReplayCommand())
	cmd.AddCommand(NewNodeChildrenCommand())
	cmd.AddCommand(NewNodeStatsCommand())

	return cmd
}

func NewNodeDumpCommand() *cobra.Command {

	var name string
	var height int32

	cmd := &cobra.Command{
		Use:   "dump",
		Short: "Replay the application of changes on a node up to certain height",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {

			dbPath := filepath.Join(dataDir, netName, "claim_dbs", cfg.NodeRepoPebble.Path)
			log.Debugf("Open node repo: %q", dbPath)
			repo, err := noderepo.NewPebble(dbPath)
			if err != nil {
				return errors.Wrapf(err, "open node repo")
			}
			defer repo.Close()

			changes, err := repo.LoadChanges([]byte(name))
			if err != nil {
				return errors.Wrapf(err, "load commands")
			}

			for _, chg := range changes {
				if chg.Height > height {
					break
				}
				showChange(chg)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name")
	cmd.MarkFlagRequired("name")
	cmd.Flags().Int32Var(&height, "height", math.MaxInt32, "Height")

	return cmd
}

func NewNodeReplayCommand() *cobra.Command {

	var name string
	var height int32

	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Replay the changes of <name> up to <height>",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {

			dbPath := filepath.Join(dataDir, netName, "claim_dbs", cfg.NodeRepoPebble.Path)
			log.Debugf("Open node repo: %q", dbPath)
			repo, err := noderepo.NewPebble(dbPath)
			if err != nil {
				return errors.Wrapf(err, "open node repo")
			}

			bm, err := node.NewBaseManager(repo)
			if err != nil {
				return errors.Wrapf(err, "create node manager")
			}
			defer bm.Close()

			nm := node.NewNormalizingManager(bm)

			n, err := nm.NodeAt(height, []byte(name))
			if err != nil || n == nil {
				return errors.Wrapf(err, "get node: %s", name)
			}

			showNode(n)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name")
	cmd.MarkFlagRequired("name")
	cmd.Flags().Int32Var(&height, "height", 0, "Height (inclusive)")
	cmd.Flags().SortFlags = false

	return cmd
}

func NewNodeChildrenCommand() *cobra.Command {

	var name string

	cmd := &cobra.Command{
		Use:   "children",
		Short: "Show all the children names of a given node name",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {

			dbPath := filepath.Join(dataDir, netName, "claim_dbs", cfg.NodeRepoPebble.Path)
			log.Debugf("Open node repo: %q", dbPath)
			repo, err := noderepo.NewPebble(dbPath)
			if err != nil {
				return errors.Wrapf(err, "open node repo")
			}
			defer repo.Close()

			fn := func(changes []change.Change) bool {
				fmt.Printf("Name: %s, Height: %d, %d\n", changes[0].Name, changes[0].Height,
					changes[len(changes)-1].Height)
				return true
			}

			err = repo.IterateChildren([]byte(name), fn)
			if err != nil {
				return errors.Wrapf(err, "iterate children: %s", name)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name")
	cmd.MarkFlagRequired("name")

	return cmd
}

func NewNodeStatsCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "stat",
		Short: "Determine the number of unique names, average changes per name, etc.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {

			dbPath := filepath.Join(dataDir, netName, "claim_dbs", cfg.NodeRepoPebble.Path)
			log.Debugf("Open node repo: %q", dbPath)
			repo, err := noderepo.NewPebble(dbPath)
			if err != nil {
				return errors.Wrapf(err, "open node repo")
			}
			defer repo.Close()

			n := 0
			c := 0
			err = repo.IterateChildren([]byte{}, func(changes []change.Change) bool {
				c += len(changes)
				n++
				if len(changes) > 5000 {
					fmt.Printf("Name: %s, Hex: %s, Changes: %d\n", string(changes[0].Name),
						hex.EncodeToString(changes[0].Name), len(changes))
				}
				return true
			})
			fmt.Printf("\nNames: %d, Average changes: %.2f\n", n, float64(c)/float64(n))
			return errors.Wrapf(err, "iterate node repo")
		},
	}

	return cmd
}
