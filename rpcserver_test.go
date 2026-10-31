package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/mempool/mempooltest"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var (
	disableRPCChainLogsOnce sync.Once
)

// newTestRPCChain returns a fresh mainnet chain backed by a temporary
// database, along with a teardown func that closes the database.
func newTestRPCChain(t *testing.T) (*blockchain.BlockChain, func()) {
	t.Helper()

	paramsCopy := chaincfg.MainNetParams
	return newTestRPCChainWithParams(t, &paramsCopy)
}

// newTestRPCChainWithParams returns a fresh chain for the given params backed by
// a temporary database, along with a teardown func that closes the database.
func newTestRPCChainWithParams(t *testing.T, params *chaincfg.Params) (*blockchain.BlockChain, func()) {
	t.Helper()

	// These tests construct chain instances directly without going through the
	// normal btcd startup path that initializes the global log rotator.
	disableRPCChainLogsOnce.Do(func() {
		blockchain.DisableLog()
		database.DisableLog()
	})

	dbPath := filepath.Join(t.TempDir(), "rpcserver")
	db, err := database.Create("ffldb", dbPath, params.Net)
	require.NoError(t, err)

	paramsCopy := *params
	for i := range paramsCopy.Deployments {
		deployment := &paramsCopy.Deployments[i]
		if starter, ok := deployment.DeploymentStarter.(*chaincfg.MedianTimeDeploymentStarter); ok {
			deployment.DeploymentStarter = chaincfg.NewMedianTimeDeploymentStarter(starter.StartTime())
		}
		if ender, ok := deployment.DeploymentEnder.(*chaincfg.MedianTimeDeploymentEnder); ok {
			deployment.DeploymentEnder = chaincfg.NewMedianTimeDeploymentEnder(ender.EndTime())
		}
	}

	chain, err := blockchain.New(&blockchain.Config{
		DB:          db,
		ChainParams: &paramsCopy,
		TimeSource:  blockchain.NewMedianTime(),
		SigCache:    txscript.NewSigCache(1000),
	})
	require.NoError(t, err)

	return chain, func() {
		db.Close()
	}
}

// newTestRPCServer returns an rpcServer backed by a fresh mainnet chain, along
// with a teardown func that closes the underlying database.
func newTestRPCServer(t *testing.T) (*rpcServer, func()) {
	t.Helper()

	chain, teardown := newTestRPCChain(t)
	return &rpcServer{cfg: rpcserverConfig{Chain: chain}}, teardown
}

// encodeMerkleProof serializes the passed merkle block and returns it as a hex
// string, as expected by verifytxoutproof.
func encodeMerkleProof(t *testing.T, proof *wire.MsgMerkleBlock) string {
	t.Helper()

	var encoded bytes.Buffer
	err := proof.BtcEncode(&encoded, wire.ProtocolVersion, wire.LatestEncoding)
	require.NoError(t, err)
	return hex.EncodeToString(encoded.Bytes())
}

// genesisProof returns a merkle proof for the coinbase transaction of the
// mainnet genesis block, along with that transaction's hash string.
func genesisProof() (*wire.MsgMerkleBlock, string) {
	genesis := chaincfg.MainNetParams.GenesisBlock
	txHash := genesis.Transactions[0].TxHash()
	proof := &wire.MsgMerkleBlock{
		Header:       genesis.Header,
		Transactions: 1,
		Hashes:       []*chainhash.Hash{&txHash},
		Flags:        []byte{0x01},
	}
	return proof, txHash.String()
}

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

// TestHandleTestMempoolAcceptRejectsTrailingBytes ensures testmempoolaccept
// rejects byte strings that contain a valid transaction plus trailing data.
func TestHandleTestMempoolAcceptRejectsTrailingBytes(t *testing.T) {
	t.Parallel()

	defer func() {
		recovered := recover()
		require.Nil(t, recovered, "handler reached mempool")
	}()

	cmd := btcjson.NewTestMempoolAcceptCmd([]string{txHex1 + "00"}, 0)
	result, err := handleTestMempoolAccept(
		&rpcServer{}, cmd, make(chan struct{}),
	)

	requireRPCErrorCode(t, err, btcjson.ErrRPCDeserialization)
	require.Nil(t, result)
}

