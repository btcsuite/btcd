package silentpayments

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
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

// runReceivingPrivKeyTweakTest runs a single receiving test vector through
// the same flow a scanning client uses: derive the transaction tweak (the
// value a tweak-data index server would serve), the ECDH shared secret and
// the candidate output keys per output index k, pairing them against the
// transaction's taproot outputs. All expected intermediate values of the
// vector are asserted: the tweak, the shared secret, each found output's
// private key tweak (which must complete the spend key to the output key)
// and the found-output count, honoring the K_max scan limit.
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

	// The given outputs become the transaction's taproot outputs, which
	// also makes the transaction pass the has-taproot-output eligibility
	// check of TransactionTweakData.
	candidates := make(map[[32]byte]struct{}, len(r.Given.Outputs))
	for _, outputHex := range r.Given.Outputs {
		outputBytes, err := hex.DecodeString(outputHex)
		require.NoError(t, err)
		require.Len(t, outputBytes, 32)

		tx.TxOut = append(tx.TxOut, &wire.TxOut{
			Value: 1000,
			PkScript: append(
				[]byte{txscript.OP_1, txscript.OP_DATA_32},
				outputBytes...,
			),
		})

		var xOnly [32]byte
		copy(xOnly[:], outputBytes)
		candidates[xOnly] = struct{}{}
	}

	prevOutFetcher := func(op wire.OutPoint) ([]byte, error) {
		pkScript, ok := prevOutputs[op]
		if !ok {
			return nil, fmt.Errorf("previous output not found")
		}

		return pkScript, nil
	}

	// Derive the transaction tweak end to end, exactly like a tweak-data
	// index server does.
	logger := btclog.NewBackend(os.Stdout)
	tweakData, err := TransactionTweakData(
		tx, prevOutFetcher, logger.Logger("TEST"),
	)
	require.NoError(t, err)

	// A vector without an expected tweak describes a transaction that
	// must be skipped (no eligible inputs, or an input key sum at the
	// point at infinity).
	if r.Expected.Tweak == "" {
		require.Nil(t, tweakData)
		require.Empty(t, r.Expected.Outputs)

		return
	}
	require.NotNil(t, tweakData)
	require.Equal(
		t, r.Expected.Tweak,
		hex.EncodeToString(tweakData.SerializeCompressed()),
	)

	scanKey, spendKey, err := r.Given.KeyMaterial.Parse()
	require.NoError(t, err)
	spendPubKey := spendKey.PubKey()

	// The scanner multiplies the served tweak by its scan key to arrive
	// at the full ECDH shared secret.
	sharedSecret := ScalarMult(scanKey.Key, tweakData)
	require.Equal(
		t, r.Expected.SharedSecret,
		hex.EncodeToString(sharedSecret.SerializeCompressed()),
	)

	// The label variants every scanner tracks: the plain spend key, the
	// change label (m=0, always scanned per the BIP) and the wallet's
	// published labels.
	labelTweaks := []*btcec.ModNScalar{nil, LabelTweak(scanKey, 0)}
	for _, m := range r.Given.Labels {
		if m == 0 {
			continue
		}
		labelTweaks = append(labelTweaks, LabelTweak(scanKey, m))
	}

	expectedByKey := make(map[string]*Output, len(r.Expected.Outputs))
	for _, output := range r.Expected.Outputs {
		expectedByKey[output.PubKey] = output
	}

	// The served tweak already contains the input hash factor, so the
	// scalar one keeps CreateOutputKey from applying it again.
	var one btcec.ModNScalar
	one.SetInt(1)

	// Walk output index k per BIP-0352 continuation semantics: k only
	// advances while the current k yields a match, and scanning stops at
	// the K_max limit.
	found := 0
	for k := uint32(0); len(candidates) > 0; k++ {
		// Spec: If k == K_max (=2323), stop scanning.
		if k == MaxRecipientsPerGroup {
			break
		}

		matchedAtK := false
		for _, labelTweak := range labelTweaks {
			spendVariant := LabelSpendKey(labelTweak, spendPubKey)
			outputKey, err := CreateOutputKey(
				*sharedSecret, *spendVariant, k, one,
			)
			require.NoError(t, err)

			var xOnly [32]byte
			copy(xOnly[:], schnorr.SerializePubKey(outputKey))
			if _, ok := candidates[xOnly]; !ok {
				continue
			}

			delete(candidates, xOnly)
			matchedAtK = true
			found++

			// Vectors that list expected outputs pin the exact
			// private key tweak of every found output.
			if len(r.Expected.Outputs) == 0 {
				continue
			}
			xOnlyHex := hex.EncodeToString(xOnly[:])
			expected, ok := expectedByKey[xOnlyHex]
			require.True(
				t, ok, "found unexpected output %v", xOnlyHex,
			)

			// t_k = hash(ser(shared_secret) || ser32(k)), plus
			// the label tweak for labeled matches.
			payload := make([]byte, 33+4)
			copy(payload, sharedSecret.SerializeCompressed())
			binary.BigEndian.PutUint32(payload[33:], k)
			tweakHash := chainhash.TaggedHash(
				TagBIP0352SharedSecret, payload,
			)
			var privKeyTweak btcec.ModNScalar
			privKeyTweak.SetBytes((*[32]byte)(tweakHash))
			if labelTweak != nil {
				privKeyTweak.Add(labelTweak)
			}

			tweakBytes := privKeyTweak.Bytes()
			require.Equal(
				t, expected.PrivKeyTweak,
				hex.EncodeToString(tweakBytes[:]),
			)

			// The tweak must complete the spend private key to
			// the found output key, otherwise the wallet could
			// detect the payment but never spend it.
			fullKey := spendKey.Key
			fullKey.Add(&privKeyTweak)
			require.Equal(
				t, xOnly[:],
				schnorr.SerializePubKey(ScalarBaseMult(fullKey)),
			)
		}

		if !matchedAtK {
			break
		}
	}

	if r.Expected.NumOutputs > 0 {
		require.EqualValues(t, r.Expected.NumOutputs, found)
	} else {
		require.Equal(t, len(r.Expected.Outputs), found)
	}
}

