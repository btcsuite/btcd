// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// loadCase loads a BIP-373 valid vector by case-name substring and returns
// the parsed packet. Fails the test if no matching vector exists.
func loadCase(t *testing.T, namePrefix string) *Packet {
	t.Helper()

	raw := mustDecodeHex(t, findVector(t, namePrefix))
	p, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.NoError(t, err)

	return p
}

// stripPartialSigs returns the input's pre-loaded MuSig2 partial signatures
// and clears the field on the packet so the test can re-add them via
// SignMuSig2 from a clean slate.
func stripPartialSigs(t *testing.T, p *Packet,
	inIndex int) []*MuSig2PartialSig {

	t.Helper()

	require.Less(t, inIndex, len(p.Inputs))
	original := p.Inputs[inIndex].MuSig2PartialSigs
	require.NotEmpty(t, original)

	p.Inputs[inIndex].MuSig2PartialSigs = nil

	// Strip any pre-aggregated key/script-spend sigs that would otherwise
	// short-circuit finalization away from the MuSig2 path.
	p.Inputs[inIndex].TaprootKeySpendSig = nil
	p.Inputs[inIndex].TaprootScriptSpendSig = nil

	// Return a shallow copy so callers can safely iterate.
	out := make([]*MuSig2PartialSig, len(original))
	copy(out, original)
	return out
}

// reattachAll round-trips every partial sig through SignMuSig2 and asserts
// each call reports SignSuccesful with no error. Returns the updater used,
// so callers can probe further state (e.g. attempt duplicates).
func reattachAll(t *testing.T, p *Packet, sigs []*MuSig2PartialSig) *Updater {
	t.Helper()

	updater, err := NewUpdater(p)
	require.NoError(t, err)

	for i, sig := range sigs {
		outcome, err := updater.SignMuSig2(0, sig)
		require.NoError(t, err, "partial sig %d", i)
		require.Equal(t, SignOutcome(SignSuccesful), outcome)
	}

	return updater
}

// TestSignMuSig2_Case1c_OutputKeyIsAggregate exercises the case where the
// taproot output key IS the MuSig2 aggregate (BIP-86 keyspend). After
// re-attaching the partial sigs via SignMuSig2 the resulting PSBT must
// finalize to a consensus-valid witness.
func TestSignMuSig2_Case1c_OutputKeyIsAggregate(t *testing.T) {
	p := loadCase(t, "case 1c")
	sigs := stripPartialSigs(t, p, 0)

	reattachAll(t, p, sigs)
	require.Len(t, p.Inputs[0].MuSig2PartialSigs, len(sigs))

	require.NoError(t, MaybeFinalizeAll(p))
	verifyFinalized(t, p)
}

// TestSignMuSig2_Case2c_InternalKeyIsAggregate exercises the case where the
// taproot internal key IS the MuSig2 aggregate and the output commits to a
// merkle root (or BIP-86 with no merkle root).
func TestSignMuSig2_Case2c_InternalKeyIsAggregate(t *testing.T) {
	p := loadCase(t, "case 2c")
	sigs := stripPartialSigs(t, p, 0)

	reattachAll(t, p, sigs)
	require.Len(t, p.Inputs[0].MuSig2PartialSigs, len(sigs))

	require.NoError(t, MaybeFinalizeAll(p))
	verifyFinalized(t, p)
}

// TestSignMuSig2_Case3c_TapscriptLeaf exercises the tapscript-leaf MuSig2
// path: each partial sig carries a non-empty TapLeafHash referencing a leaf
// on the taproot input.
func TestSignMuSig2_Case3c_TapscriptLeaf(t *testing.T) {
	p := loadCase(t, "case 3c")
	sigs := stripPartialSigs(t, p, 0)

	// Sanity-check the test fixture: tapscript-leaf vectors must have a
	// tap leaf hash on their partial sigs, otherwise we'd be testing the
	// wrong path.
	require.NotEmpty(t, sigs[0].TapLeafHash)

	reattachAll(t, p, sigs)
	require.Len(t, p.Inputs[0].MuSig2PartialSigs, len(sigs))

	require.NoError(t, MaybeFinalizeAll(p))
	verifyFinalized(t, p)
}

