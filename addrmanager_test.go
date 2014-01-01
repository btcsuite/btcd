// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"github.com/conformal/btcwire"
	"net"
	"testing"
	"time"
)

// naTest is used to describe a test to be perfomed against the NetAddressKey
// method.
type naTest struct {
	in   btcwire.NetAddress
	want string
}

type ipTest struct {
	in       btcwire.NetAddress
	rfc1918  bool
	rfc3849  bool
	rfc3927  bool
	rfc3964  bool
	rfc4193  bool
	rfc4380  bool
	rfc4843  bool
	rfc4862  bool
	rfc6052  bool
	rfc6145  bool
	local    bool
	valid    bool
	routable bool
}

// naTests houses all of the tests to be performed against the NetAddressKey
// method.
var naTests = make([]naTest, 0)
var ipTests = make([]ipTest, 0)

func addIpTest(ip string, rfc1918, rfc3849, rfc3927, rfc3964, rfc4193, rfc4380,
	rfc4843, rfc4862, rfc6052, rfc6145, local, valid, routable bool) {
	nip := net.ParseIP(ip)
	na := btcwire.NetAddress{
		Timestamp: time.Now(),
		Services:  btcwire.SFNodeNetwork,
		IP:        nip,
		Port:      8333,
	}
	test := ipTest{na, rfc1918, rfc3849, rfc3927, rfc3964, rfc4193, rfc4380,
		rfc4843, rfc4862, rfc6052, rfc6145, local, valid, routable}
	ipTests = append(ipTests, test)
}

func addIpTests() {
	addIpTest("10.255.255.255", true, false, false, false, false, false,
		false, false, false, false, false, true, false)
	addIpTest("192.168.0.1", true, false, false, false, false, false,
		false, false, false, false, false, true, false)
	addIpTest("172.31.255.1", true, false, false, false, false, false,
		false, false, false, false, false, true, false)
	addIpTest("172.32.1.1", false, false, false, false, false, false,
		false, false, false, false, false, true, true)
	addIpTest("169.254.250.120", false, false, true, false, false, false,
		false, false, false, false, false, true, false)
	addIpTest("0.0.0.0", false, false, false, false, false, false,
		false, false, false, false, true, false, false)
	addIpTest("255.255.255.255", false, false, false, false, false, false,
		false, false, false, false, false, false, false)
	addIpTest("127.0.0.1", false, false, false, false, false, false,
		false, false, false, false, true, true, false)
	addIpTest("fd00:dead::1", false, false, false, false, true, false,
		false, false, false, false, false, true, false)
	addIpTest("2001::1", false, false, false, false, false, true,
		false, false, false, false, false, true, true)
	addIpTest("2001:10:abcd::1:1", false, false, false, false, false, false,
		true, false, false, false, false, true, false)
	addIpTest("fe80::1", false, false, false, false, false, false,
		false, true, false, false, false, true, false)
	addIpTest("fe80:1::1", false, false, false, false, false, false,
		false, false, false, false, false, true, true)
	addIpTest("64:ff9b::1", false, false, false, false, false, false,
		false, false, true, false, false, true, true)
	addIpTest("::ffff:abcd:ef12:1", false, false, false, false, false, false,
		false, false, false, false, false, true, true)
	addIpTest("::1", false, false, false, false, false, false,
		false, false, false, false, true, true, false)
}

func addNaTest(ip string, port uint16, want string) {
	nip := net.ParseIP(ip)
	na := btcwire.NetAddress{
		Timestamp: time.Now(),
		Services:  btcwire.SFNodeNetwork,
		IP:        nip,
		Port:      port,
	}
	test := naTest{na, want}
	naTests = append(naTests, test)
}

// addNaTests
func addNaTests() {
	// IPv4
	// Localhost
	addNaTest("127.0.0.1", 8333, "127.0.0.1:8333")
	addNaTest("127.0.0.1", 8334, "127.0.0.1:8334")

	// Class A
	addNaTest("1.0.0.1", 8333, "1.0.0.1:8333")
	addNaTest("2.2.2.2", 8334, "2.2.2.2:8334")
	addNaTest("27.253.252.251", 8335, "27.253.252.251:8335")
	addNaTest("123.3.2.1", 8336, "123.3.2.1:8336")

	// Private Class A
	addNaTest("10.0.0.1", 8333, "10.0.0.1:8333")
	addNaTest("10.1.1.1", 8334, "10.1.1.1:8334")
	addNaTest("10.2.2.2", 8335, "10.2.2.2:8335")
	addNaTest("10.10.10.10", 8336, "10.10.10.10:8336")

	// Class B
	addNaTest("128.0.0.1", 8333, "128.0.0.1:8333")
	addNaTest("129.1.1.1", 8334, "129.1.1.1:8334")
	addNaTest("180.2.2.2", 8335, "180.2.2.2:8335")
	addNaTest("191.10.10.10", 8336, "191.10.10.10:8336")

	// Private Class B
	addNaTest("172.16.0.1", 8333, "172.16.0.1:8333")
	addNaTest("172.16.1.1", 8334, "172.16.1.1:8334")
	addNaTest("172.16.2.2", 8335, "172.16.2.2:8335")
	addNaTest("172.16.172.172", 8336, "172.16.172.172:8336")

	// Class C
	addNaTest("193.0.0.1", 8333, "193.0.0.1:8333")
	addNaTest("200.1.1.1", 8334, "200.1.1.1:8334")
	addNaTest("205.2.2.2", 8335, "205.2.2.2:8335")
	addNaTest("223.10.10.10", 8336, "223.10.10.10:8336")

	// Private Class C
	addNaTest("192.168.0.1", 8333, "192.168.0.1:8333")
	addNaTest("192.168.1.1", 8334, "192.168.1.1:8334")
	addNaTest("192.168.2.2", 8335, "192.168.2.2:8335")
	addNaTest("192.168.192.192", 8336, "192.168.192.192:8336")

	// IPv6
	// Localhost
	addNaTest("::1", 8333, "[::1]:8333")
	addNaTest("fe80::1", 8334, "[fe80::1]:8334")

	// Link-local
	addNaTest("fe80::1:1", 8333, "[fe80::1:1]:8333")
	addNaTest("fe91::2:2", 8334, "[fe91::2:2]:8334")
	addNaTest("fea2::3:3", 8335, "[fea2::3:3]:8335")
	addNaTest("feb3::4:4", 8336, "[feb3::4:4]:8336")

	// Site-local
	addNaTest("fec0::1:1", 8333, "[fec0::1:1]:8333")
	addNaTest("fed1::2:2", 8334, "[fed1::2:2]:8334")
	addNaTest("fee2::3:3", 8335, "[fee2::3:3]:8335")
	addNaTest("fef3::4:4", 8336, "[fef3::4:4]:8336")
}

