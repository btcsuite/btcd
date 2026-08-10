package descriptors

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBareMiniscriptRejected checks that a compound miniscript expression is
// rejected at the top level of a descriptor. BIP379 defines miniscript for the
// wsh() and tr() contexts, to which this package adds sh(), and Core rejects it
// anywhere else as well. A bare miniscript output script would not be one of
// the standard templates, so a standard transaction could not even pay to it.
//
// It used to parse and be analyzed in the P2WSH context, i.e. with the resource
// limits of a witness script rather than the ones a bare script is bound by.
func TestBareMiniscriptRejected(t *testing.T) {
	t.Parallel()

	const expr = "and_v(v:pk(" + testXpub1 + "),older(2))"

	_, err := NewDescriptor(expr)
	require.ErrorContains(
		t, err,
		"a miniscript expression cannot be used at the top level of a "+
			"descriptor",
	)

	// The same expression is accepted in the positions that do have a
	// miniscript context (a tr() leaf is covered by the taproot tests), and
	// the bare descriptor expressions of BIP381 and BIP383 keep working at
	// the top level.
	for _, desc := range []string{
		"wsh(" + expr + ")",
		"sh(" + expr + ")",
		"pk(" + testXpub1 + ")",
		"pkh(" + testXpub1 + ")",
		"multi(1," + testXpub1 + ")",
		"sortedmulti(2," + testXpub1 + "," + testXpub2 + ")",
	} {

		_, err := NewDescriptor(desc)
		require.NoErrorf(t, err, "descriptor %s", desc)
	}
}

// TestMultiThresholdSyntax checks that the threshold of a multi() or
// sortedmulti() has to be a plain decimal number. strconv.Atoi accepts a
// leading sign, so multi(+1,KEY) used to parse and round-trip with a valid
// checksum, blessing a descriptor that Bitcoin Core and the miniscript parser
// both reject.
func TestMultiThresholdSyntax(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"multi", "sortedmulti"} {
		for _, threshold := range []string{
			"+1", "-1", "1 ", " 1", "1.0", "0x1", "1_0", "", "one",
		} {

			desc := kind + "(" + threshold + "," + testXpub1 + ")"
			t.Run(desc, func(t *testing.T) {
				t.Parallel()

				_, err := NewDescriptor(desc)
				require.ErrorContains(
					t, err, "invalid "+kind+" threshold",
				)
			})
		}

		// A digit outside of ASCII does not even get as far as the
		// threshold: the descriptor character set of BIP380 rejects it.
		nonASCII := kind + "(０," + testXpub1 + ")"
		t.Run(nonASCII, func(t *testing.T) {
			t.Parallel()

			_, err := NewDescriptor(nonASCII)
			require.ErrorContains(
				t, err,
				"descriptor contains invalid characters",
			)
		})

		// The canonical spelling of the same threshold is accepted.
		canonical := kind + "(1," + testXpub1 + ")"
		t.Run(canonical, func(t *testing.T) {
			t.Parallel()

			_, err := NewDescriptor(canonical)
			require.NoError(t, err)
		})

		// A syntactically valid threshold that the key count does not
		// allow is still reported as the range error it is.
		outOfRange := kind + "(2," + testXpub1 + ")"
		t.Run(outOfRange, func(t *testing.T) {
			t.Parallel()

			_, err := NewDescriptor(outOfRange)
			require.ErrorContains(
				t, err, "threshold 2 out of range for 1 keys",
			)
		})
	}
}
