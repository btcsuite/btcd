// Copyright (c) 2013-2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// spendBuilder builds and executes a single-input spend of the given sighash
// type under the given script flags, returning the engine's Execute error. It
// is used to exercise the ScriptVerifyRestrictSigHash policy across the
// different signature-verification contexts.
type spendBuilder func(t *testing.T, hashType SigHashType,
	flags ScriptFlags) error

// newRestrictSigHashTx returns a fresh single-input, single-output transaction
// spending the given pkScript, along with a prevout fetcher and sighash cache.
// The single output makes SIGHASH_SINGLE valid for input 0.
func newRestrictSigHashTx(pkScript []byte, value int64) (*wire.MsgTx,
	*CannedPrevOutputFetcher, *TxSigHashes) {

	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 0},
	})
	tx.AddTxOut(&wire.TxOut{Value: value, PkScript: pkScript})

	prevFetcher := NewCannedPrevOutputFetcher(pkScript, value)
	sigHashes := NewTxSigHashes(tx, prevFetcher)

	return tx, prevFetcher, sigHashes
}

// taprootKeySpendBuilder spends a BIP-86 taproot output via the key path.
func taprootKeySpendBuilder(t *testing.T, hashType SigHashType,
	flags ScriptFlags) error {

	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	outputKey := ComputeTaprootKeyNoScript(privKey.PubKey())
	pkScript, err := PayToTaprootScript(outputKey)
	require.NoError(t, err)

	const value = 1e8
	tx, prevFetcher, sigHashes := newRestrictSigHashTx(pkScript, value)

	witness, err := TaprootWitnessSignature(
		tx, sigHashes, 0, value, pkScript, hashType, privKey,
	)
	require.NoError(t, err)
	tx.TxIn[0].Witness = witness

	vm, err := NewEngine(
		pkScript, tx, 0, flags, nil, sigHashes, value, prevFetcher,
	)
	require.NoError(t, err)

	return vm.Execute()
}

// tapscriptBuilder spends a taproot output via a script path whose leaf is a
// single <pubkey> OP_CHECKSIG.
func tapscriptBuilder(t *testing.T, hashType SigHashType,
	flags ScriptFlags) error {

	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	internalKey := privKey.PubKey()
	leafScript, err := NewScriptBuilder().
		AddData(schnorr.SerializePubKey(internalKey)).
		AddOp(OP_CHECKSIG).
		Script()
	require.NoError(t, err)

	tapLeaf := NewBaseTapLeaf(leafScript)
	tree := AssembleTaprootScriptTree(tapLeaf)
	ctrlBlock := tree.LeafMerkleProofs[0].ToControlBlock(internalKey)
	rootHash := tree.RootNode.TapHash()
	outputKey := ComputeTaprootOutputKey(internalKey, rootHash[:])
	pkScript, err := PayToTaprootScript(outputKey)
	require.NoError(t, err)

	const value = 1e8
	tx, prevFetcher, sigHashes := newRestrictSigHashTx(pkScript, value)

	sig, err := RawTxInTapscriptSignature(
		tx, sigHashes, 0, value, pkScript, tapLeaf, hashType, privKey,
	)
	require.NoError(t, err)

	ctrlBlockBytes, err := ctrlBlock.ToBytes()
	require.NoError(t, err)
	tx.TxIn[0].Witness = wire.TxWitness{sig, leafScript, ctrlBlockBytes}

	vm, err := NewEngine(
		pkScript, tx, 0, flags, nil, sigHashes, value, prevFetcher,
	)
	require.NoError(t, err)

	return vm.Execute()
}

