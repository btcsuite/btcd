package merkletrie

import (
	"bytes"
	"errors"
	"runtime"
	"sync"

	"github.com/lbryio/lbcd/chaincfg/chainhash"
	"github.com/lbryio/lbcd/claimtrie/node"
)

type MerkleTrie interface {
	SetRoot(h *chainhash.Hash) error
	Update(name []byte, h *chainhash.Hash, restoreChildren bool)
	MerkleHash() *chainhash.Hash
	MerkleHashAllClaims() *chainhash.Hash
	Flush() error
}

type RamTrie struct {
	collapsedTrie
	bufs *sync.Pool
}

func NewRamTrie() *RamTrie {
	return &RamTrie{
		bufs: &sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
		collapsedTrie: collapsedTrie{Root: &collapsedVertex{merkleHash: EmptyTrieHash}},
	}
}

var ErrFullRebuildRequired = errors.New("a full rebuild is required")

func (rt *RamTrie) SetRoot(h *chainhash.Hash) error {
	if rt.Root.merkleHash.IsEqual(h) {
		runtime.GC()
		return nil
	}

	// should technically clear the old trie first, but this is abused for partial rebuilds so don't
	return ErrFullRebuildRequired
}

func (rt *RamTrie) Update(name []byte, h *chainhash.Hash, _ bool) {
	if h == nil {
		rt.Erase(name)
	} else {
		_, n := rt.InsertOrFind(name)
		n.claimHash = h
	}
}

func (rt *RamTrie) MerkleHash() *chainhash.Hash {
	if h := rt.merkleHash(rt.Root); h == nil {
		return EmptyTrieHash
	}
	return rt.Root.merkleHash
}

func (rt *RamTrie) merkleHash(v *collapsedVertex) *chainhash.Hash {
	if v.merkleHash != nil {
		return v.merkleHash
	}

	b := rt.bufs.Get().(*bytes.Buffer)
	defer rt.bufs.Put(b)
	b.Reset()

	for _, ch := range v.children {
		h := rt.merkleHash(ch)              // h is a pointer; don't destroy its data
		b.WriteByte(ch.key[0])              // nolint : errchk
		b.Write(rt.completeHash(h, ch.key)) // nolint : errchk
	}

	if v.claimHash != nil {
		b.Write(v.claimHash[:])
	}

	if b.Len() > 0 {
		h := chainhash.DoubleHashH(b.Bytes())
		v.merkleHash = &h
	}

	return v.merkleHash
}

func (rt *RamTrie) completeHash(h *chainhash.Hash, childKey KeyType) []byte {
	var data [chainhash.HashSize + 1]byte
	copy(data[1:], h[:])
	for i := len(childKey) - 1; i > 0; i-- {
		data[0] = childKey[i]
		copy(data[1:], chainhash.DoubleHashB(data[:]))
	}
	return data[1:]
}

func (rt *RamTrie) MerkleHashAllClaims() *chainhash.Hash {
	if h := rt.merkleHashAllClaims(rt.Root); h == nil {
		return EmptyTrieHash
	}
	return rt.Root.merkleHash
}

func (rt *RamTrie) merkleHashAllClaims(v *collapsedVertex) *chainhash.Hash {
	if v.merkleHash != nil {
		return v.merkleHash
	}

	childHashes := make([]*chainhash.Hash, 0, len(v.children))
	for _, ch := range v.children {
		h := rt.merkleHashAllClaims(ch)
		childHashes = append(childHashes, h)
	}

	claimHash := NoClaimsHash
	if v.claimHash != nil {
		claimHash = v.claimHash
	} else if len(childHashes) == 0 {
		return nil
	}

	childHash := NoChildrenHash
	if len(childHashes) > 0 {
		// this shouldn't be referencing node; where else can we put this merkle root func?
		childHash = node.ComputeMerkleRoot(childHashes)
	}

	v.merkleHash = node.HashMerkleBranches(childHash, claimHash)
	return v.merkleHash
}

func (rt *RamTrie) Flush() error {
	return nil
}
