package main

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestHandleTestMempoolAcceptFailDecode checks that when invalid hex string is
// used as the raw txns, the corresponding error is returned.
func TestHandleTestMempoolAcceptFailDecode(t *testing.T) {
	t.Parallel()

	require := require.New(t)

	// Create a testing server.
	s := &rpcServer{}

	testCases := []struct {
		name            string
		txns            []string
		expectedErrCode btcjson.RPCErrorCode
	}{
		{
			name:            "hex decode fail",
			txns:            []string{"invalid"},
			expectedErrCode: btcjson.ErrRPCDecodeHexString,
		},
		{
			name:            "tx decode fail",
			txns:            []string{"696e76616c6964"},
			expectedErrCode: btcjson.ErrRPCDeserialization,
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create a request that uses invalid raw txns.
			cmd := btcjson.NewTestMempoolAcceptCmd(tc.txns, 0)

			// Call the method under test.
			closeChan := make(chan struct{})
			result, err := handleTestMempoolAccept(
				s, cmd, closeChan,
			)

			// Ensure the expected error is returned.
			require.Error(err)
			rpcErr, ok := err.(*btcjson.RPCError)
			require.True(ok)
			require.Equal(tc.expectedErrCode, rpcErr.Code)

			// No result should be returned.
			require.Nil(result)
		})
	}
}

var (
	// TODO(yy): make a `btctest` package and move these testing txns there
	// so they be used in other tests.
	//
	// txHex1 is taken from `txscript/data/tx_valid.json`.
	txHex1 = "0100000001b14bdcbc3e01bdaad36cc08e81e69c82e1060bc14e518db2b" +
		"49aa43ad90ba26000000000490047304402203f16c6f40162ab686621ef3" +
		"000b04e75418a0c0cb2d8aebeac894ae360ac1e780220ddc15ecdfc3507a" +
		"c48e1681a33eb60996631bf6bf5bc0a0682c4db743ce7ca2b01ffffffff0" +
		"140420f00000000001976a914660d4ef3a743e3e696ad990364e555c271a" +
		"d504b88ac00000000"

	// txHex2 is taken from `txscript/data/tx_valid.json`.
	txHex2 = "0100000001b14bdcbc3e01bdaad36cc08e81e69c82e1060bc14e518db2b" +
		"49aa43ad90ba260000000004a0048304402203f16c6f40162ab686621ef3" +
		"000b04e75418a0c0cb2d8aebeac894ae360ac1e780220ddc15ecdfc3507a" +
		"c48e1681a33eb60996631bf6bf5bc0a0682c4db743ce7ca2bab01fffffff" +
		"f0140420f00000000001976a914660d4ef3a743e3e696ad990364e555c27" +
		"1ad504b88ac00000000"

	// txHex3 is taken from `txscript/data/tx_valid.json`.
	txHex3 = "0100000001b14bdcbc3e01bdaad36cc08e81e69c82e1060bc14e518db2b" +
		"49aa43ad90ba260000000004a01ff47304402203f16c6f40162ab686621e" +
		"f3000b04e75418a0c0cb2d8aebeac894ae360ac1e780220ddc15ecdfc350" +
		"7ac48e1681a33eb60996631bf6bf5bc0a0682c4db743ce7ca2b01fffffff" +
		"f0140420f00000000001976a914660d4ef3a743e3e696ad990364e555c27" +
		"1ad504b88ac00000000"
)

// decodeTxHex decodes the given hex string into a transaction.
func decodeTxHex(t *testing.T, txHex string) *btcutil.Tx {
	rawBytes, err := hex.DecodeString(txHex)
	require.NoError(t, err)
	tx, err := btcutil.NewTxFromBytes(rawBytes)
	require.NoError(t, err)

	return tx
}

