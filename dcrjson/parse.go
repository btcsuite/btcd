// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

import (
	"encoding/hex"

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
