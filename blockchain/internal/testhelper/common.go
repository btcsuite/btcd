package testhelper

import (
	"math"
	"runtime"

	"github.com/btcsuite/btcd/blockchain/internal/workmath"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

var (
	// OpTrueScript is simply a public key script that contains the OP_TRUE
	// opcode.  It is defined here to reduce garbage creation.
	OpTrueScript = []byte{txscript.OP_TRUE}

	// LowFee is a single satoshi and exists to make the test code more
	// readable.
	LowFee = btcutil.Amount(1)
)

// SpendableOut represents a transaction output that is spendable along with
// additional metadata such as the block its in and how much it pays.
type SpendableOut struct {
	PrevOut wire.OutPoint
	Amount  btcutil.Amount
}

// MakeSpendableOutForTx returns a spendable output for the given transaction
// and transaction output index within the transaction.
func MakeSpendableOutForTx(tx *wire.MsgTx, txOutIndex uint32) SpendableOut {
	return SpendableOut{
		PrevOut: wire.OutPoint{
			Hash:  tx.TxHash(),
			Index: txOutIndex,
		},
		Amount: btcutil.Amount(tx.TxOut[txOutIndex].Value),
	}
}

// MakeSpendableOut returns a spendable output for the given block, transaction
// index within the block, and transaction output index within the transaction.
func MakeSpendableOut(block *wire.MsgBlock, txIndex, txOutIndex uint32) SpendableOut {
	return MakeSpendableOutForTx(block.Transactions[txIndex], txOutIndex)
}

// SolveBlock attempts to find a nonce which makes the passed block header hash
// to a value less than the target difficulty.  When a successful solution is
// found true is returned and the nonce field of the passed header is updated
// with the solution.  False is returned if no solution exists.
//
// NOTE: This function will never solve blocks with a nonce of 0.  This is done
// so the 'nextBlock' function can properly detect when a nonce was modified by
// a munge function.
func SolveBlock(header *wire.BlockHeader) bool {
	// sbResult is used by the solver goroutines to send results.
	type sbResult struct {
		found bool
		nonce uint32
	}

	// solver accepts a block header and a nonce range to test. It is
	// intended to be run as a goroutine.
	targetDifficulty := workmath.CompactToBig(header.Bits)
	quit := make(chan bool)
	results := make(chan sbResult)
	solver := func(hdr wire.BlockHeader, startNonce, stopNonce uint32) {
		// We need to modify the nonce field of the header, so make sure
		// we work with a copy of the original header.
		for i := startNonce; i >= startNonce && i <= stopNonce; i++ {
			select {
			case <-quit:
				return
			default:
				hdr.Nonce = i
				hash := hdr.BlockHash()
				if workmath.HashToBig(&hash).Cmp(
					targetDifficulty) <= 0 {

					results <- sbResult{true, i}
					return
				}
			}
		}
		results <- sbResult{false, 0}
	}

	startNonce := uint32(1)
	stopNonce := uint32(math.MaxUint32)
	numCores := uint32(runtime.NumCPU())
	noncesPerCore := (stopNonce - startNonce) / numCores
	for i := uint32(0); i < numCores; i++ {
		rangeStart := startNonce + (noncesPerCore * i)
		rangeStop := startNonce + (noncesPerCore * (i + 1)) - 1
		if i == numCores-1 {
			rangeStop = stopNonce
		}
		go solver(*header, rangeStart, rangeStop)
	}
	for i := uint32(0); i < numCores; i++ {
		result := <-results
		if result.found {
			close(quit)
			header.Nonce = result.nonce
			return true
		}
	}

	return false
}
