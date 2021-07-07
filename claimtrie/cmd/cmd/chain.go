package cmd

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lbryio/lbcd/blockchain"
	"github.com/lbryio/lbcd/claimtrie"
	"github.com/lbryio/lbcd/claimtrie/chain"
	"github.com/lbryio/lbcd/claimtrie/chain/chainrepo"
	"github.com/lbryio/lbcd/claimtrie/change"
	"github.com/lbryio/lbcd/claimtrie/config"
	"github.com/lbryio/lbcd/database"
	_ "github.com/lbryio/lbcd/database/ffldb"
	"github.com/lbryio/lbcd/txscript"
	"github.com/lbryio/lbcd/wire"
	btcutil "github.com/lbryio/lbcutil"

	"github.com/cockroachdb/errors"
	"github.com/cockroachdb/pebble"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(NewChainCommands())
}

func NewChainCommands() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "chain",
		Short: "chain related command",
	}

	cmd.AddCommand(NewChainDumpCommand())
	cmd.AddCommand(NewChainReplayCommand())
	cmd.AddCommand(NewChainConvertCommand())

	return cmd
}

func NewChainDumpCommand() *cobra.Command {

	var chainRepoPath string
	var fromHeight int32
	var toHeight int32

	cmd := &cobra.Command{
		Use:   "dump",
		Short: "Dump the chain changes between <fromHeight> and <toHeight>",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {

			dbPath := chainRepoPath
			log.Debugf("Open chain repo: %q", dbPath)
			chainRepo, err := chainrepo.NewPebble(dbPath)
			if err != nil {
				return errors.Wrapf(err, "open chain repo")
			}

			for height := fromHeight; height <= toHeight; height++ {
				changes, err := chainRepo.Load(height)
				if errors.Is(err, pebble.ErrNotFound) {
					continue
				}
				if err != nil {
					return errors.Wrapf(err, "load charnges for height: %d")
				}
				for _, chg := range changes {
					showChange(chg)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&chainRepoPath, "chaindb", "chain_db", "Claim operation database")
	cmd.Flags().Int32Var(&fromHeight, "from", 0, "From height (inclusive)")
	cmd.Flags().Int32Var(&toHeight, "to", 0, "To height (inclusive)")
	cmd.Flags().SortFlags = false

	return cmd
}

func NewChainReplayCommand() *cobra.Command {

	var chainRepoPath string
	var fromHeight int32
	var toHeight int32

	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Replay the chain changes between <fromHeight> and <toHeight>",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {

			for _, dbName := range []string{
				cfg.BlockRepoPebble.Path,
				cfg.NodeRepoPebble.Path,
				cfg.MerkleTrieRepoPebble.Path,
				cfg.TemporalRepoPebble.Path,
			} {
				dbPath := filepath.Join(dataDir, netName, "claim_dbs", dbName)
				log.Debugf("Delete repo: %q", dbPath)
				err := os.RemoveAll(dbPath)
				if err != nil {
					return errors.Wrapf(err, "delete repo: %q", dbPath)
				}
			}

			log.Debugf("Open chain repo: %q", chainRepoPath)
			chainRepo, err := chainrepo.NewPebble(chainRepoPath)
			if err != nil {
				return errors.Wrapf(err, "open chain repo")
			}

			cfg := config.DefaultConfig
			cfg.RamTrie = true
			cfg.DataDir = filepath.Join(dataDir, netName)

			ct, err := claimtrie.New(cfg)
			if err != nil {
				return errors.Wrapf(err, "create claimtrie")
			}
			defer ct.Close()

			db, err := loadBlocksDB()
			if err != nil {
				return errors.Wrapf(err, "load blocks database")
			}

			chain, err := loadChain(db)
			if err != nil {
				return errors.Wrapf(err, "load chain")
			}

			startTime := time.Now()
			for ht := fromHeight; ht < toHeight; ht++ {

				changes, err := chainRepo.Load(ht + 1)
				if errors.Is(err, pebble.ErrNotFound) {
					// do nothing.
				} else if err != nil {
					return errors.Wrapf(err, "load changes for block %d", ht)
				}

				for _, chg := range changes {

					switch chg.Type {
					case change.AddClaim:
						err = ct.AddClaim(chg.Name, chg.OutPoint, chg.ClaimID, chg.Amount)
					case change.UpdateClaim:
						err = ct.UpdateClaim(chg.Name, chg.OutPoint, chg.Amount, chg.ClaimID)
					case change.SpendClaim:
						err = ct.SpendClaim(chg.Name, chg.OutPoint, chg.ClaimID)
					case change.AddSupport:
						err = ct.AddSupport(chg.Name, chg.OutPoint, chg.Amount, chg.ClaimID)
					case change.SpendSupport:
						err = ct.SpendSupport(chg.Name, chg.OutPoint, chg.ClaimID)
					default:
						err = errors.Errorf("invalid change type: %v", chg)
					}

					if err != nil {
						return errors.Wrapf(err, "execute change %v", chg)
					}
				}
				err = appendBlock(ct, chain)
				if err != nil {
					return errors.Wrapf(err, "appendBlock")
				}

				if time.Since(startTime) > 5*time.Second {
					log.Infof("Block: %d", ct.Height())
					startTime = time.Now()
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&chainRepoPath, "chaindb", "chain_db", "Claim operation database")
	cmd.Flags().Int32Var(&fromHeight, "from", 0, "From height")
	cmd.Flags().Int32Var(&toHeight, "to", 0, "To height")
	cmd.Flags().SortFlags = false

	return cmd
}

func appendBlock(ct *claimtrie.ClaimTrie, chain *blockchain.BlockChain) error {

	err := ct.AppendBlock(false)
	if err != nil {
		return errors.Wrapf(err, "append block: %w")
	}

	blockHash, err := chain.BlockHashByHeight(ct.Height())
	if err != nil {
		return errors.Wrapf(err, "load from block repo: %w")
	}

	header, err := chain.HeaderByHash(blockHash)

	if err != nil {
		return errors.Wrapf(err, "load from block repo: %w")
	}

	if *ct.MerkleHash() != header.ClaimTrie {
		return errors.Errorf("hash mismatched at height %5d: exp: %s, got: %s",
			ct.Height(), header.ClaimTrie, ct.MerkleHash())
	}

	return nil
}

func NewChainConvertCommand() *cobra.Command {

	var chainRepoPath string
	var toHeight int32

	cmd := &cobra.Command{
		Use:   "convert",
		Short: "convert changes from 0 to <toHeight>",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {

			db, err := loadBlocksDB()
			if err != nil {
				return errors.Wrapf(err, "load block db")
			}
			defer db.Close()

			chain, err := loadChain(db)
			if err != nil {
				return errors.Wrapf(err, "load block db")
			}

			if toHeight > chain.BestSnapshot().Height {
				toHeight = chain.BestSnapshot().Height
			}

			chainRepo, err := chainrepo.NewPebble(chainRepoPath)
			if err != nil {
				return errors.Wrapf(err, "open chain repo: %v")
			}
			defer chainRepo.Close()

			converter := chainConverter{
				db:          db,
				chain:       chain,
				chainRepo:   chainRepo,
				toHeight:    toHeight,
				blockChan:   make(chan *btcutil.Block, 1000),
				changesChan: make(chan []change.Change, 1000),
				wg:          &sync.WaitGroup{},
				stat:        &stat{},
			}

			startTime := time.Now()
			err = converter.start()
			if err != nil {
				return errors.Wrapf(err, "start Converter")
			}

			converter.wait()
			log.Infof("Convert chain: took %s", time.Since(startTime))

			return nil
		},
	}

	cmd.Flags().StringVar(&chainRepoPath, "chaindb", "chain_db", "Claim operation database")
	cmd.Flags().Int32Var(&toHeight, "to", 0, "toHeight")
	cmd.Flags().SortFlags = false
	return cmd
}

type stat struct {
	blocksFetched   int
	blocksProcessed int
	changesSaved    int
}

type chainConverter struct {
	db        database.DB
	chain     *blockchain.BlockChain
	chainRepo chain.Repo
	toHeight  int32

	blockChan   chan *btcutil.Block
	changesChan chan []change.Change

	wg *sync.WaitGroup

	stat *stat
}

func (cc *chainConverter) wait() {
	cc.wg.Wait()
}

func (cb *chainConverter) start() error {

	go cb.reportStats()

	cb.wg.Add(3)
	go cb.getBlock()
	go cb.processBlock()
	go cb.saveChanges()

	return nil
}

func (cb *chainConverter) getBlock() {
	defer cb.wg.Done()
	defer close(cb.blockChan)

	for ht := int32(0); ht < cb.toHeight; ht++ {
		block, err := cb.chain.BlockByHeight(ht)
		if err != nil {
			if errors.Cause(err).Error() == "too many open files" {
				err = errors.WithHintf(err, "try ulimit -n 2048")
			}
			log.Errorf("load changes at %d: %s", ht, err)
			return
		}
		cb.stat.blocksFetched++
		cb.blockChan <- block
	}
}

func (cb *chainConverter) processBlock() {
	defer cb.wg.Done()
	defer close(cb.changesChan)

	utxoPubScripts := map[wire.OutPoint][]byte{}
	for block := range cb.blockChan {
		var changes []change.Change
		for _, tx := range block.Transactions() {

			if blockchain.IsCoinBase(tx) {
				continue
			}

			for _, txIn := range tx.MsgTx().TxIn {
				prevOutpoint := txIn.PreviousOutPoint
				pkScript := utxoPubScripts[prevOutpoint]
				cs, err := txscript.ExtractClaimScript(pkScript)
				if txscript.IsErrorCode(err, txscript.ErrNotClaimScript) {
					continue
				}
				if err != nil {
					log.Criticalf("Can't parse claim script: %s", err)
				}

				chg := change.Change{
					Height:   block.Height(),
					Name:     cs.Name,
					OutPoint: txIn.PreviousOutPoint,
				}
				delete(utxoPubScripts, prevOutpoint)

				switch cs.Opcode {
				case txscript.OP_CLAIMNAME:
					chg.Type = change.SpendClaim
					chg.ClaimID = change.NewClaimID(chg.OutPoint)
				case txscript.OP_UPDATECLAIM:
					chg.Type = change.SpendClaim
					copy(chg.ClaimID[:], cs.ClaimID)
				case txscript.OP_SUPPORTCLAIM:
					chg.Type = change.SpendSupport
					copy(chg.ClaimID[:], cs.ClaimID)
				}

				changes = append(changes, chg)
			}

			op := *wire.NewOutPoint(tx.Hash(), 0)
			for i, txOut := range tx.MsgTx().TxOut {
				cs, err := txscript.ExtractClaimScript(txOut.PkScript)
				if txscript.IsErrorCode(err, txscript.ErrNotClaimScript) {
					continue
				}

				op.Index = uint32(i)
				chg := change.Change{
					Height:   block.Height(),
					Name:     cs.Name,
					OutPoint: op,
					Amount:   txOut.Value,
				}
				utxoPubScripts[op] = txOut.PkScript

				switch cs.Opcode {
				case txscript.OP_CLAIMNAME:
					chg.Type = change.AddClaim
					chg.ClaimID = change.NewClaimID(op)
				case txscript.OP_SUPPORTCLAIM:
					chg.Type = change.AddSupport
					copy(chg.ClaimID[:], cs.ClaimID)
				case txscript.OP_UPDATECLAIM:
					chg.Type = change.UpdateClaim
					copy(chg.ClaimID[:], cs.ClaimID)
				}
				changes = append(changes, chg)
			}
		}
		cb.stat.blocksProcessed++

		if len(changes) != 0 {
			cb.changesChan <- changes
		}
	}
}

func (cb *chainConverter) saveChanges() {
	defer cb.wg.Done()

	for changes := range cb.changesChan {
		err := cb.chainRepo.Save(changes[0].Height, changes)
		if err != nil {
			log.Errorf("save to chain repo: %s", err)
			return
		}
		cb.stat.changesSaved++
	}
}

func (cb *chainConverter) reportStats() {
	stat := cb.stat
	tick := time.NewTicker(5 * time.Second)
	for range tick.C {
		log.Infof("block : %7d / %7d,  changes saved: %d",
			stat.blocksFetched, stat.blocksProcessed, stat.changesSaved)

	}
}