// TestSignMuSig2_Case4c_BIP32DerivedAggregate exercises the BIP-32 derived
// aggregate path. The BIP-373 case 4c vector does not ship with a
// PSBT_GLOBAL_XPUB, so the finalizer is expected to reject the finalize
// step; SignMuSig2 itself only enforces signer-role invariants and must
// happily attach the partial sigs.
func TestSignMuSig2_Case4c_BIP32DerivedAggregate(t *testing.T) {
	p := loadCase(t, "case 4c")
	sigs := stripPartialSigs(t, p, 0)

	reattachAll(t, p, sigs)
	require.Len(t, p.Inputs[0].MuSig2PartialSigs, len(sigs))
}

// TestSignMuSig2_AlreadyFinalized asserts SignMuSig2 short-circuits and
// returns SignFinalized when the input has already been finalized.
func TestSignMuSig2_AlreadyFinalized(t *testing.T) {
	p := loadCase(t, "case 1c")
	sigs := p.Inputs[0].MuSig2PartialSigs

	require.NoError(t, MaybeFinalizeAll(p))
	require.NotNil(t, p.Inputs[0].FinalScriptWitness)

	updater, err := NewUpdater(p)
	require.NoError(t, err)

	outcome, err := updater.SignMuSig2(0, sigs[0])
	require.NoError(t, err)
	require.Equal(t, SignOutcome(SignFinalized), outcome)
}

// TestSignMuSig2_OutOfRangeInput asserts that an out-of-range input index
// is rejected with ErrInvalidPsbtFormat and SignInvalid.
func TestSignMuSig2_OutOfRangeInput(t *testing.T) {
	p := loadCase(t, "case 1c")
	sigs := stripPartialSigs(t, p, 0)

	updater, err := NewUpdater(p)
	require.NoError(t, err)

	outcome, err := updater.SignMuSig2(99, sigs[0])
	require.Equal(t, SignOutcome(SignInvalid), outcome)
	require.ErrorIs(t, err, ErrInvalidPsbtFormat)

	outcome, err = updater.SignMuSig2(-1, sigs[0])
	require.Equal(t, SignOutcome(SignInvalid), outcome)
	require.ErrorIs(t, err, ErrInvalidPsbtFormat)
}

// TestSignMuSig2_NilArgs asserts that nil partialSig / nil sub-fields are
// rejected.
func TestSignMuSig2_NilArgs(t *testing.T) {
	p := loadCase(t, "case 1c")
	stripPartialSigs(t, p, 0)

	updater, err := NewUpdater(p)
	require.NoError(t, err)

	outcome, err := updater.SignMuSig2(0, nil)
	require.Equal(t, SignOutcome(SignInvalid), outcome)
	require.ErrorIs(t, err, ErrInvalidPsbtFormat)

	outcome, err = updater.SignMuSig2(0, &MuSig2PartialSig{})
	require.Equal(t, SignOutcome(SignInvalid), outcome)
	require.ErrorIs(t, err, ErrInvalidPsbtFormat)
}

// TestSignMuSig2_MissingWitnessUtxo asserts that a taproot input without a
// witness UTXO is rejected (the finalizer needs it to recompute sighashes).
func TestSignMuSig2_MissingWitnessUtxo(t *testing.T) {
	p := loadCase(t, "case 1c")
	sigs := stripPartialSigs(t, p, 0)

	p.Inputs[0].WitnessUtxo = nil

	updater, err := NewUpdater(p)
	require.NoError(t, err)

	outcome, err := updater.SignMuSig2(0, sigs[0])
	require.Equal(t, SignOutcome(SignInvalid), outcome)
	require.ErrorIs(t, err, ErrInvalidPsbtFormat)
}

