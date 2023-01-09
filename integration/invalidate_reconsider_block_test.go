package integration

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/integration/rpctest"
)

func TestInvalidateAndReconsiderBlock(t *testing.T) {
	// Set up regtest chain.
	r, err := rpctest.New(&chaincfg.RegressionNetParams, nil, nil, "")
	if err != nil {
		t.Fatalf("TestInvalidateAndReconsiderBlock fail."+
			"Unable to create primary harness: %v", err)
	}
	if err := r.SetUp(true, 0); err != nil {
		t.Fatalf("TestInvalidateAndReconsiderBlock fail. "+
			"Unable to setup test chain: %v", err)
	}
	defer r.TearDown()

	// Generate 4 blocks.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2 -> 3 -> 4
	_, err = r.Client.Generate(4)
	if err != nil {
		t.Fatal(err)
	}

	// Cache the active tip hash.
	block4ActiveTipHash, err := r.Client.GetBestBlockHash()
	if err != nil {
		t.Fatal(err)
	}

	// Cache block 1 hash as this will be our chaintip after we invalidate block 2.
	block1Hash, err := r.Client.GetBlockHash(1)
	if err != nil {
		t.Fatal(err)
	}

	// Invalidate block 2.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1                 (active)
	//                    \ -> 2 -> 3 -> 4  (invalid)
	block2Hash, err := r.Client.GetBlockHash(2)
	if err != nil {
		t.Fatal(err)
	}
	err = r.Client.InvalidateBlock(block2Hash)
	if err != nil {
		t.Fatal(err)
	}

	// Assert that block 1 is the active chaintip.
	bestHash, err := r.Client.GetBestBlockHash()
	if *bestHash != *block1Hash {
		t.Fatalf("TestInvalidateAndReconsiderBlock fail. Expected the "+
			"best block hash to be block 1 with hash %s but got %s",
			block1Hash.String(), bestHash.String())
	}

	// Generate 2 blocks.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2a -> 3a      (active)
	//                    \ -> 2  -> 3  -> 4 (invalid)
	_, err = r.Client.Generate(2)
	if err != nil {
		t.Fatal(err)
	}

	// Cache the active tip hash for the current active tip.
	block3aActiveTipHash, err := r.Client.GetBestBlockHash()
	if err != nil {
		t.Fatal(err)
	}

	tips, err := r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}

	// Assert that there are two branches.
	if len(tips) != 2 {
		t.Fatalf("TestInvalidateAndReconsiderBlock fail. "+
			"Expected 2 chaintips but got %d", len(tips))
	}

	for _, tip := range tips {
		if tip.Hash == block4ActiveTipHash.String() &&
			tip.Status != "invalid" {
			t.Fatalf("TestInvalidateAndReconsiderBlock fail. Expected "+
				"invalidated branch tip of %s to be invalid but got %s",
				tip.Hash, tip.Status)
		}
	}

	// Reconsider the invalidated block 2.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2a -> 3a       (valid-fork)
	//                    \ -> 2  -> 3  -> 4  (active)
	err = r.Client.ReconsiderBlock(block2Hash)
	if err != nil {
		t.Fatal(err)
	}

	tips, err = r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}
	// Assert that there are two branches.
	if len(tips) != 2 {
		t.Fatalf("TestInvalidateAndReconsiderBlock fail. "+
			"Expected 2 chaintips but got %d", len(tips))
	}

	var checkedTips int
	for _, tip := range tips {
		if tip.Hash == block4ActiveTipHash.String() {
			if tip.Status != "active" {
				t.Fatalf("TestInvalidateAndReconsiderBlock fail. Expected "+
					"the reconsidered branch tip of %s to be active but got %s",
					tip.Hash, tip.Status)
			}

			checkedTips++
		}

		if tip.Hash == block3aActiveTipHash.String() {
			if tip.Status != "valid-fork" {
				t.Fatalf("TestInvalidateAndReconsiderBlock fail. Expected "+
					"invalidated branch tip of %s to be valid-fork but got %s",
					tip.Hash, tip.Status)
			}
			checkedTips++
		}
	}

	if checkedTips != 2 {
		t.Fatalf("TestInvalidateAndReconsiderBlock fail. "+
			"Expected to check %d chaintips, checked %d", 2, checkedTips)
	}

	// Invalidate block 3a.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2a -> 3a       (invalid)
	//                    \ -> 2  -> 3  -> 4  (active)
	err = r.Client.InvalidateBlock(block3aActiveTipHash)
	if err != nil {
		t.Fatal(err)
	}

	tips, err = r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}

	// Assert that there are two branches.
	if len(tips) != 2 {
		t.Fatalf("TestInvalidateAndReconsiderBlock fail. "+
			"Expected 2 chaintips but got %d", len(tips))
	}

	checkedTips = 0
	for _, tip := range tips {
		if tip.Hash == block4ActiveTipHash.String() {
			if tip.Status != "active" {
				t.Fatalf("TestInvalidateAndReconsiderBlock fail. Expected "+
					"an active branch tip of %s but got %s",
					tip.Hash, tip.Status)
			}

			checkedTips++
		}

		if tip.Hash == block3aActiveTipHash.String() {
			if tip.Status != "invalid" {
				t.Fatalf("TestInvalidateAndReconsiderBlock fail. Expected "+
					"the invalidated tip of %s to be invalid but got %s",
					tip.Hash, tip.Status)
			}
			checkedTips++
		}
	}

	if checkedTips != 2 {
		t.Fatalf("TestInvalidateAndReconsiderBlock fail. "+
			"Expected to check %d chaintips, checked %d", 2, checkedTips)
	}

	// Reconsider block 3a.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2a -> 3a       (valid-fork)
	//                    \ -> 2  -> 3  -> 4  (active)
	err = r.Client.ReconsiderBlock(block3aActiveTipHash)
	if err != nil {
		t.Fatal(err)
	}

	tips, err = r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}

	// Assert that there are two branches.
	if len(tips) != 2 {
		t.Fatalf("TestInvalidateAndReconsiderBlock fail. "+
			"Expected 2 chaintips but got %d", len(tips))
	}

	checkedTips = 0
	for _, tip := range tips {
		if tip.Hash == block4ActiveTipHash.String() {
			if tip.Status != "active" {
				t.Fatalf("TestInvalidateAndReconsiderBlock fail. Expected "+
					"an active branch tip of %s but got %s",
					tip.Hash, tip.Status)
			}

			checkedTips++
		}

		if tip.Hash == block3aActiveTipHash.String() {
			if tip.Status != "valid-fork" {
				t.Fatalf("TestInvalidateAndReconsiderBlock fail. Expected "+
					"the reconsidered tip of %s to be a valid-fork but got %s",
					tip.Hash, tip.Status)
			}
			checkedTips++
		}
	}

	if checkedTips != 2 {
		t.Fatalf("TestInvalidateAndReconsiderBlock fail. "+
			"Expected to check %d chaintips, checked %d", 2, checkedTips)
	}
}
