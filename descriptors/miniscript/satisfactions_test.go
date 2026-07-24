package miniscript

import (
	"testing"

	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestSubsets checks the subset enumeration used by the threshold passes: it
// yields every strictly-increasing subset of {0,...,n-1} of length k, with no
// duplicates.
func TestSubsets(t *testing.T) {
	t.Parallel()

	// Sizes: choosing k from n yields C(n, k) subsets.
	require.Len(t, subsets(4, 2), 6)
	require.Len(t, subsets(5, 0), 1)
	require.Len(t, subsets(3, 3), 1)
	require.Empty(t, subsets(2, 3))

	// The single length-0 subset is empty.
	require.Empty(t, subsets(5, 0)[0])

	// Every emitted subset has length k, holds strictly increasing indices
	// in range, and is unique.
	seen := make(map[string]bool)
	for _, subset := range subsets(4, 2) {
		require.Len(t, subset, 2)
		for i, idx := range subset {
			require.GreaterOrEqual(t, idx, 0)
			require.Less(t, idx, 4)
			if i > 0 {
				require.Greater(t, idx, subset[i-1])
			}
		}
		key := containsIntKey(subset)
		require.False(t, seen[key], "duplicate subset")
		seen[key] = true
	}
}

// containsIntKey renders a subset as a stable map key for the uniqueness check.
func containsIntKey(subset []int) string {
	key := ""
	for _, idx := range subset {
		key += string(rune('a' + idx))
	}
	return key
}

// TestContainsInt checks the integer membership helper.
func TestContainsInt(t *testing.T) {
	t.Parallel()

	require.True(t, containsInt([]int{1, 3, 5}, 3))
	require.False(t, containsInt([]int{1, 3, 5}, 4))
	require.False(t, containsInt(nil, 0))
}

// TestVerifyLockTime checks the shared lock-time comparison: it rejects mixing
// the two lock-time kinds across the threshold, then requires the required lock
// time to be reached.
func TestVerifyLockTime(t *testing.T) {
	t.Parallel()

	const threshold = uint32(txscript.LockTimeThreshold)

	tests := []struct {
		name       string
		txLockTime uint32
		lockTime   uint32
		want       bool
	}{{
		name:       "height reached",
		txLockTime: 200,
		lockTime:   100,
		want:       true,
	}, {
		name:       "height equal",
		txLockTime: 100,
		lockTime:   100,
		want:       true,
	}, {
		name:       "height not reached",
		txLockTime: 100,
		lockTime:   200,
		want:       false,
	}, {
		name:       "time reached",
		txLockTime: threshold + 200,
		lockTime:   threshold + 100,
		want:       true,
	}, {
		name:       "mixed kinds rejected",
		txLockTime: threshold + 100,
		lockTime:   100,
		want:       false,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, verifyLockTime(
				tc.txLockTime, threshold, tc.lockTime,
			))
		})
	}
}

// TestCheckOlder checks the BIP68/BIP112 relative-locktime predicate: it needs
// the disable bit clear, transaction version at least 2, matching lock-time
// kinds, and the required value to be reached.
func TestCheckOlder(t *testing.T) {
	t.Parallel()

	seconds := uint32(wire.SequenceLockTimeIsSeconds)
	disabled := uint32(wire.SequenceLockTimeDisabled)

	tests := []struct {
		name       string
		lockTime   uint32
		txVersion  uint32
		txSequence uint32
		want       bool
	}{{
		name:       "height reached",
		lockTime:   10,
		txVersion:  2,
		txSequence: 20,
		want:       true,
	}, {
		name:       "height not reached",
		lockTime:   30,
		txVersion:  2,
		txSequence: 20,
		want:       false,
	}, {
		name:       "disable bit set",
		lockTime:   10,
		txVersion:  2,
		txSequence: 20 | disabled,
		want:       false,
	}, {
		name:       "version too low",
		lockTime:   10,
		txVersion:  1,
		txSequence: 20,
		want:       false,
	}, {
		name:       "mixed kinds rejected",
		lockTime:   10,
		txVersion:  2,
		txSequence: seconds | 5,
		want:       false,
	}, {
		name:       "time reached",
		lockTime:   seconds | 5,
		txVersion:  2,
		txSequence: seconds | 20,
		want:       true,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, CheckOlder(
				tc.lockTime, tc.txVersion, tc.txSequence,
			))
		})
	}
}

