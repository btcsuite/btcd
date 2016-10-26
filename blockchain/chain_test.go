// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// TestHaveBlock tests the HaveBlock API to ensure proper functionality.
func TestHaveBlock(t *testing.T) {
	// Load up blocks such that there is a side chain.
	// (genesis block) -> 1 -> 2 -> 3 -> 4
	//                          \-> 3a
	testFiles := []string{
		"blk_0_to_4.dat.bz2",
		"blk_3A.dat.bz2",
	}

	var blocks []*btcutil.Block
	for _, file := range testFiles {
		blockTmp, err := loadBlocks(file)
		if err != nil {
			t.Errorf("Error loading file: %v\n", err)
			return
		}
		blocks = append(blocks, blockTmp...)
	}

	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("haveblock",
		&chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	// Since we're not dealing with the real block chain, set the coinbase
	// maturity to 1.
	chain.TstSetCoinbaseMaturity(1)

	for i := 1; i < len(blocks); i++ {
		_, isOrphan, err := chain.ProcessBlock(blocks[i], blockchain.BFNone)
		if err != nil {
			t.Errorf("ProcessBlock fail on block %v: %v\n", i, err)
			return
		}
		if isOrphan {
			t.Errorf("ProcessBlock incorrectly returned block %v "+
				"is an orphan\n", i)
			return
		}
	}

	// Insert an orphan block.
	_, isOrphan, err := chain.ProcessBlock(btcutil.NewBlock(&Block100000),
		blockchain.BFNone)
	if err != nil {
		t.Errorf("Unable to process block: %v", err)
		return
	}
	if !isOrphan {
		t.Errorf("ProcessBlock indicated block is an not orphan when " +
			"it should be\n")
		return
	}

	tests := []struct {
		hash string
		want bool
	}{
		// Genesis block should be present (in the main chain).
		{hash: chaincfg.MainNetParams.GenesisHash.String(), want: true},

		// Block 3a should be present (on a side chain).
		{hash: "00000000474284d20067a4d33f6a02284e6ef70764a3a26d6a5b9df52ef663dd", want: true},

		// Block 100000 should be present (as an orphan).
		{hash: "000000000003ba27aa200b1cecaad478d2b00432346c3f1f3986da1afd33e506", want: true},

		// Random hashes should not be available.
		{hash: "123", want: false},
	}

	for i, test := range tests {
		hash, err := chainhash.NewHashFromStr(test.hash)
		if err != nil {
			t.Errorf("NewHashFromStr: %v", err)
			continue
		}

		result, err := chain.HaveBlock(hash)
		if err != nil {
			t.Errorf("HaveBlock #%d unexpected error: %v", i, err)
			return
		}
		if result != test.want {
			t.Errorf("HaveBlock #%d got %v want %v", i, result,
				test.want)
			continue
		}
	}
}

// TestCalcSequenceLock tests the LockTimeToSequence function, and the
// CalcSequenceLock method of a Chain instance. The tests exercise several
// combinations of inputs to the CalcSequenceLock function in order to ensure
// the returned SequenceLocks are correct for each test instance.
func TestCalcSequenceLock(t *testing.T) {
	netParams := &chaincfg.SimNetParams

	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("calcseqlock", netParams)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	// Since we're not dealing with the real block chain, set the coinbase
	// maturity to 1.
	chain.TstSetCoinbaseMaturity(1)

	// Create a test mining address to use for the blocks we'll generate
	// shortly below.
	k := bytes.Repeat([]byte{1}, 32)
	_, miningPub := btcec.PrivKeyFromBytes(btcec.S256(), k)
	miningAddr, err := btcutil.NewAddressPubKey(miningPub.SerializeCompressed(),
		netParams)
	if err != nil {
		t.Fatalf("unable to generate mining addr: %v", err)
	}

	// We'll keep track of the previous block for back pointers in blocks
	// we generated, and also the generated blocks along with the MTP from
	// their PoV to aide with our relative time lock calculations.
	var prevBlock *btcutil.Block
	var blocksWithMTP []struct {
		block *btcutil.Block
		mtp   time.Time
	}

	// We need to activate CSV in order to test the processing logic, so
	// manually craft the block version that's used to signal the soft-fork
	// activation.
	csvBit := netParams.Deployments[chaincfg.DeploymentCSV].BitNumber
	blockVersion := int32(0x20000000 | (uint32(1) << csvBit))

	// Generate enough blocks to activate CSV, collecting each of the
	// blocks into a slice for later use.
	numBlocksToActivate := (netParams.MinerConfirmationWindow * 3)
	for i := uint32(0); i < numBlocksToActivate; i++ {
		block, err := rpctest.CreateBlock(prevBlock, nil, blockVersion,
			time.Time{}, miningAddr, netParams)
		if err != nil {
			t.Fatalf("unable to generate block: %v", err)
		}

		mtp := chain.BestSnapshot().MedianTime

		_, isOrphan, err := chain.ProcessBlock(block, blockchain.BFNone)
		if err != nil {
			t.Fatalf("ProcessBlock fail on block %v: %v\n", i, err)
		}
		if isOrphan {
			t.Fatalf("ProcessBlock incorrectly returned block %v "+
				"is an orphan\n", i)
		}

		blocksWithMTP = append(blocksWithMTP, struct {
			block *btcutil.Block
			mtp   time.Time
		}{
			block: block,
			mtp:   mtp,
		})

		prevBlock = block
	}

	// Create a utxo view with all the utxos within the blocks created
	// above.
	utxoView := blockchain.NewUtxoViewpoint()
	for blockHeight, blockWithMTP := range blocksWithMTP {
		for _, tx := range blockWithMTP.block.Transactions() {
			utxoView.AddTxOuts(tx, int32(blockHeight))
		}
	}
	utxoView.SetBestHash(blocksWithMTP[len(blocksWithMTP)-1].block.Hash())

	// The median time calculated from the PoV of the best block in our
	// test chain. For unconfirmed inputs, this value will be used since
	// the MTP will be calculated from the PoV of the yet-to-be-mined
	// block.
	nextMedianTime := int64(1401292712)

	// We'll refer to this utxo within each input in the transactions
	// created below. This utxo has an age of 4 blocks and was mined within
	// block 297
	targetTx := blocksWithMTP[len(blocksWithMTP)-4].block.Transactions()[0]
	utxo := wire.OutPoint{
		Hash:  *targetTx.Hash(),
		Index: 0,
	}

	// Obtain the median time past from the PoV of the input created above.
	// The MTP for the input is the MTP from the PoV of the block *prior*
	// to the one that included it.
	medianTime := blocksWithMTP[len(blocksWithMTP)-5].mtp.Unix()

	// Add an additional transaction which will serve as our unconfirmed
	// output.
	var fakeScript []byte
	unConfTx := &wire.MsgTx{
		TxOut: []*wire.TxOut{{
			PkScript: fakeScript,
			Value:    5,
		}},
	}
	unConfUtxo := wire.OutPoint{
		Hash:  unConfTx.TxHash(),
		Index: 0,
	}

	// Adding a utxo with a height of 0x7fffffff indicates that the output
	// is currently unmined.
	utxoView.AddTxOuts(btcutil.NewTx(unConfTx), 0x7fffffff)

	tests := []struct {
		tx      *btcutil.Tx
		view    *blockchain.UtxoViewpoint
		want    *blockchain.SequenceLock
		mempool bool
	}{
		// A transaction of version one should disable sequence locks
		// as the new sequence number semantics only apply to
		// transactions version 2 or higher.
		{
			tx: btcutil.NewTx(&wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(false, 3),
				}},
			}),
			view: utxoView,
			want: &blockchain.SequenceLock{
				Seconds:     -1,
				BlockHeight: -1,
			},
		},
		// A transaction with a single input, that a max int sequence
		// number. This sequence number has the high bit set, so
		// sequence locks should be disabled.
		{
			tx: btcutil.NewTx(&wire.MsgTx{
				Version: 2,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: utxo,
					Sequence:         wire.MaxTxInSequenceNum,
				}},
			}),
			view: utxoView,
			want: &blockchain.SequenceLock{
				Seconds:     -1,
				BlockHeight: -1,
			},
		},
		// A transaction with a single input whose lock time is
		// expressed in seconds. However, the specified lock time is
		// below the required floor for time based lock times since
		// they have time granularity of 512 seconds. As a result, the
		// seconds lock-time should be just before the median time of
		// the targeted block.
		{
			tx: btcutil.NewTx(&wire.MsgTx{
				Version: 2,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(true, 2),
				}},
			}),
			view: utxoView,
			want: &blockchain.SequenceLock{
				Seconds:     medianTime - 1,
				BlockHeight: -1,
			},
		},
		// A transaction with a single input whose lock time is
		// expressed in seconds. The number of seconds should be 1023
		// seconds after the median past time of the last block in the
		// chain.
		{
			tx: btcutil.NewTx(&wire.MsgTx{
				Version: 2,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(true, 1024),
				}},
			}),
			view: utxoView,
			want: &blockchain.SequenceLock{
				Seconds:     medianTime + 1023,
				BlockHeight: -1,
			},
		},
		// A transaction with multiple inputs. The first input has a
		// sequence lock in blocks with a value of 4. The last input
		// has a sequence number with a value of 5, but has the disable
		// bit set. So the first lock should be selected as it's the
		// target lock as its the furthest in the future lock that
		// isn't disabled.
		{
			tx: btcutil.NewTx(&wire.MsgTx{
				Version: 2,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(true, 2560),
				}, {
					PreviousOutPoint: utxo,
					Sequence: blockchain.LockTimeToSequence(false, 5) |
						wire.SequenceLockTimeDisabled,
				}, {
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(false, 4),
				}},
			}),
			view: utxoView,
			want: &blockchain.SequenceLock{
				Seconds:     medianTime + (5 << wire.SequenceLockTimeGranularity) - 1,
				BlockHeight: 299,
			},
		},
		// Transaction has a single input spending the genesis block
		// transaction. The input's sequence number is encodes a
		// relative lock-time in blocks (3 blocks). The sequence lock
		// should have a value of -1 for seconds, but a block height of
		// 298 meaning it can be included at height 299.
		{
			tx: btcutil.NewTx(&wire.MsgTx{
				Version: 2,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(false, 3),
				}},
			}),
			view: utxoView,
			want: &blockchain.SequenceLock{
				Seconds:     -1,
				BlockHeight: 298,
			},
		},
		// A transaction with two inputs with lock times expressed in
		// seconds. The selected sequence lock value for seconds should
		// be the time further in the future.
		{
			tx: btcutil.NewTx(&wire.MsgTx{
				Version: 2,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(true, 5120),
				}, {
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(true, 2560),
				}},
			}),
			view: utxoView,
			want: &blockchain.SequenceLock{
				Seconds:     medianTime + (10 << wire.SequenceLockTimeGranularity) - 1,
				BlockHeight: -1,
			},
		},
		// A transaction with two inputs with lock times expressed in
		// seconds. The selected sequence lock value for blocks should
		// be the height further in the future. The converted absolute
		// block height should be 302, meaning it can be included in
		// block 303.
		{
			tx: btcutil.NewTx(&wire.MsgTx{
				Version: 2,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(false, 1),
				}, {
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(false, 7),
				}},
			}),
			view: utxoView,
			want: &blockchain.SequenceLock{
				Seconds:     -1,
				BlockHeight: 302,
			},
		},
		// A transaction with multiple inputs. Two inputs are time
		// based, and the other two are input maturity based. The lock
		// lying further into the future for both inputs should be
		// chosen.
		{
			tx: btcutil.NewTx(&wire.MsgTx{
				Version: 2,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(true, 2560),
				}, {
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(true, 6656),
				}, {
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(false, 3),
				}, {
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(false, 9),
				}},
			}),
			view: utxoView,
			want: &blockchain.SequenceLock{
				Seconds:     medianTime + (13 << wire.SequenceLockTimeGranularity) - 1,
				BlockHeight: 304,
			},
		},
		// A transaction with a single unconfirmed input. As the input
		// is confirmed, the height of the input should be interpreted
		// as the height of the *next* block. The current block height
		// is 300, so the lock time should be calculated using height
		// 301 as a base. A 2 block relative lock means the transaction
		// can be included after block 302, so in 303.
		{
			tx: btcutil.NewTx(&wire.MsgTx{
				Version: 2,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: unConfUtxo,
					Sequence:         blockchain.LockTimeToSequence(false, 2),
				}},
			}),
			view: utxoView,
			want: &blockchain.SequenceLock{
				Seconds:     -1,
				BlockHeight: 302,
			},
		},
		// A transaction with a single unconfirmed input. The input has
		// a time based lock, so the lock time should be based off the
		// MTP of the *next* block.
		{
			tx: btcutil.NewTx(&wire.MsgTx{
				Version: 2,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: unConfUtxo,
					Sequence:         blockchain.LockTimeToSequence(true, 1024),
				}},
			}),
			view: utxoView,
			want: &blockchain.SequenceLock{
				Seconds:     nextMedianTime + 1023,
				BlockHeight: -1,
			},
		},
	}

	t.Logf("Running %v SequenceLock tests", len(tests))
	for i, test := range tests {
		seqLock, err := chain.CalcSequenceLock(test.tx, test.view, test.mempool)
		if err != nil {
			t.Fatalf("test #%d, unable to calc sequence lock: %v", i, err)
		}

		if seqLock.Seconds != test.want.Seconds {
			t.Fatalf("test #%d got %v seconds want %v seconds",
				i, seqLock.Seconds, test.want.Seconds)
		}
		if seqLock.BlockHeight != test.want.BlockHeight {
			t.Fatalf("test #%d got height of %v want height of %v ",
				i, seqLock.BlockHeight, test.want.BlockHeight)
		}
	}
}
