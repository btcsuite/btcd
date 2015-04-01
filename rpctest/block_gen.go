// Copyright (c) 2015 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpctest

import (
	"errors"
	"math"
	"math/big"
	"runtime"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

var (
	miningPriv, _     = btcutil.DecodeWIF("FqVc9zp1C66n57VC7Qr5j7fLBFZ42zCmMf6efni425jNkzSbMpmJ")
	miningAddr, _     = decodeAddr("SXzLf64rFYJT5mBgQpLGnfxrL7heDzP9hD")
	miningPkScript, _ = txscript.PayToAddrScript(miningAddr)

	activeNetParams = &chaincfg.SimNetParams
)

// decodeAddr attempts to decode the passed string as an address.
func decodeAddr(addr string) (btcutil.Address, error) {
	utilAddr, err := btcutil.DecodeAddress(addr, &chaincfg.SimNetParams)
	if err != nil {
		return nil, err
	}
	return utilAddr, nil
}

// solveBlock attempts to find a nonce which makes the passed block header
// hash to a value less than the target difficulty. When a successful solution
// is found true is returned and the nonce field of the passed header is
// updated with the solution. False is returned if no solution exists.
func solveBlock(header *wire.BlockHeader, targetDifficulty *big.Int) bool {
	// sbResult is used by the solver goroutines to send results.
	type sbResult struct {
		found bool
		nonce uint32
	}

	// solver accepts a block header and a nonce range to test. It is
	// intended to be run as a goroutine.
	quit := make(chan bool)
	results := make(chan sbResult)
	solver := func(header *wire.BlockHeader, startNonce, stopNonce uint32) {
		// We need to modify the nonce field of the header, so make sure
		// we work with a copy of the original header.
		hdrCopy := *header
		for i := startNonce; i >= startNonce && i <= stopNonce; i++ {
			select {
			case <-quit:
				return
			default:
				hdrCopy.Nonce = i
				hash, _ := hdrCopy.BlockSha()
				if blockchain.ShaHashToBig(&hash).Cmp(targetDifficulty) <= 0 {
					results <- sbResult{true, i}
					return
				}
			}
		}
		results <- sbResult{false, 0}
	}

	startNonce := uint32(0)
	stopNonce := uint32(math.MaxUint32)
	numCores := uint32(runtime.NumCPU())
	noncesPerCore := (stopNonce - startNonce) / numCores
	for i := uint32(0); i < numCores; i++ {
		rangeStart := startNonce + (noncesPerCore * i)
		rangeStop := startNonce + (noncesPerCore * (i + 1)) - 1
		if i == numCores-1 {
			rangeStop = stopNonce
		}
		go solver(header, rangeStart, rangeStop)
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

// standardCoinbaseScript returns a standard script suitable for use as the
// signature script of the coinbase transaction of a new block. In particular,
// it starts with the block height that is required by version 2 blocks and adds
// the extra nonce as well as additional coinbase flags.
func standardCoinbaseScript(nextBlockHeight int64, extraNonce uint64) ([]byte, error) {
	return txscript.NewScriptBuilder().AddInt64(nextBlockHeight).
		AddUint64(extraNonce).Script()
}

// createCoinbaseTx returns a coinbase transaction paying an appropriate subsidy
// based on the passed block height to the provided address.
func createCoinbaseTx(coinbaseScript []byte, nextBlockHeight int64, addr btcutil.Address) (*btcutil.Tx, error) {
	// Create the script to pay to the provided payment address.
	pkScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return nil, err
	}

	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&wire.ShaHash{},
			wire.MaxPrevOutIndex),
		SignatureScript: coinbaseScript,
		Sequence:        wire.MaxTxInSequenceNum,
	})
	tx.AddTxOut(&wire.TxOut{
		Value: blockchain.CalcBlockSubsidy(nextBlockHeight,
			activeNetParams),
		PkScript: pkScript,
	})
	return btcutil.NewTx(tx), nil
}

// CreateBlock creates a new block building from the previous block.
func createBlock(prevBlock *btcutil.Block, inclusionTxs []*btcutil.Tx) (*btcutil.Block, error) {
	prevHash, err := prevBlock.Sha()
	if err != nil {
		return nil, err
	}
	blockHeight := prevBlock.Height() + 1

	// Add one second to the previous block unless it's the genesis block in
	// which case use the current time.
	var ts time.Time
	if blockHeight == 0 {
		ts = time.Unix(time.Now().Unix(), 0)
	} else {
		ts = prevBlock.MsgBlock().Header.Timestamp.Add(time.Second)
	}

	extraNonce := uint64(0)
	coinbaseScript, err := standardCoinbaseScript(blockHeight, extraNonce)
	if err != nil {
		return nil, err
	}
	coinbaseTx, err := createCoinbaseTx(coinbaseScript, blockHeight, miningAddr)
	if err != nil {
		return nil, err
	}

	// Create a new block ready to be solved.
	blockTxns := []*btcutil.Tx{coinbaseTx}
	if inclusionTxs != nil {
		blockTxns = append(blockTxns, inclusionTxs...)
	}
	merkles := blockchain.BuildMerkleTreeStore(blockTxns)
	var block wire.MsgBlock
	block.Header = wire.BlockHeader{
		Version:    1,
		PrevBlock:  *prevHash,
		MerkleRoot: *merkles[len(merkles)-1],
		Timestamp:  ts,
		Bits:       activeNetParams.PowLimitBits,
	}
	for _, tx := range blockTxns {
		if err := block.AddTransaction(tx.MsgTx()); err != nil {
			return nil, err
		}
	}

	found := solveBlock(&block.Header, activeNetParams.PowLimit)
	if !found {
		return nil, errors.New("Unable to solve block")
	}

	utilBlock := btcutil.NewBlock(&block)
	utilBlock.SetHeight(blockHeight)
	return utilBlock, nil
}
