// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"testing"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/wire"
)

// newHashFromStr converts the passed big-endian hex string into a
// chainhash.Hash.  It only differs from the one available in chainhash in that
// it ignores the error since it will only (and must only) be called with
// hard-coded, and therefore known good, hashes.
func newHashFromStr(hexStr string) *chainhash.Hash {
	hash, _ := chainhash.NewHashFromStr(hexStr)
	return hash
}

// hexToFinalState converts the passed hex string into an array of 6 bytes and
// will panic if there is an error.  This is only provided for the hard-coded
// constants so errors in the source code can be detected. It will only (and
// must only) be called with hard-coded values.
func hexToFinalState(s string) [6]byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}

	var finalState [6]byte
	if len(b) != len(finalState) {
		panic("invalid hex in source file: " + s)
	}
	copy(finalState[:], b)
	return finalState
}

// hexToExtraData converts the passed hex string into an array of 32 bytes and
// will panic if there is an error.  This is only provided for the hard-coded
// constants so errors in the source code can be detected. It will only (and
// must only) be called with hard-coded values.
func hexToExtraData(s string) [32]byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}

	var extraData [32]byte
	if len(b) != len(extraData) {
		panic("invalid hex in source file: " + s)
	}
	copy(extraData[:], b)
	return extraData
}

// DoStxoTest does a test on a simulated blockchain to ensure that the data
// stored in the STXO buckets is not corrupt.
func (b *BlockChain) DoStxoTest() error {
	return b.db.View(func(dbTx database.Tx) error {
		for i := int64(2); i <= b.bestNode.height; i++ {
			block, err := dbFetchBlockByHeight(dbTx, i)
			if err != nil {
				return err
			}

			parent, err := dbFetchBlockByHeight(dbTx, i-1)
			if err != nil {
				return err
			}

			ntx := countSpentOutputs(block, parent)
			stxos, err := dbFetchSpendJournalEntry(dbTx, block,
				parent)
			if err != nil {
				return err
			}

			if ntx != len(stxos) {
				return fmt.Errorf("bad number of stxos "+
					"calculated at "+"height %v, got %v "+
					"expected %v", i, len(stxos), ntx)
			}
		}

		return nil
	})
}

// TestErrNotInMainChain ensures the functions related to errNotInMainChain work
// as expected.
func TestErrNotInMainChain(t *testing.T) {
	errStr := "no block at height 1 exists"
	err := error(errNotInMainChain(errStr))

	// Ensure the stringized output for the error is as expected.
	if err.Error() != errStr {
		t.Fatalf("errNotInMainChain retuned unexpected error string - "+
			"got %q, want %q", err.Error(), errStr)
	}

	// Ensure error is detected as the correct type.
	if !isNotInMainChainErr(err) {
		t.Fatalf("isNotInMainChainErr did not detect as expected type")
	}
	err = errors.New("something else")
	if isNotInMainChainErr(err) {
		t.Fatalf("isNotInMainChainErr detected incorrect type")
	}
}

