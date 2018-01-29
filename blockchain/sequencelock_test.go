// Copyright (c) 2017-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
)

// mustLockTimeToSeq converts the passed relative lock time to a sequence number
// by using LockTimeToSequence.  It only differs in that it will panic if there
// is an error so errors in the source code can be detected.  It will only (and
// must only)  be called with hard-coded, and therefore known good, values.
func mustLockTimeToSeq(isSeconds bool, lockTime uint32) uint32 {
	sequence, err := LockTimeToSequence(isSeconds, lockTime)
	if err != nil {
		panic(fmt.Sprintf("invalid lock time in source file: "+
			"isSeconds: %v, lockTime: %d", isSeconds, lockTime))
	}
	return sequence
}

// TestCalcSequenceLock exercises several combinations of inputs to the
// CalcSequenceLock function in order to ensure the returned sequence locks are
// as expected.
func TestCalcSequenceLock(t *testing.T) {
	// Generate a synthetic simnet chain with enough nodes to properly test
	// the sequence lock functionality.
	numBlocks := uint32(20)
	params := &chaincfg.SimNetParams
	bc := newFakeChain(params)
	node := bc.bestNode
	blockTime := time.Unix(node.timestamp, 0)
	for i := uint32(0); i < numBlocks; i++ {
		blockTime = blockTime.Add(time.Second)
		node = newFakeNode(node, 1, 1, 0, blockTime)
		bc.index.AddNode(node)
		bc.bestNode = node
	}

	// Create a utxo view with a fake utxo for the inputs used in the
	// transactions created below.  This utxo is added such that it has an
	// age of 4 blocks.
	targetTx := dcrutil.NewTx(&wire.MsgTx{
		TxOut: []*wire.TxOut{{
			Value:    10,
			Version:  0,
			PkScript: nil,
		}},
	})
	view := NewUtxoViewpoint()
	view.AddTxOuts(targetTx, int64(numBlocks)-4, 0)
	view.SetBestHash(&node.hash)

	// Create a utxo that spends the fake utxo created above for use in the
	// transactions created in the tests.  It has an age of 4 blocks.  Note
	// that the sequence lock heights are always calculated from the same
	// point of view that they were originally calculated from for a given
	// utxo.  That is to say, the height prior to it.
	utxo := wire.OutPoint{
		Hash:  *targetTx.Hash(),
		Index: 0,
		Tree:  wire.TxTreeRegular,
	}
	prevUtxoHeight := int64(numBlocks) - 4

	// Obtain the median time past from the PoV of the input created above.
	// The median time for the input is the median time from the PoV of the
	// block *prior* to the one that included it.
	medianNode, err := bc.index.AncestorNode(node, node.height-5)
	if err != nil {
		t.Fatalf("Unable to obtain median node: %v", err)
	}
	medianT, err := bc.index.CalcPastMedianTime(medianNode)
	if err != nil {
		t.Fatalf("Unable to obtain median node time: %v", err)
	}
	medianTime := medianT.Unix()

	// The median time calculated from the PoV of the best block in the
	// test chain.  For unconfirmed inputs, this value will be used since
	// the median time will be calculated from the PoV of the
	// yet-to-be-mined block.
	nextMedianT, err := bc.index.CalcPastMedianTime(node)
	if err != nil {
		t.Fatalf("Unable to obtain next median node time: %v", err)
	}
	nextMedianTime := nextMedianT.Unix()
	nextBlockHeight := int64(numBlocks) + 1

	// Add an additional transaction which will serve as our unconfirmed
	// output.
	unConfTx := &wire.MsgTx{
		TxOut: []*wire.TxOut{{
			Value:    5,
			Version:  0,
			PkScript: nil,
		}},
	}
	unConfUtxo := wire.OutPoint{
		Hash:  unConfTx.TxHash(),
		Index: 0,
		Tree:  wire.TxTreeRegular,
	}

	// Adding a utxo with a height of 0x7fffffff indicates that the output
	// is currently unmined.
	view.AddTxOuts(dcrutil.NewTx(unConfTx), 0x7fffffff, wire.NullBlockIndex)

	tests := []struct {
		name      string
		txVersion uint16
		inputs    []*wire.TxIn
		isActive  bool
		want      SequenceLock
	}{
		{
			// A transaction of version one should disable sequence
			// locks as the new sequence number semantics only apply
			// to transactions version 2 or higher.
			name:      "v1 transaction",
			txVersion: 1,
			inputs: []*wire.TxIn{{
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(false, 3),
			}},
			isActive: true,
			want: SequenceLock{
				MinHeight: -1,
				MinTime:   -1,
			},
		},
		{
			// A transaction with a single input with max sequence
			// number.  This sequence number has the high bit set,
			// so sequence locks should be disabled.
			name:      "max sequence number",
			txVersion: 2,
			inputs: []*wire.TxIn{{
				PreviousOutPoint: utxo,
				Sequence:         wire.MaxTxInSequenceNum,
			}},
			isActive: true,
			want: SequenceLock{
				MinHeight: -1,
				MinTime:   -1,
			},
		},
		{
			// A transaction that would result in a specific
			// sequence lock except set the agenda is not being
			// active yet, so sequence locks should be disabled.
			name:      "agenda not yet active",
			txVersion: 2,
			inputs: []*wire.TxIn{{
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(true, 2),
			}},
			isActive: false,
			want: SequenceLock{
				MinHeight: -1,
				MinTime:   -1,
			},
		},
		{
			// A transaction with a single input whose locktime is
			// expressed in seconds.  However, the specified lock
			// time is below the required floor for time based lock
			// times since they have time granularity of 512
			// seconds.  As a result, the seconds locktime should be
			// just before the median time of the targeted block.
			name:      "seconds below granularity",
			txVersion: 2,
			inputs: []*wire.TxIn{{
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(true, 2),
			}},
			isActive: true,
			want: SequenceLock{
				MinHeight: -1,
				MinTime:   medianTime - 1,
			},
		},
		{
			// A transaction with a single input whose locktime is
			// expressed in seconds.  The number of seconds should
			// be 1023 seconds after the median past time of the
			// input.
			name:      "1024 seconds",
			txVersion: 2,
			inputs: []*wire.TxIn{{
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(true, 1024),
			}},
			isActive: true,
			want: SequenceLock{
				MinHeight: -1,
				MinTime:   medianTime + 1023,
			},
		},
		{
			// A transaction with multiple inputs.  The first input
			// has a locktime expressed in seconds.  The second
			// input has a sequence lock in blocks with a value of
			// 4.  The last input has a sequence number with a value
			// of 5, but has the disable bit set.  So the first lock
			// should be selected as it's the latest lock that isn't
			// disabled.
			name:      "multiple inputs, 1 disabled",
			txVersion: 2,
			inputs: []*wire.TxIn{{
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(true, 2560),
			}, {
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(false, 4),
			}, {
				PreviousOutPoint: utxo,
				Sequence: mustLockTimeToSeq(false, 5) |
					wire.SequenceLockTimeDisabled,
			}},
			isActive: true,
			want: SequenceLock{
				MinHeight: prevUtxoHeight + 3,
				MinTime:   medianTime + (5 << wire.SequenceLockTimeGranularity) - 1,
			},
		},
		{
			// A transaction with a single input.  The input's
			// sequence number encodes a relative locktime in blocks
			// (3 blocks).  The sequence lock should  have a value
			// of -1 for seconds, but a height of 2 meaning it can
			// be included at height 3.
			name:      "3 blocks",
			txVersion: 2,
			inputs: []*wire.TxIn{{
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(false, 3),
			}},
			isActive: true,
			want: SequenceLock{
				MinHeight: prevUtxoHeight + 2,
				MinTime:   -1,
			},
		},
		{
			// A transaction with two inputs with locktimes
			// expressed in seconds.  The selected sequence lock
			// value for seconds should be the time further in the
			// future.
			name:      "2 inputs both in seconds",
			txVersion: 2,
			inputs: []*wire.TxIn{{
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(true, 5120),
			}, {
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(true, 2560),
			}},
			isActive: true,
			want: SequenceLock{
				MinHeight: -1,
				MinTime:   medianTime + (10 << wire.SequenceLockTimeGranularity) - 1,
			},
		},
		{
			// A transaction with two inputs with locktimes
			// expressed in blocks.  The selected sequence lock
			// value for blocks should be the height further in the
			// future, so a height of 10 indicating it can be
			// included at height 11.
			name:      "2 inputs both in blocks",
			txVersion: 2,
			inputs: []*wire.TxIn{{
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(false, 1),
			}, {
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(false, 11),
			}},
			isActive: true,
			want: SequenceLock{
				MinHeight: prevUtxoHeight + 10,
				MinTime:   -1,
			},
		},
		{
			// A transaction with multiple inputs.  Two inputs are
			// seconds and the other two are blocks. The lock
			// further into the future for both inputs should be
			// chosen.
			name:      "4 inputs, 2 in seconds, 2 in blocks",
			txVersion: 2,
			inputs: []*wire.TxIn{{
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(true, 2560),
			}, {
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(true, 6656),
			}, {
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(false, 3),
			}, {
				PreviousOutPoint: utxo,
				Sequence:         mustLockTimeToSeq(false, 9),
			}},
			isActive: true,
			want: SequenceLock{
				MinHeight: prevUtxoHeight + 8,
				MinTime:   medianTime + (13 << wire.SequenceLockTimeGranularity) - 1,
			},
		},
		{
			// A transaction with a single unconfirmed input.  Since
			// the input is unconfirmed, the height of the input
			// should be interpreted as the height of the *next*
			// block.  So, a 2 block relative lock means the
			// sequence lock should be for 1 block after the *next*
			// block height, indicating it can be included 2 blocks
			// after that.
			name:      "unconfirmed input in blocks",
			txVersion: 2,
			inputs: []*wire.TxIn{{
				PreviousOutPoint: unConfUtxo,
				Sequence:         mustLockTimeToSeq(false, 2),
			}},
			isActive: true,
			want: SequenceLock{
				MinHeight: nextBlockHeight + 1,
				MinTime:   -1,
			},
		},
		{
			// A transaction with a single unconfirmed input.  The
			// input has locktime in seconds, so the locktime should
			// be based off the median time of the *next* block.
			name:      "unconfirmed input in seconds",
			txVersion: 2,
			inputs: []*wire.TxIn{{
				PreviousOutPoint: unConfUtxo,
				Sequence:         mustLockTimeToSeq(true, 1024),
			}},
			isActive: true,
			want: SequenceLock{
				MinHeight: -1,
				MinTime:   nextMedianTime + 1023,
			},
		},
	}

	for i, test := range tests {
		// Create fake spending transaction per the test input data.
		tx := wire.MsgTx{
			SerType:  wire.TxSerializeFull,
			Version:  test.txVersion,
			LockTime: 0,
			Expiry:   0,
			TxOut:    nil,
		}
		for _, txIn := range test.inputs {
			tx.AddTxIn(txIn)
		}
		utilTx := dcrutil.NewTx(&tx)

		// Calculate the sequence lock for the test input data.  Since
		// the exported function always has the agenda active, use the
		// unexported function when simulating the agenda not being
		// active, and alternate between them to ensure both are
		// exercised.
		var seqLock *SequenceLock
		var err error
		if test.isActive && i%2 == 0 {
			seqLock, err = bc.CalcSequenceLock(utilTx, view)
		} else {
			bc.chainLock.Lock()
			seqLock, err = bc.calcSequenceLock(node, utilTx, view,
				test.isActive)
			bc.chainLock.Unlock()
		}
		if err != nil {
			t.Errorf("%s: unable to calc sequence lock: %v",
				test.name, err)
			continue
		}

		// Ensure both the returned sequence lock seconds and block
		// height match the expected values.
		if seqLock.MinTime != test.want.MinTime {
			t.Errorf("%s: mistmached seconds - got %v, want %v",
				test.name, seqLock.MinTime, test.want.MinTime)
			continue
		}
		if seqLock.MinHeight != test.want.MinHeight {
			t.Errorf("%s: mismatched height - got %v, want %v",
				test.name, seqLock.MinHeight,
				test.want.MinHeight)
		}
	}
}

