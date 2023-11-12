package miniscript

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestSplitString tests the splitString function.
func TestSplitString(t *testing.T) {
	separators := func(c rune) bool {
		return c == '(' || c == ')' || c == ','
	}

	testCases := []struct {
		str      string
		expected []string
	}{
		{
			str:      "",
			expected: []string{},
		},
		{
			str:      "0",
			expected: []string{"0"},
		},
		{
			str:      "0)(1(",
			expected: []string{"0", ")", "(", "1", "("},
		},
		{
			str: "or_b(pk(key_1),s:pk(key_2))",
			expected: []string{
				"or_b", "(", "pk", "(", "key_1", ")", ",",
				"s:pk", "(", "key_2", ")", ")",
			},
		},
	}

	for _, tc := range testCases {
		require.Equal(t, tc.expected, splitString(tc.str, separators))
	}
}

// checkMiniscript makes sure the passed miniscript is top level, has the
// expected type and script length.
func checkMiniscript(miniscript, expectedType string, opCodes int) error {
	node, err := Parse(miniscript)
	if err := node.IsValidTopLevel(); err != nil {
		return err
	}
	sortString := func(s string) string {
		r := []rune(s)
		sort.Slice(r, func(i, j int) bool {
			return r[i] < r[j]
		})
		return string(r)
	}
	if sortString(expectedType) != sortString(node.formattedType()) {
		return fmt.Errorf("expected type %s, got %s",
			sortString(expectedType),
			sortString(node.formattedType()))
	}

	err = node.ApplyVars(func(identifier string) ([]byte, error) {
		if len(identifier) == 64 {
			return nil, nil
		}

		// Return an arbitrary unique 33 bytes.
		return append(
			chainhash.HashB([]byte(identifier)), 0,
		), nil
	})
	if err != nil {
		return err
	}

	script, err := node.Script()
	if err != nil {
		return err
	}

	if len(script) != node.scriptLen {
		return fmt.Errorf("expected script length %d but got %d for "+
			"script %s", node.scriptLen, len(script),
			node.DrawTree())
	}

	if opCodes != 0 && opCodes != node.maxOpCount() {
		return fmt.Errorf("expected %d opcodes but got %d for "+
			"miniscript %s", opCodes, node.maxOpCount(),
			miniscript)
	}

	return nil
}

// TestVectors asserts all test vectors in the test data text files pass.
func TestVectors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		fileName    string
		valid       bool
		withOpCodes bool
	}{
		{
			// Invalid expressions (failed type check).
			fileName: "testdata/invalid_from_alloy.txt",
			valid:    false,
		},
		{
			// Valid miniscript expressions including the expected
			// type.
			fileName: "testdata/valid_8f1e8_from_alloy.txt",
			valid:    true,
		},
		{
			// Valid miniscript expressions including the expected
			// type.
			fileName: "testdata/valid_from_alloy.txt",
			valid:    true,
		},
		{
			// Valid expressions but do not contain the `m` type
			// property, i.e. the script is guaranteed to have a
			// non-malleable satisfaction.
			fileName: "testdata/malleable_from_alloy.txt",
			valid:    true,
		},
		{
			// miniscripts with time lock mixing in `after` (same
			// expression contains both time-based and block-based
			// time locks). This unit test is not testing this
			// currently, see
			// https://github.com/rust-bitcoin/rust-miniscript/issues/514.
			fileName: "testdata/conflict_from_alloy.txt",
			valid:    true,
		},
		{
			// miniscripts with number of opcodes.
			fileName:    "testdata/opcodes.txt",
			valid:       true,
			withOpCodes: true,
		},
	}

	for _, tc := range testCases {
		content, err := os.ReadFile(tc.fileName)
		require.NoError(t, err)

		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if line == "" {
				continue
			}

			if !tc.valid {
				_, err := Parse(line)
				require.Errorf(
					t, err, "failure on line %d: %s", i,
					line,
				)

				continue
			}

			parts := strings.Split(line, " ")

			var opCodes int
			if tc.withOpCodes {
				require.Lenf(
					t, parts, 3, "malformed test on line "+
						"%d: %s", i, line,
				)
				opCodes, err = strconv.Atoi(parts[2])
				require.NoError(t, err)
			} else {
				require.Lenf(
					t, parts, 2, "malformed test on line "+
						"%d: %s", i, line,
				)
			}

			miniscript, expectedType := parts[0], parts[1]
			require.NoError(
				t, checkMiniscript(
					miniscript, expectedType, opCodes,
				), "failure on line %d: %s", i, line,
			)
		}
	}
}

type testSignFn func(pubKey []byte, hash []byte) (signature []byte,
	available bool)