// TestBlockNodeSerialization ensures serializing and deserializing block nodes
// works as expected.
func TestBlockNodeSerialization(t *testing.T) {
	t.Parallel()

	// baseNode is based on block 150287 on mainnet and serves as a template
	// for the various tests below.
	baseNode := blockNode{
		hash:         *newHashFromStr("000000000000005d9820c21dc15d859cc18d660171aaf74fe835d736eaaffc26"),
		blockVersion: 4,
		parentHash:   *newHashFromStr("000000000000016916671ae225343a5ee131c999d5cadb6348805db25737731f"),
		merkleRoot:   *newHashFromStr("5ef2bb79795d7503c0ccc5cb6e0d4731992fc8c8c5b332c1c0e2c687d864c666"),
		stakeRoot:    *newHashFromStr("022965059b7527dc2bc18daaa533f806eda1f96fd0b04bbda2381f5552d7c2de"),
		voteBits:     0x0001,
		finalState:   hexToFinalState("313e16e64c0b"),
		voters:       4,
		freshStake:   3,
		revocations:  2,
		poolSize:     41332,
		bits:         0x1a016f98,
		sbits:        7473162478,
		height:       150287,
		blockSize:    11295,
		timestamp:    1499907127,
		nonce:        4116576260,
		extraData:    hexToExtraData("8f01ed92645e0a6b11ee3b3c0000000000000000000000000000000000000000"),
		stakeVersion: 4,
		status:       statusDataStored | statusValid,
		ticketsVoted: []chainhash.Hash{
			*newHashFromStr("8b62a877544753ea80a822142a48ec066170e9381d21a9e8a84bc7373f0f9b2e"),
			*newHashFromStr("4427a003a7aceb1404ffd9072e9aff1e128a24333a543332030e91668a389db7"),
			*newHashFromStr("4415b88ac74881d7b6b15d41df465257cd1cc92d55e95f1b648434aef3a2110b"),
			*newHashFromStr("9d2621b57352088809d3a069b04b76c832f30a76da14e56aece72208b3e5b87a"),
		},
		ticketsRevoked: []chainhash.Hash{
			*newHashFromStr("8146f01b8ffca8008ebc80293d2978d63b1dffa5c456a73e7b39a9b1e695e8eb"),
			*newHashFromStr("2292ff2461e725c58cc6e2051eac2a10e6ee6d1f62327ed676b7a196fb94be0c"),
		},
		votes: []stake.VoteVersionTuple{
			{Version: 4, Bits: 0x0001},
			{Version: 4, Bits: 0x0015},
			{Version: 4, Bits: 0x0015},
			{Version: 4, Bits: 0x0001},
		},
	}
	baseNode.workSum = CalcWork(baseNode.bits)

	tests := []struct {
		name       string
		node       blockNode
		serialized []byte
	}{
		{
			name: "no votes, no revokes",
			node: func() blockNode {
				node := baseNode
				node.ticketsVoted = nil
				node.votes = nil
				node.ticketsRevoked = nil
				return node
			}(),
			serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f833a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f011aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0a6b11ee3b3c000000000000000000000000000000000000000004000000030000"),
		},
		{
			name: "1 vote, no revokes",
			node: func() blockNode {
				node := baseNode
				node.ticketsVoted = node.ticketsVoted[:1]
				node.votes = node.votes[:1]
				node.ticketsRevoked = nil
				return node
			}(),
			serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f833a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f011aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0a6b11ee3b3c00000000000000000000000000000000000000000400000003012e9b0f3f37c74ba8e8a9211d38e9706106ec482a1422a880ea53475477a8628b040100"),
		},
		{
			name: "no votes, 1 revoke",
			node: func() blockNode {
				node := baseNode
				node.ticketsVoted = nil
				node.ticketsRevoked = node.ticketsRevoked[:1]
				node.votes = nil
				return node
			}(),
			serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f833a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f011aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0a6b11ee3b3c000000000000000000000000000000000000000004000000030001ebe895e6b1a9397b3ea756c4a5ff1d3bd678293d2980bc8e00a8fc8f1bf04681"),
		},
		{
			name:       "4 votes, same vote versions, different vote bits, 2 revokes",
			node:       baseNode,
			serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f833a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f011aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0a6b11ee3b3c00000000000000000000000000000000000000000400000003042e9b0f3f37c74ba8e8a9211d38e9706106ec482a1422a880ea53475477a8628b0401b79d388a66910e033233543a33248a121eff9a2e07d9ff0414ebaca703a0274404150b11a2f3ae3484641b5fe9552dc91ccd575246df415db1b6d78148c78ab8154404157ab8e5b30822e7ec6ae514da760af332c8764bb069a0d30988085273b521269d040102ebe895e6b1a9397b3ea756c4a5ff1d3bd678293d2980bc8e00a8fc8f1bf046810cbe94fb96a1b776d67e32621f6deee6102aac1e05e2c68cc525e76124ff9222"),
		},
	}

	for _, test := range tests {
		// Ensure the function to calculate the serialized size without
		// actually serializing it is calculated properly.
		gotSize := blockNodeSerializeSize(&test.node)
		if gotSize != len(test.serialized) {
			t.Errorf("blockNodeSerializeSize (%s): did not get "+
				"expected size - got %d, want %d", test.name,
				gotSize, len(test.serialized))
		}

		// Ensure the block node serializes to the expected value.
		gotSerialized, err := serializeBlockNode(&test.node)
		if err != nil {
			t.Errorf("serializeBlockNode (%s): unexpected error: %v",
				test.name, err)
			continue
		}
		if !bytes.Equal(gotSerialized, test.serialized) {
			t.Errorf("serializeBlockNode (%s): did not get expected "+
				"bytes - got %x, want %x", test.name,
				gotSerialized, test.serialized)
			continue
		}

		// Ensure the block node serializes to the expected value and
		// produces the expected number of bytes written via a direct
		// put.
		gotSerialized2 := make([]byte, gotSize)
		gotBytesWritten, err := putBlockNode(gotSerialized2, &test.node)
		if err != nil {
			t.Errorf("putBlockNode (%s): unexpected error: %v",
				test.name, err)
			continue
		}
		if !bytes.Equal(gotSerialized2, test.serialized) {
			t.Errorf("putBlockNode (%s): did not get expected "+
				"bytes - got %x, want %x", test.name,
				gotSerialized2, test.serialized)
			continue
		}
		if gotBytesWritten != len(test.serialized) {
			t.Errorf("putBlockNode (%s): did not get expected "+
				"number of bytes written - got %d, want %d",
				test.name, gotBytesWritten,
				len(test.serialized))
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// block node.
		gotNode, err := deserializeBlockNode(test.serialized)
		if err != nil {
			t.Errorf("deserializeBlockNode (%s): unexpected error: %v",
				test.name, err)
			continue
		}
		if !reflect.DeepEqual(*gotNode, test.node) {
			t.Errorf("deserializeBlockNode (%s): mismatched entries\ngot "+
				"%+v\nwant %+v", test.name, gotNode, test.node)
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// block node.
		var gotNode2 blockNode
		bytesRead, err := decodeBlockNode(test.serialized, &gotNode2)
		if err != nil {
			t.Errorf("decodeBlockNode (%s): unexpected error: %v",
				test.name, err)
			continue
		}
		if !reflect.DeepEqual(gotNode2, test.node) {
			t.Errorf("decodeBlockNode (%s): mismatched entries\ngot "+
				"%+v\nwant %+v", test.name, gotNode2, test.node)
			continue
		}
		if bytesRead != len(test.serialized) {
			t.Errorf("decodeBlockNode (%s): did not get expected "+
				"number of bytes read - got %d, want %d",
				test.name, bytesRead, len(test.serialized))
			continue
		}
	}
}

// TestBlockNodeDecodeErrors performs negative tests against decoding block
// nodes to ensure error paths work as expected.
func TestBlockNodeDecodeErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		node       blockNode
		serialized []byte
		bytesRead  int // Expected number of bytes read.
		errType    error
	}{
		{
			name:       "nothing serialized",
			node:       blockNode{},
			serialized: hexToBytes(""),
			errType:    errDeserialize(""),
			bytesRead:  0,
		},
		{
			name:       "no data after block header",
			node:       blockNode{},
			serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f833a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f011aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0a6b11ee3b3c000000000000000000000000000000000000000004000000"),
			errType:    errDeserialize(""),
			bytesRead:  180,
		},
		{
			name:       "no data after status",
			node:       blockNode{},
			serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f833a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f011aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0a6b11ee3b3c00000000000000000000000000000000000000000400000003"),
			errType:    errDeserialize(""),
			bytesRead:  181,
		},
		{
			name:       "no data after num votes with no votes",
			node:       blockNode{},
			serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f833a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f011aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0a6b11ee3b3c0000000000000000000000000000000000000000040000000300"),
			errType:    errDeserialize(""),
			bytesRead:  182,
		},
		{
			name:       "no data after num votes with votes",
			node:       blockNode{},
			serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f833a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f011aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0a6b11ee3b3c0000000000000000000000000000000000000000040000000301"),
			errType:    errDeserialize(""),
			bytesRead:  182,
		},
		{
			name:       "short data in vote ticket hash",
			node:       blockNode{},
			serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f833a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f011aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0a6b11ee3b3c00000000000000000000000000000000000000000400000003012e9b0f3f37c74ba8e8a9211d38e9706106ec482a1422a880ea53475477a862"),
			errType:    errDeserialize(""),
			bytesRead:  182,
		},
		{
			name:       "no data after vote ticket hash",
			node:       blockNode{},
			serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f833a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f011aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0a6b11ee3b3c00000000000000000000000000000000000000000400000003012e9b0f3f37c74ba8e8a9211d38e9706106ec482a1422a880ea53475477a8628b"),
			errType:    errDeserialize(""),
			bytesRead:  214,
		},
		{
			name:       "no data after vote version",
			node:       blockNode{},
			serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f833a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f011aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0a6b11ee3b3c00000000000000000000000000000000000000000400000003012e9b0f3f37c74ba8e8a9211d38e9706106ec482a1422a880ea53475477a8628b04"),
			errType:    errDeserialize(""),
			bytesRead:  215,
		},
		{
			name:       "no data after votes",
			node:       blockNode{},
			serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f833a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f011aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0a6b11ee3b3c00000000000000000000000000000000000000000400000003012e9b0f3f37c74ba8e8a9211d38e9706106ec482a1422a880ea53475477a8628b0401"),
			errType:    errDeserialize(""),
			bytesRead:  216,
		},
		{
			name:       "no data after num revokes with revokes",
			node:       blockNode{},
			serialized: hexToBytes("040000001f733757b25d804863dbcad599c931e15e3a3425e21a6716690100000000000066c664d887c6e2c0c132b3c5c8c82f9931470d6ecbc5ccc003755d7979bbf25edec2d752551f38a2bd4bb0d06ff9a1ed06f833a5aa8dc12bdc27759b056529020100313e16e64c0b0400030274a10000986f011aee686fbd010000000f4b02001f2c000037c4665904f85df58f01ed92645e0a6b11ee3b3c000000000000000000000000000000000000000004000000030001"),
			errType:    errDeserialize(""),
			bytesRead:  183,
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned.
		gotBytesRead, err := decodeBlockNode(test.serialized, &test.node)
		if reflect.TypeOf(err) != reflect.TypeOf(test.errType) {
			t.Errorf("decodeBlockNode (%s): expected error type "+
				"does not match - got %T, want %T", test.name,
				err, test.errType)
			continue
		}

		// Ensure the expected number of bytes read is returned.
		if gotBytesRead != test.bytesRead {
			t.Errorf("decodeBlockNode (%s): unexpected number of "+
				"bytes read - got %d, want %d", test.name,
				gotBytesRead, test.bytesRead)
			continue
		}
	}
}

// TestStxoSerialization ensures serializing and deserializing spent transaction
// output entries works as expected.
func TestStxoSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stxo       spentTxOut
		txVersion  int32 // When the txout is not fully spent.
		serialized []byte
	}{
		{
			name: "Spends last output of coinbase",
			stxo: spentTxOut{
				amount:        9999,
				scriptVersion: 0,
				pkScript:      hexToBytes("76a9146edbc6c4d31bae9f1ccc38538a114bf42de65e8688ac"),
				height:        12345,
				index:         54321,
				isCoinBase:    true,
				hasExpiry:     false,
				txFullySpent:  true,
				txType:        0,
				txVersion:     1,
			},
			serialized: hexToBytes("1100006edbc6c4d31bae9f1ccc38538a114bf42de65e8601"),
		},
		{
			name: "Spends last output of non coinbase and is a ticket",
			stxo: spentTxOut{
				amount:        9999,
				scriptVersion: 0,
				pkScript:      hexToBytes("76a9146edbc6c4d31bae9f1ccc38538a114bf42de65e8688ac"),
				height:        12345,
				index:         54321,
				isCoinBase:    false,
				hasExpiry:     false,
				txFullySpent:  true,
				txType:        1,
				txVersion:     1,
				stakeExtra:    []byte{0x00},
			},
			serialized: hexToBytes("1400006edbc6c4d31bae9f1ccc38538a114bf42de65e860100"),
		},
		{
			name: "Does not spend last output",
			stxo: spentTxOut{
				amount:        34405000000,
				pkScript:      hexToBytes("76a9146edbc6c4d31bae9f1ccc38538a114bf42de65e8688ac"),
				scriptVersion: 0,
			},
			txVersion:  1,
			serialized: hexToBytes("0000006edbc6c4d31bae9f1ccc38538a114bf42de65e86"),
		},
	}

	for _, test := range tests {
		// Ensure the function to calculate the serialized size without
		// actually serializing it is calculated properly.
		gotSize := spentTxOutSerializeSize(&test.stxo)
		if gotSize != len(test.serialized) {
			t.Errorf("spentTxOutSerializeSize (%s): did not get "+
				"expected size - got %d, want %d", test.name,
				gotSize, len(test.serialized))
		}

		// Ensure the stxo serializes to the expected value.
		gotSerialized := make([]byte, gotSize)
		gotBytesWritten := putSpentTxOut(gotSerialized, &test.stxo)
		if !bytes.Equal(gotSerialized, test.serialized) {
			t.Errorf("putSpentTxOut (%s): did not get expected "+
				"bytes - got %x, want %x", test.name,
				gotSerialized, test.serialized)
			continue
		}
		if gotBytesWritten != len(test.serialized) {
			t.Errorf("putSpentTxOut (%s): did not get expected "+
				"number of bytes written - got %d, want %d",
				test.name, gotBytesWritten,
				len(test.serialized))
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// stxo.
		var gotStxo spentTxOut
		offset, err := decodeSpentTxOut(test.serialized, &gotStxo,
			test.stxo.amount, test.stxo.height, test.stxo.index)
		if err != nil {
			t.Errorf("decodeSpentTxOut (%s): unexpected error: %v",
				test.name, err)
			continue
		}
		if offset != len(test.serialized) {
			t.Errorf("decodeSpentTxOut (%s): did not get expected "+
				"number of bytes read - got %d, want %d",
				test.name, offset, len(test.serialized))
			continue
		}
	}
}

