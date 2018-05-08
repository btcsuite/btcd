// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"testing"
)

// TestExtractCoinbaseNullData ensures the ExtractCoinbaseNullData function
// produces the expected extracted data under both valid and invalid scenarios.
func TestExtractCoinbaseNullData(t *testing.T) {
	tests := []struct {
		name   string
		script []byte
		valid  bool
		result []byte
	}{{
		name:   "block 2, height only",
		script: mustParseShortForm("RETURN DATA_4 0x02000000"),
		valid:  true,
		result: hexToBytes("02000000"),
	}, {
		name:   "block 2, height and extra nonce data",
		script: mustParseShortForm("RETURN DATA_36 0x02000000000000000000000000000000000000000000000000000000ffa310d9a6a9588e"),
		valid:  true,
		result: hexToBytes("02000000000000000000000000000000000000000000000000000000ffa310d9a6a9588e"),
	}, {
		name:   "block 2, height and reduced extra nonce data",
		script: mustParseShortForm("RETURN DATA_12 0x02000000ffa310d9a6a9588e"),
		valid:  true,
		result: hexToBytes("02000000ffa310d9a6a9588e"),
	}, {
		name:   "no push",
		script: mustParseShortForm("RETURN"),
		valid:  true,
		result: nil,
	}, {
		// Normal nulldata scripts support special handling of small data,
		// however the coinbase nulldata in question does not.
		name:   "small data",
		script: mustParseShortForm("RETURN OP_2"),
		valid:  false,
		result: nil,
	}, {
		name:   "almost correct",
		script: mustParseShortForm("OP_TRUE RETURN DATA_12 0x02000000ffa310d9a6a9588e"),
		valid:  false,
		result: nil,
	}, {
		name:   "almost correct 2",
		script: mustParseShortForm("DATA_12 0x02000000 0xffa310d9a6a9588e"),
		valid:  false,
		result: nil,
	}}

	for _, test := range tests {
		nullData, err := ExtractCoinbaseNullData(test.script)
		if test.valid && err != nil {
			t.Errorf("test '%s' unexpected error: %v", test.name, err)
			continue
		} else if !test.valid && err == nil {
			t.Errorf("test '%s' passed when it should have failed", test.name)
			continue
		}

		if !bytes.Equal(nullData, test.result) {
			t.Errorf("test '%s' mismatched result - got %x, want %x", test.name,
				nullData, test.result)
			continue
		}
	}
}
