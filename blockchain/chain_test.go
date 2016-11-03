// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
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

	// Since we're not dealing with the real block chain, disable
	// checkpoints and set the coinbase maturity to 1.
	chain.DisableCheckpoints(true)
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
	fileName := "blk_0_to_4.dat.bz2"
	blocks, err := loadBlocks(fileName)
	if err != nil {
		t.Errorf("Error loading file: %v\n", err)
		return
	}

	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("haveblock", &chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	// Since we're not dealing with the real block chain, disable
	// checkpoints and set the coinbase maturity to 1.
	chain.DisableCheckpoints(true)
	chain.TstSetCoinbaseMaturity(1)

	// Load all the blocks into our test chain.
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

	// Create with all the utxos within the create created above.
	utxoView := blockchain.NewUtxoViewpoint()
	for blockHeight, block := range blocks {
		for _, tx := range block.Transactions() {
			utxoView.AddTxOuts(tx, int32(blockHeight))
		}
	}
	utxoView.SetBestHash(blocks[len(blocks)-1].Hash())

	// The median past time from the point of view of the second to last
	// block in the chain.
	medianTime := blocks[2].MsgBlock().Header.Timestamp.Unix()

	// The median past time of the *next* block will be the timestamp of
	// the 2nd block due to the way MTP is calculated in order to be
	// compatible with Bitcoin Core.
	nextMedianTime := blocks[2].MsgBlock().Header.Timestamp.Unix()

	// We'll refer to this utxo within each input in the transactions
	// created below. This block that includes this UTXO has a height of 4.
	targetTx := blocks[4].Transactions()[0]
	utxo := wire.OutPoint{
		Hash:  *targetTx.Hash(),
		Index: 0,
	}

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
					Sequence: blockchain.LockTimeToSequence(false, 3) |
						wire.SequenceLockTimeDisabled,
				}, {
					PreviousOutPoint: utxo,
					Sequence:         blockchain.LockTimeToSequence(false, 3),
				}},
			}),
			view: utxoView,
			want: &blockchain.SequenceLock{
				Seconds:     medianTime + (5 << wire.SequenceLockTimeGranularity) - 1,
				BlockHeight: 6,
			},
		},
		// Transaction has a single input spending the genesis block
		// transaction. The input's sequence number is encodes a
		// relative lock-time in blocks (3 blocks). The sequence lock
		// should have a value of -1 for seconds, but a block height of
		// 6 meaning it can be included at height 7.
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
				BlockHeight: 6,
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
		// be the height further in the future, so a height of 10
		// indicating in can be included at height 7.
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
				BlockHeight: 10,
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
				BlockHeight: 12,
			},
		},
		// A transaction with a single unconfirmed input. As the input
		// is confirmed, the height of the input should be interpreted
		// as the height of the *next* block. So the relative block
		// lock should be based from a height of 5 rather than a height
		// of 4.
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
				BlockHeight: 6,
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
