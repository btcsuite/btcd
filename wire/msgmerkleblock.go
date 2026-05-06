// Copyright (c) 2014-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// maxFlagsPerMerkleBlock is the maximum number of flag bytes that could
// possibly fit into a merkle block.  Since each transaction is represented by
// a single bit, this is the max number of transactions per block divided by
// 8 bits per byte.  Then an extra one to cover partials.
const maxFlagsPerMerkleBlock = maxTxPerBlock / 8

// MsgMerkleBlock implements the Message interface and represents a bitcoin
// merkleblock message which is used to reset a Bloom filter.
//
// This message was not added until protocol version BIP0037Version.
type MsgMerkleBlock struct {
	Header       BlockHeader
	Transactions uint32
	Hashes       []*chainhash.Hash
	Flags        []byte
}

type merkleBlockExtractor struct {
	numTx     uint32
	hashes    []*chainhash.Hash
	flags     []byte
	hashIndex uint32
	flagIndex uint32
}

// calcTreeWidth calculates and returns the number of nodes (width) in a
// merkle tree at the given depth-first height.
func (m *merkleBlockExtractor) calcTreeWidth(height uint32) uint32 {
	return (m.numTx + (1 << height) - 1) >> height
}

func (m *merkleBlockExtractor) nextFlag() (bool, error) {
	if m.flagIndex/8 >= uint32(len(m.flags)) {
		return false, messageError("MsgMerkleBlock.ExtractMatches",
			"ran out of flag bits while extracting matches")
	}

	flag := m.flags[m.flagIndex/8]&(1<<(m.flagIndex%8)) != 0
	m.flagIndex++
	return flag, nil
}

func (m *merkleBlockExtractor) nextHash() (*chainhash.Hash, error) {
	if m.hashIndex >= uint32(len(m.hashes)) {
		return nil, messageError("MsgMerkleBlock.ExtractMatches",
			"ran out of hashes while extracting matches")
	}

	hash := m.hashes[m.hashIndex]
	m.hashIndex++
	return hash, nil
}

func (m *merkleBlockExtractor) extract(height, pos uint32, matches *[]chainhash.Hash) (*chainhash.Hash, error) {
	parentOfMatch, err := m.nextFlag()
	if err != nil {
		return nil, err
	}

	if height == 0 || !parentOfMatch {
		hash, err := m.nextHash()
		if err != nil {
			return nil, err
		}

		if height == 0 && parentOfMatch {
			*matches = append(*matches, *hash)
		}
		return hash, nil
	}

	left, err := m.extract(height-1, pos*2, matches)
	if err != nil {
		return nil, err
	}

	var right *chainhash.Hash
	if pos*2+1 < m.calcTreeWidth(height-1) {
		right, err = m.extract(height-1, pos*2+1, matches)
		if err != nil {
			return nil, err
		}
	} else {
		right = left
	}

	merged := chainhash.DoubleHashRaw(func(w io.Writer) error {
		if _, err := w.Write(left[:]); err != nil {
			return err
		}

		_, err := w.Write(right[:])
		return err
	})
	return &merged, nil
}

// ExtractMatches validates the partial merkle tree, reconstructs the matched
// transaction hashes, and ensures the extracted merkle root matches the block
// header.
func (msg *MsgMerkleBlock) ExtractMatches() ([]chainhash.Hash, error) {
	if msg.Transactions == 0 {
		return nil, messageError("MsgMerkleBlock.ExtractMatches",
			"merkle block contains no transactions")
	}
	if len(msg.Hashes) == 0 {
		return nil, messageError("MsgMerkleBlock.ExtractMatches",
			"merkle block contains no hashes")
	}
	if len(msg.Flags) == 0 {
		return nil, messageError("MsgMerkleBlock.ExtractMatches",
			"merkle block contains no flag bytes")
	}

	height := uint32(0)
	extractor := merkleBlockExtractor{
		numTx:  msg.Transactions,
		hashes: msg.Hashes,
		flags:  msg.Flags,
	}
	for extractor.calcTreeWidth(height) > 1 {
		height++
	}

	matches := make([]chainhash.Hash, 0)
	merkleRoot, err := extractor.extract(height, 0, &matches)
	if err != nil {
		return nil, err
	}
	if extractor.hashIndex != uint32(len(msg.Hashes)) {
		return nil, messageError("MsgMerkleBlock.ExtractMatches",
			"merkle block has unused hashes")
	}
	if (extractor.flagIndex+7)/8 != uint32(len(msg.Flags)) {
		return nil, messageError("MsgMerkleBlock.ExtractMatches",
			"merkle block has unused flag bits")
	}
	if !merkleRoot.IsEqual(&msg.Header.MerkleRoot) {
		return nil, messageError("MsgMerkleBlock.ExtractMatches",
			"merkle root mismatch")
	}

	return matches, nil
}

