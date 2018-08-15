// Copyright (c) 2013-2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package addrmgr_test

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/lbryio/lbcd/addrmgr"
	"github.com/lbryio/lbcd/wire"
)

// naTest is used to describe a test to be performed against the NetAddressKey
// method.
type naTest struct {
	in   wire.NetAddress
	want string
}

// naTests houses all of the tests to be performed against the NetAddressKey
// method.
var naTests = make([]naTest, 0)

// Put some IP in here for convenience. Points to google.
var someIP = "173.194.115.66"

// addNaTests
func addNaTests() {
	// IPv4
	// Localhost
	addNaTest("127.0.0.1", 9244, "127.0.0.1:9244")
	addNaTest("127.0.0.1", 9245, "127.0.0.1:9245")

	// Class A
	addNaTest("1.0.0.1", 9244, "1.0.0.1:9244")
	addNaTest("2.2.2.2", 9245, "2.2.2.2:9245")
	addNaTest("27.253.252.251", 9246, "27.253.252.251:9246")
	addNaTest("123.3.2.1", 9247, "123.3.2.1:9247")

	// Private Class A
	addNaTest("10.0.0.1", 9244, "10.0.0.1:9244")
	addNaTest("10.1.1.1", 9245, "10.1.1.1:9245")
	addNaTest("10.2.2.2", 9246, "10.2.2.2:9246")
	addNaTest("10.10.10.10", 9247, "10.10.10.10:9247")

	// Class B
	addNaTest("128.0.0.1", 9244, "128.0.0.1:9244")
	addNaTest("129.1.1.1", 9245, "129.1.1.1:9245")
	addNaTest("180.2.2.2", 9246, "180.2.2.2:9246")
	addNaTest("191.10.10.10", 9247, "191.10.10.10:9247")

	// Private Class B
	addNaTest("172.16.0.1", 9244, "172.16.0.1:9244")
	addNaTest("172.16.1.1", 9245, "172.16.1.1:9245")
	addNaTest("172.16.2.2", 9246, "172.16.2.2:9246")
	addNaTest("172.16.172.172", 9247, "172.16.172.172:9247")

	// Class C
	addNaTest("193.0.0.1", 9244, "193.0.0.1:9244")
	addNaTest("200.1.1.1", 9245, "200.1.1.1:9245")
	addNaTest("205.2.2.2", 9246, "205.2.2.2:9246")
	addNaTest("223.10.10.10", 9247, "223.10.10.10:9247")

	// Private Class C
	addNaTest("192.168.0.1", 9244, "192.168.0.1:9244")
	addNaTest("192.168.1.1", 9245, "192.168.1.1:9245")
	addNaTest("192.168.2.2", 9246, "192.168.2.2:9246")
	addNaTest("192.168.192.192", 9247, "192.168.192.192:9247")

	// IPv6
	// Localhost
	addNaTest("::1", 9244, "[::1]:9244")
	addNaTest("fe80::1", 9245, "[fe80::1]:9245")

	// Link-local
	addNaTest("fe80::1:1", 9244, "[fe80::1:1]:9244")
	addNaTest("fe91::2:2", 9245, "[fe91::2:2]:9245")
	addNaTest("fea2::3:3", 9246, "[fea2::3:3]:9246")
	addNaTest("feb3::4:4", 9247, "[feb3::4:4]:9247")

	// Site-local
	addNaTest("fec0::1:1", 9244, "[fec0::1:1]:9244")
	addNaTest("fed1::2:2", 9245, "[fed1::2:2]:9245")
	addNaTest("fee2::3:3", 9246, "[fee2::3:3]:9246")
	addNaTest("fef3::4:4", 9247, "[fef3::4:4]:9247")
}

func addNaTest(ip string, port uint16, want string) {
	nip := net.ParseIP(ip)
	na := *wire.NewNetAddressIPPort(nip, port, wire.SFNodeNetwork)
	test := naTest{na, want}
	naTests = append(naTests, test)
}

func lookupFunc(host string) ([]net.IP, error) {
	return nil, errors.New("not implemented")
}

func TestStartStop(t *testing.T) {
	n := addrmgr.New("teststartstop", lookupFunc)
	n.Start()
	err := n.Stop()
	if err != nil {
		t.Fatalf("Address Manager failed to stop: %v", err)
	}
}

func TestAddAddressByIP(t *testing.T) {
	fmtErr := fmt.Errorf("")
	addrErr := &net.AddrError{}
	var tests = []struct {
		addrIP string
		err    error
	}{
		{
			someIP + ":9244",
			nil,
		},
		{
			someIP,
			addrErr,
		},
		{
			someIP[:12] + ":9244",
			fmtErr,
		},
		{
			someIP + ":abcd",
			fmtErr,
		},
	}

	amgr := addrmgr.New("testaddressbyip", nil)
	for i, test := range tests {
		err := amgr.AddAddressByIP(test.addrIP)
		if test.err != nil && err == nil {
			t.Errorf("TestGood test %d failed expected an error and got none", i)
			continue
		}
		if test.err == nil && err != nil {
			t.Errorf("TestGood test %d failed expected no error and got one", i)
			continue
		}
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("TestGood test %d failed got %v, want %v", i,
				reflect.TypeOf(err), reflect.TypeOf(test.err))
			continue
		}
	}
}

