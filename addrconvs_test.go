// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil_test

import (
	"bytes"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"testing"
)

var encodePrivateKeyTests = []struct {
	in         []byte
	net        btcwire.BitcoinNet
	compressed bool
	out        string
}{
	{[]byte{
		0x0c, 0x28, 0xfc, 0xa3, 0x86, 0xc7, 0xa2, 0x27,
		0x60, 0x0b, 0x2f, 0xe5, 0x0b, 0x7c, 0xae, 0x11,
		0xec, 0x86, 0xd3, 0xbf, 0x1f, 0xbe, 0x47, 0x1b,
		0xe8, 0x98, 0x27, 0xe1, 0x9d, 0x72, 0xaa, 0x1d,
	}, btcwire.MainNet, false, "5HueCGU8rMjxEXxiPuD5BDku4MkFqeZyd4dZ1jvhTVqvbTLvyTJ"},
	{[]byte{
		0xdd, 0xa3, 0x5a, 0x14, 0x88, 0xfb, 0x97, 0xb6,
		0xeb, 0x3f, 0xe6, 0xe9, 0xef, 0x2a, 0x25, 0x81,
		0x4e, 0x39, 0x6f, 0xb5, 0xdc, 0x29, 0x5f, 0xe9,
		0x94, 0xb9, 0x67, 0x89, 0xb2, 0x1a, 0x03, 0x98,
	}, btcwire.TestNet3, true, "cV1Y7ARUr9Yx7BR55nTdnR7ZXNJphZtCCMBTEZBJe1hXt2kB684q"},
}

func TestEncodeDecodePrivateKey(t *testing.T) {
	for x, test := range encodePrivateKeyTests {
		wif, err := btcutil.EncodePrivateKey(test.in, test.net, test.compressed)
		if err != nil {
			t.Errorf("%x: %v", x, err)
			continue
		}
		if wif != test.out {
			t.Errorf("TestEncodeDecodePrivateKey failed: want '%s', got '%s'",
				test.out, wif)
			continue
		}

		key, _, compressed, err := btcutil.DecodePrivateKey(test.out)
		if err != nil {
			t.Error(err)
			continue
		}
		if !bytes.Equal(key, test.in) || compressed != test.compressed {
			t.Errorf("TestEncodeDecodePrivateKey failed: want '%x', got '%x'",
				test.out, key)
		}

	}
}
