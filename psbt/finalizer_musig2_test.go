// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

import (
	"bytes"
	"testing"

	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/stretchr/testify/require"
)

// finalizeAndExtract finalizes every input in the parsed PSBT and returns
// the extracted (signed) transaction.
func finalizeAndExtract(t *testing.T, hexStr string) (*Packet, []byte) {
	t.Helper()

	raw := mustDecodeHex(t, hexStr)
	p, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.NoError(t, err)

	require.NoError(t, MaybeFinalizeAll(p))

	require.Len(t, p.Inputs, 1)
	require.NotNil(t, p.Inputs[0].FinalScriptWitness)

	return p, p.Inputs[0].FinalScriptWitness
}

// verifyFinalized runs the txscript engine over the finalized transaction
// to confirm the produced witness is consensus-valid.
func verifyFinalized(t *testing.T, p *Packet) {
	t.Helper()

	finalTx, err := Extract(p)
	require.NoError(t, err)

	pInput := p.Inputs[0]
	require.NotNil(t, pInput.WitnessUtxo)

	pkScript := pInput.WitnessUtxo.PkScript
	amount := pInput.WitnessUtxo.Value
	prevFetcher := txscript.NewCannedPrevOutputFetcher(pkScript, amount)
	hashCache := txscript.NewTxSigHashes(finalTx, prevFetcher)

	vm, err := txscript.NewEngine(
		pkScript, finalTx, 0, txscript.StandardVerifyFlags, nil,
		hashCache, amount, prevFetcher,
	)
	require.NoError(t, err)
	require.NoError(t, vm.Execute())
}

// TestFinalize_MuSig2_Case1c_BIP86Keyspend asserts that the BIP-373 case 1c
// PSBT (output key IS the aggregate, BIP-86 keyspend) finalizes to a
// consensus-valid taproot key spend witness.
func TestFinalize_MuSig2_Case1c_BIP86Keyspend(t *testing.T) {
	p, witness := finalizeAndExtract(t, findVector(t, "case 1c"))

	// The witness should contain exactly one element: the 64-byte
	// BIP-340 Schnorr signature (default sighash, so no flag byte
	// appended). Total serialized form is:
	//   varint(1) || varint(64) || sig(64) = 1 + 1 + 64 = 66 bytes.
	require.Len(t, witness, 66)

	verifyFinalized(t, p)
}

// TestFinalize_MuSig2_Case2c_InternalKeyAggregate asserts that case 2c
// (internal key IS aggregate, BIP-86 tweak — no script tree on the
// taproot output) finalizes to a consensus-valid keyspend witness. The
// vector ships with a pre-aggregated PSBT_IN_TAP_KEY_SIG, which the
// finalizer would normally consume directly; we strip it here to force
// the MuSig2 keyspend path through finalizeMuSig2KeySpend and exercise
// the BIP-86 tweak branch.
func TestFinalize_MuSig2_Case2c_InternalKeyAggregate(t *testing.T) {
	raw := mustDecodeHex(t, findVector(t, "case 2c"))
	p, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.NoError(t, err)

	// Force the MuSig2 finalize path.
	p.Inputs[0].TaprootKeySpendSig = nil

	require.NoError(t, MaybeFinalizeAll(p))
	require.NotNil(t, p.Inputs[0].FinalScriptWitness)
	require.Len(t, p.Inputs[0].FinalScriptWitness, 66)

	verifyFinalized(t, p)
}

// TestFinalize_MuSig2_Case3c_TapscriptLeaf asserts that case 3c (key in
// tapscript leaf is aggregate) finalizes to a consensus-valid script
// spend witness of the form [aggSig, leafScript, controlBlock]. The
// vector ships with a pre-aggregated PSBT_IN_TAP_SCRIPT_SIG, which we
// strip to force the new finalizeMuSig2ScriptSpend path.
func TestFinalize_MuSig2_Case3c_TapscriptLeaf(t *testing.T) {
	raw := mustDecodeHex(t, findVector(t, "case 3c"))
	p, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.NoError(t, err)

	// Force the MuSig2 script-spend finalize path.
	p.Inputs[0].TaprootScriptSpendSig = nil

	require.NoError(t, MaybeFinalizeAll(p))
	require.NotNil(t, p.Inputs[0].FinalScriptWitness)

	// Decode the witness to check the stack shape: 3 elements (sig,
	// script, controlBlock).
	finalTx, err := Extract(p)
	require.NoError(t, err)
	require.Len(t, finalTx.TxIn[0].Witness, 3)

	// First element must be a 64-byte BIP-340 signature (default sighash).
	require.Len(t, finalTx.TxIn[0].Witness[0], 64)

	verifyFinalized(t, p)
}

// TestFinalize_MuSig2_Case4c_NoXpub asserts that case 4c (internal key
// derived from aggregate via BIP-32) without a PSBT_GLOBAL_XPUB for the
// aggregate is rejected with a clear error. The BIP-373 vector ships with
// a pre-aggregated keyspend sig (which would otherwise let Finalize
// succeed), so we strip it to force the MuSig2 finalize path. The vector
// has no PSBT_GLOBAL_XPUB, so the BIP-328 fallback path triggers — which
// is intentionally not supported because BIP-328 requires per-participant
// chain codes that the BIP-373 vector does not provide.
func TestFinalize_MuSig2_Case4c_NoXpub(t *testing.T) {
	raw := mustDecodeHex(t, findVector(t, "case 4c"))
	p, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.NoError(t, err)

	p.Inputs[0].TaprootKeySpendSig = nil

	err = MaybeFinalizeAll(p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no matching PSBT_GLOBAL_XPUB")
}
