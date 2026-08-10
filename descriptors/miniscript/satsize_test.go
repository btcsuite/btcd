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

// TestAccessors checks the exported type and resource accessors, which are
// what a test harness outside this package compares against the test vectors.
func TestAccessors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		expr      string
		ctx       Context
		typ       string
		ops       int
		size      int
		elements  int
		satisfies bool
	}{{
		expr: "pk(A)", ctx: P2WSH, typ: "Bondumse",
		ops: 1, size: 73, elements: 2, satisfies: true,
	}, {
		// The two keys of the CHECKMULTISIG count as ops on top of
		// the opcode itself and the CHECKSIGVERIFY of v:pk(A). The
		// witness is the dummy element and two signatures.
		expr: "and_v(v:pk(A),multi(1,B,C))", ctx: P2WSH,
		typ: "Bnumsf", ops: 4, size: 147, elements: 4,
		satisfies: true,
	}, {
		expr: "pk(A)", ctx: P2TR, typ: "Bondumse",
		ops: 1, size: 66, elements: 2, satisfies: true,
	}, {
		// An unsatisfiable expression still has a type, but no
		// satisfaction to measure.
		expr: "0", ctx: P2WSH, typ: "Bzdumse",
	}} {
		t.Run(tc.ctx.String()+"/"+tc.expr, func(t *testing.T) {
			t.Parallel()

			node, err := ParseInsane(tc.expr, tc.ctx)
			require.NoError(t, err)

			require.Equal(t, tc.typ, node.Type())

			ops, err := node.MaxSatisfactionOps()
			if !tc.satisfies {
				require.ErrorIs(t, err, errImpossibleSatisfaction)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.ops, ops)

			size, err := node.MaxSatisfactionSize()
			require.NoError(t, err)
			require.Equal(t, tc.size, size)

			elements, err := node.MaxSatisfactionWitnessElements()
			require.NoError(t, err)
			require.Equal(t, tc.elements, elements)
		})
	}
}

// TestThreshSatField checks threshSatField, which finds one field of a thresh
// satisfaction by satisfying the k sub expressions with the largest delta
// between their sat and dsat and dissatisfying the rest.
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

	// With k=1 the largest-delta sub is satisfied (100) and the other two
	// are dissatisfied (0 each).
	total, ok := threshSatField(subs, 1, size)
	require.True(t, ok)
	require.Equal(t, 100, total)

	// With k=2 the two largest-delta subs are satisfied (100, 50) and the
	// last is dissatisfied (0).
	total, ok = threshSatField(subs, 2, size)
	require.True(t, ok)
	require.Equal(t, 150, total)

	// The field selection is per projection: over the count field the
	// deltas are 4, 2 and 1, so k=1 satisfies the top one (5) and
	// dissatisfies the other two (1 each).
	countSubs := []*AST{
		sub(b(0, 5), b(0, 1)),
		sub(b(0, 3), b(0, 1)),
		sub(b(0, 2), b(0, 1)),
	}
	total, ok = threshSatField(countSubs, 1, count)
	require.True(t, ok)
	require.Equal(t, 7, total)

	// A sub that must be dissatisfied but cannot be makes the whole field
	// impossible. With k=0 every sub is dissatisfied, and one of these two
	// has no dissatisfaction.
	impossible := []*AST{
		sub(b(50, 1), satBound{}),
		sub(b(30, 1), b(0, 1)),
	}
	_, ok = threshSatField(impossible, 0, size)
	require.False(t, ok)
}

// TestThreshSatSizeExactlyK checks that a thresh satisfaction is sized with
// exactly k satisfied sub expressions, which is what its script enforces: it
// adds up the results of all of them and compares the sum to k, so a witness
// that satisfies any other number of them is invalid.
//
// Sizing k+1 of them, as this used to do, inflated the weight of every thresh
// by one satisfaction, and made a thresh whose k+1 largest sub expressions
// were not all satisfiable look like it had no satisfaction at all, even
// though the satisfier produced one. rust-miniscript had the same off-by-one
// and corrected it in rust-bitcoin/rust-miniscript#1040.
func TestThreshSatSizeExactlyK(t *testing.T) {
	t.Parallel()

	const sigLen = 72

	for _, tc := range []struct {
		name string
		expr string
		want int
	}{{
		// One signature with its length prefix, and the single empty
		// element that dissatisfies the other key.
		name: "one of two keys",
		expr: "thresh(1,pk(A),s:pk(B))",
		want: sigLen + 1 + 1,
	}, {
		name: "two of three keys",
		expr: "thresh(2,pk(A),s:pk(B),s:pk(C))",
		want: 2*(sigLen+1) + 1,
	}, {
		// A sub expression that can never be satisfied does not make
		// the thresh unsatisfiable as long as k others can be. The 0
		// consumes no witness element, so dissatisfying it is free.
		name: "dead branch",
		expr: "thresh(1,pk(A),a:0)",
		want: sigLen + 1,
	}} {

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			node, err := ParseInsane(tc.expr, P2WSH)
			require.NoError(t, err)

			stated, err := node.MaxSatisfactionSize()
			require.NoError(t, err)
			require.Equal(t, tc.want, stated)

			// Every key identifier gets a distinct key, since an
			// expression that repeats one is not sane.
			var next byte
			require.NoError(t, node.ApplyVars(
				func(string) ([]byte, error) {
					next++
					key := make([]byte, compressedPubKeyLen)
					key[0], key[compressedPubKeyLen-1] =
						2, next

					return key, nil
				},
			))

			// The size the pass states has to be the size of the
			// witness the satisfier actually produces.
			witness, err := node.Satisfy(&Satisfier{
				Sign: func([]byte) ([]byte, bool) {
					return make([]byte, sigLen), true
				},
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, witnessSize(witness))
		})
	}
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
					Preimage: func(string, []byte) ([]byte,
						bool) {

						return nil, false
					},
				})
				require.NoError(t, err)
				require.Equal(t, tc.want, witnessSize(witness))
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
