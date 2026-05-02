package bip322

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

var (
	netParams             = &chaincfg.MainNetParams
	testValue      int64  = 345678
	testCSVTimeout uint32 = 2016

	hexDecode = hex.DecodeString
	hexEncode = hex.EncodeToString

	testRevokePrivateKey, _ = hexDecode(
		"318ea3020520a834e013c38ef4400d098fd56299c98dba89ece7a2466526" +
			"984b",
	)

	timeLockTemplate = `
		OP_IF
			{{ hex .RevokeKey }}
		OP_ELSE
			{{ .CsvTimeout }} OP_CHECKSEQUENCEVERIFY OP_DROP
			{{ hex .SelfKey }}
		OP_ENDIF
		OP_CHECKSIG
	`
	multiSig2of2Template = `
		OP_2
		{{ hex .FirstKey }}
		{{ hex .SecondKey }}
		OP_2
		OP_CHECKMULTISIG
	`
	multiSig3of3Template = `
		OP_3
		{{ hex .FirstKey }}
		{{ hex .SecondKey }}
		{{ hex .ThirdKey }}
		OP_3
		OP_CHECKMULTISIG
	`
)

// testVectors is the top-level structure of the test data files.
type testVectors struct {
	TxHashes     []txHashVector          `json:"tx_hashes,omitempty"`
	Simple       []simpleSignatureVector `json:"simple"`
	Full         []fullSignatureVector   `json:"full"`
	ProofOfFunds []pofSignatureVector    `json:"proof_of_funds"`
	Error        []errorVector           `json:"error"`
}

// txHashVector contains expected transaction hashes for a given message.
type txHashVector struct {
	Message       string `json:"message"`
	Address       string `json:"address"`
	MessageHash   string `json:"message_hash"`
	ToSpendTxHash string `json:"to_spend_tx_hash"`
	ToSignTxHash  string `json:"to_sign_tx_hash"`
}

// simpleSignatureVector contains BIP-322 signature data for a given "simple"
// variant test case.
type simpleSignatureVector struct {
	Message          string   `json:"message"`
	PrivateKeys      []string `json:"private_keys"`
	Address          string   `json:"address"`
	Type             string   `json:"type"`
	WitnessScript    string   `json:"witness_script"`
	Bip322Signatures []string `json:"bip322_signatures"`
}

// fullSignatureVector contains BIP-322 signature data for a given "full"
// variant test case.
type fullSignatureVector struct {
	simpleSignatureVector
	SigScript string `json:"sig_script"`
	TxVersion int32  `json:"tx_version"`
	LockTime  uint32 `json:"lock_time"`
	Sequence  uint32 `json:"sequence"`
}

// additionalInput defines an additional input that is part of a
// "Proof of Funds" test case.
type additionalInput struct {
	PrivateKeyIndex int    `json:"private_key_index"`
	Type            string `json:"type"`
	Value           int64  `json:"value"`
	PkScript        string `json:"pk_script"`
}

// pofSignatureVector contains BIP-322 signature data for a given
// "Proof of Fund" variant test case.
type pofSignatureVector struct {
	fullSignatureVector

	// AdditionalInputs is a two-dimensional slice that defines inputs we
	// want to add to a Proof of Fund transaction, grouped by previous
	// transaction.
	AdditionalInputs [][]additionalInput `json:"additional_inputs"`
}

// errorVector contains a test case that must fail verification.
type errorVector struct {
	Description string `json:"description"`
	Message     string `json:"message"`
	Address     string `json:"address"`
	Signature   string `json:"signature"`
	ErrorSubstr string `json:"error_substr"`
}

// loadTestVectors reads and parses a test data file.
func loadTestVectors(t *testing.T, fileName string) *testVectors {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", fileName))
	require.NoError(t, err)

	var vectors testVectors
	err = json.Unmarshal(data, &vectors)
	require.NoError(t, err)

	return &vectors
}

// storeTestVectors writes the test vectors to the test file.
func storeTestVectors(t *testing.T, vectors *testVectors, fileName string) {
	t.Helper()

	data, err := json.MarshalIndent(vectors, "", "  ")
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join("testdata", fileName), data, 0644)
	require.NoError(t, err)
}

// testName returns a human-readable sub-test name for a given message.
func testName(scriptType, message string) string {
	if message == "" {
		return fmt.Sprintf("type=%s,msg=<empty>", scriptType)
	}

	return fmt.Sprintf("type=%s,msg=%s", scriptType, message)
}

// parseWIFs parses a list of WIFs into a slice of *btcutil.WIF.
func parseWIFs(t *testing.T, keys []string) []*btcutil.WIF {
	wifs := make([]*btcutil.WIF, len(keys))
	for i, key := range keys {
		var err error
		wifs[i], err = btcutil.DecodeWIF(key)
		require.NoError(t, err)
	}
	return wifs
}

// formatWIFs formats a slice of *btcutil.WIF into a slice of strings.
func formatWIFs(wifs []*btcutil.WIF) []string {
	keys := make([]string, len(wifs))
	for i, wif := range wifs {
		keys[i] = wif.String()
	}
	return keys
}

// signAndExtract signs the given toSignTx with the given private keys and
// returns the resulting signed transaction.
func signAndExtract(t *testing.T, privateKeys []string, typ, witnessScriptHex,
	sigScriptHex string, toSignTx *psbt.Packet) (string, *wire.MsgTx) {

	wifs := parseWIFs(t, privateKeys)
	inputType, err := parseInputType(typ)
	require.NoError(t, err)

	witnessScript, err := hexDecode(witnessScriptHex)
	require.NoError(t, err)

	// Must be nil when empty for PSBT library.
	if len(witnessScript) == 0 {
		witnessScript = nil
	}

	sigScript, err := hexDecode(sigScriptHex)
	require.NoError(t, err)

	if len(sigScript) == 0 {
		sigScript = nil
	}

	inputType.decorateInput(
		t, wifs[0].PrivKey, &toSignTx.Inputs[0],
		witnessScript, sigScript,
	)
	inputType.sign(t, 0, wifs, toSignTx)
	inputType.finalize(t, toSignTx)

	signature, err := SerializeSignature(toSignTx)
	require.NoError(t, err)

	signedTx, err := psbt.Extract(toSignTx)
	require.NoError(t, err)

	return signature, signedTx
}

