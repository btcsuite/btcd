package merkletrie

import (
	"github.com/lbryio/lbcd/chaincfg/chainhash"
)

type vertex struct {
	merkleHash *chainhash.Hash
	claimsHash *chainhash.Hash
	childLinks map[byte]*vertex
}

func newVertex(hash *chainhash.Hash) *vertex {
	return &vertex{childLinks: map[byte]*vertex{}, merkleHash: hash}
}

// TODO: more professional to use msgpack here?

// nbuf decodes the on-disk format of a node, which has the following form:
//   ch(1B) hash(32B)
//   ...
//   ch(1B) hash(32B)
//   vhash(32B)
type nbuf []byte

func (nb nbuf) entries() int {
	return len(nb) / 33
}

func (nb nbuf) entry(i int) (byte, *chainhash.Hash) {
	h := chainhash.Hash{}
	copy(h[:], nb[33*i+1:])
	return nb[33*i], &h
}

func (nb nbuf) hasValue() (bool, *chainhash.Hash) {
	if len(nb)%33 == 0 {
		return false, nil
	}
	h := chainhash.Hash{}
	copy(h[:], nb[len(nb)-32:])
	return true, &h
}
