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
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

var (
	key1Bytes, _ = hex.DecodeString("F9308A019258C31049344F85F89D5229B53" +
		"1C845836F99B08601F113BCE036F9")
	key2Bytes, _ = hex.DecodeString("DFF1D77F2A671C5F36183726DB2341BE58F" +
		"EAE1DA2DECED843240F7B502BA659")
	key3Bytes, _ = hex.DecodeString("3590A94E768F8E1815C2F24B4D80A8E3149" +
		"316C3518CE7B7AD338368D038CA66")

	invalidPk1, _ = hex.DecodeString("00000000000000000000000000000000" +
		"00000000000000000000000000000005")
	invalidPk2, _ = hex.DecodeString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF" +
		"FFFFFFFFFFFFFFFFFFFFFFFEFFFFFC30")
	invalidTweak, _ = hex.DecodeString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFE" +
		"BAAEDCE6AF48A03BBFD25E8CD0364141")

	testKeys = [][]byte{key1Bytes, key2Bytes, key3Bytes, invalidPk1,
		invalidPk2}

	keyCombo1, _ = hex.DecodeString("E5830140512195D74C8307E39637CBE5FB73" +
		"0EBEAB80EC514CF88A877CEEEE0B")
	keyCombo2, _ = hex.DecodeString("D70CD69A2647F7390973DF48CBFA2CCC407B" +
		"8B2D60B08C5F1641185C7998A290")
	keyCombo3, _ = hex.DecodeString("81A8B093912C9E481408D09776CEFB48AEB8" +
		"B65481B6BAAFB3C5810106717BEB")
	keyCombo4, _ = hex.DecodeString("2EB18851887E7BDC5E830E89B19DDBC28078" +
		"F1FA88AAD0AD01CA06FE4F80210B")
)

// getInfinityTweak returns a tweak that, when tweaking the Generator, triggers
// the ErrTweakedKeyIsInfinity error.
func getInfinityTweak() KeyTweakDesc {
	generator := btcec.Generator()

	keySet := []*btcec.PublicKey{generator}

	keysHash := keyHashFingerprint(keySet, true)
	uniqueKeyIndex := secondUniqueKeyIndex(keySet, true)

	n := &btcec.ModNScalar{}

	n.SetByteSlice(invalidTweak)

	coeff := aggregationCoefficient(
		keySet, generator, keysHash, uniqueKeyIndex,
	).Negate().Add(n)

	return KeyTweakDesc{
		Tweak:   coeff.Bytes(),
		IsXOnly: false,
	}
}

const (
	keyAggTestVectorName = "key_agg_vectors.json"

	nonceAggTestVectorName = "nonce_agg_vectors.json"

	signTestVectorName = "sign_vectors.json"
)

var dumpJson = flag.Bool("dumpjson", false, "if true, a JSON version of the "+
	"test vectors will be written to the cwd")

type jsonKeyAggTestCase struct {
	Keys          []string    `json:"keys"`
	Tweaks        []jsonTweak `json:"tweaks"`
	ExpectedKey   string      `json:"expected_key"`
	ExpectedError string      `json:"expected_error"`
}

