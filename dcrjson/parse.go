// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg/chainhash"
)

// EncodeConcatenatedHashes serializes a slice of chainhash.Hash values into a
// string of hex-encoded bytes.
func EncodeConcatenatedHashes(hashSlice []chainhash.Hash) string {
	length := len(hashSlice) * chainhash.HashSize
	hashBytes := make([]byte, 0, length)
	for i := range hashSlice {
		hashBytes = append(hashBytes, hashSlice[i][:]...)
	}

	return hex.EncodeToString(hashBytes)
}

// DecodeConcatenatedHashes return a slice of contiguous chainhash.Hash objects
// created by decoding a single string of concatenated hex-encoded hashes.
//
// These hashes must NOT be the byte reversed string encoding that is typically
// used for block and transaction hashes, or each resulting hash will also be
// reversed.
//
// The length of the string must be evenly divisible by twice the hash size in
// order for the parameter to be valid.  This function assumes the input is from
// a JSON-RPC request and any errors will be of type *RPCError with an
// ErrRPCInvalidParameter or ErrRPCDecodedHexString error code.
func DecodeConcatenatedHashes(hashes string) ([]chainhash.Hash, error) {
	numHashes := len(hashes) / (2 * chainhash.HashSize)
	if numHashes*2*chainhash.HashSize != len(hashes) {
		return nil, &RPCError{
			Code:    ErrRPCInvalidParameter,
			Message: "Hashes is not evenly divisible by the hash size",
		}
	}
	decoded := make([]chainhash.Hash, numHashes)
	hashSrcCpy := make([]byte, 2*chainhash.HashSize)
	for i, b := 0, 0; b < len(hashes); i, b = i+1, b+2*chainhash.HashSize {
		copy(hashSrcCpy, hashes[b:])
		_, err := hex.Decode(decoded[i][:], hashSrcCpy)
		if err != nil {
			return nil, &RPCError{
				Code: ErrRPCDecodeHexString,
				Message: "Parameter contains invalid hexadecimal " +
					"encoding: " + string(hashSrcCpy),
			}
		}
	}
	return decoded, nil
}

// EncodeConcatenatedVoteBits encodes a slice of VoteBits into a serialized byte
// slice.  The entirety of the voteBits are encoded individually in series as
// follows:
//
//   Size            Description
//   1 byte          Length of the concatenated voteBits in bytes
//   2 bytes         Vote bits
//   up to 73 bytes  Extended vote bits
//
// The result may be concatenated into a slice and then passed to callers
func EncodeConcatenatedVoteBits(voteBitsSlice []stake.VoteBits) (string, error) {
	length := 0
	for i := range voteBitsSlice {
		if len(voteBitsSlice[i].ExtendedBits) > stake.SSGenVoteBitsExtendedMaxSize {
			return "", fmt.Errorf("extended votebits too long (got %v, want "+
				"%v max", len(voteBitsSlice[i].ExtendedBits),
				stake.SSGenVoteBitsExtendedMaxSize)
		}

		length += 1 + 2 + len(voteBitsSlice[i].ExtendedBits)
	}

	vbBytes := make([]byte, length)
	offset := 0
	for i := range voteBitsSlice {
		vbBytes[offset] = 2 + uint8(len(voteBitsSlice[i].ExtendedBits))
		offset++

		binary.LittleEndian.PutUint16(vbBytes[offset:offset+2],
			voteBitsSlice[i].Bits)
		offset += 2

		copy(vbBytes[offset:], voteBitsSlice[i].ExtendedBits[:])
		offset += len(voteBitsSlice[i].ExtendedBits)
	}

	return hex.EncodeToString(vbBytes), nil
}

// DecodeConcatenatedVoteBits decodes a string encoded as a slice of concatenated
// voteBits and extended voteBits, and returns the slice of DecodedVoteBits to
// the caller.
func DecodeConcatenatedVoteBits(voteBitsString string) ([]stake.VoteBits, error) {
	asBytes, err := hex.DecodeString(voteBitsString)
	if err != nil {
		return nil, err
	}

	var dvbs []stake.VoteBits
	cursor := 0
	for {
		var dvb stake.VoteBits
		length := int(asBytes[cursor])
		if length < 2 {
			return nil, &RPCError{
				Code:    ErrRPCInvalidParameter,
				Message: "invalid length byte for votebits (short)",
			}
		}

		if cursor+length >= len(asBytes) {
			return nil, &RPCError{
				Code: ErrRPCInvalidParameter,
				Message: "cursor read past memory when decoding " +
					"votebits",
			}
		}
		cursor++

		dvb.Bits = binary.LittleEndian.Uint16(asBytes[cursor : cursor+2])
		cursor += 2

		dvb.ExtendedBits = asBytes[cursor : cursor+length-2]
		cursor += length - 2

		dvbs = append(dvbs, dvb)
		if cursor == len(asBytes) {
			break
		}
	}

	return dvbs, nil
}