// TestStxoDecodeErrors performs negative tests against decoding spent
// transaction outputs to ensure error paths work as expected.
func TestStxoDecodeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stxo       spentTxOut
		txVersion  int32 // When the txout is not fully spent.
		serialized []byte
		bytesRead  int // Expected number of bytes read.
		errType    error
	}{
		{
			name:       "nothing serialized",
			stxo:       spentTxOut{},
			serialized: hexToBytes(""),
			errType:    errDeserialize(""),
			bytesRead:  0,
		},
		{
			name:       "no data after flags w/o version",
			stxo:       spentTxOut{},
			serialized: hexToBytes("00"),
			errType:    errDeserialize(""),
			bytesRead:  1,
		},
		{
			name:       "no data after flags code",
			stxo:       spentTxOut{},
			serialized: hexToBytes("14"),
			errType:    errDeserialize(""),
			bytesRead:  1,
		},
		{
			name:       "no tx version data after script",
			stxo:       spentTxOut{},
			serialized: hexToBytes("1400016edbc6c4d31bae9f1ccc38538a114bf42de65e86"),
			errType:    errDeserialize(""),
			bytesRead:  23,
		},
		{
			name:       "no stakeextra data after script for ticket",
			stxo:       spentTxOut{},
			serialized: hexToBytes("1400016edbc6c4d31bae9f1ccc38538a114bf42de65e8601"),
			errType:    errDeserialize(""),
			bytesRead:  24,
		},
		{
			name:       "incomplete compressed txout",
			stxo:       spentTxOut{},
			txVersion:  1,
			serialized: hexToBytes("1432"),
			errType:    errDeserialize(""),
			bytesRead:  2,
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned.
		gotBytesRead, err := decodeSpentTxOut(test.serialized,
			&test.stxo, test.stxo.amount, test.stxo.height, test.stxo.index)
		if reflect.TypeOf(err) != reflect.TypeOf(test.errType) {
			t.Errorf("decodeSpentTxOut (%s): expected error type "+
				"does not match - got %T, want %T", test.name,
				err, test.errType)
			continue
		}

		// Ensure the expected number of bytes read is returned.
		if gotBytesRead != test.bytesRead {
			t.Errorf("decodeSpentTxOut (%s): unexpected number of "+
				"bytes read - got %d, want %d", test.name,
				gotBytesRead, test.bytesRead)
			continue
		}
	}
}

