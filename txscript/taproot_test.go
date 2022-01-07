// Copyright (c) 2013-2022 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

var (
	testPubBytes, _ = hex.DecodeString("F9308A019258C31049344F85F89D5229B531C845836F99B08601F113BCE036F9")
)

// TestControlBlockParsing tests that we're able to generate and parse a valid
// control block.
func TestControlBlockParsing(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		controlBlockGen func() []byte
		valid           bool
	}{
		// An invalid control block, it's only 5 bytes and needs to be
		// at least 33 bytes.
		{
			controlBlockGen: func() []byte {
				return bytes.Repeat([]byte{0x00}, 5)
			},
			valid: false,
		},

		// An invalid control block, it's greater than the largest
		// accepted control block.
		{
			controlBlockGen: func() []byte {
				return bytes.Repeat([]byte{0x00}, ControlBlockMaxSize+1)
			},
			valid: false,
		},

		// An invalid control block, it isn't a multiple of 32 bytes
		// enough though it has a valid starting byte length.
		{
			controlBlockGen: func() []byte {
				return bytes.Repeat([]byte{0x00}, ControlBlockBaseSize+34)
			},
			valid: false,
		},

		// A valid control block, of the largest possible size.
		{
			controlBlockGen: func() []byte {
				privKey, _ := btcec.NewPrivateKey()
				pubKey := privKey.PubKey()

				yIsOdd := (pubKey.SerializeCompressed()[0] ==
					secp.PubKeyFormatCompressedOdd)

				ctrl := ControlBlock{
					InternalKey:     pubKey,
					OutputKeyYIsOdd: yIsOdd,
					LeafVersion:     BaseLeafVersion,
					InclusionProof: bytes.Repeat(
						[]byte{0x00},
						ControlBlockMaxSize-ControlBlockBaseSize,
					),
				}

				ctrlBytes, _ := ctrl.ToBytes()
				return ctrlBytes
			},
			valid: true,
		},

		// A valid control block, only has a single element in the
		// proof as the tree only has a single element.
		{
			controlBlockGen: func() []byte {
				privKey, _ := btcec.NewPrivateKey()
				pubKey := privKey.PubKey()

				yIsOdd := (pubKey.SerializeCompressed()[0] ==
					secp.PubKeyFormatCompressedOdd)

				ctrl := ControlBlock{
					InternalKey:     pubKey,
					OutputKeyYIsOdd: yIsOdd,
					LeafVersion:     BaseLeafVersion,
					InclusionProof: bytes.Repeat(
						[]byte{0x00}, ControlBlockNodeSize,
					),
				}

				ctrlBytes, _ := ctrl.ToBytes()
				return ctrlBytes
			},
			valid: true,
		},
	}
	for i, testCase := range testCases {
		ctrlBlockBytes := testCase.controlBlockGen()

		ctrlBlock, err := ParseControlBlock(ctrlBlockBytes)
		switch {
		case testCase.valid && err != nil:
			t.Fatalf("#%v: unable to parse valid control block: %v", i, err)

		case !testCase.valid && err == nil:
			t.Fatalf("#%v: invalid control block should have failed: %v", i, err)
		}

		if !testCase.valid {
			continue
		}

		// If we serialize the control block, we should get the exact same
		// set of bytes as the input.
		ctrlBytes, err := ctrlBlock.ToBytes()
		if err != nil {
			t.Fatalf("#%v: unable to encode bytes: %v", i, err)
		}
		if !bytes.Equal(ctrlBytes, ctrlBlockBytes) {
			t.Fatalf("#%v: encoding mismatch: expected %x, "+
				"got %x", i, ctrlBlockBytes, ctrlBytes)
		}
	}
}