// TestMuSig2KeyAggTestVectors tests that this implementation of musig2 key
// aggregation lines up with the secp256k1-zkp test vectors.
func TestMuSig2KeyAggTestVectors(t *testing.T) {
	t.Parallel()

	var jsonCases []jsonKeyAggTestCase

	testCases := []struct {
		keyOrder      []int
		explicitKeys  []*btcec.PublicKey
		tweaks        []KeyTweakDesc
		expectedKey   []byte
		expectedError error
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

		// Invalid public key.
		{
			keyOrder:      []int{0, 3},
			expectedError: secp256k1.ErrPubKeyNotOnCurve,
		},

		// Public key exceeds field size.
		{
			keyOrder:      []int{0, 4},
			expectedError: secp256k1.ErrPubKeyXTooBig,
		},

		//  Tweak is out of range.
		{
			keyOrder: []int{0, 1},
			tweaks: []KeyTweakDesc{
				KeyTweakDesc{
					Tweak:   to32ByteSlice(invalidTweak),
					IsXOnly: true,
				},
			},
			expectedError: ErrTweakedKeyOverflows,
		},

		// Intermediate tweaking result is point at infinity.
		{
			explicitKeys: []*secp256k1.PublicKey{btcec.Generator()},
			tweaks: []KeyTweakDesc{
				getInfinityTweak(),
			},
			expectedError: ErrTweakedKeyIsInfinity,
		},
	}
	for i, testCase := range testCases {
		testName := fmt.Sprintf("%v", testCase.keyOrder)
		t.Run(testName, func(t *testing.T) {
			var (
				keys      []*btcec.PublicKey
				strKeys   []string
				strTweaks []jsonTweak
				jsonError string
			)
			for _, keyIndex := range testCase.keyOrder {
				keyBytes := testKeys[keyIndex]
				pub, err := schnorr.ParsePubKey(keyBytes)

				switch {
				case testCase.expectedError != nil &&
					errors.Is(err, testCase.expectedError):
					return
				case err != nil:
					t.Fatalf("unable to parse pubkeys: %v", err)
				}

				keys = append(keys, pub)
				strKeys = append(strKeys, hex.EncodeToString(keyBytes))
			}

			for _, explicitKey := range testCase.explicitKeys {
				keys = append(keys, explicitKey)
				strKeys = append(
					strKeys,
					hex.EncodeToString(
						explicitKey.SerializeCompressed(),
					))
			}

			for _, tweak := range testCase.tweaks {
				strTweaks = append(
					strTweaks,
					jsonTweak{
						Tweak: hex.EncodeToString(
							tweak.Tweak[:],
						),
						XOnly: tweak.IsXOnly,
					})
			}

			if testCase.expectedError != nil {
				jsonError = testCase.expectedError.Error()
			}

			jsonCases = append(
				jsonCases,
				jsonKeyAggTestCase{
					Keys:   strKeys,
					Tweaks: strTweaks,
					ExpectedKey: hex.EncodeToString(
						testCase.expectedKey),
					ExpectedError: jsonError,
				})

			uniqueKeyIndex := secondUniqueKeyIndex(keys, false)
			opts := []KeyAggOption{WithUniqueKeyIndex(uniqueKeyIndex)}
			if len(testCase.tweaks) > 0 {
				opts = append(opts, WithKeyTweaks(testCase.tweaks...))
			}

			combinedKey, _, _, err := AggregateKeys(
				keys, false, opts...,
			)

			switch {
			case testCase.expectedError != nil &&
				errors.Is(err, testCase.expectedError):
				return

			case err != nil:
				t.Fatalf("case #%v, got error %v", i, err)
			}

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
		mustParseHex("7FB9E0E687ADA1EEBF7ECFE2F21E73EBDB51A7D450948DF" +
			"E8D76D7F2D1007671"),
	)
	signSetPubKey = schnorr.SerializePubKey(signSetPrivKey.PubKey())

	signTestMsg = mustParseHex("F95466D086770E689964664219266FE5ED215C92A" +
		"E20BAB5C9D79ADDDDF3C0CF")

	signSetKey2 = mustParseHex("F9308A019258C31049344F85F89D5229B531C8458" +
		"36F99B08601F113BCE036F9")

	signSetKey3 = mustParseHex("DFF1D77F2A671C5F36183726DB2341BE58FEAE1DA" +
		"2DECED843240F7B502BA659")

	invalidSetKey1 = mustParseHex("00000000000000000000000000000000" +
		"00000000000000000000000000000007")

	signExpected1 = mustParseHex("68537CC5234E505BD14061F8DA9E90C220A1818" +
		"55FD8BDB7F127BB12403B4D3B")
	signExpected2 = mustParseHex("2DF67BFFF18E3DE797E13C6475C963048138DAE" +
		"C5CB20A357CECA7C8424295EA")
	signExpected3 = mustParseHex("0D5B651E6DE34A29A12DE7A8B4183B4AE6A7F7F" +
		"BE15CDCAFA4A3D1BCAABC7517")

	signSetKeys = [][]byte{signSetPubKey, signSetKey2, signSetKey3, invalidPk1}

	aggregatedNonce = toPubNonceSlice(mustParseHex("028465FCF0BBDBCF443AA" +
		"BCCE533D42B4B5A10966AC09A49655E8C42DAAB8FCD61037496A3CC86926" +
		"D452CAFCFD55D25972CA1675D549310DE296BFF42F72EEEA8C9"))
	verifyPnonce1 = mustParsePubNonce("0337C87821AFD50A8644D820A8F3E02E49" +
		"9C931865C2360FB43D0A0D20DAFE07EA0287BF891D2A6DEAEBADC909352A" +
		"A9405D1428C15F4B75F04DAE642A95C2548480")
	verifyPnonce2 = mustParsePubNonce("0279BE667EF9DCBBAC55A06295CE870B07" +
		"029BFCDB2DCE28D959F2815B16F817980279BE667EF9DCBBAC55A06295CE" +
		"870B07029BFCDB2DCE28D959F2815B16F81798")
	verifyPnonce3 = mustParsePubNonce("032DE2662628C90B03F5E720284EB52FF7" +
		"D71F4284F627B68A853D78C78E1FFE9303E4C5524E83FFE1493B9077CF1C" +
		"A6BEB2090C93D930321071AD40B2F44E599046")

	tweak1 = KeyTweakDesc{
		Tweak: [32]byte{
			0xE8, 0xF7, 0x91, 0xFF, 0x92, 0x25, 0xA2, 0xAF,
			0x01, 0x02, 0xAF, 0xFF, 0x4A, 0x9A, 0x72, 0x3D,
			0x96, 0x12, 0xA6, 0x82, 0xA2, 0x5E, 0xBE, 0x79,
			0x80, 0x2B, 0x26, 0x3C, 0xDF, 0xCD, 0x83, 0xBB,
		},
	}
	tweak2 = KeyTweakDesc{
		Tweak: [32]byte{
			0xae, 0x2e, 0xa7, 0x97, 0xcc, 0xf, 0xe7, 0x2a,
			0xc5, 0xb9, 0x7b, 0x97, 0xf3, 0xc6, 0x95, 0x7d,
			0x7e, 0x41, 0x99, 0xa1, 0x67, 0xa5, 0x8e, 0xb0,
			0x8b, 0xca, 0xff, 0xda, 0x70, 0xac, 0x4, 0x55,
		},
	}
	tweak3 = KeyTweakDesc{
		Tweak: [32]byte{
			0xf5, 0x2e, 0xcb, 0xc5, 0x65, 0xb3, 0xd8, 0xbe,
			0xa2, 0xdf, 0xd5, 0xb7, 0x5a, 0x4f, 0x45, 0x7e,
			0x54, 0x36, 0x98, 0x9, 0x32, 0x2e, 0x41, 0x20,
			0x83, 0x16, 0x26, 0xf2, 0x90, 0xfa, 0x87, 0xe0,
		},
	}
	tweak4 = KeyTweakDesc{
		Tweak: [32]byte{
			0x19, 0x69, 0xad, 0x73, 0xcc, 0x17, 0x7f, 0xa0,
			0xb4, 0xfc, 0xed, 0x6d, 0xf1, 0xf7, 0xbf, 0x99,
			0x7, 0xe6, 0x65, 0xfd, 0xe9, 0xba, 0x19, 0x6a,
			0x74, 0xfe, 0xd0, 0xa3, 0xcf, 0x5a, 0xef, 0x9d,
		},
	}
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

type jsonTweakSignCase struct {
	Keys     []string    `json:"keys"`
	Tweaks   []jsonTweak `json:"tweaks,omitempty"`
	AggNonce string      `json:"agg_nonce"`

	ExpectedSig   string `json:"expected_sig"`
	ExpectedError string `json:"expected_error`
}

type jsonSignTestCase struct {
	SecNonce   string `json:"secret_nonce"`
	SigningKey string `json:"signing_key"`
	Msg        string `json:"msg"`

	TestCases []jsonTweakSignCase `json:"test_cases"`
}

// TestMuSig2SigningTestVectors tests that the musig2 implementation produces
// the same set of signatures.
func TestMuSig2SigningTestVectors(t *testing.T) {
	t.Parallel()

	var jsonCases jsonSignTestCase

	jsonCases.SigningKey = hex.EncodeToString(signSetPrivKey.Serialize())
	jsonCases.Msg = hex.EncodeToString(signTestMsg)

	var secNonce [SecNonceSize]byte
	copy(secNonce[:], mustParseHex("508B81A611F100A6B2B6B29656590898AF488B"+
		"CF2E1F55CF22E5CFB84421FE61"))
	copy(secNonce[32:], mustParseHex("FA27FD49B1D50085B481285E1CA205D55C82"+
		"CC1B31FF5CD54A489829355901F7"))

	jsonCases.SecNonce = hex.EncodeToString(secNonce[:])

	testCases := []struct {
		keyOrder           []int
		aggNonce           [66]byte
		expectedPartialSig []byte
		tweaks             []KeyTweakDesc
		expectedError      error
	}{
		// Vector 1
		{
			keyOrder:           []int{0, 1, 2},
			aggNonce:           aggregatedNonce,
			expectedPartialSig: signExpected1,
		},

		// Vector 2
		{
			keyOrder:           []int{1, 0, 2},
			aggNonce:           aggregatedNonce,
			expectedPartialSig: signExpected2,
		},

		// Vector 3
		{
			keyOrder:           []int{1, 2, 0},
			aggNonce:           aggregatedNonce,
			expectedPartialSig: signExpected3,
		},

		// Vector 4: Signer 2 provided an invalid public key
		{
			keyOrder:      []int{1, 0, 3},
			aggNonce:      aggregatedNonce,
			expectedError: secp256k1.ErrPubKeyNotOnCurve,
		},

		// Vector 5: Aggregate nonce is invalid due wrong tag, 0x04,
		// in the first half.
		{

			keyOrder: []int{1, 2, 0},
			aggNonce: toPubNonceSlice(
				mustParseHex("048465FCF0BBDBCF443AABCCE533D42" +
					"B4B5A10966AC09A49655E8C42DAAB8FCD610" +
					"37496A3CC86926D452CAFCFD55D25972CA16" +
					"75D549310DE296BFF42F72EEEA8C9")),
			expectedError: secp256k1.ErrPubKeyInvalidFormat,
		},

		// Vector 6: Aggregate nonce is invalid because the second half
		// does not correspond to an X coordinate.
		{

			keyOrder: []int{1, 2, 0},
			aggNonce: toPubNonceSlice(
				mustParseHex("028465FCF0BBDBCF443AABCCE533D42" +
					"B4B5A10966AC09A49655E8C42DAAB8FCD610" +
					"200000000000000000000000000000000000" +
					"00000000000000000000000000009")),
			expectedError: secp256k1.ErrPubKeyNotOnCurve,
		},

		// Vector 7: Aggregate nonce is invalid because the second half
		// exceeds field size.
		{

			keyOrder: []int{1, 2, 0},
			aggNonce: toPubNonceSlice(
				mustParseHex("028465FCF0BBDBCF443AABCCE533D42" +
					"B4B5A10966AC09A49655E8C42DAAB8FCD610" +
					"2FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF" +
					"FFFFFFFFFFFFFFFFFFFFEFFFFFC30")),
			expectedError: secp256k1.ErrPubKeyXTooBig,
		},

		// A single x-only tweak.
		{
			keyOrder: []int{1, 2, 0},
			aggNonce: aggregatedNonce,
			expectedPartialSig: mustParseHex("5e24c7496b565debc3b" +
				"9639e6f1304a21597f9603d3ab05b4913641775e1375b"),
			tweaks: []KeyTweakDesc{genTweakParity(tweak1, true)},
		},

		// A single ordinary tweak.
		{
			keyOrder: []int{1, 2, 0},
			aggNonce: aggregatedNonce,
			expectedPartialSig: mustParseHex("78408ddcab4813d1394c" +
				"97d493ef1084195c1d4b52e63ecd7bc5991644e44ddd"),
			tweaks: []KeyTweakDesc{genTweakParity(tweak1, false)},
		},

		// An ordinary tweak then an x-only tweak.
		{
			keyOrder: []int{1, 2, 0},
			aggNonce: aggregatedNonce,
			expectedPartialSig: mustParseHex("C3A829A81480E36EC3A" +
				"B052964509A94EBF34210403D16B226A6F16EC85B7357"),
			tweaks: []KeyTweakDesc{
				genTweakParity(tweak1, false),
				genTweakParity(tweak2, true),
			},
		},

		// Four tweaks, in the order: x-only, ordinary, x-only, ordinary.
		{
			keyOrder: []int{1, 2, 0},
			aggNonce: aggregatedNonce,
			expectedPartialSig: mustParseHex("8C4473C6A382BD3C4AD" +
				"7BE59818DA5ED7CF8CEC4BC21996CFDA08BB4316B8BC7"),
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
			keySet := make([]*btcec.PublicKey, 0, len(testCase.keyOrder))
			for _, keyIndex := range testCase.keyOrder {
				keyBytes := signSetKeys[keyIndex]
				pub, err := schnorr.ParsePubKey(keyBytes)

				switch {
				case testCase.expectedError != nil &&
					errors.Is(err, testCase.expectedError):

					return
				case err != nil:
					t.Fatalf("unable to parse pubkeys: %v", err)
				}

				keySet = append(keySet, pub)
			}

			var opts []SignOption
			if len(testCase.tweaks) != 0 {
				opts = append(
					opts, WithTweaks(testCase.tweaks...),
				)
			}

			partialSig, err := Sign(
				secNonce, signSetPrivKey, testCase.aggNonce,
				keySet, msg, opts...,
			)

			switch {
			case testCase.expectedError != nil &&
				errors.Is(err, testCase.expectedError):

				return
			case err != nil:
				t.Fatalf("unable to generate partial sig: %v", err)
			}

			var partialSigBytes [32]byte
			partialSig.S.PutBytesUnchecked(partialSigBytes[:])

			if !bytes.Equal(partialSigBytes[:], testCase.expectedPartialSig) {
				t.Fatalf("sigs don't match: expected %x, got %x",
					testCase.expectedPartialSig, partialSigBytes,
				)
			}

		})

		if *dumpJson {
			var (
				strKeys   []string
				jsonError string
			)

			for _, keyIndex := range testCase.keyOrder {
				keyBytes := signSetKeys[keyIndex]
				strKeys = append(strKeys, hex.EncodeToString(keyBytes))
			}

			if testCase.expectedError != nil {
				jsonError = testCase.expectedError.Error()
			}

			tweakSignCase := jsonTweakSignCase{
				Keys:          strKeys,
				ExpectedSig:   hex.EncodeToString(testCase.expectedPartialSig),
				AggNonce:      hex.EncodeToString(testCase.aggNonce[:]),
				ExpectedError: jsonError,
			}

			var jsonTweaks []jsonTweak
			for _, tweak := range testCase.tweaks {
				jsonTweaks = append(
					jsonTweaks,
					jsonTweak{
						Tweak: hex.EncodeToString(tweak.Tweak[:]),
						XOnly: tweak.IsXOnly,
					})
			}
			tweakSignCase.Tweaks = jsonTweaks

			jsonCases.TestCases = append(jsonCases.TestCases, tweakSignCase)
		}
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

func TestMusig2PartialSigVerifyTestVectors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		partialSig    []byte
		nonces        [][66]byte
		pubnonceIndex int
		keyOrder      []int
		tweaks        []KeyTweakDesc
		expectedError error
	}{
		// A single x-only tweak.
		{
			keyOrder: []int{1, 2, 0},
			nonces: [][66]byte{
				verifyPnonce2,
				verifyPnonce3,
				verifyPnonce1,
			},
			pubnonceIndex: 2,
			partialSig: mustParseHex("5e24c7496b565debc3b9639e" +
				"6f1304a21597f9603d3ab05b4913641775e1375b"),
			tweaks: []KeyTweakDesc{genTweakParity(tweak1, true)},
		},
		// A single ordinary tweak.
		{
			keyOrder: []int{1, 2, 0},
			nonces: [][66]byte{
				verifyPnonce2,
				verifyPnonce3,
				verifyPnonce1,
			},
			pubnonceIndex: 2,
			partialSig: mustParseHex("78408ddcab4813d1394c97d4" +
				"93ef1084195c1d4b52e63ecd7bc5991644e44ddd"),
			tweaks: []KeyTweakDesc{genTweakParity(tweak1, false)},
		},
		// An ordinary tweak then an x-only tweak.
		{
			keyOrder: []int{1, 2, 0},
			nonces: [][66]byte{
				verifyPnonce2,
				verifyPnonce3,
				verifyPnonce1,
			},
			pubnonceIndex: 2,
			partialSig: mustParseHex("C3A829A81480E36EC3AB0529" +
				"64509A94EBF34210403D16B226A6F16EC85B7357"),
			tweaks: []KeyTweakDesc{
				genTweakParity(tweak1, false),
				genTweakParity(tweak2, true),
			},
		},

		// Four tweaks, in the order: x-only, ordinary, x-only, ordinary.
		{
			keyOrder: []int{1, 2, 0},
			nonces: [][66]byte{
				verifyPnonce2,
				verifyPnonce3,
				verifyPnonce1,
			},
			pubnonceIndex: 2,
			partialSig: mustParseHex("8C4473C6A382BD3C4AD7BE5" +
				"9818DA5ED7CF8CEC4BC21996CFDA08BB4316B8BC7"),
			tweaks: []KeyTweakDesc{
				genTweakParity(tweak1, true),
				genTweakParity(tweak2, false),
				genTweakParity(tweak3, true),
				genTweakParity(tweak4, false),
			},
		},
		// Vector 8.
		{

			partialSig:    signExpected1,
			pubnonceIndex: 0,
			keyOrder:      []int{0, 1, 2},
			nonces: [][66]byte{
				verifyPnonce1,
				verifyPnonce2,
				verifyPnonce3,
			},
		},
		// Vector 9.
		{

			partialSig:    signExpected2,
			pubnonceIndex: 1,
			keyOrder:      []int{1, 0, 2},
			nonces: [][66]byte{
				verifyPnonce2,
				verifyPnonce1,
				verifyPnonce3,
			},
		},
		// Vector 10.
		{

			partialSig:    signExpected3,
			pubnonceIndex: 2,
			keyOrder:      []int{1, 2, 0},
			nonces: [][66]byte{
				verifyPnonce2,
				verifyPnonce3,
				verifyPnonce1,
			},
		},
		// Vector 11: Wrong signature (which is equal to the negation
		// of valid signature expected[0]).
		{

			partialSig: mustParseHex("97AC833ADCB1AFA42EBF9E0" +
				"725616F3C9A0D5B614F6FE283CEAAA37A8FFAF406"),
			pubnonceIndex: 0,
			keyOrder:      []int{0, 1, 2},
			nonces: [][66]byte{
				verifyPnonce1,
				verifyPnonce2,
				verifyPnonce3,
			},
			expectedError: ErrPartialSigInvalid,
		},
		// Vector 12: Wrong signer.
		{

			partialSig:    signExpected1,
			pubnonceIndex: 1,
			keyOrder:      []int{0, 1, 2},
			nonces: [][66]byte{
				verifyPnonce1,
				verifyPnonce2,
				verifyPnonce3,
			},
			expectedError: ErrPartialSigInvalid,
		},
		// Vector 13: Signature exceeds group size.
		{

			partialSig: mustParseHex("FFFFFFFFFFFFFFFFFFFFFFFF" +
				"FFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141"),
			pubnonceIndex: 0,
			keyOrder:      []int{0, 1, 2},
			nonces: [][66]byte{
				verifyPnonce1,
				verifyPnonce2,
				verifyPnonce3,
			},
			expectedError: ErrPartialSigInvalid,
		},
		// Vector 14: Invalid pubnonce.
		{

			partialSig:    signExpected1,
			pubnonceIndex: 0,
			keyOrder:      []int{0, 1, 2},
			nonces: [][66]byte{
				canParsePubNonce("020000000000000000000000000" +
					"000000000000000000000000000000000000009"),
				verifyPnonce2,
				verifyPnonce3,
			},
			expectedError: secp256k1.ErrPubKeyNotOnCurve,
		},
		// Vector 15: Invalid public key.
		{

			partialSig:    signExpected1,
			pubnonceIndex: 0,
			keyOrder:      []int{3, 1, 2},
			nonces: [][66]byte{
				verifyPnonce1,
				verifyPnonce2,
				verifyPnonce3,
			},
			expectedError: secp256k1.ErrPubKeyNotOnCurve,
		},
	}

	for _, testCase := range testCases {

		// todo find name
		testName := fmt.Sprintf("%v/tweak=%v", testCase.pubnonceIndex, testCase.keyOrder)

		t.Run(testName, func(t *testing.T) {

			combinedNonce, err := AggregateNonces(testCase.nonces)

			switch {
			case testCase.expectedError != nil &&
				errors.Is(err, testCase.expectedError):

				return
			case err != nil:
				t.Fatalf("unable to aggregate nonces %v", err)
			}

			keySet := make([]*btcec.PublicKey, 0, len(testCase.keyOrder))
			for _, keyIndex := range testCase.keyOrder {
				keyBytes := signSetKeys[keyIndex]
				pub, err := schnorr.ParsePubKey(keyBytes)

				switch {
				case testCase.expectedError != nil &&
					errors.Is(err, testCase.expectedError):

					return
				case err != nil:
					t.Fatalf("unable to parse pubkeys: %v", err)
				}

				keySet = append(keySet, pub)
			}

			ps := &PartialSignature{}
			err = ps.Decode(bytes.NewBuffer(testCase.partialSig))

			switch {
			case testCase.expectedError != nil &&
				errors.Is(err, testCase.expectedError):

				return
			case err != nil:
				t.Fatal(err)
			}

			var opts []SignOption
			if len(testCase.tweaks) != 0 {
				opts = append(
					opts, WithTweaks(testCase.tweaks...),
				)
			}

			err = verifyPartialSig(
				ps,
				testCase.nonces[testCase.pubnonceIndex],
				combinedNonce,
				keySet,
				signSetKeys[testCase.keyOrder[testCase.pubnonceIndex]],
				to32ByteSlice(signTestMsg),
				opts...,
			)

			switch {
			case testCase.expectedError != nil &&
				errors.Is(err, testCase.expectedError):

				return
			case err != nil:
				t.Fatalf("unable to aggregate nonces %v", err)
			}
		})
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

// TestMusig2NonceGenTestVectors tests the nonce generation function with
// the testvectors defined in the Musig2 BIP.
func TestMusig2NonceGenTestVectors(t *testing.T) {
	t.Parallel()

	msg := bytes.Repeat([]byte{0x01}, 32)
	sk := bytes.Repeat([]byte{0x02}, 32)
	aggpk := bytes.Repeat([]byte{0x07}, 32)
	extra_in := bytes.Repeat([]byte{0x08}, 32)

	testCases := []struct {
		opts          nonceGenOpts
		expectedNonce string
	}{
		{
			opts: nonceGenOpts{
				randReader:  &memsetRandReader{i: 0},
				secretKey:   sk[:],
				combinedKey: aggpk[:],
				auxInput:    extra_in[:],
				msg:         msg[:],
			},
			expectedNonce: "E8F2E103D86800F19A4E97338D371CB885DB2" +
				"F19D08C0BD205BBA9B906C971D0D786A17718AAFAD6D" +
				"E025DDDD99DC823E2DFC1AE1DDFE920888AD53FFF423FC4",
		},
		{
			opts: nonceGenOpts{
				randReader:  &memsetRandReader{i: 0},
				secretKey:   sk[:],
				combinedKey: aggpk[:],
				auxInput:    extra_in[:],
				msg:         nil,
			},
			expectedNonce: "8A633F5EECBDB690A6BE4921426F41BE78D50" +
				"9DC1CE894C1215844C0E4C6DE7ABC9A5BE0A3BF3FE31" +
				"2CCB7E4817D2CB17A7CEA8382B73A99A583E323387B3C32",
		},
		{
			opts: nonceGenOpts{
				randReader:  &memsetRandReader{i: 0},
				secretKey:   nil,
				combinedKey: nil,
				auxInput:    nil,
				msg:         nil,
			},
			expectedNonce: "7B3B5A002356471AF0E961DE2549C121BD0D4" +
				"8ABCEEDC6E034BDDF86AD3E0A187ECEE674CEF7364B0" +
				"BC4BEEFB8B66CAD89F98DE2F8C5A5EAD5D1D1E4BD7D04CD",
		},
	}

	for _, testCase := range testCases {
		nonce, err := GenNonces(withCustomOptions(testCase.opts))
		if err != nil {
			t.Fatalf("err gen nonce aux bytes %v", err)
		}

		expectedBytes, _ := hex.DecodeString(testCase.expectedNonce)
		if !bytes.Equal(nonce.SecNonce[:], expectedBytes) {

			t.Fatalf("nonces don't match: expected %x, got %x",
				expectedBytes, nonce.SecNonce[:])
		}
	}

}

var (
	pNonce1, _ = hex.DecodeString("020151C80F435648DF67A22B749CD798CE54E0321D034B92B709B567D60A42E666" +
		"03BA47FBC1834437B3212E89A84D8425E7BF12E0245D98262268EBDCB385D50641")
	pNonce2, _ = hex.DecodeString("03FF406FFD8ADB9CD29877E4985014F66A59F6CD01C0E88CAA8E5F3166B1F676A6" +
		"0248C264CDD57D3C24D79990B0F865674EB62A0F9018277A95011B41BFC193B833")

	expectedNonce, _ = hex.DecodeString("035FE1873B4F2967F52FEA4A06AD5A8ECCBE9D0FD73068012C894E2E87CCB5804B" +
		"024725377345BDE0E9C33AF3C43C0A29A9249F2F2956FA8CFEB55C8573D0262DC8")

	invalidNonce1, _ = hex.DecodeString("04FF406FFD8ADB9CD29877E4985014F66A59F6CD01C0E88CAA8E5F3166B1F676A6" + "0248C264CDD57D3C24D79990B0F865674EB62A0F9018277A95011B41BFC193B833")
	invalidNonce2, _ = hex.DecodeString("03FF406FFD8ADB9CD29877E4985014F66A59F6CD01C0E88CAA8E5F3166B1F676A6" + "0248C264CDD57D3C24D79990B0F865674EB62A0F9018277A95011B41BFC193B831")
	invalidNonce3, _ = hex.DecodeString("03FF406FFD8ADB9CD29877E4985014F66A59F6CD01C0E88CAA8E5F3166B1F676A6" + "02FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC30")
)

type jsonNonceAggTestCase struct {
	Nonces        []string `json:"nonces"`
	ExpectedNonce string   `json:"expected_key"`
	ExpectedError string   `json:"expected_error"`
}

func TestMusig2AggregateNoncesTestVectors(t *testing.T) {
	t.Parallel()

	var jsonCases []jsonNonceAggTestCase

	testCases := []struct {
		nonces        [][]byte
		expectedNonce []byte
		expectedError error
	}{
		// Vector 1: Valid.
		{
			nonces:        [][]byte{pNonce1, pNonce2},
			expectedNonce: expectedNonce,
		},

		// Vector 2: Public nonce from signer 1 is invalid due wrong
		// tag, 0x04, inthe first half.
		{
			nonces:        [][]byte{pNonce1, invalidNonce1},
			expectedError: secp256k1.ErrPubKeyInvalidFormat,
		},

		// Vector 3: Public nonce from signer 0 is invalid because the
		// second half does not correspond to an X coordinate.
		{
			nonces:        [][]byte{invalidNonce2, pNonce2},
			expectedError: secp256k1.ErrPubKeyNotOnCurve,
		},

		// Vector 4: Public nonce from signer 0 is invalid because
		// second half exceeds field size.
		{
			nonces:        [][]byte{invalidNonce3, pNonce2},
			expectedError: secp256k1.ErrPubKeyXTooBig,
		},

		// Vector 5: Sum of second points encoded in the nonces would
		// be point at infinity, therefore set sum to base point G.
		{
			nonces: [][]byte{
				append(
					append([]byte{}, pNonce1[0:33]...),
					getGBytes()...,
				),
				append(
					append([]byte{}, pNonce2[0:33]...),
					getNegGBytes()...,
				),
			},
			expectedNonce: append(
				append([]byte{}, expectedNonce[0:33]...),
				getGBytes()...,
			),
		},
	}
	for i, testCase := range testCases {
		testName := fmt.Sprintf("Vector %v", i+1)
		t.Run(testName, func(t *testing.T) {
			var (
				nonces    [][66]byte
				strNonces []string
				jsonError string
			)
			for _, nonce := range testCase.nonces {
				nonces = append(nonces, toPubNonceSlice(nonce))
				strNonces = append(strNonces, hex.EncodeToString(nonce))
			}

			if testCase.expectedError != nil {
				jsonError = testCase.expectedError.Error()
			}

			jsonCases = append(jsonCases, jsonNonceAggTestCase{
				Nonces:        strNonces,
				ExpectedNonce: hex.EncodeToString(expectedNonce),
				ExpectedError: jsonError,
			})

			aggregatedNonce, err := AggregateNonces(nonces)

			switch {
			case testCase.expectedError != nil &&
				errors.Is(err, testCase.expectedError):

				return
			case err != nil:
				t.Fatalf("aggregating nonce error: %v", err)
			}

			if !bytes.Equal(testCase.expectedNonce, aggregatedNonce[:]) {
				t.Fatalf("case: #%v, invalid nonce aggregation: "+
					"expected %x, got %x", i, testCase.expectedNonce,
					aggregatedNonce)
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
			nonceAggTestVectorName, formattedJson.Bytes(), 0644,
		)
		if err != nil {
			t.Fatalf("unable to write file: %v", err)
		}
	}
}

type memsetRandReader struct {
	i int
}

func (mr *memsetRandReader) Read(buf []byte) (n int, err error) {
	for i := range buf {
		buf[i] = byte(mr.i)
	}
	return len(buf), nil
}

func memsetLoop(a []byte, v uint8) {
	for i := range a {
		a[i] = byte(v)
	}
}

func to32ByteSlice(input []byte) [32]byte {
	if len(input) != 32 {
		panic("input byte slice has invalid length")
	}
	var output [32]byte
	copy(output[:], input)
	return output
}

func toPubNonceSlice(input []byte) [PubNonceSize]byte {
	var output [PubNonceSize]byte
	copy(output[:], input)

	return output
}

func getGBytes() []byte {
	return btcec.Generator().SerializeCompressed()
}

func getNegGBytes() []byte {
	pk := getGBytes()
	pk[0] = 0x3

	return pk
}

func mustParsePubNonce(str string) [PubNonceSize]byte {
	b, err := hex.DecodeString(str)
	if err != nil {
		panic(fmt.Errorf("unable to parse hex: %v", err))
	}
	if len(b) != PubNonceSize {
		panic(fmt.Errorf("not a public nonce: %v", err))
	}
	return toPubNonceSlice(b)
}

func canParsePubNonce(str string) [PubNonceSize]byte {
	b, err := hex.DecodeString(str)
	if err != nil {
		panic(fmt.Errorf("unable to parse hex: %v", err))
	}
	return toPubNonceSlice(b)
}
