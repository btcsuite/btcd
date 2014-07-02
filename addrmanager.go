// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"container/list"
	crand "crypto/rand" // for seeding
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/conformal/btcwire"
	"io"
	"math"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
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

	// triedBucketCount is the number of buckets we split tried
	// addresses over.
	triedBucketCount = 64

	// newBucketSize is the maximum number of addresses in each new address
	// bucket.
	newBucketSize = 64

	// newBucketCount is the number of buckets taht we spread new addresses
	// over.
	newBucketCount = 256

	// triedBucketsPerGroup is the number of trieed buckets over which an
	// address group will be spread.
	triedBucketsPerGroup = 4

	// newBucketsPerGroup is the number of new buckets over which an
	// source address group will be spread.
	newBucketsPerGroup = 32

	// newBucketsPerAddress is the number of buckets a frequently seen new
	// address may end up in.
	newBucketsPerAddress = 4

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

	// getAddrMax is the most addresses that we will send in response
	// to a getAddr (in practise the most addresses we will return from a
	// call to AddressCache()).
	getAddrMax = 2500

	// getAddrPercent is the percentage of total addresses known that we
	// will share with a call to AddressCache.
	getAddrPercent = 23

	// serialisationVersion is the current version of the on-disk format.
	serialisationVersion = 1
)

// updateAddress is a helper function to either update an address already known
// to the address manager, or to add the address if not already known.
func (a *AddrManager) updateAddress(netAddr, srcAddr *btcwire.NetAddress) {
	// Filter out non-routable addresses. Note that non-routable
	// also includes invalid and local addresses.
	if !Routable(netAddr) {
		return
	}

	// Protect concurrent access.
	a.mtx.Lock()
	defer a.mtx.Unlock()

	addr := NetAddressKey(netAddr)
	ka := a.find(netAddr)
	if ka != nil {
		// TODO(oga) only update adresses periodically.
		// Update the last seen time and services.
		// note that to prevent causing excess garbage on getaddr
		// messages the netaddresses in addrmaanger are *immutable*,
		// if we need to change them then we replace the pointer with a
		// new copy so that we don't have to copy every na for getaddr.
		if netAddr.Timestamp.After(ka.na.Timestamp) ||
			(ka.na.Services&netAddr.Services) !=
				netAddr.Services {

			naCopy := *ka.na
			naCopy.Timestamp = netAddr.Timestamp
			naCopy.AddService(netAddr.Services)
			ka.na = &naCopy
		}

		// If already in tried, we have nothing to do here.
		if ka.tried {
			return
		}

		// Already at our max?
		if ka.refs == newBucketsPerAddress {
			return
		}

		// The more entries we have, the less likely we are to add more.
		// likelyhood is 2N.
		factor := int32(2 * ka.refs)
		if a.rand.Int31n(factor) != 0 {
			return
		}
	} else {
		// Make a copy of the net address to avoid races since it is
		// updated elsewhere in the addrmanager code and would otherwise
		// change the actual netaddress on the peer.
		netAddrCopy := *netAddr
		ka = &knownAddress{na: &netAddrCopy, srcAddr: srcAddr}
		a.addrIndex[addr] = ka
		a.nNew++
		// XXX time penalty?
	}

	bucket := a.getNewBucket(netAddr, srcAddr)

	// Already exists?
	if _, ok := a.addrNew[bucket][addr]; ok {
		return
	}

	// Enforce max addresses.
	if len(a.addrNew[bucket]) > newBucketSize {
		amgrLog.Tracef("new bucket is full, expiring old ")
		a.expireNew(bucket)
	}

	// Add to new bucket.
	ka.refs++
	a.addrNew[bucket][addr] = ka

	amgrLog.Tracef("Added new address %s for a total of %d addresses",
		addr, a.nTried+a.nNew)
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
		c /= float64(ka.attempts) * 1.5
	}

	return c
}