// TestHandleTestMempoolAcceptMixedResults checks that when different txns get
// different responses from calling the mempool method `CheckMempoolAcceptance`
// their results are correctly returned.
func TestHandleTestMempoolAcceptMixedResults(t *testing.T) {
	t.Parallel()

	require := require.New(t)

	// Create a mock mempool.
	mm := &mempool.MockTxMempool{}

	// Create a testing server with the mock mempool.
	s := &rpcServer{cfg: rpcserverConfig{
		TxMemPool: mm,
	}}

	// Decode the hex so we can assert the mock mempool is called with it.
	tx1 := decodeTxHex(t, txHex1)
	tx2 := decodeTxHex(t, txHex2)
	tx3 := decodeTxHex(t, txHex3)

	// Create a slice to hold the expected results. We will use three txns
	// so we expect threeresults.
	expectedResults := make([]*btcjson.TestMempoolAcceptResult, 3)

	// We now mock the first call to `CheckMempoolAcceptance` to return an
	// error.
	dummyErr := errors.New("dummy error")
	mm.On("CheckMempoolAcceptance", tx1).Return(nil, dummyErr).Once()

	// Since the call failed, we expect the first result to give us the
	// error.
	expectedResults[0] = &btcjson.TestMempoolAcceptResult{
		Txid:         tx1.Hash().String(),
		Wtxid:        tx1.WitnessHash().String(),
		Allowed:      false,
		RejectReason: dummyErr.Error(),
	}

	// We mock the second call to `CheckMempoolAcceptance` to return a
	// result saying the tx is missing inputs.
	mm.On("CheckMempoolAcceptance", tx2).Return(
		&mempool.MempoolAcceptResult{
			MissingParents: []*chainhash.Hash{},
		}, nil,
	).Once()

	// We expect the second result to give us the missing-inputs error.
	expectedResults[1] = &btcjson.TestMempoolAcceptResult{
		Txid:         tx2.Hash().String(),
		Wtxid:        tx2.WitnessHash().String(),
		Allowed:      false,
		RejectReason: "missing-inputs",
	}

	// We mock the third call to `CheckMempoolAcceptance` to return a
	// result saying the tx allowed.
	const feeSats = btcutil.Amount(1000)
	mm.On("CheckMempoolAcceptance", tx3).Return(
		&mempool.MempoolAcceptResult{
			TxFee:  feeSats,
			TxSize: 100,
		}, nil,
	).Once()

	// We expect the third result to give us the fee details.
	expectedResults[2] = &btcjson.TestMempoolAcceptResult{
		Txid:    tx3.Hash().String(),
		Wtxid:   tx3.WitnessHash().String(),
		Allowed: true,
		Vsize:   100,
		Fees: &btcjson.TestMempoolAcceptFees{
			Base:             feeSats.ToBTC(),
			EffectiveFeeRate: feeSats.ToBTC() * 1e3 / 100,
		},
	}

	// Create a mock request with default max fee rate of 0.1 BTC/KvB.
	cmd := btcjson.NewTestMempoolAcceptCmd(
		[]string{txHex1, txHex2, txHex3}, 0.1,
	)

	// Call the method handler and assert the expected results are
	// returned.
	closeChan := make(chan struct{})
	results, err := handleTestMempoolAccept(s, cmd, closeChan)
	require.NoError(err)
	require.Equal(expectedResults, results)

	// Assert the mocked method is called as expected.
	mm.AssertExpectations(t)
}

