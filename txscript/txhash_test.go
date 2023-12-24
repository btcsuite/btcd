package txscript

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"

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
