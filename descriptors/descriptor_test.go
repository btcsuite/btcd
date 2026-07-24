package descriptors

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/stretchr/testify/require"
)

// networkParams maps a test-vector network name to its chaincfg parameters.
func networkParams(t *testing.T, network string) *chaincfg.Params {
	switch network {
	case "mainnet":
		return &chaincfg.MainNetParams

	case "testnet":
		return &chaincfg.TestNet3Params

	case "regtest":
		return &chaincfg.RegressionNetParams

	default:
		require.FailNowf(t, "unknown network", "network: %s", network)
		return nil
	}
}

type testAddress struct {
	Network         string `json:"network"`
	MultipathIndex  uint32 `json:"multipathIndex"`
	DerivationIndex uint32 `json:"derivationIndex"`
	Address         string `json:"address"`
	ExpectErr       string `json:"expectErr"`
}

type derivationTestVector struct {
	Name         string         `json:"name"`
	Descriptor   string         `json:"descriptor"`
	HasChecksum  bool           `json:"hasChecksum"`
	Checksum     string         `json:"checksum"`
	NumMultipath uint32         `json:"numMultipath"`
	ExpectErr    string         `json:"expectErr"`
	Addresses    []*testAddress `json:"addresses"`
}

// TestDerivationVectors runs the derivation test vectors copied from the
// descriptors-go reference implementation.
func TestDerivationVectors(t *testing.T) {
	t.Parallel()

	file, err := os.Open(filepath.Join("testdata", "derivation.json"))
	require.NoError(t, err)
	defer file.Close()

	var vectors []*derivationTestVector
	require.NoError(t, json.NewDecoder(file).Decode(&vectors))

	for _, vector := range vectors {
		vector := vector
		t.Run(vector.Name, func(t *testing.T) {
			t.Parallel()

			descriptor, err := NewDescriptor(vector.Descriptor)
			if vector.ExpectErr != "" {
				require.ErrorContains(t, err, vector.ExpectErr)
				return
			}
			require.NoError(t, err)

			require.EqualValues(
				t, vector.NumMultipath,
				descriptor.MultipathLen(),
			)

			expected := vector.Descriptor
			if !vector.HasChecksum {
				expected += vector.Checksum
			}
			require.Equal(t, expected, descriptor.String())

			for _, addr := range vector.Addresses {
				params := networkParams(t, addr.Network)
				got, err := descriptor.AddressAt(
					params, addr.MultipathIndex,
					addr.DerivationIndex,
				)
				if addr.ExpectErr != "" {
					require.ErrorContains(
						t, err, addr.ExpectErr,
					)
					continue
				}

				require.NoError(t, err)
				require.Equal(t, addr.Address, got)
			}
		})
	}
}

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
