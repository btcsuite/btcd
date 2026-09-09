package miniscript

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/txscript/v2"
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
		node, err := ParseInsane(tc.miniscript, P2WSH)
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
	mixed, err := ParseInsane(
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

	// A chain of and_v(v:pk, ...) requires one signature per key. With 100
	// keys the satisfaction has 100 witness elements, exactly at the limit,
	// since the witness script it is pushed with does not count towards it.
	// With 101 keys it exceeds the limit and must be rejected.
	andVChain := func(numKeys int) string {
		var sb strings.Builder
		for i := range numKeys - 1 {
			sb.WriteString(fmt.Sprintf("and_v(v:pk(key%d),", i))
		}
		sb.WriteString(fmt.Sprintf("pk(key%d)", numKeys-1))
		sb.WriteString(strings.Repeat(")", numKeys-1))
		return sb.String()
	}

	atLimit, err := Parse(andVChain(100), P2WSH)
	require.NoError(t, err)
	require.Equal(t, 100, atLimit.maxWitnessSize())
	require.NoError(t, atLimit.IsSane())

	overLimit, err := ParseInsane(andVChain(101), P2WSH)
	require.NoError(t, err)
	require.Equal(t, 101, overLimit.maxWitnessSize())
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
			for mask := range 1 << len(names) {
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

// TestTapscriptSizeLimit checks that the Tapscript script size limit is one the
// script builder can actually emit. The limit used to be the maximum block
// weight (4 MB), while Script() builds with a builder that refuses to grow a
// script past txscript.MaxScriptSize, so leaves between 10 KB and 4 MB passed
// the size check at parse time and could never be compiled: the error surfaced
// at address-derivation or signing time instead.
func TestTapscriptSizeLimit(t *testing.T) {
	t.Parallel()

	require.Equal(t, txscript.MaxScriptSize, maxTapscriptSize)

	// lookupVar assigns a value to the key identifiers of the expression;
	// the hash arguments are hex-encoded already and resolve themselves.
	lookupVar := func(identifier string) ([]byte, error) {
		if len(identifier) > 1 {
			return nil, nil
		}

		key := make([]byte, xOnlyPubKeyLen)
		copy(key, identifier)

		return key, nil
	}

	// 255 hash fragments are 9979 script bytes, just below the limit, 256
	// are 10018 bytes, just above it.
	atLimit, err := Parse(hashChain(255), P2TR)
	require.NoError(t, err)
	require.LessOrEqual(t, atLimit.ScriptLen(), maxTapscriptSize)

	// The expression the size check accepts has to be compilable, which is
	// the property that was broken.
	require.NoError(t, atLimit.ApplyVars(lookupVar))
	script, err := atLimit.Script()
	require.NoError(t, err)
	require.Equal(t, atLimit.ScriptLen(), len(script))

	_, err = Parse(hashChain(256), P2TR)
	require.ErrorContains(
		t, err, "larger than the maximum script size of 10000",
	)

	// Without the sanity check the oversized expression still parses, and
	// building its script is exactly what fails.
	oversized, err := ParseInsane(hashChain(256), P2TR)
	require.NoError(t, err)
	require.Greater(t, oversized.ScriptLen(), maxTapscriptSize)
	require.NoError(t, oversized.ApplyVars(lookupVar))
	_, err = oversized.Script()
	require.ErrorContains(t, err, "maximum allowed canonical script length")
}

// TestLegacyContext checks the resource limits of the Legacy script context, in
// which a miniscript is the redeem script of a P2SH output rather than a
// witness script: its script is limited to 520 bytes, because it is pushed as a
// single element in the spending scriptSig, and multi_a does not exist
// pre-taproot.
//
// Before the context existed, an sh() inner was compiled as P2WSH, so redeem
// scripts of up to 3600 bytes were accepted and the derived P2SH addresses were
// unspendable.
func TestLegacyContext(t *testing.T) {
	t.Parallel()

	require.Equal(t, maxRedeemScriptSize, Legacy.maxScriptSize())
	require.Equal(t, "Legacy", Legacy.String())

	// Legacy uses compressed keys and the multi fragment, like P2WSH.
	require.Equal(t, compressedPubKeyLen, Legacy.keyLen())
	require.Equal(t, multisigMaxKeys, Legacy.maxMultiKeys())

	// pkChain returns and_v(v:pk(K1),...,pk(Kn)), which is 35 script bytes
	// per key: the key push plus its OP_CHECKSIG, collapsed into
	// OP_CHECKSIGVERIFY by the v: wrapper for all but the last one.
	pkChain := func(numKeys int) string {
		var b strings.Builder
		for i := range numKeys - 1 {
			fmt.Fprintf(&b, "and_v(v:pk(key%d),", i)
		}
		fmt.Fprintf(&b, "pk(key%d)", numKeys-1)
		b.WriteString(strings.Repeat(")", numKeys-1))

		return b.String()
	}

	// 14 keys are 490 bytes, within the redeem script limit.
	atLimit, err := Parse(pkChain(14), Legacy)
	require.NoError(t, err)
	require.Equal(t, 490, atLimit.ScriptLen())

	// 15 keys are 525 bytes, over it, while the same expression is a
	// perfectly good witness script.
	_, err = Parse(pkChain(15), Legacy)
	require.ErrorContains(
		t, err,
		"larger than the maximum script size of 520 in the Legacy "+
			"context",
	)

	_, err = Parse(pkChain(15), P2WSH)
	require.NoError(t, err)

	// multi is the pre-taproot multisig fragment, multi_a the taproot one.
	_, err = Parse("multi(1,key1)", Legacy)
	require.NoError(t, err)

	_, err = Parse("multi_a(1,key1)", Legacy)
	require.ErrorContains(t, err, "multi_a is not allowed in the Legacy")
}

// TestPushScriptSize checks the size of the redeem script push that a legacy
// scriptSig has to hold, which grows by the length bytes of the push opcode.
func TestPushScriptSize(t *testing.T) {
	t.Parallel()

	// Up to 75 bytes the size is the push opcode itself.
	require.Equal(t, 1+10, pushScriptSize(10))
	require.Equal(t, 1+75, pushScriptSize(75))

	// From 76 bytes on, OP_PUSHDATA1 plus a length byte.
	require.Equal(t, 2+76, pushScriptSize(76))
	require.Equal(t, 2+255, pushScriptSize(255))

	// From 256 bytes on, OP_PUSHDATA2 plus two length bytes.
	require.Equal(t, 3+256, pushScriptSize(256))
	require.Equal(t, 3+520, pushScriptSize(520))
}

// TestStackSizeLimit checks that the consensus limit on the number of stack
// elements applies to the P2WSH context as well, which is what BIP379 states
// ("In both Tapscript and P2WSH, script satisfactions which make the stack
// exceed 1000 elements before or during execution are invalid") and what
// rust-miniscript enforces in its Segwitv0 context. It used to be checked for
// Tapscript only.
//
// No sane expression can actually reach the limit under the script size limits
// of either context: every witness element a sane satisfaction needs costs at
// least a key push in the script (34 bytes), so 10,000 script bytes cap the
// stack at roughly 300 elements. The rule is therefore checked directly on the
// computed sizes, and it is worth having: it is the limit that binds if the
// script size limit is ever raised to what Tapscript really allows.
func TestStackSizeLimit(t *testing.T) {
	t.Parallel()

	// In P2WSH the standardness limit on witness elements (100) is reached
	// long before the stack limit.
	var chain strings.Builder
	for i := range 98 {
		fmt.Fprintf(&chain, "and_v(v:pk(key%d),", i)
	}
	chain.WriteString("pk(key98)")
	chain.WriteString(strings.Repeat(")", 98))

	atWitnessLimit, err := Parse(chain.String(), P2WSH)
	require.NoError(t, err)
	require.Equal(t, 99, atWitnessLimit.maxWitnessSize())
	require.Less(
		t,
		atWitnessLimit.maxWitnessSize()+
			atWitnessLimit.maxExecStackSize(),
		maxStackSize,
	)

	// A satisfaction whose witness and execution stack together exceed the
	// limit is rejected in both segwit contexts.
	node, err := ParseInsane("pk(key0)", P2TR)
	require.NoError(t, err)
	require.NoError(t, node.validSatisfactions())

	node.stackSize.sat = maxInt{valid: true, value: maxStackSize}
	node.execStack.sat = maxInt{valid: true, value: 1}

	for _, ctx := range []Context{P2TR, P2WSH} {
		node.ctx = ctx
		require.ErrorContainsf(
			t, node.validSatisfactions(),
			"larger than the consensus limit of 1000", "context %v",
			ctx,
		)
	}

	// The legacy context does not check it, since the 520-byte redeem
	// script limit keeps the stack far out of reach there, matching
	// rust-miniscript.
	node.ctx = Legacy
	require.NoError(t, node.validSatisfactions())
}

// TestValueArgumentWrappers checks that a wrapper on an argument which is a
// value rather than a sub expression is rejected. Wrappers are only defined for
// sub expressions, so the tree passes never look at the wrappers of a key, hash
// or number argument, and accepting them would silently drop them from the
// compiled script: older(v:100) would compile to the script of older(100).
func TestValueArgumentWrappers(t *testing.T) {
	t.Parallel()

	const hash = "6c60f404f8167a38fc70eaf8aa17ac351023bef86bcb9d1086a19af" +
		"e95bd5333"

	for _, expr := range []string{
		"older(v:100)",
		"after(xyz:500000000)",
		"multi(j:1,key1)",
		"multi(1,v:key1)",
		"thresh(n:1,pk(key1))",
		"pk(v:key1)",
		"pk_k(d:key1)",
		"pkh(a:key1)",
		"pk_h(s:key1)",
		"sha256(v:" + hash + ")",
		"hash160(c:" + hash[:40] + ")",
	} {

		t.Run(expr, func(t *testing.T) {
			t.Parallel()

			_, err := ParseInsane(expr, P2WSH)
			require.ErrorContains(t, err, "must not have wrappers")
		})
	}

	// The same expressions without the wrapper are accepted, so it is
	// really only the wrapper that is rejected.
	for _, expr := range []string{
		"older(100)", "after(500000000)", "multi(1,key1)",
		"thresh(1,pk(key1))", "pk(key1)", "pk_k(key1)", "pkh(key1)",
		"pk_h(key1)", "sha256(" + hash + ")",
		"hash160(" + hash[:40] + ")",
	} {

		t.Run(expr, func(t *testing.T) {
			t.Parallel()

			_, err := ParseInsane(expr, P2WSH)
			require.NoError(t, err)
		})
	}
}

// TestDupIfUnit checks that the d: wrapper is unit in Tapscript but not in
// P2WSH. MINIMALIF is a consensus rule in Tapscript, so the only element that
// satisfies the OP_IF of a d: there is the single byte 0x01, which is what the
// u property means; P2WSH consensus also permits other Script-true values.
// BIP379 assigns the
// property accordingly, and without it a valid Tapscript expression was
// rejected for the type requirements of its parent fragment.
func TestDupIfUnit(t *testing.T) {
	t.Parallel()

	tap, err := ParseInsane("dv:older(1)", P2TR)
	require.NoError(t, err)
	require.True(t, tap.props.u)

	wsh, err := ParseInsane("dv:older(1)", P2WSH)
	require.NoError(t, err)
	require.False(t, wsh.props.u)

	// The first argument of andor has to be unit, so the same expression
	// only type-checks in the Tapscript context.
	_, err = ParseInsane("andor(dv:older(1),pk(A),pk(B))", P2TR)
	require.NoError(t, err)

	_, err = ParseInsane("andor(dv:older(1),pk(A),pk(B))", P2WSH)
	require.ErrorContains(
		t, err,
		"wrong properties on `d` in the first argument of `andor`",
	)
}
