// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chainec

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestGeneralEd25519(t *testing.T) {
	// Sample pubkey
	samplePubkey, _ := hex.DecodeString("b0d88c8d1d327d1bc6f00f6d7682c98" +
		"562869a798b96367bf8d67712c9cb1d17")
	_, err := Edwards.ParsePubKey(samplePubkey)
	if err != nil {
		t.Errorf("failure parsing pubkey: %v", err)
	}

	// Sample privkey secret
	samplePrivKey, _ := hex.DecodeString("a980f892db13c99a3e8971e965b2ff3d4" +
		"1eafd54093bc9f34d1fd22d84115bb644b57ee30cdb55829d0a5d4f046baef078f1e97" +
		"a7f21b62d75f8e96ea139c35f")
	privTest, _ := Edwards.PrivKeyFromBytes(samplePrivKey)
	if privTest == nil {
		t.Errorf("failure parsing privkey from secret")
	}

	// Sample privkey scalar
	samplePrivKeyScalar, _ := hex.DecodeString("04c723f67789d320bfcccc0ff2bc84" +
		"95a09c2356fa63ac6457107c295e6fde68")
	privTest, _ = Edwards.PrivKeyFromScalar(samplePrivKeyScalar)
	if privTest == nil {
		t.Errorf("failure parsing privkey from secret")
	}

	// Sample signature
	sampleSig, _ := hex.DecodeString(
		"71301d3212915df23211bbd0bae5e678a51c7212ecc9341a91c48fbe96772e08" +
			"cdd3d3b1f8ec828b3546b61a27b53a5472597ffd1771c39219741070ca62a40c")
	_, err = Edwards.ParseDERSignature(sampleSig)
	if err != nil {
		t.Errorf("failure parsing DER signature: %v", err)
	}
}

func TestPrivKeysEdwards(t *testing.T) {
	tests := []struct {
		name string
		key  []byte
	}{
		{
			name: "check curve",
			key: []byte{
				0x0e, 0x10, 0xcb, 0xb0, 0x70, 0x27, 0xb9, 0x76,
				0x36, 0xf8, 0x36, 0x48, 0xb2, 0xb5, 0x1a, 0x98,
				0x7d, 0xad, 0x78, 0x2e, 0xbd, 0xaf, 0xcf, 0xbc,
				0x4f, 0xe8, 0xd7, 0x49, 0x84, 0x2b, 0x24, 0xd8,
			},
		},
	}

	for _, test := range tests {
		priv, pub := Edwards.PrivKeyFromScalar(test.key)
		if priv == nil || pub == nil {
			t.Errorf("failure deserializing from bytes")
			continue
		}

		hash := []byte{0x0, 0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9}
		r, s, err := Edwards.Sign(priv, hash)
		if err != nil {
			t.Errorf("%s could not sign: %v", test.name, err)
			continue
		}
		sig := Edwards.NewSignature(r, s)

		if !Edwards.Verify(pub, hash, sig.GetR(), sig.GetS()) {
			t.Errorf("%s could not verify: %v", test.name, err)
			continue
		}

		serializedKey := priv.Serialize()
		if !bytes.Equal(serializedKey, test.key) {
			t.Errorf("%s unexpected serialized bytes - got: %x, "+
				"want: %x", test.name, serializedKey, test.key)
		}
	}
}
