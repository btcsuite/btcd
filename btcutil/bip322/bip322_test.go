package bip322

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/stretchr/testify/require"
)

var (
	hexEncode    = hex.EncodeToString
)

// testVectors is the top-level structure of the test data file.
type testVectors struct {
	TxHashes []txHashVector    `json:"tx_hashes,omitempty"`
	Simple   []simpleSignatureVector `json:"simple"`
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
	Message          string   `json:"message"`
	PrivateKeys      []string `json:"private_keys"`
	Address          string   `json:"address"`
	Type             string   `json:"type"`
	WitnessScript    string   `json:"witness_script"`
	TxVersion        int32    `json:"tx_version"`
	LockTime         uint32   `json:"lock_time"`
	Sequence         uint32   `json:"sequence"`
	Bip322Signatures []string `json:"bip322_signatures"`
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

// testName returns a human-readable sub-test name for a given message.
func testName(message string) string {
	if message == "" {
		return "msg=<empty>"
	}

	return fmt.Sprintf("msg=%s", message)
}

// TestTxHashes tests the tx_hashes test vectors as mentioned in BIP-322:
// https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
func TestTxHashes(t *testing.T) {
	vectors := loadTestVectors(t, "basic-test-vectors.json")

	for _, tc := range vectors.TxHashes {
		t.Run(testName(tc.Message), func(t *testing.T) {
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
