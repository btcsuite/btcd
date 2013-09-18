// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/list"
	"encoding/json"
	"github.com/conformal/btcwire"
	"math"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	// maxAddresses identifies the maximum number of addresses that the
	// address manager will track.
	maxAddresses = 2500

	// needAddressThreshold is the number of addresses under which the
	// address manager will claim to need more addresses.
	needAddressThreshold = 1000

	newAddressBufferSize = 50

	// dumpAddressInterval is the interval used to dump the address
	// cache to disk for future use.
	dumpAddressInterval = time.Minute * 2

	// triedBucketSize is the maximum number of addresses in each
	// tried address bucket.
	triedBucketSize = 64

	// newBucketSize is the maximum number of addresses in each new address
	// bucket.
	newBucketSize = 64

	// numMissingDays is the number of days before which we assume an
	// address has vanished if we have not seen it announced  in that long.
	numMissingDays = 30

	// numRetries is the number of tried without a single success before
	// we assume an address is bad.
	numRetries = 3

	// maxFailures is the maximum number of failures we will accept without
	// a success before considering an address bad.
	maxFailures = 10

	// minBadDays is the number of days since the last success before we
	// will consider evicting an address.
	minBadDays = 7
)

// updateAddress is a helper function to either update an address already known
// to the address manager, or to add the address if not already known.
func (a *AddrManager) updateAddress(netAddr, srcAddr *btcwire.NetAddress) {
	// Protect concurrent access.
	a.mtx.Lock()
	defer a.mtx.Unlock()

	ka := a.find(netAddr)
	if ka != nil {
		// Update the last seen time.
		if netAddr.Timestamp.After(ka.na.Timestamp) {
			ka.na.Timestamp = netAddr.Timestamp
		}

		// Update services.
		ka.na.AddService(netAddr.Services)

		log.Tracef("[AMGR] Updated address manager address %s",
			NetAddressKey(netAddr))
		return
	}

	// Enforce max addresses.
	if len(a.addrNew) > newBucketSize {
		log.Tracef("[AMGR] new bucket is full, expiring old ")
		a.expireNew()
	}

	addr := NetAddressKey(netAddr)
	ka = &knownAddress{na: netAddr}

	// Fill in index.
	a.addrIndex[addr] = ka

	// Add to new bucket.
	a.addrNew[addr] = ka

	log.Tracef("[AMGR] Added new address %s for a total of %d addresses",
		addr, len(a.addrNew)+a.addrTried.Len())
}

// bad returns true if the address in question has not been tried in the last
// minute and meets one of the following criteria:
// 1) It claims to be from the future
// 2) It hasn't been seen in over a month
// 3) It has failed at least three times and never succeeded
// 4) It has failed ten times in the last week
// All addresses that meet these criteria are assumed to be worthless and not
// worth keeping hold of.
func bad(ka *knownAddress) bool {
	if ka.lastattempt.After(time.Now().Add(-1 * time.Minute)) {
		return false

	}

	// From the future?
	if ka.na.Timestamp.After(time.Now().Add(10 * time.Minute)) {
		return true
	}

	// Over a month old?
	if ka.na.Timestamp.After(time.Now().Add(-1 * numMissingDays * time.Hour * 24)) {
		return true
	}

	// Never succeeded?
	if ka.lastsuccess.IsZero() && ka.attempts >= numRetries {
		return true
	}

	// Hasn't succeeded in too long?
	if !ka.lastsuccess.After(time.Now().Add(-1*minBadDays*time.Hour*24)) &&
		ka.attempts >= maxFailures {
		return true
	}

	return false
}

// chance returns the selection probability for a known address.  The priority
// depends upon how recent the address has been seen, how recent it was last
// attempted and how often attempts to connect to it have failed.
func chance(ka *knownAddress) float64 {
	c := 1.0

	now := time.Now()
	var lastSeen float64
	var lastTry float64
	if !ka.na.Timestamp.After(now) {
		var dur time.Duration
		if ka.na.Timestamp.IsZero() {
			// use unix epoch to match bitcoind.
			dur = now.Sub(time.Unix(0, 0))

		} else {
			dur = now.Sub(ka.na.Timestamp)
		}
		lastSeen = dur.Seconds()
	}
	if !ka.lastattempt.After(now) {
		var dur time.Duration
		if ka.lastattempt.IsZero() {
			// use unix epoch to match bitcoind.
			dur = now.Sub(time.Unix(0, 0))
		} else {
			dur = now.Sub(ka.lastattempt)
		}
		lastTry = dur.Seconds()
	}

	c = 600.0 / (600.0 + lastSeen)

	// Very recent attempts are less likely to be retried.
	if lastTry > 60.0*10.0 {
		c *= 0.01
	}

	// Failed attempts deprioritise.
	if ka.attempts > 0 {
		c /= (float64(ka.attempts) * 1.5)
	}

	return c
}

