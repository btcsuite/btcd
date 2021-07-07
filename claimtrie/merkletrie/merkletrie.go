package merkletrie

import (
	"bytes"
	"fmt"
	"runtime"
	"sort"
	"sync"

	"github.com/pkg/errors"

	"github.com/lbryio/lbcd/chaincfg/chainhash"
	"github.com/lbryio/lbcd/claimtrie/node"
)

var (
	// EmptyTrieHash represents the Merkle Hash of an empty PersistentTrie.
	// "0000000000000000000000000000000000000000000000000000000000000001"
	EmptyTrieHash  = &chainhash.Hash{1}
	NoChildrenHash = &chainhash.Hash{2}
	NoClaimsHash   = &chainhash.Hash{3}
)

// PersistentTrie implements a 256-way prefix tree.
type PersistentTrie struct {
	repo Repo

	root *vertex
	bufs *sync.Pool
}

// NewPersistentTrie returns a PersistentTrie.
func NewPersistentTrie(repo Repo) *PersistentTrie {

	tr := &PersistentTrie{
		repo: repo,
		bufs: &sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
		root: newVertex(EmptyTrieHash),
	}

	return tr
}

// SetRoot drops all resolved nodes in the PersistentTrie, and set the Root with specified hash.
func (t *PersistentTrie) SetRoot(h *chainhash.Hash) error {
	t.root = newVertex(h)
	runtime.GC()
	return nil
}

// Update updates the nodes along the path to the key.
// Each node is resolved or created with their Hash cleared.
func (t *PersistentTrie) Update(name []byte, hash *chainhash.Hash, restoreChildren bool) {

	n := t.root
	for i, ch := range name {
		if restoreChildren && len(n.childLinks) == 0 {
			t.resolveChildLinks(n, name[:i])
		}
		if n.childLinks[ch] == nil {
			n.childLinks[ch] = newVertex(nil)
		}
		n.merkleHash = nil
		n = n.childLinks[ch]
	}

	if restoreChildren && len(n.childLinks) == 0 {
		t.resolveChildLinks(n, name)
	}
	n.merkleHash = nil
	n.claimsHash = hash
}

// resolveChildLinks updates the links on n
func (t *PersistentTrie) resolveChildLinks(n *vertex, key []byte) {

	if n.merkleHash == nil {
		return
	}

	b := t.bufs.Get().(*bytes.Buffer)
	defer t.bufs.Put(b)
	b.Reset()
	b.Write(key)
	b.Write(n.merkleHash[:])

	result, closer, err := t.repo.Get(b.Bytes())
	if result == nil {
		return
	} else if err != nil {
		panic(err)
	}
	defer closer.Close()

	nb := nbuf(result)
	_, n.claimsHash = nb.hasValue()
	for i := 0; i < nb.entries(); i++ {
		p, h := nb.entry(i)
		n.childLinks[p] = newVertex(h)
	}
}

// MerkleHash returns the Merkle Hash of the PersistentTrie.
// All nodes must have been resolved before calling this function.
func (t *PersistentTrie) MerkleHash() *chainhash.Hash {
	buf := make([]byte, 0, 256)
	if h := t.merkle(buf, t.root); h == nil {
		return EmptyTrieHash
	}
	return t.root.merkleHash
}

// merkle recursively resolves the hashes of the node.
// All nodes must have been resolved before calling this function.
func (t *PersistentTrie) merkle(prefix []byte, v *vertex) *chainhash.Hash {
	if v.merkleHash != nil {
		return v.merkleHash
	}

	b := t.bufs.Get().(*bytes.Buffer)
	defer t.bufs.Put(b)
	b.Reset()

	keys := keysInOrder(v)

	for _, ch := range keys {
		child := v.childLinks[ch]
		if child == nil {
			continue
		}
		p := append(prefix, ch)
		h := t.merkle(p, child)
		if h != nil {
			b.WriteByte(ch) // nolint : errchk
			b.Write(h[:])   // nolint : errchk
		}
		if h == nil || len(prefix) > 4 { // TODO: determine the right number here
			delete(v.childLinks, ch) // keep the RAM down (they get recreated on Update)
		}
	}

	if v.claimsHash != nil {
		b.Write(v.claimsHash[:])
	}

	if b.Len() > 0 {
		h := chainhash.DoubleHashH(b.Bytes())
		v.merkleHash = &h
		t.repo.Set(append(prefix, h[:]...), b.Bytes())
	}

	return v.merkleHash
}

func keysInOrder(v *vertex) []byte {
	keys := make([]byte, 0, len(v.childLinks))
	for key := range v.childLinks {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func (t *PersistentTrie) MerkleHashAllClaims() *chainhash.Hash {
	buf := make([]byte, 0, 256)
	if h := t.merkleAllClaims(buf, t.root); h == nil {
		return EmptyTrieHash
	}
	return t.root.merkleHash
}

func (t *PersistentTrie) merkleAllClaims(prefix []byte, v *vertex) *chainhash.Hash {
	if v.merkleHash != nil {
		return v.merkleHash
	}
	b := t.bufs.Get().(*bytes.Buffer)
	defer t.bufs.Put(b)
	b.Reset()

	keys := keysInOrder(v)
	childHashes := make([]*chainhash.Hash, 0, len(keys))
	for _, ch := range keys {
		n := v.childLinks[ch]
		if n == nil {
			continue
		}
		p := append(prefix, ch)
		h := t.merkleAllClaims(p, n)
		if h != nil {
			childHashes = append(childHashes, h)
			b.WriteByte(ch) // nolint : errchk
			b.Write(h[:])   // nolint : errchk
		}
		if h == nil || len(prefix) > 4 { // TODO: determine the right number here
			delete(v.childLinks, ch) // keep the RAM down (they get recreated on Update)
		}
	}

	if len(childHashes) > 1 || v.claimsHash != nil { // yeah, about that 1 there -- old code used the condensed trie
		left := NoChildrenHash
		if len(childHashes) > 0 {
			left = node.ComputeMerkleRoot(childHashes)
		}
		right := NoClaimsHash
		if v.claimsHash != nil {
			b.Write(v.claimsHash[:]) // for Has Value, nolint : errchk
			right = v.claimsHash
		}

		h := node.HashMerkleBranches(left, right)
		v.merkleHash = h
		t.repo.Set(append(prefix, h[:]...), b.Bytes())
	} else if len(childHashes) == 1 {
		v.merkleHash = childHashes[0] // pass it up the tree
		t.repo.Set(append(prefix, v.merkleHash[:]...), b.Bytes())
	}

	return v.merkleHash
}

func (t *PersistentTrie) Close() error {
	return errors.WithStack(t.repo.Close())
}

func (t *PersistentTrie) Dump(s string) {
	// TODO: this function is in the wrong spot; either it goes with its caller or it needs to be a generic iterator
	// we don't want fmt used in here either way

	v := t.root

	for i := 0; i < len(s); i++ {
		t.resolveChildLinks(v, []byte(s[:i]))
		ch := s[i]
		v = v.childLinks[ch]
		if v == nil {
			fmt.Printf("Missing child at %s\n", s[:i+1])
			return
		}
	}
	t.resolveChildLinks(v, []byte(s))

	fmt.Printf("Node hash: %s, has value: %t\n", v.merkleHash.String(), v.claimsHash != nil)

	for key, value := range v.childLinks {
		fmt.Printf("  Child %s hash: %s\n", string(key), value.merkleHash.String())
	}
}

func (t *PersistentTrie) Flush() error {
	return t.repo.Flush()
}
