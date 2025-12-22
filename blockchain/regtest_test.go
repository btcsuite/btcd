package blockchain

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// stubChainCtx provides the minimal ChainCtx implementation needed for header
// context checks in tests.
type stubChainCtx struct {
	params *chaincfg.Params
}

// ChainParams returns the active chain parameters.
func (s stubChainCtx) ChainParams() *chaincfg.Params {
	return s.params
}

// BlocksPerRetarget returns the blocks per difficulty retarget.
func (s stubChainCtx) BlocksPerRetarget() int32 {
	return int32(s.params.TargetTimespan / s.params.TargetTimePerBlock)
}

// MinRetargetTimespan returns the lower bound of the retarget timespan.
func (s stubChainCtx) MinRetargetTimespan() int64 {
	return int64(
		s.params.TargetTimespan /
			time.Duration(s.params.RetargetAdjustmentFactor),
	)
}

// MaxRetargetTimespan returns the upper bound of the retarget timespan.
func (s stubChainCtx) MaxRetargetTimespan() int64 {
	return int64(
		s.params.TargetTimespan *
			time.Duration(s.params.RetargetAdjustmentFactor),
	)
}

// VerifyCheckpoint reports whether a checkpoint matches.
func (s stubChainCtx) VerifyCheckpoint(_ int32, _ *chainhash.Hash) bool {
	return true
}

// FindPreviousCheckpoint returns the last known checkpoint.
func (s stubChainCtx) FindPreviousCheckpoint() (HeaderCtx, error) {
	return nil, nil
}

// regtestPrevNode returns a blockNode for the regtest genesis block to serve
// as the parent of height-1 test headers.
func regtestPrevNode(t *testing.T) *blockNode {
	t.Helper()

	params := chaincfg.RegressionNetParams

	return newBlockNode(&params.GenesisBlock.Header, nil)
}

// TestRegtestBlockVersions ensures regtest enforces BIP34/66/65 version floors
// from height 1.
func TestRegtestBlockVersions(t *testing.T) {
	params := chaincfg.RegressionNetParams
	prevNode := regtestPrevNode(t)

	testCases := []struct {
		name    string
		version int32
		wantErr ErrorCode
	}{
		{
			name:    "v1_rejected",
			version: 1,
			wantErr: ErrBlockVersionTooOld,
		},
		{
			name:    "v2_rejected",
			version: 2,
			wantErr: ErrBlockVersionTooOld,
		},
		{
			name:    "v3_rejected",
			version: 3,
			wantErr: ErrBlockVersionTooOld,
		},
		{
			name:    "v4_allowed",
			version: 4,
			wantErr: ErrorCode(0),
		},
		{
			name:    "vb_signal_allowed",
			version: 0x20000000,
			wantErr: ErrorCode(0),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			block := &wire.BlockHeader{
				Version:   tc.version,
				PrevBlock: *params.GenesisHash,
				Bits:      params.PowLimitBits,
				Timestamp: time.Unix(prevNode.Timestamp()+1, 0),
			}

			err := CheckBlockHeaderContext(
				block, prevNode, BFFastAdd,
				stubChainCtx{params: &params}, true,
			)

			if tc.wantErr == 0 {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)

			var rErr RuleError
			require.ErrorAs(t, err, &rErr)
			require.Equal(t, tc.wantErr, rErr.ErrorCode)
		})
	}
}

// TestRegtestBuriedDeploymentsAlwaysActive asserts CSV, SegWit, and Taproot
// are always active on regtest, mirroring Bitcoin Core.
func TestRegtestBuriedDeploymentsAlwaysActive(t *testing.T) {
	chain := newFakeChain(&chaincfg.RegressionNetParams)
	prevNode := chain.bestChain.Tip()

	stateCSV, err := chain.deploymentState(prevNode, chaincfg.DeploymentCSV)
	require.NoError(t, err)
	require.Equal(t, ThresholdActive, stateCSV)

	stateSegwit, err := chain.deploymentState(
		prevNode, chaincfg.DeploymentSegwit,
	)
	require.NoError(t, err)
	require.Equal(t, ThresholdActive, stateSegwit)

	stateTaproot, err := chain.deploymentState(
		prevNode, chaincfg.DeploymentTaproot,
	)
	require.NoError(t, err)
	require.Equal(t, ThresholdActive, stateTaproot)
}

// TestRegtestRejectsCoinbaseMissingHeight ensures contextual validation
// enforces coinbase height serialization on regtest.
func TestRegtestRejectsCoinbaseMissingHeight(t *testing.T) {
	chain := newFakeChain(&chaincfg.RegressionNetParams)
	prevNode := chain.bestChain.Tip()

	block := wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:   4,
			PrevBlock: prevNode.hash,
			Bits:      chain.chainParams.PowLimitBits,
			Timestamp: time.Unix(prevNode.timestamp+1, 0),
		},
	}

	coinbase := wire.NewMsgTx(wire.TxVersion)
	coinbase.AddTxIn(&wire.TxIn{SignatureScript: []byte{}})
	coinbase.AddTxOut(&wire.TxOut{Value: 0})
	block.AddTransaction(coinbase)
	block.Header.MerkleRoot = block.Transactions[0].TxHash()

	btcBlock := btcutil.NewBlock(&block)

	// The block without a height in coinbase is not considered valid.
	err := chain.checkBlockContext(btcBlock, prevNode, BFNone)
	require.Error(t, err)

	// Make sure the error is in coinbase height record.
	var rErr RuleError
	require.ErrorAs(t, err, &rErr)
	require.Equal(t, ErrMissingCoinbaseHeight, rErr.ErrorCode)
}

// TestRegtestAcceptsCoinbaseHeight ensures a properly encoded coinbase height
// passes contextual checks on regtest.
func TestRegtestAcceptsCoinbaseHeight(t *testing.T) {
	chain := newFakeChain(&chaincfg.RegressionNetParams)
	prevNode := chain.bestChain.Tip()

	coinbaseScript, err := txscript.NewScriptBuilder().AddInt64(1).Script()
	require.NoError(t, err)

	block := wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:   4,
			PrevBlock: prevNode.hash,
			Bits:      chain.chainParams.PowLimitBits,
			Timestamp: time.Unix(prevNode.timestamp+1, 0),
		},
	}

	coinbase := wire.NewMsgTx(wire.TxVersion)
	coinbase.AddTxIn(&wire.TxIn{SignatureScript: coinbaseScript})
	coinbase.AddTxOut(&wire.TxOut{Value: 0})
	block.AddTransaction(coinbase)
	block.Header.MerkleRoot = block.Transactions[0].TxHash()

	btcBlock := btcutil.NewBlock(&block)

	// Make sure the block with the height in coinbase is considered valid.
	require.NoError(t, chain.checkBlockContext(btcBlock, prevNode, BFNone))
}
