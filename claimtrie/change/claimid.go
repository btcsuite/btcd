package change

import (
	"encoding/binary"
	"encoding/hex"

	"github.com/lbryio/lbcd/chaincfg/chainhash"
	"github.com/lbryio/lbcd/wire"
	btcutil "github.com/lbryio/lbcutil"
)

// ClaimID represents a Claim's ClaimID.
const ClaimIDSize = 20

type ClaimID [ClaimIDSize]byte

// NewClaimID returns a Claim ID calculated from Ripemd160(Sha256(OUTPOINT).
func NewClaimID(op wire.OutPoint) (id ClaimID) {

	var buffer [chainhash.HashSize + 4]byte // hoping for stack alloc
	copy(buffer[:], op.Hash[:])
	binary.BigEndian.PutUint32(buffer[chainhash.HashSize:], op.Index)
	copy(id[:], btcutil.Hash160(buffer[:]))
	return id
}

// NewIDFromString returns a Claim ID from a string.
func NewIDFromString(s string) (id ClaimID, err error) {

	if len(s) == 40 {
		_, err = hex.Decode(id[:], []byte(s))
	} else {
		copy(id[:], s)
	}
	for i, j := 0, len(id)-1; i < j; i, j = i+1, j-1 {
		id[i], id[j] = id[j], id[i]
	}
	return id, err
}

// Key is for in-memory maps
func (id ClaimID) Key() string {
	return string(id[:])
}

// String is for anything written to a DB
func (id ClaimID) String() string {

	for i, j := 0, len(id)-1; i < j; i, j = i+1, j-1 {
		id[i], id[j] = id[j], id[i]
	}

	return hex.EncodeToString(id[:])
}
