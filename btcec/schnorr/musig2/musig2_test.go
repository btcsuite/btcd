// Copyright 2013-2022 The btcsuite developers

package musig2

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"sync"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

var (
	key1Bytes, _ = hex.DecodeString("F9308A019258C31049344F85F89D5229B53" +
		"1C845836F99B08601F113BCE036F9")
	key2Bytes, _ = hex.DecodeString("DFF1D77F2A671C5F36183726DB2341BE58F" +
		"EAE1DA2DECED843240F7B502BA659")
	key3Bytes, _ = hex.DecodeString("3590A94E768F8E1815C2F24B4D80A8E3149" +
		"316C3518CE7B7AD338368D038CA66")

	testKeys = [][]byte{key1Bytes, key2Bytes, key3Bytes}

	keyCombo1, _ = hex.DecodeString("E5830140512195D74C8307E39637CBE5FB730EBEAB80EC514CF88A877CEEEE0B")
	keyCombo2, _ = hex.DecodeString("D70CD69A2647F7390973DF48CBFA2CCC407B8B2D60B08C5F1641185C7998A290")
	keyCombo3, _ = hex.DecodeString("81A8B093912C9E481408D09776CEFB48AEB8B65481B6BAAFB3C5810106717BEB")
	keyCombo4, _ = hex.DecodeString("2EB18851887E7BDC5E830E89B19DDBC28078F1FA88AAD0AD01CA06FE4F80210B")
)

const (
	keyAggTestVectorName = "key_agg_vectors.json"

	signTestVectorName = "sign_vectors.json"
)

var dumpJson = flag.Bool("dumpjson", false, "if true, a JSON version of the "+
	"test vectors will be written to the cwd")

type jsonKeyAggTestCase struct {
	Keys        []string `json:"keys"`
	ExpectedKey string   `json:"expected_key"`
}

// TestMuSig2KeyAggTestVectors tests that this implementation of musig2 key
// aggregation lines up with the secp256k1-zkp test vectors.
func TestMuSig2KeyAggTestVectors(t *testing.T) {
	t.Parallel()

	var jsonCases []jsonKeyAggTestCase

	testCases := []struct {
		keyOrder    []int
		expectedKey []byte
	}{
		// Keys in backwards lexicographical order.
		{
			keyOrder:    []int{0, 1, 2},
			expectedKey: keyCombo1,
		},

		// Keys in sorted order.
		{
			keyOrder:    []int{2, 1, 0},
			expectedKey: keyCombo2,
		},

		// Only the first key.
		{
			keyOrder:    []int{0, 0, 0},
			expectedKey: keyCombo3,
		},

		// Duplicate the first key and second keys.
		{
			keyOrder:    []int{0, 0, 1, 1},
			expectedKey: keyCombo4,
		},
	}
	for i, testCase := range testCases {
		testName := fmt.Sprintf("%v", testCase.keyOrder)
		t.Run(testName, func(t *testing.T) {
			var (
				keys    []*btcec.PublicKey
				strKeys []string
			)
			for _, keyIndex := range testCase.keyOrder {
				keyBytes := testKeys[keyIndex]
				pub, err := schnorr.ParsePubKey(keyBytes)
				if err != nil {
					t.Fatalf("unable to parse pubkeys: %v", err)
				}

				keys = append(keys, pub)
				strKeys = append(strKeys, hex.EncodeToString(keyBytes))
			}

			jsonCases = append(jsonCases, jsonKeyAggTestCase{
				Keys:        strKeys,
				ExpectedKey: hex.EncodeToString(testCase.expectedKey),
			})

			uniqueKeyIndex := secondUniqueKeyIndex(keys, false)
			combinedKey, _, _, _ := AggregateKeys(
				keys, false, WithUniqueKeyIndex(uniqueKeyIndex),
			)
			combinedKeyBytes := schnorr.SerializePubKey(combinedKey.FinalKey)
			if !bytes.Equal(combinedKeyBytes, testCase.expectedKey) {
				t.Fatalf("case: #%v, invalid aggregation: "+
					"expected %x, got %x", i, testCase.expectedKey,
					combinedKeyBytes)
			}
		})
	}

	if *dumpJson {
		jsonBytes, err := json.Marshal(jsonCases)
		if err != nil {
			t.Fatalf("unable to encode json: %v", err)
		}

		var formattedJson bytes.Buffer
		json.Indent(&formattedJson, jsonBytes, "", "\t")
		err = ioutil.WriteFile(
			keyAggTestVectorName, formattedJson.Bytes(), 0644,
		)
		if err != nil {
			t.Fatalf("unable to write file: %v", err)
		}
	}
}

