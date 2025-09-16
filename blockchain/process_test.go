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
			Nonce:      0, // To be solved.
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
		if err != nil {
			t.Fatal(err)
		}

		if !isMainChain {
			t.Fatalf("expected header %v to be in the main chain",
				header.BlockHash())
		}
	}

	// Check that the tip is correct.
	lastHeader := headers[len(headers)-1]
	lastHeaderHash := lastHeader.BlockHash()
	tipNode := chain.bestHeader.Tip()
	if !tipNode.hash.IsEqual(&lastHeaderHash) {
		t.Fatalf("expected %v but got %v",
			lastHeaderHash.String(), tipNode.hash.String())
	}
	if tipNode.status != statusHeaderStored {
		t.Fatalf("expected statusHeaderStored but got %v",
			tipNode.status)
	}
	if tipNode.height != int32(len(headers)) {
		t.Fatalf("expected height of %v but got %v",
			len(headers), tipNode.height)
	}

	// Create invalid header at the checkpoint.
	//
	thirdHeaderHash := headers[2].BlockHash()
	thirdNode := chain.index.LookupNode(&thirdHeaderHash)
	invalidForkHeight := thirdNode.height
	invalidHeaders := chainedHeaders(headers[2], params, invalidForkHeight, 1)

	// Check that the header fails validation.
	_, err := chain.ProcessBlockHeader(invalidHeaders[0], BFNone, false)
	if err == nil {
		err := fmt.Errorf("invalidHeader %v passed verification but "+
			"should've failed verification "+
			"as the header doesn't match the checkpoint",
			invalidHeaders[0].BlockHash().String(),
		)
		t.Fatal(err)
	}

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
	if err == nil {
		err := fmt.Errorf("sideChainHeader %v passed verification but "+
			"should've failed verification"+
			"as the previous header is not known",
			sideChainHeaders[len(sideChainHeaders)-1].BlockHash().String(),
		)
		t.Fatal(err)
	}

	// Verify that the side-chain headers verify.
	for _, header := range sideChainHeaders {
		isMainChain, err := chain.ProcessBlockHeader(header, BFNone, false)
		if err != nil {
			t.Fatal(err)
		}
		if isMainChain {
			t.Fatalf("expected header %v to not be in the main chain",
				header.BlockHash())
		}
	}

	// Check that the tip is still the same as before.
	if !tipNode.hash.IsEqual(&lastHeaderHash) {
		t.Fatalf("expected %v but got %v", lastHeaderHash.String(), tipNode.hash.String())
	}
	if tipNode.status != statusHeaderStored {
		t.Fatalf("expected statusHeaderStored but got %v", tipNode.status)
	}
	if tipNode.height != int32(len(headers)) {
		t.Fatalf("expected height of %v but got %v", len(headers), tipNode.height)
	}

	// Verify that the side-chain extending headers verify.
	sidechainExtendingHeaders := chainedHeaders(
		sidechainTip, params, forkHeight+int32(len(sideChainHeaders)), 10)
	for _, header := range sidechainExtendingHeaders {
		isMainChain, err := chain.ProcessBlockHeader(header, BFNone, false)
		if err != nil {
			t.Fatal(err)
		}
		blockHash := header.BlockHash()
		node := chain.index.LookupNode(&blockHash)
		if node.height <= 10 {
			if isMainChain {
				t.Fatalf("expected header %v to not be in the main chain",
					header.BlockHash())
			}
		} else {
			if !isMainChain {
				t.Fatalf("expected header %v to be in the main chain",
					header.BlockHash())
			}
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
	if !tipNode.hash.IsEqual(&lastSidechainHeaderHash) {
		t.Fatalf("expected %v but got %v",
			lastSidechainHeaderHash.String(), tipNode.hash.String())
	}
	if tipNode.status != statusHeaderStored {
		t.Fatalf("expected statusHeaderStored but got %v", tipNode.status)
	}
	if tipNode.height !=
		int32(len(sideChainHeaders)+len(sidechainExtendingHeaders))+forkHeight {
		t.Fatalf("expected height of %v but got %v", len(headers), tipNode.height)
	}

	// Extend the original headers and check it still verifies.
	extendedOrigHeaders := chainedHeaders(lastHeader, params, int32(len(headers)), 2)
	for _, header := range extendedOrigHeaders {
		isMainChain, err := chain.ProcessBlockHeader(header, BFNone, false)
		if err != nil {
			t.Fatal(err)
		}
		if isMainChain {
			t.Fatalf("expected header %v to not be in the main chain",
				header.BlockHash())
		}
	}

	// Check that the tip didn't change.
	if !tipNode.hash.IsEqual(&lastSidechainHeaderHash) {
		t.Fatalf("expected %v but got %v",
			lastSidechainHeaderHash.String(), tipNode.hash.String())
	}
}
