package blockchain

import (
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/btcsuite/btcd/blockchain/internal/testhelper"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// chainedHeaders returns desired amount of connected headers from the parentHeight.
func chainedHeaders(parent *wire.BlockHeader, chainParams *chaincfg.Params,
	parentHeight int32, numHeaders int) []*wire.BlockHeader {

	headers := make([]*wire.BlockHeader, 0, numHeaders)
	tip := parent

	blockHeight := parentHeight
	for range numHeaders {
		// Use a timestamp that is one second after the previous block unless
		// this is the first block in which case the current time is used.
		var ts time.Time
		if blockHeight == 1 {
			ts = time.Unix(time.Now().Unix(), 0)
		} else {
			ts = tip.Timestamp.Add(time.Second)
		}

		var randBytes [4]byte
		rand.Read(randBytes[:])
		merkle := chainhash.HashH(randBytes[:])

		header := wire.BlockHeader{
			Version:    1,
			PrevBlock:  tip.BlockHash(),
			MerkleRoot: merkle,
			Bits:       chainParams.PowLimitBits,
			Timestamp:  ts,
			Nonce:      0,
		}
		if !testhelper.SolveBlock(&header) {
			panic(fmt.Sprintf("Unable to solve block at height %d",
				blockHeight))
		}
		headers = append(headers, &header)
		tip = &header
	}

	return headers
}

func TestProcessBlockHeader(t *testing.T) {
	chain, params, tearDown := utxoCacheTestChain("TestProcessBlockHeader")
	defer tearDown()

	// Generate and process the intial 10 block headers.
	//
	// genesis -> 1  -> 2  -> ...  -> 10 (active)
	headers := chainedHeaders(&params.GenesisBlock.Header, params, 0, 10)

	// Set checkpoint at block 4.
	fourthHeader := headers[3]
	fourthHeaderHash := fourthHeader.BlockHash()
	checkpoint := chaincfg.Checkpoint{
		Height: 4,
		Hash:   &fourthHeaderHash,
	}
	chain.checkpoints = append(chain.checkpoints, checkpoint)
	chain.checkpointsByHeight = make(map[int32]*chaincfg.Checkpoint)
	chain.checkpointsByHeight[checkpoint.Height] = &checkpoint

	for _, header := range headers {
		isMainChain, err := chain.ProcessBlockHeader(header, BFNone, false)
		require.NoError(t, err)
		require.True(t, isMainChain)
	}

	// Check that the tip is correct.
	lastHeader := headers[len(headers)-1]
	lastHeaderHash := lastHeader.BlockHash()
	tipNode := chain.bestHeader.Tip()
	require.Equal(t, lastHeaderHash, tipNode.hash)
	require.Equal(t, statusHeaderStored, tipNode.status)
	require.Equal(t, int32(len(headers)), tipNode.height)

	// Create invalid header at the checkpoint.
	thirdHeaderHash := headers[2].BlockHash()
	thirdNode := chain.index.LookupNode(&thirdHeaderHash)
	invalidForkHeight := thirdNode.height
	invalidHeaders := chainedHeaders(headers[2], params, invalidForkHeight, 1)

	// Check that the header fails validation.
	_, err := chain.ProcessBlockHeader(invalidHeaders[0], BFNone, false)
	require.Errorf(t, err,
		"invalidHeader %v passed verification but "+
			"should've failed verification "+
			"as the header doesn't match the checkpoint",
		invalidHeaders[0].BlockHash().String(),
	)

	// Create sidechain block headers.
	//
	// genesis -> 1  -> 2  -> 3  -> 4  -> 5  -> ... -> 10 (active)
	//                                      \-> 6  -> ... -> 8 (valid-fork)
	blockHash := headers[4].BlockHash()
	node := chain.index.LookupNode(&blockHash)
	forkHeight := node.height
	sideChainHeaders := chainedHeaders(headers[4], params, node.height, 3)
	sidechainTip := sideChainHeaders[len(sideChainHeaders)-1]

	// Test that the last block header fails as it's missing the previous block
	// header.
	_, err = chain.ProcessBlockHeader(sidechainTip, BFNone, false)
	require.Errorf(t, err,
		"sideChainHeader %v passed verification but "+
			"should've failed verification"+
			"as the previous header is not known",
		sideChainHeaders[len(sideChainHeaders)-1].BlockHash().String(),
	)

	// Verify that the side-chain headers verify.
	for _, header := range sideChainHeaders {
		isMainChain, err := chain.ProcessBlockHeader(header, BFNone, false)
		require.NoError(t, err)
		require.False(t, isMainChain)
	}

	// Check that the tip is still the same as before.
	require.Equal(t, lastHeaderHash, tipNode.hash)
	require.Equal(t, statusHeaderStored, tipNode.status)
	require.Equal(t, int32(len(headers)), tipNode.height)

	// Verify that the side-chain extending headers verify.
	sidechainExtendingHeaders := chainedHeaders(
		sidechainTip, params, forkHeight+int32(len(sideChainHeaders)), 10)
	for _, header := range sidechainExtendingHeaders {
		isMainChain, err := chain.ProcessBlockHeader(header, BFNone, false)
		require.NoError(t, err)

		blockHash := header.BlockHash()
		node := chain.index.LookupNode(&blockHash)
		if node.height <= 10 {
			require.False(t, isMainChain)
		} else {
			require.True(t, isMainChain)
		}
	}

	// Create more sidechain block headers so that it becomes the active chain.
	//
	// 	genesis -> 1  -> 2  -> 3  -> 4  -> 5  -> ... -> 10 (valid-fork)
	//                                            \-> 6  -> ... -> 18 (active)
	lastSideChainHeaderIdx := len(sidechainExtendingHeaders) - 1
	lastSidechainHeader := sidechainExtendingHeaders[lastSideChainHeaderIdx]
	lastSidechainHeaderHash := lastSidechainHeader.BlockHash()

	// Check that the tip is now different.
	tipNode = chain.bestHeader.Tip()
	require.Equal(t, lastSidechainHeaderHash, tipNode.hash)
	require.Equal(t, statusHeaderStored, tipNode.status)
	require.Equal(t,
		int32(len(sideChainHeaders)+len(sidechainExtendingHeaders))+forkHeight,
		tipNode.height)

	// Extend the original headers and check it still verifies.
	extendedOrigHdrs := chainedHeaders(lastHeader, params, int32(len(headers)), 2)
	for _, header := range extendedOrigHdrs {
		isMainChain, err := chain.ProcessBlockHeader(header, BFNone, false)
		require.NoError(t, err)
		require.False(t, isMainChain)
	}

	// Check that the tip didn't change.
	require.Equal(t, lastSidechainHeaderHash, tipNode.hash)
}
