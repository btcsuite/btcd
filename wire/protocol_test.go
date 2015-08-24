// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire_test

import (
	"testing"

	"github.com/btcsuite/btcd/wire"
)

// TestServiceFlagStringer tests the stringized output for service flag types.
func TestServiceFlagStringer(t *testing.T) {
	tests := []struct {
		in   wire.ServiceFlag
		want string
	}{
		{0, "0x0"},
		{wire.SFNodeNetwork, "SFNodeNetwork"},
		{wire.SFNodeGetUTXO, "SFNodeGetUTXO"},
		{wire.SFNodeBloom, "SFNodeBloom"},
		{0xffffffff, "SFNodeNetwork|SFNodeGetUTXO|SFNodeBloom|0xfffffff8"},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.String()
		if result != test.want {
			t.Errorf("String #%d\n got: %s want: %s", i, result,
				test.want)
			continue
		}
	}
}

// TestBitcoinNetStringer tests the stringized output for bitcoin net types.
func TestBitcoinNetStringer(t *testing.T) {
	tests := []struct {
		in   wire.BitcoinNet
		want string
	}{
		{wire.MainNet, "MainNet"},
		{wire.TestNet, "TestNet"},
		{wire.TestNet3, "TestNet3"},
		{wire.SimNet, "SimNet"},
		{0xffffffff, "Unknown BitcoinNet (4294967295)"},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.String()
		if result != test.want {
			t.Errorf("String #%d\n got: %s want: %s", i, result,
				test.want)
			continue
		}
	}
}
