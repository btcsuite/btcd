// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
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

// TestFinalize_MuSig2_MultipleParticipantRecords asserts that the finalizer
// picks the participants record the partial signatures actually belong to when
// an input carries records for more than one aggregate key. BIP-373 keys
// PSBT_IN_MUSIG2_PARTICIPANT_PUBKEYS by the aggregate pubkey, so several
// records may be present, and their order on the input says nothing about which
// one the signatures were produced under.
func TestFinalize_MuSig2_MultipleParticipantRecords(t *testing.T) {
	// A record for an unrelated aggregate key. None of the tweak chains the
	// finalizer knows about turn this into the key in the script, so it
	// must be skipped.
	agg, keys := bip373Participants(t)
	decoy := &MuSig2Participants{
		AggregateKey: keys[0],
		Keys:         keys,
	}
	require.False(t, decoy.AggregateKey.IsEqual(agg))

	tests := []struct {
		name    string
		records []*MuSig2Participants
	}{
		{
			name: "decoy record first",
			records: []*MuSig2Participants{
				decoy, {AggregateKey: agg, Keys: keys},
			},
		},
		{
			name: "decoy record last",
			records: []*MuSig2Participants{
				{AggregateKey: agg, Keys: keys}, decoy,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Case 2c is the interesting one: the key in the script
			// is the BIP-86 tweak of the record's aggregate, so the
			// finalizer cannot find the right record by a plain
			// equality check against the partial sig keydata.
			raw := mustDecodeHex(t, findVector(t, "case 2c"))
			p, err := NewFromRawBytes(bytes.NewReader(raw), false)
			require.NoError(t, err)

			// Force the MuSig2 keyspend finalize path.
			p.Inputs[0].TaprootKeySpendSig = nil

			// Sanity check that we are replacing a single record
			// with the one real record plus the decoy.
			require.Len(t, p.Inputs[0].MuSig2Participants, 1)
			p.Inputs[0].MuSig2Participants = tc.records

			require.NoError(t, MaybeFinalizeAll(p))
			require.Len(t, p.Inputs[0].FinalScriptWitness, 66)

			verifyFinalized(t, p)
		})
	}
}

// TestFinalize_MuSig2_NoMatchingParticipantRecord asserts that an input whose
// participants record cannot account for the key in the script is rejected,
// rather than finalizing to a signature that does not verify.
func TestFinalize_MuSig2_NoMatchingParticipantRecord(t *testing.T) {
	_, keys := bip373Participants(t)

	tests := []struct {
		name      string
		mutate    func(pInput *PInput)
		expectErr string
	}{
		{
			// The record is for an unrelated aggregate key, so the
			// finalizer concludes a tweak must have been applied
			// but has no internal key to recover it from.
			name: "record for unrelated aggregate key",
			mutate: func(pInput *PInput) {
				pInput.MuSig2Participants[0].AggregateKey =
					keys[0]
			},
			expectErr: "without PSBT_IN_TAP_INTERNAL_KEY",
		},
		{
			// Everything agrees on an aggregate key that is not
			// actually the aggregate of the participant keys, so no
			// tweak is inferred but the key aggregation does not
			// reproduce it either.
			name: "aggregate is not the aggregate of the keys",
			mutate: func(pInput *PInput) {
				pInput.MuSig2Participants[0].AggregateKey =
					keys[0]
				for _, n := range pInput.MuSig2PubNonces {
					n.AggregateKey = keys[0]
				}
				for _, ps := range pInput.MuSig2PartialSigs {
					ps.AggregateKey = keys[0]
				}
			},
			expectErr: "does not produce the key in the script",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := mustDecodeHex(t, findVector(t, "case 1c"))
			p, err := NewFromRawBytes(bytes.NewReader(raw), false)
			require.NoError(t, err)

			tc.mutate(&p.Inputs[0])

			err = MaybeFinalizeAll(p)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.expectErr)
		})
	}
}

