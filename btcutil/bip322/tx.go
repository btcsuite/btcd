package bip322

import (
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// BuildToSpendTx constructs the BIP-322 "to_spend" virtual transaction for
// the given message and address scriptPubKey.
//
// The input's scriptSig encodes the BIP-322 message hash as:
//
//	OP_0 PUSH32[MessageHash(message)]
//
// See https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
func BuildToSpendTx(message string, scriptPubKey []byte) (*wire.MsgTx, error) {
	msgHash := *chainhash.TaggedHash(
		bip322MsgTag, []byte(message),
	)

	builder := txscript.NewScriptBuilder()
	builder.AddOp(txscript.OP_0)
	builder.AddData(msgHash[:])

	scriptSig, err := builder.Script()
	if err != nil {
		return nil, err
	}

	tx := wire.NewMsgTx(0)
	tx.LockTime = 0

	prevOut := wire.OutPoint{
		Hash:  chainhash.Hash{},
		Index: wire.MaxPrevOutIndex,
	}

	txIn := &wire.TxIn{
		PreviousOutPoint: prevOut,
		SignatureScript:  scriptSig,
		Witness:          nil,
		Sequence:         0,
	}
	tx.AddTxIn(txIn)

	txOut := wire.NewTxOut(0, scriptPubKey)
	tx.AddTxOut(txOut)

	return tx, nil
}

// BuildToSignTx constructs the BIP-322 "to_sign" virtual transaction.
//
// See https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
func BuildToSignTx(toSpendTxHash chainhash.Hash, scriptPubKey []byte) (*wire.MsgTx, error) {
	opReturnScript, err := txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).Script()
	if err != nil {
		return nil, err
	}

	tx := wire.NewMsgTx(0)
	tx.LockTime = 0

	prevOut := wire.OutPoint{
		Hash:  toSpendTxHash,
		Index: 0,
	}

	txIn := &wire.TxIn{
		PreviousOutPoint: prevOut,
		SignatureScript:  nil,
		Witness:          nil,
		Sequence:         0,
	}
	tx.AddTxIn(txIn)

	txOut := wire.NewTxOut(0, opReturnScript)
	tx.AddTxOut(txOut)

	return tx, nil
}