// TestTxHashesVectors tests the tx_hashes test vectors as mentioned in BIP-322:
// https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
func TestTxHashesVectors(t *testing.T) {
	vectors := loadTestVectors(t, "basic-test-vectors.json")

	for _, tc := range vectors.TxHashes {
		t.Run(testName("n/a", tc.Message), func(t *testing.T) {
			addr, err := btcutil.DecodeAddress(
				tc.Address, &chaincfg.MainNetParams,
			)
			require.NoError(t, err)

			scriptPubKey, err := txscript.PayToAddrScript(addr)
			require.NoError(t, err)

			toSpendTx := buildToSpendTx(
				[]byte(tc.Message), scriptPubKey,
			)

			// The message hash must be set as the OP_PUSH of the
			// first input's scriptSig.
			msgHash := toSpendTx.TxIn[0].SignatureScript[2:]
			require.Equal(t, tc.MessageHash, hexEncode(msgHash))

			require.Equal(
				t, tc.ToSpendTxHash,
				toSpendTx.TxHash().String(),
			)

			toSignTx, err := BuildToSignPacketSimple(
				[]byte(tc.Message), scriptPubKey,
			)
			require.NoError(t, err)

			require.Equal(
				t, tc.ToSignTxHash,
				toSignTx.UnsignedTx.TxHash().String(),
			)
		})
	}
}

// TestSimpleSignaturesVectors tests the simple variant test vectors as
// mentioned in BIP-322:
// https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
func TestSimpleSignaturesVectors(t *testing.T) {
	validateSimpleVector := func(t *testing.T, tc simpleSignatureVector) {
		addr, err := btcutil.DecodeAddress(
			tc.Address, &chaincfg.MainNetParams,
		)
		require.NoError(t, err)

		scriptPubKey, err := txscript.PayToAddrScript(addr)
		require.NoError(t, err)

		toSignTx, err := BuildToSignPacketSimple(
			[]byte(tc.Message), scriptPubKey,
		)
		require.NoError(t, err)

		signature, _ := signAndExtract(
			t, tc.PrivateKeys, tc.Type, tc.WitnessScript, "",
			toSignTx,
		)
		require.NoError(t, err)

		// Special case for the fallback to no prefix, which is allowed
		// for the "simple" variant.
		expectedSigs := tc.Bip322Signatures
		if len(expectedSigs) == 1 &&
			!strings.Contains(expectedSigs[0], PrefixSimple) {

			require.Contains(
				t, expectedSigs, signature[len(PrefixSimple):],
			)
		} else {
			require.Contains(t, expectedSigs, signature)
		}

		require.Contains(t, signature, PrefixSimple)
		witnessBytes, err := b64Decode(signature[len(PrefixSimple):])
		require.NoError(t, err)
		witness, err := ParseTxWitness(witnessBytes)
		require.NoError(t, err)

		// Verify the signature with the corresponding function.
		valid, _, err := VerifyMessageSimple(
			[]byte(tc.Message), scriptPubKey, witness,
		)
		require.NoError(t, err)
		require.True(t, valid, "signature must be valid")

		// And with the generic function.
		valid, _, err = VerifyMessage(tc.Message, addr, expectedSigs[0])
		require.NoError(t, err)
		require.True(t, valid, "signature must be valid")

		// And make sure an incorrect message produces a negative
		// result.
		valid, _, err = VerifyMessage(
			"incorrect message", addr, expectedSigs[0],
		)
		require.ErrorContains(t, err, "invalid signature")
		require.False(t, valid, "signature of fake msg must be invalid")
	}

	vectors := loadTestVectors(t, "basic-test-vectors.json")

	for _, tc := range vectors.Simple {
		t.Run(testName(tc.Type, tc.Message), func(t *testing.T) {
			validateSimpleVector(t, tc)
		})
	}
}

// TestFullSignaturesVectors tests the full variant test vectors as mentioned in
// BIP-322: https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
func TestFullSignaturesVectors(t *testing.T) {
	validateFullVector := func(t *testing.T, tc fullSignatureVector) {
		addr, err := btcutil.DecodeAddress(
			tc.Address, &chaincfg.MainNetParams,
		)
		require.NoError(t, err)

		scriptPubKey, err := txscript.PayToAddrScript(addr)
		require.NoError(t, err)

		toSignTx := BuildToSignPacketFull(
			[]byte(tc.Message), scriptPubKey, tc.TxVersion,
			tc.LockTime, tc.Sequence,
		)
		require.NoError(t, err)

		signature, signedTx := signAndExtract(
			t, tc.PrivateKeys, tc.Type, tc.WitnessScript,
			tc.SigScript, toSignTx,
		)

		require.Contains(t, tc.Bip322Signatures, signature)

		// Verify the signature with the corresponding function.
		valid, _, err := VerifyMessageFull(
			[]byte(tc.Message), scriptPubKey,
			signedTx.TxIn[0].SignatureScript,
			signedTx.TxIn[0].Witness, tc.TxVersion, tc.LockTime,
			tc.Sequence,
		)
		require.NoError(t, err)
		require.True(t, valid, "signature must be valid")

		// And with the generic function.
		valid, _, err = VerifyMessage(
			tc.Message, addr, tc.Bip322Signatures[0],
		)
		require.NoError(t, err)
		require.True(t, valid, "signature must be valid")

		// And make sure an incorrect message produces a negative
		// result.
		valid, _, err = VerifyMessage(
			"incorrect message", addr, tc.Bip322Signatures[0],
		)
		require.ErrorContains(t, err, "invalid signature")
		require.False(t, valid, "signature of fake msg must be invalid")
	}

	vectors := loadTestVectors(t, "basic-test-vectors.json")
	for _, tc := range vectors.Full {
		t.Run(testName(tc.Type, tc.Message), func(t *testing.T) {
			validateFullVector(t, tc)
		})
	}

	vectors = loadTestVectors(t, "generated-test-vectors.json")
	for _, tc := range vectors.Full {
		t.Run(testName(tc.Type, tc.Message), func(t *testing.T) {
			validateFullVector(t, tc)
		})
	}
}