// TestFinalize_MuSig2_Case4c_BIP328Fallback asserts that case 4c (the taproot
// internal key is a BIP-32 child of the MuSig2 aggregate) finalizes to a
// consensus-valid keyspend witness even though the vector carries no
// PSBT_GLOBAL_XPUB for the aggregate. BIP-373 says derivation from the aggregate
// pubkey can be assumed to follow BIP-328 in that case, so the finalizer builds
// the BIP-328 synthetic xpub itself.
//
// The vector ships with a pre-aggregated PSBT_IN_TAP_KEY_SIG, which the
// finalizer would otherwise consume directly, so we strip it to force the MuSig2
// keyspend path.
func TestFinalize_MuSig2_Case4c_BIP328Fallback(t *testing.T) {
	raw := mustDecodeHex(t, findVector(t, "case 4c"))
	p, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.NoError(t, err)

	// Confirm the premise: the packet has no global xpub at all, so only the
	// BIP-328 fallback can supply the chain code.
	require.Empty(t, p.XPubs)

	p.Inputs[0].TaprootKeySpendSig = nil

	require.NoError(t, MaybeFinalizeAll(p))
	require.Len(t, p.Inputs[0].FinalScriptWitness, 66)

	verifyFinalized(t, p)
}

// TestBIP328SyntheticXpub asserts that the synthetic extended key the finalizer
// derives from a MuSig2 aggregate matches the test vectors of BIP-328. Getting
// the fixed chain code or any of the other extended key fields wrong would
// silently produce the wrong derivation tweaks, so we pin them here.
func TestBIP328SyntheticXpub(t *testing.T) {
	tests := []struct {
		name       string
		aggregate  string
		expectXpub string
	}{
		{
			name: "bip-328 vector 1",
			aggregate: "0354240c76b8f2999143301a99c7f721ee57eee0" +
				"bce401df3afeaa9ae218c70f23",
			expectXpub: "xpub661MyMwAqRbcFt6tk3uaczE1y6EvM1TqXva" +
				"wXcYmFEWijEM4PDBnuCXwwXEKGEouzXE6QLLRxja" +
				"tMcLLzJ5LV5Nib1BN7vJg6yp45yHHRbm",
		},
		{
			name: "bip-328 vector 2",
			aggregate: "0290539eede565f5d054f32cc0c220126889ed1e" +
				"5d193baf15aef344fe59d4610c",
			expectXpub: "xpub661MyMwAqRbcFt6tk3uaczE1y6EvM1TqXva" +
				"wXcYmFEWijEM4PDBnuCXwwVk5TFJk8Tw5WAdV3Dhr" +
				"GfbFA216sE9BsQQiSFTdudkETnKdg8k",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			aggregate := mustParsePubKey(t, tc.aggregate)

			xpub := bip328SyntheticXpub(aggregate)
			require.Equal(t, tc.expectXpub, xpub.String())

			// The chain code must be the SHA256 of the string the
			// BIP names, which is where the constant comes from.
			expectChainCode := sha256.Sum256(
				[]byte("MuSig2MuSig2MuSig2"),
			)
			require.Equal(
				t, expectChainCode[:], xpub.ChainCode(),
			)
		})
	}
}

