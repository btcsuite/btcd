package descriptors

import (
	"sync"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/stretchr/testify/require"
)

// TestConcurrentMiniscriptDerivation exercises the AST cache added for
// miniscript nodes: a single Descriptor is shared across goroutines that each
// derive an address. Because every derivation clones the cached AST before
// mutating it (via ApplyVars), concurrent use must be race-free and produce the
// same addresses as a sequential derivation.
func TestConcurrentMiniscriptDerivation(t *testing.T) {
	t.Parallel()

	// A wsh(miniscript) descriptor, so address derivation goes through the
	// cached-and-cloned miniscript path.
	const desc = "wsh(or_d(pk(" + testXpub1 + "),and_v(v:pk(" +
		testXpub2 + "),older(52560))))"

	d, err := NewDescriptor(desc)
	require.NoError(t, err)

	params := &chaincfg.MainNetParams
	const n = 64

	// Compute the expected address for each index sequentially first.
	want := make([]string, n)
	for i := 0; i < n; i++ {
		addr, err := d.AddressAt(params, 0, uint32(i))
		require.NoError(t, err)
		want[i] = addr
	}

	// Now derive every index concurrently, many times over, from the same
	// shared Descriptor and assert the results are stable.
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n; i++ {
				got, err := d.AddressAt(params, 0, uint32(i))
				require.NoError(t, err)
				require.Equal(t, want[i], got)
			}
		}()
	}
	wg.Wait()
}
