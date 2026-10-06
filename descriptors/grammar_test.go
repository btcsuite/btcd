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

// TestUnsatisfiableRejected checks that a descriptor whose script can never be
// satisfied is rejected, and that NewDescriptorInsane still parses it.
//
// Such a descriptor used to parse and derive an address, so a receive flow
// that only called NewDescriptor and AddressAt handed out an address whose
// coins nobody can ever move. The sanity check did not cover it because the
// unsatisfiable fragment 0 carries the "s" and "f" properties vacuously, which
// is why wsh(1) was rejected for needing no signature while wsh(0) was not.
func TestUnsatisfiableRejected(t *testing.T) {
	t.Parallel()

	const key = "03d04e74a4a87f872d20c9e1a7195379364e8e3596926bfa346d0" +
		"ea4bfee7e3e28"
	xOnly := key[2:]

	for _, desc := range []string{
		"wsh(0)",
		"wsh(and_v(v:pk(" + key + "),0))",
		"sh(wsh(0))",
		"wsh(and_b(pk(" + key + "),a:0))",
		"sh(and_v(v:pk(" + key + "),0))",

		// A dead tap leaf does not lock up the coins, since the key
		// path still spends them, but Core rejects the descriptor all
		// the same.
		"tr(" + xOnly + ",0)",
	} {

		t.Run(desc, func(t *testing.T) {
			t.Parallel()

			_, err := NewDescriptor(desc)
			require.ErrorContains(t, err, "cannot be satisfied")

			// The insane parse accepts it, so an expression that is
			// known not to be sane can still be inspected.
			d, err := NewDescriptorInsane(desc)
			require.NoError(t, err)
			require.Equal(t, desc, stripChecksumOrFail(t, d))
		})
	}

	// A branch that can never be taken is not the same thing: the
	// expression as a whole still has a satisfaction, so it stays valid.
	for _, desc := range []string{
		"wsh(or_i(0,pk(" + key + ")))",
		"wsh(thresh(1,pk(" + key + "),a:0))",
	} {

		t.Run(desc, func(t *testing.T) {
			t.Parallel()

			_, err := NewDescriptor(desc)
			require.NoError(t, err)
		})
	}
}

// stripChecksumOrFail returns the canonical string of the descriptor without
// the checksum String appends to it.
func stripChecksumOrFail(t *testing.T, d *Descriptor) string {
	t.Helper()

	body, err := stripChecksum(d.String())
	require.NoError(t, err)

	return body
}
