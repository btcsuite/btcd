package blockchain

import (
	"testing"

	"github.com/btcsuite/btcd/blockchain/internal/testhelper"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/stretchr/testify/require"
)

func TestBlockHashByTxHash(t *testing.T) {
	t.Parallel()

	require := require.New(t)
	chain, teardown, err := chainSetup("blockhashbytxhash", &chaincfg.RegressionNetParams)
	require.NoError(err)
	defer teardown()

	chain.TstSetCoinbaseMaturity(1)
	prev := btcutil.NewBlock(chain.chainParams.GenesisBlock)
	prev.SetHeight(0)

	block1, outs, err := addBlock(chain, prev, nil)
	require.NoError(err)

	block2, spendOuts, err := addBlock(chain, block1, []*testhelper.SpendableOut{outs[0]})
	require.NoError(err)

	unspentTxHash := spendOuts[1].PrevOut.Hash
	resolvedHash, err := chain.BlockHashByTxHash(&unspentTxHash)
	require.NoError(err)
	require.NotNil(resolvedHash)
	require.True(block2.Hash().IsEqual(resolvedHash))

	spentTxHash := outs[0].PrevOut.Hash
	resolvedHash, err = chain.BlockHashByTxHash(&spentTxHash)
	require.NoError(err)
	require.Nil(resolvedHash)

	missingHash := chainhash.DoubleHashH([]byte("missing"))
	resolvedHash, err = chain.BlockHashByTxHash(&missingHash)
	require.NoError(err)
	require.Nil(resolvedHash)
}
