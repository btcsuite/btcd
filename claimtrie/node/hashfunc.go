package node

import (
	"crypto/sha256"
	"encoding/binary"
	"strconv"

	"github.com/lbryio/lbcd/chaincfg/chainhash"
	"github.com/lbryio/lbcd/wire"
)

func HashMerkleBranches(left *chainhash.Hash, right *chainhash.Hash) *chainhash.Hash {
	// Concatenate the left and right nodes.
	var hash [chainhash.HashSize * 2]byte
	copy(hash[:chainhash.HashSize], left[:])
	copy(hash[chainhash.HashSize:], right[:])

	newHash := chainhash.DoubleHashH(hash[:])
	return &newHash
}

func ComputeMerkleRoot(hashes []*chainhash.Hash) *chainhash.Hash {
	if len(hashes) <= 0 {
		return nil
	}
	for len(hashes) > 1 {
		if (len(hashes) & 1) > 0 { // odd count
			hashes = append(hashes, hashes[len(hashes)-1])
		}
		for i := 0; i < len(hashes); i += 2 { // TODO: parallelize this loop (or use a lib that does it)
			hashes[i>>1] = HashMerkleBranches(hashes[i], hashes[i+1])
		}
		hashes = hashes[:len(hashes)>>1]
	}
	return hashes[0]
}

func calculateNodeHash(op wire.OutPoint, takeover int32) *chainhash.Hash {

	txHash := chainhash.DoubleHashH(op.Hash[:])

	nOut := []byte(strconv.Itoa(int(op.Index)))
	nOutHash := chainhash.DoubleHashH(nOut)

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(takeover))
	heightHash := chainhash.DoubleHashH(buf)

	h := make([]byte, 0, sha256.Size*3)
	h = append(h, txHash[:]...)
	h = append(h, nOutHash[:]...)
	h = append(h, heightHash[:]...)

	hh := chainhash.DoubleHashH(h)

	return &hh
}