// TestPoFSignaturesVectors tests the Proof of Funds variant test vectors as
// mentioned in BIP-322:
// https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
func TestPoFSignaturesVectors(t *testing.T) {
	validatePoFVector := func(t *testing.T, tc pofSignatureVector) {
		addr, err := btcutil.DecodeAddress(
			tc.Address, &chaincfg.MainNetParams,
		)
		require.NoError(t, err)

		// And with the generic function.
		valid, _, err := VerifyMessage(
			tc.Message, addr, tc.Bip322Signatures[0],
		)
		require.NoError(t, err)
		require.True(t, valid, "signature must be valid")

		// And make sure an incorrect message produces a negative
		// result.
		valid, _, err = VerifyMessage(
			"incorrect message", addr, tc.Bip322Signatures[0],
		)
		require.ErrorContains(t, err, "invalid signature")
		require.False(t, valid, "signature of fake msg must be invalid")
	}

	vectors := loadTestVectors(t, "generated-test-vectors.json")
	for _, tc := range vectors.ProofOfFunds {
		t.Run(testName(tc.Type, tc.Message), func(t *testing.T) {
			validatePoFVector(t, tc)
		})
	}
}

// TestErrorVectors tests that invalid signatures, wrong messages, and wrong
// addresses are correctly rejected during BIP-322 verification.
func TestErrorVectors(t *testing.T) {
	testErrorVectors := func(t *testing.T, vectors *testVectors) {
		for _, tc := range vectors.Error {
			t.Run(tc.Description, func(t *testing.T) {
				addr, err := btcutil.DecodeAddress(
					tc.Address, netParams,
				)
				require.NoError(t, err)

				valid, _, err := VerifyMessage(
					tc.Message, addr, tc.Signature,
				)
				require.False(t, valid)
				require.Error(t, err)
				require.Contains(
					t, err.Error(), tc.ErrorSubstr,
				)
			})
		}
	}

	vectors := loadTestVectors(t, "basic-test-vectors.json")
	testErrorVectors(t, vectors)

	vectors = loadTestVectors(t, "generated-test-vectors.json")
	testErrorVectors(t, vectors)
}

// testInputType is a type that represents different types of inputs that are
// used to test the BIP-322 message signing and verification.
type testInputType uint8

const (
	plainP2PKH testInputType = iota
	plainP2WPKH
	plainP2TR
	timeLockP2TR
	nestedP2WPKH
	timeLockP2WSH
	multisig2of2P2WSH
	multisig3of3P2WSH
	nestedMultisig2of2P2WSH
	legacyMultisig2of2P2SH
)

// pofTestCase is a single test case for a Proof of Funds variant.
type pofTestCase struct {
	name               string
	challengeInputType testInputType
	pofInputTypes      [][]testInputType
}

var (
	// allTypes is the list of all defined testInputType constants. This is
	// used to test the "full" variant.
	allTypes = []testInputType{
		plainP2PKH,
		plainP2WPKH,
		plainP2TR,
		timeLockP2TR,
		nestedP2WPKH,
		timeLockP2WSH,
		multisig2of2P2WSH,
		multisig3of3P2WSH,
		nestedMultisig2of2P2WSH,
		legacyMultisig2of2P2SH,
	}

	// nativeSegWitTypes is the list of testInputType constants that use
	// native segwit scripts. This is used to test the "simple" variant.
	nativeSegWitTypes = []testInputType{
		plainP2WPKH,
		plainP2TR,
		multisig2of2P2WSH,
		multisig3of3P2WSH,
	}

	// legacyTypes is the list of testInputType constants that use legacy
	// scripts. This is used to test that the "simple" variant errors out
	// for legacy scripts.
	legacyTypes = []testInputType{
		plainP2PKH,
		nestedP2WPKH,
		nestedMultisig2of2P2WSH,
		legacyMultisig2of2P2SH,
	}

	// pofTestCases is the list of all Proof of Funds test cases.
	pofTestCases = []pofTestCase{
		{
			name:               "p2pkh",
			challengeInputType: plainP2PKH,
			pofInputTypes: [][]testInputType{
				{plainP2PKH},
				{nestedP2WPKH},
			},
		},
		{
			name:               "p2pkh-with-utxo-re-use",
			challengeInputType: plainP2PKH,
			pofInputTypes: [][]testInputType{
				{plainP2PKH, plainP2PKH, plainP2PKH},
				{nestedP2WPKH},
			},
		},
		{
			name:               "p2tr",
			challengeInputType: plainP2TR,
			pofInputTypes: [][]testInputType{
				{plainP2TR},
				{plainP2TR, plainP2TR},
			},
		},
	}
)

func parseInputType(s string) (testInputType, error) {
	for _, t := range allTypes {
		if t.String() == s {
			return t, nil
		}
	}
	return 0, fmt.Errorf("unknown input type: %s", s)
}