// expireNew makes space in the new buckets by expiring the really bad entries.
// If no bad entries are available we look at a few and remove the oldest.
func (a *AddrManager) expireNew() {
	// First see if there are any entries that are so bad we can just throw
	// them away. otherwise we throw away the oldest entry in the cache.
	// Bitcoind here chooses four random and just throws the oldest of
	// those away, but we keep track of oldest in the initial traversal and
	// use that information instead
	var oldest *knownAddress
	for k, v := range a.addrNew {
		if bad(v) {
			log.Tracef("[AMGR] expiring bad address %v", k)
			delete(a.addrIndex, k)
			delete(a.addrNew, k)
			return
		}
		if oldest == nil {
			oldest = v
		} else if !v.na.Timestamp.After(oldest.na.Timestamp) {
			oldest = v
		}
	}

	if oldest != nil {
		key := NetAddressKey(oldest.na)
		log.Tracef("[AMGR] expiring oldest address %v", key)

		delete(a.addrIndex, key)
		delete(a.addrNew, key)
	}
}

// pickTried selects an address from the tried bucket to be evicted.
// We just choose the eldest.
func (a *AddrManager) pickTried() *list.Element {
	var oldest *knownAddress
	var oldestElem *list.Element
	for e := a.addrTried.Front(); e != nil; e = e.Next() {
		ka := e.Value.(*knownAddress)
		if oldest == nil || oldest.na.Timestamp.After(ka.na.Timestamp) {
			oldestElem = e
			oldest = ka
		}

	}
	return oldestElem
}

// knownAddress tracks information about a known network address that is used
// to determine how viable an address is.
type knownAddress struct {
	na          *btcwire.NetAddress
	attempts    int
	lastattempt time.Time
	lastsuccess time.Time
	time        time.Time
	tried       bool
}

// AddrManager provides a concurrency safe address manager for caching potential
// peers on the bitcoin network.
type AddrManager struct {
	mtx       sync.Mutex
	rand      *rand.Rand
	addrIndex map[string]*knownAddress // address key to ka for all addrs.
	addrNew   map[string]*knownAddress
	addrTried *list.List
	started   bool
	shutdown  bool
	wg        sync.WaitGroup
	quit      chan bool
}

type JsonSave struct {
	AddrList []string
}

// addressHandler is the main handler for the address manager.  It must be run
// as a goroutine.
func (a *AddrManager) addressHandler() {
	dumpAddressTicker := time.NewTicker(dumpAddressInterval)
out:
	for !a.shutdown {
		select {
		case <-dumpAddressTicker.C:
			if !a.shutdown {
				a.savePeers()
			}

		case <-a.quit:
			a.savePeers()
			break out
		}
	}
	dumpAddressTicker.Stop()
	a.wg.Done()
	log.Trace("[AMGR] Address handler done")
}

// savePeers saves all the known addresses to a file so they can be read back
// in at next run.
func (a *AddrManager) savePeers() {
	// May give some way to specify this later.
	filename := "peers.json"
	filePath := filepath.Join(cfg.DataDir, filename)

	var toSave JsonSave

	list := a.AddressCacheFlat()
	toSave.AddrList = list

	w, err := os.Create(filePath)
	if err != nil {
		log.Error("[AMGR] Error opening file: ", filePath, err)
	}
	enc := json.NewEncoder(w)
	defer w.Close()
	enc.Encode(&toSave)
}

// loadPeers loads the known address from the saved file.  If empty, missing, or
// malformed file, just don't load anything and start fresh
func (a *AddrManager) loadPeers() {
	// May give some way to specify this later.
	filename := "peers.json"
	filePath := filepath.Join(cfg.DataDir, filename)

	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		log.Debugf("[AMGR] %s does not exist.", filePath)
	} else {
		r, err := os.Open(filePath)
		if err != nil {
			log.Error("[AMGR] Error opening file: ", filePath, err)
			return
		}
		defer r.Close()

		var inList JsonSave
		dec := json.NewDecoder(r)
		err = dec.Decode(&inList)
		if err != nil {
			log.Error("[AMGR] Error reading:", filePath, err)
			return
		}
		log.Debug("[AMGR] Adding ", len(inList.AddrList), " saved peers.")
		if len(inList.AddrList) > 0 {
			for _, ip := range inList.AddrList {
				a.AddAddressByIP(ip)
			}
		}
	}
}