// requireRPCErrorCode asserts that the error is an RPC error with the expected
// error code.
func requireRPCErrorCode(t *testing.T, err error, code btcjson.RPCErrorCode) {
	t.Helper()

	require.Error(t, err)
	rpcErr, ok := err.(*btcjson.RPCError)
	require.True(t, ok)
	require.Equal(t, code, rpcErr.Code)
}

// blockHexWithTrailingByte serializes a valid block and appends one extra byte.
func blockHexWithTrailingByte(t *testing.T) string {
	t.Helper()

	var block bytes.Buffer
	err := chaincfg.MainNetParams.GenesisBlock.Serialize(&block)
	require.NoError(t, err)

	return hex.EncodeToString(append(block.Bytes(), 0x00))
}

// invalidatingBlockDB clears fetched block bytes as soon as its managed view
// ends. This models database backends whose zero-copy buffers are only valid
// for the lifetime of a transaction.
type invalidatingBlockDB struct {
	database.DB
	blockBytes []byte
}

// View runs the callback with a transaction backed by the configured block
// bytes, then invalidates those bytes before returning.
func (d *invalidatingBlockDB) View(fn func(database.Tx) error) error {
	err := fn(&invalidatingBlockTx{blockBytes: d.blockBytes})
	clear(d.blockBytes)

	return err
}

// invalidatingBlockTx returns the parent database's transaction-scoped block
// bytes.
type invalidatingBlockTx struct {
	database.Tx
	blockBytes []byte
}

// FetchBlock returns bytes that are invalidated when the enclosing view ends.
func (t *invalidatingBlockTx) FetchBlock(*chainhash.Hash) ([]byte, error) {
	return t.blockBytes, nil
}

// TestHandleGetBlockCopiesTransactionBytes verifies getblock does not retain
// transaction-scoped database memory after its managed view ends.
func TestHandleGetBlockCopiesTransactionBytes(t *testing.T) {
	t.Parallel()

	var serializedBlock bytes.Buffer
	err := chaincfg.MainNetParams.GenesisBlock.Serialize(&serializedBlock)
	require.NoError(t, err)

	wantBytes := serializedBlock.Bytes()
	dbBytes := append([]byte(nil), wantBytes...)
	dbBytes = append(dbBytes, 0x00)
	db := &invalidatingBlockDB{blockBytes: dbBytes}

	verbosity := 0
	cmd := btcjson.NewGetBlockCmd(
		chaincfg.MainNetParams.GenesisHash.String(), &verbosity,
	)
	result, err := handleGetBlock(
		&rpcServer{cfg: rpcserverConfig{DB: db}}, cmd, make(chan struct{}),
	)
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(wantBytes), result)
}

// TestHandleSendRawTransactionRejectsTrailingBytes ensures sendrawtransaction
// rejects byte strings that contain a valid transaction plus trailing data.
func TestHandleSendRawTransactionRejectsTrailingBytes(t *testing.T) {
	t.Parallel()

	mm := &mempooltest.MockTxMempool{}
	mm.On(
		"ProcessTransaction", mock.Anything, false, false, mempool.Tag(0),
	).Return(nil, errors.New("mempool should not be reached")).Maybe()

	s := &rpcServer{cfg: rpcserverConfig{
		TxMemPool: mm,
	}}
	cmd := btcjson.NewSendRawTransactionCmd(txHex1+"00", nil)

	result, err := handleSendRawTransaction(s, cmd, make(chan struct{}))
	requireRPCErrorCode(t, err, btcjson.ErrRPCDeserialization)
	require.Nil(t, result)
}

// TestHandleDecodeRawTransactionRejectsTrailingBytes ensures
// decoderawtransaction rejects byte strings that contain a valid transaction
// plus trailing data.
func TestHandleDecodeRawTransactionRejectsTrailingBytes(t *testing.T) {
	t.Parallel()

	cmd := btcjson.NewDecodeRawTransactionCmd(txHex1 + "00")
	result, err := handleDecodeRawTransaction(
		&rpcServer{}, cmd, make(chan struct{}),
	)

	requireRPCErrorCode(t, err, btcjson.ErrRPCDeserialization)
	require.Nil(t, result)
}

