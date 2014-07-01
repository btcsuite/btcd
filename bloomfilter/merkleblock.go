package bloomfilter

import (
	"github.com/conformal/btcchain"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

type merkleBlock struct {
	msgMerkleBlock *btcwire.MsgMerkleBlock
	allHashes      []*btcwire.ShaHash
	finalHashes    []*btcwire.ShaHash
	matchedBits    []byte
	bits           []byte
}

// NewMerkleBlock returns a new *btcwire.MsgMerkleBlock based on the passed
// block and filter.
func NewMerkleBlock(block *btcutil.Block, filter *BloomFilter) (*btcwire.MsgMerkleBlock, []*btcwire.ShaHash) {
	blockHeader := block.MsgBlock().Header
	mBlock := merkleBlock{
		msgMerkleBlock: btcwire.NewMsgMerkleBlock(&blockHeader),
	}

	var matchedHashes []*btcwire.ShaHash
	for _, tx := range block.Transactions() {
		if filter.MatchesTx(tx) {
			mBlock.matchedBits = append(mBlock.matchedBits, 0x01)
			matchedHashes = append(matchedHashes, tx.Sha())
		} else {
			mBlock.matchedBits = append(mBlock.matchedBits, 0x00)
		}
		mBlock.allHashes = append(mBlock.allHashes, tx.Sha())
	}
	mBlock.msgMerkleBlock.Transactions = uint32(len(block.Transactions()))

	height := uint32(0)
	for mBlock.calcTreeWidth(height) > 1 {
		height++
	}

	mBlock.traverseAndBuild(height, 0)

	for _, sha := range mBlock.finalHashes {
		mBlock.msgMerkleBlock.AddTxHash(sha)
	}
	mBlock.msgMerkleBlock.Flags = make([]byte, (len(mBlock.bits)+7)/8)
	for i := uint32(0); i < uint32(len(mBlock.bits)); i++ {
		mBlock.msgMerkleBlock.Flags[i/8] |= mBlock.bits[i] << (i % 8)
	}
	return mBlock.msgMerkleBlock, matchedHashes
}

func (m *merkleBlock) calcTreeWidth(height uint32) uint32 {
	return (m.msgMerkleBlock.Transactions + (1 << height) - 1) >> height
}

func (m *merkleBlock) calcHash(height, pos uint32) *btcwire.ShaHash {
	if height == 0 {
		return m.allHashes[pos]
	} else {
		var right *btcwire.ShaHash
		left := m.calcHash(height-1, pos*2)
		if pos*2+1 < m.calcTreeWidth(height-1) {
			right = m.calcHash(height-1, pos*2+1)
		} else {
			right = left
		}
		return btcchain.HashMerkleBranches(left, right)
	}
}

func (m *merkleBlock) traverseAndBuild(height, pos uint32) {
	var isParent byte

	for i := pos << height; i < (pos+1)<<height && i < m.msgMerkleBlock.Transactions; i++ {
		isParent |= m.matchedBits[i]
	}

	m.bits = append(m.bits, isParent)
	if height == 0 || isParent == 0x00 {
		m.finalHashes = append(m.finalHashes, m.calcHash(height, pos))
	} else {
		m.traverseAndBuild(height-1, pos*2)
		if pos*2+1 < m.calcTreeWidth(height-1) {
			m.traverseAndBuild(height-1, pos*2+1)
		}
	}
}