// Start begins the core address handler which manages a pool of known
// addresses, timeouts, and interval based writes.
func (a *AddrManager) Start() {
	// Already started?
	if a.started {
		return
	}

	log.Trace("[AMGR] Starting address manager")

	a.wg.Add(1)
	go a.addressHandler()
	a.started = true

	// Load peers we already know about from file.
	a.loadPeers()
}

// Stop gracefully shuts down the address manager by stopping the main handler.
func (a *AddrManager) Stop() error {
	if a.shutdown {
		log.Warnf("[AMGR] Address manager is already in the process of " +
			"shutting down")
		return nil
	}

	log.Infof("[AMGR] Address manager shutting down")
	a.savePeers()
	a.shutdown = true
	a.quit <- true
	a.wg.Wait()
	return nil
}

// AddAddresses adds new addresses to the address manager.  It enforces a max
// number of addresses and silently ignores duplicate addresses.  It is
// safe for concurrent access.
func (a *AddrManager) AddAddresses(addrs []*btcwire.NetAddress,
	srcAddr *btcwire.NetAddress) {
	for _, na := range addrs {
		// Filter out non-routable addresses. Note that non-routable
		// also includes invalid and local addresses.
		if Routable(na) {
			a.updateAddress(na, srcAddr)
		}
	}
}

// AddAddress adds a new address to the address manager.  It enforces a max
// number of addresses and silently ignores duplicate addresses.  It is
// safe for concurrent access.
func (a *AddrManager) AddAddress(addr *btcwire.NetAddress,
	srcAddr *btcwire.NetAddress) {
	a.AddAddresses([]*btcwire.NetAddress{addr}, srcAddr)
}

// AddAddressByIP adds an address where we are given an ip:port and not a
// btcwire.NetAddress.
func (a *AddrManager) AddAddressByIP(addrIP string) {
	// Split IP and port
	addr, portStr, err := net.SplitHostPort(addrIP)
	if err != nil {
		log.Warnf("[AMGR] AddADddressByIP given bullshit adddress"+
			"(%s): %v", err)
		return
	}
	// Put it in btcwire.Netaddress
	var na btcwire.NetAddress
	na.Timestamp = time.Now()
	na.IP = net.ParseIP(addr)
	if na.IP == nil {
		log.Error("[AMGR] Invalid ip address:", addr)
		return
	}
	port, err := strconv.ParseUint(portStr, 10, 0)
	if err != nil {
		log.Error("[AMGR] Invalid port: ", portStr, err)
		return
	}
	na.Port = uint16(port)
	a.AddAddress(&na, &na) // XXX use correct src address
}

// NeedMoreAddresses returns whether or not the address manager needs more
// addresses.
func (a *AddrManager) NeedMoreAddresses() bool {
	// NumAddresses handles concurrent access for us.

	return a.NumAddresses() < needAddressThreshold
}

// NumAddresses returns the number of addresses known to the address manager.
func (a *AddrManager) NumAddresses() int {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	return len(a.addrNew) + a.addrTried.Len()
}

// AddressCache returns the current address cache.  It must be treated as
// read-only (but since it is a copy now, this is not as dangerous).
func (a *AddrManager) AddressCache() map[string]*btcwire.NetAddress {
	allAddr := make(map[string]*btcwire.NetAddress)

	a.mtx.Lock()
	defer a.mtx.Unlock()
	for k, v := range a.addrNew {
		allAddr[k] = v.na
	}

	for e := a.addrTried.Front(); e != nil; e = e.Next() {
		ka := e.Value.(*knownAddress)
		allAddr[NetAddressKey(ka.na)] = ka.na
	}

	return allAddr
}

// AddressCacheFlat returns a flat list of strings with the current address
// cache.  Just a copy, so one can do whatever they want to it.
func (a *AddrManager) AddressCacheFlat() []string {
	var allAddr []string

	a.mtx.Lock()
	defer a.mtx.Unlock()
	for k := range a.addrNew {
		allAddr = append(allAddr, k)
	}

	for e := a.addrTried.Front(); e != nil; e = e.Next() {
		ka := e.Value.(*knownAddress)
		allAddr = append(allAddr, NetAddressKey(ka.na))
	}

	return allAddr
}

