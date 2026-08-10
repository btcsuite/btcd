package miniscript

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

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

// TestSatisfyIncompleteSatisfier checks that a satisfier which is missing a
// function the expression needs produces a descriptive error rather than a
// nil-pointer panic, and that a satisfier only has to provide what the
// expression actually uses.
func TestSatisfyIncompleteSatisfier(t *testing.T) {
	t.Parallel()

	sign := func([]byte) ([]byte, bool) {
		return bytes.Repeat([]byte{0x01}, 72), true
	}
	checkTrue := func(uint32) (bool, error) {
		return true, nil
	}
	preimage := func(string, []byte) ([]byte, bool) {
		return make([]byte, 32), true
	}

	// lookupVar resolves the key identifiers; hash values are hex-encoded
	// in the expressions and resolve themselves.
	lookupVar := func(identifier string) ([]byte, error) {
		if len(identifier) > 1 {
			return nil, nil
		}

		key, err := hex.DecodeString(
			"0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959" +
				"f2815b16f81798",
		)
		if err != nil {
			return nil, err
		}

		return key, nil
	}

	tests := []struct {
		name      string
		expr      string
		satisfier *Satisfier
		errStr    string
	}{{
		name:      "older without CheckOlder",
		expr:      "older(144)",
		satisfier: &Satisfier{Sign: sign},
		errStr:    "no CheckOlder function",
	}, {
		name:      "after without CheckAfter",
		expr:      "after(500000100)",
		satisfier: &Satisfier{Sign: sign},
		errStr:    "no CheckAfter function",
	}, {
		name:      "pk without Sign",
		expr:      "pk(A)",
		satisfier: &Satisfier{CheckOlder: checkTrue},
		errStr:    "no Sign function",
	}, {
		name:      "pkh without Sign",
		expr:      "pkh(A)",
		satisfier: &Satisfier{},
		errStr:    "no Sign function",
	}, {
		name:      "multi without Sign",
		expr:      "multi(1,A)",
		satisfier: &Satisfier{},
		errStr:    "no Sign function",
	}, {
		name: "sha256 without Preimage",
		expr: "sha256(926a54995ca48600920a19bf7bc502ca5f2f7d07e6f804" +
			"c4f00ebf0325084dbc)",
		satisfier: &Satisfier{Sign: sign},
		errStr:    "no Preimage function",
	}, {
		// The nested time lock is what needs the missing function, so
		// the check has to look at the whole tree.
		name:      "nested older without CheckOlder",
		expr:      "and_v(v:pk(A),older(144))",
		satisfier: &Satisfier{Sign: sign},
		errStr:    "no CheckOlder function",
	}, {
		name:      "no satisfier at all",
		expr:      "pk(A)",
		satisfier: nil,
		errStr:    "no satisfier provided",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Some of the expressions are not sane on their own,
			// which is beside the point here.
			node, err := ParseInsane(tc.expr, P2WSH)
			require.NoError(t, err)
			require.NoError(t, node.ApplyVars(lookupVar))

			_, err = node.Satisfy(tc.satisfier)
			require.ErrorContains(t, err, tc.errStr)
		})
	}

	// A satisfier that provides exactly what the expression needs, and
	// nothing else, has to work.
	node, err := Parse("and_v(v:pk(A),older(144))", P2WSH)
	require.NoError(t, err)
	require.NoError(t, node.ApplyVars(lookupVar))

	witness, err := node.Satisfy(&Satisfier{
		Sign:       sign,
		CheckOlder: checkTrue,
	})
	require.NoError(t, err)
	require.Len(t, witness, 1)

	// The same for an expression whose only secret is a preimage.
	node, err = ParseInsane(
		"sha256(926a54995ca48600920a19bf7bc502ca5f2f7d07e6f804c4f00e"+
			"bf0325084dbc)", P2WSH,
	)
	require.NoError(t, err)
	require.NoError(t, node.ApplyVars(lookupVar))

	witness, err = node.Satisfy(&Satisfier{Preimage: preimage})
	require.NoError(t, err)
	require.Len(t, witness, 1)
}