func (i testInputType) String() string {
	switch i {
	case plainP2PKH:
		return "p2pkh"
	case plainP2WPKH:
		return "p2wpkh"
	case plainP2TR:
		return "p2tr"
	case timeLockP2TR:
		return "p2tr-time-lock"
	case nestedP2WPKH:
		return "p2sh-p2wpkh"
	case timeLockP2WSH:
		return "p2wsh-time-lock"
	case multisig2of2P2WSH:
		return "p2wsh-multisig-2of2"
	case multisig3of3P2WSH:
		return "p2wsh-multisig-3of3"
	case nestedMultisig2of2P2WSH:
		return "p2sh-p2wsh-multisig-2of2"
	case legacyMultisig2of2P2SH:
		return "p2sh-multisig-2of2"
	default:
		return "unknown"
	}
}

func (i testInputType) output(t *testing.T,
	privKeys []*btcec.PrivateKey) (*wire.TxOut, string, []*btcutil.WIF,
	[]byte, []byte) {

	require.GreaterOrEqual(t, len(privKeys), 1)
	privKey := privKeys[0]
	pubKey := privKey.PubKey()
	_, revokeKey := btcec.PrivKeyFromBytes(testRevokePrivateKey)

	// We're simulating a delay-to-self script which we're going to spend
	// through the time lock path.
	timeLockScript, err := txscript.ScriptTemplate(
		timeLockTemplate, txscript.WithScriptTemplateParams(
			map[string]interface{}{
				"RevokeKey":  revokeKey.SerializeCompressed(),
				"CsvTimeout": int64(testCSVTimeout),
				"SelfKey":    pubKey.SerializeCompressed(),
			},
		),
	)
	require.NoError(t, err)

	taprootTimeLockScript, err := txscript.ScriptTemplate(
		timeLockTemplate, txscript.WithScriptTemplateParams(
			map[string]interface{}{
				"RevokeKey": schnorr.SerializePubKey(
					revokeKey,
				),
				"CsvTimeout": int64(testCSVTimeout),
				"SelfKey": schnorr.SerializePubKey(
					pubKey,
				),
			},
		),
	)
	require.NoError(t, err)

	privKeyWif, err := btcutil.NewWIF(privKey, netParams, true)
	require.NoError(t, err)

	var (
		addr          btcutil.Address
		witnessScript []byte
		sigScript     []byte
		signingKeys   = []*btcutil.WIF{privKeyWif}
	)
	switch i {
	case plainP2PKH:
		h := btcutil.Hash160(privKey.PubKey().SerializeCompressed())
		addr, err = btcutil.NewAddressPubKeyHash(h, netParams)
		require.NoError(t, err)

	case plainP2WPKH:
		h := btcutil.Hash160(privKey.PubKey().SerializeCompressed())
		addr, err = btcutil.NewAddressWitnessPubKeyHash(h, netParams)
		require.NoError(t, err)

	case plainP2TR:
		trKey := txscript.ComputeTaprootKeyNoScript(privKey.PubKey())
		addr, err = btcutil.NewAddressTaproot(
			schnorr.SerializePubKey(trKey), netParams,
		)
		require.NoError(t, err)

	case timeLockP2TR:
		witnessScript = taprootTimeLockScript
		leaf := txscript.NewBaseTapLeaf(witnessScript)
		tree := txscript.AssembleTaprootScriptTree(leaf)
		rootHash := tree.RootNode.TapHash()
		trKey := txscript.ComputeTaprootOutputKey(
			privKey.PubKey(), rootHash[:],
		)
		addr, err = btcutil.NewAddressTaproot(
			schnorr.SerializePubKey(trKey), netParams,
		)
		require.NoError(t, err)

	case nestedP2WPKH:
		h := btcutil.Hash160(privKey.PubKey().SerializeCompressed())
		witnessAddr, err := btcutil.NewAddressWitnessPubKeyHash(
			h, netParams,
		)
		require.NoError(t, err)

		witnessProgram, err := txscript.PayToAddrScript(witnessAddr)
		require.NoError(t, err)

		addr, err = btcutil.NewAddressScriptHash(
			witnessProgram, netParams,
		)
		require.NoError(t, err)
		sigScript = witnessProgram

	case timeLockP2WSH:
		witnessScript = timeLockScript

		h := sha256.Sum256(witnessScript)
		addr, err = btcutil.NewAddressWitnessScriptHash(h[:], netParams)
		require.NoError(t, err)

	case multisig2of2P2WSH:
		require.GreaterOrEqual(t, len(privKeys), 2)

		// We're simulating a 2-of-2 multisig.
		key2 := privKeys[1]
		params := map[string]interface{}{
			"FirstKey":  pubKey.SerializeCompressed(),
			"SecondKey": key2.PubKey().SerializeCompressed(),
		}
		witnessScript, err = txscript.ScriptTemplate(
			multiSig2of2Template, txscript.WithScriptTemplateParams(
				params,
			),
		)
		require.NoError(t, err)

		key2Wif, err := btcutil.NewWIF(key2, netParams, true)
		require.NoError(t, err)
		signingKeys = []*btcutil.WIF{privKeyWif, key2Wif}

		h := sha256.Sum256(witnessScript)
		addr, err = btcutil.NewAddressWitnessScriptHash(h[:], netParams)
		require.NoError(t, err)

	case multisig3of3P2WSH:
		require.Len(t, privKeys, 3)

		// We're simulating a 3-of-3 multisig.
		key2 := privKeys[1]
		key3 := privKeys[2]
		params := map[string]interface{}{
			"FirstKey":  pubKey.SerializeCompressed(),
			"SecondKey": key2.PubKey().SerializeCompressed(),
			"ThirdKey":  key3.PubKey().SerializeCompressed(),
		}
		witnessScript, err = txscript.ScriptTemplate(
			multiSig3of3Template, txscript.WithScriptTemplateParams(
				params,
			),
		)
		require.NoError(t, err)

		key2Wif, err := btcutil.NewWIF(key2, netParams, true)
		require.NoError(t, err)
		key3Wif, err := btcutil.NewWIF(key3, netParams, true)
		require.NoError(t, err)
		signingKeys = []*btcutil.WIF{
			privKeyWif, key2Wif, key3Wif,
		}

		h := sha256.Sum256(witnessScript)
		addr, err = btcutil.NewAddressWitnessScriptHash(h[:], netParams)
		require.NoError(t, err)

	case nestedMultisig2of2P2WSH:
		require.GreaterOrEqual(t, len(privKeys), 2)

		// We're simulating a 2-of-2 multisig wrapped in P2SH-P2WSH.
		key2 := privKeys[1]
		params := map[string]interface{}{
			"FirstKey":  pubKey.SerializeCompressed(),
			"SecondKey": key2.PubKey().SerializeCompressed(),
		}
		witnessScript, err = txscript.ScriptTemplate(
			multiSig2of2Template, txscript.WithScriptTemplateParams(
				params,
			),
		)
		require.NoError(t, err)

		key2Wif, err := btcutil.NewWIF(key2, netParams, true)
		require.NoError(t, err)
		signingKeys = []*btcutil.WIF{privKeyWif, key2Wif}

		// Create the P2WSH address, then wrap it in P2SH.
		h := sha256.Sum256(witnessScript)
		wshAddr, err := btcutil.NewAddressWitnessScriptHash(
			h[:], netParams,
		)
		require.NoError(t, err)

		wshScript, err := txscript.PayToAddrScript(wshAddr)
		require.NoError(t, err)

		addr, err = btcutil.NewAddressScriptHash(wshScript, netParams)
		require.NoError(t, err)
		sigScript = wshScript

	case legacyMultisig2of2P2SH:
		require.GreaterOrEqual(t, len(privKeys), 2)

		// We're simulating a pure legacy P2SH 2-of-2 multisig.
		key2 := privKeys[1]
		params := map[string]interface{}{
			"FirstKey":  pubKey.SerializeCompressed(),
			"SecondKey": key2.PubKey().SerializeCompressed(),
		}
		sigScript, err = txscript.ScriptTemplate(
			multiSig2of2Template, txscript.WithScriptTemplateParams(
				params,
			),
		)
		require.NoError(t, err)

		key2Wif, err := btcutil.NewWIF(key2, netParams, true)
		require.NoError(t, err)
		signingKeys = []*btcutil.WIF{privKeyWif, key2Wif}

		addr, err = btcutil.NewAddressScriptHash(sigScript, netParams)
		require.NoError(t, err)

	default:
		t.Fatalf("invalid input type")
	}

	pkScript, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)
	return &wire.TxOut{
		Value:    testValue,
		PkScript: pkScript,
	}, addr.String(), signingKeys, witnessScript, sigScript
}