func TestAddLocalAddress(t *testing.T) {
	var tests = []struct {
		address  wire.NetAddress
		priority addrmgr.AddressPriority
		valid    bool
	}{
		{
			wire.NetAddress{IP: net.ParseIP("192.168.0.100")},
			addrmgr.InterfacePrio,
			false,
		},
		{
			wire.NetAddress{IP: net.ParseIP("204.124.1.1")},
			addrmgr.InterfacePrio,
			true,
		},
		{
			wire.NetAddress{IP: net.ParseIP("204.124.1.1")},
			addrmgr.BoundPrio,
			true,
		},
		{
			wire.NetAddress{IP: net.ParseIP("::1")},
			addrmgr.InterfacePrio,
			false,
		},
		{
			wire.NetAddress{IP: net.ParseIP("fe80::1")},
			addrmgr.InterfacePrio,
			false,
		},
		{
			wire.NetAddress{IP: net.ParseIP("2620:100::1")},
			addrmgr.InterfacePrio,
			true,
		},
	}
	amgr := addrmgr.New("testaddlocaladdress", nil)
	for x, test := range tests {
		result := amgr.AddLocalAddress(&test.address, test.priority)
		if result == nil && !test.valid {
			t.Errorf("TestAddLocalAddress test #%d failed: %s should have "+
				"been accepted", x, test.address.IP)
			continue
		}
		if result != nil && test.valid {
			t.Errorf("TestAddLocalAddress test #%d failed: %s should not have "+
				"been accepted", x, test.address.IP)
			continue
		}
	}
}

func TestAttempt(t *testing.T) {
	n := addrmgr.New("testattempt", lookupFunc)

	// Add a new address and get it
	err := n.AddAddressByIP(someIP + ":9244")
	if err != nil {
		t.Fatalf("Adding address failed: %v", err)
	}
	ka := n.GetAddress()

	if !ka.LastAttempt().IsZero() {
		t.Errorf("Address should not have attempts, but does")
	}

	na := ka.NetAddress()
	n.Attempt(na)

	if ka.LastAttempt().IsZero() {
		t.Errorf("Address should have an attempt, but does not")
	}
}

func TestConnected(t *testing.T) {
	n := addrmgr.New("testconnected", lookupFunc)

	// Add a new address and get it
	err := n.AddAddressByIP(someIP + ":9244")
	if err != nil {
		t.Fatalf("Adding address failed: %v", err)
	}
	ka := n.GetAddress()
	na := ka.NetAddress()
	// make it an hour ago
	na.Timestamp = time.Unix(time.Now().Add(time.Hour*-1).Unix(), 0)

	n.Connected(na)

	if !ka.NetAddress().Timestamp.After(na.Timestamp) {
		t.Errorf("Address should have a new timestamp, but does not")
	}
}

func TestNeedMoreAddresses(t *testing.T) {
	n := addrmgr.New("testneedmoreaddresses", lookupFunc)
	addrsToAdd := 1500
	b := n.NeedMoreAddresses()
	if !b {
		t.Errorf("Expected that we need more addresses")
	}
	addrs := make([]*wire.NetAddress, addrsToAdd)

	var err error
	for i := 0; i < addrsToAdd; i++ {
		s := fmt.Sprintf("%d.%d.173.147:9244", i/128+60, i%128+60)
		addrs[i], err = n.DeserializeNetAddress(s, wire.SFNodeNetwork)
		if err != nil {
			t.Errorf("Failed to turn %s into an address: %v", s, err)
		}
	}

	srcAddr := wire.NewNetAddressIPPort(net.IPv4(173, 144, 173, 111), 9244, 0)

	n.AddAddresses(addrs, srcAddr)
	numAddrs := n.NumAddresses()
	if numAddrs > addrsToAdd {
		t.Errorf("Number of addresses is too many %d vs %d", numAddrs, addrsToAdd)
	}

	b = n.NeedMoreAddresses()
	if b {
		t.Errorf("Expected that we don't need more addresses")
	}
}

