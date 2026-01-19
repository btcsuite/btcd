// Copyright (c) 2024 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"testing"
)

// TestSigNetSwiftSyncData verifies that the signet swift sync data is
// properly initialized from the embedded bitmap file.
func TestSigNetSwiftSyncData(t *testing.T) {
	if sigNetSwiftSync == nil {
		t.Fatal("sigNetSwiftSync is nil - embedded bitmap file may be missing")
	}

	// Verify height is correct.
	if sigNetSwiftSync.Height != signetSwiftSyncHeight {
		t.Errorf("Height = %d, want %d", sigNetSwiftSync.Height, signetSwiftSyncHeight)
	}

	// Verify hash is set.
	if sigNetSwiftSync.Hash == nil {
		t.Fatal("Hash is nil")
	}

	// Verify hash matches expected value.
	expectedHashStr := "00000004b1cc694c48295fc56c2d78b88abdd648ee60a21548feba259e6dedf1"
	if sigNetSwiftSync.Hash.String() != expectedHashStr {
		t.Errorf("Hash = %s, want %s", sigNetSwiftSync.Hash.String(), expectedHashStr)
	}

	// Verify bitmap is not empty.
	if len(sigNetSwiftSync.Bitmap) == 0 {
		t.Fatal("Bitmap is empty")
	}

	// The bitmap should be the file size minus the 8-byte header.
	// File is ~15MB, so bitmap should be at least 15 million bytes.
	if len(sigNetSwiftSync.Bitmap) < 15_000_000 {
		t.Errorf("Bitmap size = %d, expected at least 15,000,000 bytes",
			len(sigNetSwiftSync.Bitmap))
	}

	t.Logf("Swift sync data valid: height=%d, hash=%s, bitmap_size=%d",
		sigNetSwiftSync.Height, sigNetSwiftSync.Hash, len(sigNetSwiftSync.Bitmap))
}

// TestSigNetParamsHasSwiftSync verifies that SigNetParams includes swift sync data.
func TestSigNetParamsHasSwiftSync(t *testing.T) {
	if SigNetParams.SwiftSync == nil {
		t.Fatal("SigNetParams.SwiftSync is nil")
	}

	if SigNetParams.SwiftSync != sigNetSwiftSync {
		t.Error("SigNetParams.SwiftSync does not point to sigNetSwiftSync")
	}
}