func (i testInputType) decorateInput(t *testing.T, privKey *btcec.PrivateKey,
	in *psbt.PInput, witnessScript, sigScript []byte) {

	in.WitnessScript = witnessScript
	in.RedeemScript = sigScript

	switch i {
	case timeLockP2TR:
		leaf := txscript.NewBaseTapLeaf(witnessScript)
		tree := txscript.AssembleTaprootScriptTree(leaf)
		controlBlock := tree.LeafMerkleProofs[0].ToControlBlock(
			privKey.PubKey(),
		)
		controlBlockBytes, err := controlBlock.ToBytes()
		require.NoError(t, err)

		in.TaprootLeafScript = []*psbt.TaprootTapLeafScript{
			{
				ControlBlock: controlBlockBytes,
				Script:       witnessScript,
				LeafVersion:  leaf.LeafVersion,
			},
		}

	default:
		// Nothing to do for the other script types.
	}
}

func (i testInputType) sign(t *testing.T, idx int, signingKeys []*btcutil.WIF,
	packet *psbt.Packet) {

	in := &packet.Inputs[idx]
	txIn := packet.UnsignedTx.TxIn[idx]
	tx := packet.UnsignedTx
	utxo := in.WitnessUtxo
	if utxo == nil {
		targetOP := txIn.PreviousOutPoint
		if in.NonWitnessUtxo != nil {
			utxo = in.NonWitnessUtxo.TxOut[targetOP.Index]
		} else {
			numInputs := len(packet.UnsignedTx.TxIn)
			for lookupIdx := 1; lookupIdx < numInputs; lookupIdx++ {
				lookupInput := packet.Inputs[lookupIdx]
				prevTx := lookupInput.NonWitnessUtxo
				if prevTx == nil {
					continue
				}

				if prevTx.TxHash() != targetOP.Hash {
					continue
				}

				utxo = prevTx.TxOut[targetOP.Index]
			}
		}
	}

	switch i {
	case plainP2PKH:
		require.NoError(t, signInputLegacy(
			packet, idx, utxo.PkScript, signingKeys[0].PrivKey,
		))

	case plainP2WPKH, timeLockP2WSH, nestedP2WPKH:
		script := in.WitnessScript
		if i == plainP2WPKH {
			script = utxo.PkScript
		}
		if i == nestedP2WPKH {
			script = in.RedeemScript
		}

		require.NoError(t, signInputWitness(
			packet, idx, script, signingKeys[0].PrivKey,
		))

	case plainP2TR:
		require.NoError(t, signInputTaprootKeySpend(
			packet, idx, signingKeys[0].PrivKey,
		))

	case timeLockP2TR:
		prevOutFetcher := txscript.NewCannedPrevOutputFetcher(
			utxo.PkScript, utxo.Value,
		)
		sigHashes := txscript.NewTxSigHashes(tx, prevOutFetcher)

		leaf := txscript.NewBaseTapLeaf(in.WitnessScript)
		leafHash := leaf.TapHash()
		sig, err := txscript.RawTxInTapscriptSignature(
			tx, sigHashes, idx, utxo.Value, utxo.PkScript,
			leaf, txscript.SigHashDefault, signingKeys[0].PrivKey,
		)
		require.NoError(t, err)
		in.TaprootScriptSpendSig = []*psbt.TaprootScriptSpendSig{
			{
				XOnlyPubKey: schnorr.SerializePubKey(
					signingKeys[0].PrivKey.PubKey(),
				),
				LeafHash:  leafHash[:],
				Signature: sig,
				SigHash:   txscript.SigHashDefault,
			},
		}

	case multisig2of2P2WSH, multisig3of3P2WSH, nestedMultisig2of2P2WSH:
		for _, key := range signingKeys {
			require.NoError(t, signInputWitness(
				packet, idx, in.WitnessScript, key.PrivKey,
			))
		}

	case legacyMultisig2of2P2SH:
		for _, key := range signingKeys {
			require.NoError(t, signInputLegacy(
				packet, idx, in.RedeemScript, key.PrivKey,
			))
		}
	}
}

