package peer

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// TestP2PDowngraderMarkAndShouldDowngrade tests the basic MarkForDowngrade and
// ShouldDowngrade functionality.
func TestP2PDowngraderMarkAndShouldDowngrade(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		// addrGen creates somewhat realistic-looking address strings.
		addrGen := rapid.Map(rapid.SliceOfN(
			rapid.Byte(), 10, 20), func(bs []byte) string {
			return fmt.Sprintf("%x.testpeer.net", bs)
		})

		addr1 := addrGen.Draw(t, "addr1")
		addr2 := addrGen.Draw(t, "addr2")

		// Ensure addr1 and addr2 are different for a clearer test,
		// though the logic holds even if they are the same.
		for addr1 == addr2 {
			// Suffix with retry to ensure rapid sees it as a new
			// draw attempt for addr2.
			addr2 = addrGen.Draw(t, "addr2_retry")
		}

		// Create a P2PDowngrader with a small capacity.
		downgrader := NewP2PDowngrader(2)

		// Initially, no address should be marked for downgrade.
		require.False(
			t, downgrader.ShouldDowngrade(addr1),
			"addr1 should not be marked initially",
		)
		require.False(
			t, downgrader.ShouldDowngrade(addr2),
			"addr2 should not be marked initially",
		)

		// Mark addr1 for downgrade.
		downgrader.MarkForDowngrade(addr1)

		// Now, ShouldDowngrade for addr1 should return true.
		require.True(
			t, downgrader.ShouldDowngrade(addr1),
			"addr1 should be marked for downgrade after "+
				"MarkForDowngrade",
		)

		// A subsequent call for addr1 should return false as it is
		// consumed.
		require.False(
			t, downgrader.ShouldDowngrade(addr1),
			"addr1 should not be marked after being consumed "+
				"by ShouldDowngrade",
		)

		// addr2 should still not be marked.
		require.False(
			t, downgrader.ShouldDowngrade(addr2),
			"addr2 should remain unmarked",
		)

		downgrader.MarkForDowngrade(addr2)

		require.True(
			t, downgrader.ShouldDowngrade(addr2),
			"addr2 should be marked",
		)
		require.False(
			t, downgrader.ShouldDowngrade(addr2),
			"addr2 should be consumed after ShouldDowngrade",
		)
	})
}

// TestP2PDowngraderLRUEviction tests the LRU eviction behavior of the cache.
func TestP2PDowngraderLRUEviction(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		// Capacity for the LRU cache.
		capacity := rapid.UintRange(1, 5).Draw(t, "capacity")

		// Generate a list of unique addresses, one more than the capacity
		// to trigger eviction.
		numAddresses := int(capacity + 1)
		addresses := rapid.SliceOfNDistinct(
			rapid.String(), numAddresses, numAddresses, rapid.ID,
		).Draw(t, "addresses")

		downgrader := NewP2PDowngrader(capacity)

		// Mark the first 'capacity' addresses. These are addresses[0]
		// through addresses[capacity-1]. In dcrd/lru, Add puts new
		// items at the front (most recent). So, after this loop,
		// addresses[capacity-1] is newest, addresses[0] is oldest.
		for i := 0; i < int(capacity); i++ {
			downgrader.MarkForDowngrade(addresses[i])
		}

		// The address that should be evicted is addresses[0] (the

		addressToBeEvicted := addresses[0]

		// The address that causes the eviction is addresses[capacity].
		evictingAddress := addresses[capacity]
		downgrader.MarkForDowngrade(evictingAddress)

		// Check that the address that should have been evicted is no
		// longer marked.
		require.False(
			t, downgrader.ShouldDowngrade(addressToBeEvicted),
			"address %s should have been evicted by %s",
			addressToBeEvicted, evictingAddress,
		)

		// Check that the evicting address is marked and consumable.
		require.True(
			t, downgrader.ShouldDowngrade(evictingAddress),
			"address %s (the evicting one) should "+
				"be marked", evictingAddress,
		)

		// Check the remaining (capacity-1) addresses that were not the
		// first one. These are addresses[1] through
		// addresses[capacity-1]. They should still be present.
		for i := 1; i < int(capacity); i++ {
			require.True(
				t, downgrader.ShouldDowngrade(addresses[i]),
				"address %s should still be marked",
				addresses[i],
			)
		}
	})
}

// TestP2PDowngraderMarkExistingUpdatesLRU tests that marking an existing
// address moves it to the front of the LRU list. This prevents it from being
// evicted prematurely.
func TestP2PDowngraderMarkExistingUpdatesLRU(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		const capacity = 3

		// Generate 4 unique addresses.
		numAddresses := capacity + 1
		addresses := rapid.SliceOfNDistinct(
			rapid.String(), numAddresses, numAddresses, rapid.ID,
		).Draw(t, "addresses")

		addrA := addresses[0]
		addrB := addresses[1]
		addrC := addresses[2]

		// This will be the evicting address.
		addrD := addresses[3]

		downgrader := NewP2PDowngrader(capacity)

		// Step 1: Mark A, B, C.
		// Cache state (newest to oldest): [C, B, A].
		downgrader.MarkForDowngrade(addrA)
		downgrader.MarkForDowngrade(addrB)
		downgrader.MarkForDowngrade(addrC)

		// Step 2: Mark A again. This should move A to the front
		// (newest). Cache state (newest to oldest): [A, C, B].
		downgrader.MarkForDowngrade(addrA)

		// Step 3: Mark D. This should evict B (which is now the
		// oldest). Cache state (newest to oldest): [D, A, C].
		downgrader.MarkForDowngrade(addrD)

		// Assert that B was evicted.
		require.False(
			t, downgrader.ShouldDowngrade(addrB),
			"addrB should have been evicted",
		)

		// Assert that A, C, and D are still present and consumable.
		// Order of checking does not strictly matter here, just
		// presence.
		require.True(
			t, downgrader.ShouldDowngrade(addrA),
			"addrA should be marked (was re-marked "+
				"and moved to front)",
		)
		require.True(
			t, downgrader.ShouldDowngrade(addrC),
			"addrC should be marked",
		)
		require.True(
			t, downgrader.ShouldDowngrade(addrD),
			"addrD should be marked (evicted B)",
		)

		// Verify they are consumed.
		require.False(
			t, downgrader.ShouldDowngrade(addrA),
			"addrA should be consumed",
		)
		require.False(
			t, downgrader.ShouldDowngrade(addrC),
			"addrC should be consumed",
		)
		require.False(
			t, downgrader.ShouldDowngrade(addrD),
			"addrD should be consumed",
		)
	})
}
