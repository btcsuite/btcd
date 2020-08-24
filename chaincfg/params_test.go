// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"bytes"
	"testing"
)

// TestInvalidHashStr ensures the newShaHashFromStr function panics when used to
// with an invalid hash string.
func TestInvalidHashStr(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for invalid hash, got nil")
		}
	}()
	newHashFromStr("banana")
}

// TestMustRegisterPanic ensures the mustRegister function panics when used to
// register an invalid network.
func TestMustRegisterPanic(t *testing.T) {
	t.Parallel()

	// Setup a defer to catch the expected panic to ensure it actually
	// paniced.
	defer func() {
		if err := recover(); err == nil {
			t.Error("mustRegister did not panic as expected")
		}
	}()

	// Intentionally try to register duplicate params to force a panic.
	mustRegister(&MainNetParams)
}

func TestRegisterHDKeyID(t *testing.T) {
	t.Parallel()

	// Ref: https://github.com/satoshilabs/slips/blob/master/slip-0132.md
	hdKeyIDZprv := []byte{0x02, 0xaa, 0x7a, 0x99}
	hdKeyIDZpub := []byte{0x02, 0xaa, 0x7e, 0xd3}

	if err := RegisterHDKeyID(hdKeyIDZpub, hdKeyIDZprv); err != nil {
		t.Fatalf("RegisterHDKeyID: expected no error, got %v", err)
	}

	got, err := HDPrivateKeyToPublicKeyID(hdKeyIDZprv)
	if err != nil {
		t.Fatalf("HDPrivateKeyToPublicKeyID: expected no error, got %v", err)
	}

	if !bytes.Equal(got, hdKeyIDZpub) {
		t.Fatalf("HDPrivateKeyToPublicKeyID: expected result %v, got %v",
			hdKeyIDZpub, got)
	}
}

func TestInvalidHDKeyID(t *testing.T) {
	t.Parallel()

	prvValid := []byte{0x02, 0xaa, 0x7a, 0x99}
	pubValid := []byte{0x02, 0xaa, 0x7e, 0xd3}
	prvInvalid := []byte{0x00}
	pubInvalid := []byte{0x00}

	if err := RegisterHDKeyID(pubInvalid, prvValid); err != ErrInvalidHDKeyID {
		t.Fatalf("RegisterHDKeyID: want err ErrInvalidHDKeyID, got %v", err)
	}

	if err := RegisterHDKeyID(pubValid, prvInvalid); err != ErrInvalidHDKeyID {
		t.Fatalf("RegisterHDKeyID: want err ErrInvalidHDKeyID, got %v", err)
	}

	if err := RegisterHDKeyID(pubInvalid, prvInvalid); err != ErrInvalidHDKeyID {
		t.Fatalf("RegisterHDKeyID: want err ErrInvalidHDKeyID, got %v", err)
	}

	// FIXME: The error type should be changed to ErrInvalidHDKeyID.
	if _, err := HDPrivateKeyToPublicKeyID(prvInvalid); err != ErrUnknownHDKeyID {
		t.Fatalf("HDPrivateKeyToPublicKeyID: want err ErrUnknownHDKeyID, got %v", err)
	}
}