// TestHandleGetBlockTemplateProposalRejectsTrailingBytes ensures proposal mode
// rejects byte strings that contain a valid block plus trailing data.
func TestHandleGetBlockTemplateProposalRejectsTrailingBytes(t *testing.T) {
	t.Parallel()

	defer func() {
		recovered := recover()
		require.Nil(t, recovered, "handler reached chain state")
	}()

	request := &btcjson.TemplateRequest{
		Mode: "proposal",
		Data: blockHexWithTrailingByte(t),
	}

	result, err := handleGetBlockTemplateProposal(&rpcServer{}, request)
	requireRPCErrorCode(t, err, btcjson.ErrRPCDeserialization)
	require.Nil(t, result)
}

// TestHandleSubmitBlockRejectsTrailingBytes ensures submitblock rejects byte
// strings that contain a valid block plus trailing data.
func TestHandleSubmitBlockRejectsTrailingBytes(t *testing.T) {
	t.Parallel()

	defer func() {
		recovered := recover()
		require.Nil(t, recovered, "handler reached sync manager")
	}()

	cmd := btcjson.NewSubmitBlockCmd(blockHexWithTrailingByte(t), nil)
	result, err := handleSubmitBlock(&rpcServer{}, cmd, make(chan struct{}))

	requireRPCErrorCode(t, err, btcjson.ErrRPCDeserialization)
	require.Nil(t, result)
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
	mm := &mempooltest.MockTxMempool{}

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
	mm := &mempooltest.MockTxMempool{}

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

// TestHandleGetTxOutProof checks that handleGetTxOutProof builds a proof for a
// transaction in an explicitly provided block, and that the proof round-trips
// through handleVerifyTxOutProof.
func TestHandleGetTxOutProof(t *testing.T) {
	t.Parallel()

	require := require.New(t)
	s, teardown := newTestRPCServer(t)
	defer teardown()

	genesis := chaincfg.MainNetParams.GenesisBlock
	txHash := genesis.Transactions[0].TxHash().String()
	blockHash := genesis.BlockHash().String()

	result, err := handleGetTxOutProof(s, btcjson.NewGetTxOutProofCmd(
		[]string{txHash}, btcjson.String(blockHash),
	), make(chan struct{}))
	require.NoError(err)

	proofHex, ok := result.(string)
	require.True(ok)
	require.NotEmpty(proofHex)

	verified, err := handleVerifyTxOutProof(s,
		btcjson.NewVerifyTxOutProofCmd(proofHex), make(chan struct{}))
	require.NoError(err)
	require.Equal([]string{txHash}, verified)
}

// TestHandleGetTxOutProofWithoutBlockHash checks that handleGetTxOutProof
// locates the containing block through the UTXO set when no block hash is
// given, and that the resulting proof verifies.
func TestHandleGetTxOutProofWithoutBlockHash(t *testing.T) {
	t.Parallel()

	require := require.New(t)
	paramsCopy := chaincfg.RegressionNetParams
	chain, teardown := newTestRPCChainWithParams(t, &paramsCopy)
	defer teardown()
	s := &rpcServer{cfg: rpcserverConfig{Chain: chain}}

	miningAddr, err := address.NewAddressPubKeyHash(make([]byte, 20), &paramsCopy)
	require.NoError(err)

	block, err := rpctest.CreateBlock(nil, nil, 4, time.Time{}, miningAddr, nil, &paramsCopy)
	require.NoError(err)

	isMainChain, isOrphan, err := chain.ProcessBlock(block, blockchain.BFNone)
	require.NoError(err)
	require.True(isMainChain)
	require.False(isOrphan)

	txHash := block.Transactions()[0].Hash().String()

	result, err := handleGetTxOutProof(s, btcjson.NewGetTxOutProofCmd(
		[]string{txHash}, nil,
	), make(chan struct{}))
	require.NoError(err)

	proofHex, ok := result.(string)
	require.True(ok)
	require.NotEmpty(proofHex)

	verified, err := handleVerifyTxOutProof(s,
		btcjson.NewVerifyTxOutProofCmd(proofHex), make(chan struct{}))
	require.NoError(err)
	require.Equal([]string{txHash}, verified)
}

// TestHandleGetTxOutProofErrors checks that handleGetTxOutProof returns the
// expected RPC error codes for invalid parameters.
func TestHandleGetTxOutProofErrors(t *testing.T) {
	t.Parallel()

	require := require.New(t)
	s, teardown := newTestRPCServer(t)
	defer teardown()
	genesisTxID := chaincfg.MainNetParams.GenesisBlock.Transactions[0].TxHash().String()

	tests := []struct {
		name            string
		cmd             *btcjson.GetTxOutProofCmd
		expectedErrCode btcjson.RPCErrorCode
	}{
		{
			name:            "empty tx list",
			cmd:             btcjson.NewGetTxOutProofCmd(nil, nil),
			expectedErrCode: btcjson.ErrRPCInvalidParameter,
		},
		{
			name:            "duplicate tx ids",
			cmd:             btcjson.NewGetTxOutProofCmd([]string{genesisTxID, genesisTxID}, nil),
			expectedErrCode: btcjson.ErrRPCInvalidParameter,
		},
		{
			name:            "invalid tx id",
			cmd:             btcjson.NewGetTxOutProofCmd([]string{"nope"}, nil),
			expectedErrCode: btcjson.ErrRPCDecodeHexString,
		},
		{
			name:            "invalid block hash",
			cmd:             btcjson.NewGetTxOutProofCmd([]string{genesisTxID}, btcjson.String("nope")),
			expectedErrCode: btcjson.ErrRPCDecodeHexString,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := handleGetTxOutProof(s, test.cmd, make(chan struct{}))
			require.Nil(result)
			requireRPCErrorCode(t, err, test.expectedErrCode)
		})
	}
}

