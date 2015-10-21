// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"
)

const (
	// MaxInvPerMsg is the maximum number of inventory vectors that can be in a
	// single bitcoin inv message.
	MaxInvPerMsg = 50000

	// Maximum payload size for an inventory vector.
	maxInvVectPayload = 4 + HashSize
)

// InvType represents the allowed types of inventory vectors.  See InvVect.
type InvType uint32

// These constants define the various supported inventory vector types.
const (
	InvTypeError         InvType = 0
	InvTypeTx            InvType = 1
	InvTypeBlock         InvType = 2
	InvTypeFilteredBlock InvType = 3
)

// Map of service flags back to their constant names for pretty printing.
var ivStrings = map[InvType]string{
	InvTypeError:         "ERROR",
	InvTypeTx:            "MSG_TX",
	InvTypeBlock:         "MSG_BLOCK",
	InvTypeFilteredBlock: "MSG_FILTERED_BLOCK",
}

// String returns the InvType in human-readable form.
func (invtype InvType) String() string {
	if s, ok := ivStrings[invtype]; ok {
		return s
	}

	return fmt.Sprintf("Unknown InvType (%d)", uint32(invtype))
}

// InvVect defines a bitcoin inventory vector which is used to describe data,
// as specified by the Type field, that a peer wants, has, or does not have to
// another peer.
type InvVect struct {
	Type InvType // Type of data
	Hash ShaHash // Hash of the data
}

// NewInvVect returns a new InvVect using the provided type and hash.
func NewInvVect(typ InvType, hash *ShaHash) *InvVect {
	return &InvVect{
		Type: typ,
		Hash: *hash,
	}
}

// readInvVect reads an encoded InvVect from r depending on the protocol
// version.
func readInvVect(r io.Reader, pver uint32, iv *InvVect) error {
	err := readElements(r, &iv.Type, &iv.Hash)
	if err != nil {
		return err
	}
	return nil
}

// writeInvVect serializes an InvVect to w depending on the protocol version.
func writeInvVect(w io.Writer, pver uint32, iv *InvVect) error {
	err := writeElements(w, iv.Type, &iv.Hash)
	if err != nil {
		return err
	}
	return nil
}