func (i testInputType) finalize(t *testing.T, packet *psbt.Packet) {
	in := &packet.Inputs[0]

	var witnessStack wire.TxWitness
	switch i {
	case timeLockP2TR:
		script := in.TaprootLeafScript[0]
		scriptSpend := in.TaprootScriptSpendSig[0]

		witnessStack = make([][]byte, 4)
		witnessStack[0] = scriptSpend.Signature
		witnessStack[1] = nil
		witnessStack[2] = in.WitnessScript
		witnessStack[3] = script.ControlBlock

	case timeLockP2WSH:
		sigBytes := in.PartialSigs[0].Signature

		witnessStack = make([][]byte, 3)
		witnessStack[0] = sigBytes
		witnessStack[1] = nil
		witnessStack[2] = in.WitnessScript

	case legacyMultisig2of2P2SH:
		// Pure legacy P2SH multisig requires manual finalization.
		// scriptSig: OP_0 <sig1> <sig2> <redeemScript>
		builder := txscript.NewScriptBuilder()
		builder.AddOp(txscript.OP_0)
		for _, partialSig := range in.PartialSigs {
			builder.AddData(partialSig.Signature)
		}
		builder.AddData(in.RedeemScript)

		var err error
		in.FinalScriptSig, err = builder.Script()
		require.NoError(t, err)

		return

	default:
		// The PSBT finalizer knows what to do if we're not using a
		// custom script.
		err := psbt.MaybeFinalizeAll(packet)
		require.NoError(t, err)

		return
	}

	var err error
	in.FinalScriptWitness, err = SerializeTxWitness(witnessStack)
	require.NoError(t, err)
}

// deduplicateNonWitnessUtxos deduplicates a PSBT packet's input NonSegwitUTXO
// fields for the special case allowed in BIP-322 for Proof of Funds packets.
func deduplicateNonWitnessUtxos(packet *psbt.Packet) {
	for idx := range packet.Inputs {
		if idx == 0 || packet.Inputs[idx].NonWitnessUtxo == nil {
			continue
		}

		currentTx := packet.Inputs[idx].NonWitnessUtxo
		for inner := 0; inner < idx; inner++ {
			if packet.Inputs[inner].NonWitnessUtxo == nil {
				continue
			}

			prevTx := packet.Inputs[inner].NonWitnessUtxo
			if prevTx.TxHash() == currentTx.TxHash() {
				// We already have this TX in a previous input,
				// so we can deduplicate it.
				packet.Inputs[idx].NonWitnessUtxo = nil
				break
			}
		}
	}
}

// TestSignAndVerifySimpleScriptTypes tests the full BIP-322 signing and
// verification flow end-to-end for the script types compatible with the
// "simple" variant.
func TestSignAndVerifySimpleScriptTypes(t *testing.T) {
	for _, inputType := range nativeSegWitTypes {
		t.Run(inputType.String(), func(t *testing.T) {
			_ = runTestCaseSimple(t, inputType)
		})
	}
}

// TestSignAndVerifyFullScriptTypes tests the full BIP-322 signing and
// verification flow end-to-end for the "full" variant.
func TestSignAndVerifyFullScriptTypes(t *testing.T) {
	for _, inputType := range allTypes {
		t.Run(inputType.String(), func(t *testing.T) {
			_ = runTestCaseFull(t, inputType)
		})
	}
}

// TestSignAndVerifyPoFScriptTypes tests the BIP-322 signing and verification
// flow end-to-end for the "Proof of Fund" variant.
func TestSignAndVerifyPoFScriptTypes(t *testing.T) {
	for _, tc := range pofTestCases {
		t.Run(tc.name, func(t *testing.T) {
			_ = runTestCasePoF(
				t, tc.challengeInputType, tc.pofInputTypes,
			)
		})
	}
}

// TestIncompatibleSimpleScriptTypes tests that the "simple" variant cannot be
// used with legacy script types.
func TestIncompatibleSimpleScriptTypes(t *testing.T) {
	for _, inputType := range legacyTypes {
		t.Run(inputType.String(), func(t *testing.T) {
			message := []byte(rand.Text())

			// This is the private key we're going to sign with.
			privKey, err := btcec.NewPrivateKey()
			require.NoError(t, err)

			// Extra keys for multisig types.
			privKey2, err := btcec.NewPrivateKey()
			require.NoError(t, err)

			txOut, _, _, _, _ := inputType.output(
				t, []*btcec.PrivateKey{privKey, privKey2},
			)
			_, err = BuildToSignPacketSimple(
				message, txOut.PkScript,
			)
			require.ErrorIs(t, err, errSimpleSegWitOnly)
		})
	}
}