// TestSpendJournalSerialization ensures serializing and deserializing spend
// journal entries works as expected.
func TestSpendJournalSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entry      []spentTxOut
		blockTxns  []*wire.MsgTx
		utxoView   *UtxoViewpoint
		serialized []byte
	}{
		{
			name:       "No spends",
			entry:      nil,
			blockTxns:  nil,
			utxoView:   NewUtxoViewpoint(),
			serialized: nil,
		},
		{
			name: "One tx with one input spends last output of coinbase",
			entry: []spentTxOut{{
				amount:        5000000000,
				scriptVersion: 0,
				pkScript:      hexToBytes("0511db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c"),
				compressed:    true,
				height:        9,
				index:         0,
				isCoinBase:    true,
				hasExpiry:     false,
				txFullySpent:  true,
				txType:        0,
				txVersion:     1,
				stakeExtra:    nil,
			}},
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("0437cd7f8525ceed2324359c2d0ba26006d92d856a9c20fa0241106ee5a597c9"),
						Index: 0,
						Tree:  0,
					},
					SignatureScript: hexToBytes("47304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901"),
					Sequence:        0xffffffff,
					BlockHeight:     9,
					BlockIndex:      0,
					ValueIn:         5000000000,
				}},
				TxOut: []*wire.TxOut{{
					Value:    1000000000,
					Version:  0,
					PkScript: hexToBytes("4104ae1a62fe09c5f51b13905f07f06b99a2f7159b2225f374cd378d71302fa28414e7aab37397f554a7df5f142c21c1b7303b8a0626f1baded5c72a704f7e6cd84cac"),
				}, {
					Value:    4000000000,
					Version:  0,
					PkScript: hexToBytes("410411db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3ac"),
				}},
				LockTime: 0,
				Expiry:   0,
			}},
			utxoView:   NewUtxoViewpoint(),
			serialized: hexToBytes("11000511db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c01"),
		},
		{
			name: "Two txns when one spends last output, one doesn't",
			entry: []spentTxOut{{
				amount:        34405000000,
				height:        321,
				index:         123,
				scriptVersion: 0,
				pkScript:      hexToBytes("016edbc6c4d31bae9f1ccc38538a114bf42de65e86"),
				compressed:    true,
			}, {
				amount:        13761000000,
				scriptVersion: 0,
				pkScript:      hexToBytes("01b2fb57eadf61e106a100a7445a8c3f67898841ec"),
				compressed:    true,
				height:        3214,
				index:         1234,
				isCoinBase:    false,
				hasExpiry:     false,
				txFullySpent:  true,
				txType:        0,
				txVersion:     1,
				stakeExtra:    nil,
			}},
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("c0ed017828e59ad5ed3cf70ee7c6fb0f426433047462477dc7a5d470f987a537"),
						Index: 1,
						Tree:  0,
					},
					SignatureScript: hexToBytes("493046022100c167eead9840da4a033c9a56470d7794a9bb1605b377ebe5688499b39f94be59022100fb6345cab4324f9ea0b9ee9169337534834638d818129778370f7d378ee4a325014104d962cac5390f12ddb7539507065d0def320d68c040f2e73337c3a1aaaab7195cb5c4d02e0959624d534f3c10c3cf3d73ca5065ebd62ae986b04c6d090d32627c"),
					Sequence:        0xffffffff,
					BlockHeight:     321,
					BlockIndex:      123,
					ValueIn:         34405000000,
				}},
				TxOut: []*wire.TxOut{{
					Value:    5000000,
					Version:  0,
					PkScript: hexToBytes("76a914f419b8db4ba65f3b6fcc233acb762ca6f51c23d488ac"),
				}, {
					Value:    34400000000,
					Version:  0,
					PkScript: hexToBytes("76a914cadf4fc336ab3c6a4610b75f31ba0676b7f663d288ac"),
				}},
				LockTime: 0,
				Expiry:   0,
			}, {
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("92fbe1d4be82f765dfabc9559d4620864b05cc897c4db0e29adac92d294e52b7"),
						Index: 0,
						Tree:  0,
					},
					SignatureScript: hexToBytes("483045022100e256743154c097465cf13e89955e1c9ff2e55c46051b627751dee0144183157e02201d8d4f02cde8496aae66768f94d35ce54465bd4ae8836004992d3216a93a13f00141049d23ce8686fe9b802a7a938e8952174d35dd2c2089d4112001ed8089023ab4f93a3c9fcd5bfeaa9727858bf640dc1b1c05ec3b434bb59837f8640e8810e87742"),
					Sequence:        0xffffffff,
					BlockHeight:     3214,
					BlockIndex:      1234,
					ValueIn:         13761000000,
				}},
				TxOut: []*wire.TxOut{{
					Value:    5000000,
					Version:  0,
					PkScript: hexToBytes("76a914a983ad7c92c38fc0e2025212e9f972204c6e687088ac"),
				}, {
					Value:    13756000000,
					Version:  0,
					PkScript: hexToBytes("76a914a6ebd69952ab486a7a300bfffdcb395dc7d47c2388ac"),
				}},
				LockTime: 0,
				Expiry:   0,
			}},
			utxoView: &UtxoViewpoint{entries: map[chainhash.Hash]*UtxoEntry{
				*newHashFromStr("c0ed017828e59ad5ed3cf70ee7c6fb0f426433047462477dc7a5d470f987a537"): {
					txVersion:  1,
					isCoinBase: false,
					hasExpiry:  false,
					height:     321,
					index:      123,
					sparseOutputs: map[uint32]*utxoOutput{
						1: {
							amount:        34405000000,
							scriptVersion: 0,
							pkScript:      hexToBytes("76a9142084541c3931677527a7eafe56fd90207c344eb088ac"),
							compressed:    false,
						},
					},
				},
			}},
			serialized: hexToBytes("100001b2fb57eadf61e106a100a7445a8c3f67898841ec010000016edbc6c4d31bae9f1ccc38538a114bf42de65e86"),
		},
		{
			name: "One tx, two inputs from same tx, neither spend last output",
			entry: []spentTxOut{{
				amount:        159747816,
				height:        1111,
				index:         2222,
				scriptVersion: 0,
				pkScript:      hexToBytes("4151"),
				compressed:    true,
			}, {
				amount:        159747816,
				height:        3333,
				index:         4444,
				scriptVersion: 0,
				pkScript:      hexToBytes("4151"),
				compressed:    true,
			}},
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("c0ed017828e59ad5ed3cf70ee7c6fb0f426433047462477dc7a5d470f987a537"),
						Index: 1,
					},
					SignatureScript: hexToBytes(""),
					Sequence:        0xffffffff,
					BlockHeight:     1111,
					BlockIndex:      2222,
					ValueIn:         159747816,
				}, {
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("c0ed017828e59ad5ed3cf70ee7c6fb0f426433047462477dc7a5d470f987a537"),
						Index: 2,
					},
					SignatureScript: hexToBytes(""),
					Sequence:        0xffffffff,
					BlockHeight:     3333,
					BlockIndex:      4444,
					ValueIn:         159747816,
				}},
				TxOut: []*wire.TxOut{{
					Value:    165125632,
					Version:  0,
					PkScript: hexToBytes("51"),
				}, {
					Value:    154370000,
					Version:  0,
					PkScript: hexToBytes("51"),
				}},
				LockTime: 0,
				Expiry:   0,
			}},
			utxoView: &UtxoViewpoint{entries: map[chainhash.Hash]*UtxoEntry{
				*newHashFromStr("c0ed017828e59ad5ed3cf70ee7c6fb0f426433047462477dc7a5d470f987a537"): {
					txVersion:  1,
					isCoinBase: false,
					hasExpiry:  false,
					height:     100000,
					sparseOutputs: map[uint32]*utxoOutput{
						0: {
							amount:   165712179,
							pkScript: hexToBytes("51"),
						},
					},
				},
			}},
			serialized: hexToBytes("0000415100004151"),
		},
	}

	for i, test := range tests {
		// Ensure the journal entry serializes to the expected value.
		gotBytes, err := serializeSpendJournalEntry(test.entry)
		if err != nil {
			t.Errorf("serializeSpendJournalEntry #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeSpendJournalEntry #%d (%s): "+
				"mismatched bytes - got %x, want %x", i,
				test.name, gotBytes, test.serialized)
			continue
		}

		// Deserialize to a spend journal entry.
		gotEntry, err := deserializeSpendJournalEntry(test.serialized,
			test.blockTxns)
		if err != nil {
			t.Errorf("deserializeSpendJournalEntry #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}

		// Ensure that the deserialized spend journal entry has the
		// correct properties.
		for j := range gotEntry {
			if !reflect.DeepEqual(gotEntry[j], test.entry[j]) {
				t.Errorf("deserializeSpendJournalEntry #%d (%s) "+
					"mismatched entries in idx %v - got %v, want %v",
					i, test.name, j, gotEntry[j], test.entry[j])
				continue
			}
		}
	}
}

// TestSpendJournalErrors performs negative tests against deserializing spend
// journal entries to ensure error paths work as expected.
func TestSpendJournalErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		blockTxns  []*wire.MsgTx
		serialized []byte
		errType    error
	}{
		// Adapted from block 170 in main blockchain.
		{
			name: "Force assertion due to missing stxos",
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("0437cd7f8525ceed2324359c2d0ba26006d92d856a9c20fa0241106ee5a597c9"),
						Index: 0,
					},
					SignatureScript: hexToBytes("47304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901"),
					Sequence:        0xffffffff,
				}},
				LockTime: 0,
			}},
			serialized: hexToBytes(""),
			errType:    AssertError(""),
		},
		{
			name: "Force deserialization error in stxos",
			blockTxns: []*wire.MsgTx{{ // Coinbase omitted.
				SerType: wire.TxSerializeFull,
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: wire.OutPoint{
						Hash:  *newHashFromStr("0437cd7f8525ceed2324359c2d0ba26006d92d856a9c20fa0241106ee5a597c9"),
						Index: 0,
					},
					SignatureScript: hexToBytes("47304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901"),
					Sequence:        0xffffffff,
				}},
				LockTime: 0,
			}},
			serialized: hexToBytes("1301320511db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a"),
			errType:    errDeserialize(""),
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned and the returned
		// slice is nil.
		stxos, err := deserializeSpendJournalEntry(test.serialized,
			test.blockTxns)
		if reflect.TypeOf(err) != reflect.TypeOf(test.errType) {
			t.Errorf("deserializeSpendJournalEntry (%s): expected "+
				"error type does not match - got %T, want %T",
				test.name, err, test.errType)
			continue
		}
		if stxos != nil {
			t.Errorf("deserializeSpendJournalEntry (%s): returned "+
				"slice of spent transaction outputs is not nil",
				test.name)
			continue
		}
	}
}

