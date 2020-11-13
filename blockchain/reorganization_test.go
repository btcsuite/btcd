package blockchain

import (
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"testing"
)

// TestReorganization loads a set of test blocks which force a chain
// reorganization to test the block chain handling code.
// The test blocks were originally from a post on the bitcoin talk forums:
// https://bitcointalk.org/index.php?topic=46370.msg577556#msg577556
func TestReorganization(t *testing.T) {
	// Intentionally load the side chain blocks out of order to ensure
	// orphans are handled properly along with chain reorganization.
	testFiles := []string{
		"blk_0_to_4.dat.bz2",
		"blk_4A.dat.bz2",
		"blk_5A.dat.bz2",
		"blk_3A.dat.bz2",
	}

	var blocks []*btcutil.Block
	for _, file := range testFiles {
		blockTmp, err := loadBlocks(file)
		if err != nil {
			t.Errorf("Error loading file: %v\n", err)
		}
		blocks = append(blocks, blockTmp...)
	}

	t.Logf("Number of blocks: %v\n", len(blocks))

	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("reorg", &chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	// Since we're not dealing with the real block chain set the coinbase
	// maturity to 1.
	chain.TstSetCoinbaseMaturity(1)

	expectedOrphans := map[int]struct{}{5: {}, 6: {}}
	for i := 1; i < len(blocks); i++ {
		_, isOrphan, err := chain.ProcessBlock(blocks[i], BFNone)
		if err != nil {
			t.Errorf("ProcessBlock fail on block %v: %v\n", i, err)
			return
		}
		if _, ok := expectedOrphans[i]; !ok && isOrphan {
			t.Errorf("ProcessBlock incorrectly returned block %v "+
				"is an orphan\n", i)
		}
	}
}