func mustParseHex(str string) []byte {
	b, err := hex.DecodeString(str)
	if err != nil {
		panic(fmt.Errorf("unable to parse hex: %v", err))
	}

	return b
}

func parseKey(xHex string) *btcec.PublicKey {
	xB, err := hex.DecodeString(xHex)
	if err != nil {
		panic(err)
	}

	var x, y btcec.FieldVal
	x.SetByteSlice(xB)
	if !btcec.DecompressY(&x, false, &y) {
		panic("x not on curve")
	}
	y.Normalize()

	return btcec.NewPublicKey(&x, &y)
}

var (
	signSetPrivKey, _ = btcec.PrivKeyFromBytes(
		mustParseHex("7FB9E0E687ADA1EEBF7ECFE2F21E73EBDB51A7D450948DFE8D76D7F2D1007671"),
	)
	signSetPubKey, _ = schnorr.ParsePubKey(schnorr.SerializePubKey(signSetPrivKey.PubKey()))

	signTestMsg = mustParseHex("F95466D086770E689964664219266FE5ED215C92AE20BAB5C9D79ADDDDF3C0CF")

	signSetKey2, _ = schnorr.ParsePubKey(
		mustParseHex("F9308A019258C31049344F85F89D5229B531C845836F99B086" +
			"01F113BCE036F9"),
	)
	signSetKey3, _ = schnorr.ParsePubKey(
		mustParseHex("DFF1D77F2A671C5F36183726DB2341BE58FEAE1DA2DECED843" +
			"240F7B502BA659"),
	)

	signSetKeys = []*btcec.PublicKey{signSetPubKey, signSetKey2, signSetKey3}
)

func formatTweakParity(tweaks []KeyTweakDesc) string {
	var s string
	for _, tweak := range tweaks {
		s += fmt.Sprintf("%v/", tweak.IsXOnly)
	}

	// Snip off that last '/'.
	s = s[:len(s)-1]

	return s
}

func genTweakParity(tweak KeyTweakDesc, isXOnly bool) KeyTweakDesc {
	tweak.IsXOnly = isXOnly
	return tweak
}

type jsonTweak struct {
	Tweak string `json:"tweak"`
	XOnly bool   `json:"x_only"`
}

type tweakSignCase struct {
	Keys   []string    `json:"keys"`
	Tweaks []jsonTweak `json:"tweaks,omitempty"`

	ExpectedSig string `json:"expected_sig"`
}

type jsonSignTestCase struct {
	SecNonce   string `json:"secret_nonce"`
	AggNonce   string `json:"agg_nonce"`
	SigningKey string `json:"signing_key"`
	Msg        string `json:"msg"`

	TestCases []tweakSignCase `json:"test_cases"`
}

