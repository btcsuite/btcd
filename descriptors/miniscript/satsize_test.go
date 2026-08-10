package miniscript

import (
	"testing"

	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestSatBound checks the concat and fieldMax combinators of the satBound
// helper used by the satisfaction-size pass. concat sums the byte size and
// element count of two bounds applied in sequence (and is impossible if either
// is); fieldMax takes the field-wise maximum of two alternatives, ignoring an
// impossible one.
func TestSatBound(t *testing.T) {
	t.Parallel()

	b := func(size, count int) satBound {
		return satBound{valid: true, size: size, count: count}
	}
	invalid := satBound{}

	t.Run("concat", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, b(30, 3), b(10, 1).concat(b(20, 2)))
		require.Equal(t, invalid, b(10, 1).concat(invalid))
		require.Equal(t, invalid, invalid.concat(b(20, 2)))
		require.Equal(t, invalid, invalid.concat(invalid))
	})

	t.Run("fieldMax", func(t *testing.T) {
		t.Parallel()

		// Each field is maximised independently, so the result can draw
		// its size from one operand and its count from the other.
		require.Equal(t, b(30, 9), b(30, 1).fieldMax(b(5, 9)))
		require.Equal(t, b(20, 3), b(10, 1).fieldMax(b(20, 3)))

		// An impossible alternative is ignored entirely.
		require.Equal(t, b(10, 1), b(10, 1).fieldMax(invalid))
		require.Equal(t, b(20, 2), invalid.fieldMax(b(20, 2)))
		require.Equal(t, invalid, invalid.fieldMax(invalid))
	})
}

// TestThreshSatField checks threshSatField, which finds one field of a thresh
// satisfaction by satisfying the k+1 sub expressions with the largest delta
// between their sat and dsat and dissatisfying the rest. The k+1 (rather than
// k) is the conservative over-count carried over from rust-miniscript.
func TestThreshSatField(t *testing.T) {
	t.Parallel()

	sub := func(sat, dsat satBound) *AST {
		return &AST{satSize: witSize{sat: sat, dsat: dsat}}
	}
	b := func(size, count int) satBound {
		return satBound{valid: true, size: size, count: count}
	}
	size := func(bound satBound) int { return bound.size }
	count := func(bound satBound) int { return bound.count }

	// Three sub expressions with distinct size deltas 100, 50 and 10.
	subs := []*AST{
		sub(b(100, 1), b(0, 1)),
		sub(b(50, 1), b(0, 1)),
		sub(b(10, 1), b(0, 1)),
	}

	// With k=1 the two largest-delta subs are satisfied (100, 50) and the
	// last is dissatisfied (0), so 150 rather than the tight 1-of-3 total.
	total, ok := threshSatField(subs, 1, size)
	require.True(t, ok)
	require.Equal(t, 150, total)

	// With k=2 all three (k+1 capped at n) are satisfied.
	total, ok = threshSatField(subs, 2, size)
	require.True(t, ok)
	require.Equal(t, 160, total)

	// The field selection is per projection: over the count field the
	// deltas are 4, 2 and 1, so k=1 satisfies the top two (5, 3) and
	// dissatisfies the last (1).
	countSubs := []*AST{
		sub(b(0, 5), b(0, 1)),
		sub(b(0, 3), b(0, 1)),
		sub(b(0, 2), b(0, 1)),
	}
	total, ok = threshSatField(countSubs, 1, count)
	require.True(t, ok)
	require.Equal(t, 9, total)

	// A sub that must be dissatisfied but cannot be makes the whole field
	// impossible. With k=0 one sub is satisfied and one dissatisfied; the
	// sub with an impossible dissatisfaction sorts into the dissatisfied
	// slot.
	impossible := []*AST{
		sub(b(50, 1), satBound{}),
		sub(b(30, 1), b(0, 1)),
	}
	_, ok = threshSatField(impossible, 0, size)
	require.False(t, ok)
}

// TestSatSizeAgainstSatisfier checks the computed maximum satisfaction size of
// the d: wrapper against the witness its own satisfier produces. The wrapper
// adds a selector element holding 0x01, which is two bytes on the witness
// stack: the length prefix and the byte itself, the same as the <1> selector of
// or_i. Since d: only wraps fragments that consume no witness elements of their
// own (its argument is of type Vz), the selector is the whole difference.
func TestSatSizeAgainstSatisfier(t *testing.T) {
	t.Parallel()

	for _, ctx := range []Context{P2WSH, P2TR} {
		// A signature of the size the satisfaction size pass assumes
		// for the context, so that the computed maximum and the
		// produced witness are directly comparable.
		sigLen, keyLen := 72, compressedPubKeyLen
		if ctx == P2TR {
			sigLen, keyLen = 65, xOnlyPubKeyLen
		}

		for _, tc := range []struct {
			expr string
			want int
		}{{
			// <1>
			expr: "dv:older(144)",
			want: 2,
		}, {
			// <1> <1> plus the empty element of the a: branch
			expr: "and_b(dv:older(144),adv:older(100))",
			want: 4,
		}, {
			// <signature> <1>
			expr: "and_v(v:pk(A),dv:older(144))",
			want: sigLen + 1 + 2,
		}} {

			t.Run(tc.expr+" "+ctx.String(), func(t *testing.T) {
				t.Parallel()

				// The first two expressions hold no
				// signature at all, which makes them
				// insane, so the sanity checks are
				// skipped here.
				node, err := ParseInsane(tc.expr, ctx)
				require.NoError(t, err)

				stated, err := node.MaxSatisfactionSize()
				require.NoError(t, err)
				require.Equal(t, tc.want, stated)

				key := make([]byte, keyLen)
				key[keyLen-1] = 1
				if keyLen == compressedPubKeyLen {
					key[0] = 2
				}
				require.NoError(t, node.ApplyVars(
					func(string) ([]byte, error) {
						return key, nil
					},
				))

				witness, err := node.Satisfy(&Satisfier{
					Sign: func([]byte) ([]byte, bool) {
						return make([]byte, sigLen),
							true
					},
					CheckOlder: func(uint32) (bool, error) {
						return true, nil
					},
					CheckAfter: func(uint32) (bool, error) {
						return true, nil
					},
					Preimage: func(string,
						[]byte) ([]byte, bool) {

						return nil, false
					},
				})
				require.NoError(t, err)
				require.Equal(
					t, tc.want, witnessSize(witness),
				)
			})
		}
	}
}

// witnessSize returns the size in bytes of the given witness elements, each
// including its own length prefix but without the prefix that encodes the
// number of elements, which is the convention of the satisfaction size pass.
func witnessSize(witness [][]byte) int {
	size := 0
	for _, element := range witness {
		size += wire.VarIntSerializeSize(uint64(len(element))) +
			len(element)
	}

	return size
}