// TestUtxoSerialization ensures serializing and deserializing unspent
// trasaction output entries works as expected.
func TestUtxoSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entry      *UtxoEntry
		serialized []byte
	}{
		{
			name: "Only output 0, coinbase, even uncomp pubkey",
			entry: &UtxoEntry{
				txVersion:  1,
				isCoinBase: true,
				hasExpiry:  false,
				txType:     0,
				height:     12345,
				index:      54321,
				stakeExtra: nil,
				sparseOutputs: map[uint32]*utxoOutput{
					0: {
						amount:        5000000000,
						scriptVersion: 0,
						pkScript:      hexToBytes("410496b538e853519c726a2c91e61ec11600ae1390813a627c66fb8be7947be63c52da7589379515d4e0a604f8141781e62294721166bf621e73a82cbf2342c858eeac"),
						compressed:    false,
					},
				},
			},
			serialized: hexToBytes("01df3982a731010132000496b538e853519c726a2c91e61ec11600ae1390813a627c66fb8be7947be63c52"),
		},
		{
			name: "Only output 0, coinbase, odd uncomp pubkey",
			entry: &UtxoEntry{
				txVersion:  1,
				isCoinBase: true,
				hasExpiry:  false,
				txType:     0,
				height:     12345,
				index:      54321,
				stakeExtra: nil,
				sparseOutputs: map[uint32]*utxoOutput{
					0: {
						amount:        5000000000,
						scriptVersion: 0,
						pkScript:      hexToBytes("410496b538e853519c726a2c91e61ec11600ae1390813a627c66fb8be7947be63c52258a76c86aea2b1f59fb07ebe87e19dd6b8dee99409de18c57d340dbbd37a341ac"),
						compressed:    false,
					},
				},
			},
			serialized: hexToBytes("01df3982a731010132000596b538e853519c726a2c91e61ec11600ae1390813a627c66fb8be7947be63c52"),
		},
		{
			name: "Only output 1, not coinbase",
			entry: &UtxoEntry{
				txVersion:  1,
				isCoinBase: false,
				hasExpiry:  false,
				txType:     0,
				height:     55555,
				index:      1,
				stakeExtra: nil,
				sparseOutputs: map[uint32]*utxoOutput{
					1: {
						amount:        1000000,
						scriptVersion: 0,
						pkScript:      hexToBytes("76a914ee8bd501094a7d5ca318da2506de35e1cb025ddc88ac"),
						compressed:    false,
					},
				},
			},
			serialized: hexToBytes("0182b103010002070000ee8bd501094a7d5ca318da2506de35e1cb025ddc"),
		},
		{
			name: "Ticket with one output",
			entry: &UtxoEntry{
				txVersion:  1,
				isCoinBase: false,
				hasExpiry:  false,
				txType:     1,
				height:     55555,
				index:      1,
				stakeExtra: hexToBytes("030f001aba76a9140cdf9941c0c221243cb8672cd1ad2c4c0933850588ac0000206a1e1a221182c26bbae681e4d96d452794e1951e70a208520000000000000054b5f466001abd76a9146c4f8b15918566534d134be7d7004b7f481bf36988ac"),
				sparseOutputs: map[uint32]*utxoOutput{
					1: {
						amount:        1000000,
						scriptVersion: 0,
						pkScript:      hexToBytes("76a914ee8bd501094a7d5ca318da2506de35e1cb025ddc88ac"),
						compressed:    false,
					},
				},
			},
			serialized: hexToBytes("0182b103010402070000ee8bd501094a7d5ca318da2506de35e1cb025ddc030f001aba76a9140cdf9941c0c221243cb8672cd1ad2c4c0933850588ac0000206a1e1a221182c26bbae681e4d96d452794e1951e70a208520000000000000054b5f466001abd76a9146c4f8b15918566534d134be7d7004b7f481bf36988ac"),
		},
		{
			name: "Only output 2, coinbase, non-zero script version",
			entry: &UtxoEntry{
				txVersion:  1,
				isCoinBase: true,
				hasExpiry:  false,
				txType:     0,
				height:     12345,
				index:      1,
				sparseOutputs: map[uint32]*utxoOutput{
					2: {
						amount:        100937281,
						scriptVersion: 0xffff,
						pkScript:      hexToBytes("76a914da33f77cee27c2a975ed5124d7e4f7f97513510188ac"),
						compressed:    false,
					},
				},
			},
			serialized: hexToBytes("01df390101000182b095bf4182fe7f00da33f77cee27c2a975ed5124d7e4f7f975135101"),
		},
		{
			name: "outputs 0 and 2 not coinbase",
			entry: &UtxoEntry{
				txVersion:  1,
				isCoinBase: false,
				hasExpiry:  false,
				txType:     0,
				height:     99999,
				index:      3,
				sparseOutputs: map[uint32]*utxoOutput{
					0: {
						amount:        20000000,
						scriptVersion: 0,
						pkScript:      hexToBytes("76a914e2ccd6ec7c6e2e581349c77e067385fa8236bf8a88ac"),
						compressed:    false,
					},
					2: {
						amount:        15000000,
						scriptVersion: 0,
						pkScript:      hexToBytes("76a914b8025be1b3efc63b0ad48e7f9f10e87544528d5888ac"),
						compressed:    false,
					},
				},
			},
			serialized: hexToBytes("01858c1f03000501120000e2ccd6ec7c6e2e581349c77e067385fa8236bf8a80090000b8025be1b3efc63b0ad48e7f9f10e87544528d58"),
		},
		{
			name: "outputs 0 and 2 not coinbase, has expiry",
			entry: &UtxoEntry{
				txVersion:  1,
				isCoinBase: false,
				hasExpiry:  true,
				txType:     0,
				height:     99999,
				index:      3,
				sparseOutputs: map[uint32]*utxoOutput{
					0: {
						amount:        20000000,
						scriptVersion: 0,
						pkScript:      hexToBytes("76a914e2ccd6ec7c6e2e581349c77e067385fa8236bf8a88ac"),
						compressed:    false,
					},
					2: {
						amount:        15000000,
						scriptVersion: 0,
						pkScript:      hexToBytes("76a914b8025be1b3efc63b0ad48e7f9f10e87544528d5888ac"),
						compressed:    false,
					},
				},
			},
			serialized: hexToBytes("01858c1f03020501120000e2ccd6ec7c6e2e581349c77e067385fa8236bf8a80090000b8025be1b3efc63b0ad48e7f9f10e87544528d58"),
		},
		{
			name: "outputs 0 and 2, not coinbase, 1 marked spent",
			entry: &UtxoEntry{
				txVersion:  1,
				isCoinBase: false,
				hasExpiry:  false,
				txType:     0,
				height:     12345,
				index:      1,
				sparseOutputs: map[uint32]*utxoOutput{
					0: {
						amount:        20000000,
						scriptVersion: 0,
						pkScript:      hexToBytes("76a914e2ccd6ec7c6e2e581349c77e067385fa8236bf8a88ac"),
						compressed:    false,
					},
					1: { // This won't be serialized.
						spent:         true,
						amount:        1000000,
						scriptVersion: 0,
						pkScript:      hexToBytes("76a914e43031c3e46f20bf1ccee9553ce815de5a48467588ac"),
						compressed:    false,
					},
					2: {
						amount:        15000000,
						scriptVersion: 0,
						pkScript:      hexToBytes("76a914b8025be1b3efc63b0ad48e7f9f10e87544528d5888ac"),
						compressed:    false,
					},
				},
			},
			serialized: hexToBytes("01df3901000501120000e2ccd6ec7c6e2e581349c77e067385fa8236bf8a80090000b8025be1b3efc63b0ad48e7f9f10e87544528d58"),
		},
		{
			name: "outputs 0 and 2, not coinbase, output 2 compressed",
			entry: &UtxoEntry{
				txVersion:  1,
				isCoinBase: false,
				hasExpiry:  false,
				txType:     0,
				height:     12345,
				index:      1,
				sparseOutputs: map[uint32]*utxoOutput{
					0: {
						amount:        20000000,
						scriptVersion: 0,
						pkScript:      hexToBytes("76a914e2ccd6ec7c6e2e581349c77e067385fa8236bf8a88ac"),
						compressed:    false,
					},
					2: {
						// Uncompressed Amount: 15000000
						// Uncompressed PkScript: 76a914b8025be1b3efc63b0ad48e7f9f10e87544528d5888ac
						amount: 137,

						pkScript:   hexToBytes("00b8025be1b3efc63b0ad48e7f9f10e87544528d58"),
						compressed: true,
					},
				},
			},
			serialized: hexToBytes("01df3901000501120000e2ccd6ec7c6e2e581349c77e067385fa8236bf8a884f0000b8025be1b3efc63b0ad48e7f9f10e87544528d58"),
		},
		{
			name: "outputs 0 and 2, not coinbase, output 2 compressed, packed indexes reversed",
			entry: &UtxoEntry{
				txVersion:  1,
				isCoinBase: false,
				hasExpiry:  false,
				txType:     0,
				height:     33333,
				index:      21,
				sparseOutputs: map[uint32]*utxoOutput{
					0: {
						amount:        20000000,
						scriptVersion: 0,
						pkScript:      hexToBytes("76a914e2ccd6ec7c6e2e581349c77e067385fa8236bf8a88ac"),
						compressed:    false,
					},
					2: {
						// Uncompressed Amount: 15000000
						// Uncompressed PkScript: 76a914b8025be1b3efc63b0ad48e7f9f10e87544528d5888ac
						amount:        137,
						scriptVersion: 0,
						pkScript:      hexToBytes("00b8025be1b3efc63b0ad48e7f9f10e87544528d58"),
						compressed:    true,
					},
				},
			},
			serialized: hexToBytes("0181833515000501120000e2ccd6ec7c6e2e581349c77e067385fa8236bf8a884f0000b8025be1b3efc63b0ad48e7f9f10e87544528d58"),
		},
		{
			name: "Only output 0, coinbase, fully spent",
			entry: &UtxoEntry{
				txVersion:  1,
				isCoinBase: false,
				hasExpiry:  false,
				txType:     0,
				height:     33333,
				index:      231,
				sparseOutputs: map[uint32]*utxoOutput{
					0: {
						spent:         true,
						amount:        5000000000,
						scriptVersion: 0,
						pkScript:      hexToBytes("410496b538e853519c726a2c91e61ec11600ae1390813a627c66fb8be7947be63c52da7589379515d4e0a604f8141781e62294721166bf621e73a82cbf2342c858eeac"),
						compressed:    false,
					},
				},
			},
			serialized: nil,
		},
		{
			name: "Only output 22, not coinbase",
			entry: &UtxoEntry{
				txVersion:  1,
				isCoinBase: false,
				hasExpiry:  false,
				txType:     0,
				height:     3221,
				index:      211,
				sparseOutputs: map[uint32]*utxoOutput{
					22: {
						spent:         false,
						amount:        366875659,
						scriptVersion: 0,
						pkScript:      hexToBytes("a9141dd46a006572d820e448e12d2bbb38640bc718e687"),
						compressed:    false,
					},
				},
			},
			serialized: hexToBytes("019815805300080000108ba5b9e76300011dd46a006572d820e448e12d2bbb38640bc718e6"),
		},
	}

	for i, test := range tests {
		// Ensure the utxo entry serializes to the expected value.
		gotBytes, err := serializeUtxoEntry(test.entry)
		if err != nil {
			t.Errorf("serializeUtxoEntry #%d (%s) unexpected "+
				"error: %v", i, test.name, err)
			continue
		}
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeUtxoEntry #%d (%s): mismatched "+
				"bytes - got %x, want %x", i, test.name,
				gotBytes, test.serialized)
			continue
		}

		// Don't try to deserialize if the test entry was fully spent
		// since it will have a nil serialization.
		if test.entry.IsFullySpent() {
			continue
		}

		// Deserialize to a utxo entry.
		utxoEntry, err := deserializeUtxoEntry(test.serialized)
		if err != nil {
			t.Errorf("deserializeUtxoEntry #%d (%s) unexpected "+
				"error: %v", i, test.name, err)
			continue
		}

		// Ensure that the deserialized utxo entry has the same
		// properties for the containing transaction and block height.
		if utxoEntry.TxVersion() != test.entry.TxVersion() {
			t.Errorf("deserializeUtxoEntry #%d (%s) mismatched "+
				" txVersion: got %d, want %d", i, test.name,
				utxoEntry.TxVersion(), test.entry.TxVersion())
			continue
		}
		if utxoEntry.IsCoinBase() != test.entry.IsCoinBase() {
			t.Errorf("deserializeUtxoEntry #%d (%s) mismatched "+
				"coinbase flag: got %v, want %v", i, test.name,
				utxoEntry.IsCoinBase(), test.entry.IsCoinBase())
			continue
		}
		if utxoEntry.BlockHeight() != test.entry.BlockHeight() {
			t.Errorf("deserializeUtxoEntry #%d (%s) mismatched "+
				"block height: got %d, want %d", i, test.name,
				utxoEntry.BlockHeight(),
				test.entry.BlockHeight())
			continue
		}
		if utxoEntry.IsFullySpent() != test.entry.IsFullySpent() {
			t.Errorf("deserializeUtxoEntry #%d (%s) mismatched "+
				"fully spent: got %v, want %v", i, test.name,
				utxoEntry.IsFullySpent(),
				test.entry.IsFullySpent())
			continue
		}

		// Ensure all of the outputs in the test entry match the
		// spentness of the output in the deserialized entry and the
		// deserialized entry does not contain any additional utxos.
		var numUnspent int
		for outputIndex := range test.entry.sparseOutputs {
			gotSpent := utxoEntry.IsOutputSpent(outputIndex)
			wantSpent := test.entry.IsOutputSpent(outputIndex)
			if !wantSpent {
				numUnspent++
			}
			if gotSpent != wantSpent {
				t.Errorf("deserializeUtxoEntry #%d (%s) output "+
					"#%d: mismatched spent: got %v, want "+
					"%v", i, test.name, outputIndex,
					gotSpent, wantSpent)
				continue

			}
		}
		if len(utxoEntry.sparseOutputs) != numUnspent {
			t.Errorf("deserializeUtxoEntry #%d (%s): mismatched "+
				"number of unspent outputs: got %d, want %d", i,
				test.name, len(utxoEntry.sparseOutputs),
				numUnspent)
			continue
		}

		// Ensure all of the amounts and scripts of the utxos in the
		// deserialized entry match the ones in the test entry.
		for outputIndex := range utxoEntry.sparseOutputs {
			gotAmount := utxoEntry.AmountByIndex(outputIndex)
			wantAmount := test.entry.AmountByIndex(outputIndex)
			if gotAmount != wantAmount {
				t.Errorf("deserializeUtxoEntry #%d (%s) "+
					"output #%d: mismatched amounts: got "+
					"%d, want %d", i, test.name,
					outputIndex, gotAmount, wantAmount)
				continue
			}

			gotPkScript := utxoEntry.PkScriptByIndex(outputIndex)
			wantPkScript := test.entry.PkScriptByIndex(outputIndex)
			if !bytes.Equal(gotPkScript, wantPkScript) {
				t.Errorf("deserializeUtxoEntry #%d (%s) "+
					"output #%d mismatched scripts: got "+
					"%x, want %x", i, test.name,
					outputIndex, gotPkScript, wantPkScript)
				continue
			}
		}
	}
}

