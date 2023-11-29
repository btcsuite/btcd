//go:build rpctest
// +build rpctest

package integration

import (
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestTestMempoolAccept checks that `TestTestMempoolAccept` behaves as
// expected. It checks that,
// - an error is returned when invalid params are used.
// - orphan tx is rejected.
// - fee rate above the max is rejected.
// - a mixed of both allowed and rejected can be returned in the same response.
func TestTestMempoolAccept(t *testing.T) {
	t.Parallel()

	// Boilerplate codetestDir to make a pruned node.
	btcdCfg := []string{"--rejectnonstd", "--debuglevel=debug"}
	r, err := rpctest.New(&chaincfg.SimNetParams, nil, btcdCfg, "")
	require.NoError(t, err)

	// Setup the node.
	require.NoError(t, r.SetUp(true, 100))
	t.Cleanup(func() {
		require.NoError(t, r.TearDown())
	})

	// Create testing txns.
	invalidTx := decodeHex(t, missingParentsHex)
	validTx := createTestTx(t, r)

	// Create testing constants.
	const feeRate = 10

	testCases := []struct {
		name           string
		txns           []*wire.MsgTx
		maxFeeRate     float64
		expectedErr    error
		expectedResult []*btcjson.TestMempoolAcceptResult
	}{
		{
			// When too many txns are provided, the method should
			// return an error.
			name:           "too many txns",
			txns:           make([]*wire.MsgTx, 26),
			maxFeeRate:     0,
			expectedErr:    rpcclient.ErrInvalidParam,
			expectedResult: nil,
		},
		{
			// When no txns are provided, the method should return
			// an error.
			name:           "empty txns",
			txns:           nil,
			maxFeeRate:     0,
			expectedErr:    rpcclient.ErrInvalidParam,
			expectedResult: nil,
		},
		{
			// When a corrupted txn is provided, the method should
			// return an error.
			name:           "corrupted tx",
			txns:           []*wire.MsgTx{{}},
			maxFeeRate:     0,
			expectedErr:    rpcclient.ErrInvalidParam,
			expectedResult: nil,
		},
		{
			// When an orphan tx is provided, the method should
			// return a test mempool accept result which says this
			// tx is not allowed.
			name:       "orphan tx",
			txns:       []*wire.MsgTx{invalidTx},
			maxFeeRate: 0,
			expectedResult: []*btcjson.TestMempoolAcceptResult{{
				Txid:         invalidTx.TxHash().String(),
				Wtxid:        invalidTx.TxHash().String(),
				Allowed:      false,
				RejectReason: "missing-inputs",
			}},
		},
		{
			// When a valid tx is provided but it exceeds the max
			// fee rate, the method should return a test mempool
			// accept result which says it's not allowed.
			name:       "valid tx but exceeds max fee rate",
			txns:       []*wire.MsgTx{validTx},
			maxFeeRate: 1e-5,
			expectedResult: []*btcjson.TestMempoolAcceptResult{{
				Txid:         validTx.TxHash().String(),
				Wtxid:        validTx.TxHash().String(),
				Allowed:      false,
				RejectReason: "max-fee-exceeded",
			}},
		},
		{
			// When a valid tx is provided and it doesn't exceeds
			// the max fee rate, the method should return a test
			// mempool accept result which says it's allowed.
			name: "valid tx and sane fee rate",
			txns: []*wire.MsgTx{validTx},
			expectedResult: []*btcjson.TestMempoolAcceptResult{{
				Txid:    validTx.TxHash().String(),
				Wtxid:   validTx.TxHash().String(),
				Allowed: true,
				// TODO(yy): need to calculate the fees, atm
				// there's no easy way.
				// Fees: &btcjson.TestMempoolAcceptFees{},
			}},
		},
		{
			// When multiple txns are provided, the method should
			// return the correct results for each of the txns.
			name: "multiple txns",
			txns: []*wire.MsgTx{invalidTx, validTx},
			expectedResult: []*btcjson.TestMempoolAcceptResult{{
				Txid:         invalidTx.TxHash().String(),
				Wtxid:        invalidTx.TxHash().String(),
				Allowed:      false,
				RejectReason: "missing-inputs",
			}, {
				Txid:    validTx.TxHash().String(),
				Wtxid:   validTx.TxHash().String(),
				Allowed: true,
			}},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require := require.New(t)

			results, err := r.Client.TestMempoolAccept(
				tc.txns, tc.maxFeeRate,
			)

			require.ErrorIs(err, tc.expectedErr)
			require.Len(results, len(tc.expectedResult))

			// Check each item is returned as expected.
			for i, r := range results {
				expected := tc.expectedResult[i]

				// TODO(yy): check all the fields?
				require.Equal(expected.Txid, r.Txid)
				require.Equal(expected.Wtxid, r.Wtxid)
				require.Equal(expected.Allowed, r.Allowed)
				require.Equal(expected.RejectReason,
					r.RejectReason)
			}
		})
	}
}

var (
	//nolint:lll
	missingParentsHex = "0100000003bcb2054607a921b3c6df992a9486776863b28485e731a805931b6feb14221acff2000000001c75619cdff9d694a434b13abfbbd618e2ece4460f24b4821cf47d5afc481a386c59565c4900000000cff75994dceb5f5568f8ada45d428630f512fb8efacd46682b4367b4edaf1985c5e4af4b07010000003c029216047236f3000000000017a9141d5a2c690c3e2dacb3cead240f0ce4a273b9d0e48758020000000000001600149d38710eb90e420b159c7a9263994c88e6810bc758020000000000001976a91490770ceff2b1c32e9dbf952fbe65b04a54d1949388ac580200000000000017a914f017945d4d088c7d42ab3bcbc1adce51d74fbd9f8784d7ee4b"
)

// createTestTx creates a `wire.MsgTx` and asserts its creation.
func createTestTx(t *testing.T, h *rpctest.Harness) *wire.MsgTx {
	addr, err := h.NewAddress()
	require.NoError(t, err)

	script, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	output := &wire.TxOut{
		PkScript: script,
		Value:    1e6,
	}

	tx, err := h.CreateTransaction([]*wire.TxOut{output}, 10, true)
	require.NoError(t, err)

	return tx
}

// decodeHex takes a tx hexstring and asserts it can be decoded into a
// `wire.MsgTx`.
func decodeHex(t *testing.T, txHex string) *wire.MsgTx {
	serializedTx, err := hex.DecodeString(txHex)
	require.NoError(t, err)

	tx, err := btcutil.NewTxFromBytes(serializedTx)
	require.NoError(t, err)

	return tx.MsgTx()
}
