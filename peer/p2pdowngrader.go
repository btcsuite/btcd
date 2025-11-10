package peer

import (
	"github.com/decred/dcrd/lru"
)

const (
	// defaultDowngradeCacheSize is the default number of addresses to store
	// in the P2P downgrader cache.
	defaultDowngradeCacheSize = 100
)

// P2PDowngrader manages a list of peer addresses that should be attempted
// with the v1 P2P protocol on their next connection, typically after a v2
// handshake failure.
type P2PDowngrader struct {
	cache lru.Cache
}

// NewP2PDowngrader returns a new P2PDowngrader instance.
// cacheSize specifies the maximum number of addresses to remember for downgrade.
func NewP2PDowngrader(cacheSize uint) *P2PDowngrader {
	if cacheSize == 0 {
		cacheSize = defaultDowngradeCacheSize
	}
	return &P2PDowngrader{
		cache: lru.NewCache(cacheSize),
	}
}

// MarkForDowngrade flags an address so that the next outbound connection
// attempt to it will use the v1 P2P protocol.
func (pd *P2PDowngrader) MarkForDowngrade(addr string) {
	pd.cache.Add(addr)

	log.Debugf("P2PDowngrader: Marked %s for v1 downgrade", addr)
}

// ShouldDowngrade checks if an address is marked for a v1 downgrade. If the
// address is found, it is removed from the list (consumed), and the function
// returns true. Otherwise, it returns false.
func (pd *P2PDowngrader) ShouldDowngrade(addr string) bool {

	if exists := pd.cache.Contains(addr); exists {
		pd.cache.Delete(addr)

		log.Debugf("P2PDowngrader: Consumed v1 downgrade request "+
			"for %s", addr)

		return true
	}

	return false
}
