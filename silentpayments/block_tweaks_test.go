package silentpayments

import (
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/btcsuite/btclog"
	"github.com/stretchr/testify/require"
)

// TestPrivKeyTweak tests the tweaking of private keys for receiving silent
// payments, using the test vectors from the BIP.
func TestPrivKeyTweak(t *testing.T) {
	vectors, err := ReadTestVectors()
	require.NoError(t, err)

	for _, vector := range vectors {
		for _, receiving := range vector.Receiving {
			t.Run(vector.Comment, func(tt *testing.T) {
				runReceivingPrivKeyTweakTest(tt, receiving)
			})
		}
	}
}

// runReceivingPrivKeyTweakTest runs a single receiving private key tweak test.
func runReceivingPrivKeyTweakTest(t *testing.T, r *Receiving) {
	givenInputs := parseInputs(t, r.Given.Vin)
	prevOutputs := make(map[wire.OutPoint][]byte)
	tx := &wire.MsgTx{
		TxIn: make([]*wire.TxIn, len(givenInputs)),
	}

	for idx, input := range givenInputs {
		prevOutputs[input.OutPoint] = input.Utxo.PkScript

		sigScript, err := hex.DecodeString(r.Given.Vin[idx].ScriptSig)
		require.NoError(t, err)

		witnessBytes, err := hex.DecodeString(
			r.Given.Vin[idx].TxInWitness,
		)
		require.NoError(t, err)
		witness, err := parseWitness(witnessBytes)
		require.NoError(t, err)

		tx.TxIn[idx] = &wire.TxIn{
			PreviousOutPoint: input.OutPoint,
			SignatureScript:  sigScript,
			Witness:          witness,
		}
	}
	prevOutFetcher := func(op wire.OutPoint) ([]byte, error) {
		pkScript, ok := prevOutputs[op]
		if !ok {
			return nil, fmt.Errorf("previous output not found")
		}

		return pkScript, nil
	}

	logger := btclog.NewBackend(os.Stdout)
	sumKey := SumInputPubKeys(tx, prevOutFetcher, logger.Logger("TEST"))

	if sumKey == nil || !sumKey.IsOnCurve() {
		require.Empty(t, r.Expected.Outputs)

		return
	}

	inputHash, err := CalculateInputHashTweak(
		slices.Collect(maps.Keys(prevOutputs)), sumKey,
	)
	require.NoError(t, err)

	scanKey, spendKey, err := r.Given.KeyMaterial.Parse()
	require.NoError(t, err)

	shareSum := ScalarMult(scanKey.Key, sumKey)
	spendPubKey := spendKey.PubKey()

	outputsToCheck := make(map[int]struct{})
	for idx := range r.Expected.Outputs {
		outputsToCheck[idx] = struct{}{}
	}

	checkOutput := func(toCheck map[int]struct{}, k int,
		spendPubKey btcec.PublicKey) bool {

		for idx := range toCheck {
			expectedOutput := r.Expected.Outputs[idx]
			expectedKey, err := expectedOutput.ParsePubKey()
			require.NoError(t, err)

			changeLabel := LabelTweak(scanKey, 0)
			match, err := OutputMatches(
				*expectedKey, spendPubKey, changeLabel,
				*shareSum, uint32(k), *inputHash,
			)
			require.NoError(t, err)

			if match {
				delete(toCheck, idx)

				return true
			}

			// We now try with the actual labels.
			for _, givenLabel := range r.Given.Labels {
				label := LabelTweak(scanKey, givenLabel)
				match, err = OutputMatches(
					*expectedKey, spendPubKey, label,
					*shareSum, uint32(k), *inputHash,
				)
				require.NoError(t, err)

				if match {
					delete(toCheck, idx)

					return true
				}
			}
		}

		return false
	}

	for k := 0; k < len(r.Expected.Outputs); k++ {
		found := checkOutput(outputsToCheck, k, *spendPubKey)
		if !found || len(outputsToCheck) == 0 {
			break
		}
	}

	require.Empty(t, outputsToCheck)
}

