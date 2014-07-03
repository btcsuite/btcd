// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire_test

import (
	"testing"

	"github.com/conformal/btcwire"
)

// TestServiceFlagStringer tests the stringized output for service flag types.
func TestServiceFlagStringer(t *testing.T) {
	tests := []struct {
		in   btcwire.ServiceFlag
		want string
	}{
		{0, "0x0"},
		{btcwire.SFNodeNetwork, "SFNodeNetwork"},
		{0xffffffff, "SFNodeNetwork|0xfffffffe"},
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
		in   btcwire.BitcoinNet
		want string
	}{
		{btcwire.MainNet, "MainNet"},
		{btcwire.TestNet, "TestNet"},
		{btcwire.TestNet3, "TestNet3"},
		{btcwire.SimNet, "SimNet"},
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