// TestValidateFeeRate checks that `validateFeeRate` behaves as expected.
func TestValidateFeeRate(t *testing.T) {
	t.Parallel()

	const (
		// testFeeRate is in BTC/kvB.
		testFeeRate = 0.1

		// testTxSize is in vb.
		testTxSize = 100

		// testFeeSats is in sats.
		// We have 0.1BTC/kvB =
		//   0.1 * 1e8 sats/kvB =
		//   0.1 * 1e8 / 1e3 sats/vb = 0.1 * 1e5 sats/vb.
		testFeeSats = btcutil.Amount(testFeeRate * 1e5 * testTxSize)
	)

	testCases := []struct {
		name         string
		feeSats      btcutil.Amount
		txSize       int64
		maxFeeRate   float64
		expectedFees *btcjson.TestMempoolAcceptFees
		allowed      bool
	}{
		{
			// When the fee rate(0.1) is above the max fee
			// rate(0.01), we expect a nil result and false.
			name:         "fee rate above max",
			feeSats:      testFeeSats,
			txSize:       testTxSize,
			maxFeeRate:   testFeeRate / 10,
			expectedFees: nil,
			allowed:      false,
		},
		{
			// When the fee rate(0.1) is no greater than the max
			// fee rate(0.1), we expect a result and true.
			name:       "fee rate below max",
			feeSats:    testFeeSats,
			txSize:     testTxSize,
			maxFeeRate: testFeeRate,
			expectedFees: &btcjson.TestMempoolAcceptFees{
				Base:             testFeeSats.ToBTC(),
				EffectiveFeeRate: testFeeRate,
			},
			allowed: true,
		},
		{
			// When the fee rate(1) is above the default max fee
			// rate(0.1), we expect a nil result and false.
			name:         "fee rate above default max",
			feeSats:      testFeeSats,
			txSize:       testTxSize / 10,
			expectedFees: nil,
			allowed:      false,
		},
		{
			// When the fee rate(0.1) is no greater than the
			// default max fee rate(0.1), we expect a result and
			// true.
			name:    "fee rate below default max",
			feeSats: testFeeSats,
			txSize:  testTxSize,
			expectedFees: &btcjson.TestMempoolAcceptFees{
				Base:             testFeeSats.ToBTC(),
				EffectiveFeeRate: testFeeRate,
			},
			allowed: true,
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			require := require.New(t)

			result, allowed := validateFeeRate(
				tc.feeSats, tc.txSize, tc.maxFeeRate,
			)

			require.Equal(tc.expectedFees, result)
			require.Equal(tc.allowed, allowed)
		})
	}
}

// TestHandleTestMempoolAcceptFees checks that the `Fees` field is correctly
// populated based on the max fee rate and the tx being checked.
func TestHandleTestMempoolAcceptFees(t *testing.T) {
	t.Parallel()

	// Create a mock mempool.
	mm := &mempool.MockTxMempool{}

	// Create a testing server with the mock mempool.
	s := &rpcServer{cfg: rpcserverConfig{
		TxMemPool: mm,
	}}

	const (
		// Set transaction's fee rate to be 0.2BTC/kvB.
		feeRate = defaultMaxFeeRate * 2

		// txSize is 100vb.
		txSize = 100

		// feeSats is 2e6 sats.
		feeSats = feeRate * 1e8 * txSize / 1e3
	)

	testCases := []struct {
		name         string
		maxFeeRate   float64
		txHex        string
		rejectReason string
		allowed      bool
	}{
		{
			// When the fee rate(0.2) used by the tx is below the
			// max fee rate(2) specified, the result should allow
			// it.
			name:       "below max fee rate",
			maxFeeRate: feeRate * 10,
			txHex:      txHex1,
			allowed:    true,
		},
		{
			// When the fee rate(0.2) used by the tx is above the
			// max fee rate(0.02) specified, the result should
			// disallow it.
			name:         "above max fee rate",
			maxFeeRate:   feeRate / 10,
			txHex:        txHex1,
			allowed:      false,
			rejectReason: "max-fee-exceeded",
		},
		{
			// When the max fee rate is not set, the default
			// 0.1BTC/kvB is used and the fee rate(0.2) used by the
			// tx is above it, the result should disallow it.
			name:         "above default max fee rate",
			txHex:        txHex1,
			allowed:      false,
			rejectReason: "max-fee-exceeded",
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			require := require.New(t)

			// Decode the hex so we can assert the mock mempool is
			// called with it.
			tx := decodeTxHex(t, txHex1)

			// We mock the call to `CheckMempoolAcceptance` to
			// return the result.
			mm.On("CheckMempoolAcceptance", tx).Return(
				&mempool.MempoolAcceptResult{
					TxFee:  feeSats,
					TxSize: txSize,
				}, nil,
			).Once()

			// We expect the third result to give us the fee
			// details.
			expected := &btcjson.TestMempoolAcceptResult{
				Txid:    tx.Hash().String(),
				Wtxid:   tx.WitnessHash().String(),
				Allowed: tc.allowed,
			}

			if tc.allowed {
				expected.Vsize = txSize
				expected.Fees = &btcjson.TestMempoolAcceptFees{
					Base:             feeSats / 1e8,
					EffectiveFeeRate: feeRate,
				}
			} else {
				expected.RejectReason = tc.rejectReason
			}

			// Create a mock request with specified max fee rate.
			cmd := btcjson.NewTestMempoolAcceptCmd(
				[]string{txHex1}, tc.maxFeeRate,
			)

			// Call the method handler and assert the expected
			// result is returned.
			closeChan := make(chan struct{})
			r, err := handleTestMempoolAccept(s, cmd, closeChan)
			require.NoError(err)

			// Check the interface type.
			results, ok := r.([]*btcjson.TestMempoolAcceptResult)
			require.True(ok)

			// Expect exactly one result.
			require.Len(results, 1)

			// Check the result is returned as expected.
			require.Equal(expected, results[0])

			// Assert the mocked method is called as expected.
			mm.AssertExpectations(t)
		})
	}
}

