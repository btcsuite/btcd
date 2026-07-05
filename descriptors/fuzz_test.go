package descriptors

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/stretchr/testify/require"
)

// maxFuzzInput bounds the size of a fuzz input. Descriptor parsing recurses
// into miniscript, whose threshold passes are exponential in the number of
// thresh sub expressions, so this keeps individual executions fast while
// leaving room to reach every descriptor and script shape.
const maxFuzzInput = 4096

// gPointX is the x coordinate of the secp256k1 generator, used to build seed
// descriptors with valid raw keys (which, unlike the xpub seeds, survive
// mutation without breaking base58 decoding).
const gPointX = "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f28" +
	"15b16f81798"

// hardcodedDescriptorSeeds keep the seed set representative even if the corpus
// file is unavailable, and add raw-key forms that mutate more gracefully than
// the long xpub corpus entries.
var hardcodedDescriptorSeeds = []string{
	"pk(" + basicTestXpub + ")",
	"pkh(" + basicTestXpub + "/*)",
	"wpkh(" + basicTestXpub + "/<0;1>/*)",
	"sh(wpkh(" + basicTestXpub + "/*))",
	"wsh(pk(" + basicTestXpub + "/*))",
	"sh(wsh(pkh(" + basicTestXpub + "/*)))",
	"wsh(multi(1,02" + gPointX + "))",
	"tr(" + gPointX + ")",
	"tr(" + gPointX + ",pk(" + gPointX + "))",
	"tr(" + gPointX + ",{pk(" + gPointX + "),pk(" + gPointX + ")})",
}

// FuzzNewDescriptor fuzzes descriptor parsing and, on every successful parse,
// exercises the deep downstream methods (type classification, lifting, address
// and script-code derivation, weight estimation and planning). It checks the
// no-crash property throughout, plus the invariant that the descriptor's string
// form is a stable, re-parseable fixed point.
func FuzzNewDescriptor(f *testing.F) {
	addDescriptorSeeds(f, filepath.Join(
		"testdata", "descriptors_corpus.txt",
	))
	for _, seed := range hardcodedDescriptorSeeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, desc string) {
		if len(desc) > maxFuzzInput {
			return
		}

		d, err := NewDescriptor(desc)
		if err != nil {
			return
		}
		exerciseDescriptor(t, d)
	})
}

// exerciseDescriptor drives the deep operations of a successfully parsed
// descriptor, asserting only invariants that must hold for any valid one.
func exerciseDescriptor(t *testing.T, d *Descriptor) {
	// The string form must be a stable fixed point: re-parsing it must
	// succeed and reproduce exactly the same string (the checksum is
	// recomputed on each round).
	s := d.String()
	reparsed, err := NewDescriptor(s)
	require.NoErrorf(t, err, "re-parsing own string %q failed", s)
	require.Equal(t, s, reparsed.String(),
		"descriptor string is not a stable fixed point")

	// The remaining methods must never panic on a valid descriptor.
	_ = d.DescType()
	_ = d.Keys()
	_, _ = d.Lift()
	_, _ = d.MaxWeightToSatisfy()

	multipath := d.MultipathLen()
	require.GreaterOrEqualf(t, multipath, 1,
		"multipath length must be at least 1 for %q", s)

	// Derive an address and script code for a bounded sample of the
	// multipath sub-descriptors; both may legitimately error, but must not
	// panic.
	params := &chaincfg.RegressionNetParams
	for mp := uint32(0); int(mp) < multipath && mp < 4; mp++ {
		_, _ = d.AddressAt(params, mp, 0)
		_, _ = d.ScriptCodeAt(mp, 0)
	}

	// Planning with everything available drives the plan builder and its
	// satisfaction machinery.
	_, _ = d.PlanAt(0, 0, Assets{
		LookupEcdsaSig: func(string) bool {
			return true
		},
		LookupTapKeySpendSig: func(string) (uint32, bool) {
			return 64, true
		},
		LookupTapLeafScriptSig: func(string, string) (uint32, bool) {
			return 64, true
		},
	})
}

// addDescriptorSeeds adds each non-empty, non-comment line of the given file as
// a fuzz seed. A missing file is ignored so the fuzzer stays usable without the
// full corpus.
func addDescriptorSeeds(f *testing.F, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f.Add(line)
	}
}