func testRedeem(t *testing.T, miniscript string,
	lookupVar func(identifier string) ([]byte, error), sequence uint32,
	sign testSignFn, preimage PreimageFunc) error {

	// We construct a p2wsh(<miniscript>) UTXO, which we will spend with a
	// satisfaction generated from the miniscript.
	node, err := Parse(miniscript)
	if err != nil {
		return err
	}
	err = node.IsSane()
	if err != nil {
		return err
	}
	err = node.ApplyVars(lookupVar)
	if err != nil {
		return err
	}
	t.Logf("Tree for miniscript %v: %v", miniscript, node.DrawTree())
	t.Logf("Max op count: %v (%d + %d)", node.maxOpCount(),
		node.opCount.count, node.opCount.sat.value)

	t.Logf("Script: %v", scriptStr(node, false))

	// Create the script.
	witnessScript, err := node.Script()
	if err != nil {
		return err
	}

	// Create the p2wsh(<script>) UTXO.
	addr, err := btcutil.NewAddressWitnessScriptHash(
		chainhash.HashB(witnessScript), &chaincfg.TestNet3Params,
	)
	if err != nil {
		return err
	}

	utxoAmount := int64(999799)
	if err != nil {
		return err
	}
	utxoPkScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return err
	}

	// Our test spend is a 1-input 1-output transaction. The input spends
	// the miniscript UTXO. The output is an arbitrary output - we use a
	// OP_RETURN burn output.
	burnPkScript, err := txscript.NullDataScript(nil)
	if err != nil {
		return err
	}

	// Dummy prevout hash.
	hash, err := chainhash.NewHashFromStr(
		"000000000000000000000000000000000000000000000000000000000000" +
			"0000",
	)
	if err != nil {
		return err
	}
	txInput := wire.NewTxIn(&wire.OutPoint{Hash: *hash}, nil, nil)
	txInput.Sequence = sequence

	transaction := wire.MsgTx{
		Version: 2,
		TxIn:    []*wire.TxIn{txInput},
		TxOut: []*wire.TxOut{{
			Value:    utxoAmount - 200,
			PkScript: burnPkScript,
		}},
		LockTime: 0,
	}

	// We only have one input, for which we will execute the script.
	inputIndex := 0

	// We only have one input, so the previous outputs fetcher for the
	// transaction simply returns our UTXO. The previous output is needed as
	// it is signed as part of the transaction sighash for the input.
	previousOutputs := txscript.NewCannedPrevOutputFetcher(
		utxoPkScript, utxoAmount,
	)

	// Compute the signature hash to be signed for the first input:
	sigHashes := txscript.NewTxSigHashes(&transaction, previousOutputs)
	signatureHash, err := txscript.CalcWitnessSigHash(
		witnessScript, sigHashes, txscript.SigHashAll, &transaction,
		inputIndex, utxoAmount,
	)
	if err != nil {
		return err
	}

	// Construct a satisfaction (witness) from the miniscript.
	witness, err := node.Satisfy(&Satisfier{
		CheckOlder: func(lockTime uint32) (bool, error) {
			return CheckOlder(
				lockTime, uint32(transaction.Version),
				transaction.TxIn[inputIndex].Sequence,
			), nil
		},
		CheckAfter: func(lockTime uint32) (bool, error) {
			return CheckAfter(
				lockTime, transaction.LockTime,
				transaction.TxIn[inputIndex].Sequence,
			), nil
		},
		Sign: func(pubKey []byte) ([]byte, bool) {
			signature, available := sign(pubKey, signatureHash)
			if !available {
				return nil, false
			}
			signature = append(signature, byte(txscript.SigHashAll))
			return signature, true
		},
		Preimage: preimage,
	})
	if err != nil {
		return err
	}

	// Put the created witness into the transaction input, then execute the
	// script to test that the UTXO can be spent successfully.
	transaction.TxIn[inputIndex].Witness = append(witness, witnessScript)
	engine, err := txscript.NewEngine(
		utxoPkScript, &transaction, inputIndex,
		txscript.StandardVerifyFlags, nil, sigHashes, utxoAmount,
		previousOutputs,
	)
	if err != nil {
		return err
	}
	err = engine.Execute()
	if err != nil {
		return err
	}

	var rawTx bytes.Buffer
	err = transaction.Serialize(&rawTx)
	require.NoError(t, err)
	t.Logf("Raw witness: %v", witness.ToHexStrings())
	t.Logf("Raw transaction: %x", rawTx.Bytes())
	return nil
}

type RedeemTestVectors struct {
	Identifiers map[string]string `json:"identifiers"`
	TestCases   []RedeemTestCase  `json:"test_cases"`
}