// TestUtxoEntryDeserializeErrors performs negative tests against deserializing
// unspent transaction outputs to ensure error paths work as expected.
func TestUtxoEntryDeserializeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serialized []byte
		errType    error
	}{
		{
			name:       "no data after version",
			serialized: hexToBytes("01"),
			errType:    errDeserialize(""),
		},
		{
			name:       "no data after block height",
			serialized: hexToBytes("0101"),
			errType:    errDeserialize(""),
		},
		{
			name:       "no data after header code",
			serialized: hexToBytes("010102"),
			errType:    errDeserialize(""),
		},
		{
			name:       "not enough bytes for unspentness bitmap",
			serialized: hexToBytes("01017800"),
			errType:    errDeserialize(""),
		},
		{
			name:       "incomplete compressed txout",
			serialized: hexToBytes("01010232"),
			errType:    errDeserialize(""),
		},
	}

	for _, test := range tests {
		// Ensure the expected error type is returned and the returned
		// entry is nil.
		entry, err := deserializeUtxoEntry(test.serialized)
		if reflect.TypeOf(err) != reflect.TypeOf(test.errType) {
			t.Errorf("deserializeUtxoEntry (%s): expected error "+
				"type does not match - got %T, want %T",
				test.name, err, test.errType)
			continue
		}
		if entry != nil {
			t.Errorf("deserializeUtxoEntry (%s): returned entry "+
				"is not nil", test.name)
			continue
		}
	}
}

