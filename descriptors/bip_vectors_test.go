package descriptors

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/stretchr/testify/require"
)

// bipVector is one test vector transcribed from the descriptor BIPs.
type bipVector struct {
	// BIP is the number of the BIP the vector comes from.
	BIP int `json:"bip"`

	// Kind is what the vector exercises: "key" for a key expression
	// (wrapped in a pk() so it can be parsed), "descriptor" for a
	// descriptor with optional output scripts, "multipath" for a multipath
	// descriptor with the descriptors it expands into, "multi_script" for a
	// descriptor that produces more than one script per index, and
	// "checksum" for the checksum and character set vectors.
	Kind string `json:"kind"`

	// Label is the description the BIP gives the vector.
	Label string `json:"label"`

	// Desc is the descriptor to parse.
	Desc string `json:"desc"`

	// Expr is the bare key expression of a "key" vector, before it was
	// wrapped in a pk().
	Expr string `json:"expr,omitempty"`

	// Valid is whether the BIP lists the vector as valid.
	Valid bool `json:"valid"`

	// Scripts holds the output scripts the BIP lists for the vector, one
	// per derivation index starting at zero.
	Scripts []string `json:"scripts,omitempty"`

	// Expansions holds the single-path descriptors a multipath vector
	// expands into, in order.
	Expansions []string `json:"expansions,omitempty"`

	// Divergence explains why this package deliberately disagrees with the
	// BIP's verdict, in which case the descriptor has to parse.
	Divergence string `json:"divergence,omitempty"`

	// Note records an observation about the vector itself, such as a typo
	// in the BIP text that had to be corrected to transcribe it.
	Note string `json:"note,omitempty"`
}

// loadBIPVectors reads the test vectors transcribed from the descriptor BIPs.
func loadBIPVectors(t *testing.T) []bipVector {
	t.Helper()

	file, err := os.Open(filepath.Join("testdata", "bip_vectors.json"))
	require.NoError(t, err)
	defer func() {
		require.NoError(t, file.Close())
	}()

	var vectors []bipVector
	require.NoError(t, json.NewDecoder(file).Decode(&vectors))
	require.NotEmpty(t, vectors)

	return vectors
}

// outputScriptAt returns the output script of a descriptor at the given
// multipath and derivation index: the script of its address, or its script code
// if it is a bare descriptor, which has no address.
func outputScriptAt(d *Descriptor, mp, idx uint32) ([]byte, error) {
	addr, err := d.AddressAt(&chaincfg.MainNetParams, mp, idx)
	if err != nil {
		return d.ScriptCodeAt(mp, idx)
	}

	decoded, err := address.DecodeAddress(addr, &chaincfg.MainNetParams)
	if err != nil {
		return nil, err
	}

	return txscript.PayToAddrScript(decoded)
}

// TestBIPVectors runs the test vectors of the descriptor BIPs: BIP380 (general
// operation, key expressions, checksum), BIP381 (pk/pkh/sh), BIP382 (wpkh/wsh),
// BIP383 (multi/sortedmulti), BIP384 (combo), BIP385 (raw/addr), BIP386 (tr),
// BIP387 (multi_a/sortedmulti_a) and BIP389 (multipath).
//
// A vector marked as using an unsupported feature has to be rejected: that
// keeps the gaps of this package explicit and turns any of them into a failing
// test as soon as it is closed.
func TestBIPVectors(t *testing.T) {
	t.Parallel()

	counts := map[string]int{}
	for _, vector := range loadBIPVectors(t) {
		name := fmt.Sprintf("bip%d/%s", vector.BIP, vector.Label)
		counts[fmt.Sprintf("bip%d", vector.BIP)]++

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			runBIPVector(t, vector)
		})
	}

	t.Logf("ran BIP vectors: %v", counts)
}

// runBIPVector runs a single BIP test vector.
func runBIPVector(t *testing.T, vector bipVector) {
	t.Helper()

	// The checksum vectors exercise the checksum and character set only,
	// since the raw() descriptor they are written with is not supported.
	if vector.Kind == "checksum" {
		_, err := stripChecksum(vector.Desc)
		if vector.Valid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}

		return
	}

	d, err := NewDescriptor(vector.Desc)

	switch {
	// A vector using a feature this package does not implement has to be
	// rejected, which keeps the gap explicit: implementing the feature
	// makes this fail until the case is removed from unsupportedFeature.
	case unsupportedFeature(vector.Desc) != "":
		require.Errorf(
			t, err, "vector needs unsupported feature %q but was "+
				"accepted: %s", unsupportedFeature(vector.Desc),
			vector.Desc,
		)

		return

	// A vector this package deliberately treats differently from the BIP.
	case vector.Divergence != "":
		require.NoErrorf(t, err, "%s: %s", vector.Divergence,
			vector.Desc)

		return

	case !vector.Valid:
		require.Errorf(t, err, "invalid vector accepted: %s",
			vector.Desc)

		return
	}

	require.NoErrorf(t, err, "valid vector rejected: %s", vector.Desc)

	// The output scripts the BIP lists, one per derivation index.
	for idx, want := range vector.Scripts {
		script, err := outputScriptAt(d, 0, uint32(idx))
		require.NoErrorf(t, err, "output script at %d for %s", idx,
			vector.Desc)
		require.Equalf(
			t, want, hex.EncodeToString(script),
			"output script at %d for %s", idx, vector.Desc,
		)
	}

	// A multipath descriptor has to produce the same scripts as the
	// single-path descriptors it expands into.
	if len(vector.Expansions) > 0 {
		require.Equalf(
			t, len(vector.Expansions), d.MultipathLen(),
			"multipath length of %s", vector.Desc,
		)

		for mp, expansion := range vector.Expansions {
			single, err := NewDescriptor(expansion)
			require.NoErrorf(t, err, "expansion %s", expansion)

			for idx := range uint32(2) {
				want, wantErr := outputScriptAt(single, 0, idx)
				got, gotErr := outputScriptAt(
					d, uint32(mp), idx,
				)

				if wantErr != nil {
					require.Errorf(
						t, gotErr, "expansion %d of %s",
						mp, vector.Desc,
					)

					continue
				}

				require.NoErrorf(
					t, gotErr, "expansion %d of %s", mp,
					vector.Desc,
				)
				require.Equalf(
					t, hex.EncodeToString(want),
					hex.EncodeToString(got),
					"expansion %d at index %d of %s", mp,
					idx, vector.Desc,
				)
			}
		}
	}
}

// unsupportedFeature returns a description of the feature a descriptor needs
// that this package does not implement, or the empty string if it needs none. A
// vector that needs one has to be rejected at parse time.
//
// This is the explicit list of gaps against the descriptor BIPs. Closing one
// makes TestBIPVectors fail until its case is removed here, so the list cannot
// go stale.
func unsupportedFeature(desc string) string {
	switch {
	// BIP384: a combo() descriptor stands for two or four output scripts,
	// which the single-script API of this package cannot represent.
	case strings.HasPrefix(desc, "combo("):
		return "combo() (BIP384)"

	// BIP385: raw() and addr() wrap a script or an address that has no keys
	// and no satisfaction, so most of this package's API is meaningless for
	// them.
	case strings.HasPrefix(desc, "raw("), strings.HasPrefix(desc, "addr("),
		strings.Contains(desc, "(raw("),
		strings.Contains(desc, "(addr("):

		return "raw() and addr() (BIP385)"

	}

	return ""
}
