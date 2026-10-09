package miniscript

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	// maxFuzzInput bounds the size of a fuzz input so individual executions
	// stay practical while leaving room to reach every fragment. Nesting
	// limits alone do not bound input length or the number of sibling
	// nodes.
	maxFuzzInput = 4096
)

var (
	// miniscriptFuzzSeedFiles are the committed corpora fed in as fuzz
	// seeds. Their first whitespace-separated token is the expression (the
	// remaining columns are the expected type and op count in some files).
	miniscriptFuzzSeedFiles = []string{
		"valid_from_alloy.txt",
		"valid_8f1e8_from_alloy.txt",
		"malleable_from_alloy.txt",
		"conflict_from_alloy.txt",
		"edge_cases.txt",
		"opcodes.txt",
		"invalid.txt",
	}

	// hardcodedMiniscriptSeeds covers every fragment kind with
	// single-letter keys, keeping the seed set representative even without
	// the corpus files.
	hardcodedMiniscriptSeeds = []string{
		"0",
		"1",
		"pk(A)",
		"pkh(A)",
		"pk_k(A)",
		"pk_h(A)",
		"older(144)",
		"after(500000000)",
		"sha256(" + strings.Repeat("0", 64) + ")",
		"and_v(v:pk(A),pk(B))",
		"and_b(pk(A),s:pk(B))",
		"or_b(pk(A),s:pk(B))",
		"or_c(pk(A),v:pk(B))",
		"or_d(pk(A),pk(B))",
		"or_i(pk(A),pk(B))",
		"andor(pk(A),pk(B),pk(C))",
		"thresh(2,pk(A),s:pk(B),s:pk(C))",
		"multi(2,A,B,C)",
		"multi_a(2,A,B,C)",
		"and_v(v:pk(A),or_d(pk(B),older(65535)))",
	}
)

// FuzzParse fuzzes miniscript parsing and, on every successful parse, exercises
// the deep downstream passes (lifting, satisfaction sizing, key extraction,
// script generation and satisfaction) in both script contexts. It checks the
// no-crash property throughout, plus the invariant that the statically computed
// script length matches the length of the actually generated script.
func FuzzParse(f *testing.F) {
	// Combine the large regression corpora with small seeds covering each
	// fragment, so a checkout without the corpora still exercises the API.
	for _, name := range miniscriptFuzzSeedFiles {
		addMiniscriptSeeds(f, filepath.Join("testdata", name))
	}
	for _, seed := range hardcodedMiniscriptSeeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, expr string) {
		if len(expr) > maxFuzzInput {
			return
		}

		// A miniscript is valid (or not) largely independent of the
		// context, but the allowed fragments differ (multi vs multi_a),
		// so both are worth exercising.
		for _, ctx := range []Context{P2WSH, P2TR} {
			node, err := ParseInsane(expr, ctx)
			if err != nil {
				// Parse is ParseInsane plus the sanity checks,
				// so it must reject whatever ParseInsane
				// rejects.
				_, saneErr := Parse(expr, ctx)
				require.Error(t, saneErr)

				continue
			}
			exerciseAST(t, node, ctx, expr)
		}
	})
}

// exerciseAST drives the deep, key-independent and key-dependent operations of
// a successfully parsed miniscript, asserting only invariants that hold for any
// valid expression.
func exerciseAST(t *testing.T, node *AST, ctx Context, expr string) {
	t.Helper()

	// Key-independent operations must never panic.
	_ = node.DrawTree()
	_ = node.Keys()
	_, _ = node.Lift()
	_, _ = node.MaxSatisfactionSize()
	_, _ = node.MaxSatisfactionWitnessElements()

	// Substitute a concrete key of the length the context expects, then
	// check that the compiled script has exactly the statically computed
	// length. This ties the size pass to the real script builder.
	keyLen := 33
	if ctx == P2TR {
		keyLen = 32
	}
	dummyKey := append([]byte{
		0x02,
	}, bytes.Repeat([]byte{
		0x01,
	}, keyLen-1)...)

	err := node.ApplyVars(func(string) ([]byte, error) {
		return dummyKey, nil
	})
	if err != nil {
		return
	}

	script, err := node.Script()
	if err == nil {
		require.Equalf(
			t, node.ScriptLen(), len(script),
			"ScriptLen mismatch for %q in %v", expr, ctx,
		)
	}

	// A satisfier that provides everything drives the full satisfaction
	// machinery; it must not panic regardless of the shape of the script.
	// The signature it hands out has the size the satisfaction size pass
	// assumes for the context, so that the witness it produces can be held
	// against the computed maximum.
	sigLen := 72
	if ctx == P2TR {
		sigLen = 65
	}

	witness, err := node.Satisfy(&Satisfier{
		Sign: func([]byte) ([]byte, bool) {
			return make([]byte, sigLen), true
		},
		Preimage: func(string, []byte) ([]byte, bool) {
			return make([]byte, 32), true
		},
		CheckOlder: func(uint32) (bool, error) {
			return true, nil
		},
		CheckAfter: func(uint32) (bool, error) {
			return true, nil
		},
	})
	if err != nil {
		return
	}

	// The satisfaction must fit in the computed maximum, which is what a
	// fee estimate is built on. Only the size is checked: the element count
	// of a dissatisfaction is modelled after the canonical, non-malleable
	// path, which an insane expression may not take.
	if stated, err := node.MaxSatisfactionSize(); err == nil {
		require.LessOrEqualf(
			t, witnessSize(witness), stated,
			"satisfaction of %q in %v is larger than its computed "+
				"maximum size",
			expr, ctx,
		)
	}
}

// addMiniscriptSeeds adds the first token of each non-empty, non-comment line
// of the given file as a fuzz seed. A missing file is ignored so the fuzzer
// stays usable without the full corpus.
func addMiniscriptSeeds(f *testing.F, path string) {
	f.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f.Add(strings.Fields(line)[0])
	}
}
