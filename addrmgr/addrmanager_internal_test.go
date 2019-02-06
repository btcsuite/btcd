package addrmgr

import (
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"testing"

	"github.com/btcsuite/btcd/wire"
)

// randAddr generates a *wire.NetAddress backed by a random IPv4/IPv6 address.
func randAddr(t *testing.T) *wire.NetAddress {
	t.Helper()

	ipv4 := rand.Intn(2) == 0
	var ip net.IP
	if ipv4 {
		var b [4]byte
		if _, err := rand.Read(b[:]); err != nil {
			t.Fatal(err)
		}
		ip = b[:]
	} else {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			t.Fatal(err)
		}
		ip = b[:]
	}

	return &wire.NetAddress{
		Services: wire.ServiceFlag(rand.Uint64()),
		IP:       ip,
		Port:     uint16(rand.Uint32()),
	}
}

// assertAddr ensures that the two addresses match. The timestamp is not
// checked as it does not affect uniquely identifying a specific address.
func assertAddr(t *testing.T, got, expected *wire.NetAddress) {
	if got.Services != expected.Services {
		t.Fatalf("expected address services %v, got %v",
			expected.Services, got.Services)
	}
	if !got.IP.Equal(expected.IP) {
		t.Fatalf("expected address IP %v, got %v", expected.IP, got.IP)
	}
	if got.Port != expected.Port {
		t.Fatalf("expected address port %d, got %d", expected.Port,
			got.Port)
	}
}

// assertAddrs ensures that the manager's address cache matches the given
// expected addresses.
func assertAddrs(t *testing.T, addrMgr *AddrManager,
	expectedAddrs map[string]*wire.NetAddress) {

	t.Helper()

	addrs := addrMgr.getAddresses()

	if len(addrs) != len(expectedAddrs) {
		t.Fatalf("expected to find %d addresses, found %d",
			len(expectedAddrs), len(addrs))
	}

	for _, addr := range addrs {
		addrStr := NetAddressKey(addr)
		expectedAddr, ok := expectedAddrs[addrStr]
		if !ok {
			t.Fatalf("expected to find address %v", addrStr)
		}

		assertAddr(t, addr, expectedAddr)
	}
}

// TestAddrManagerSerialization ensures that we can properly serialize and
// deserialize the manager's current address cache.
func TestAddrManagerSerialization(t *testing.T) {
	t.Parallel()

	// We'll start by creating our address manager backed by a temporary
	// directory.
	tempDir, err := ioutil.TempDir("", "addrmgr")
	if err != nil {
		t.Fatalf("unable to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	addrMgr := New(tempDir, nil)

	// We'll be adding 5 random addresses to the manager.
	const numAddrs = 5

	expectedAddrs := make(map[string]*wire.NetAddress, numAddrs)
	for i := 0; i < numAddrs; i++ {
		addr := randAddr(t)
		expectedAddrs[NetAddressKey(addr)] = addr
		addrMgr.AddAddress(addr, randAddr(t))
	}

	// Now that the addresses have been added, we should be able to retrieve
	// them.
	assertAddrs(t, addrMgr, expectedAddrs)

	// Then, we'll persist these addresses to disk and restart the address
	// manager.
	addrMgr.savePeers()
	addrMgr = New(tempDir, nil)

	// Finally, we'll read all of the addresses from disk and ensure they
	// match as expected.
	addrMgr.loadPeers()
	assertAddrs(t, addrMgr, expectedAddrs)
}
