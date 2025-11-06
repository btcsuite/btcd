//go:build !gen_test_vectors
// +build !gen_test_vectors

// Copyright (c) 2024 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/wire"
)

// shouldGenerateVectors returns false when test vector generation is disabled
func shouldGenerateVectors() bool {
	return false
}

// writeTestVector is a no-op when test vector generation is disabled.
func writeTestVector(vector ecOpTestVector, shouldFail bool) error {
	return nil
}

// serializePrevOut is a stub when test vector generation is disabled.
func serializePrevOut(txOut *wire.TxOut) (string, error) {
	return "", nil
}

// generateTestPrivateKey generates a random private key for tests. When test
// vector generation is disabled, this returns random keys.
func generateTestPrivateKey(testName string, keyIndex int) (*btcec.PrivateKey, error) {
	// Generate a random key as normal when not generating test vectors.
	return btcec.NewPrivateKey()
}

