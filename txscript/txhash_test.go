package txscript

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestCalculateTxHash tests the calculation of the tx hash for a given
// transaction, prevouts and tx field selector.
func TestCalculateTxHash(t *testing.T) {
	// First read the test vectors from the file.
	testVectors := parseTestVectorsFromFile(t, "./data/txhash_vectors.json")

	// Create a new transaction from the hex string.
	tx := wire.NewMsgTx(wire.TxVersion)
	err := tx.Deserialize(bytes.NewReader(
		decodeHex(t, testVectors.Tx),
	))
	require.NoError(t, err)

	// Create the prevout map for the prevout fetcher.
	prevOuts := make([]*wire.TxOut, len(testVectors.Prevs))
	prevOutMap := make(map[wire.OutPoint]*wire.TxOut)
	for idx, prev := range testVectors.Prevs {
		txOut := wire.NewTxOut(0, nil)

		r := bytes.NewReader(decodeHex(t, prev))

		err = wire.ReadTxOut(r, 0, wire.TxVersion, txOut)
		require.NoError(t, err)

		prevOuts[idx] = txOut

		prevOutMap[tx.TxIn[idx].PreviousOutPoint] = txOut
	}

	prevOutFetcher := NewMultiPrevOutFetcher(prevOutMap)

	// Run through the test vectors and make sure the tx hash matches.
	for testIdx, testVector := range testVectors.Vectors {
		txfs := decodeHex(t, testVector.Txfs)

		// Create the TxFieldSelector from the bytes.
		TxFields, err := NewTxFieldSelectorFromBytes(txfs)
		require.NoError(t, err)

		// Calculate the tx hash.
		txHash, err := TxFields.GetTxHash(
			tx, testVector.Input, prevOutFetcher, 0,
		)
		require.NoError(
			t, err, fmt.Sprintf("failed at test vector %d"+
				" expected hash %s", testIdx, testVector.TxHash),
		)

		// Make sure the tx hash matches the expected.
		require.Equal(
			t, testVector.TxHash, fmt.Sprintf("%x", txHash),
			fmt.Sprintf("failed at test vector %d", testIdx),
		)

		// Now we'll serialize the TxFieldSelector and make sure it
		// matches the original.
		txfs2, err := TxFields.ToBytes()
		require.NoError(t, err, "failed to serialize txfs")

		// We'll need to handle the special cases where the expected is
		// either empty or the special template selector.
		compareTxfs(t, txfs, txfs2)
	}
	t.Logf("passed %d test vectors", len(testVectors.Vectors))
}

// compareTxfs compares two txfs byte slices. It handles the special cases
// where the expected is either empty or the special template selector.
func compareTxfs(t *testing.T, expected, actual []byte) {
	t.Helper()

	// Check if the expected is either empty or the special template
	// selector.
	if len(expected) == 0 {
		if bytes.Equal(actual, TXFSSpecialTemplate[:]) {
			actual = []byte{}
		}
	} else if len(expected) == 1 && expected[0] == 0x00 {
		if bytes.Equal(actual, TXFSSpecialAll[:]) {
			actual = []byte{0x00}
		}
	}

	require.Equal(t, expected, actual)
}