// TestOutputMatches tests the library's single-output convenience matcher
// against a base payment and a change-labeled payment from the official
// vectors.
func TestOutputMatches(t *testing.T) {
	vectors, err := ReadTestVectors()
	require.NoError(t, err)

	runMatch := func(t *testing.T, comment string) {
		var r *Receiving
		for _, vector := range vectors {
			if vector.Comment == comment {
				r = vector.Receiving[0]
			}
		}
		require.NotNil(t, r, "vector not found: %v", comment)

		givenInputs := parseInputs(t, r.Given.Vin)
		outpoints := make([]wire.OutPoint, len(givenInputs))
		pubKeys := make([]*btcec.PublicKey, 0, len(givenInputs))
		prevOutputs := make(map[wire.OutPoint][]byte)
		for idx, input := range givenInputs {
			outpoints[idx] = input.OutPoint
			prevOutputs[input.OutPoint] = input.Utxo.PkScript
		}
		for idx := range givenInputs {
			sigScript, err := hex.DecodeString(
				r.Given.Vin[idx].ScriptSig,
			)
			require.NoError(t, err)
			witnessBytes, err := hex.DecodeString(
				r.Given.Vin[idx].TxInWitness,
			)
			require.NoError(t, err)
			witness, err := parseWitness(witnessBytes)
			require.NoError(t, err)

			pubKey, err := PublicKeyFromInput(&wire.TxIn{
				PreviousOutPoint: outpoints[idx],
				SignatureScript:  sigScript,
				Witness:          witness,
			}, func(op wire.OutPoint) ([]byte, error) {
				return prevOutputs[op], nil
			})
			require.NoError(t, err)
			pubKeys = append(pubKeys, pubKey)
		}

		sumKey := sumPubKeys(pubKeys)
		require.NotNil(t, sumKey)
		inputHash, err := CalculateInputHashTweak(outpoints, sumKey)
		require.NoError(t, err)

		scanKey, spendKey, err := r.Given.KeyMaterial.Parse()
		require.NoError(t, err)
		shareSum := ScalarMult(scanKey.Key, sumKey)
		changeLabel := LabelTweak(scanKey, 0)

		expectedKey, err := r.Expected.Outputs[0].ParsePubKey()
		require.NoError(t, err)

		match, err := OutputMatches(
			*expectedKey, *spendKey.PubKey(), changeLabel,
			*shareSum, 0, *inputHash,
		)
		require.NoError(t, err)
		require.True(t, match)

		// A random unrelated output must not match.
		otherKey, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		match, err = OutputMatches(
			*otherKey.PubKey(), *spendKey.PubKey(), changeLabel,
			*shareSum, 0, *inputHash,
		)
		require.NoError(t, err)
		require.False(t, match)
	}

	t.Run("base payment", func(t *testing.T) {
		runMatch(t, "Simple send: two inputs")
	})
	t.Run("change label", func(t *testing.T) {
		runMatch(
			t, "Single recipient: use silent payments for "+
				"sender change",
		)
	})
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

// TestPublicKeyFromInputAnnex tests the BIP-0341 annex handling of taproot
// input parsing. The annex is any last witness element whose first byte is
// 0x50 — not only a single-byte 0x50 element, a divergence found in other
// BIP-0352 implementations. Mishandling the annex shifts which element is
// treated as the control block, bypassing the NUMS internal key skip.
func TestPublicKeyFromInputAnnex(t *testing.T) {
	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	outputKey := schnorr.SerializePubKey(privKey.PubKey())
	p2trScript := append(
		[]byte{txscript.OP_1, txscript.OP_DATA_32}, outputKey...,
	)
	prevOutFetcher := func(wire.OutPoint) ([]byte, error) {
		return p2trScript, nil
	}

	// A multi-byte annex: first byte 0x50, arbitrary payload.
	annex := []byte{0x50, 0xde, 0xad, 0xbe, 0xef}
	keyPathSig := make([]byte, 64)

	// Control blocks: leaf version/parity byte plus the 32-byte internal
	// key. The internal key is only compared against the NUMS point, so
	// the non-NUMS one doesn't have to be a valid key.
	numsControlBlock := append([]byte{0xc0}, BIP0341NUMSPoint...)
	otherInternalKey := make([]byte, 32)
	for i := range otherInternalKey {
		otherInternalKey[i] = 0x22
	}
	otherControlBlock := append([]byte{0xc0}, otherInternalKey...)
	leafScript := []byte{txscript.OP_TRUE}

	testCases := []struct {
		name      string
		witness   wire.TxWitness
		expectErr bool
	}{{
		// A key path spend with an annex still extracts the output
		// key from the scriptPubKey.
		name:    "key path with annex",
		witness: wire.TxWitness{keyPathSig, annex},
	}, {
		// A script path spend with a NUMS internal key must be
		// skipped, with or without an annex: if the annex were not
		// stripped, it would be mistaken for the control block and
		// the NUMS check would miss.
		name: "script path NUMS with annex",
		witness: wire.TxWitness{
			leafScript, numsControlBlock, annex,
		},
		expectErr: true,
	}, {
		name:      "script path NUMS without annex",
		witness:   wire.TxWitness{leafScript, numsControlBlock},
		expectErr: true,
	}, {
		// A script path spend with any other internal key uses the
		// output key from the scriptPubKey.
		name: "script path non-NUMS with annex",
		witness: wire.TxWitness{
			leafScript, otherControlBlock, annex,
		},
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(tt *testing.T) {
			pubKey, err := PublicKeyFromInput(&wire.TxIn{
				Witness: tc.witness,
			}, prevOutFetcher)

			if tc.expectErr {
				require.Error(tt, err)

				return
			}

			require.NoError(tt, err)
			require.Equal(
				tt, outputKey, schnorr.SerializePubKey(pubKey),
			)
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