// expireNew makes space in the new buckets by expiring the really bad entries.
// If no bad entries are available we look at a few and remove the oldest.
func (a *AddrManager) expireNew(bucket int) {
	// First see if there are any entries that are so bad we can just throw
	// them away. otherwise we throw away the oldest entry in the cache.
	// Bitcoind here chooses four random and just throws the oldest of
	// those away, but we keep track of oldest in the initial traversal and
	// use that information instead.
	var oldest *knownAddress
	for k, v := range a.addrNew[bucket] {
		if bad(v) {
			amgrLog.Tracef("expiring bad address %v", k)
			delete(a.addrNew[bucket], k)
			v.refs--
			if v.refs == 0 {
				a.nNew--
				delete(a.addrIndex, k)
			}
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
		amgrLog.Tracef("expiring oldest address %v", key)

		delete(a.addrNew[bucket], key)
		oldest.refs--
		if oldest.refs == 0 {
			a.nNew--
			delete(a.addrIndex, key)
		}
	}
}

// pickTried selects an address from the tried bucket to be evicted.
// We just choose the eldest. Bitcoind selects 4 random entries and throws away
// the older of them.
func (a *AddrManager) pickTried(bucket int) *list.Element {
	var oldest *knownAddress
	var oldestElem *list.Element
	for e := a.addrTried[bucket].Front(); e != nil; e = e.Next() {
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
	srcAddr     *btcwire.NetAddress
	attempts    int
	lastattempt time.Time
	lastsuccess time.Time
	tried       bool
	refs        int // reference count of new buckets
}

// AddrManager provides a concurrency safe address manager for caching potential
// peers on the bitcoin network.
type AddrManager struct {
	mtx            sync.Mutex
	rand           *rand.Rand
	key            [32]byte
	addrIndex      map[string]*knownAddress // address key to ka for all addrs.
	addrNew        [newBucketCount]map[string]*knownAddress
	addrTried      [triedBucketCount]*list.List
	started        int32
	shutdown       int32
	wg             sync.WaitGroup
	quit           chan bool
	nTried         int
	nNew           int
	lamtx          sync.Mutex
	localAddresses map[string]*localAddress
}

func (a *AddrManager) getNewBucket(netAddr, srcAddr *btcwire.NetAddress) int {
	// bitcoind:
	// doublesha256(key + sourcegroup + int64(doublesha256(key + group + sourcegroup))%bucket_per_source_group) % num_new_buckes

	data1 := []byte{}
	data1 = append(data1, a.key[:]...)
	data1 = append(data1, []byte(GroupKey(netAddr))...)
	data1 = append(data1, []byte(GroupKey(srcAddr))...)
	hash1 := btcwire.DoubleSha256(data1)
	hash64 := binary.LittleEndian.Uint64(hash1)
	hash64 %= newBucketsPerGroup
	var hashbuf [8]byte
	binary.LittleEndian.PutUint64(hashbuf[:], hash64)
	data2 := []byte{}
	data2 = append(data2, a.key[:]...)
	data2 = append(data2, GroupKey(srcAddr)...)
	data2 = append(data2, hashbuf[:]...)

	hash2 := btcwire.DoubleSha256(data2)
	return int(binary.LittleEndian.Uint64(hash2) % newBucketCount)
}

func (a *AddrManager) getTriedBucket(netAddr *btcwire.NetAddress) int {
	// bitcoind hashes this as:
	// doublesha256(key + group + truncate_to_64bits(doublesha256(key)) % buckets_per_group) % num_buckets
	data1 := []byte{}
	data1 = append(data1, a.key[:]...)
	data1 = append(data1, []byte(NetAddressKey(netAddr))...)
	hash1 := btcwire.DoubleSha256(data1)
	hash64 := binary.LittleEndian.Uint64(hash1)
	hash64 %= triedBucketsPerGroup
	var hashbuf [8]byte
	binary.LittleEndian.PutUint64(hashbuf[:], hash64)
	data2 := []byte{}
	data2 = append(data2, a.key[:]...)
	data2 = append(data2, GroupKey(netAddr)...)
	data2 = append(data2, hashbuf[:]...)

	hash2 := btcwire.DoubleSha256(data2)
	return int(binary.LittleEndian.Uint64(hash2) % triedBucketCount)
}

// addressHandler is the main handler for the address manager.  It must be run
// as a goroutine.
func (a *AddrManager) addressHandler() {
	dumpAddressTicker := time.NewTicker(dumpAddressInterval)
out:
	for {
		select {
		case <-dumpAddressTicker.C:
			a.savePeers()

		case <-a.quit:
			break out
		}
	}
	dumpAddressTicker.Stop()
	a.savePeers()
	a.wg.Done()
	amgrLog.Trace("Address handler done")
}

type serialisedKnownAddress struct {
	Addr        string
	Src         string
	Attempts    int
	TimeStamp   int64
	LastAttempt int64
	LastSuccess int64
	// no refcount or tried, that is available from context.
}

type serialisedAddrManager struct {
	Version      int
	Key          [32]byte
	Addresses    []*serialisedKnownAddress
	NewBuckets   [newBucketCount][]string // string is NetAddressKey
	TriedBuckets [triedBucketCount][]string
}

// savePeers saves all the known addresses to a file so they can be read back
// in at next run.
func (a *AddrManager) savePeers() {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	// First we make a serialisable datastructure so we can encode it to
	// json.

	sam := new(serialisedAddrManager)
	sam.Version = serialisationVersion
	copy(sam.Key[:], a.key[:])

	sam.Addresses = make([]*serialisedKnownAddress, len(a.addrIndex))
	i := 0
	for k, v := range a.addrIndex {
		ska := new(serialisedKnownAddress)
		ska.Addr = k
		ska.TimeStamp = v.na.Timestamp.Unix()
		ska.Src = NetAddressKey(v.srcAddr)
		ska.Attempts = v.attempts
		ska.LastAttempt = v.lastattempt.Unix()
		ska.LastSuccess = v.lastsuccess.Unix()
		// Tried and refs are implicit in the rest of the structure
		// and will be worked out from context on unserialisation.
		sam.Addresses[i] = ska
		i++
	}
	for i := range a.addrNew {
		sam.NewBuckets[i] = make([]string, len(a.addrNew[i]))
		j := 0
		for k := range a.addrNew[i] {
			sam.NewBuckets[i][j] = k
			j++
		}
	}
	for i := range a.addrTried {
		sam.TriedBuckets[i] = make([]string, a.addrTried[i].Len())
		j := 0
		for e := a.addrTried[i].Front(); e != nil; e = e.Next() {
			ka := e.Value.(*knownAddress)
			sam.TriedBuckets[i][j] = NetAddressKey(ka.na)
			j++
		}
	}

	// May give some way to specify this later.
	filename := "peers.json"
	filePath := filepath.Join(cfg.DataDir, filename)

	w, err := os.Create(filePath)
	if err != nil {
		amgrLog.Error("Error opening file: ", filePath, err)
		return
	}
	enc := json.NewEncoder(w)
	defer w.Close()
	enc.Encode(&sam)
}

// loadPeers loads the known address from the saved file.  If empty, missing, or
// malformed file, just don't load anything and start fresh
func (a *AddrManager) loadPeers() {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	// May give some way to specify this later.
	filename := "peers.json"
	filePath := filepath.Join(cfg.DataDir, filename)

	err := a.deserialisePeers(filePath)
	if err != nil {
		amgrLog.Errorf("Failed to parse %s: %v", filePath, err)
		// if it is invalid we nuke the old one unconditionally.
		err = os.Remove(filePath)
		if err != nil {
			amgrLog.Warn("Failed to remove corrupt peers "+
				"file: ", err)
		}
		a.reset()
		return
	}
	amgrLog.Infof("Loaded %d addresses from '%s'", a.nNew+a.nTried, filePath)
}

func (a *AddrManager) deserialisePeers(filePath string) error {

	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return nil
	}
	r, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("%s error opening file: %v", filePath, err)
	}
	defer r.Close()

	var sam serialisedAddrManager
	dec := json.NewDecoder(r)
	err = dec.Decode(&sam)
	if err != nil {
		return fmt.Errorf("error reading %s: %v", filePath, err)
	}

	if sam.Version != serialisationVersion {
		return fmt.Errorf("unknown version %v in serialised "+
			"addrmanager", sam.Version)
	}
	copy(a.key[:], sam.Key[:])

	for _, v := range sam.Addresses {
		ka := new(knownAddress)
		ka.na, err = deserialiseNetAddress(v.Addr)
		if err != nil {
			return fmt.Errorf("failed to deserialise netaddress "+
				"%s: %v", v.Addr, err)
		}
		ka.srcAddr, err = deserialiseNetAddress(v.Src)
		if err != nil {
			return fmt.Errorf("failed to deserialise netaddress "+
				"%s: %v", v.Src, err)
		}
		ka.attempts = v.Attempts
		ka.lastattempt = time.Unix(v.LastAttempt, 0)
		ka.lastsuccess = time.Unix(v.LastSuccess, 0)
		a.addrIndex[NetAddressKey(ka.na)] = ka
	}

	for i := range sam.NewBuckets {
		for _, val := range sam.NewBuckets[i] {
			ka, ok := a.addrIndex[val]
			if !ok {
				return fmt.Errorf("newbucket contains %s but "+
					"none in address list", val)
			}

			if ka.refs == 0 {
				a.nNew++
			}
			ka.refs++
			a.addrNew[i][val] = ka
		}
	}
	for i := range sam.TriedBuckets {
		for _, val := range sam.TriedBuckets[i] {
			ka, ok := a.addrIndex[val]
			if !ok {
				return fmt.Errorf("Newbucket contains %s but "+
					"none in address list", val)
			}

			ka.tried = true
			a.nTried++
			a.addrTried[i].PushBack(ka)
		}
	}

	// Sanity checking.
	for k, v := range a.addrIndex {
		if v.refs == 0 && !v.tried {
			return fmt.Errorf("address %s after serialisation "+
				"with no references", k)
		}

		if v.refs > 0 && v.tried {
			return fmt.Errorf("address %s after serialisation "+
				"which is both new and tried!", k)
		}
	}

	return nil
}