// New returns a new bitcoin address manager.
// Use Start to begin processing asynchronous address updates.
func NewAddrManager() *AddrManager {
	am := AddrManager{
		rand:      rand.New(rand.NewSource(time.Now().UnixNano())),
		addrIndex: make(map[string]*knownAddress),
		addrNew:   make(map[string]*knownAddress),
		addrTried: list.New(),
		quit:      make(chan bool),
	}
	return &am
}

// NetAddressKey returns a string key in the form of ip:port for IPv4 addresses
// or [ip]:port for IPv6 addresses.
func NetAddressKey(na *btcwire.NetAddress) string {
	port := strconv.FormatUint(uint64(na.Port), 10)
	addr := net.JoinHostPort(na.IP.String(), port)
	return addr
}

// GetAddress returns a single address that should be routable.  It picks a
// random one from the possible addresses with preference given to ones that
// have not been used recently and should not pick 'close' addresses
// consecutively.
func (a *AddrManager) GetAddress(class string, newBias int) *knownAddress {
	// Protect concurrent access.
	a.mtx.Lock()
	defer a.mtx.Unlock()

	if newBias > 100 {
		newBias = 100
	}
	if newBias < 0 {
		newBias = 0
	}

	// Bias 50% for now between new and tried.
	triedCorrelation := math.Sqrt(float64(a.addrTried.Len())) *
		(100.0 - float64(newBias))
	newCorrelation := math.Sqrt(float64(len(a.addrNew))) * float64(newBias)

	if (newCorrelation+triedCorrelation)*a.rand.Float64() <
		triedCorrelation {
		// Tried entry.
		large := 1 << 30
		factor := 1.0
		for {
			// Pick a random entry in the list
			e := a.addrTried.Front()
			for i := a.rand.Int63n(int64(a.addrTried.Len())); i > 0; i-- {
				e = e.Next()
			}
			ka := e.Value.(*knownAddress)
			randval := a.rand.Intn(large)
			if float64(randval) < (factor * chance(ka) * float64(large)) {
				log.Tracef("[AMGR] Selected %v from tried "+
					"bucket", NetAddressKey(ka.na))
				return ka
			}
			factor *= 1.2
		}
	} else {
		// new node.
		// XXX use a closure/function to avoid repeating this.
		keyList := []string{}
		for key := range a.addrNew {
			keyList = append(keyList, key)
		}
		large := 1 << 30
		factor := 1.0
		for {
			testKey := keyList[a.rand.Int63n(int64(len(keyList)))]
			ka := a.addrNew[testKey]
			randval := a.rand.Intn(large)
			if float64(randval) < (factor * chance(ka) * float64(large)) {
				log.Tracef("[AMGR] Selected %v from new bucket",
					NetAddressKey(ka.na))
				return ka
			}
			factor *= 1.2
		}
	}
}

func (a *AddrManager) find(addr *btcwire.NetAddress) *knownAddress {
	return a.addrIndex[NetAddressKey(addr)]
}

/*
 * Connected - updates the last seen time but only every 20 minutes.
 * Good - last tried = last success = last seen = now. attmempts = 0.
 *      - move address to tried.
 * Attempted - set last tried to time. nattempts++
 */
func (a *AddrManager) Attempt(addr *btcwire.NetAddress) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	// find address.
	// Surely address will be in tried by now?
	ka := a.find(addr)
	if ka == nil {
		return
	}
	// set last tried time to now
	ka.attempts++
	ka.lastattempt = time.Now()
}

// Connected Marks the given address as currently connected and working at the
// current time.  The address must already be known to AddrManager else it will
// be ignored.
func (a *AddrManager) Connected(addr *btcwire.NetAddress) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	ka := a.find(addr)
	if ka == nil {
		return
	}

	// Update the time as long as it has been 20 minutes since last we did
	// so.
	now := time.Now()
	if now.After(ka.na.Timestamp.Add(time.Minute * 20)) {
		ka.na.Timestamp = time.Now()
	}
}