// AddTxHash adds a new transaction hash to the message.
func (msg *MsgMerkleBlock) AddTxHash(hash *chainhash.Hash) error {
	if len(msg.Hashes)+1 > maxTxPerBlock {
		str := fmt.Sprintf("too many tx hashes for message [max %v]",
			maxTxPerBlock)
		return messageError("MsgMerkleBlock.AddTxHash", str)
	}

	msg.Hashes = append(msg.Hashes, hash)
	return nil
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgMerkleBlock) BtcDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	if pver < BIP0037Version {
		str := fmt.Sprintf("merkleblock message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgMerkleBlock.BtcDecode", str)
	}

	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	err := readBlockHeaderBuf(r, pver, &msg.Header, buf)
	if err != nil {
		return err
	}

	if _, err := io.ReadFull(r, buf[:4]); err != nil {
		return err
	}
	msg.Transactions = littleEndian.Uint32(buf[:4])

	// Read num block locator hashes and limit to max.
	count, err := ReadVarIntBuf(r, pver, buf)
	if err != nil {
		return err
	}
	if count > maxTxPerBlock {
		str := fmt.Sprintf("too many transaction hashes for message "+
			"[count %v, max %v]", count, maxTxPerBlock)
		return messageError("MsgMerkleBlock.BtcDecode", str)
	}

	// Create a contiguous slice of hashes to deserialize into in order to
	// reduce the number of allocations.
	hashes := make([]chainhash.Hash, count)
	msg.Hashes = make([]*chainhash.Hash, 0, count)
	for i := uint64(0); i < count; i++ {
		hash := &hashes[i]
		_, err := io.ReadFull(r, hash[:])
		if err != nil {
			return err
		}
		msg.AddTxHash(hash)
	}

	msg.Flags, err = ReadVarBytesBuf(r, pver, buf, maxFlagsPerMerkleBlock,
		"merkle block flags size")
	return err
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgMerkleBlock) BtcEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	if pver < BIP0037Version {
		str := fmt.Sprintf("merkleblock message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgMerkleBlock.BtcEncode", str)
	}

	// Read num transaction hashes and limit to max.
	numHashes := len(msg.Hashes)
	if numHashes > maxTxPerBlock {
		str := fmt.Sprintf("too many transaction hashes for message "+
			"[count %v, max %v]", numHashes, maxTxPerBlock)
		return messageError("MsgMerkleBlock.BtcDecode", str)
	}
	numFlagBytes := len(msg.Flags)
	if numFlagBytes > maxFlagsPerMerkleBlock {
		str := fmt.Sprintf("too many flag bytes for message [count %v, "+
			"max %v]", numFlagBytes, maxFlagsPerMerkleBlock)
		return messageError("MsgMerkleBlock.BtcDecode", str)
	}

	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	err := writeBlockHeaderBuf(w, pver, &msg.Header, buf)
	if err != nil {
		return err
	}

	littleEndian.PutUint32(buf[:4], msg.Transactions)
	if _, err := w.Write(buf[:4]); err != nil {
		return err
	}

	err = WriteVarIntBuf(w, pver, uint64(numHashes), buf)
	if err != nil {
		return err
	}
	for _, hash := range msg.Hashes {
		_, err := w.Write(hash[:])
		if err != nil {
			return err
		}
	}

	err = WriteVarBytesBuf(w, pver, msg.Flags, buf)
	return err
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgMerkleBlock) Command() string {
	return CmdMerkleBlock
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgMerkleBlock) MaxPayloadLength(pver uint32) uint32 {
	return MaxBlockPayload
}

// NewMsgMerkleBlock returns a new bitcoin merkleblock message that conforms to
// the Message interface.  See MsgMerkleBlock for details.
func NewMsgMerkleBlock(bh *BlockHeader) *MsgMerkleBlock {
	return &MsgMerkleBlock{
		Header:       *bh,
		Transactions: 0,
		Hashes:       make([]*chainhash.Hash, 0),
		Flags:        make([]byte, 0),
	}
}