// runTestCaseSimple runs a test case for the "simple" variant of BIP-322.
func runTestCaseSimple(t *testing.T,
	inputType testInputType) simpleSignatureVector {

	message := []byte(rand.Text())

	// This is the private key we're going to sign with.
	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	// And two more random keys for the multisig test cases.
	privKey2, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	privKey3, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	txOut, addr, wifs, witnessScript, sigScript := inputType.output(
		t, []*btcec.PrivateKey{privKey, privKey2, privKey3},
	)
	toSign, err := BuildToSignPacketSimple(message, txOut.PkScript)
	require.NoError(t, err)

	// Add the necessary input values.
	inputType.decorateInput(
		t, privKey, &toSign.Inputs[0], witnessScript, sigScript,
	)

	// Now sign the input.
	inputType.sign(t, 0, wifs, toSign)

	// If the witness stack needs to be assembled, give the caller
	// the option to do that now.
	inputType.finalize(t, toSign)

	finalTx, err := psbt.Extract(toSign)
	require.NoError(t, err)

	valid, _, err := VerifyMessageSimple(
		message, txOut.PkScript, finalTx.TxIn[0].Witness,
	)
	require.NoError(t, err)
	require.True(t, valid)

	signature, err := SerializeSignature(toSign)
	require.NoError(t, err)

	// Make sure the signature is valid.
	parsedAddr, err := btcutil.DecodeAddress(addr, netParams)
	require.NoError(t, err)
	valid, constraints, err := VerifyMessage(
		string(message), parsedAddr, signature,
	)
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, TimeConstraints{}, constraints)

	return simpleSignatureVector{
		Message:       string(message),
		PrivateKeys:   formatWIFs(wifs),
		Address:       addr,
		Type:          inputType.String(),
		WitnessScript: hexEncode(witnessScript),
		Bip322Signatures: []string{
			signature,
		},
	}
}

// runTestCaseFull runs a test case for the "full" variant of BIP-322.
func runTestCaseFull(t *testing.T,
	inputType testInputType) fullSignatureVector {

	message := []byte(rand.Text())

	// This is the private key we're going to sign with.
	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	// And two more random keys for the multisig test cases.
	privKey2, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	privKey3, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	txOut, addr, wifs, witnessScript, sigScript := inputType.output(
		t, []*btcec.PrivateKey{privKey, privKey2, privKey3},
	)

	// The transaction types that don't have any custom script won't
	// care about the lock time and sequence, so we just always set
	// it.
	toSign := BuildToSignPacketFull(
		message, txOut.PkScript, 2, testCSVTimeout,
		testCSVTimeout,
	)

	// Add the necessary input values.
	inputType.decorateInput(
		t, privKey, &toSign.Inputs[0], witnessScript, sigScript,
	)

	// Now sign the input.
	inputType.sign(t, 0, wifs, toSign)

	// If the witness stack needs to be assembled, give the caller
	// the option to do that now.
	inputType.finalize(t, toSign)

	finalTx, err := psbt.Extract(toSign)
	require.NoError(t, err)

	valid, _, err := VerifyMessageFull(
		message, txOut.PkScript,
		finalTx.TxIn[0].SignatureScript,
		finalTx.TxIn[0].Witness, 2, testCSVTimeout,
		testCSVTimeout,
	)
	require.NoError(t, err)
	require.True(t, valid)

	signature, err := SerializeSignature(toSign)
	require.NoError(t, err)

	// Make sure the signature is valid.
	parsedAddr, err := btcutil.DecodeAddress(addr, netParams)
	require.NoError(t, err)
	valid, constraints, err := VerifyMessage(
		string(message), parsedAddr, signature,
	)
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(
		t, TimeConstraints{true, testCSVTimeout, testCSVTimeout},
		constraints,
	)

	return fullSignatureVector{
		simpleSignatureVector: simpleSignatureVector{
			Message:       string(message),
			PrivateKeys:   formatWIFs(wifs),
			Address:       addr,
			Type:          inputType.String(),
			WitnessScript: hexEncode(witnessScript),
			Bip322Signatures: []string{
				signature,
			},
		},
		SigScript: hexEncode(sigScript),
		TxVersion: 2,
		LockTime:  testCSVTimeout,
		Sequence:  testCSVTimeout,
	}
}

