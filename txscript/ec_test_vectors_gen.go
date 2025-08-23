//go:build gen_test_vectors
// +build gen_test_vectors

// Copyright (c) 2024 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/wire"
)

// shouldGenerateVectors returns true when test vector generation is enabled.
func shouldGenerateVectors() bool {
	return true
}

// generateTestVectorFilename creates a filename based on content hash.
func generateTestVectorFilename(jsonContent []byte, isValid bool) string {
	hash := sha256.Sum256(jsonContent)
	suffix := "valid"
	if !isValid {
		suffix = "invalid"
	}
	return fmt.Sprintf("%x_%s.json", hash[:8], suffix)
}

// writeTestVector writes a test vector to the appropriate directory.
func writeTestVector(vector ecOpTestVector, shouldFail bool) error {
	jsonContent, err := json.MarshalIndent(vector, "", "  ")
	if err != nil {
		return err
	}

	// Add trailing comma and newline to match existing format.
	jsonContent = append(jsonContent, ',', '\n')

	filename := generateTestVectorFilename(jsonContent, !shouldFail)
	dirPath := filepath.Join("data", "ec-op-ref")

	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(dirPath, filename)

	fmt.Printf("Writing test vector: %s\n", filename)

	return os.WriteFile(filePath, jsonContent, 0644)
}

// serializePrevOut serializes a transaction output for test vectors.
func serializePrevOut(txOut *wire.TxOut) (string, error) {
	var buf bytes.Buffer
	if err := wire.WriteTxOut(&buf, 0, 0, txOut); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf.Bytes()), nil
}

// testKeyGenerator provides deterministic key generation for test vectors.
type testKeyGenerator struct {
	rng *rand.Rand
}

// newTestKeyGenerator creates a new deterministic key generator with a fixed seed.
func newTestKeyGenerator(seed int64) *testKeyGenerator {
	return &testKeyGenerator{
		rng: rand.New(rand.NewSource(seed)),
	}
}

// generateTestPrivateKey generates a deterministic private key for test vectors.
func (g *testKeyGenerator) generateTestPrivateKey() (*btcec.PrivateKey, error) {
	// Generate a 32-byte scalar deterministically
	scalar := make([]byte, 32)
	g.rng.Read(scalar)

	// Ensure the scalar is valid (non-zero and less than the curve order).
	// Set the highest bit to 0 to ensure it's less than the curve order.
	scalar[0] &= 0x7f

	// Ensure it's not zero (set lowest bit if all zero).
	isZero := true
	for _, b := range scalar {
		if b != 0 {
			isZero = false
			break
		}
	}
	if isZero {
		scalar[31] = 1
	}

	privKey, _ := btcec.PrivKeyFromBytes(scalar)
	return privKey, nil
}

// generateTestPrivateKey generates a deterministic private key using a seed
// value. When test vector generation is enabled, this returns deterministic
// keys.
func generateTestPrivateKey(testName string,
	keyIndex int) (*btcec.PrivateKey, error) {

	// Use a deterministic seed based on test name and key index.
	h := sha256.New()
	h.Write([]byte(testName))
	h.Write([]byte{byte(keyIndex)})
	seedBytes := h.Sum(nil)

	// Convert first 8 bytes to int64 for seed.
	seed := int64(0)
	for i := 0; i < 8; i++ {
		seed = (seed << 8) | int64(seedBytes[i])
	}

	gen := newTestKeyGenerator(seed)
	return gen.generateTestPrivateKey()
}