// TestMuSig2SigningTestVectors tests that the musig2 implementation produces
// the same set of signatures.
func TestMuSig2SigningTestVectors(t *testing.T) {
	t.Parallel()

	var jsonCases jsonSignTestCase

	jsonCases.SigningKey = hex.EncodeToString(signSetPrivKey.Serialize())
	jsonCases.Msg = hex.EncodeToString(signTestMsg)

	var aggregatedNonce [PubNonceSize]byte
	copy(
		aggregatedNonce[:],
		mustParseHex("028465FCF0BBDBCF443AABCCE533D42B4B5A10966AC09A49655E8C42DAAB8FCD61"),
	)
	copy(
		aggregatedNonce[33:],
		mustParseHex("037496A3CC86926D452CAFCFD55D25972CA1675D549310DE296BFF42F72EEEA8C9"),
	)

	jsonCases.AggNonce = hex.EncodeToString(aggregatedNonce[:])

	var secNonce [SecNonceSize]byte
	copy(secNonce[:], mustParseHex("508B81A611F100A6B2B6B29656590898AF488BCF2E1F55CF22E5CFB84421FE61"))
	copy(secNonce[32:], mustParseHex("FA27FD49B1D50085B481285E1CA205D55C82CC1B31FF5CD54A489829355901F7"))

	jsonCases.SecNonce = hex.EncodeToString(secNonce[:])

	tweak1 := KeyTweakDesc{
		Tweak: [32]byte{
			0xE8, 0xF7, 0x91, 0xFF, 0x92, 0x25, 0xA2, 0xAF,
			0x01, 0x02, 0xAF, 0xFF, 0x4A, 0x9A, 0x72, 0x3D,
			0x96, 0x12, 0xA6, 0x82, 0xA2, 0x5E, 0xBE, 0x79,
			0x80, 0x2B, 0x26, 0x3C, 0xDF, 0xCD, 0x83, 0xBB,
		},
	}
	tweak2 := KeyTweakDesc{
		Tweak: [32]byte{
			0xae, 0x2e, 0xa7, 0x97, 0xcc, 0xf, 0xe7, 0x2a,
			0xc5, 0xb9, 0x7b, 0x97, 0xf3, 0xc6, 0x95, 0x7d,
			0x7e, 0x41, 0x99, 0xa1, 0x67, 0xa5, 0x8e, 0xb0,
			0x8b, 0xca, 0xff, 0xda, 0x70, 0xac, 0x4, 0x55,
		},
	}
	tweak3 := KeyTweakDesc{
		Tweak: [32]byte{
			0xf5, 0x2e, 0xcb, 0xc5, 0x65, 0xb3, 0xd8, 0xbe,
			0xa2, 0xdf, 0xd5, 0xb7, 0x5a, 0x4f, 0x45, 0x7e,
			0x54, 0x36, 0x98, 0x9, 0x32, 0x2e, 0x41, 0x20,
			0x83, 0x16, 0x26, 0xf2, 0x90, 0xfa, 0x87, 0xe0,
		},
	}
	tweak4 := KeyTweakDesc{
		Tweak: [32]byte{
			0x19, 0x69, 0xad, 0x73, 0xcc, 0x17, 0x7f, 0xa0,
			0xb4, 0xfc, 0xed, 0x6d, 0xf1, 0xf7, 0xbf, 0x99,
			0x7, 0xe6, 0x65, 0xfd, 0xe9, 0xba, 0x19, 0x6a,
			0x74, 0xfe, 0xd0, 0xa3, 0xcf, 0x5a, 0xef, 0x9d,
		},
	}

	testCases := []struct {
		keyOrder           []int
		expectedPartialSig []byte
		tweaks             []KeyTweakDesc
	}{
		{
			keyOrder:           []int{0, 1, 2},
			expectedPartialSig: mustParseHex("68537CC5234E505BD14061F8DA9E90C220A181855FD8BDB7F127BB12403B4D3B"),
		},
		{
			keyOrder:           []int{1, 0, 2},
			expectedPartialSig: mustParseHex("2DF67BFFF18E3DE797E13C6475C963048138DAEC5CB20A357CECA7C8424295EA"),
		},
		{
			keyOrder:           []int{1, 2, 0},
			expectedPartialSig: mustParseHex("0D5B651E6DE34A29A12DE7A8B4183B4AE6A7F7FBE15CDCAFA4A3D1BCAABC7517"),
		},

		// A single x-only tweak.
		{
			keyOrder:           []int{1, 2, 0},
			expectedPartialSig: mustParseHex("5e24c7496b565debc3b9639e6f1304a21597f9603d3ab05b4913641775e1375b"),
			tweaks:             []KeyTweakDesc{genTweakParity(tweak1, true)},
		},

		// A single ordinary tweak.
		{
			keyOrder:           []int{1, 2, 0},
			expectedPartialSig: mustParseHex("78408ddcab4813d1394c97d493ef1084195c1d4b52e63ecd7bc5991644e44ddd"),
			tweaks:             []KeyTweakDesc{genTweakParity(tweak1, false)},
		},

		// An ordinary tweak then an x-only tweak.
		{
			keyOrder:           []int{1, 2, 0},
			expectedPartialSig: mustParseHex("C3A829A81480E36EC3AB052964509A94EBF34210403D16B226A6F16EC85B7357"),
			tweaks: []KeyTweakDesc{
				genTweakParity(tweak1, false),
				genTweakParity(tweak2, true),
			},
		},

		// Four tweaks, in the order: x-only, ordinary, x-only, ordinary.
		{
			keyOrder:           []int{1, 2, 0},
			expectedPartialSig: mustParseHex("8C4473C6A382BD3C4AD7BE59818DA5ED7CF8CEC4BC21996CFDA08BB4316B8BC7"),
			tweaks: []KeyTweakDesc{
				genTweakParity(tweak1, true),
				genTweakParity(tweak2, false),
				genTweakParity(tweak3, true),
				genTweakParity(tweak4, false),
			},
		},
	}

	var msg [32]byte
	copy(msg[:], signTestMsg)

	for _, testCase := range testCases {
		testName := fmt.Sprintf("%v/tweak=%v", testCase.keyOrder, len(testCase.tweaks) != 0)
		if len(testCase.tweaks) != 0 {
			testName += fmt.Sprintf("/x_only=%v", formatTweakParity(testCase.tweaks))
		}

		t.Run(testName, func(t *testing.T) {
			var strKeys []string
			keySet := make([]*btcec.PublicKey, 0, len(testCase.keyOrder))
			for _, keyIndex := range testCase.keyOrder {
				keySet = append(keySet, signSetKeys[keyIndex])
				strKeys = append(
					strKeys, hex.EncodeToString(
						schnorr.SerializePubKey(signSetKeys[keyIndex]),
					),
				)
			}

			var opts []SignOption
			if len(testCase.tweaks) != 0 {
				opts = append(
					opts, WithTweaks(testCase.tweaks...),
				)
			}

			var jsonTweaks []jsonTweak
			for _, tweak := range testCase.tweaks {
				jsonTweaks = append(jsonTweaks, jsonTweak{
					Tweak: hex.EncodeToString(tweak.Tweak[:]),
					XOnly: tweak.IsXOnly,
				})
			}

			partialSig, err := Sign(
				secNonce, signSetPrivKey, aggregatedNonce,
				keySet, msg, opts...,
			)
			if err != nil {
				t.Fatalf("unable to generate partial sig: %v", err)
			}

			var partialSigBytes [32]byte
			partialSig.S.PutBytesUnchecked(partialSigBytes[:])

			if !bytes.Equal(partialSigBytes[:], testCase.expectedPartialSig) {
				t.Fatalf("sigs don't match: expected %x, got %x",
					testCase.expectedPartialSig, partialSigBytes,
				)
			}

			jsonCases.TestCases = append(jsonCases.TestCases, tweakSignCase{
				Keys:        strKeys,
				Tweaks:      jsonTweaks,
				ExpectedSig: hex.EncodeToString(testCase.expectedPartialSig),
			})
		})
	}

	if *dumpJson {
		jsonBytes, err := json.Marshal(jsonCases)
		if err != nil {
			t.Fatalf("unable to encode json: %v", err)
		}

		var formattedJson bytes.Buffer
		json.Indent(&formattedJson, jsonBytes, "", "\t")
		err = ioutil.WriteFile(
			signTestVectorName, formattedJson.Bytes(), 0644,
		)
		if err != nil {
			t.Fatalf("unable to write file: %v", err)
		}
	}
}

