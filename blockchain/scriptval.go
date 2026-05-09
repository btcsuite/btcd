// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"runtime"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/stringintech/go-bitcoinkernel/kernel"
	"golang.org/x/sync/errgroup"
)

// btcdFlagsToKernel translates btcd's txscript.ScriptFlags to the
// equivalent kernel.ScriptFlags for use with libbitcoinkernel verification.
func btcdFlagsToKernel(flags txscript.ScriptFlags) kernel.ScriptFlags {
	var kFlags kernel.ScriptFlags
	if flags&txscript.ScriptBip16 != 0 {
		kFlags |= kernel.ScriptFlagsVerifyP2SH
	}
	if flags&txscript.ScriptVerifyDERSignatures != 0 {
		kFlags |= kernel.ScriptFlagsVerifyDERSig
	}
	if flags&txscript.ScriptStrictMultiSig != 0 {
		kFlags |= kernel.ScriptFlagsVerifyNullDummy
	}
	if flags&txscript.ScriptVerifyCheckLockTimeVerify != 0 {
		kFlags |= kernel.ScriptFlagsVerifyCheckLockTimeVerify
	}
	if flags&txscript.ScriptVerifyCheckSequenceVerify != 0 {
		kFlags |= kernel.ScriptFlagsVerifyCheckSequenceVerify
	}
	if flags&txscript.ScriptVerifyWitness != 0 {
		kFlags |= kernel.ScriptFlagsVerifyWitness
	}
	if flags&txscript.ScriptVerifyTaproot != 0 {
		kFlags |= kernel.ScriptFlagsVerifyTaproot
	}
	return kFlags
}

type scriptPubkeyEntry struct {
	pkScript []byte
	amount   int64
	index    uint
}

type kernelTxVerifier struct {
	tx                *kernel.Transaction
	precomputedTxData *kernel.PrecomputedTransactionData
	flags             kernel.ScriptFlags
	entries           []scriptPubkeyEntry
}

func newKernelTxVerifier(tx *btcutil.Tx, utxoView *UtxoViewpoint, flags txscript.ScriptFlags) (*kernelTxVerifier, error) {
	var txBuf bytes.Buffer
	if err := tx.MsgTx().Serialize(&txBuf); err != nil {
		return nil, err
	}

	kernelTx, err := kernel.NewTransaction(txBuf.Bytes())
	if err != nil {
		return nil, err
	}

	spentOutputs := make([]kernel.TransactionOutputLike, 0, len(tx.MsgTx().TxIn))
	scriptPubkeys := make([]scriptPubkeyEntry, 0, len(tx.MsgTx().TxIn))
	for txInIdx, txIn := range tx.MsgTx().TxIn {
		if txIn.PreviousOutPoint.Index == math.MaxUint32 {
			// coinbase - add empty placeholder for spent
			sp := kernel.NewScriptPubkey([]byte{})
			spentOutputs = append(spentOutputs, kernel.NewTransactionOutput(sp, 0))
			sp.Destroy()
			continue
		}

		utxo := utxoView.LookupEntry(txIn.PreviousOutPoint)
		if utxo == nil {
			kernelTx.Destroy()
			return nil, ruleError(ErrMissingTxOut, fmt.Sprintf(
				"unable to find unspent output %v", txIn.PreviousOutPoint))
		}

		pubKey := kernel.NewScriptPubkey(utxo.PkScript())
		spentOutputs = append(spentOutputs, kernel.NewTransactionOutput(pubKey, utxo.Amount()))
		pubKey.Destroy()

		scriptPubkeys = append(scriptPubkeys, scriptPubkeyEntry{
			pkScript: utxo.PkScript(),
			amount:   utxo.Amount(),
			index:    uint(txInIdx),
		})
	}

	precomputedTxData, err := kernel.NewPrecomputedTransactionData(kernelTx, spentOutputs)
	if err != nil {
		kernelTx.Destroy()
		return nil, err
	}

	return &kernelTxVerifier{
		tx:                kernelTx,
		precomputedTxData: precomputedTxData,
		flags:             btcdFlagsToKernel(flags),
		entries:           scriptPubkeys,
	}, nil
}

func (v *kernelTxVerifier) verifyInput(pkScript []byte, amount int64, index uint) error {
	sp := kernel.NewScriptPubkey(pkScript)
	defer sp.Destroy()

	valid, err := sp.Verify(amount, v.tx, v.precomputedTxData, index, v.flags)
	if err != nil {
		return ruleError(ErrScriptMalformed, fmt.Sprintf(
			"failed to verify input %d: %v", index, err))
	}

	if !valid {
		return ruleError(ErrScriptValidation, fmt.Sprintf(
			"failed to validate input %d", index))
	}

	return nil
}

func (v *kernelTxVerifier) validateAll(ctx context.Context, g *errgroup.Group) {
	for _, entry := range v.entries {
		g.Go(func() error {
			// ctx.Err() is checked before starting work so that goroutines
			// scheduled after a failure return early. verifyInput can not be
			// interrupted mid-execution.
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return v.verifyInput(entry.pkScript, entry.amount, entry.index)
		})
	}
}

func (v *kernelTxVerifier) destroy() {
	v.precomputedTxData.Destroy()
	v.tx.Destroy()
}

// ValidateTransactionScripts validates the scripts for the passed transaction
// using multiple goroutines.
func ValidateTransactionScripts(tx *btcutil.Tx, utxoView *UtxoViewpoint,
	flags txscript.ScriptFlags) error {

	verifier, err := newKernelTxVerifier(tx, utxoView, flags)
	if err != nil {
		return err
	}
	defer verifier.destroy()

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(runtime.NumCPU() * 3)

	verifier.validateAll(ctx, g)

	return g.Wait()
}

// checkBlockScripts executes and validates the scripts for all transactions in
// the passed block using multiple goroutines.
func checkBlockScripts(block *btcutil.Block, utxoView *UtxoViewpoint,
	scriptFlags txscript.ScriptFlags) error {

	g, ctx := errgroup.WithContext(context.Background())

	g.SetLimit(runtime.NumCPU() * 3)

	var verifiers []*kernelTxVerifier
	defer func() {
		for _, v := range verifiers {
			v.destroy()
		}
	}()

	start := time.Now()
	for _, tx := range block.Transactions() {
		verifier, err := newKernelTxVerifier(tx, utxoView, scriptFlags)
		if err != nil {
			return err
		}

		verifiers = append(verifiers, verifier)
		verifier.validateAll(ctx, g)
	}

	if err := g.Wait(); err != nil {
		return err
	}

	elapsed := time.Since(start)
	log.Tracef("block %v took %v to verify", block.Hash(), elapsed)

	return nil
}