func TestGood(t *testing.T) {
	n := addrmgr.New("testgood", lookupFunc)
	addrsToAdd := 64 * 64
	addrs := make([]*wire.NetAddress, addrsToAdd)

	var err error
	for i := 0; i < addrsToAdd; i++ {
		s := fmt.Sprintf("%d.173.147.%d:9244", i/64+60, i%64+60)
		addrs[i], err = n.DeserializeNetAddress(s, wire.SFNodeNetwork)
		if err != nil {
			t.Errorf("Failed to turn %s into an address: %v", s, err)
		}
	}

	srcAddr := wire.NewNetAddressIPPort(net.IPv4(173, 144, 173, 111), 9244, 0)

	n.AddAddresses(addrs, srcAddr)
	for _, addr := range addrs {
		n.Good(addr)
	}

	numAddrs := n.NumAddresses()
	if numAddrs >= addrsToAdd {
		t.Errorf("Number of addresses is too many: %d vs %d", numAddrs, addrsToAdd)
	}

	numCache := len(n.AddressCache())
	if numCache >= numAddrs/4 {
		t.Errorf("Number of addresses in cache: got %d, want %d", numCache, numAddrs/4)
	}
}

func TestGetAddress(t *testing.T) {
	n := addrmgr.New("testgetaddress", lookupFunc)

	// Get an address from an empty set (should error)
	if rv := n.GetAddress(); rv != nil {
		t.Errorf("GetAddress failed: got: %v want: %v\n", rv, nil)
	}

	// Add a new address and get it
	err := n.AddAddressByIP(someIP + ":9244")
	if err != nil {
		t.Fatalf("Adding address failed: %v", err)
	}
	ka := n.GetAddress()
	if ka == nil {
		t.Fatalf("Did not get an address where there is one in the pool")
	}
	if ka.NetAddress().IP.String() != someIP {
		t.Errorf("Wrong IP: got %v, want %v", ka.NetAddress().IP.String(), someIP)
	}

	// Mark this as a good address and get it
	n.Good(ka.NetAddress())
	ka = n.GetAddress()
	if ka == nil {
		t.Fatalf("Did not get an address where there is one in the pool")
	}
	if ka.NetAddress().IP.String() != someIP {
		t.Errorf("Wrong IP: got %v, want %v", ka.NetAddress().IP.String(), someIP)
	}

	numAddrs := n.NumAddresses()
	if numAddrs != 1 {
		t.Errorf("Wrong number of addresses: got %d, want %d", numAddrs, 1)
	}
}

func TestGetBestLocalAddress(t *testing.T) {
	localAddrs := []wire.NetAddress{
		{IP: net.ParseIP("192.168.0.100")},
		{IP: net.ParseIP("::1")},
		{IP: net.ParseIP("fe80::1")},
		{IP: net.ParseIP("2001:470::1")},
	}

	var tests = []struct {
		remoteAddr wire.NetAddress
		want0      wire.NetAddress
		want1      wire.NetAddress
		want2      wire.NetAddress
		want3      wire.NetAddress
	}{
		{
			// Remote connection from public IPv4
			wire.NetAddress{IP: net.ParseIP("204.124.8.1")},
			wire.NetAddress{IP: net.IPv4zero},
			wire.NetAddress{IP: net.IPv4zero},
			wire.NetAddress{IP: net.ParseIP("204.124.8.100")},
			wire.NetAddress{IP: net.ParseIP("fd87:d87e:eb43:25::1")},
		},
		{
			// Remote connection from private IPv4
			wire.NetAddress{IP: net.ParseIP("172.16.0.254")},
			wire.NetAddress{IP: net.IPv4zero},
			wire.NetAddress{IP: net.IPv4zero},
			wire.NetAddress{IP: net.IPv4zero},
			wire.NetAddress{IP: net.IPv4zero},
		},
		{
			// Remote connection from public IPv6
			wire.NetAddress{IP: net.ParseIP("2602:100:abcd::102")},
			wire.NetAddress{IP: net.IPv6zero},
			wire.NetAddress{IP: net.ParseIP("2001:470::1")},
			wire.NetAddress{IP: net.ParseIP("2001:470::1")},
			wire.NetAddress{IP: net.ParseIP("2001:470::1")},
		},
		/* XXX
		{
			// Remote connection from Tor
			wire.NetAddress{IP: net.ParseIP("fd87:d87e:eb43::100")},
			wire.NetAddress{IP: net.IPv4zero},
			wire.NetAddress{IP: net.ParseIP("204.124.8.100")},
			wire.NetAddress{IP: net.ParseIP("fd87:d87e:eb43:25::1")},
		},
		*/
	}

	amgr := addrmgr.New("testgetbestlocaladdress", nil)

	// Test against default when there's no address
	for x, test := range tests {
		got := amgr.GetBestLocalAddress(&test.remoteAddr)
		if !test.want0.IP.Equal(got.IP) {
			t.Errorf("TestGetBestLocalAddress test1 #%d failed for remote address %s: want %s got %s",
				x, test.remoteAddr.IP, test.want1.IP, got.IP)
			continue
		}
	}

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
	localAddr := wire.NetAddress{IP: net.ParseIP("204.124.8.100")}
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
		// Add a Tor generated IP address
		localAddr = wire.NetAddress{IP: net.ParseIP("fd87:d87e:eb43:25::1")}
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
