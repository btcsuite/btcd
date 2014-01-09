// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcchain

import (
	"fmt"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"math"
)

// txValidate is used to track results of validating scripts for each
// transaction input index.
type txValidate struct {
	txIndex int
	err     error
}

// validateTxIn validates a the script pair for the passed spending transaction
// (along with the specific input index) and origin transaction (with the
// specific output index).
func validateTxIn(txInIdx int, txin *btcwire.TxIn, tx *btcutil.Tx, originTx *btcutil.Tx, flags btcscript.ScriptFlags) error {
	// If the input transaction has no previous input, there is nothing
	// to check.
	originTxIdx := txin.PreviousOutpoint.Index
	if originTxIdx == math.MaxUint32 {
		return nil
	}

	if originTxIdx >= uint32(len(originTx.MsgTx().TxOut)) {
		originTxSha := &txin.PreviousOutpoint.Hash
		log.Warnf("unable to locate source tx %v spending tx %v",
			originTxSha, tx.Sha())
		return fmt.Errorf("invalid index %x", originTxIdx)
	}

	sigScript := txin.SignatureScript
	pkScript := originTx.MsgTx().TxOut[originTxIdx].PkScript
	engine, err := btcscript.NewScript(sigScript, pkScript, txInIdx,
		tx.MsgTx(), flags)
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

// ValidateTransactionScripts validates the scripts for the passed transaction
// using multiple goroutines.
func ValidateTransactionScripts(tx *btcutil.Tx, txStore TxStore, flags btcscript.ScriptFlags) (err error) {
	c := make(chan txValidate)
	job := tx.MsgTx().TxIn
	resultErrors := make([]error, len(job))

	var currentItem int
	var completedItems int

	processFunc := func(txInIdx int) {
		log.Tracef("validating tx %v input %v len %v",
			tx.Sha(), txInIdx, len(job))
		txin := job[txInIdx]
		originTxSha := &txin.PreviousOutpoint.Hash
		origintxidx := txin.PreviousOutpoint.Index

		var originTx *btcutil.Tx
		if origintxidx != math.MaxUint32 {
			txInfo, ok := txStore[*originTxSha]
			if !ok {
				//wtf?
				fmt.Printf("obj not found in txStore %v",
					originTxSha)
			}
			originTx = txInfo.Tx
		}
		err := validateTxIn(txInIdx, txin, tx, originTx, flags)
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
			log.Warnf("tx %v failed input %v, err %v", tx.Sha(), i,
				resultErrors[i])
		}
	}
	return
}

// checkBlockScripts executes and validates the scripts for all transactions in
// the passed block.
func checkBlockScripts(block *btcutil.Block, txStore TxStore) error {
	// Setup the script validation flags.  Blocks created after the BIP0016
	// activation time need to have the pay-to-script-hash checks enabled.
	var flags btcscript.ScriptFlags
	if block.MsgBlock().Header.Timestamp.After(btcscript.Bip16Activation) {
		flags |= btcscript.ScriptBip16
	}

	txList := block.Transactions()
	c := make(chan txValidate)
	resultErrors := make([]error, len(txList))

	var currentItem int
	var completedItems int
	processFunc := func(txIdx int) {
		err := ValidateTransactionScripts(txList[txIdx], txStore, flags)
		r := txValidate{txIdx, err}
		c <- r
	}
	for currentItem = 0; currentItem < len(txList) && currentItem < 8; currentItem++ {
		go processFunc(currentItem)
	}
	for completedItems < len(txList) {
		select {
		case result := <-c:
			completedItems++
			resultErrors[result.txIndex] = result.err
			// would be nice to determine if we could stop
			// on early errors here instead of running more.

			if currentItem < len(txList) {
				go processFunc(currentItem)
				currentItem++
			}
		}
	}
	for i := 0; i < len(txList); i++ {
		if resultErrors[i] != nil {
			return resultErrors[i]
		}
	}

	return nil
}
