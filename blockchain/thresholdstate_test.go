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

type customDeploymentChecker struct {
	started bool
	ended   bool

	eligible bool

	isSpeedy bool

	conditionTrue bool

	activationThreshold uint32
	minerWindow         uint32
}

func (c customDeploymentChecker) HasStarted(_ *blockNode) bool {
	return c.started
}

func (c customDeploymentChecker) HasEnded(_ *blockNode) bool {
	return c.ended
}

func (c customDeploymentChecker) RuleChangeActivationThreshold() uint32 {
	return c.activationThreshold
}

func (c customDeploymentChecker) MinerConfirmationWindow() uint32 {
	return c.minerWindow
}

func (c customDeploymentChecker) EligibleToActivate(_ *blockNode) bool {
	return c.eligible
}

func (c customDeploymentChecker) IsSpeedy() bool {
	return c.isSpeedy
}

func (c customDeploymentChecker) Condition(_ *blockNode) (bool, error) {
	return c.conditionTrue, nil
}

// TestThresholdStateTransition tests that the thresholdStateTransition
// properly implements the BIP 009 state machine, along with the speedy trial
// augments.
func TestThresholdStateTransition(t *testing.T) {
	t.Parallel()

	// Prev node always points back to itself, effectively creating an
	// infinite chain for the purposes of this test.
	prevNode := &blockNode{}
	prevNode.parent = prevNode

	window := int32(2016)

	testCases := []struct {
		currentState ThresholdState
		nextState    ThresholdState

		checker thresholdConditionChecker
	}{
		// From defined, we stay there if we haven't started the
		// window, and the window hasn't ended.
		{
			currentState: ThresholdDefined,
			nextState:    ThresholdDefined,

			checker: &customDeploymentChecker{},
		},

		// From defined, we go to failed if the window has ended, and
		// this isn't a speedy trial.
		{
			currentState: ThresholdDefined,
			nextState:    ThresholdFailed,

			checker: &customDeploymentChecker{
				ended: true,
			},
		},

		// From defined, even if the window has ended, we go to started
		// if this isn't a speedy trial.
		{
			currentState: ThresholdDefined,
			nextState:    ThresholdStarted,

			checker: &customDeploymentChecker{
				started: true,
			},
		},

		// From started, we go to failed if this isn't speed, and the
		// deployment has ended.
		{
			currentState: ThresholdStarted,
			nextState:    ThresholdFailed,

			checker: &customDeploymentChecker{
				ended: true,
			},
		},

		// From started, we go to locked in if the window passed the
		// condition.
		{
			currentState: ThresholdStarted,
			nextState:    ThresholdLockedIn,

			checker: &customDeploymentChecker{
				started:       true,
				conditionTrue: true,
			},
		},

		// From started, we go to failed if this is a speedy trial, and
		// the condition wasn't met in the window.
		{
			currentState: ThresholdStarted,
			nextState:    ThresholdFailed,

			checker: &customDeploymentChecker{
				started:             true,
				ended:               true,
				isSpeedy:            true,
				conditionTrue:       false,
				activationThreshold: 1815,
			},
		},

		// From locked in, we go straight to active is this isn't a
		// speedy trial.
		{
			currentState: ThresholdLockedIn,
			nextState:    ThresholdActive,

			checker: &customDeploymentChecker{
				eligible: true,
			},
		},

		// From locked in, we remain in locked in if we're not yet
		// eligible to activate.
		{
			currentState: ThresholdLockedIn,
			nextState:    ThresholdLockedIn,

			checker: &customDeploymentChecker{},
		},

		// From active, we always stay here.
		{
			currentState: ThresholdActive,
			nextState:    ThresholdActive,

			checker: &customDeploymentChecker{},
		},

		// From failed, we always stay here.
		{
			currentState: ThresholdFailed,
			nextState:    ThresholdFailed,

			checker: &customDeploymentChecker{},
		},
	}
	for i, testCase := range testCases {
		nextState, err := thresholdStateTransition(
			testCase.currentState, prevNode, testCase.checker,
			window,
		)
		if err != nil {
			t.Fatalf("#%v: unable to transition to next "+
				"state: %v", i, err)
		}

		if nextState != testCase.nextState {
			t.Fatalf("#%v: incorrect state transition: "+
				"expected %v got %v", i, testCase.nextState,
				nextState)
		}
	}
}
