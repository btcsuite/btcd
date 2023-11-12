package miniscript

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/stretchr/testify/require"
)

// TestTimelockMixing asserts that the time lock mixing detection flags
// expressions that combine a height-based and a time-based lock of the same
// kind on a single spending path, and only those.
func TestTimelockMixing(t *testing.T) {
	t.Parallel()

	// The relative time lock type flag is bit 22 (4194304). A value with
	// the bit set is time-based, otherwise it is height-based. The absolute
	// time lock threshold is 500000000.
	testCases := []struct {
		miniscript string
		mixed      bool
	}{{
		// Relative height and relative time lock on the same path.
		miniscript: "and_v(v:older(10),older(4194305))",
		mixed:      true,
	}, {
		// Absolute height and absolute time lock on the same path.
		miniscript: "and_v(v:after(10),after(500000001))",
		mixed:      true,
	}, {
		// Same conflict, but reached via a threshold with k > 1.
		miniscript: "thresh(2,pk(A),sln:older(10),sln:older(4194305))",
		mixed:      true,
	}, {
		// The two conflicting locks are on different branches of an or,
		// so they are never required together.
		miniscript: "or_i(older(10),older(4194305))",
		mixed:      false,
	}, {
		// A threshold of k == 1 behaves like a disjunction, so no
		// conflict.
		miniscript: "thresh(1,pk(A),sln:older(10),sln:older(4194305))",
		mixed:      false,
	}, {
		// Relative height lock and absolute time lock: different kinds,
		// no conflict.
		miniscript: "and_v(v:older(10),after(500000001))",
		mixed:      false,
	}, {
		// Two height-based relative locks: same kind, no conflict.
		miniscript: "and_v(v:older(10),older(20))",
		mixed:      false,
	}, {
		// No time locks at all.
		miniscript: "and_v(v:pk(A),pk(B))",
		mixed:      false,
	}}

	for _, tc := range testCases {
		node, err := Parse(tc.miniscript, P2WSH)
		require.NoErrorf(t, err, "parsing %s", tc.miniscript)

		require.Equalf(
			t, tc.mixed, node.timelock.containsCombination,
			"timelock mixing for %s (info: %+v)", tc.miniscript,
			node.timelock,
		)
	}

	// A script that has a signature (and is thus otherwise sane) but mixes
	// two relative locks of different kinds on the same path must be
	// rejected by IsSane with a time lock error, while the same script with
	// two locks of the same kind is accepted.
	mixed, err := Parse(
		"and_v(v:pk(A),and_v(v:older(10),older(4194305)))", P2WSH,
	)
	require.NoError(t, err)
	saneErr := mixed.IsSane()
	require.Error(t, saneErr)
	require.Contains(t, saneErr.Error(), "time locks")

	clean, err := Parse(
		"and_v(v:pk(A),and_v(v:older(10),older(20)))", P2WSH,
	)
	require.NoError(t, err)
	require.NoError(t, clean.IsSane())
}

// TestStackSize asserts that the maximum witness stack size is computed
// correctly and that the P2WSH standardness limit is enforced.
func TestStackSize(t *testing.T) {
	t.Parallel()

	// maxWitnessSize returns the number of witness elements excluding the
	// witness script, which is pushed separately in a P2WSH spend.
	testCases := []struct {
		miniscript      string
		witnessElements int
	}{{
		miniscript:      "pk(A)",
		witnessElements: 1,
	}, {
		miniscript:      "pkh(A)",
		witnessElements: 2,
	}, {
		// multi is satisfied by a dummy zero plus k signatures.
		miniscript:      "multi(2,A,B,C)",
		witnessElements: 3,
	}, {
		// or_i pushes one extra element to select the branch.
		miniscript:      "or_i(pk(A),pk(B))",
		witnessElements: 2,
	}, {
		// Worst case is the branch that dissatisfies the first multi
		// (3 zeros) and satisfies the second (dummy + 2 sigs).
		miniscript:      "or_d(multi(2,A,B,C),multi(2,K,L,M))",
		witnessElements: 6,
	}, {
		// Satisfy two of the three pk sub expressions (2 sigs) and
		// dissatisfy the third (1 empty push).
		miniscript:      "thresh(2,pk(A),s:pk(B),s:pk(C))",
		witnessElements: 3,
	}}

	for _, tc := range testCases {
		node, err := Parse(tc.miniscript, P2WSH)
		require.NoErrorf(t, err, "parsing %s", tc.miniscript)

		require.Equalf(
			t, tc.witnessElements, node.maxWitnessSize(),
			"witness size for %s", tc.miniscript,
		)
	}

	// A chain of and_v(v:pk, ...) requires one signature per key. With 99
	// keys the satisfaction has 99 witness elements plus the witness
	// script, exactly at the limit of 100. With 100 keys it exceeds the
	// limit and must be rejected.
	andVChain := func(numKeys int) string {
		var sb strings.Builder
		for i := 0; i < numKeys-1; i++ {
			sb.WriteString(fmt.Sprintf("and_v(v:pk(key%d),", i))
		}
		sb.WriteString(fmt.Sprintf("pk(key%d)", numKeys-1))
		sb.WriteString(strings.Repeat(")", numKeys-1))
		return sb.String()
	}

	atLimit, err := Parse(andVChain(99), P2WSH)
	require.NoError(t, err)
	require.Equal(t, 99, atLimit.maxWitnessSize())
	require.NoError(t, atLimit.IsSane())

	overLimit, err := Parse(andVChain(100), P2WSH)
	require.NoError(t, err)
	require.Equal(t, 100, overLimit.maxWitnessSize())
	saneErr := overLimit.IsSane()
	require.Error(t, saneErr)
	require.Contains(t, saneErr.Error(), "witness stack elements")
}

