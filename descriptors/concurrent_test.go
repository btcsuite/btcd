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
	for i := range n {
		addr, err := d.AddressAt(params, 0, uint32(i))
		require.NoError(t, err)
		want[i] = addr
	}

	// Now derive every index concurrently, many times over, from the same
	// shared Descriptor. Workers own disjoint result slots; assertions run
	// after Wait, in the test goroutine, where FailNow is permitted.
	var wg sync.WaitGroup
	const workers = 16
	var got [workers][n]string
	var errs [workers][n]error
	for worker := range workers {
		wg.Go(func() {
			for i := range n {
				got[worker][i], errs[worker][i] = d.AddressAt(
					params, 0, uint32(i),
				)
			}
		})
	}
	wg.Wait()

	for worker := range workers {
		for i := range n {
			require.NoError(t, errs[worker][i])
			require.Equal(t, want[i], got[worker][i])
		}
	}
}

// TestConcurrentPrivateKeyDerivation exercises the eager initialization of an
// extended private key's public key: a single Descriptor holding an xprv is
// shared across goroutines that each derive an address. Without that
// initialization, the first derivation of every goroutine writes the lazily
// computed public key into the retained hdkeychain.ExtendedKey, which the race
// detector reports as a data race.
func TestConcurrentPrivateKeyDerivation(t *testing.T) {
	t.Parallel()

	const xprv = "xprvA1RpRA33e1JQ7ifknakTFpgNXPmW2YvmhqLQYMmrj4xJXXWYpDP" +
		"S3xz7iAxn8L39njGVyuoseXzU6rcxFLJ8HFsTjSyQbLYnMpCqE2VbFWc"

	d, err := NewDescriptor("wpkh(" + xprv + "/0/*)")
	require.NoError(t, err)

	params := &chaincfg.MainNetParams
	const n = 64

	// Unlike the test above, the concurrent derivations have to come first:
	// a sequential warm-up would fill the public key cache of the xprv and
	// hide the very race this test is about.
	var wg sync.WaitGroup
	got := make([]string, n)
	errs := make([]error, n)
	start := make(chan struct{})
	for i := range n {
		wg.Go(func() {
			<-start
			got[i], errs[i] = d.AddressAt(params, 0, uint32(i))
		})
	}
	close(start)
	wg.Wait()

	// Every concurrent derivation must have succeeded and must agree with a
	// sequential derivation of the same index.
	for i, err := range errs {
		require.NoError(t, err)

		want, err := d.AddressAt(params, 0, uint32(i))
		require.NoError(t, err)
		require.Equal(t, want, got[i])
	}
}