func deserialiseNetAddress(addr string) (*btcwire.NetAddress, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, err
	}

	return hostToNetAddress(host, uint16(port), btcwire.SFNodeNetwork)
}

// Start begins the core address handler which manages a pool of known
// addresses, timeouts, and interval based writes.
func (a *AddrManager) Start() {
	// Already started?
	if atomic.AddInt32(&a.started, 1) != 1 {
		return
	}

	amgrLog.Trace("Starting address manager")

	a.wg.Add(1)

	// Load peers we already know about from file.
	a.loadPeers()

	// Start the address ticker to save addresses periodically.
	go a.addressHandler()
}

// Stop gracefully shuts down the address manager by stopping the main handler.
func (a *AddrManager) Stop() error {
	if atomic.AddInt32(&a.shutdown, 1) != 1 {
		amgrLog.Warnf("Address manager is already in the process of " +
			"shutting down")
		return nil
	}

	amgrLog.Infof("Address manager shutting down")
	close(a.quit)
	a.wg.Wait()
	return nil
}

// AddAddresses adds new addresses to the address manager.  It enforces a max
// number of addresses and silently ignores duplicate addresses.  It is
// safe for concurrent access.
func (a *AddrManager) AddAddresses(addrs []*btcwire.NetAddress,
	srcAddr *btcwire.NetAddress) {
	for _, na := range addrs {
		a.updateAddress(na, srcAddr)
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
		amgrLog.Warnf("AddADddressByIP given bullshit adddress"+
			"(%s): %v", err)
		return
	}
	// Put it in btcwire.Netaddress
	var na btcwire.NetAddress
	na.Timestamp = time.Now()
	na.IP = net.ParseIP(addr)
	if na.IP == nil {
		amgrLog.Error("Invalid ip address:", addr)
		return
	}
	port, err := strconv.ParseUint(portStr, 10, 0)
	if err != nil {
		amgrLog.Error("Invalid port: ", portStr, err)
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

	return a.nTried + a.nNew
}

// AddressCache returns the current address cache.  It must be treated as
// read-only (but since it is a copy now, this is not as dangerous).
func (a *AddrManager) AddressCache() []*btcwire.NetAddress {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	if a.nNew+a.nTried == 0 {
		return nil
	}

	allAddr := make([]*btcwire.NetAddress, a.nNew+a.nTried)
	i := 0
	// Iteration order is undefined here, but we randomise it anyway.
	for _, v := range a.addrIndex {
		allAddr[i] = v.na
		i++
	}

	numAddresses := len(allAddr) * getAddrPercent / 100
	if numAddresses > getAddrMax {
		numAddresses = getAddrMax
	}

	// Fisher-Yates shuffle the array. We only need to do the first
	// `numAddresses' since we are throwing the rest.
	for i := 0; i < numAddresses; i++ {
		// pick a number between current index and the end
		j := rand.Intn(len(allAddr)-i) + i
		allAddr[i], allAddr[j] = allAddr[j], allAddr[i]
	}

	// slice off the limit we are willing to share.
	return allAddr[:numAddresses]
}

// reset resets the address manager by reinitialising the random source
// and allocating fresh empty bucket storage.
func (a *AddrManager) reset() {

	a.addrIndex = make(map[string]*knownAddress)

	// fill key with bytes from a good random source.
	io.ReadFull(crand.Reader, a.key[:])
	for i := range a.addrNew {
		a.addrNew[i] = make(map[string]*knownAddress)
	}
	for i := range a.addrTried {
		a.addrTried[i] = list.New()
	}
}

// NewAddrManager returns a new bitcoin address manager.
// Use Start to begin processing asynchronous address updates.
func NewAddrManager() *AddrManager {
	am := AddrManager{
		rand:           rand.New(rand.NewSource(time.Now().UnixNano())),
		quit:           make(chan bool),
		localAddresses: make(map[string]*localAddress),
	}
	am.reset()
	return &am
}

// hostToNetAddress returns a netaddress given a host address. If the address is
// a tor .onion address this will be taken care of. else if the host is not an
// IP address it will be resolved (via tor if required).
func hostToNetAddress(host string, port uint16, services btcwire.ServiceFlag) (*btcwire.NetAddress, error) {
	// tor address is 16 char base32 + ".onion"
	var ip net.IP
	if len(host) == 22 && host[16:] == ".onion" {
		// go base32 encoding uses capitals (as does the rfc
		// but tor and bitcoind tend to user lowercase, so we switch
		// case here.
		data, err := base32.StdEncoding.DecodeString(
			strings.ToUpper(host[:16]))
		if err != nil {
			return nil, err
		}
		prefix := []byte{0xfd, 0x87, 0xd8, 0x7e, 0xeb, 0x43}
		ip = net.IP(append(prefix, data...))
	} else if ip = net.ParseIP(host); ip == nil {
		ips, err := btcdLookup(host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no addresses found for %s", host)
		}
		ip = ips[0]
	}

	return btcwire.NewNetAddressIPPort(ip, port, services), nil
}

// ipString returns a string for the ip from the provided NetAddress. If the
// ip is in the range used for tor addresses then it will be transformed into
// the relavent .onion address.
func ipString(na *btcwire.NetAddress) string {
	if Tor(na) {
		// We know now that na.IP is long enogh.
		base32 := base32.StdEncoding.EncodeToString(na.IP[6:])
		return strings.ToLower(base32) + ".onion"
	}
	return na.IP.String()
}

// NetAddressKey returns a string key in the form of ip:port for IPv4 addresses
// or [ip]:port for IPv6 addresses.
func NetAddressKey(na *btcwire.NetAddress) string {
	port := strconv.FormatUint(uint64(na.Port), 10)
	addr := net.JoinHostPort(ipString(na), port)
	return addr
}

// GetAddress returns a single address that should be routable.  It picks a
// random one from the possible addresses with preference given to ones that
// have not been used recently and should not pick 'close' addresses
// consecutively.
func (a *AddrManager) GetAddress(class string, newBias int) *knownAddress {
	if a.NumAddresses() == 0 {
		return nil
	}

	// Protect concurrent access.
	a.mtx.Lock()
	defer a.mtx.Unlock()

	if newBias > 100 {
		newBias = 100
	}
	if newBias < 0 {
		newBias = 0
	}

	// Bias between new and tried addresses.
	triedCorrelation := math.Sqrt(float64(a.nTried)) *
		(100.0 - float64(newBias))
	newCorrelation := math.Sqrt(float64(a.nNew)) * float64(newBias)

	if (newCorrelation+triedCorrelation)*a.rand.Float64() <
		triedCorrelation {
		// Tried entry.
		large := 1 << 30
		factor := 1.0
		for {
			// pick a random bucket.
			bucket := a.rand.Intn(len(a.addrTried))
			if a.addrTried[bucket].Len() == 0 {
				continue
			}

			// Pick a random entry in the list
			e := a.addrTried[bucket].Front()
			for i :=
				a.rand.Int63n(int64(a.addrTried[bucket].Len())); i > 0; i-- {
				e = e.Next()
			}
			ka := e.Value.(*knownAddress)
			randval := a.rand.Intn(large)
			if float64(randval) < (factor * chance(ka) * float64(large)) {
				amgrLog.Tracef("Selected %v from tried bucket",
					NetAddressKey(ka.na))
				return ka
			}
			factor *= 1.2
		}
	} else {
		// new node.
		// XXX use a closure/function to avoid repeating this.
		large := 1 << 30
		factor := 1.0
		for {
			// Pick a random bucket.
			bucket := a.rand.Intn(len(a.addrNew))
			if len(a.addrNew[bucket]) == 0 {
				continue
			}
			// Then, a random entry in it.
			var ka *knownAddress
			nth := a.rand.Intn(len(a.addrNew[bucket]))
			for _, value := range a.addrNew[bucket] {
				if nth == 0 {
					ka = value
				}
				nth--
			}
			randval := a.rand.Intn(large)
			if float64(randval) < (factor * chance(ka) * float64(large)) {
				amgrLog.Tracef("Selected %v from new bucket",
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
		// ka.na is immutable, so replace it.
		naCopy := *ka.na
		naCopy.Timestamp = time.Now()
		ka.na = &naCopy
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
	naCopy := *ka.na
	naCopy.Timestamp = time.Now()
	ka.na = &naCopy
	ka.attempts = 0

	// move to tried set, optionally evicting other addresses if neeed.
	if ka.tried {
		return
	}

	// ok, need to move it to tried.

	// remove from all new buckets.
	// record one of the buckets in question and call it the `first'
	addrKey := NetAddressKey(addr)
	oldBucket := -1
	for i := range a.addrNew {
		// we check for existance so we can record the first one
		if _, ok := a.addrNew[i][addrKey]; ok {
			delete(a.addrNew[i], addrKey)
			ka.refs--
			if oldBucket == -1 {
				oldBucket = i
			}
		}
	}
	a.nNew--

	if oldBucket == -1 {
		// What? wasn't in a bucket after all.... Panic?
		return
	}

	bucket := a.getTriedBucket(ka.na)

	// Room in this tried bucket?
	if a.addrTried[bucket].Len() < triedBucketSize {
		ka.tried = true
		a.addrTried[bucket].PushBack(ka)
		a.nTried++
		return
	}

	// No room, we have to evict something else.
	entry := a.pickTried(bucket)
	rmka := entry.Value.(*knownAddress)

	// First bucket it would have been put in.
	newBucket := a.getNewBucket(rmka.na, rmka.srcAddr)

	// If no room in the original bucket, we put it in a bucket we just
	// freed up a space in.
	if len(a.addrNew[newBucket]) >= newBucketSize {
		newBucket = oldBucket
	}

	// replace with ka in list.
	ka.tried = true
	entry.Value = ka

	rmka.tried = false
	rmka.refs++

	// We don't touch a.nTried here since the number of tried stays the same
	// but we decemented new above, raise it again since we're putting
	// something back.
	a.nNew++

	rmkey := NetAddressKey(rmka.na)
	amgrLog.Tracef("Replacing %s with %s in tried", rmkey, addrKey)

	// We made sure there is space here just above.
	a.addrNew[newBucket][rmkey] = rmka
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

// RFC4193 IPv6 unique local (FC00::/7)
var rfc4193 = net.IPNet{IP: net.ParseIP("FC00::"),
	Mask: net.CIDRMask(7, 128)}

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
var rfc4843 = net.IPNet{IP: net.ParseIP("2001:10::"),
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
var rfc6052 = net.IPNet{IP: net.ParseIP("64:FF9B::"),
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

var onioncatrange = net.IPNet{IP: net.ParseIP("FD87:d87e:eb43::"),
	Mask: net.CIDRMask(48, 128)}

func Tor(na *btcwire.NetAddress) bool {
	// bitcoind encodes a .onion address as a 16 byte number by decoding the
	// address prior to the .onion (i.e. the key hash) base32 into a ten
	// byte number. it then stores the first 6 bytes of the address as
	// 0xfD, 0x87, 0xD8, 0x7e, 0xeb, 0x43
	// this is the same range used by onioncat, part of the
	// RFC4193 Unique local IPv6 range.
	// In summary the format is:
	// { magic 6 bytes, 10 bytes base32 decode of key hash }
	return onioncatrange.Contains(na.IP)
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
	return na.IP != nil && !(na.IP.IsUnspecified() || RFC3849(na) ||
		na.IP.Equal(net.IPv4bcast))
}

// Routable returns whether a netaddress is routable on the public internet or
// not. This is true as long as the address is valid and is not in any reserved
// ranges.
func Routable(na *btcwire.NetAddress) bool {
	// TODO(oga) bitcoind doesn't include RFC3849 here, but should we?
	return Valid(na) && !(RFC1918(na) || RFC3927(na) || RFC4862(na) ||
		(RFC4193(na) && !Tor(na)) || RFC4843(na) || Local(na))
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
	if Tor(na) {
		// group is keyed off the first 4 bits of the actual onion key.
		return fmt.Sprintf("tor:%d", na.IP[6]&((1<<4)-1))
	}

	// OK, so now we know ourselves to be a IPv6 address.
	// bitcoind uses /32 for everything, except for Hurricane Electric's
	// (he.net) IP range, which it uses /36 for.
	bits := 32
	heNet := &net.IPNet{IP: net.ParseIP("2001:470::"),
		Mask: net.CIDRMask(32, 128)}
	if heNet.Contains(na.IP) {
		bits = 36
	}

	return (&net.IPNet{IP: na.IP, Mask: net.CIDRMask(bits, 128)}).String()
}

// addressPrio is an enum type used to describe the heirarchy of local address
// discovery methods.
type addressPrio int

const (
	InterfacePrio addressPrio = iota // address of local interface.
	BoundPrio                        // Address explicitly bound to.
	UpnpPrio                         // External IP discovered from UPnP
	HTTPPrio                         // Obtained from internet service.
	ManualPrio                       // provided by --externalip.
)

type localAddress struct {
	na    *btcwire.NetAddress
	score addressPrio
}

// addLocalAddress adds na to the list of known local addresses to advertise
// with the given priority.
func (a *AddrManager) addLocalAddress(na *btcwire.NetAddress,
	priority addressPrio) {
	// sanity check.
	if !Routable(na) {
		amgrLog.Debugf("rejecting address %s:%d due to routability",
			na.IP, na.Port)
		return
	}
	amgrLog.Debugf("adding address %s:%d",
		na.IP, na.Port)

	a.lamtx.Lock()
	defer a.lamtx.Unlock()

	key := NetAddressKey(na)
	la, ok := a.localAddresses[key]
	if !ok || la.score < priority {
		if ok {
			la.score = priority + 1
		} else {
			a.localAddresses[key] = &localAddress{
				na:    na,
				score: priority,
			}
		}
	}
}

// getReachabilityFrom returns the relative reachability of na from fromna.
func getReachabilityFrom(na, fromna *btcwire.NetAddress) int {
	const (
		Unreachable = 0
		Default     = iota
		Teredo
		Ipv6Weak
		Ipv4
		Ipv6Strong
		Private
	)

	if !Routable(fromna) {
		return Unreachable
	}

	if Tor(fromna) {
		if Tor(na) {
			return Private
		}

		if Routable(na) && na.IP.To4() != nil {
			return Ipv4
		}

		return Default
	}

	if RFC4380(fromna) {
		if !Routable(na) {
			return Default
		}

		if RFC4380(na) {
			return Teredo
		}

		if na.IP.To4() != nil {
			return Ipv4
		}

		return Ipv6Weak
	}

	if fromna.IP.To4() != nil {
		if Routable(na) && na.IP.To4() != nil {
			return Ipv4
		}
		return Default
	}

	/* ipv6 */
	var tunnelled bool
	// Is our v6 is tunnelled?
	if RFC3964(na) || RFC6052(na) || RFC6145(na) {
		tunnelled = true
	}

	if !Routable(na) {
		return Default
	}

	if RFC4380(na) {
		return Teredo
	}

	if na.IP.To4() != nil {
		return Ipv4
	}

	if tunnelled {
		// only prioritise ipv6 if we aren't tunnelling it.
		return Ipv6Weak
	}

	return Ipv6Strong
}

// getBestLocalAddress returns the most appropriate local address that we know
// of to be contacted by rna
func (a *AddrManager) getBestLocalAddress(rna *btcwire.NetAddress) *btcwire.NetAddress {
	a.lamtx.Lock()
	defer a.lamtx.Unlock()

	bestreach := 0
	var bestscore addressPrio
	var bestna *btcwire.NetAddress
	for _, la := range a.localAddresses {
		reach := getReachabilityFrom(la.na, rna)
		if reach > bestreach ||
			(reach == bestreach && la.score > bestscore) {
			bestreach = reach
			bestscore = la.score
			bestna = la.na
		}
	}
	if bestna != nil {
		amgrLog.Debugf("Suggesting address %s:%d for %s:%d",
			bestna.IP, bestna.Port, rna.IP, rna.Port)
	} else {
		amgrLog.Debugf("No worthy address for %s:%d",
			rna.IP, rna.Port)
		// Send something unroutable if nothing suitable.
		bestna = &btcwire.NetAddress{
			Timestamp: time.Now(),
			Services:  0,
			IP:        net.IP([]byte{0, 0, 0, 0}),
			Port:      0,
		}
	}

	return bestna
}