// Good marks the given address as good.  To be called after a successful
// connection and version exchange.  If the address is unknown to the addresss
// manager it will be ignored.
func (a *AddrManager) Good(addr *btcwire.NetAddress) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	ka := a.find(addr)
	if ka == nil {
		return
	}
	now := time.Now()
	ka.lastsuccess = now
	ka.lastattempt = now
	ka.na.Timestamp = now
	ka.attempts = 0

	// move to tried set, optionally evicting other addresses if neeed.
	if ka.tried {
		return
	}

	// ok, need to move it to tried.

	// remove from new buckets.
	addrKey := NetAddressKey(addr)
	delete(a.addrNew, addrKey)

	// is tried full? or is it ok?
	if a.addrTried.Len() < triedBucketSize {
		a.addrTried.PushBack(ka)
		return
	}

	// No room, we have to evict something else.

	// pick another one to throw out
	entry := a.pickTried()
	rmka := entry.Value.(*knownAddress)

	rmkey := NetAddressKey(rmka.na)

	// replace with ka.
	entry.Value = ka

	rmka.tried = false

	log.Tracef("[AMGR] replacing %s with %s in tried", rmkey, addrKey)

	// We know there is space for it since we just moved out of new.
	// TODO(oga) when we move to multiple buckets then we will need to
	// check for size and consider putting it elsewhere.
	a.addrNew[rmkey] = rmka
}

// RFC1918: IPv4 Private networks (10.0.0.0/8, 192.168.0.0/16, 172.16.0.0/12)
var rfc1918ten = net.IPNet{IP: net.ParseIP("10.0.0.0"),
					Mask: net.CIDRMask(8, 32)}
var rfc1918oneninetwo = net.IPNet{IP: net.ParseIP("192.168.0.0"),
					Mask: net.CIDRMask(16, 32)}
var rfc1918oneseventwo = net.IPNet{IP: net.ParseIP("172.16.0.0"),
	Mask: net.CIDRMask(12, 32)}

func RFC1918(na *btcwire.NetAddress) bool {
	return rfc1918ten.Contains(na.IP) ||
		rfc1918oneninetwo.Contains(na.IP) ||
		rfc1918oneseventwo.Contains(na.IP)
}

// RFC3849 IPv6 Documentation address  (2001:0DB8::/32)
var rfc3849 = net.IPNet{IP: net.ParseIP("2001:0DB8::"),
	Mask: net.CIDRMask(32, 128)}

func RFC3849(na *btcwire.NetAddress) bool {
	return rfc3849.Contains(na.IP)
}

// RFC3927 IPv4 Autoconfig (169.254.0.0/16)
var rfc3927 = net.IPNet{IP: net.ParseIP("169.254.0.0"), Mask: net.CIDRMask(16, 32)}

func RFC3927(na *btcwire.NetAddress) bool {
	return rfc3927.Contains(na.IP)
}

// RFC3964 IPv6 6to4 (2002::/16)
var rfc3964 = net.IPNet{IP: net.ParseIP("2002::"),
	Mask: net.CIDRMask(16, 128)}

func RFC3964(na *btcwire.NetAddress) bool {
	return rfc3964.Contains(na.IP)
}

// RFC4193 IPv6 unique local (FC00::/15)
var rfc4193 = net.IPNet{IP: net.ParseIP("FC00::"),
	Mask: net.CIDRMask(15, 128)}

func RFC4193(na *btcwire.NetAddress) bool {
	return rfc4193.Contains(na.IP)
}

// RFC4380 IPv6 Teredo tunneling (2001::/32)
var rfc4380 = net.IPNet{IP: net.ParseIP("2001::"),
	Mask: net.CIDRMask(32, 128)}

func RFC4380(na *btcwire.NetAddress) bool {
	return rfc4380.Contains(na.IP)
}

// RFC4843 IPv6 ORCHID: (2001:10::/28)
var rfc4843 = net.IPNet{IP: net.ParseIP("2001;10::"),
	Mask: net.CIDRMask(28, 128)}

func RFC4843(na *btcwire.NetAddress) bool {
	return rfc4843.Contains(na.IP)
}

// RFC4862 IPv6 Autoconfig (FE80::/64)
var rfc4862 = net.IPNet{IP: net.ParseIP("FE80::"),
	Mask: net.CIDRMask(64, 128)}

func RFC4862(na *btcwire.NetAddress) bool {
	return rfc4862.Contains(na.IP)
}

// RFC6052: IPv6 well known prefix (64:FF9B::/96)
var rfc6052 = net.IPNet{IP: net.ParseIP("64::FF9B::"),
	Mask: net.CIDRMask(96, 128)}

