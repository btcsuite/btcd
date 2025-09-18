package blockchain

import (
	"bytes"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"golang.org/x/crypto/scrypt"
)

type UtoParams struct {
	ScryptHash bool
}

var (
	UtoParamsGlobal = UtoParams{ScryptHash: true}
)

func ScryptHash(bh *wire.BlockHeader) chainhash.Hash {
	buf := new(bytes.Buffer)
	if err := bh.Serialize(buf); err != nil {
		return chainhash.Hash{}
	}

	N := 1 << 15
	r := 1
	p := 1
	keyLen := chainhash.HashSize // 32

	dk, err := scrypt.Key(buf.Bytes(), buf.Bytes(), N, r, p, keyLen)
	if err != nil {
		return chainhash.Hash{}
	}

	var hash chainhash.Hash
	copy(hash[:], dk)
	return hash
}