type RedeemTestCase struct {
	Miniscript        string `json:"miniscript"`
	ScriptDescription string `json:"script_description,omitempty"`
	Comment           string `json:"comment"`
	Valid             bool   `json:"valid"`
	Sequence          uint32 `json:"sequence,omitempty"`
	CanSign1          bool   `json:"can_sign_1,omitempty"`
	CanSign2          bool   `json:"can_sign_2,omitempty"`
	CanSign3          bool   `json:"can_sign_3,omitempty"`
	HasPreimage       bool   `json:"has_preimage,omitempty"`
}

// TestRedeem tests that the script generated from a miniscript can be spent
// successfully.
func TestRedeem(t *testing.T) {
	t.Parallel()

	fileBytes, err := os.ReadFile(filepath.Join("testdata", "redeem.json"))
	require.NoError(t, err)

	vec := &RedeemTestVectors{}
	err = json.Unmarshal(fileBytes, vec)
	require.NoError(t, err)

	unHex := func(s string) []byte {
		b, err := hex.DecodeString(s)
		require.NoError(t, err)
		return b
	}

	lookupVar := func(identifier string) ([]byte, error) {
		return unHex(vec.Identifiers[identifier]), nil
	}

	sign := func(canSign1, canSign2, canSign3 bool) testSignFn {
		privKey1Bytes := unHex(vec.Identifiers["pk_1"])
		privKey2Bytes := unHex(vec.Identifiers["pk_2"])
		privKey3Bytes := unHex(vec.Identifiers["pk_3"])
		privKey1, pubKey1 := btcec.PrivKeyFromBytes(privKey1Bytes)
		privKey2, pubKey2 := btcec.PrivKeyFromBytes(privKey2Bytes)
		privKey3, pubKey3 := btcec.PrivKeyFromBytes(privKey3Bytes)
		return func(pk []byte, hash []byte) ([]byte, bool) {
			isPk1 := bytes.Equal(pk, pubKey1.SerializeCompressed())
			isPk2 := bytes.Equal(pk, pubKey2.SerializeCompressed())
			isPk3 := bytes.Equal(pk, pubKey3.SerializeCompressed())
			if canSign1 && isPk1 {
				return ecdsa.Sign(privKey1, hash).Serialize(),
					true
			}

			if canSign2 && isPk2 {
				return ecdsa.Sign(privKey2, hash).Serialize(),
					true
			}

			if canSign3 && isPk3 {
				return ecdsa.Sign(privKey3, hash).Serialize(),
					true
			}

			return nil, false
		}
	}

	preimage := func(hasPreimage bool) PreimageFunc {
		preimage := unHex(vec.Identifiers["preimage_1"])
		return func(hashFunc string, hash []byte) ([]byte, bool) {
			if !hasPreimage {
				return nil, false
			}

			switch hashFunc {
			case "ripemd160":
				h := btcutil.Hash160(preimage)
				return preimage, bytes.Equal(hash, h)

			case "sha256":
				h := chainhash.HashB(preimage)
				return preimage, bytes.Equal(hash, h)
			}

			return nil, false
		}
	}

	for _, tc := range vec.TestCases {
		t.Logf("-----------------------------------")
		t.Logf("Test case: %s", tc.Comment)
		t.Logf("-----------------------------------")
		vec.TestCases = append(vec.TestCases, tc)

		err := testRedeem(
			t, tc.Miniscript, lookupVar, tc.Sequence,
			sign(tc.CanSign1, tc.CanSign2, tc.CanSign3),
			preimage(tc.HasPreimage),
		)

		t.Logf("\n\n")

		if !tc.Valid {
			require.Errorf(
				t, err, "comment: %s, miniscript: %s",
				tc.Comment, tc.Miniscript,
			)

			continue
		}

		require.NoErrorf(
			t, err, "comment: %s, miniscript: %s", tc.Comment,
			tc.Miniscript,
		)
	}
}

// TestComputeOpCount tests that the maxOpCount function returns the correct
// number of operations.
func TestComputeOpCount(t *testing.T) {
	testCases := []struct {
		script     string
		maxOpCount int
	}{
		{
			script: "or_i(multi(2,key1,key2,key3)," +
				"multi(3,key4,key5,key6,key7))",
			maxOpCount: 9,
		},
		{
			script: "thresh(2,or_i(multi(2,key1,key2,key3)," +
				"multi(3,key4,key5,key6,key7))," +
				"s:pk(key8),s:pk(key9))",
			maxOpCount: 16,
		},
		{
			script: "thresh(2,or_d(multi(2,key1,key2,key3)," +
				"multi(3,key4,key5,key6,key7))," +
				"s:pk(key8),s:pk(key9))",
			maxOpCount: 19,
		},
	}

	for _, tc := range testCases {
		node, err := Parse(tc.script)
		require.NoError(t, err)
		require.Equal(t, tc.maxOpCount, node.maxOpCount())
	}
}