// TestFindTxOutProofBlockHash checks that findTxOutProofBlockHash returns the
// first block hash resolved by the lookup, skips unresolved transactions, and
// propagates lookup errors.
func TestFindTxOutProofBlockHash(t *testing.T) {
	t.Parallel()

	blockHash := chainhash.DoubleHashH([]byte("block"))
	otherHash := chainhash.DoubleHashH([]byte("other-block"))
	txHashes := []*chainhash.Hash{
		func() *chainhash.Hash {
			h := chainhash.DoubleHashH([]byte("tx-1"))
			return &h
		}(),
		func() *chainhash.Hash {
			h := chainhash.DoubleHashH([]byte("tx-2"))
			return &h
		}(),
		func() *chainhash.Hash {
			h := chainhash.DoubleHashH([]byte("tx-3"))
			return &h
		}(),
	}

	tests := []struct {
		name          string
		lookupResults map[chainhash.Hash]*chainhash.Hash
		lookupErr     error
		wantHash      *chainhash.Hash
		wantErrCode   btcjson.RPCErrorCode
	}{
		{
			name: "uses any resolved hash",
			lookupResults: map[chainhash.Hash]*chainhash.Hash{
				*txHashes[0]: nil,
				*txHashes[1]: &blockHash,
				*txHashes[2]: nil,
			},
			wantHash: &blockHash,
		},
		{
			name: "returns nil when no hashes resolve",
			lookupResults: map[chainhash.Hash]*chainhash.Hash{
				*txHashes[0]: nil,
				*txHashes[1]: nil,
				*txHashes[2]: nil,
			},
		},
		{
			name: "uses first resolved hash",
			lookupResults: map[chainhash.Hash]*chainhash.Hash{
				*txHashes[0]: &blockHash,
				*txHashes[1]: &otherHash,
			},
			wantHash: &blockHash,
		},
		{
			name:        "propagates lookup error",
			lookupErr:   errors.New("boom"),
			wantErrCode: btcjson.ErrRPCMisc,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resolvedHash, err := findTxOutProofBlockHash(txHashes, func(txHash *chainhash.Hash) (*chainhash.Hash, error) {
				if test.lookupErr != nil {
					return nil, &btcjson.RPCError{
						Code:    btcjson.ErrRPCMisc,
						Message: test.lookupErr.Error(),
					}
				}

				return test.lookupResults[*txHash], nil
			})

			if test.wantErrCode != 0 {
				require.Error(t, err)
				rpcErr, ok := err.(*btcjson.RPCError)
				require.True(t, ok)
				require.Equal(t, test.wantErrCode, rpcErr.Code)
				return
			}

			require.NoError(t, err)
			if test.wantHash == nil {
				require.Nil(t, resolvedHash)
				return
			}

			require.NotNil(t, resolvedHash)
			require.True(t, test.wantHash.IsEqual(resolvedHash))
		})
	}
}

