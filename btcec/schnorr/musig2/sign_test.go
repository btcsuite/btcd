// Copyright 2013-2022 The btcsuite developers

package musig2

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/require"
)

const (
	signVerifyTestVectorFileName = "sign_verify_vectors.json"

	sigCombineTestVectorFileName = "sig_agg_vectors.json"
)

type signVerifyValidCase struct {
	Indices []int `json:"key_indices"`

	NonceIndices []int `json:"nonce_indices"`

	AggNonceIndex int `json:"aggnonce_index"`

	MsgIndex int `json:"msg_index"`

	SignerIndex int `json:"signer_index"`

	Expected string `json:"expected"`
}

type signErrorCase struct {
	Indices []int `json:"key_indices"`

	AggNonceIndex int `json:"aggnonce_index"`

	MsgIndex int `json:"msg_index"`

	SecNonceIndex int `json:"secnonce_index"`

	Comment string `json:"comment"`
}

type verifyFailCase struct {
	Sig string `json:"sig"`

	Indices []int `json:"key_indices"`

	NonceIndices []int `json:"nonce_indices"`

	MsgIndex int `json:"msg_index"`

	SignerIndex int `json:"signer_index"`

	Comment string `json:"comment"`
}

type verifyErrorCase struct {
	Sig string `json:"sig"`

	Indices []int `json:"key_indices"`

	NonceIndices []int `json:"nonce_indices"`

	MsgIndex int `json:"msg_index"`

	SignerIndex int `json:"signer_index"`

	Comment string `json:"comment"`
}

type signVerifyTestVectors struct {
	PrivKey string `json:"sk"`

	PubKeys []string `json:"pubkeys"`

	PrivNonces []string `json:"secnonces"`

	PubNonces []string `json:"pnonces"`

	AggNonces []string `json:"aggnonces"`

	Msgs []string `json:"msgs"`

	ValidCases []signVerifyValidCase `json:"valid_test_cases"`

	SignErrorCases []signErrorCase `json:"sign_error_test_cases"`

	VerifyFailCases []verifyFailCase `json:"verify_fail_test_cases"`

	VerifyErrorCases []verifyErrorCase `json:"verify_error_test_cases"`
}