type signer struct {
	privKey *btcec.PrivateKey
	pubKey  *btcec.PublicKey

	nonces *Nonces

	partialSig *PartialSignature
}

type signerSet []signer

func (s signerSet) keys() []*btcec.PublicKey {
	keys := make([]*btcec.PublicKey, len(s))
	for i := 0; i < len(s); i++ {
		keys[i] = s[i].pubKey
	}

	return keys
}

func (s signerSet) partialSigs() []*PartialSignature {
	sigs := make([]*PartialSignature, len(s))
	for i := 0; i < len(s); i++ {
		sigs[i] = s[i].partialSig
	}

	return sigs
}

func (s signerSet) pubNonces() [][PubNonceSize]byte {
	nonces := make([][PubNonceSize]byte, len(s))
	for i := 0; i < len(s); i++ {
		nonces[i] = s[i].nonces.PubNonce
	}

	return nonces
}

func (s signerSet) combinedKey() *btcec.PublicKey {
	uniqueKeyIndex := secondUniqueKeyIndex(s.keys(), false)
	key, _, _, _ := AggregateKeys(
		s.keys(), false, WithUniqueKeyIndex(uniqueKeyIndex),
	)
	return key.FinalKey
}

// testMultiPartySign executes a multi-party signing context w/ 100 signers.
func testMultiPartySign(t *testing.T, taprootTweak []byte,
	tweaks ...KeyTweakDesc) {

	const numSigners = 100

	// First generate the set of signers along with their public keys.
	signerKeys := make([]*btcec.PrivateKey, numSigners)
	signSet := make([]*btcec.PublicKey, numSigners)
	for i := 0; i < numSigners; i++ {
		privKey, err := btcec.NewPrivateKey()
		if err != nil {
			t.Fatalf("unable to gen priv key: %v", err)
		}

		pubKey, err := schnorr.ParsePubKey(
			schnorr.SerializePubKey(privKey.PubKey()),
		)
		if err != nil {
			t.Fatalf("unable to gen key: %v", err)
		}

		signerKeys[i] = privKey
		signSet[i] = pubKey
	}

	var combinedKey *btcec.PublicKey

	var ctxOpts []ContextOption
	switch {
	case len(taprootTweak) == 0:
		ctxOpts = append(ctxOpts, WithBip86TweakCtx())
	case taprootTweak != nil:
		ctxOpts = append(ctxOpts, WithTaprootTweakCtx(taprootTweak))
	case len(tweaks) != 0:
		ctxOpts = append(ctxOpts, WithTweakedContext(tweaks...))
	}

	ctxOpts = append(ctxOpts, WithKnownSigners(signSet))

	// Now that we have all the signers, we'll make a new context, then
	// generate a new session for each of them(which handles nonce
	// generation).
	signers := make([]*Session, numSigners)
	for i, signerKey := range signerKeys {
		signCtx, err := NewContext(
			signerKey, false, ctxOpts...,
		)
		if err != nil {
			t.Fatalf("unable to generate context: %v", err)
		}

		if combinedKey == nil {
			combinedKey, err = signCtx.CombinedKey()
			if err != nil {
				t.Fatalf("combined key not available: %v", err)
			}
		}

		session, err := signCtx.NewSession()
		if err != nil {
			t.Fatalf("unable to generate new session: %v", err)
		}
		signers[i] = session
	}

	// Next, in the pre-signing phase, we'll send all the nonces to each
	// signer.
	var wg sync.WaitGroup
	for i, signCtx := range signers {
		signCtx := signCtx

		wg.Add(1)
		go func(idx int, signer *Session) {
			defer wg.Done()

			for j, otherCtx := range signers {
				if idx == j {
					continue
				}

				nonce := otherCtx.PublicNonce()
				haveAll, err := signer.RegisterPubNonce(nonce)
				if err != nil {
					t.Fatalf("unable to add public nonce")
				}

				if j == len(signers)-1 && !haveAll {
					t.Fatalf("all public nonces should have been detected")
				}
			}
		}(i, signCtx)
	}

	wg.Wait()

	msg := sha256.Sum256([]byte("let's get taprooty"))

	// In the final step, we'll use the first signer as our combiner, and
	// generate a signature for each signer, and then accumulate that with
	// the combiner.
	combiner := signers[0]
	for i := range signers {
		signer := signers[i]
		partialSig, err := signer.Sign(msg)
		if err != nil {
			t.Fatalf("unable to generate partial sig: %v", err)
		}

		// We don't need to combine the signature for the very first
		// signer, as it already has that partial signature.
		if i != 0 {
			haveAll, err := combiner.CombineSig(partialSig)
			if err != nil {
				t.Fatalf("unable to combine sigs: %v", err)
			}

			if i == len(signers)-1 && !haveAll {
				t.Fatalf("final sig wasn't reconstructed")
			}
		}
	}

	// Finally we'll combined all the nonces, and ensure that it validates
	// as a single schnorr signature.
	finalSig := combiner.FinalSig()
	if !finalSig.Verify(msg[:], combinedKey) {
		t.Fatalf("final sig is invalid!")
	}

	// Verify that if we try to sign again with any of the existing
	// signers, then we'll get an error as the nonces have already been
	// used.
	for _, signer := range signers {
		_, err := signer.Sign(msg)
		if err != ErrSigningContextReuse {
			t.Fatalf("expected to get signing context reuse")
		}
	}
}