// runTestCasePoF runs a test case for the "proof of funds" variant of BIP-322.
func runTestCasePoF(t *testing.T, challengeInputType testInputType,
	pofInputTypes [][]testInputType) pofSignatureVector {

	message := []byte(rand.Text())

	challengePrivateKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	challengeWif, err := btcutil.NewWIF(
		challengePrivateKey, netParams, true,
	)
	require.NoError(t, err)

	allWifs := []*btcutil.WIF{challengeWif}

	type pofInput struct {
		typ         testInputType
		prevTx      *wire.MsgTx
		prevTxIndex int
		txOut       *wire.TxOut
		pInput      psbt.PInput
		privateKey  *btcec.PrivateKey
	}

	// We start by creating enough private keys. We need a separate key for
	// the challenge input and then one each per POF input.
	keyIndex := 1
	additionalInputs := make([][]additionalInput, len(pofInputTypes))
	var pofInputs []pofInput
	for idx, inputTypes := range pofInputTypes {
		additionalInputs[idx] = make([]additionalInput, len(inputTypes))

		prevTx := &wire.MsgTx{
			TxIn: []*wire.TxIn{{
				Witness: wire.TxWitness{},
			}},
			TxOut: make([]*wire.TxOut, len(inputTypes)),
		}
		for j, inputType := range inputTypes {
			privKey, err := btcec.NewPrivateKey()
			require.NoError(t, err)

			txOut, _, wifs, witnessScript, sigScript :=
				inputType.output(
					t, []*btcec.PrivateKey{privKey},
				)

			prevTx.TxOut[j] = txOut
			additionalInputs[idx][j] = additionalInput{
				PrivateKeyIndex: keyIndex,
				Type:            inputType.String(),
				Value:           txOut.Value,
				PkScript:        hexEncode(txOut.PkScript),
			}
			allWifs = append(allWifs, wifs...)

			pInput := psbt.PInput{
				RedeemScript:  sigScript,
				WitnessScript: witnessScript,
			}

			if isNativeSegWitPkScript(txOut.PkScript) ||
				inputType == nestedP2WPKH {

				pInput.WitnessUtxo = txOut
			} else {
				pInput.NonWitnessUtxo = prevTx
			}

			pofInputs = append(pofInputs, pofInput{
				typ:         inputType,
				prevTx:      prevTx,
				prevTxIndex: j,
				txOut:       txOut,
				pInput:      pInput,
				privateKey:  privKey,
			})

			keyIndex++
		}
	}

	// We assume that all input types are single-sig types for this test
	// case. We start by assembling the base to_sign transaction.
	txOut, addr, _, witnessScript, sigScript := challengeInputType.output(
		t, []*btcec.PrivateKey{challengePrivateKey},
	)
	toSign := BuildToSignPacketFull(message, txOut.PkScript, 2, 123, 456)

	// Now we can add the additional inputs to the transaction.
	for _, pofInput := range pofInputs {
		toSign.UnsignedTx.TxIn = append(
			toSign.UnsignedTx.TxIn, &wire.TxIn{
				PreviousOutPoint: wire.OutPoint{
					Hash:  pofInput.prevTx.TxHash(),
					Index: uint32(pofInput.prevTxIndex),
				},
			},
		)
		toSign.Inputs = append(toSign.Inputs, pofInput.pInput)
	}

	// Now that we've assembled the full transaction, we can start to sign.
	challengeInputType.sign(t, 0, []*btcutil.WIF{challengeWif}, toSign)
	for idx, pofInput := range pofInputs {
		wif, err := btcutil.NewWIF(pofInput.privateKey, netParams, true)
		require.NoError(t, err)
		pofInput.typ.sign(t, idx+1, []*btcutil.WIF{wif}, toSign)
	}

	err = psbt.MaybeFinalizeAll(toSign)
	require.NoError(t, err)

	// For our test vectors, we want to show the special case for
	// deduplicating NonWitnessUTXOs.
	deduplicateNonWitnessUtxos(toSign)

	signature, err := SerializeSignature(toSign)
	require.NoError(t, err)

	// Make sure the signature is valid.
	parsedAddr, err := btcutil.DecodeAddress(addr, netParams)
	require.NoError(t, err)
	valid, constraints, err := VerifyMessage(
		string(message), parsedAddr, signature,
	)
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, TimeConstraints{true, 123, 456}, constraints)

	return pofSignatureVector{
		fullSignatureVector: fullSignatureVector{
			simpleSignatureVector: simpleSignatureVector{
				Message:       string(message),
				PrivateKeys:   formatWIFs(allWifs),
				Address:       addr,
				Type:          challengeInputType.String(),
				WitnessScript: hexEncode(witnessScript),
				Bip322Signatures: []string{
					signature,
				},
			},
			SigScript: hexEncode(sigScript),
			TxVersion: 2,
			LockTime:  123,
			Sequence:  456,
		},
		AdditionalInputs: additionalInputs,
	}
}

// randomAddress generates a random address for the given input type.
func randomAddress(t *testing.T, inputType testInputType) string {
	t.Helper()

	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	privKey2, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	privKey3, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	_, addr, _, _, _ := inputType.output(
		t, []*btcec.PrivateKey{privKey, privKey2, privKey3},
	)
	return addr
}

// TestUpdateTestVectors updates the test vectors in the testdata folder.
func TestUpdateTestVectors(t *testing.T) {
	// Comment this out to update the test vectors with new random values
	// (not deterministic, will change all values)!
	t.Skipf("Skipping test vectors update by default")

	var vectors testVectors
	for _, inputType := range nativeSegWitTypes {
		vectors.Simple = append(
			vectors.Simple, runTestCaseSimple(t, inputType),
		)
	}
	for _, inputType := range allTypes {
		vectors.Full = append(
			vectors.Full, runTestCaseFull(t, inputType),
		)
	}
	for _, tc := range pofTestCases {
		vectors.ProofOfFunds = append(
			vectors.ProofOfFunds, runTestCasePoF(
				t, tc.challengeInputType, tc.pofInputTypes,
			),
		)
	}

	// Generate error vectors from the simple variants.
	for i, tc := range vectors.Simple {
		inputType := nativeSegWitTypes[i]

		// Wrong message: valid signature verified against a
		// different message.
		vectors.Error = append(vectors.Error, errorVector{
			Description: fmt.Sprintf(
				"wrong message for %s simple signature",
				inputType,
			),
			Message:     rand.Text(),
			Address:     tc.Address,
			Signature:   tc.Bip322Signatures[0],
			ErrorSubstr: "invalid signature",
		})

		// Wrong signer: valid signature verified against a
		// different address of the same type.
		vectors.Error = append(vectors.Error, errorVector{
			Description: fmt.Sprintf(
				"wrong signer for %s simple signature",
				inputType,
			),
			Message:     tc.Message,
			Address:     randomAddress(t, inputType),
			Signature:   tc.Bip322Signatures[0],
			ErrorSubstr: "invalid signature",
		})
	}

	// Generate error vectors from the full variants.
	for i, tc := range vectors.Full {
		inputType := allTypes[i]

		// Wrong message: valid signature verified against a
		// different message.
		vectors.Error = append(vectors.Error, errorVector{
			Description: fmt.Sprintf(
				"wrong message for %s full signature",
				inputType,
			),
			Message:     rand.Text(),
			Address:     tc.Address,
			Signature:   tc.Bip322Signatures[0],
			ErrorSubstr: "invalid signature",
		})

		// Wrong signer: valid signature verified against a
		// different address of the same type.
		vectors.Error = append(vectors.Error, errorVector{
			Description: fmt.Sprintf(
				"wrong signer for %s full signature",
				inputType,
			),
			Message:     tc.Message,
			Address:     randomAddress(t, inputType),
			Signature:   tc.Bip322Signatures[0],
			ErrorSubstr: "invalid signature",
		})
	}

	storeTestVectors(t, &vectors, "generated-test-vectors.json")
}
