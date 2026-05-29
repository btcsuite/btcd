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

// restrictSigHashSpend is a single-input spend used to exercise the
// ScriptVerifyRestrictSigHash policy. It contains everything needed to execute
// the input's scripts with the script engine.
type restrictSigHashSpend struct {
	pkScript    []byte
	tx          *wire.MsgTx
	value       int64
	prevFetcher *CannedPrevOutputFetcher
	sigHashes   *TxSigHashes
}

// spendBuilder builds a single-input spend of the given sighash type in one of
// the signature-verification contexts the ScriptVerifyRestrictSigHash policy is
// exercised in.
type spendBuilder func(t *testing.T,
	hashType SigHashType) *restrictSigHashSpend

// newRestrictSigHashSpend returns a fresh, unsigned single-input,
// single-output spend of the given pkScript, along with a prevout fetcher and
// sighash cache. The single output makes SIGHASH_SINGLE valid for input 0.
func newRestrictSigHashSpend(pkScript []byte) *restrictSigHashSpend {
	const value = 1e8

	tx := wire.NewMsgTx(2)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{Index: 0},
	})
	tx.AddTxOut(&wire.TxOut{Value: value, PkScript: pkScript})

	prevFetcher := NewCannedPrevOutputFetcher(pkScript, value)

	return &restrictSigHashSpend{
		pkScript:    pkScript,
		tx:          tx,
		value:       value,
		prevFetcher: prevFetcher,
		sigHashes:   NewTxSigHashes(tx, prevFetcher),
	}
}

// checkSigScript returns a script that checks a signature against the given
// public key.
func checkSigScript(t *testing.T, pubKey []byte) []byte {
	t.Helper()

	script, err := NewScriptBuilder().
		AddData(pubKey).
		AddOp(OP_CHECKSIG).
		Script()
	require.NoError(t, err)

	return script
}

// taprootKeySpend returns a spend of a BIP-86 taproot output via the key path,
// signed with the given sighash type.
func taprootKeySpend(t *testing.T,
	hashType SigHashType) *restrictSigHashSpend {

	t.Helper()

	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	outputKey := ComputeTaprootKeyNoScript(privKey.PubKey())
	pkScript, err := PayToTaprootScript(outputKey)
	require.NoError(t, err)

	spend := newRestrictSigHashSpend(pkScript)
	witness, err := TaprootWitnessSignature(
		spend.tx, spend.sigHashes, 0, spend.value, pkScript, hashType,
		privKey,
	)
	require.NoError(t, err)
	spend.tx.TxIn[0].Witness = witness

	return spend
}

// tapscriptSpend returns a spend of a taproot output via a script path whose
// leaf is a single <pubkey> OP_CHECKSIG, signed with the given sighash type.
func tapscriptSpend(t *testing.T, hashType SigHashType) *restrictSigHashSpend {
	t.Helper()

	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	internalKey := privKey.PubKey()
	leafScript := checkSigScript(t, schnorr.SerializePubKey(internalKey))

	tapLeaf := NewBaseTapLeaf(leafScript)
	tree := AssembleTaprootScriptTree(tapLeaf)
	ctrlBlock := tree.LeafMerkleProofs[0].ToControlBlock(internalKey)
	rootHash := tree.RootNode.TapHash()
	outputKey := ComputeTaprootOutputKey(internalKey, rootHash[:])
	pkScript, err := PayToTaprootScript(outputKey)
	require.NoError(t, err)

	spend := newRestrictSigHashSpend(pkScript)
	sig, err := RawTxInTapscriptSignature(
		spend.tx, spend.sigHashes, 0, spend.value, pkScript, tapLeaf,
		hashType, privKey,
	)
	require.NoError(t, err)

	ctrlBlockBytes, err := ctrlBlock.ToBytes()
	require.NoError(t, err)
	spend.tx.TxIn[0].Witness = wire.TxWitness{
		sig, leafScript, ctrlBlockBytes,
	}

	return spend
}

// p2wshECDSASpend returns a spend of a P2WSH output whose witness script is a
// single <pubkey> OP_CHECKSIG, signed with the given sighash type.
func p2wshECDSASpend(t *testing.T,
	hashType SigHashType) *restrictSigHashSpend {

	t.Helper()

	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	witnessScript := checkSigScript(
		t, privKey.PubKey().SerializeCompressed(),
	)
	scriptHash := sha256.Sum256(witnessScript)
	pkScript, err := payToWitnessScriptHashScript(scriptHash[:])
	require.NoError(t, err)

	spend := newRestrictSigHashSpend(pkScript)
	sig, err := RawTxInWitnessSignature(
		spend.tx, spend.sigHashes, 0, spend.value, witnessScript,
		hashType, privKey,
	)
	require.NoError(t, err)
	spend.tx.TxIn[0].Witness = wire.TxWitness{sig, witnessScript}

	return spend
}

// TestDisallowSighashDefaultNonTaproot verifies that SIGHASH_DEFAULT (taproot)
// is not allowed for non-taproot scripts.
func TestDisallowSighashDefaultNonTaproot(t *testing.T) {
	spend := p2wshECDSASpend(t, SigHashDefault)

	vm, err := NewEngine(
		spend.pkScript, spend.tx, 0, ScriptBip16|ScriptVerifyWitness|
			ScriptVerifyTaproot|ScriptVerifyRestrictSigHash,
		nil, spend.sigHashes, spend.value, spend.prevFetcher,
	)
	require.NoError(t, err)

	err = vm.Execute()
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
			build:           taprootKeySpend,
		},
		{
			name:            "tapscript",
			supportsDefault: true,
			build:           tapscriptSpend,
		},
		{
			name:            "p2wsh-ecdsa",
			supportsDefault: false,
			build:           p2wshECDSASpend,
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

				// The same signed spend is executed with and
				// without the policy flag.
				spend := ctx.build(t, hashType)
				execute := func(flags ScriptFlags) error {
					vm, err := NewEngine(
						spend.pkScript, spend.tx, 0,
						flags, nil, spend.sigHashes,
						spend.value, spend.prevFetcher,
					)
					require.NoError(t, err)

					return vm.Execute()
				}

				// Without the policy flag, every sighash type
				// must be accepted.
				err := execute(StandardVerifyFlags)
				require.NoError(t, err, "spend should be "+
					"valid without the restrict-sighash "+
					"flag")

				// With the policy flag, only SIGHASH_DEFAULT
				// and SIGHASH_ALL are accepted.
				err = execute(
					StandardVerifyFlags |
						ScriptVerifyRestrictSigHash,
				)
				if allowedSigHash(hashType) {
					require.NoError(t, err, "allowed "+
						"sighash type must pass with "+
						"the flag")

					return
				}

				require.True(t, IsErrorCode(
					err, ErrDisallowedSigHashType,
				), "disallowed sighash type must fail with "+
					"ErrDisallowedSigHashType, got: %v",
					err)
			})
		}
	}
}
