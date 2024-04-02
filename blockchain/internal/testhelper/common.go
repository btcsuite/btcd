package testhelper

import (
	"encoding/binary"
	"math"
	"runtime"

	"github.com/btcsuite/btcd/blockchain/internal/workmath"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
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

// CreateSpendTx creates a transaction that spends from the provided spendable
// output and includes an additional unique OP_RETURN output to ensure the
// transaction ends up with a unique hash.  The script is a simple OP_TRUE
// script which avoids the need to track addresses and signature scripts in the
// tests.
func CreateSpendTx(spend *SpendableOut, fee btcutil.Amount) *wire.MsgTx {
	spendTx := wire.NewMsgTx(1)
	spendTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: spend.PrevOut,
		Sequence:         wire.MaxTxInSequenceNum,
		SignatureScript:  nil,
	})
	spendTx.AddTxOut(wire.NewTxOut(int64(spend.Amount-fee),
		OpTrueScript))
	opRetScript, err := UniqueOpReturnScript()
	if err != nil {
		panic(err)
	}
	spendTx.AddTxOut(wire.NewTxOut(0, opRetScript))

	return spendTx
}

// CreateCoinbaseTx returns a coinbase transaction paying an appropriate
// subsidy based on the passed block height and the block subsidy.  The
// coinbase signature script conforms to the requirements of version 2 blocks.
func CreateCoinbaseTx(blockHeight int32, blockSubsidy int64) *wire.MsgTx {
	extraNonce := uint64(0)
	coinbaseScript, err := StandardCoinbaseScript(blockHeight, extraNonce)
	if err != nil {
		panic(err)
	}

	tx := wire.NewMsgTx(1)
	tx.AddTxIn(&wire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex),
		Sequence:        wire.MaxTxInSequenceNum,
		SignatureScript: coinbaseScript,
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    blockSubsidy,
		PkScript: OpTrueScript,
	})
	return tx
}

// StandardCoinbaseScript returns a standard script suitable for use as the
// signature script of the coinbase transaction of a new block.  In particular,
// it starts with the block height that is required by version 2 blocks.
func StandardCoinbaseScript(blockHeight int32, extraNonce uint64) ([]byte, error) {
	return txscript.NewScriptBuilder().AddInt64(int64(blockHeight)).
		AddInt64(int64(extraNonce)).Script()
}

// OpReturnScript returns a provably-pruneable OP_RETURN script with the
// provided data.
func OpReturnScript(data []byte) ([]byte, error) {
	builder := txscript.NewScriptBuilder()
	script, err := builder.AddOp(txscript.OP_RETURN).AddData(data).Script()
	if err != nil {
		return nil, err
	}
	return script, nil
}

// UniqueOpReturnScript returns a standard provably-pruneable OP_RETURN script
// with a random uint64 encoded as the data.
func UniqueOpReturnScript() ([]byte, error) {
	rand, err := wire.RandomUint64()
	if err != nil {
		return nil, err
	}

	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data[0:8], rand)
	return OpReturnScript(data)
}

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