// p2wshECDSABuilder spends a P2WSH output whose witness script is a single
// <pubkey> OP_CHECKSIG, exercising the segwit-v0 ECDSA verification path.
func p2wshECDSABuilder(t *testing.T, hashType SigHashType,
	flags ScriptFlags) error {

	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	witnessScript, err := NewScriptBuilder().
		AddData(privKey.PubKey().SerializeCompressed()).
		AddOp(OP_CHECKSIG).
		Script()
	require.NoError(t, err)

	scriptHash := sha256.Sum256(witnessScript)
	pkScript, err := NewScriptBuilder().
		AddOp(OP_0).
		AddData(scriptHash[:]).
		Script()
	require.NoError(t, err)

	const value = 1e8
	tx, prevFetcher, sigHashes := newRestrictSigHashTx(pkScript, value)

	sig, err := RawTxInWitnessSignature(
		tx, sigHashes, 0, value, witnessScript, hashType, privKey,
	)
	require.NoError(t, err)
	tx.TxIn[0].Witness = wire.TxWitness{sig, witnessScript}

	vm, err := NewEngine(
		pkScript, tx, 0, flags, nil, sigHashes, value, prevFetcher,
	)
	require.NoError(t, err)

	return vm.Execute()
}

// TestDisallowSighashDefaultNonTaproot verifies that SIGHASH_DEFAULT (taproot)
// is not allowed for non-taproot scripts.
func TestDisallowSighashDefaultNonTaproot(t *testing.T) {
	err := p2wshECDSABuilder(
		t, SigHashDefault, ScriptBip16|
			ScriptVerifyWitness|ScriptVerifyTaproot|
			ScriptVerifyRestrictSigHash,
	)
	require.Error(t, err)
	require.True(t, IsErrorCode(err, ErrInvalidSigHashType))
}

// TestScriptVerifyRestrictSigHash verifies the ScriptVerifyRestrictSigHash
// policy across the taproot key-spend, tapscript, and segwit-v0 ECDSA
// signature-verification contexts. With the flag unset every sighash type is
// accepted (proving the policy is strictly opt-in); with the flag set only
// SIGHASH_DEFAULT (taproot) and SIGHASH_ALL are accepted, and any other type
// fails with ErrDisallowedSigHashType.
func TestScriptVerifyRestrictSigHash(t *testing.T) {
	t.Parallel()

	// allowedSigHash reports whether a sighash type is permitted by the
	// policy: SIGHASH_DEFAULT (taproot implicit) and SIGHASH_ALL only.
	allowedSigHash := func(hashType SigHashType) bool {
		return hashType == SigHashDefault || hashType == SigHashAll
	}

	contexts := []struct {
		name string

		// supportsDefault indicates whether the context can carry an
		// implicit SIGHASH_DEFAULT (only taproot signatures can).
		supportsDefault bool

		build spendBuilder
	}{
		{
			name:            "taproot-keyspend",
			supportsDefault: true,
			build:           taprootKeySpendBuilder,
		},
		{
			name:            "tapscript",
			supportsDefault: true,
			build:           tapscriptBuilder,
		},
		{
			name:            "p2wsh-ecdsa",
			supportsDefault: false,
			build:           p2wshECDSABuilder,
		},
	}

	// The set of sighash types exercised. SIGHASH_DEFAULT is only valid for
	// taproot signatures and is filtered out for the ECDSA context.
	sigHashTypes := []SigHashType{
		SigHashDefault,
		SigHashAll,
		SigHashNone,
		SigHashSingle,
		SigHashAll | SigHashAnyOneCanPay,
		SigHashNone | SigHashAnyOneCanPay,
		SigHashSingle | SigHashAnyOneCanPay,
	}

	for _, ctx := range contexts {
		for _, hashType := range sigHashTypes {
			if hashType == SigHashDefault && !ctx.supportsDefault {
				continue
			}

			name := fmt.Sprintf("%s/sighash=0x%02x", ctx.name,
				hashType)
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				// Without the policy flag, every sighash type
				// must be accepted.
				err := ctx.build(t, hashType, StandardVerifyFlags)
				require.NoError(t, err, "spend should be valid "+
					"without the restrict-sighash flag")

				// With the policy flag, only SIGHASH_DEFAULT and
				// SIGHASH_ALL are accepted.
				err = ctx.build(
					t, hashType, StandardVerifyFlags|
						ScriptVerifyRestrictSigHash,
				)
				if allowedSigHash(hashType) {
					require.NoError(t, err, "allowed sighash "+
						"type must pass with the flag")
				} else {
					require.True(t, IsErrorCode(
						err, ErrDisallowedSigHashType,
					), "disallowed sighash type must fail "+
						"with ErrDisallowedSigHashType, "+
						"got: %v", err)
				}
			})
		}
	}
}
