// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire_test

import (
	"testing"

	"github.com/decred/dcrd/wire"
)

// TestServiceFlagStringer tests the stringized output for service flag types.
func TestServiceFlagStringer(t *testing.T) {
	tests := []struct {
		in   wire.ServiceFlag
		want string
	}{
		{0, "0x0"},
		{wire.SFNodeNetwork, "SFNodeNetwork"},
		{wire.SFNodeBloom, "SFNodeBloom"},
		{0xffffffff, "SFNodeNetwork|SFNodeBloom|0xfffffffc"},
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

// TestCurrencyNetStringer tests the stringized output for decred net types.
func TestCurrencyNetStringer(t *testing.T) {
	tests := []struct {
		in   wire.CurrencyNet
		want string
	}{
		{wire.MainNet, "MainNet"},
		{wire.TestNet, "TestNet"},
		{wire.TestNet, "TestNet"},
		{wire.SimNet, "SimNet"},
		{0xffffffff, "Unknown CurrencyNet (4294967295)"},
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
