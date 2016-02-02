// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2013-2014 Dave Collins
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcec_test

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/btcec"
)

// TestSetInt ensures that setting a field value to various native integers
// works as expected.
func TestNfieldSetInt(t *testing.T) {
	tests := []struct {
		in  uint
		raw [10]uint32
	}{
		{5, [10]uint32{5, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		// 2^26
		{67108864, [10]uint32{67108864, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		// 2^26 + 1
		{67108865, [10]uint32{67108865, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		// 2^32 - 1
		{4294967295, [10]uint32{4294967295, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		f := btcec.NewNfieldVal().SetInt(test.in)
		result := f.TstNfieldRawInts()
		if !reflect.DeepEqual(result, test.raw) {
			t.Errorf("nfieldVal.Set #%d wrong result\ngot: %v\n"+
				"want: %v", i, result, test.raw)
			continue
		}
	}
}

// TestZero ensures that zeroing a field value zero works as expected.
func TestNfieldZero(t *testing.T) {
	f := btcec.NewNfieldVal().SetInt(2)
	f.Zero()
	for idx, rawInt := range f.TstNfieldRawInts() {
		if rawInt != 0 {
			t.Errorf("internal field integer at index #%d is not "+
				"zero - got %d", idx, rawInt)
		}
	}
}

// TestIsZero ensures that checking if a field IsZero works as expected.
func TestNfieldIsZero(t *testing.T) {
	f := btcec.NewNfieldVal()
	if !f.IsZero() {
		t.Errorf("new field value is not zero - got %v (rawints %x)", f,
			f.TstNfieldRawInts())
	}

	f.SetInt(1)
	if f.IsZero() {
		t.Errorf("field claims it's zero when it's not - got %v "+
			"(raw rawints %x)", f, f.TstNfieldRawInts())
	}

	f.Zero()
	if !f.IsZero() {
		t.Errorf("field claims it's not zero when it is - got %v "+
			"(raw rawints %x)", f, f.TstNfieldRawInts())
	}
}

// TestStringer ensures the stringer returns the appropriate hex string.
func TestNfieldStringer(t *testing.T) {
	tests := []struct {
		in       string
		expected string
	}{
		{"0", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"1", "0000000000000000000000000000000000000000000000000000000000000001"},
		{"a", "000000000000000000000000000000000000000000000000000000000000000a"},
		{"b", "000000000000000000000000000000000000000000000000000000000000000b"},
		{"c", "000000000000000000000000000000000000000000000000000000000000000c"},
		{"d", "000000000000000000000000000000000000000000000000000000000000000d"},
		{"e", "000000000000000000000000000000000000000000000000000000000000000e"},
		{"f", "000000000000000000000000000000000000000000000000000000000000000f"},
		{"f0", "00000000000000000000000000000000000000000000000000000000000000f0"},
		// 2^26-1
		{
			"3ffffff",
			"0000000000000000000000000000000000000000000000000000000003ffffff",
		},
		// 2^32-1
		{
			"ffffffff",
			"00000000000000000000000000000000000000000000000000000000ffffffff",
		},
		// 2^64-1
		{
			"ffffffffffffffff",
			"000000000000000000000000000000000000000000000000ffffffffffffffff",
		},
		// 2^96-1
		{
			"ffffffffffffffffffffffff",
			"0000000000000000000000000000000000000000ffffffffffffffffffffffff",
		},
		// 2^128-1
		{
			"ffffffffffffffffffffffffffffffff",
			"00000000000000000000000000000000ffffffffffffffffffffffffffffffff",
		},
		// 2^160-1
		{
			"ffffffffffffffffffffffffffffffffffffffff",
			"000000000000000000000000ffffffffffffffffffffffffffffffffffffffff",
		},
		// 2^192-1
		{
			"ffffffffffffffffffffffffffffffffffffffffffffffff",
			"0000000000000000ffffffffffffffffffffffffffffffffffffffffffffffff",
		},
		// 2^224-1
		{
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"00000000ffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
		// 2^256-432420386565659656852420866394968145599
		// (the btcec n, so should result in 0)
		{
			"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141",
			"0000000000000000000000000000000000000000000000000000000000000000",
		},
		// 2^256-432420386565659656852420866394968145599
		// (the secp256k1 n+1, so should result in 1)
		{
			"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364142",
			"0000000000000000000000000000000000000000000000000000000000000001",
		},

		// Invalid hex
		{"g", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"1h", "0000000000000000000000000000000000000000000000000000000000000000"},
		{"i1", "0000000000000000000000000000000000000000000000000000000000000000"},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		f := btcec.NewNfieldVal().SetHex(test.in)
		result := f.String()
		if result != test.expected {
			t.Errorf("nfieldVal.String #%d wrong result\ngot: %v\n"+
				"want: %v", i, result, test.expected)
			continue
		}
	}
}

// TestNormalize ensures that normalizing the internal field words works as
// expected.
func TestNfieldNormalize(t *testing.T) {
	tests := []struct {
		raw        [10]uint32 // Intentionally denormalized value
		normalized [10]uint32 // Normalized form of the raw value
	}{
		{
			[10]uint32{0x00000005, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			[10]uint32{0x00000005, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		// 2^26
		{
			[10]uint32{0x04000000, 0x0, 0, 0, 0, 0, 0, 0, 0, 0},
			[10]uint32{0x00000000, 0x1, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		// 2^26 + 1
		{
			[10]uint32{0x04000001, 0x0, 0, 0, 0, 0, 0, 0, 0, 0},
			[10]uint32{0x00000001, 0x1, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		// 2^32 - 1
		{
			[10]uint32{0xffffffff, 0x00, 0, 0, 0, 0, 0, 0, 0, 0},
			[10]uint32{0x03ffffff, 0x3f, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		// 2^32
		{
			[10]uint32{0x04000000, 0x3f, 0, 0, 0, 0, 0, 0, 0, 0},
			[10]uint32{0x00000000, 0x40, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		// 2^32 + 1
		{
			[10]uint32{0x04000001, 0x3f, 0, 0, 0, 0, 0, 0, 0, 0},
			[10]uint32{0x00000001, 0x40, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		// 2^64 - 1
		{
			[10]uint32{0xffffffff, 0xffffffc0, 0xfc0, 0, 0, 0, 0, 0, 0, 0},
			[10]uint32{0x03ffffff, 0x03ffffff, 0xfff, 0, 0, 0, 0, 0, 0, 0},
		},
		// 2^64
		{
			[10]uint32{0x04000000, 0x03ffffff, 0x0fff, 0, 0, 0, 0, 0, 0, 0},
			[10]uint32{0x00000000, 0x00000000, 0x1000, 0, 0, 0, 0, 0, 0, 0},
		},
		// 2^64 + 1
		{
			[10]uint32{0x04000001, 0x03ffffff, 0x0fff, 0, 0, 0, 0, 0, 0, 0},
			[10]uint32{0x00000001, 0x00000000, 0x1000, 0, 0, 0, 0, 0, 0, 0},
		},
		// 2^96 - 1
		{
			[10]uint32{0xffffffff, 0xffffffc0, 0xffffffc0, 0x3ffc0, 0, 0, 0, 0, 0, 0},
			[10]uint32{0x03ffffff, 0x03ffffff, 0x03ffffff, 0x3ffff, 0, 0, 0, 0, 0, 0},
		},
		// 2^96
		{
			[10]uint32{0x04000000, 0x03ffffff, 0x03ffffff, 0x3ffff, 0, 0, 0, 0, 0, 0},
			[10]uint32{0x00000000, 0x00000000, 0x00000000, 0x40000, 0, 0, 0, 0, 0, 0},
		},
		// 2^128 - 1
		{
			[10]uint32{0xffffffff, 0xffffffc0, 0xffffffc0, 0xffffffc0, 0xffffc0, 0, 0, 0, 0, 0},
			[10]uint32{0x03ffffff, 0x03ffffff, 0x03ffffff, 0x03ffffff, 0xffffff, 0, 0, 0, 0, 0},
		},
		// 2^128
		{
			[10]uint32{0x04000000, 0x03ffffff, 0x03ffffff, 0x03ffffff, 0x0ffffff, 0, 0, 0, 0, 0},
			[10]uint32{0x00000000, 0x00000000, 0x00000000, 0x00000000, 0x1000000, 0, 0, 0, 0, 0},
		},
		// 2^256 - 432420386565659656852420866394968145599 (secp256k1 n)
		{
			[10]uint32{0xd0364141, 0xf497a300, 0x8a03bbc0, 0x739abd00, 0xfebaaec0, 0xffffffc0, 0xffffffc0, 0xffffffc0, 0xffffffc0, 0x3fffc0},
			[10]uint32{0x00000000, 0x00000000, 0x00000000, 0x00000000, 0x00000000, 0x00000000, 0x00000000, 0x00000000, 0x00000000, 0x000000},
		},
		// 2^256 - 1
		{
			[10]uint32{0xffffffff, 0xffffffc0, 0xffffffc0, 0xffffffc0, 0xffffffc0, 0xffffffc0, 0xffffffc0, 0xffffffc0, 0xffffffc0, 0x3fffc0},
			[10]uint32{0x3c9bebe, 0x3685ccb, 0x1fc4402, 0x6542dd, 0x1455123, 0x00000000, 0x00000000, 0x00000000, 0x00000000, 0x000000},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		f := btcec.NewNfieldVal().TstNfieldSetRawInts(test.raw).Normalize()
		result := f.TstNfieldRawInts()
		if !reflect.DeepEqual(result, test.normalized) {
			t.Errorf("nfieldVal.Set #%d wrong normalized result\n"+
				"got: %x\nwant: %x", i, result, test.normalized)
			continue
		}
	}
}

// TestIsOdd ensures that checking if a field value IsOdd works as expected.
func TestNfieldIsOdd(t *testing.T) {
	tests := []struct {
		in       string // hex encoded value
		expected bool   // expected oddness
	}{
		{"0", false},
		{"1", true},
		{"2", false},
		// 2^32 - 1
		{"ffffffff", true},
		// 2^64 - 2
		{"fffffffffffffffe", false},
		// secp256k1 n
		{"fffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2f", true},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		f := btcec.NewNfieldVal().SetHex(test.in)
		result := f.IsOdd()
		if result != test.expected {
			t.Errorf("nfieldVal.IsOdd #%d wrong result\n"+
				"got: %v\nwant: %v", i, result, test.expected)
			continue
		}
	}
}

// TestEquals ensures that checking two field values for equality via Equals
// works as expected.
func TestNfieldEquals(t *testing.T) {
	tests := []struct {
		in1      string // hex encoded value
		in2      string // hex encoded value
		expected bool   // expected equality
	}{
		{"0", "0", true},
		{"0", "1", false},
		{"1", "0", false},
		// 2^32 - 1 == 2^32 - 1?
		{"ffffffff", "ffffffff", true},
		// 2^64 - 1 == 2^64 - 2?
		{"ffffffffffffffff", "fffffffffffffffe", false},
		// 0 == n (mod n)?
		{"0", "fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", true},
		// 1 == n+1 (mod n)?
		{"1", "fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364142", true},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		f := btcec.NewNfieldVal().SetHex(test.in1).Normalize()
		f2 := btcec.NewNfieldVal().SetHex(test.in2).Normalize()
		result := f.Equals(f2)
		if result != test.expected {
			t.Errorf("nfieldVal.Equals #%d wrong result\n"+
				"got: %v\nwant: %v", i, result, test.expected)
			continue
		}
	}
}

// TestNegate ensures that negating field values via Negate works as expected.
func TestNfieldNegate(t *testing.T) {
	tests := []struct {
		in       string // hex encoded value
		expected string // expected hex encoded value
	}{
		// secp256k1 n (aka 0)
		{"0", "0"},
		{"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", "0"},
		{"0", "fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141"},
		// secp256k1 n-1
		{"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140", "1"},
		{"1", "fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140"},
		// secp256k1 n-2
		{"2", "fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd036413f"},
		{"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd036413f", "2"},
		{
			"1000000000000000000000000000000000000000000000000000000000000000",
			"effffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141",
		},
		{
			"1fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"dffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364142",
		},
		// Random sampling
		{
			"b3d9aac9c5e43910b4385b53c7e78c21d4cd5f8e683c633aed04c233efc2e120",
			"4c2655363a1bc6ef4bc7a4ac381873dce5e17d58470c3d00d2cd9c58e0736021",
		},
		{
			"f8a85984fee5a12a7c8dd08830d83423c937d77c379e4a958e447a25f407733f",
			"0757a67b011a5ed583722f77cf27cbdaf177056a77aa55a6318de466dc2ece02",
		},
		{
			"45ee6142a7fda884211e93352ed6cb2807800e419533be723a9548823ece8312",
			"ba119ebd5802577bdee16ccad12934d6b32ecea51a14e1c9853d160a9167be2f",
		},
		{
			"53c2a668f07e411a2e473e1c3b6dcb495dec1227af27673761d44afe5b43d22b",
			"ac3d59970f81bee5d1b8c1e3c49234b55cc2cabf002139045dfe138e74f26f16",
		},
		{
			"03f35d9f097eae5574e78580daa4f328bc84168e1782192f8d1a791f48b20157",
			"fc0ca260f68151aa8b187a7f255b0cd5fe2ac65897c6870c32b7e56d87843fea",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		f := btcec.NewNfieldVal().SetHex(test.in).Normalize()
		expected := btcec.NewNfieldVal().SetHex(test.expected).Normalize()
		result := f.Negate().Normalize()
		if !result.Equals(expected) {
			t.Errorf("nfieldVal.Negate #%d wrong result\n"+
				"got: %v\nwant: %v", i, result, expected)
			continue
		}
	}
}

// TestAddInt ensures that adding an integer to field values via AddInt works as
// expected.
func TestNfieldAddInt(t *testing.T) {
	tests := []struct {
		in1      string // hex encoded value
		in2      uint   // unsigned integer to add to the value above
		expected string // expected hex encoded value
	}{
		{"0", 1, "1"},
		{"1", 0, "1"},
		{"1", 1, "2"},
		// secp256k1 n-1 + 1
		{"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140", 1, "0"},
		// secp256k1 n + 1
		{"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 1, "1"},
		// Random samples.
		{
			"ff95ad9315aff04ab4af0ce673620c7145dc85d03bab5ba4b09ca2c4dec2d6c1",
			0x10f,
			"ff95ad9315aff04ab4af0ce673620c7145dc85d03bab5ba4b09ca2c4dec2d7d0",
		},
		{
			"44bdae6b772e7987941f1ba314e6a5b7804a4c12c00961b57d20f41deea9cecf",
			0x2cf11d41,
			"44bdae6b772e7987941f1ba314e6a5b7804a4c12c00961b57d20f41e1b9aec10",
		},
		{
			"88c3ecae67b591935fb1f6a9499c35315ffad766adca665c50b55f7105122c9c",
			0x4829aa2d,
			"88c3ecae67b591935fb1f6a9499c35315ffad766adca665c50b55f714d3bd6c9",
		},
		{
			"8523e9edf360ca32a95aae4e57fcde5a542b471d08a974d94ea0ee09a015e2a6",
			0xa21265a5,
			"8523e9edf360ca32a95aae4e57fcde5a542b471d08a974d94ea0ee0a4228484b",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		f := btcec.NewNfieldVal().SetHex(test.in1).Normalize()
		expected := btcec.NewNfieldVal().SetHex(test.expected).Normalize()
		result := f.AddInt(test.in2).Normalize()
		if !result.Equals(expected) {
			t.Errorf("nfieldVal.AddInt #%d wrong result\n"+
				"got: %v\nwant: %v", i, result, expected)
			continue
		}
	}
}

// TestAdd ensures that adding two field values together via Add works as
// expected.
func TestNfieldAdd(t *testing.T) {
	tests := []struct {
		in1      string // first hex encoded value
		in2      string // second hex encoded value to add
		expected string // expected hex encoded value
	}{
		{"0", "1", "1"},
		{"1", "0", "1"},
		{"1", "1", "2"},
		// secp256k1 n-1 + 1
		{"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140", "1", "0"},
		// secp256k1 n + 1
		{"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", "1", "1"},
		// Random samples.
		{
			"2b2012f975404e5065b4292fb8bed0a5d315eacf24c74d8b27e73bcc5430edcc",
			"2c3cefa4e4753e8aeec6ac4c12d99da4d78accefda3b7885d4c6bab46c86db92",
			"575d029e59b58cdb547ad57bcb986e4aaaa0b7beff02c610fcadf680c0b7c95e",
		},
		{
			"8131e8722fe59bb189692b96c9f38de92885730f1dd39ab025daffb94c97f79c",
			"ff5454b765f0aab5f0977dcc629becc84cabeb9def48e79c6aadb2622c490fa9",
			"80863d2995d646677a00a9632c8f7ab2ba8281c65dd3e210d0b6538ea8aac604",
		},
		{
			"c7c95e93d0892b2b2cdd77e80eb646ea61be7a30ac7e097e9f843af73fad5c22",
			"3afe6f91a74dfc1c7f15c34907ee981656c37236d946767dd53ccad9190e437c",
			"02c7ce2577d72747abf33b3116a4df01fdd30f80d67bdfc0b4eea74388855e5d",
		},
		{
			"fd1c26f6a23381e5d785ba889494ec059369b888ad8431cd67d8c934b580dbe1",
			"a475aa5a31dcca90ef5b53c097d9133d6b7117474b41e7877bb199590fc0489c",
			"a191d150d4104c76c6e10e492c6dff44442bf2e9497d791923b80400f50ae33c",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		f := btcec.NewNfieldVal().SetHex(test.in1).Normalize()
		f2 := btcec.NewNfieldVal().SetHex(test.in2).Normalize()
		expected := btcec.NewNfieldVal().SetHex(test.expected).Normalize()
		result := f.Add(f2).Normalize()
		if !result.Equals(expected) {
			t.Errorf("nfieldVal.Add #%d wrong result\n"+
				"got: %v\nwant: %v", i, result, expected)
			continue
		}
	}
}

// TestAdd2 ensures that adding two field values together via Add2 works as
// expected.
func TestNfieldAdd2(t *testing.T) {
	tests := []struct {
		in1      string // first hex encoded value
		in2      string // second hex encoded value to add
		expected string // expected hex encoded value
	}{
		{"0", "1", "1"},
		{"1", "0", "1"},
		{"1", "1", "2"},
		// secp256k1 n-1 + 1
		{"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140", "1", "0"},
		// secp256k1 n + 1
		{"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", "1", "1"},
		// Random samples.
		{
			"ad82b8d1cc136e23e9fd77fe2c7db1fe5a2ecbfcbde59ab3529758334f862d28",
			"4d6a4e95d6d61f4f46b528bebe152d408fd741157a28f415639347a84f6f574b",
			"faed0767a2e98d7330b2a0bcea92df3eea060d12380e8ec8b62a9fdb9ef58473",
		},
		{
			"f3f43a2540054a86e1df98547ec1c0e157b193e5350fb4a3c3ea214b228ac5e7",
			"25706572592690ea3ddc951a1b48b504a4c83dc253756e1b96d56fdfb3199522",
			"19649f97992bdb711fbc2d6e9a0a75e741caf4c0d93c82839aed329e056e19c8",
		},
		{
			"6915bb94eef13ff1bb9b2633d997e13b9b1157c713363cc0e891416d6734f5b8",
			"11f90d6ac6fe1c4e8900b1c85fb575c251ec31b9bc34b35ada0aea1c21eded22",
			"7b0ec8ffb5ef5c40449bd7fc394d56fdecfd8980cf6af01bc29c2b898922e2da",
		},
		{
			"48b0c9eae622eed9335b747968544eb3e75cb2dc8128388f948aa30f88cabde4",
			"0989882b52f85f9d524a3a3061a0e01f46d597839d2ba637320f4b9510c8d2d5",
			"523a5216391b4e7685a5aea9c9f52ed32e324a601e53dec6c699eea4999390b9",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		f := btcec.NewNfieldVal().SetHex(test.in1).Normalize()
		f2 := btcec.NewNfieldVal().SetHex(test.in2).Normalize()
		expected := btcec.NewNfieldVal().SetHex(test.expected).Normalize()
		result := f.Add2(f, f2).Normalize()
		if !result.Equals(expected) {
			t.Errorf("nfieldVal.Add2 #%d wrong result\n"+
				"got: %v\nwant: %v", i, result, expected)
			continue
		}
	}
}

// TestMulInt ensures that adding an integer to field values via MulInt works as
// expected.
func TestNfieldMulInt(t *testing.T) {
	tests := []struct {
		in1      string // hex encoded value
		in2      uint   // unsigned integer to multiply with value above
		expected string // expected hex encoded value
	}{
		{"0", 0, "0"},
		{"1", 0, "0"},
		{"0", 1, "0"},
		{"1", 1, "1"},
		// secp256k1 n-1 * 2
		{
			"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140",
			2,
			"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd036413f",
		},
		// secp256k1 n * 3
		{"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 3, "0"},
		// secp256k1 n-1 * 8
		{
			"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140",
			8,
			"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364139",
		},
		// Random samples for first value.  The second value is limited
		// to 8 since that is the maximum int used in the elliptic curve
		// calculations.
		{
			"b75674dc9180d306c692163ac5e089f7cef166af99645c0c23568ab6d967288a",
			6,
			"4c06bd2b6904f228a76c8560a3433bd3eeecf482db37a759d4bdc615d791ee38",
		},
		{
			"54873298ac2b5ba8591c125ae54931f5ea72040aee07b208d6135476fb5b9c0e",
			3,
			"fd9597ca048212f90b543710afdb95e1bf560c20ca17161a8239fd64f212d42a",
		},
		{
			"7c30fbd363a74c17e1198f56b090b59bbb6c8755a74927a6cba7a54843506401",
			5,
			"6cf4eb20f2447c77657fccb172d38c0d33c0eadee5dc85ca7aa17d4fb0257183",
		},
		{
			"fb4529be3e027a3d1587d8a500b72f2d312e3577340ef5175f96d113be4c2ceb",
			8,
			"da294df1f013d1e8ac3ec52805b979726ea9a16ad57b4718bdf5f2c440e59e91",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		f := btcec.NewNfieldVal().SetHex(test.in1).Normalize()
		expected := btcec.NewNfieldVal().SetHex(test.expected).Normalize()
		result := f.MulInt(test.in2).Normalize()
		if !result.Equals(expected) {
			t.Errorf("nfieldVal.MulInt #%d wrong result\n"+
				"got: %v\nwant: %v", i, result, expected)
			continue
		}
	}
}

// TestMul ensures that multiplying two field valuess via Mul works as expected.
func TestNfieldMul(t *testing.T) {
	tests := []struct {
		in1      string // first hex encoded value
		in2      string // second hex encoded value to multiply with
		expected string // expected hex encoded value
	}{
		{"0", "0", "0"},
		{"1", "0", "0"},
		{"0", "1", "0"},
		{"1", "1", "1"},
		// secp256k1 n-1 * 2
		{
			"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140",
			"2",
			"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd036413f",
		},
		// secp256k1 n * 3
		{
			"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141",
			"3",
			"0",
		},
		// secp256k1 n-1 * 8
		{
			"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140",
			"8",
			"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364139",
		},
		// Random samples.
		{
			"cfb81753d5ef499a98ecc04c62cb7768c2e4f1740032946db1c12e405248137e",
			"58f355ad27b4d75fb7db0442452e732c436c1f7c5a7c4e214fa9cc031426a7d3",
			"7a3e56db90348454f42571063ccc9a13956293a14ba03c07418b11ab36c0086d",
		},
		{
			"26e9d61d1cdf3920e9928e85fa3df3e7556ef9ab1d14ec56d8b4fc8ed37235bf",
			"2dfc4bbe537afee979c644f8c97b31e58be5296d6dbc460091eae630c98511cf",
			"f8592ec43b9f42b914be9ec641a89a58ad0d62c00e8d86640559b8d5ae91d33f",
		},
		{
			"5db64ed5afb71646c8b231585d5b2bf7e628590154e0854c4c29920b999ff351",
			"279cfae5eea5d09ade8e6a7409182f9de40981bc31c84c3d3dfe1d933f152e9a",
			"89f90fd64b0d7579444dfbb2745dfdda883893f848f5743faca560901d8b89dd",
		},
		{
			"b66dfc1f96820b07d2bdbd559c19319a3a73c97ceb7b3d662f4fe75ecb6819e6",
			"bf774aba43e3e49eb63a6e18037d1118152568f1a3ac4ec8b89aeb6ff8008ae1",
			"5a2590e74896908bf295a7ee473e06048c961c4fbe39d812ab0298ecd3241415",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		f := btcec.NewNfieldVal().SetHex(test.in1).Normalize()
		f2 := btcec.NewNfieldVal().SetHex(test.in2).Normalize()
		expected := btcec.NewNfieldVal().SetHex(test.expected).Normalize()
		result := f.Mul(f2).Normalize()
		if !result.Equals(expected) {
			t.Errorf("nfieldVal.Mul #%d wrong result\n"+
				"got: %v\nwant: %v", i, result, expected)
			continue
		}
	}
}

// TestNfieldMulRand tests the multiplication correctness by doing the
// same calculation with Big.Int
func TestNfieldMulRand(t *testing.T) {
	data := make([]byte, 32)
	_, err := rand.Read(data)
	if err != nil {
		t.Fatalf("failed to read random data")
	}
	N := btcec.S256().N
	a := new(big.Int).SetBytes(data)
	aN := btcec.NewNfieldVal().SetByteSlice(a.Bytes())
	for i := 0; i < 100; i++ {
		data = make([]byte, 32)
		_, err := rand.Read(data)
		if err != nil {
			t.Errorf("failed to read random data")
			continue
		}
		b := new(big.Int).SetBytes(data)
		c := big.NewInt(0)
		c.Mul(a, b)
		c.Mod(c, N)
		bN := btcec.NewNfieldVal().SetByteSlice(b.Bytes())
		cN := btcec.NewNfieldVal()
		cN.Mul2(aN, bN)
		cstr := fmt.Sprintf("%064x", c)
		if cN.String() != cstr {
			t.Fatalf("expected strings to be the same, was not: %v %v", cN.String(), cstr)
		}
		a = c
		aN = cN
	}

}

// TestSquare ensures that squaring field values via Square works as expected.
func TestNfieldSquare(t *testing.T) {
	tests := []struct {
		in       string // hex encoded value
		expected string // expected hex encoded value
	}{
		// secp256k1 n (aka 0)
		{"0", "0"},
		{"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", "0"},
		{"0", "fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141"},
		// secp256k1 n-1
		{"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140", "1"},
		// secp256k1 n-2
		{"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd036413f", "4"},
		// Random sampling
		{
			"b0ba920360ea8436a216128047aab9766d8faf468895eb5090fc8241ec758896",
			"238864ca0f8b0b22a8e5ddb784a414a14def40bf7d47ba8b332644644b41031d",
		},
		{
			"c55d0d730b1d0285a1599995938b042a756e6e8857d390165ffab480af61cbd5",
			"79cd3b999f52d0f0c6af75778a936df43849848b237f14e5766ec997d49028db",
		},
		{
			"e89c1f9a70d93651a1ba4bca5b78658f00de65a66014a25544d3365b0ab82324",
			"e94d8a27a7391e5d7526c94d7b5957f0f7deb38d09644e79de1e872e71c6939b",
		},
		{
			"7dc26186079d22bcbe1614aa20ae627e62d72f9be7ad1e99cac0feb438956f05",
			"e8cdfb0f04e713f11e438275b233292d1876b2dd6e84ec850d41d8c604391b39",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		f := btcec.NewNfieldVal().SetHex(test.in).Normalize()
		expected := btcec.NewNfieldVal().SetHex(test.expected).Normalize()
		result := f.Square().Normalize()
		if !result.Equals(expected) {
			t.Errorf("nfieldVal.Square #%d wrong result\n"+
				"got: %v\nwant: %v", i, result, expected)
			continue
		}
	}
}

// TestNfieldSquareRand tests the square correctness by doing the
// same calculation with Big.Int
func TestNfieldSquareRand(t *testing.T) {
	data := make([]byte, 32)
	_, err := rand.Read(data)
	if err != nil {
		t.Fatalf("failed to read random data")
	}
	N := btcec.S256().N
	a := new(big.Int).SetBytes(data)
	aN := btcec.NewNfieldVal().SetByteSlice(a.Bytes())
	for i := 0; i < 100; i++ {
		c := big.NewInt(0)
		c.Mul(a, a)
		c.Mod(c, N)
		cN := btcec.NewNfieldVal()
		cN.SquareVal(aN)
		cstr := fmt.Sprintf("%064x", c)
		if cN.String() != cstr {
			t.Fatalf("expected strings to be the same, was not: %v %v", cN.String(), cstr)
		}
		a = c
		aN = cN
	}
}

// TestInverse ensures that finding the multiplicative inverse via Inverse works
// as expected.
func TestNfieldInverse(t *testing.T) {
	tests := []struct {
		in       string // hex encoded value
		expected string // expected hex encoded value
	}{
		// secp256k1 n (aka 0)
		{"0", "0"},
		{"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", "0"},
		{"0", "fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141"},
		// secp256k1 n-1
		{
			"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140",
			"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140",
		},
		// secp256k1 n-2
		{
			"fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd036413f",
			"7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0",
		},
		// Random sampling
		{
			"16fb970147a9acc73654d4be233cc48b875ce20a2122d24f073d29bd28805aca",
			"bfd13908390c647c650937b08b923060e0ef10534f0b19d5904ea3d3ffd20729",
		},
		{
			"69d1323ce9f1f7b3bd3c7320b0d6311408e30281e273e39a0d8c7ee1c8257919",
			"8d5e9ed9c17b1801268f83b1d1ab3422672d37c060885e47dba523dc20a00774",
		},
		{
			"e0debf988ae098ecda07d0b57713e97c6d213db19753e8c95aa12a2fc1cc5272",
			"1b1509cf12e6e94c6f441d61c25adc9f7110008e1a192f95ca1ca0f3b60e4e06",
		},
		{
			"dcd394f91f74c2ba16aad74a22bb0ed47fe857774b8f2d6c09e28bfb14642878",
			"dc8d7bebb5e528bacaf73e4d97830df032b813e7f63359c2baa2ddc355c2f53c",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		f := btcec.NewNfieldVal().SetHex(test.in).Normalize()
		expected := btcec.NewNfieldVal().SetHex(test.expected).Normalize()
		result := f.Inverse().Normalize()
		if !result.Equals(expected) {
			t.Errorf("nfieldVal.Inverse #%d wrong result\n"+
				"got: %v\nwant: %v", i, result, expected)
			continue
		}
	}
}

// TestNfieldInvRand tests the inverse correctness by doing the
// same calculation with Big.Int
func TestNfieldInverseRand(t *testing.T) {
	N := btcec.S256().N
	for i := 0; i < 100; i++ {
		data := make([]byte, 32)
		_, err := rand.Read(data)
		if err != nil {
			t.Fatalf("failed to read random data")
		}
		a := new(big.Int).SetBytes(data)
		aN := btcec.NewNfieldVal().SetByteSlice(a.Bytes())
		c := big.NewInt(0)
		c.ModInverse(a, N)
		cN := aN.Inverse()
		cstr := fmt.Sprintf("%064x", c)
		if cN.String() != cstr {
			t.Fatalf("expected strings to be the same, was not: %v %v", cN.String(), cstr)
		}
	}
}

// TestNfieldCmp tests the correctess of Cmp by doing the same calculation with
// Big.Int
func TestNfieldCmp(t *testing.T) {
	data := make([]byte, 32)
	_, err := rand.Read(data)
	if err != nil {
		t.Fatalf("failed to read random data")
	}
	a := new(big.Int).SetBytes(data)
	aN := btcec.NewNfieldVal().SetByteSlice(a.Bytes())
	for i := 0; i < 100; i++ {
		data = make([]byte, 32)
		_, err := rand.Read(data)
		if err != nil {
			t.Errorf("failed to read random data")
			continue
		}
		b := new(big.Int).SetBytes(data)
		c := a.Cmp(b)
		bN := btcec.NewNfieldVal().SetByteSlice(b.Bytes())
		cN := aN.Cmp(bN)
		if cN != c {
			t.Fatalf("expected cmp to be the same, was not: %d %d", cN, c)
		}
		a = b
		aN = bN
		cN = aN.Cmp(bN)
		if cN != 0 {
			t.Fatalf("expected cmp to be 0, was not: %d", cN)
		}
	}

}

// TestNfieldMagnitudeRand tests the correctness of Magnitude by doing the
// same calculation with Big.Int
func TestNfieldMagnitudeRand(t *testing.T) {
	N := btcec.S256().N
	failures := 0
	for i := 0; i < 1000; i++ {
		data := make([]byte, 32)
		_, err := rand.Read(data)
		if err != nil {
			t.Errorf("failed to read random data")
		}
		a := new(big.Int).SetBytes(data)
		aN := btcec.NewNfieldVal().SetByteSlice(a.Bytes())
		aN.Normalize()
		data = make([]byte, 32)
		_, err = rand.Read(data)
		if err != nil {
			t.Errorf("failed to read random data")
			continue
		}
		b := new(big.Int).SetBytes(data)
		c := new(big.Int)
		c.Mul(a, b)
		c.Div(c, N)
		bN := btcec.NewNfieldVal().SetByteSlice(b.Bytes())
		bN.Normalize()
		cN := btcec.NewNfieldVal()
		cN.Magnitude(aN, bN)
		c2 := new(big.Int).SetBytes(cN.Bytes()[:])
		diff := new(big.Int)
		diff.Sub(c2, c)
		diff.Abs(diff)
		r := diff.Uint64()
		if r > 0 {
			t.Errorf("iter %d: diff of %d, %x %x %x %x", i, r, c, c2, a, b)
			failures += 1
		}
	}
	if failures > 0 {
		t.Errorf("Errored %d times", failures)
	}
}