// TestMuSigMultiParty tests that for a given set of 100 signers, we're able to
// properly generate valid sub signatures, which ultimately can be combined
// into a single valid signature.
func TestMuSigMultiParty(t *testing.T) {
	t.Parallel()

	testTweak := [32]byte{
		0xE8, 0xF7, 0x91, 0xFF, 0x92, 0x25, 0xA2, 0xAF,
		0x01, 0x02, 0xAF, 0xFF, 0x4A, 0x9A, 0x72, 0x3D,
		0x96, 0x12, 0xA6, 0x82, 0xA2, 0x5E, 0xBE, 0x79,
		0x80, 0x2B, 0x26, 0x3C, 0xDF, 0xCD, 0x83, 0xBB,
	}

	t.Run("no_tweak", func(t *testing.T) {
		t.Parallel()

		testMultiPartySign(t, nil)
	})

	t.Run("tweaked", func(t *testing.T) {
		t.Parallel()

		testMultiPartySign(t, nil, KeyTweakDesc{
			Tweak: testTweak,
		})
	})

	t.Run("tweaked_x_only", func(t *testing.T) {
		t.Parallel()

		testMultiPartySign(t, nil, KeyTweakDesc{
			Tweak:   testTweak,
			IsXOnly: true,
		})
	})

	t.Run("taproot_tweaked_x_only", func(t *testing.T) {
		t.Parallel()

		testMultiPartySign(t, testTweak[:])
	})

	t.Run("taproot_bip_86", func(t *testing.T) {
		t.Parallel()

		testMultiPartySign(t, []byte{})
	})
}