// TestMusig2SignVerify tests that we pass the musig2 verification tests.
func TestMusig2SignVerify(t *testing.T) {
	t.Parallel()

	testVectorPath := path.Join(
		testVectorBaseDir, signVerifyTestVectorFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(t, err)

	var testCases signVerifyTestVectors
	require.NoError(t, json.Unmarshal(testVectorBytes, &testCases))

	privKey, _ := btcec.PrivKeyFromBytes(mustParseHex(testCases.PrivKey))

	for i, testCase := range testCases.ValidCases {
		testCase := testCase

		testName := fmt.Sprintf("valid_case_%v", i)
		t.Run(testName, func(t *testing.T) {
			pubKeys, err := keysFromIndices(
				t, testCase.Indices, testCases.PubKeys,
			)
			require.NoError(t, err)

			pubNonces := pubNoncesFromIndices(
				t, testCase.NonceIndices, testCases.PubNonces,
			)

			combinedNonce, err := AggregateNonces(pubNonces)
			require.NoError(t, err)

			var msg [32]byte
			copy(msg[:], mustParseHex(testCases.Msgs[testCase.MsgIndex]))

			var secNonce [SecNonceSize]byte
			copy(secNonce[:], mustParseHex(testCases.PrivNonces[0]))

			partialSig, err := Sign(
				secNonce, privKey, combinedNonce, pubKeys,
				msg,
			)

			var partialSigBytes [32]byte
			partialSig.S.PutBytesUnchecked(partialSigBytes[:])

			require.Equal(
				t, hex.EncodeToString(partialSigBytes[:]),
				hex.EncodeToString(mustParseHex(testCase.Expected)),
			)
		})
	}

	for _, testCase := range testCases.SignErrorCases {
		testCase := testCase

		testName := fmt.Sprintf("invalid_case_%v",
			strings.ToLower(testCase.Comment))

		t.Run(testName, func(t *testing.T) {
			pubKeys, err := keysFromIndices(
				t, testCase.Indices, testCases.PubKeys,
			)
			if err != nil {
				require.ErrorIs(t, err, secp.ErrPubKeyNotOnCurve)
				return
			}

			var aggNonce [PubNonceSize]byte
			copy(
				aggNonce[:],
				mustParseHex(
					testCases.AggNonces[testCase.AggNonceIndex],
				),
			)

			var msg [32]byte
			copy(msg[:], mustParseHex(testCases.Msgs[testCase.MsgIndex]))

			var secNonce [SecNonceSize]byte
			copy(
				secNonce[:],
				mustParseHex(
					testCases.PrivNonces[testCase.SecNonceIndex],
				),
			)

			_, err = Sign(
				secNonce, privKey, aggNonce, pubKeys,
				msg,
			)
			require.Error(t, err)
		})
	}

	for _, testCase := range testCases.VerifyFailCases {
		testCase := testCase

		testName := fmt.Sprintf("verify_fail_%v",
			strings.ToLower(testCase.Comment))
		t.Run(testName, func(t *testing.T) {
			pubKeys, err := keysFromIndices(
				t, testCase.Indices, testCases.PubKeys,
			)
			require.NoError(t, err)

			pubNonces := pubNoncesFromIndices(
				t, testCase.NonceIndices, testCases.PubNonces,
			)

			combinedNonce, err := AggregateNonces(pubNonces)
			require.NoError(t, err)

			var msg [32]byte
			copy(
				msg[:],
				mustParseHex(testCases.Msgs[testCase.MsgIndex]),
			)

			var secNonce [SecNonceSize]byte
			copy(secNonce[:], mustParseHex(testCases.PrivNonces[0]))

			signerNonce := secNonceToPubNonce(secNonce)

			var partialSig PartialSignature
			err = partialSig.Decode(
				bytes.NewReader(mustParseHex(testCase.Sig)),
			)
			if err != nil && strings.Contains(testCase.Comment, "group size") {
				require.ErrorIs(t, err, ErrPartialSigInvalid)
			}

			err = verifyPartialSig(
				&partialSig, signerNonce, combinedNonce,
				pubKeys, privKey.PubKey().SerializeCompressed(),
				msg,
			)
			require.Error(t, err)
		})
	}

	for _, testCase := range testCases.VerifyErrorCases {
		testCase := testCase

		testName := fmt.Sprintf("verify_error_%v",
			strings.ToLower(testCase.Comment))
		t.Run(testName, func(t *testing.T) {
			switch testCase.Comment {
			case "Invalid pubnonce":
				pubNonces := pubNoncesFromIndices(
					t, testCase.NonceIndices, testCases.PubNonces,
				)
				_, err := AggregateNonces(pubNonces)
				require.ErrorIs(t, err, secp.ErrPubKeyNotOnCurve)

			case "Invalid pubkey":
				_, err := keysFromIndices(
					t, testCase.Indices, testCases.PubKeys,
				)
				require.ErrorIs(t, err, secp.ErrPubKeyNotOnCurve)

			default:
				t.Fatalf("unhandled case: %v", testCase.Comment)
			}
		})
	}

}

type sigCombineValidCase struct {
	AggNonce string `json:"aggnonce"`

	NonceIndices []int `json:"nonce_indices"`

	Indices []int `json:"key_indices"`

	TweakIndices []int `json:"tweak_indices"`

	IsXOnly []bool `json:"is_xonly"`

	PSigIndices []int `json:"psig_indices"`

	Expected string `json:"expected"`
}

type sigCombineTestVectors struct {
	PubKeys []string `json:"pubkeys"`

	PubNonces []string `json:"pnonces"`

	Tweaks []string `json:"tweaks"`

	Psigs []string `json:"psigs"`

	Msg string `json:"msg"`

	ValidCases []sigCombineValidCase `json:"valid_test_cases"`
}

func pSigsFromIndices(t *testing.T, sigs []string, indices []int) []*PartialSignature {
	pSigs := make([]*PartialSignature, len(indices))
	for i, idx := range indices {
		var pSig PartialSignature
		err := pSig.Decode(bytes.NewReader(mustParseHex(sigs[idx])))
		require.NoError(t, err)

		pSigs[i] = &pSig
	}

	return pSigs
}

// TestMusig2SignCombine tests that we pass the musig2 sig combination tests.
func TestMusig2SignCombine(t *testing.T) {
	t.Parallel()

	testVectorPath := path.Join(
		testVectorBaseDir, sigCombineTestVectorFileName,
	)
	testVectorBytes, err := os.ReadFile(testVectorPath)
	require.NoError(t, err)

	var testCases sigCombineTestVectors
	require.NoError(t, json.Unmarshal(testVectorBytes, &testCases))

	var msg [32]byte
	copy(msg[:], mustParseHex(testCases.Msg))

	for i, testCase := range testCases.ValidCases {
		testCase := testCase

		testName := fmt.Sprintf("valid_case_%v", i)
		t.Run(testName, func(t *testing.T) {
			pubKeys, err := keysFromIndices(
				t, testCase.Indices, testCases.PubKeys,
			)
			require.NoError(t, err)

			pubNonces := pubNoncesFromIndices(
				t, testCase.NonceIndices, testCases.PubNonces,
			)

			partialSigs := pSigsFromIndices(
				t, testCases.Psigs, testCase.PSigIndices,
			)

			var (
				combineOpts []CombineOption
				keyOpts     []KeyAggOption
			)
			if len(testCase.TweakIndices) > 0 {
				tweaks := tweaksFromIndices(
					t, testCase.TweakIndices,
					testCases.Tweaks, testCase.IsXOnly,
				)

				combineOpts = append(combineOpts, WithTweakedCombine(
					msg, pubKeys, tweaks, false,
				))

				keyOpts = append(keyOpts, WithKeyTweaks(tweaks...))
			}

			combinedKey, _, _, err := AggregateKeys(
				pubKeys, false, keyOpts...,
			)
			require.NoError(t, err)

			combinedNonce, err := AggregateNonces(pubNonces)
			require.NoError(t, err)

			finalNonceJ, _, err := computeSigningNonce(
				combinedNonce, combinedKey.FinalKey, msg,
			)

			finalNonceJ.ToAffine()
			finalNonce := btcec.NewPublicKey(
				&finalNonceJ.X, &finalNonceJ.Y,
			)

			combinedSig := CombineSigs(
				finalNonce, partialSigs, combineOpts...,
			)
			require.Equal(t,
				strings.ToLower(testCase.Expected),
				hex.EncodeToString(combinedSig.Serialize()),
			)
		})
	}
}

func TestMusig2SignCombineAdaptor(t *testing.T) {
	// Pre-generate 10 deterministic private keys
	privKeys := make([]*btcec.PrivateKey, 10)
	pubKeys := make([]*btcec.PublicKey, 10)
	for i := 0; i < 10; i++ {
		seed := sha256.Sum256([]byte(fmt.Sprintf("privkey_%d", i)))
		privKey, _ := btcec.PrivKeyFromBytes(seed[:])
		privKeys[i] = privKey
		pubKeys[i] = privKey.PubKey()
	}

	// Helper function to run test with different number of signers and context options
	runTest := func(t *testing.T, numSigners int, ctxOpt ContextOption) {
		contexts := make([]*Context, numSigners)
		sessions := make([]*Session, numSigners)

		// Create context for each signer
		for i := 0; i < numSigners; i++ {
			ctx, err := NewContext(
				privKeys[i], true,
				WithNumSigners(numSigners),
				ctxOpt,
			)
			require.NoError(t, err)
			contexts[i] = ctx

			// Register all other public keys
			for j := 0; j < numSigners; j++ {
				if j != i {
					_, err = contexts[i].RegisterSigner(pubKeys[j])
					require.NoError(t, err)
				}
			}

			// Create session
			sess, err := contexts[i].NewSession()
			require.NoError(t, err)
			sessions[i] = sess
		}

		// Exchange public nonces
		for i := 0; i < numSigners; i++ {
			for j := 0; j < numSigners; j++ {
				if j != i {
					_, err := sessions[i].RegisterPubNonce(sessions[j].PublicNonce())
					require.NoError(t, err)
				}
			}
		}

		// Message to sign
		msg := [32]byte{1, 2, 3, 4}

		// Signer 0 generates and uses the adaptor secret
		adaptorSecret, adaptorPoint, err := sessions[0].GenerateAdaptor(msg)
		require.NoError(t, err)

		// No-op, but test SetAdaptorSecret
		err = sessions[0].SetAdaptorSecret(msg, adaptorSecret)
		require.NoError(t, err)

		// All other signers set the adaptor point
		for i := 1; i < numSigners; i++ {
			err = sessions[i].SetAdaptorPoint(msg, adaptorPoint)
			require.NoError(t, err)
		}

		// Each signer generates a partial signature
		partialSigs := make([]*PartialSignature, numSigners)
		for i := 0; i < numSigners; i++ {
			sig, err := sessions[i].Sign(msg)
			require.NoError(t, err)
			partialSigs[i] = sig
		}

		// Each signer combines all partial signatures
		for i := 0; i < numSigners; i++ {
			for j := 0; j < numSigners; j++ {
				if j != i {
					_, err := sessions[i].CombineSig(partialSigs[j])
					require.NoError(t, err)
				}
			}
		}

		// Signer 0 adapts the final signature
		validSig, err := sessions[0].AdaptFinalSig(adaptorSecret)
		require.NoError(t, err)

		// Verify the final signature is valid under the combined key
		combinedKey, _ := contexts[0].CombinedKey()
		require.True(t, validSig.Verify(msg[:], combinedKey),
			"Final signature verification failed")

		// All other signers recover the adaptor secret
		for i := 1; i < numSigners; i++ {
			recoveredSecret, err := sessions[i].RecoverAdaptorSecret(validSig)
			require.NoError(t, err)
			require.Equal(t, adaptorSecret.Bytes(), recoveredSecret.Bytes(),
				"Recovered secret mismatch for signer %d", i)
		}
	}

	// Run test for 2-10 signers with different context options
	for numSigners := 2; numSigners <= 10; numSigners++ {
		t.Run(fmt.Sprintf("BIP86_%d_signers", numSigners), func(t *testing.T) {
			runTest(t, numSigners, WithBip86TweakCtx())
		})

		scriptRoot := sha256.Sum256([]byte("test_script_root"))
		t.Run(fmt.Sprintf("Taproot_%d_signers", numSigners), func(t *testing.T) {
			runTest(t, numSigners, WithTaprootTweakCtx(scriptRoot[:]))
		})
	}
}