// TestLockTimeToSequence ensure the convenience function to convert relative
// lock times to a sequence number works as expected.
func TestLockTimeToSequence(t *testing.T) {
	const (
		// The following constants are used over the package-level
		// definitions to ensure tests correctly detect any changes to
		// them.
		secondsGranularityBits = 9
		secondsBit             = 1 << 22
		maxValue               = 1<<16 - 1
		maxBlockHeight         = maxValue
		maxSeconds             = maxValue << secondsGranularityBits
	)

	tests := []struct {
		name      string
		locktime  uint32
		isSeconds bool
		expected  uint32
		invalid   bool
	}{
		{
			name:      "relative block height 0",
			locktime:  0,
			isSeconds: false,
			expected:  0,
		},
		{
			name:      "max relative block height",
			locktime:  maxBlockHeight,
			isSeconds: false,
			expected:  maxBlockHeight,
		},
		{
			name:      "max relative block height +1",
			locktime:  maxBlockHeight + 1,
			isSeconds: false,
			expected:  0,
			invalid:   true,
		},
		{
			name:      "relative seconds 0",
			locktime:  0,
			isSeconds: true,
			expected:  secondsBit,
		},
		{
			name:      "relative seconds granularity - 1",
			locktime:  (1 << secondsGranularityBits) - 1,
			isSeconds: true,
			expected:  secondsBit,
		},
		{
			name:      "relative seconds exact granularity",
			locktime:  1 << secondsGranularityBits,
			isSeconds: true,
			expected:  secondsBit + 1,
		},
		{
			name:      "relative seconds granularity + 1",
			locktime:  (1 << secondsGranularityBits) + 1,
			isSeconds: true,
			expected:  secondsBit + 1,
		},
		{
			name:      "relative seconds max - 1",
			locktime:  maxSeconds - 1,
			isSeconds: true,
			expected:  secondsBit + maxValue - 1,
		},
		{
			name:      "relative seconds max",
			locktime:  maxSeconds,
			isSeconds: true,
			expected:  secondsBit + maxValue,
		},
		{
			name:      "relative seconds max +1",
			locktime:  maxSeconds + 1,
			isSeconds: true,
			expected:  0,
			invalid:   true,
		},
	}

	for _, test := range tests {
		gotSequence, err := LockTimeToSequence(test.isSeconds,
			test.locktime)
		if err != nil && !test.invalid {
			t.Errorf("%s: unexpected error: %v", test.name, err)
			continue

		}
		if err == nil && test.invalid {
			t.Errorf("%s: did not receive expected error", test.name)
			continue
		}

		if gotSequence != test.expected {
			t.Errorf("%s: mismatched sequence - got %d, want %d",
				test.name, gotSequence, test.expected)
			continue
		}

	}
}