// TestSumInputPubKeysInfinity tests that an input public key sum at the
// point at infinity skips the transaction instead of producing a bogus
// all-zero tweak point. This happened in the wild (signet block 198023
// spends two outputs whose public keys share the same x coordinate with
// opposite parity), and the resulting 0x02||0x00...00 "tweak" was served to
// light clients before this check existed.
func TestSumInputPubKeysInfinity(t *testing.T) {
	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	pubKey := privKey.PubKey()
	negKey := Negate(pubKey)

	otherPriv, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	otherKey := otherPriv.PubKey()

	// P + (-P) sums to the point at infinity: no valid key.
	require.Nil(t, sumPubKeys([]*btcec.PublicKey{pubKey, negKey}))

	// Only the final sum matters: an intermediate sum at infinity must
	// not abort the summation (official vector "Input keys intermediate
	// sum is zero but final sum is non-zero").
	sum := sumPubKeys([]*btcec.PublicKey{pubKey, negKey, otherKey})
	require.NotNil(t, sum)
	require.True(t, sum.IsEqual(otherKey))

	// End to end: a transaction whose eligible inputs sum to infinity
	// yields no tweak data at all.
	sig := ecdsa.Sign(privKey, chainhash.HashB([]byte("test")))
	sigBytes := append(sig.Serialize(), byte(txscript.SigHashAll))
	tx := &wire.MsgTx{
		TxIn: []*wire.TxIn{
			{Witness: wire.TxWitness{
				sigBytes, pubKey.SerializeCompressed(),
			}},
			{Witness: wire.TxWitness{
				sigBytes, negKey.SerializeCompressed(),
			}},
		},
		TxOut: []*wire.TxOut{{
			Value: 1000,
			PkScript: append(
				[]byte{txscript.OP_1, txscript.OP_DATA_32},
				schnorr.SerializePubKey(otherKey)...,
			),
		}},
	}
	prevOutFetcher := func(wire.OutPoint) ([]byte, error) {
		return nil, fmt.Errorf("no prev out needed")
	}

	require.Nil(t, SumInputPubKeys(tx, prevOutFetcher, nil))

	tweak, err := TransactionTweakData(tx, prevOutFetcher, nil)
	require.NoError(t, err)
	require.Nil(t, tweak)
}

// TestPublicKeyFromInputRejects tests that inputs BIP-0352 excludes from
// shared secret derivation do not yield a public key: uncompressed and
// hybrid keys, and script shapes that merely look like one of the supported
// types.
func TestPublicKeyFromInputRejects(t *testing.T) {
	// A valid DER signature and key material to build realistic witness
	// stacks and scriptSigs from.
	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	sig := ecdsa.Sign(privKey, chainhash.HashB([]byte("test")))
	sigBytes := append(
		sig.Serialize(), byte(txscript.SigHashAll),
	)

	compressed := privKey.PubKey().SerializeCompressed()
	uncompressed := privKey.PubKey().SerializeUncompressed()

	// A hybrid key is an uncompressed key whose format byte also encodes
	// the parity of Y (0x06 for even, 0x07 for odd), a long-deprecated
	// format that ParsePubKey still accepts.
	hybrid := slices.Clone(uncompressed)
	hybrid[0] = 0x06 + (compressed[0] - 0x02)

	pushData := func(items ...[]byte) []byte {
		builder := txscript.NewScriptBuilder()
		for _, item := range items {
			builder.AddData(item)
		}
		script, err := builder.Script()
		require.NoError(t, err)

		return script
	}

	p2pkhScript := func(pubKey []byte) []byte {
		script, err := txscript.NewScriptBuilder().
			AddOp(txscript.OP_DUP).
			AddOp(txscript.OP_HASH160).
			AddData(address.Hash160(pubKey)).
			AddOp(txscript.OP_EQUALVERIFY).
			AddOp(txscript.OP_CHECKSIG).
			Script()
		require.NoError(t, err)

		return script
	}
	p2wpkhScript := func(pubKey []byte) []byte {
		script, err := txscript.NewScriptBuilder().
			AddOp(txscript.OP_0).
			AddData(address.Hash160(pubKey)).
			Script()
		require.NoError(t, err)

		return script
	}
	p2shScript := func(redeem []byte) []byte {
		script, err := txscript.NewScriptBuilder().
			AddOp(txscript.OP_HASH160).
			AddData(address.Hash160(redeem)).
			AddOp(txscript.OP_EQUAL).
			Script()
		require.NoError(t, err)

		return script
	}

	testCases := []struct {
		name      string
		sigScript []byte
		witness   wire.TxWitness
		prevOut   []byte
	}{{
		// BIP-0352: "only X-only and compressed public keys are
		// permitted".
		name:    "P2WPKH with uncompressed key",
		witness: wire.TxWitness{sigBytes, uncompressed},
		prevOut: p2wpkhScript(uncompressed),
	}, {
		name:    "P2WPKH with hybrid key",
		witness: wire.TxWitness{sigBytes, hybrid},
		prevOut: p2wpkhScript(hybrid),
	}, {
		name:      "P2PKH with uncompressed key",
		sigScript: pushData(sigBytes, uncompressed),
		prevOut:   p2pkhScript(uncompressed),
	}, {
		name:      "P2SH-P2WPKH with uncompressed key",
		sigScript: pushData(p2wpkhScript(uncompressed)),
		witness:   wire.TxWitness{sigBytes, uncompressed},
		prevOut:   p2shScript(p2wpkhScript(uncompressed)),
	}, {
		// A P2SH-P2WSH spend whose witness script is exactly a
		// parseable 33-byte public key must not be mistaken for a
		// nested P2WPKH spend — its scriptSig pushes a witness
		// program of a different version/length than the canonical
		// P2WPKH redeem script.
		name: "P2SH-P2WSH witness-script lookalike",
		sigScript: pushData(append(
			[]byte{txscript.OP_0, txscript.OP_DATA_32},
			chainhash.HashB(compressed)...,
		)),
		witness: wire.TxWitness{sigBytes, compressed},
		prevOut: p2shScript(append(
			[]byte{txscript.OP_0, txscript.OP_DATA_32},
			chainhash.HashB(compressed)...,
		)),
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			prevOutFetcher := func(wire.OutPoint) ([]byte, error) {
				return tc.prevOut, nil
			}

			_, err := PublicKeyFromInput(&wire.TxIn{
				SignatureScript: tc.sigScript,
				Witness:         tc.witness,
			}, prevOutFetcher)
			require.Error(tt, err)
		})
	}
}