// TestSatisfyMultiThresh exercises the (rewritten, non-naive) satisfaction
// logic for multi and thresh fragments. For every possible subset of available
// signers it builds a real P2WSH spend and checks that the transaction is valid
// exactly when a valid satisfaction should exist.
func TestSatisfyMultiThresh(t *testing.T) {
	t.Parallel()

	// Generate a set of distinct test keys, one per single-letter name.
	names := []string{"A", "B", "C", "D", "E"}
	privKeys := make(map[string]*btcec.PrivateKey)
	pubKeys := make(map[string][]byte)
	for _, name := range names {
		priv, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		privKeys[name] = priv
		pubKeys[name] = priv.PubKey().SerializeCompressed()
	}

	lookupVar := func(identifier string) ([]byte, error) {
		if pk, ok := pubKeys[identifier]; ok {
			return pk, nil
		}
		return nil, fmt.Errorf("unknown identifier %s", identifier)
	}

	// signWith returns a sign function that can produce signatures only for
	// the keys whose names are in the given set.
	signWith := func(canSign map[string]bool) testSignFn {
		return func(pk []byte, hash []byte) ([]byte, bool) {
			for name, pub := range pubKeys {
				if !bytes.Equal(pk, pub) || !canSign[name] {
					continue
				}
				return ecdsa.Sign(
					privKeys[name], hash,
				).Serialize(), true
			}
			return nil, false
		}
	}

	noPreimage := func(string, []byte) ([]byte, bool) { return nil, false }

	testCases := []struct {
		miniscript string

		// satisfiable returns whether a valid, non-malleable
		// satisfaction should exist given the set of signers.
		satisfiable func(signers map[string]bool) bool
	}{{
		// 2-of-3 multisig.
		miniscript: "multi(2,A,B,C)",
		satisfiable: func(s map[string]bool) bool {
			return countSigners(s, "A", "B", "C") >= 2
		},
	}, {
		// 3-of-5 multisig: exercises dropping surplus signatures.
		miniscript: "multi(3,A,B,C,D,E)",
		satisfiable: func(s map[string]bool) bool {
			return countSigners(s, "A", "B", "C", "D", "E") >= 3
		},
	}, {
		// 2-of-3 threshold of single-key checks.
		miniscript: "thresh(2,pk(A),s:pk(B),s:pk(C))",
		satisfiable: func(s map[string]bool) bool {
			return countSigners(s, "A", "B", "C") >= 2
		},
	}, {
		// 2-of-4 threshold: exercises picking the best 2 of 4.
		miniscript: "thresh(2,pk(A),s:pk(B),s:pk(C),s:pk(D))",
		satisfiable: func(s map[string]bool) bool {
			return countSigners(s, "A", "B", "C", "D") >= 2
		},
	}, {
		// Disjunction of two multisigs: the dissatisfaction of the
		// first branch combined with the second is also a valid path.
		miniscript: "or_d(multi(2,A,B,C),multi(2,D,E))",
		satisfiable: func(s map[string]bool) bool {
			return countSigners(s, "A", "B", "C") >= 2 ||
				countSigners(s, "D", "E") >= 2
		},
	}, {
		// 1-of-2 threshold of pkh sub expressions. This is a regression
		// test for the pk_h satisfaction missing its withSig() marker:
		// with the bug, having more signers available than needed made
		// the threshold wrongly report itself unsatisfiable.
		miniscript: "thresh(1,pkh(A),a:pkh(B))",
		satisfiable: func(s map[string]bool) bool {
			return countSigners(s, "A", "B") >= 1
		},
	}, {
		// 2-of-3 threshold of pkh sub expressions.
		miniscript: "thresh(2,pkh(A),a:pkh(B),a:pkh(C))",
		satisfiable: func(s map[string]bool) bool {
			return countSigners(s, "A", "B", "C") >= 2
		},
	}, {
		// pkh mixed with pk in a threshold.
		miniscript: "thresh(2,pk(A),a:pkh(B),a:pk(C))",
		satisfiable: func(s map[string]bool) bool {
			return countSigners(s, "A", "B", "C") >= 2
		},
	}}

	for _, tc := range testCases {
		t.Run(tc.miniscript, func(t *testing.T) {
			t.Parallel()

			// Sweep over every subset of the signer set.
			for mask := 0; mask < (1 << len(names)); mask++ {
				signers := make(map[string]bool)
				for i, name := range names {
					if mask&(1<<i) != 0 {
						signers[name] = true
					}
				}

				err := testRedeem(
					t, tc.miniscript, lookupVar, 0,
					signWith(signers), noPreimage,
				)

				want := tc.satisfiable(signers)
				if want {
					require.NoErrorf(
						t, err, "signers %v should "+
							"satisfy %s", signers,
						tc.miniscript,
					)
				} else {
					require.Errorf(
						t, err, "signers %v should "+
							"not satisfy %s",
						signers, tc.miniscript,
					)
				}
			}
		})
	}
}

// countSigners returns how many of the given key names are in the signer set.
func countSigners(signers map[string]bool, names ...string) int {
	count := 0
	for _, name := range names {
		if signers[name] {
			count++
		}
	}
	return count
}
