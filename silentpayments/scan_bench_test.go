package silentpayments

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
)

// The scanning hot path derives, per served transaction tweak, the
// candidate output keys for the wallet's addresses. These benchmarks break
// that path into its primitives, so the cost profile can be compared
// between platforms (in particular native vs. GOOS=js GOARCH=wasm, where a
// browser scanner runs):
//
//	GOOS=js GOARCH=wasm go test -bench BenchmarkScan -run - \
//	    -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec"
var benchmarkSink any

// benchmarkKeys returns deterministic key material for the benchmarks.
func benchmarkKeys(b *testing.B) (*btcec.PrivateKey, []ScanAddress,
	*btcec.PublicKey) {

	b.Helper()

	newKey := func(fill byte) *btcec.PrivateKey {
		var keyBytes [32]byte
		for i := range keyBytes {
			keyBytes[i] = fill
		}
		privKey, _ := btcec.PrivKeyFromBytes(keyBytes[:])

		return privKey
	}

	scanPriv := newKey(0x01)
	spendPriv := newKey(0x02)
	tweakPriv := newKey(0x03)

	scanPub := *scanPriv.PubKey()
	spendPub := *spendPriv.PubKey()
	baseAddr := NewAddress(MainNetHRP, scanPub, spendPub, nil)
	changeAddr := NewAddress(
		MainNetHRP, scanPub, spendPub, LabelTweak(scanPriv, 0),
	)

	addresses := []ScanAddress{
		NewScanAddress(*baseAddr, *scanPriv),
		NewScanAddress(*changeAddr, *scanPriv),
	}

	return scanPriv, addresses, tweakPriv.PubKey()
}

// BenchmarkScanParseTweak measures decompressing a served 33-byte tweak
// into a point (one modular square root).
func BenchmarkScanParseTweak(b *testing.B) {
	_, _, tweak := benchmarkKeys(b)
	raw := tweak.SerializeCompressed()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchmarkSink, _ = btcec.ParsePubKey(raw)
	}
}

// BenchmarkScanECDH measures the shared secret derivation, the one
// variable-point scalar multiplication per served tweak.
func BenchmarkScanECDH(b *testing.B) {
	scanPriv, _, tweak := benchmarkKeys(b)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchmarkSink = ScalarMult(scanPriv.Key, tweak)
	}
}

// BenchmarkScanBaseMultAdd measures deriving one candidate output key from
// a tweak scalar (t_k*G plus the spend key), including the affine
// conversion.
func BenchmarkScanBaseMultAdd(b *testing.B) {
	scanPriv, addresses, _ := benchmarkKeys(b)
	spendKey := addresses[0].LabelTweakedSpendKey

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchmarkSink = ScalarBaseMultAdd(scanPriv.Key, &spendKey)
	}
}

// BenchmarkScanTaggedHash measures the t_k tagged hash.
func BenchmarkScanTaggedHash(b *testing.B) {
	_, _, tweak := benchmarkKeys(b)
	payload := make([]byte, 33+4)
	copy(payload, tweak.SerializeCompressed())

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchmarkSink = chainhash.TaggedHash(
			TagBIP0352SharedSecret, payload,
		)
	}
}

// BenchmarkScanFilterKeys measures the full per-tweak filter candidate
// derivation for the common base+change address pair, the hot loop of a
// light client scanning tweak data.
func BenchmarkScanFilterKeys(b *testing.B) {
	_, addresses, tweak := benchmarkKeys(b)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchmarkSink, _ = TransactionOutputKeysForFilter(
			*tweak, addresses,
		)
	}
}

// BenchmarkScanFilterKeysBatch measures the batched filter candidate
// derivation, normalized per tweak for comparison with
// BenchmarkScanFilterKeys.
func BenchmarkScanFilterKeysBatch(b *testing.B) {
	_, addresses, tweak := benchmarkKeys(b)

	// One batch roughly the size of a spam block's tweak list.
	const batchSize = 256
	tweaks := make([]*btcec.PublicKey, batchSize)
	for i := range tweaks {
		tweaks[i] = tweak
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkSink, _ = TransactionOutputKeysForFilterBatch(
			addresses, tweaks,
		)
	}
	b.StopTimer()
	b.ReportMetric(
		float64(b.Elapsed().Nanoseconds())/float64(b.N*batchSize),
		"ns/tweak",
	)
}