// TestHandleGetTxOutProofMixedBlocks checks that handleGetTxOutProof rejects
// transactions from different blocks when no block hash is given, matching
// Bitcoin Core's "Not all transactions found" error.
func TestHandleGetTxOutProofMixedBlocks(t *testing.T) {
	t.Parallel()

	require := require.New(t)
	paramsCopy := chaincfg.RegressionNetParams
	chain, teardown := newTestRPCChainWithParams(t, &paramsCopy)
	defer teardown()
	s := &rpcServer{cfg: rpcserverConfig{Chain: chain}}

	miningAddr, err := address.NewAddressPubKeyHash(make([]byte, 20), &paramsCopy)
	require.NoError(err)

	block1, err := rpctest.CreateBlock(nil, nil, 4, time.Time{}, miningAddr, nil, &paramsCopy)
	require.NoError(err)

	isMainChain, isOrphan, err := chain.ProcessBlock(block1, blockchain.BFNone)
	require.NoError(err)
	require.True(isMainChain)
	require.False(isOrphan)

	block2, err := rpctest.CreateBlock(block1, nil, 4, time.Time{}, miningAddr, nil, &paramsCopy)
	require.NoError(err)

	isMainChain, isOrphan, err = chain.ProcessBlock(block2, blockchain.BFNone)
	require.NoError(err)
	require.True(isMainChain)
	require.False(isOrphan)

	block1TxHash := block1.Transactions()[0].Hash().String()
	block2TxHash := block2.Transactions()[0].Hash().String()

	result, err := handleGetTxOutProof(s, btcjson.NewGetTxOutProofCmd(
		[]string{block1TxHash, block2TxHash}, nil,
	), make(chan struct{}))
	require.Nil(result)

	rpcErr, ok := err.(*btcjson.RPCError)
	require.True(ok)
	require.Equal(btcjson.ErrRPCInvalidAddressOrKey, rpcErr.Code)
	require.Equal("Not all transactions found in specified or retrieved block", rpcErr.Message)
}

// TestHandleVerifyTxOutProof checks that handleVerifyTxOutProof returns the
// matched transaction hashes for a valid proof of a block in the main chain.
func TestHandleVerifyTxOutProof(t *testing.T) {
	t.Parallel()

	require := require.New(t)
	s, teardown := newTestRPCServer(t)
	defer teardown()

	proof, txHash := genesisProof()
	result, err := handleVerifyTxOutProof(s,
		btcjson.NewVerifyTxOutProofCmd(encodeMerkleProof(t, proof)),
		make(chan struct{}))
	require.NoError(err)
	require.Equal([]string{txHash}, result)
}

// TestHandleVerifyTxOutProofErrors checks that handleVerifyTxOutProof returns
// errors for invalid hex, undecodable proofs, and proofs of unknown blocks.
func TestHandleVerifyTxOutProofErrors(t *testing.T) {
	t.Parallel()

	require := require.New(t)
	s, teardown := newTestRPCServer(t)
	defer teardown()

	result, err := handleVerifyTxOutProof(s,
		btcjson.NewVerifyTxOutProofCmd("zz"), make(chan struct{}))
	require.Nil(result)
	requireRPCErrorCode(t, err, btcjson.ErrRPCDecodeHexString)

	result, err = handleVerifyTxOutProof(s,
		btcjson.NewVerifyTxOutProofCmd(hex.EncodeToString([]byte{0x01, 0x02})),
		make(chan struct{}))
	require.Nil(result)
	requireRPCErrorCode(t, err, btcjson.ErrRPCDeserialization)

	proof, _ := genesisProof()
	proof.Header.Timestamp = proof.Header.Timestamp.Add(1)
	proof.Header.Nonce++
	result, err = handleVerifyTxOutProof(s,
		btcjson.NewVerifyTxOutProofCmd(encodeMerkleProof(t, proof)),
		make(chan struct{}))
	require.Nil(result)
	requireRPCErrorCode(t, err, btcjson.ErrRPCBlockNotFound)
}

// TestGetTxSpendingPrevOut checks that handleGetTxSpendingPrevOut handles the
// cmd as expected.
func TestGetTxSpendingPrevOut(t *testing.T) {
	t.Parallel()

	require := require.New(t)

	// Create a mock mempool.
	mm := &mempooltest.MockTxMempool{}
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
