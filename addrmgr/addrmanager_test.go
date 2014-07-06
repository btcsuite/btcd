// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package addrmgr_test

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/conformal/btcd/addrmgr"
	"github.com/conformal/btcwire"
)

// naTest is used to describe a test to be perfomed against the NetAddressKey
// method.
type naTest struct {
	in   btcwire.NetAddress
	want string
}

// naTests houses all of the tests to be performed against the NetAddressKey
// method.
var naTests = make([]naTest, 0)

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

func lookupFunc(host string) ([]net.IP, error) {
	return nil, errors.New("not implemented")
}

func TestAddLocalAddress(t *testing.T) {
	var tests = []struct {
		address btcwire.NetAddress
		valid   bool
	}{
		{
			btcwire.NetAddress{IP: net.ParseIP("192.168.0.100")},
			false,
		},
		{
			btcwire.NetAddress{IP: net.ParseIP("204.124.1.1")},
			true,
		},
		{
			btcwire.NetAddress{IP: net.ParseIP("::1")},
			false,
		},
		{
			btcwire.NetAddress{IP: net.ParseIP("fe80::1")},
			false,
		},
		{
			btcwire.NetAddress{IP: net.ParseIP("2620:100::1")},
			true,
		},
	}
	amgr := addrmgr.New("", nil)
	for x, test := range tests {
		result := amgr.AddLocalAddress(&test.address, addrmgr.InterfacePrio)
		if result == nil && !test.valid {
			t.Errorf("TestAddLocalAddress test #%d failed: %s should have "+
				"been accepted", x, test.address.IP)
			continue
		}
		if result != nil && test.valid {
			t.Errorf("TestAddLocalAddress test #%d failed: %s should not have "+
				"been accepted", test.address.IP)
			continue
		}
	}
}

func TestGetAddress(t *testing.T) {
	n := addrmgr.New("testdir", lookupFunc)
	if rv := n.GetAddress("any", 10); rv != nil {
		t.Errorf("GetAddress failed: got: %v want: %v\n", rv, nil)
	}
}

func TestGetBestLocalAddress(t *testing.T) {
	localAddrs := []btcwire.NetAddress{
		{IP: net.ParseIP("192.168.0.100")},
		{IP: net.ParseIP("::1")},
		{IP: net.ParseIP("fe80::1")},
		{IP: net.ParseIP("2001:470::1")},
	}

	var tests = []struct {
		remoteAddr btcwire.NetAddress
		want1      btcwire.NetAddress
		want2      btcwire.NetAddress
		want3      btcwire.NetAddress
	}{
		{
			// Remote connection from public IPv4
			btcwire.NetAddress{IP: net.ParseIP("204.124.8.1")},
			btcwire.NetAddress{IP: net.IPv4zero},
			btcwire.NetAddress{IP: net.ParseIP("204.124.8.100")},
			btcwire.NetAddress{IP: net.ParseIP("fd87:d87e:eb43:25::1")},
		},
		{
			// Remote connection from private IPv4
			btcwire.NetAddress{IP: net.ParseIP("172.16.0.254")},
			btcwire.NetAddress{IP: net.IPv4zero},
			btcwire.NetAddress{IP: net.IPv4zero},
			btcwire.NetAddress{IP: net.IPv4zero},
		},
		{
			// Remote connection from public IPv6
			btcwire.NetAddress{IP: net.ParseIP("2602:100:abcd::102")},
			btcwire.NetAddress{IP: net.ParseIP("2001:470::1")},
			btcwire.NetAddress{IP: net.ParseIP("2001:470::1")},
			btcwire.NetAddress{IP: net.ParseIP("2001:470::1")},
		},
		/* XXX
		{
			// Remote connection from Tor
			btcwire.NetAddress{IP: net.ParseIP("fd87:d87e:eb43::100")},
			btcwire.NetAddress{IP: net.IPv4zero},
			btcwire.NetAddress{IP: net.ParseIP("204.124.8.100")},
			btcwire.NetAddress{IP: net.ParseIP("fd87:d87e:eb43:25::1")},
		},
		*/
	}

	amgr := addrmgr.New("", nil)
	for _, localAddr := range localAddrs {
		amgr.AddLocalAddress(&localAddr, addrmgr.InterfacePrio)
	}

	// Test against want1
	for x, test := range tests {
		got := amgr.GetBestLocalAddress(&test.remoteAddr)
		if !test.want1.IP.Equal(got.IP) {
			t.Errorf("TestGetBestLocalAddress test1 #%d failed for remote address %s: want %s got %s",
				x, test.remoteAddr.IP, test.want1.IP, got.IP)
			continue
		}
	}

	// Add a public IP to the list of local addresses.
	localAddr := btcwire.NetAddress{IP: net.ParseIP("204.124.8.100")}
	amgr.AddLocalAddress(&localAddr, addrmgr.InterfacePrio)

	// Test against want2
	for x, test := range tests {
		got := amgr.GetBestLocalAddress(&test.remoteAddr)
		if !test.want2.IP.Equal(got.IP) {
			t.Errorf("TestGetBestLocalAddress test2 #%d failed for remote address %s: want %s got %s",
				x, test.remoteAddr.IP, test.want2.IP, got.IP)
			continue
		}
	}
	/*
		// Add a tor generated IP address
		localAddr = btcwire.NetAddress{IP: net.ParseIP("fd87:d87e:eb43:25::1")}
		amgr.AddLocalAddress(&localAddr, addrmgr.ManualPrio)

		// Test against want3
		for x, test := range tests {
			got := amgr.GetBestLocalAddress(&test.remoteAddr)
			if !test.want3.IP.Equal(got.IP) {
				t.Errorf("TestGetBestLocalAddress test3 #%d failed for remote address %s: want %s got %s",
					x, test.remoteAddr.IP, test.want3.IP, got.IP)
				continue
			}
		}
	*/
}

func TestNetAddressKey(t *testing.T) {
	addNaTests()

	t.Logf("Running %d tests", len(naTests))
	for i, test := range naTests {
		key := addrmgr.NetAddressKey(&test.in)
		if key != test.want {
			t.Errorf("NetAddressKey #%d\n got: %s want: %s", i, key, test.want)
			continue
		}
	}

}