func TestGetAddress(t *testing.T) {
	n := NewAddrManager()
	if rv := n.GetAddress("any", 10); rv != nil {
		t.Errorf("GetAddress failed: got: %v want: %v\n", rv, nil)
	}
}

func TestIpTypes(t *testing.T) {
	addIpTests()

	t.Logf("Running %d tests", len(ipTests))
	for _, test := range ipTests {
		rv := RFC1918(&test.in)
		if rv != test.rfc1918 {
			t.Errorf("RFC1918 %s\n got: %v want: %v", test.in.IP, rv, test.rfc1918)
			continue
		}
	}
	for _, test := range ipTests {
		rv := RFC3849(&test.in)
		if rv != test.rfc3849 {
			t.Errorf("RFC3849 %s\n got: %v want: %v", test.in.IP, rv, test.rfc3849)
			continue
		}
	}
	for _, test := range ipTests {
		rv := RFC3927(&test.in)
		if rv != test.rfc3927 {
			t.Errorf("RFC3927 %s\n got: %v want: %v", test.in.IP, rv, test.rfc3927)
			continue
		}
	}
	for _, test := range ipTests {
		rv := RFC3964(&test.in)
		if rv != test.rfc3964 {
			t.Errorf("RFC3964 %s\n got: %v want: %v", test.in.IP, rv, test.rfc3964)
			continue
		}
	}
	for _, test := range ipTests {
		rv := RFC4193(&test.in)
		if rv != test.rfc4193 {
			t.Errorf("RFC4193 %s\n got: %v want: %v", test.in.IP, rv, test.rfc4193)
			continue
		}
	}
	for _, test := range ipTests {
		rv := RFC4380(&test.in)
		if rv != test.rfc4380 {
			t.Errorf("RFC4380 %s\n got: %v want: %v", test.in.IP, rv, test.rfc4380)
			continue
		}
	}
	for _, test := range ipTests {
		rv := RFC4843(&test.in)
		if rv != test.rfc4843 {
			t.Errorf("RFC4843 %s\n got: %v want: %v", test.in.IP, rv, test.rfc4843)
			continue
		}
	}
	for _, test := range ipTests {
		rv := RFC4862(&test.in)
		if rv != test.rfc4862 {
			t.Errorf("RFC4862 %s\n got: %v want: %v", test.in.IP, rv, test.rfc4862)
			continue
		}
	}
	for _, test := range ipTests {
		rv := RFC6052(&test.in)
		if rv != test.rfc6052 {
			t.Errorf("RFC6052 %s\n got: %v want: %v", test.in.IP, rv, test.rfc6052)
			continue
		}
	}
	for _, test := range ipTests {
		rv := RFC6145(&test.in)
		if rv != test.rfc6145 {
			t.Errorf("RFC1918 %s\n got: %v want: %v", test.in.IP, rv, test.rfc6145)
			continue
		}
	}
	for _, test := range ipTests {
		rv := Local(&test.in)
		if rv != test.local {
			t.Errorf("Local %s\n got: %v want: %v", test.in.IP, rv, test.local)
			continue
		}
	}
	for _, test := range ipTests {
		rv := Valid(&test.in)
		if rv != test.valid {
			t.Errorf("Valid %s\n got: %v want: %v", test.in.IP, rv, test.valid)
			continue
		}
	}
	for _, test := range ipTests {
		rv := Routable(&test.in)
		if rv != test.routable {
			t.Errorf("Routable %s\n got: %v want: %v", test.in.IP, rv, test.routable)
			continue
		}
	}
}

func TestNetAddressKey(t *testing.T) {
	addNaTests()

	t.Logf("Running %d tests", len(naTests))
	for i, test := range naTests {
		key := NetAddressKey(&test.in)
		if key != test.want {
			t.Errorf("NetAddressKey #%d\n got: %s want: %s", i, key, test.want)
			continue
		}
	}

}
