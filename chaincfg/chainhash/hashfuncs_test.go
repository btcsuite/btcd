// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chainhash

import (
	"fmt"
	"testing"
)

// TestHashFuncs ensures the hash functions which perform hash(b) work as
// expected.
func TestHashFuncs(t *testing.T) {
	tests := []struct {
		out string
		in  string
	}{
		{"716f6e863f744b9ac22c97ec7b76ea5f5908bc5b2f67c61510bfc4751384ea7a", ""},
		{"43234ff894a9c0590d0246cfc574eb781a80958b01d7a2fa1ac73c673ba5e311", "a"},
		{"658c6d9019a1deddbcb3640a066dfd23471553a307ab941fd3e677ba887be329", "ab"},
		{"1833a9fa7cf4086bd5fda73da32e5a1d75b4c3f89d5c436369f9d78bb2da5c28", "abc"},
		{"35282468f3b93c5aaca6408582fced36e578f67671ed0741c332d68ac72d7aa2", "abcd"},
		{"9278d633efce801c6aa62987d7483d50e3c918caed7d46679551eed91fba8904", "abcde"},
		{"7a17ee5e289845adcafaf6ca1b05c4a281b232a71c7083f66c19ba1d1169a8d4", "abcdef"},
		{"ee8c7f94ff805cb2e644643010ea43b0222056420917ec70c3da764175193f8f", "abcdefg"},
		{"7b37c0876d29c5add7800a1823795a82b809fc12f799ff6a4b5e58d52c42b17e", "abcdefgh"},
		{"bdc514bea74ffbb9c3aa6470b08ceb80a88e313ad65e4a01457bbffd0acc86de", "abcdefghi"},
		{"12e3afb9739df8d727e93d853faeafc374cc55aedc937e5a1e66f5843b1d4c2e", "abcdefghij"},
		{"22297d373b751f581944bb26315133f6fda2f0bf60f65db773900f61f81b7e79", "Discard medicine more than two years old."},
		{"4d48d137bc9cf6d21415b805bf33f59320337d85c673998260e03a02a0d760cd", "He who has a shady past knows that nice guys finish last."},
		{"beba299e10f93e17d45663a6dc4b8c9349e4f5b9bac0d7832389c40a1b401e5c", "I wouldn't marry him with a ten foot pole."},
		{"42e082ae7f967781c6cd4e0ceeaeeb19fb2955adbdbaf8c7ec4613ac130071b3", "Free! Free!/A trip/to Mars/for 900/empty jars/Burma Shave"},
		{"207d06b205bfb359df91b48b6fd8aa6e4798b712d1cc5e91a254da9cef8684a3", "The days of the digital watch are numbered.  -Tom Stoppard"},
		{"d56eab6927e371e2148b0788779aaf565d30567af2af822b6be3b90db9767a70", "Nepal premier won't resign."},
		{"01020709ca7fd10dc7756ce767d508d7206167d300b7a7ed76838a8547a7898c", "For every action there is an equal and opposite government program."},
		{"5569a6cc6535a66da221d8f6ad25008f28752d0343f3f1d757f1ecc9b1c61536", "His money is twice tainted: 'taint yours and 'taint mine."},
		{"8ff699b5ac7687c82600e89d0ff6cfa87e7179759184386971feb76fbae9975f", "There is no reason for any individual to have a computer in their home. -Ken Olsen, 1977"},
		{"f4b3a7c85a418b15ce330fd41ae0254b036ad48dd98aa37f0506a995ba9c6029", "It's a tiny change to the code and not completely disgusting. - Bob Manchek"},
		{"1ed94bab64fe560ef0983165fcb067e9a8a971c1db8e6fb151ff9a7c7fe877e3", "size:  a.out:  bad magic"},
		{"ff15b54992eedf9889f7b4bbb16692881aa01ed10dfc860fdb04785d8185cd3c", "The major problem is with sendmail.  -Mark Horton"},
		{"8a0a7c417a47deec0b6474d8c247da142d2e315113a2817af3de8f45690d8652", "Give me a rock, paper and scissors and I will move the world.  CCFestoon"},
		{"310d263fdab056a930324cdea5f46f9ea70219c1a74b01009994484113222a62", "If the enemy is within range, then so are you."},
		{"1aaa0903aa4cf872fe494c322a6e535698ea2140e15f26fb6088287aedceb6ba", "It's well we cannot hear the screams/That we create in others' dreams."},
		{"2eb81bcaa9e9185a7587a1b26299dcfb30f2a58a7f29adb584b969725457ad4f", "You remind me of a TV show, but that's all right: I watch it anyway."},
		{"c27b1683ef76e274680ab5492e592997b0d9d5ac5a5f4651b6036f64215256af", "C is as portable as Stonehedge!!"},
		{"3995cce8f32b174c22ffac916124bd095c80205d9d5f1bb08a155ac24b40d6cb", "Even if I could be Shakespeare, I think I should still choose to be Faraday. - A. Huxley"},
		{"496f7063f8bd479bf54e9d87e9ba53e277839ac7fdaecc5105f2879b58ee562f", "The fugacity of a constituent in a mixture of gases at a given temperature is proportional to its mole fraction.  Lewis-Randall Rule"},
		{"2e0eff918940b01eea9539a02212f33ee84f77fab201f4287aa6167e4a1ed043", "How can you write a big system without C++?  -Paul Glick"},
	}

	// Ensure the hash function which returns a byte slice returns the
	// expected result.
	for _, test := range tests {
		h := fmt.Sprintf("%x", HashB([]byte(test.in)))
		if h != test.out {
			t.Errorf("HashB(%q) = %s, want %s", test.in, h, test.out)
			continue
		}
	}

	// Ensure the hash function which returns a Hash returns the expected
	// result.
	for _, test := range tests {
		hash := HashH([]byte(test.in))
		h := fmt.Sprintf("%x", hash[:])
		if h != test.out {
			t.Errorf("HashH(%q) = %s, want %s", test.in, h, test.out)
			continue
		}
	}
}
