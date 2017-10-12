// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package base58_test

import (
	"testing"

	"github.com/decred/dcrutil/base58"
)

var checkEncodingStringTests = []struct {
	version byte
	in      string
	out     string
}{
	{20, "", "Axk2WA6L"},
	{20, " ", "kxg5DGCa1"},
	{20, "-", "kxhWqwoTY"},
	{20, "0", "kxhrrcZDw"},
	{20, "1", "kxhzgbzwe"},
	{20, "-1", "4M2qnQVfVwu"},
	{20, "11", "4M2smzp65NR"},
	{20, "abc", "FmT72s9HXyp6"},
	{20, "1234598760", "3UFLKR4oYrL1hSX1Eu2W3F"},
	{20, "abcdefghijklmnopqrstuvwxyz", "2M5VSfthNqvveeGWTcKRgY4Rm258o4ZDKBZGkAQ799jp"},
	{20, "00000000000000000000000000000000000000000000000000000000000000", "3cmTs9hNQGCVmurJUgS7UokKFYZCCJWvWfYRBCaox5hXDn3Giiy1u9AEKn7vLS8K87BcDr6Ckr4JYRnnaSMRDsB49i3eU"},
}

func TestBase58Check(t *testing.T) {
	for x, test := range checkEncodingStringTests {
		var ver [2]byte
		ver[0] = test.version
		ver[1] = 0
		// test encoding
		if res := base58.CheckEncode([]byte(test.in), ver); res != test.out {
			t.Errorf("CheckEncode test #%d failed: got %s, want: %s", x, res, test.out)
		}

		// test decoding
		res, version, err := base58.CheckDecode(test.out)
		if err != nil {
			t.Errorf("CheckDecode test #%d failed with err: %v", x, err)
		} else if version != ver {
			t.Errorf("CheckDecode test #%d failed: got version: %d want: %d", x, version, ver)
		} else if string(res) != test.in {
			t.Errorf("CheckDecode test #%d failed: got: %s want: %s", x, res, test.in)
		}
	}

	// test the two decoding failure cases
	// case 1: checksum error
	_, _, err := base58.CheckDecode("Axk2WA6M")
	if err != base58.ErrChecksum {
		t.Error("Checkdecode test failed, expected ErrChecksum")
	}
	// case 2: invalid formats (string lengths below 5 mean the version byte and/or the checksum
	// bytes are missing).
	testString := ""
	for len := 0; len < 4; len++ {
		// make a string of length `len`
		_, _, err = base58.CheckDecode(testString)
		if err != base58.ErrInvalidFormat {
			t.Error("Checkdecode test failed, expected ErrInvalidFormat")
		}
	}

}
