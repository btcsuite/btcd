package descriptors

import (
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