// TestMuSigEarlyNonce tests that for protocols where nonces need to be
// exchagned before all signers are known, the context API works as expected.
func TestMuSigEarlyNonce(t *testing.T) {
	t.Parallel()

	privKey1, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("unable to gen priv key: %v", err)
	}
	privKey2, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("unable to gen priv key: %v", err)
	}

	// If we try to make a context, with just the private key and sorting
	// value, we should get an error.
	_, err = NewContext(privKey1, true)
	if !errors.Is(err, ErrSignersNotSpecified) {
		t.Fatalf("unexpected ctx error: %v", err)
	}

	numSigners := 2

	ctx1, err := NewContext(
		privKey1, true, WithNumSigners(numSigners), WithEarlyNonceGen(),
	)
	if err != nil {
		t.Fatalf("unable to make ctx: %v", err)
	}
	pubKey1 := ctx1.PubKey()

	ctx2, err := NewContext(
		privKey2, true, WithNumSigners(numSigners), WithEarlyNonceGen(),
	)
	if err != nil {
		t.Fatalf("unable to make ctx: %v", err)
	}
	pubKey2 := ctx2.PubKey()

	// At this point, the combined key shouldn't be available for both
	// signers, since we only know of the sole signers.
	if _, err := ctx1.CombinedKey(); !errors.Is(err, ErrNotEnoughSigners) {
		t.Fatalf("unepxected error: %v", err)
	}
	if _, err := ctx2.CombinedKey(); !errors.Is(err, ErrNotEnoughSigners) {
		t.Fatalf("unepxected error: %v", err)
	}

	// The early nonces _should_ be available at this point.
	nonce1, err := ctx1.EarlySessionNonce()
	if err != nil {
		t.Fatalf("session nonce not available: %v", err)
	}
	nonce2, err := ctx2.EarlySessionNonce()
	if err != nil {
		t.Fatalf("session nonce not available: %v", err)
	}

	// The number of registered signers should still be 1 for both parties.
	if ctx1.NumRegisteredSigners() != 1 {
		t.Fatalf("expected 1 signer, instead have: %v",
			ctx1.NumRegisteredSigners())
	}
	if ctx2.NumRegisteredSigners() != 1 {
		t.Fatalf("expected 1 signer, instead have: %v",
			ctx2.NumRegisteredSigners())
	}

	// If we try to make a session, we should get an error since we dn't
	// have all the signers yet.
	if _, err := ctx1.NewSession(); !errors.Is(err, ErrNotEnoughSigners) {
		t.Fatalf("unexpected session key error: %v", err)
	}

	// The combined key should also be unavailable as well.
	if _, err := ctx1.CombinedKey(); !errors.Is(err, ErrNotEnoughSigners) {
		t.Fatalf("unexpected combined key error: %v", err)
	}

	// We'll now register the other signer for both parties.
	done, err := ctx1.RegisterSigner(&pubKey2)
	if err != nil {
		t.Fatalf("unable to register signer: %v", err)
	}
	if !done {
		t.Fatalf("signer 1 doesn't have all keys")
	}
	done, err = ctx2.RegisterSigner(&pubKey1)
	if err != nil {
		t.Fatalf("unable to register signer: %v", err)
	}
	if !done {
		t.Fatalf("signer 2 doesn't have all keys")
	}

	// If we try to register the signer again, we should get an error.
	_, err = ctx2.RegisterSigner(&pubKey1)
	if !errors.Is(err, ErrAlreadyHaveAllSigners) {
		t.Fatalf("should not be able to register too many signers")
	}

	// We should be able to create the session at this point.
	session1, err := ctx1.NewSession()
	if err != nil {
		t.Fatalf("unable to create new session: %v", err)
	}
	session2, err := ctx2.NewSession()
	if err != nil {
		t.Fatalf("unable to create new session: %v", err)
	}

	msg := sha256.Sum256([]byte("let's get taprooty, LN style"))

	// If we try to sign before we have the combined nonce, we shoudl get
	// an error.
	_, err = session1.Sign(msg)
	if !errors.Is(err, ErrCombinedNonceUnavailable) {
		t.Fatalf("unable to gen sig: %v", err)
	}

	// Now we can exchange nonces to continue with the rest of the signing
	// process as normal.
	done, err = session1.RegisterPubNonce(nonce2.PubNonce)
	if err != nil {
		t.Fatalf("unable to register nonce: %v", err)
	}
	if !done {
		t.Fatalf("signer 1 doesn't have all nonces")
	}
	done, err = session2.RegisterPubNonce(nonce1.PubNonce)
	if err != nil {
		t.Fatalf("unable to register nonce: %v", err)
	}
	if !done {
		t.Fatalf("signer 2 doesn't have all nonces")
	}

	// Registering the nonce again should error out.
	_, err = session2.RegisterPubNonce(nonce1.PubNonce)
	if !errors.Is(err, ErrAlredyHaveAllNonces) {
		t.Fatalf("shouldn't be able to register nonces twice")
	}

	// Sign the message and combine the two partial sigs into one.
	_, err = session1.Sign(msg)
	if err != nil {
		t.Fatalf("unable to gen sig: %v", err)
	}
	sig2, err := session2.Sign(msg)
	if err != nil {
		t.Fatalf("unable to gen sig: %v", err)
	}
	done, err = session1.CombineSig(sig2)
	if err != nil {
		t.Fatalf("unable to combine sig: %v", err)
	}
	if !done {
		t.Fatalf("all sigs should be known now: %v", err)
	}

	// If we try to combine another sig, then we should get an error.
	_, err = session1.CombineSig(sig2)
	if !errors.Is(err, ErrAlredyHaveAllSigs) {
		t.Fatalf("shouldn't be able to combine again")
	}

	// Finally, verify that the final signature is valid.
	combinedKey, err := ctx1.CombinedKey()
	if err != nil {
		t.Fatalf("unexpected combined key error: %v", err)
	}
	finalSig := session1.FinalSig()
	if !finalSig.Verify(msg[:], combinedKey) {
		t.Fatalf("final sig is invalid!")
	}
}