func RFC6052(na *btcwire.NetAddress) bool {
	return rfc6052.Contains(na.IP)
}

// RFC6145: IPv6 IPv4 translated address ::FFFF:0:0:0/96
var rfc6145 = net.IPNet{IP: net.ParseIP("::FFFF:0:0:0"),
	Mask: net.CIDRMask(96, 128)}

func RFC6145(na *btcwire.NetAddress) bool {
	return rfc6145.Contains(na.IP)
}

func Tor(na *btcwire.NetAddress) bool {
	// bitcoind encodes a .onion address as a 16 byte number by decoding the
	// address prior to the .onion (i.e. the key hash) base32 into a ten
	// byte number. it then stores the first 6 bytes of the address as
	// 0xfD, 0x87, 0xD8, 0x7e, 0xeb, 0x43
	// making the format
	// { magic 6 bytes, 10 bytes base32 decode of key hash }
	// Since we use btcwire.NetAddress to represent and address we may
	// well have to emulate this.
	// XXX fillmein
	return false
}

var zero4 = net.IPNet{IP: net.ParseIP("0.0.0.0"),
	Mask: net.CIDRMask(8, 32)}

func Local(na *btcwire.NetAddress) bool {
	return na.IP.IsLoopback() || zero4.Contains(na.IP)
}

// Valid returns true if an address is not one of the invalid formats.
// For IPv4 these are either a 0 or all bits set address. For IPv6 a zero
// address or one that matches the RFC3849 documentation address format.
func Valid(na *btcwire.NetAddress) bool {
	// IsUnspecified returns if address is 0, so only all bits set, and
	// RFC3849 need to be explicitly checked. bitcoind here also checks for
	// invalid protocol addresses from earlier versions of bitcoind (before
	// 0.2.9), however, since protocol versions before 70001 are
	// disconnected by the bitcoin network now we have elided it.
	return !(na.IP.IsUnspecified() || RFC3849(na) ||
		na.IP.Equal(net.IPv4bcast))
}

// Routable returns whether a netaddress is routable on the public internet or
// not. This is true as long as the address is valid and is not in any reserved
// ranges.
func Routable(na *btcwire.NetAddress) bool {
	return Valid(na) && !(RFC1918(na) || RFC3927(na) || RFC4862(na) ||
		RFC4193(na) || Tor(na) || RFC4843(na) || Local(na))
}

// GroupKey returns a string representing the network group an address
// is part of.
// This is the /16 for IPv6, the /32 (/36 for he.net) for IPv6, the string
// "local" for a local address and the string "unroutable for an unroutable
// address.
func GroupKey(na *btcwire.NetAddress) string {
	if Local(na) {
		return "local"
	}
	if !Routable(na) {
		return "unroutable"
	}

	if ipv4 := na.IP.To4(); ipv4 != nil {
		return (&net.IPNet{IP: na.IP, Mask: net.CIDRMask(16, 32)}).String()
	}
	if RFC6145(na) || RFC6052(na) {
		// last four bytes are the ip address
		ip := net.IP(na.IP[12:16])
		return (&net.IPNet{IP: ip, Mask: net.CIDRMask(16, 32)}).String()
	}

	if RFC3964(na) {
		ip := net.IP(na.IP[2:7])
		return (&net.IPNet{IP: ip, Mask: net.CIDRMask(16, 32)}).String()

	}
	if RFC4380(na) {
		// teredo tunnels have the last 4 bytes as the v4 address XOR
		// 0xff.
		ip := net.IP(make([]byte, 4))
		for i, byte := range na.IP[12:16] {
			ip[i] = byte ^ 0xff
		}
		return (&net.IPNet{IP: ip, Mask: net.CIDRMask(16, 32)}).String()
	}
	// XXX tor?
	if Tor(na) {
		panic("oga should have implemented me")
	}

	// OK, so now we know ourselves to be a IPv6 address.
	// bitcoind uses /32 for everything but what it calls he.net, which is
	// it uses /36 for. he.net is actualy 2001:470::/32, whereas bitcoind
	// counts it as 2011:470::/32.

	bits := 32
	heNet := &net.IPNet{IP: net.ParseIP("2011:470::"),
		Mask: net.CIDRMask(32, 128)}
	if heNet.Contains(na.IP) {
		bits = 36
	}

	return (&net.IPNet{IP: na.IP, Mask: net.CIDRMask(bits, 128)}).String()
}