// TestBestChainStateSerialization ensures serializing and deserializing the
// best chain state works as expected.
func TestBestChainStateSerialization(t *testing.T) {
	t.Parallel()

	workSum := new(big.Int)
	tests := []struct {
		name       string
		state      bestChainState
		serialized []byte
	}{
		{
			name: "genesis",
			state: bestChainState{
				hash:         *newHashFromStr("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"),
				height:       0,
				totalTxns:    1,
				totalSubsidy: 0,
				workSum: func() *big.Int {
					workSum.Add(workSum, CalcWork(486604799))
					return new(big.Int).Set(workSum)
				}(), // 0x0100010001
			},
			serialized: hexToBytes("6fe28c0ab6f1b372c1a6a246ae63f74f931e8365e15a089c68d61900000000000000000001000000000000000000000000000000050000000100010001"),
		},
		{
			name: "block 1",
			state: bestChainState{
				hash:         *newHashFromStr("00000000839a8e6886ab5951d76f411475428afc90947ee320161bbf18eb6048"),
				height:       1,
				totalTxns:    2,
				totalSubsidy: 123456789,
				workSum: func() *big.Int {
					workSum.Add(workSum, CalcWork(486604799))
					return new(big.Int).Set(workSum)
				}(), // 0x0200020002,
			},
			serialized: hexToBytes("4860eb18bf1b1620e37e9490fc8a427514416fd75159ab86688e9a830000000001000000020000000000000015cd5b0700000000050000000200020002"),
		},
	}

	for i, test := range tests {
		// Ensure the state serializes to the expected value.
		gotBytes := serializeBestChainState(test.state)
		if !bytes.Equal(gotBytes, test.serialized) {
			t.Errorf("serializeBestChainState #%d (%s): mismatched "+
				"bytes - got %x, want %x", i, test.name,
				gotBytes, test.serialized)
			continue
		}

		// Ensure the serialized bytes are decoded back to the expected
		// state.
		state, err := deserializeBestChainState(test.serialized)
		if err != nil {
			t.Errorf("deserializeBestChainState #%d (%s) "+
				"unexpected error: %v", i, test.name, err)
			continue
		}
		if !reflect.DeepEqual(state, test.state) {
			t.Errorf("deserializeBestChainState #%d (%s) "+
				"mismatched state - got %v, want %v", i,
				test.name, state, test.state)
			continue

		}
	}
}

