// Copyright (c) 2013-2023 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestDebugEngine checks that the StepCallbck called during debug script
// execution contains the expected data.
func TestDebugEngine(t *testing.T) {
	t.Parallel()

	// We'll generate a private key and a signature for the tx.
	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	internalKey := privKey.PubKey()

	// We use a simple script that will utilize both the stack and alt
	// stack in order to test the step callback, and wrap it in a taproot
	// witness script.
	builder := NewScriptBuilder()
	builder.AddData([]byte{0xab})
	builder.AddOp(OP_TOALTSTACK)
	builder.AddData(schnorr.SerializePubKey(internalKey))
	builder.AddOp(OP_CHECKSIG)
	builder.AddOp(OP_VERIFY)
	builder.AddOp(OP_1)
	pkScript, err := builder.Script()
	require.NoError(t, err)

	tapLeaf := NewBaseTapLeaf(pkScript)
	tapScriptTree := AssembleTaprootScriptTree(tapLeaf)

	ctrlBlock := tapScriptTree.LeafMerkleProofs[0].ToControlBlock(
		internalKey,
	)

	tapScriptRootHash := tapScriptTree.RootNode.TapHash()
	outputKey := ComputeTaprootOutputKey(
		internalKey, tapScriptRootHash[:],
	)
	p2trScript, err := PayToTaprootScript(outputKey)
	require.NoError(t, err)

	testTx := wire.NewMsgTx(2)
	testTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Index: 1,
		},
	})
	txOut := &wire.TxOut{
		Value: 1e8, PkScript: p2trScript,
	}
	testTx.AddTxOut(txOut)

	prevFetcher := NewCannedPrevOutputFetcher(
		txOut.PkScript, txOut.Value,
	)
	sigHashes := NewTxSigHashes(testTx, prevFetcher)

	sig, err := RawTxInTapscriptSignature(
		testTx, sigHashes, 0, txOut.Value,
		txOut.PkScript, tapLeaf,
		SigHashDefault, privKey,
	)
	require.NoError(t, err)

	// Now that we have the sig, we'll make a valid witness
	// including the control block.
	ctrlBlockBytes, err := ctrlBlock.ToBytes()
	require.NoError(t, err)
	txCopy := testTx.Copy()
	txCopy.TxIn[0].Witness = wire.TxWitness{
		sig, pkScript, ctrlBlockBytes,
	}

	expCallback := []StepInfo{
		// First callback is looking at the OP_1 witness version.
		{
			ScriptIndex: 1,
			OpcodeIndex: 0,
			Stack:       [][]byte{},
			AltStack:    [][]byte{},
		},
		// The OP_1 witness version is pushed to stack,
		{
			ScriptIndex: 1,
			OpcodeIndex: 1,
			Stack:       [][]byte{{0x01}},
			AltStack:    [][]byte{},
		},
		// Then the taproot script is being executed, starting with
		// only the signature on the stacks.
		{
			ScriptIndex: 2,
			OpcodeIndex: 0,
			Stack:       [][]byte{sig},
			AltStack:    [][]byte{},
		},
		// 0xab is pushed to the stack.
		{
			ScriptIndex: 2,
			OpcodeIndex: 1,
			Stack:       [][]byte{sig, {0xab}},
			AltStack:    [][]byte{},
		},
		// 0xab is moved to the alt stack.
		{
			ScriptIndex: 2,
			OpcodeIndex: 2,
			Stack:       [][]byte{sig},
			AltStack:    [][]byte{{0xab}},
		},
		// The public key is pushed to the stack.
		{
			ScriptIndex: 2,
			OpcodeIndex: 3,
			Stack: [][]byte{
				sig,
				schnorr.SerializePubKey(internalKey),
			},
			AltStack: [][]byte{{0xab}},
		},
		// OP_CHECKSIG is executed, resulting in 0x01 on the stack.
		{
			ScriptIndex: 2,
			OpcodeIndex: 4,
			Stack: [][]byte{
				{0x01},
			},
			AltStack: [][]byte{{0xab}},
		},
		// OP_VERIFY pops and checks the top stack element.
		{
			ScriptIndex: 2,
			OpcodeIndex: 5,
			Stack:       [][]byte{},
			AltStack:    [][]byte{{0xab}},
		},
		// A single OP_1 push completes the script execution (note that
		// the alt stack is cleared when the script is "done").
		{
			ScriptIndex: 2,
			OpcodeIndex: 6,
			Stack:       [][]byte{{0x01}},
			AltStack:    [][]byte{},
		},
	}

	stepIndex := 0
	callback := func(s *StepInfo) error {
		require.Less(
			t, stepIndex, len(expCallback), "unexpected callback",
		)

		require.Equal(t, &expCallback[stepIndex], s)
		stepIndex++
		return nil
	}

	// Run the debug engine.
	vm, err := NewDebugEngine(
		txOut.PkScript, txCopy, 0, StandardVerifyFlags,
		nil, sigHashes, txOut.Value, prevFetcher,
		callback,
	)
	require.NoError(t, err)
	require.NoError(t, vm.Execute())
}
