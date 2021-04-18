// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// TestThresholdStateStringer tests the stringized output for the
// ThresholdState type.
func TestThresholdStateStringer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   ThresholdState
		want string
	}{
		{ThresholdDefined, "ThresholdDefined"},
		{ThresholdStarted, "ThresholdStarted"},
		{ThresholdLockedIn, "ThresholdLockedIn"},
		{ThresholdActive, "ThresholdActive"},
		{ThresholdFailed, "ThresholdFailed"},
		{0xff, "Unknown ThresholdState (255)"},
	}

	// Detect additional threshold states that don't have the stringer added.
	if len(tests)-1 != int(numThresholdsStates) {
		t.Errorf("It appears a threshold statewas added without " +
			"adding an associated stringer test")
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.String()
		if result != test.want {
			t.Errorf("String #%d\n got: %s want: %s", i, result,
				test.want)
			continue
		}
	}
}

// TestThresholdStateCache ensure the threshold state cache works as intended
// including adding entries, updating existing entries, and flushing.
func TestThresholdStateCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		numEntries int
		state      ThresholdState
	}{
		{name: "2 entries defined", numEntries: 2, state: ThresholdDefined},
		{name: "7 entries started", numEntries: 7, state: ThresholdStarted},
		{name: "10 entries active", numEntries: 10, state: ThresholdActive},
		{name: "5 entries locked in", numEntries: 5, state: ThresholdLockedIn},
		{name: "3 entries failed", numEntries: 3, state: ThresholdFailed},
	}

nextTest:
	for _, test := range tests {
		cache := &newThresholdCaches(1)[0]
		for i := 0; i < test.numEntries; i++ {
			var hash chainhash.Hash
			hash[0] = uint8(i + 1)

			// Ensure the hash isn't available in the cache already.
			_, ok := cache.Lookup(&hash)
			if ok {
				t.Errorf("Lookup (%s): has entry for hash %v",
					test.name, hash)
				continue nextTest
			}

			// Ensure hash that was added to the cache reports it's
			// available and the state is the expected value.
			cache.Update(&hash, test.state)
			state, ok := cache.Lookup(&hash)
			if !ok {
				t.Errorf("Lookup (%s): missing entry for hash "+
					"%v", test.name, hash)
				continue nextTest
			}
			if state != test.state {
				t.Errorf("Lookup (%s): state mismatch - got "+
					"%v, want %v", test.name, state,
					test.state)
				continue nextTest
			}

			// Ensure adding an existing hash with the same state
			// doesn't break the existing entry.
			cache.Update(&hash, test.state)
			state, ok = cache.Lookup(&hash)
			if !ok {
				t.Errorf("Lookup (%s): missing entry after "+
					"second add for hash %v", test.name,
					hash)
				continue nextTest
			}
			if state != test.state {
				t.Errorf("Lookup (%s): state mismatch after "+
					"second add - got %v, want %v",
					test.name, state, test.state)
				continue nextTest
			}

			// Ensure adding an existing hash with a different state
			// updates the existing entry.
			newState := ThresholdFailed
			if newState == test.state {
				newState = ThresholdStarted
			}
			cache.Update(&hash, newState)
			state, ok = cache.Lookup(&hash)
			if !ok {
				t.Errorf("Lookup (%s): missing entry after "+
					"state change for hash %v", test.name,
					hash)
				continue nextTest
			}
			if state != newState {
				t.Errorf("Lookup (%s): state mismatch after "+
					"state change - got %v, want %v",
					test.name, state, newState)
				continue nextTest
			}
		}
	}
}