// TestGetTxSpendingPrevOut checks that handleGetTxSpendingPrevOut handles the
// cmd as expected.
func TestGetTxSpendingPrevOut(t *testing.T) {
	t.Parallel()

	require := require.New(t)

	// Create a mock mempool.
	mm := &mempool.MockTxMempool{}
	defer mm.AssertExpectations(t)

	// Create a testing server with the mock mempool.
	s := &rpcServer{cfg: rpcserverConfig{
		TxMemPool: mm,
	}}

	// First, check the error case.
	//
	// Create a request that will cause an error.
	cmd := &btcjson.GetTxSpendingPrevOutCmd{
		Outputs: []*btcjson.GetTxSpendingPrevOutCmdOutput{
			{Txid: "invalid"},
		},
	}

	// Call the method handler and assert the error is returned.
	closeChan := make(chan struct{})
	results, err := handleGetTxSpendingPrevOut(s, cmd, closeChan)
	require.Error(err)
	require.Nil(results)

	// We now check the normal case. Two outputs will be tested - one found
	// in mempool and other not.
	//
	// Decode the hex so we can assert the mock mempool is called with it.
	tx := decodeTxHex(t, txHex1)

	// Create testing outpoints.
	opInMempool := wire.OutPoint{Hash: chainhash.Hash{1}, Index: 1}
	opNotInMempool := wire.OutPoint{Hash: chainhash.Hash{2}, Index: 1}

	// We only expect to see one output being found as spent in mempool.
	expectedResults := []*btcjson.GetTxSpendingPrevOutResult{
		{
			Txid:         opInMempool.Hash.String(),
			Vout:         opInMempool.Index,
			SpendingTxid: tx.Hash().String(),
		},
		{
			Txid: opNotInMempool.Hash.String(),
			Vout: opNotInMempool.Index,
		},
	}

	// We mock the first call to `CheckSpend` to return a result saying the
	// output is found.
	mm.On("CheckSpend", opInMempool).Return(tx).Once()

	// We mock the second call to `CheckSpend` to return a result saying the
	// output is NOT found.
	mm.On("CheckSpend", opNotInMempool).Return(nil).Once()

	// Create a request with the above outputs.
	cmd = &btcjson.GetTxSpendingPrevOutCmd{
		Outputs: []*btcjson.GetTxSpendingPrevOutCmdOutput{
			{
				Txid: opInMempool.Hash.String(),
				Vout: opInMempool.Index,
			},
			{
				Txid: opNotInMempool.Hash.String(),
				Vout: opNotInMempool.Index,
			},
		},
	}

	// Call the method handler and assert the expected result is returned.
	closeChan = make(chan struct{})
	results, err = handleGetTxSpendingPrevOut(s, cmd, closeChan)
	require.NoError(err)
	require.Equal(expectedResults, results)
}
