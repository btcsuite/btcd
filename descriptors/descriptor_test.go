package descriptors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSplitArgs checks that a comma-separated argument list is split only at
// the top nesting level, respecting (), {}, [] and <> grouping.
func TestSplitArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want []string
	}{{
		name: "flat list",
		in:   "a,b,c",
		want: []string{"a", "b", "c"},
	}, {
		name: "nested parens",
		in:   "a,f(b,c),d",
		want: []string{"a", "f(b,c)", "d"},
	}, {
		name: "nested braces",
		in:   "a,{b,c},d",
		want: []string{"a", "{b,c}", "d"},
	}, {
		name: "multipath angle brackets",
		in:   "a,<0;1>,b",
		want: []string{"a", "<0;1>", "b"},
	}, {
		name: "single element",
		in:   "a",
		want: []string{"a"},
	}, {
		name: "empty string",
		in:   "",
		want: []string{""},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, splitArgs(tc.in))
		})
	}
}

// TestSplitFunc checks that a "name(inner)" expression is split into its name
// and inner content, and that inputs without that shape are rejected.
func TestSplitFunc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		in        string
		wantName  string
		wantInner string
		wantOK    bool
	}{{
		name:      "single arg",
		in:        "wpkh(inner)",
		wantName:  "wpkh",
		wantInner: "inner",
		wantOK:    true,
	}, {
		name:      "multiple args",
		in:        "tr(a,b)",
		wantName:  "tr",
		wantInner: "a,b",
		wantOK:    true,
	}, {
		name:      "empty inner",
		in:        "pk()",
		wantName:  "pk",
		wantInner: "",
		wantOK:    true,
	}, {
		name:   "no parentheses",
		in:     "0",
		wantOK: false,
	}, {
		name:   "no name",
		in:     "()",
		wantOK: false,
	}, {
		name:   "unterminated",
		in:     "wpkh(inner",
		wantOK: false,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			name, inner, ok := splitFunc(tc.in)
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.Equal(t, tc.wantName, name)
				require.Equal(t, tc.wantInner, inner)
			}
		})
	}
}

// TestStripChecksum checks that a valid "#checksum" suffix is accepted and
// stripped, a missing suffix is passed through, and a wrong checksum is
// rejected.
func TestStripChecksum(t *testing.T) {
	t.Parallel()

	body := "wpkh(" + basicTestXpub + "/*)"
	withChecksum := body + "#" + descriptorChecksum(body)

	got, err := stripChecksum(withChecksum)
	require.NoError(t, err)
	require.Equal(t, body, got)

	got, err = stripChecksum(body)
	require.NoError(t, err)
	require.Equal(t, body, got)

	_, err = stripChecksum(body + "#deadbeef")
	require.Error(t, err)
}

// TestForEachLeaf checks that a taproot script tree is walked left to right,
// with the correct per-leaf depth, that a single leaf has depth zero, and that
// an error from the callback stops the walk.
func TestForEachLeaf(t *testing.T) {
	t.Parallel()

	leaf := func(name string) *node {
		return &node{kind: nodeMs, msExpr: name}
	}

	// tr(key, {A, {B, C}}): A is at depth 1, B and C at depth 2.
	tree := &tapTree{
		left: &tapTree{leaf: leaf("A")},
		right: &tapTree{
			left:  &tapTree{leaf: leaf("B")},
			right: &tapTree{leaf: leaf("C")},
		},
	}

	type visit struct {
		expr  string
		depth int
	}
	collect := func(tt *tapTree) []visit {
		var visits []visit
		err := tt.forEachLeaf(0, func(l *node, depth int) error {
			visits = append(visits, visit{l.msExpr, depth})
			return nil
		})
		require.NoError(t, err)
		return visits
	}

	require.Equal(t, []visit{{"A", 1}, {"B", 2}, {"C", 2}}, collect(tree))

	// A single leaf is at depth zero.
	require.Equal(t, []visit{{"A", 0}},
		collect(&tapTree{leaf: leaf("A")}))

	// An error from the callback halts the walk and propagates.
	sentinel := errors.New("stop")
	err := tree.forEachLeaf(0, func(*node, int) error {
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
}

// TestNewDescriptorRejectsInsaneMiniscript checks that the miniscript sanity
// checks reach the descriptor layer: descriptors whose script is provably
// unspendable, malleable by third parties, over a consensus limit or broken by
// mixed time locks used to be accepted here and produced a perfectly
// ordinary-looking address.
func TestNewDescriptorRejectsInsaneMiniscript(t *testing.T) {
	t.Parallel()

	const (
		key1 = "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f" +
			"2815b16f81798"
		key2 = "02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7aba" +
			"c09b95c709ee5"
		xOnly1 = "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f" +
			"2815b16f81798"
		xOnly2 = "c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7aba" +
			"c09b95c709ee5"
	)

	tests := []struct {
		name   string
		desc   string
		errStr string
	}{{
		// A type-V witness script cannot leave a true value on the
		// stack, so coins sent to this address are unspendable.
		name:   "unspendable wsh",
		desc:   "wsh(v:pk(" + key1 + "))",
		errStr: "type B, but is type V",
	}, {
		name:   "malleable wsh",
		desc:   "wsh(or_b(pk(" + key1 + "),al:pk(" + key2 + ")))",
		errStr: "malleable",
	}, {
		name: "timelock mixing wsh",
		desc: "wsh(and_v(v:pk(" + key1 + "),and_v(v:older(10)," +
			"older(4194305))))",
		errStr: "combination of height-based and time-based",
	}, {
		// Anyone can spend this tapscript leaf once the time lock
		// expires.
		name:   "tapscript leaf without signature",
		desc:   "tr(" + xOnly1 + ",older(144))",
		errStr: "does not need signature",
	}, {
		name: "malleable tapscript leaf",
		desc: "tr(" + xOnly1 + ",or_b(pk(" + xOnly2 + "),al:pk(" +
			xOnly1 + ")))",
		errStr: "malleable",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewDescriptor(tc.desc)
			require.ErrorContains(t, err, tc.errStr)
		})
	}
}
