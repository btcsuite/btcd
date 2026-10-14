package descriptors

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/descriptors/miniscript"
	"github.com/stretchr/testify/require"
)

const (
	// msDescriptor is a wsh(miniscript) descriptor with a wildcard, i.e.
	// the case where deriving an address must build the inner miniscript at
	// the requested index. This isolates the per-derivation miniscript cost
	// that the AST cache targets.
	msDescriptor = "wsh(or_d(pk([e81a5744/48'/0'/0'/2']xpub6Duv8Gj9gZeA3s" +
		"Uo5nUMPEv6FZ81GHn3feyaUej5KqcjPKsYLww4xBX4MmYZUPX5NqzaVJWYdY" +
		"ZwGLECtgQruG4FkZMh566RkfUT2pbzsEg/*),and_v(v:pk([3c157b79/48" +
		"'/0'/0'/2']xpub6DdSN9RNZi3eDjhZWA8PJ5mSuWgfmPdBduXWzSP91Y3Gx" +
		"KWNwkjyc5mF9FcpTFymUh9C4Bar45b6rWv6Y5kSbi9yJDjuJUDzQSWUh3ijz" +
		"XP/*),older(52560))))"
)

// BenchmarkNewDescriptor measures parsing every descriptor in the corpus once
// per iteration. This is the "loading" path a wallet hits when it ingests a set
// of descriptors.
func BenchmarkNewDescriptor(b *testing.B) {
	descs := loadCorpus(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		for _, s := range descs {
			d, err := NewDescriptor(s)
			if err != nil {
				require.NoErrorf(b, err, "parse %q", s)
			}
			_ = d
		}
	}
}

// BenchmarkAddressAtMiniscript derives addresses at increasing indices from a
// single wsh(miniscript) descriptor. Each derivation used to re-parse the whole
// miniscript expression; it now clones the AST cached at construction time.
func BenchmarkAddressAtMiniscript(b *testing.B) {
	d, err := NewDescriptor(msDescriptor)
	if err != nil {
		require.NoError(b, err, "parse")
	}
	params := &chaincfg.MainNetParams

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		if _, err := d.AddressAt(params, 0, uint32(i)); err != nil {
			require.NoError(b, err, "derive")
		}
	}
}

// BenchmarkMiniscriptParseVsClone compares parsing a miniscript expression from
// scratch against cloning an already-parsed AST, which is what the derivation
// path does per address. It quantifies the saving the cache provides.
func BenchmarkMiniscriptParseVsClone(b *testing.B) {
	d, err := NewDescriptor(msDescriptor)
	if err != nil {
		require.NoError(b, err, "parse")
	}

	// Reach the inner miniscript node of the wsh wrapper.
	inner := d.root.sub

	b.Run("Parse", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			ast, err := miniscript.Parse(inner.msExpr, inner.msCtx)
			if err != nil {
				require.NoError(b, err)
			}
			_ = ast
		}
	})

	b.Run("Clone", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = inner.msAST.Clone()
		}
	})
}

// BenchmarkAddressAt measures deriving a single address from every corpus
// descriptor that supports one. The descriptor is parsed once up front so the
// benchmark isolates the per-derivation cost.
func BenchmarkAddressAt(b *testing.B) {
	raw := loadCorpus(b)

	params := &chaincfg.MainNetParams
	var descs []*Descriptor
	for _, s := range raw {
		d, err := NewDescriptor(s)
		if err != nil {
			require.NoErrorf(b, err, "parse %q", s)
		}

		// Only keep descriptors that can produce an address at index 0.
		if _, err := d.AddressAt(params, 0, 0); err != nil {
			continue
		}
		descs = append(descs, d)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		for _, d := range descs {
			if _, err := d.AddressAt(params, 0, 0); err != nil {
				require.NoError(b, err, "derive")
			}
		}
	}
}