// TestFinalize_MuSig2_IndependentContexts asserts that an input carrying more
// than one MuSig2 signing context still finalizes through whichever of them is
// complete.
//
// BIP-373 keys the nonce and partial signature fields by the participant key, the
// aggregate key and an optional tap leaf hash, so several aggregates and spend
// paths can coexist on one input. A complete key path session must therefore not
// be held back by a session for another leaf that is still collecting nonces.
func TestFinalize_MuSig2_IndependentContexts(t *testing.T) {
	agg, keys := bip373Participants(t)
	otherLeaf := bytes.Repeat([]byte{0xab}, sha256.Size)

	var strayNonce [musig2.PubNonceSize]byte
	for idx := range strayNonce {
		strayNonce[idx] = byte(idx + 1)
	}

	strayScalar := new(btcec.ModNScalar).SetInt(42)

	tests := []struct {
		name   string
		mutate func(pInput *PInput)
	}{{
		// The reviewer's case: a complete key path session plus a single
		// nonce for another leaf of the same aggregate key.
		name: "stray nonce for another leaf",
		mutate: func(pInput *PInput) {
			pInput.MuSig2PubNonces = append(
				pInput.MuSig2PubNonces, &MuSig2PubNonce{
					PubKey:       keys[0],
					AggregateKey: agg,
					TapLeafHash:  otherLeaf,
					PubNonce:     strayNonce,
				},
			)
		},
	}, {
		name: "stray nonce for another aggregate key",
		mutate: func(pInput *PInput) {
			pInput.MuSig2PubNonces = append(
				pInput.MuSig2PubNonces, &MuSig2PubNonce{
					PubKey:       keys[0],
					AggregateKey: keys[1],
					PubNonce:     strayNonce,
				},
			)
		},
	}, {
		// A partial signature without a nonce is just as incomplete, and
		// must be ignored the same way.
		name: "stray partial sig for another leaf",
		mutate: func(pInput *PInput) {
			pInput.MuSig2PartialSigs = append(
				pInput.MuSig2PartialSigs, &MuSig2PartialSig{
					PubKey:       keys[0],
					AggregateKey: agg,
					TapLeafHash:  otherLeaf,
					PartialSig: musig2.PartialSignature{
						S: strayScalar,
					},
				},
			)
		},
	}, {
		name: "stray nonce and partial sig for another leaf",
		mutate: func(pInput *PInput) {
			pInput.MuSig2PubNonces = append(
				pInput.MuSig2PubNonces, &MuSig2PubNonce{
					PubKey:       keys[0],
					AggregateKey: agg,
					TapLeafHash:  otherLeaf,
					PubNonce:     strayNonce,
				},
			)
			pInput.MuSig2PartialSigs = append(
				pInput.MuSig2PartialSigs, &MuSig2PartialSig{
					PubKey:       keys[0],
					AggregateKey: agg,
					TapLeafHash:  otherLeaf,
					PartialSig: musig2.PartialSignature{
						S: strayScalar,
					},
				},
			)
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Case 1c is a complete key path session for the
			// aggregate key.
			raw := mustDecodeHex(t, findVector(t, "case 1c"))
			p, err := NewFromRawBytes(bytes.NewReader(raw), false)
			require.NoError(t, err)

			tc.mutate(&p.Inputs[0])

			// Go through the wire encoding so the extra fields are
			// parsed back the way a real packet would deliver them.
			parsed := serializeAndParse(t, p)

			require.True(t, isFinalizable(parsed, 0))
			require.NoError(t, MaybeFinalizeAll(parsed))
			require.Len(t, parsed.Inputs[0].FinalScriptWitness, 66)

			verifyFinalized(t, parsed)
		})
	}
}

// TestFinalize_MuSig2_IncompleteContextOnly asserts that an input whose only
// signing context is incomplete is reported as not finalizable, rather than
// being reported ready and then failing (or worse, succeeding) later.
func TestFinalize_MuSig2_IncompleteContextOnly(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(pInput *PInput)
	}{{
		name: "missing one partial signature",
		mutate: func(pInput *PInput) {
			pInput.MuSig2PartialSigs = pInput.MuSig2PartialSigs[:2]
		},
	}, {
		name: "missing one nonce",
		mutate: func(pInput *PInput) {
			pInput.MuSig2PubNonces = pInput.MuSig2PubNonces[:2]
		},
	}, {
		name: "partial sig from a participant without a nonce",
		mutate: func(pInput *PInput) {
			_, keys := bip373Participants(t)
			pInput.MuSig2PartialSigs[0].PubKey = keys[0]
			pInput.MuSig2PubNonces[0].PubKey = keys[1]
			pInput.MuSig2PubNonces[1].PubKey = keys[2]
			pInput.MuSig2PubNonces[2].PubKey =
				mustParsePubKey(t, bip373AggregateKeyHex)
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := mustDecodeHex(t, findVector(t, "case 1c"))
			p, err := NewFromRawBytes(bytes.NewReader(raw), false)
			require.NoError(t, err)

			tc.mutate(&p.Inputs[0])

			require.False(t, isFinalizable(p, 0))
			require.ErrorIs(
				t, MaybeFinalizeAll(p), ErrNotFinalizable,
			)
			require.Nil(t, p.Inputs[0].FinalScriptWitness)
		})
	}
}

