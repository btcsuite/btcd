// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcchain

import (
	"fmt"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"math"
	"time"
)

// txValidate is used to track results of validating scripts for each
// transaction input index.
type txValidate struct {
	txIndex int
	err     error
}

// txProcessList
type txProcessList struct {
	txsha btcwire.ShaHash
	tx    *btcwire.MsgTx
}

// validateTxIn validates a the script pair for the passed spending transaction
// (along with the specific input index) and origin transaction (with the
// specific output index).
func validateTxIn(txInIdx int, txin *btcwire.TxIn, txSha *btcwire.ShaHash, tx *btcwire.MsgTx, pver uint32, timestamp time.Time, originTx *btcwire.MsgTx) error {
	// If the input transaction has no previous input, there is nothing
	// to check.
	originTxIdx := txin.PreviousOutpoint.Index
	if originTxIdx == math.MaxUint32 {
		return nil
	}

	if originTxIdx >= uint32(len(originTx.TxOut)) {
		originTxSha := &txin.PreviousOutpoint.Hash
		log.Warnf("unable to locate source tx %v spending tx %v", originTxSha, &txSha)
		return fmt.Errorf("invalid index %x", originTxIdx)
	}

	sigScript := txin.SignatureScript
	pkScript := originTx.TxOut[originTxIdx].PkScript
	engine, err := btcscript.NewScript(sigScript, pkScript, txInIdx, tx,
		pver, timestamp.After(btcscript.Bip16Activation))
	if err != nil {
		return err
	}

	err = engine.Execute()
	if err != nil {
		log.Warnf("validate of input %v failed: %v", txInIdx, err)
		return err
	}

	return nil
}

// validateAllTxIn validates the scripts for all of the passed transaction
// inputs using multiple goroutines.
func validateAllTxIn(txsha *btcwire.ShaHash, txValidator *btcwire.MsgTx, pver uint32, timestamp time.Time, job []*btcwire.TxIn, txStore map[btcwire.ShaHash]*txData) (err error) {
	c := make(chan txValidate)
	resultErrors := make([]error, len(job))

	var currentItem int
	var completedItems int

	processFunc := func(txInIdx int) {
		log.Tracef("validating tx %v input %v len %v",
			txsha, currentItem, len(job))
		txin := job[txInIdx]
		originTxSha := &txin.PreviousOutpoint.Hash
		origintxidx := txin.PreviousOutpoint.Index

		var originTx *btcwire.MsgTx
		if origintxidx != math.MaxUint32 {
			txInfo, ok := txStore[*originTxSha]
			if !ok {
				//wtf?
				fmt.Printf("obj not found in txStore %v",
					originTxSha)
			}
			originTx = txInfo.tx
		}
		err := validateTxIn(txInIdx, job[txInIdx], txsha, txValidator,
			pver, timestamp, originTx)
		r := txValidate{txInIdx, err}
		c <- r
	}
	for currentItem = 0; currentItem < len(job) && currentItem < 16; currentItem++ {
		go processFunc(currentItem)
	}
	for completedItems < len(job) {
		select {
		case result := <-c:
			completedItems++
			resultErrors[result.txIndex] = result.err
			// would be nice to determine if we could stop
			// on early errors here instead of running more.
			if err == nil {
				err = result.err
			}

			if currentItem < len(job) {
				go processFunc(currentItem)
				currentItem++
			}
		}
	}
	for i := 0; i < len(job); i++ {
		if resultErrors[i] != nil {
			log.Warnf("tx %v failed input %v, err %v", txsha, i, resultErrors[i])
		}
	}
	return
}

// checkBlockScripts executes and validates the scripts for all transactions in
// the passed block.
func checkBlockScripts(block *btcutil.Block, txStore map[btcwire.ShaHash]*txData) error {
	pver := block.ProtocolVersion()
	timestamp := block.MsgBlock().Header.Timestamp
	for i, tx := range block.MsgBlock().Transactions {
		txHash, _ := block.TxSha(i)
		err := validateAllTxIn(txHash, tx, pver, timestamp, tx.TxIn, txStore)
		if err != nil {
			return err
		}
	}

	return nil
}