// TestBestChainStateDeserializeErrors performs negative tests against
// deserializing the chain state to ensure error paths work as expected.
func TestBestChainStateDeserializeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serialized []byte
		errType    error
	}{
		{
			name:       "nothing serialized",
			serialized: hexToBytes(""),
			errType:    database.Error{ErrorCode: database.ErrCorruption},
		},
		{
			name:       "short data in hash",
			serialized: hexToBytes("0000"),
			errType:    database.Error{ErrorCode: database.ErrCorruption},
		},
		{
			name:       "short data in work sum",
			serialized: hexToBytes("6fe28c0ab6f1b372c1a6a246ae63f74f931e8365e15a089c68d61900000000000000000001000000000000000500000001000100"),
			errType:    database.Error{ErrorCode: database.ErrCorruption},
		},
	}

	for _, test := range tests {
		// Ensure the expected error type and code is returned.
		_, err := deserializeBestChainState(test.serialized)
		if reflect.TypeOf(err) != reflect.TypeOf(test.errType) {
			t.Errorf("deserializeBestChainState (%s): expected "+
				"error type does not match - got %T, want %T",
				test.name, err, test.errType)
			continue
		}
		if derr, ok := err.(database.Error); ok {
			tderr := test.errType.(database.Error)
			if derr.ErrorCode != tderr.ErrorCode {
				t.Errorf("deserializeBestChainState (%s): "+
					"wrong  error code got: %v, want: %v",
					test.name, derr.ErrorCode,
					tderr.ErrorCode)
				continue
			}
		}
	}
}