// TestFinalize_MuSig2_RejectsInvalidCombinedSig asserts that finalization fails
// when the combined signature does not verify under the key the script engine
// will check it against.
//
// Finalization is destructive: it replaces the input's MuSig2 fields with the
// witness it builds. Without this check a tampered partial signature, or one
// produced for a different message, would be reported as a successful
// finalization and leave an unspendable transaction with no partial signatures
// left to retry from. We therefore also assert that the MuSig2 fields survive the
// failure.
func TestFinalize_MuSig2_RejectsInvalidCombinedSig(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(p *Packet)
		expectErr string
	}{{
		name: "tampered partial signature scalar",
		mutate: func(p *Packet) {
			partialSig := &p.Inputs[0].MuSig2PartialSigs[0].PartialSig
			partialSig.S.Add(new(btcec.ModNScalar).SetInt(1))
		},
		expectErr: "does not verify under spend key",
	}, {
		// A corrupted nonce is no longer a curve point, so this one is
		// already rejected when the nonces are aggregated.
		name: "corrupted public nonce",
		mutate: func(p *Packet) {
			p.Inputs[0].MuSig2PubNonces[0].PubNonce[1] ^= 0x01
		},
		expectErr: "error aggregating pub nonces",
	}, {
		// A well-formed nonce that simply is not the one the signers
		// used gets all the way to the combined signature, which is
		// where it has to be caught.
		name: "public nonce replaced with another valid nonce",
		mutate: func(p *Packet) {
			nonces, err := musig2.GenNonces(
				musig2.WithPublicKey(
					p.Inputs[0].MuSig2PubNonces[0].PubKey,
				),
			)
			require.NoError(t, err)

			p.Inputs[0].MuSig2PubNonces[0].PubNonce = nonces.PubNonce
		},
		expectErr: "does not verify under spend key",
	}, {
		// The signature commits to the sighash, so changing what the
		// transaction spends to must invalidate it.
		name: "transaction modified after signing",
		mutate: func(p *Packet) {
			p.UnsignedTx.TxOut[0].Value -= 1000
		},
		expectErr: "does not verify under spend key",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := mustDecodeHex(t, findVector(t, "case 1c"))
			p, err := NewFromRawBytes(bytes.NewReader(raw), false)
			require.NoError(t, err)

			tc.mutate(p)

			err = MaybeFinalizeAll(p)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.expectErr)

			// Nothing was finalized, and the MuSig2 fields are still
			// there to retry from.
			require.Nil(t, p.Inputs[0].FinalScriptWitness)
			require.Len(t, p.Inputs[0].MuSig2PartialSigs, 3)
			require.Len(t, p.Inputs[0].MuSig2PubNonces, 3)
		})
	}
}

// TestAssertLeafContainsKey asserts that the leaf script check backing the
// script spend verification key accepts a key the script pushes and rejects one
// it does not.
func TestAssertLeafContainsKey(t *testing.T) {
	_, keys := bip373Participants(t)
	inScript := schnorr.SerializePubKey(keys[0])
	notInScript := schnorr.SerializePubKey(keys[1])

	checkSig, err := txscript.NewScriptBuilder().AddData(inScript).
		AddOp(txscript.OP_CHECKSIG).Script()
	require.NoError(t, err)

	checkSigAdd, err := txscript.NewScriptBuilder().
		AddData(schnorr.SerializePubKey(keys[2])).
		AddOp(txscript.OP_CHECKSIG).
		AddData(inScript).
		AddOp(txscript.OP_CHECKSIGADD).
		AddInt64(2).
		AddOp(txscript.OP_NUMEQUAL).Script()
	require.NoError(t, err)

	tests := []struct {
		name      string
		script    []byte
		key       []byte
		expectErr bool
	}{{
		name:   "key in single sig leaf",
		script: checkSig,
		key:    inScript,
	}, {
		name:   "key in checksigadd leaf",
		script: checkSigAdd,
		key:    inScript,
	}, {
		name:      "key not in leaf",
		script:    checkSig,
		key:       notInScript,
		expectErr: true,
	}, {
		name:      "empty leaf script",
		script:    nil,
		key:       inScript,
		expectErr: true,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := assertLeafContainsKey(tc.script, tc.key)
			if tc.expectErr {
				require.Error(t, err)
				require.Contains(
					t, err.Error(), "does not contain",
				)

				return
			}

			require.NoError(t, err)
		})
	}
}
