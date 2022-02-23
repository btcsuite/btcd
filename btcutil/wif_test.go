// Copyright (c) 2013 - 2020 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	. "github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

func TestEncodeDecodeWIF(t *testing.T) {
	validEncodeCases := []struct {
		privateKey []byte           // input
		net        *chaincfg.Params // input
		compress   bool             // input
		wif        string           // output
		publicKey  []byte           // output
		name       string           // name of subtest
	}{
		{
			privateKey: []byte{
				0x0c, 0x28, 0xfc, 0xa3, 0x86, 0xc7, 0xa2, 0x27,
				0x60, 0x0b, 0x2f, 0xe5, 0x0b, 0x7c, 0xae, 0x11,
				0xec, 0x86, 0xd3, 0xbf, 0x1f, 0xbe, 0x47, 0x1b,
				0xe8, 0x98, 0x27, 0xe1, 0x9d, 0x72, 0xaa, 0x1d},
			net:      &chaincfg.MainNetParams,
			compress: false,
			wif:      "5HueCGU8rMjxEXxiPuD5BDku4MkFqeZyd4dZ1jvhTVqvbTLvyTJ",
			publicKey: []byte{
				0x04, 0xd0, 0xde, 0x0a, 0xae, 0xae, 0xfa, 0xd0,
				0x2b, 0x8b, 0xdc, 0x8a, 0x01, 0xa1, 0xb8, 0xb1,
				0x1c, 0x69, 0x6b, 0xd3, 0xd6, 0x6a, 0x2c, 0x5f,
				0x10, 0x78, 0x0d, 0x95, 0xb7, 0xdf, 0x42, 0x64,
				0x5c, 0xd8, 0x52, 0x28, 0xa6, 0xfb, 0x29, 0x94,
				0x0e, 0x85, 0x8e, 0x7e, 0x55, 0x84, 0x2a, 0xe2,
				0xbd, 0x11, 0x5d, 0x1e, 0xd7, 0xcc, 0x0e, 0x82,
				0xd9, 0x34, 0xe9, 0x29, 0xc9, 0x76, 0x48, 0xcb,
				0x0a},
			name: "encodeValidUncompressedMainNetWif",
		},
		{
			privateKey: []byte{
				0xdd, 0xa3, 0x5a, 0x14, 0x88, 0xfb, 0x97, 0xb6,
				0xeb, 0x3f, 0xe6, 0xe9, 0xef, 0x2a, 0x25, 0x81,
				0x4e, 0x39, 0x6f, 0xb5, 0xdc, 0x29, 0x5f, 0xe9,
				0x94, 0xb9, 0x67, 0x89, 0xb2, 0x1a, 0x03, 0x98},
			net:      &chaincfg.TestNet3Params,
			compress: true,
			wif:      "cV1Y7ARUr9Yx7BR55nTdnR7ZXNJphZtCCMBTEZBJe1hXt2kB684q",
			publicKey: []byte{
				0x02, 0xee, 0xc2, 0x54, 0x06, 0x61, 0xb0, 0xc3,
				0x9d, 0x27, 0x15, 0x70, 0x74, 0x24, 0x13, 0xbd,
				0x02, 0x93, 0x2d, 0xd0, 0x09, 0x34, 0x93, 0xfd,
				0x0b, 0xec, 0xed, 0x0b, 0x7f, 0x93, 0xad, 0xde,
				0xc4},
			name: "encodeValidCompressedTestNet3Wif",
		},
	}

	for _, validCase := range validEncodeCases {
		validCase := validCase

		t.Run(validCase.name, func(t *testing.T) {
			priv, _ := btcec.PrivKeyFromBytes(validCase.privateKey)
			wif, err := NewWIF(priv, validCase.net, validCase.compress)
			if err != nil {
				t.Fatalf("NewWIF failed: expected no error, got '%v'", err)
			}

			if !wif.IsForNet(validCase.net) {
				t.Fatal("IsForNet failed: got 'false', want 'true'")
			}

			if gotPubKey := wif.SerializePubKey(); !bytes.Equal(gotPubKey, validCase.publicKey) {
				t.Fatalf("SerializePubKey failed: got '%s', want '%s'",
					hex.EncodeToString(gotPubKey), hex.EncodeToString(validCase.publicKey))
			}

			// Test that encoding the WIF structure matches the expected string.
			got := wif.String()
			if got != validCase.wif {
				t.Fatalf("NewWIF failed: want '%s', got '%s'",
					validCase.wif, got)
			}

			// Test that decoding the expected string results in the original WIF
			// structure.
			decodedWif, err := DecodeWIF(got)
			if err != nil {
				t.Fatalf("DecodeWIF failed: expected no error, got '%v'", err)
			}
			if decodedWifString := decodedWif.String(); decodedWifString != validCase.wif {
				t.Fatalf("NewWIF failed: want '%v', got '%v'", validCase.wif, decodedWifString)
			}
		})
	}

	invalidDecodeCases := []struct {
		name string
		wif  string
		err  error
	}{
		{
			name: "decodeInvalidLengthWif",
			wif:  "deadbeef",
			err:  ErrMalformedPrivateKey,
		},
		{
			name: "decodeInvalidCompressMagicWif",
			wif:  "KwDiBf89QgGbjEhKnhXJuH7LrciVrZi3qYjgd9M7rFU73sfZr2ym",
			err:  ErrMalformedPrivateKey,
		},
		{
			name: "decodeInvalidChecksumWif",
			wif:  "5HueCGU8rMjxEXxiPuD5BDku4MkFqeZyd4dZ1jvhTVqvbTLvyTj",
			err:  ErrChecksumMismatch,
		},
	}

	for _, invalidCase := range invalidDecodeCases {
		invalidCase := invalidCase

		t.Run(invalidCase.name, func(t *testing.T) {
			decodedWif, err := DecodeWIF(invalidCase.wif)
			if decodedWif != nil {
				t.Fatalf("DecodeWIF: unexpectedly succeeded - got '%v', want '%v'",
					decodedWif, nil)
			}
			if err != invalidCase.err {
				t.Fatalf("DecodeWIF: expected error '%v', got '%v'",
					invalidCase.err, err)
			}
		})
	}

	t.Run("encodeInvalidNetworkWif", func(t *testing.T) {
		privateKey := []byte{
			0x0c, 0x28, 0xfc, 0xa3, 0x86, 0xc7, 0xa2, 0x27,
			0x60, 0x0b, 0x2f, 0xe5, 0x0b, 0x7c, 0xae, 0x11,
			0xec, 0x86, 0xd3, 0xbf, 0x1f, 0xbe, 0x47, 0x1b,
			0xe8, 0x98, 0x27, 0xe1, 0x9d, 0x72, 0xaa, 0x1d}
		priv, _ := btcec.PrivKeyFromBytes(privateKey)

		wif, err := NewWIF(priv, nil, true)

		if wif != nil {
			t.Fatalf("NewWIF: unexpectedly succeeded - got '%v', want '%v'",
				wif, nil)
		}
		if err == nil || err.Error() != "no network" {
			t.Fatalf("NewWIF: expected error 'no network', got '%v'", err)
		}
	})
}