// TestCheckAfter checks the BIP65 absolute-locktime predicate: it aborts if the
// input sequence is final, rejects mixing lock-time kinds, and requires the
// required value to be reached.
func TestCheckAfter(t *testing.T) {
	t.Parallel()

	const threshold = uint32(txscript.LockTimeThreshold)

	tests := []struct {
		name       string
		value      uint32
		txLockTime uint32
		txSequence uint32
		want       bool
	}{{
		name:       "height reached",
		value:      100,
		txLockTime: 200,
		txSequence: 0,
		want:       true,
	}, {
		name:       "height not reached",
		value:      300,
		txLockTime: 200,
		txSequence: 0,
		want:       false,
	}, {
		name:       "final sequence aborts",
		value:      100,
		txLockTime: 200,
		txSequence: wire.MaxTxInSequenceNum,
		want:       false,
	}, {
		name:       "mixed kinds rejected",
		value:      100,
		txLockTime: threshold + 100,
		txSequence: 0,
		want:       false,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, CheckAfter(
				tc.value, tc.txLockTime, tc.txSequence,
			))
		})
	}
}

// TestSatisfactionAnd checks that combining two satisfactions with "and"
// concatenates their witnesses and folds the flags: available is the logical
// and, while malleable and hasSig are the logical or.
func TestSatisfactionAnd(t *testing.T) {
	t.Parallel()

	s := &satisfaction{
		witness:   wire.TxWitness{{0x01}},
		available: true,
		hasSig:    true,
	}
	b := &satisfaction{
		witness:   wire.TxWitness{{0x02}},
		available: true,
		malleable: true,
	}

	got := s.and(b)
	require.Equal(t, wire.TxWitness{{0x01}, {0x02}}, got.witness)
	require.True(t, got.available)
	require.True(t, got.malleable)
	require.True(t, got.hasSig)

	// An unavailable operand makes the conjunction unavailable.
	unavailable := &satisfaction{available: false}
	require.False(t, s.and(unavailable).available)
}

// TestSatisfactionOr checks the branch preference when combining two
// satisfactions with "or": an unavailable branch is skipped, a branch needing
// a signature is avoided when a signature-free one exists, a malleable branch
// loses to a non-malleable one, and otherwise the smaller witness wins.
func TestSatisfactionOr(t *testing.T) {
	t.Parallel()

	// A one-byte marker in the witness identifies which branch was chosen.
	sat := func(marker byte, avail, malleable, hasSig bool) *satisfaction {
		return &satisfaction{
			witness:   wire.TxWitness{{marker}},
			available: avail,
			malleable: malleable,
			hasSig:    hasSig,
		}
	}
	chosen := func(s *satisfaction) byte { return s.witness[0][0] }

	t.Run("unavailable branch skipped", func(t *testing.T) {
		t.Parallel()

		s := sat(0x01, false, false, true)
		b := sat(0x02, true, false, true)
		require.Equal(t, byte(0x02), chosen(s.or(b)))
	})

	t.Run("signature-free branch preferred", func(t *testing.T) {
		t.Parallel()

		// s needs no signature; the or must keep it over the signed b.
		s := sat(0x01, true, false, false)
		b := sat(0x02, true, false, true)
		require.Equal(t, byte(0x01), chosen(s.or(b)))
	})

	t.Run("non-malleable branch preferred", func(t *testing.T) {
		t.Parallel()

		s := sat(0x01, true, true, true)
		b := sat(0x02, true, false, true)
		require.Equal(t, byte(0x02), chosen(s.or(b)))
	})

	t.Run("smaller witness wins", func(t *testing.T) {
		t.Parallel()

		s := &satisfaction{
			witness:   wire.TxWitness{{0x01}},
			available: true, hasSig: true,
		}
		b := &satisfaction{
			witness:   wire.TxWitness{{0x02, 0x03, 0x04}},
			available: true, hasSig: true,
		}
		require.Equal(t, byte(0x01), chosen(s.or(b)))
	})
}