// TestPublicKeyFromInputShortControlBlock tests that a taproot witness that
// looks like a script path spend but carries a control block shorter than
// the mandatory 33 bytes yields an error instead of a panic. Such witnesses
// cannot appear in valid blocks, but the function is also fed unconfirmed
// transactions.
func TestPublicKeyFromInputShortControlBlock(t *testing.T) {
	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	p2trScript := append(
		[]byte{txscript.OP_1, txscript.OP_DATA_32},
		schnorr.SerializePubKey(privKey.PubKey())...,
	)
	prevOutFetcher := func(wire.OutPoint) ([]byte, error) {
		return p2trScript, nil
	}

	require.NotPanics(t, func() {
		_, err := PublicKeyFromInput(&wire.TxIn{
			Witness: wire.TxWitness{{0x01}, {0x02}},
		}, prevOutFetcher)
		require.ErrorIs(t, err, ErrInvalidTaprootWitness)
	})
}

// TestPublicKeyFromInput tests the extraction of public keys from transaction
// inputs. It uses a selection of real mainnet transactions (from the first
// couple of blocks after Taproot activation) that contain the various supported
// input types.
func TestPublicKeyFromInput(t *testing.T) {
	tc, err := ReadTxInPubKeyTestCases()
	require.NoError(t, err)

	for _, testCase := range tc {
		prevOutFetcher := func(op wire.OutPoint) ([]byte, error) {
			if len(testCase.PrevOutScript) == 0 {
				return nil, fmt.Errorf("no prev out script")
			}

			return hex.DecodeString(testCase.PrevOutScript)
		}

		txIn, err := testCase.AsTxIn()
		require.NoError(t, err)

		expectedPubKeyBytes, err := hex.DecodeString(
			testCase.ExpectedPubKey,
		)
		require.NoError(t, err)

		if len(expectedPubKeyBytes) == 33 {
			expectedPubKeyBytes = expectedPubKeyBytes[1:]
		}

		parsedKey, err := PublicKeyFromInput(txIn, prevOutFetcher)
		require.NoError(t, err)

		require.Equal(
			t, expectedPubKeyBytes,
			schnorr.SerializePubKey(parsedKey),
		)
	}
}