// This example demonstrates creating a transaction that spends to a
// OP_TXHASHVERIFY output that constraints the transaction to a template
// commiting to 2 outputs along with their values.
func TestExampleTx(t *testing.T) {
	var (
		txValue  = int64(5000)
		feeValue = int64(1000)
	)

	// Get 2 addresses we will restrict the output to be spent to.
	addr1 := getAddress(t)
	pkscript1, err := PayToAddrScript(addr1)
	require.NoError(t, err)

	addr2 := getAddress(t)
	pkscript2, err := PayToAddrScript(addr2)
	require.NoError(t, err)

	// Create the tx template we're commiting to.
	txTemplate := wire.NewMsgTx(wire.TxVersion)
	txTemplate.AddTxOut(&wire.TxOut{
		Value:    txValue,
		PkScript: pkscript1,
	})
	txTemplate.AddTxOut(&wire.TxOut{
		Value:    txValue,
		PkScript: pkscript2,
	})

	// We'll create the TxFieldSelector that will be used to generate and
	// verify the OP_TXHASHVERIFY output. We'll fully commit to the
	// output number, scriptPubkey and value.
	outputSelector := &OutputSelector{
		CommitNumber:  true,
		ScriptPubkeys: true,
		Values:        true,

		InOutSelector: &InOutSelectorAll{},
	}

	txFieldSelector := TxFieldSelector{
		Control: true,
		Version: true,
		Outputs: outputSelector,
	}
	txfs, err := txFieldSelector.ToBytes()
	require.NoError(t, err)

	// We'll now create the transaction that will spend to the
	// OP_TXHASHVERIFY output.
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add a fake input to the transaction.
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  sha256.Sum256([]byte{1, 2, 3}),
			Index: 0,
		}})

	// Now we'll create the TxHash that the OP_TXHASHVERIFY output will
	// require.
	txHash, err := txFieldSelector.GetTxHash(txTemplate, 0, nil, 0)
	require.NoError(t, err)

	txHashScript, err := genTxHashScript(txHash, txfs)
	require.NoError(t, err)

	// This prevout fetcher will ensure that the engine checks against
	// the txHashScript we created above.
	prevoutFetcher := NewCannedPrevOutputFetcher(
		txHashScript, txValue*2+feeValue,
	)

	// Add the outputs as defined in the template
	for _, txOut := range txTemplate.TxOut {
		tx.AddTxOut(txOut)
	}

	sigHashes := NewTxSigHashes(tx, prevoutFetcher)

	// Now we can define some test cases that we will test.
	testCases := []struct {
		// The flags we want the engine to run with.
		verifyFlags ScriptFlags
		// The expected error we should get.
		expectedErrCode ErrorCode
		// The value of the first output.
		modifiedValueTxOut1 int64
	}{

		{
			// This tests the case where we have the
			// ScriptVerifyTxHash flag set.
			verifyFlags: StandardVerifyFlags | ScriptVerifyTxHash,
		},
		{
			// We'll modify the value of the first output to
			// generate a different tx hash and thus fail the
			// opcode verification.
			verifyFlags:         StandardVerifyFlags | ScriptVerifyTxHash,
			expectedErrCode:     ErrCheckTxHashVerify,
			modifiedValueTxOut1: txValue - 1,
		},
		{
			// This tests the case where we don't have the
			// ScriptVerifyTxHash flag set, the opcode should
			// succeed.
			verifyFlags:         StandardVerifyFlags,
			modifiedValueTxOut1: txValue - 1,
		},
	}

	for _, testCase := range testCases {
		// We'll modify the value of the first output if needed.
		if testCase.modifiedValueTxOut1 != 0 {
			tx.TxOut[0].Value = testCase.modifiedValueTxOut1
		}

		// Create a new engine with the ScriptVerifyTxHash flag set.
		engine, err := NewEngine(
			txHashScript, tx, 0, testCase.verifyFlags, nil,
			sigHashes, txValue+feeValue, prevoutFetcher,
		)
		require.NoError(t, err)

		err = engine.Execute()
		if testCase.expectedErrCode != 0 {
			// We expect an error, so make sure it's the one we
			// expect.
			require.Error(t, err)
			require.Equal(
				t, testCase.expectedErrCode,
				err.(Error).ErrorCode,
			)
		} else {
			require.NoError(t, err)
		}
	}
}

// genTxHashScript generates a script that will require the tx hash to be
// committed to in the transaction.
func genTxHashScript(txhash, txfs []byte) ([]byte, error) {
	builder := NewScriptBuilder()

	builder.AddData(append(txhash, txfs...))
	builder.AddOp(OP_CHECKTXHASHVERIFY)

	return builder.Script()
}

// getAddress returns a new address.
func getAddress(t *testing.T) btcutil.Address {
	// Ordinarily the private key would come from whatever storage mechanism
	// is being used, but for this example just hard code it.
	privKeyBytes, err := hex.DecodeString("22a47fa09a223f2aa079edf85a7c2" +
		"d4f8720ee63e502ee2869afab7de234b80c")
	if err != nil {
		fmt.Println(err)
		return nil
	}
	_, pubKey := btcec.PrivKeyFromBytes(privKeyBytes)
	pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())
	addr, err := btcutil.NewAddressPubKeyHash(pubKeyHash,
		&chaincfg.MainNetParams)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	return addr
}

type TestVectorFile []TestVectors

// TestVectors is a struct that represents the test vectors for the
// CalculateTxHash tests.
type TestVectors struct {
	Prevs   []string `json:"prevs"`
	Tx      string   `json:"tx"`
	Vectors []struct {
		Input  uint32 `json:"input"`
		Txfs   string `json:"txfs"`
		TxHash string `json:"txhash"`
	} `json:"vectors"`
}

// parseTestVectorsFromFile parses the test vectors from a file.
func parseTestVectorsFromFile(t *testing.T, filename string) TestVectors {
	t.Helper()

	fileContents, err := os.ReadFile(filename)
	require.NoError(t, err)

	var testVectors TestVectorFile
	err = json.Unmarshal(fileContents, &testVectors)

	require.NoError(t, err)

	return testVectors[0]
}

// decodeHex decodes a hex string into a byte slice.
func decodeHex(t *testing.T, hexStr string) []byte {
	decoded, err := hex.DecodeString(hexStr)
	require.NoError(t, err)

	return decoded
}
