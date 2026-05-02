package silentpayments

import (
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/wire"
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
