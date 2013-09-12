// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil_test

import (
	"bytes"
	"github.com/conformal/btcutil"
	"testing"
)

var encodeTests = []struct {
	raw []byte
	net byte
	res string
	err error
}{
	{[]byte{0xe3, 0x4c, 0xce, 0x70, 0xc8, 0x63, 0x73, 0x27, 0x3e, 0xfc, 0xc5, 0x4c, 0xe7, 0xd2, 0xa4, 0x91, 0xbb, 0x4a, 0x0e, 0x84},
		btcutil.MainNetAddr, "1MirQ9bwyQcGVJPwKUgapu5ouK2E2Ey4gX", nil},
	{[]byte{0x0e, 0xf0, 0x30, 0x10, 0x7f, 0xd2, 0x6e, 0x0b, 0x6b, 0xf4, 0x05, 0x12, 0xbc, 0xa2, 0xce, 0xb1, 0xdd, 0x80, 0xad, 0xaa},
		btcutil.MainNetAddr, "12MzCDwodF9G1e7jfwLXfR164RNtx4BRVG", nil},
	{[]byte{0x78, 0xb3, 0x16, 0xa0, 0x86, 0x47, 0xd5, 0xb7, 0x72, 0x83, 0xe5, 0x12, 0xd3, 0x60, 0x3f, 0x1f, 0x1c, 0x8d, 0xe6, 0x8f},
		btcutil.TestNetAddr, "mrX9vMRYLfVy1BnZbc5gZjuyaqH3ZW2ZHz", nil},

	// Raw address not 20 bytes (padded with leading 0s)
	{[]byte{0x00, 0x0e, 0xf0, 0x30, 0x10, 0x7f, 0xd2, 0x6e, 0x0b, 0x6b, 0xf4, 0x05, 0x12, 0xbc, 0xa2, 0xce, 0xb1, 0xdd, 0x80, 0xad, 0xaa},
		btcutil.MainNetAddr, "12MzCDwodF9G1e7jfwLXfR164RNtx4BRVG", btcutil.ErrMalformedAddress},

	// Bad network byte
	{make([]byte, 20), btcutil.MainNetAddr + 1, "", btcutil.ErrAddrUnknownNet},
}

func TestEncodeAddresses(t *testing.T) {
	for i := range encodeTests {
		res, err := btcutil.EncodeAddress(encodeTests[i].raw,
			encodeTests[i].net)
		if err != encodeTests[i].err {
			t.Error(err)
			continue
		}
		if err == nil && res != encodeTests[i].res {
			t.Errorf("Results differ: Expected '%s', returned '%s'",
				encodeTests[i].res, res)
		}
	}
}

var decodeTests = []struct {
	addr string
	res  []byte
	net  byte
	err  error
}{
	{"1MirQ9bwyQcGVJPwKUgapu5ouK2E2Ey4gX",
		[]byte{0xe3, 0x4c, 0xce, 0x70, 0xc8, 0x63, 0x73, 0x27, 0x3e, 0xfc, 0xc5, 0x4c, 0xe7, 0xd2, 0xa4, 0x91, 0xbb, 0x4a, 0x0e, 0x84},
		btcutil.MainNetAddr, nil},
	{"mrX9vMRYLfVy1BnZbc5gZjuyaqH3ZW2ZHz",
		[]byte{0x78, 0xb3, 0x16, 0xa0, 0x86, 0x47, 0xd5, 0xb7, 0x72, 0x83, 0xe5, 0x12, 0xd3, 0x60, 0x3f, 0x1f, 0x1c, 0x8d, 0xe6, 0x8f},
		btcutil.TestNetAddr, nil},

	// Wrong length
	{"01MirQ9bwyQcGVJPwKUgapu5ouK2E2Ey4gX", nil, btcutil.MainNetAddr, btcutil.ErrMalformedAddress},

	// Bad magic
	{"2MirQ9bwyQcGVJPwKUgapu5ouK2E2Ey4gX", nil, btcutil.MainNetAddr, btcutil.ErrAddrUnknownNet},

	// Bad checksum
	{"1MirQ9bwyQcGVJPwKUgapu5ouK2E2dpuqz", nil, btcutil.MainNetAddr, btcutil.ErrMalformedAddress},
}

func TestDecodeAddresses(t *testing.T) {
	for i := range decodeTests {
		res, _, err := btcutil.DecodeAddress(decodeTests[i].addr)
		if err != decodeTests[i].err {
			t.Error(err)
		}
		if err == nil && !bytes.Equal(res, decodeTests[i].res) {
			t.Errorf("Results differ: Expected '%v', returned '%v'",
				decodeTests[i].res, res)
		}
	}
}