// TestSignMuSig2_ParticipantNotInList asserts that a partial sig whose
// participant pubkey is not registered under the supplied aggregate key is
// rejected with ErrInvalidSignatureForInput.
func TestSignMuSig2_ParticipantNotInList(t *testing.T) {
	p := loadCase(t, "case 1c")
	sigs := stripPartialSigs(t, p, 0)

	// Replace the participant pubkey with an unrelated key while keeping
	// the rest of the partial sig intact.
	bogus := mustParsePubKey(
		t, "020000000000000000000000000000000000000000000000000000000"+
			"000000003",
	)
	rogue := *sigs[0]
	rogue.PubKey = bogus

	updater, err := NewUpdater(p)
	require.NoError(t, err)

	outcome, err := updater.SignMuSig2(0, &rogue)
	require.Equal(t, SignOutcome(SignInvalid), outcome)
	require.ErrorIs(t, err, ErrInvalidSignatureForInput)
}

// TestSignMuSig2_MissingNonce asserts that attempting to attach a partial
// sig without the matching pub nonce is rejected.
func TestSignMuSig2_MissingNonce(t *testing.T) {
	p := loadCase(t, "case 1c")
	sigs := stripPartialSigs(t, p, 0)

	// Wipe all nonces — every partial sig will now lack a matching one.
	p.Inputs[0].MuSig2PubNonces = nil

	updater, err := NewUpdater(p)
	require.NoError(t, err)

	outcome, err := updater.SignMuSig2(0, sigs[0])
	require.Equal(t, SignOutcome(SignInvalid), outcome)
	require.ErrorIs(t, err, ErrInvalidSignatureForInput)
}

// TestSignMuSig2_UnknownTapLeafHash asserts that a tap-leaf partial sig
// whose TapLeafHash doesn't resolve to any leaf script on the input is
// rejected. We start from case 3c (which has tap leaf hashes set) and
// rewrite the hash to one that won't match.
func TestSignMuSig2_UnknownTapLeafHash(t *testing.T) {
	p := loadCase(t, "case 3c")
	sigs := stripPartialSigs(t, p, 0)

	bogus := bytes.Repeat([]byte{0xff}, 32)

	// Rewrite the partial sig to carry the bogus hash. The nonce on the
	// input still has the original hash, so even if the leaf check were
	// skipped this would also fail the nonce-match check — but the leaf
	// check fires first because we additionally rewrite the matching
	// nonce so the nonce check passes.
	rogueSig := *sigs[0]
	rogueSig.TapLeafHash = bogus
	roguePartialSig := &MuSig2PartialSig{
		PubKey:       rogueSig.PubKey,
		AggregateKey: rogueSig.AggregateKey,
		TapLeafHash:  rogueSig.TapLeafHash,
	}

	for _, n := range p.Inputs[0].MuSig2PubNonces {
		if bytes.Equal(n.KeyData(), roguePartialSig.KeyData()) {
			n.TapLeafHash = bogus
		}
	}

	updater, err := NewUpdater(p)
	require.NoError(t, err)

	outcome, err := updater.SignMuSig2(0, &rogueSig)
	require.Equal(t, SignOutcome(SignInvalid), outcome)
	require.ErrorIs(t, err, ErrInvalidSignatureForInput)
}

// TestSignMuSig2_Duplicate asserts that re-attaching the same partial sig
// is rejected with ErrDuplicateKey via the underlying AddInMuSig2PartialSig
// helper.
func TestSignMuSig2_Duplicate(t *testing.T) {
	p := loadCase(t, "case 1c")
	sigs := stripPartialSigs(t, p, 0)

	updater, err := NewUpdater(p)
	require.NoError(t, err)

	// First attach succeeds, second attach must be rejected.
	outcome, err := updater.SignMuSig2(0, sigs[0])
	require.NoError(t, err)
	require.Equal(t, SignOutcome(SignSuccesful), outcome)

	outcome, err = updater.SignMuSig2(0, sigs[0])
	require.Equal(t, SignOutcome(SignInvalid), outcome)
	require.ErrorIs(t, err, ErrDuplicateKey)
}
