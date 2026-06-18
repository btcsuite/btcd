package psbt

import (
	"bytes"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestSilentPaymentsPacket tests the serialization and deserialization of
// SilentPaymentShare data in a PSBT packet.
func TestSilentPaymentsPacket(t *testing.T) {
	randomPrivKey1, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	randomPrivKey2, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	randomPrivKey3, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	randomData1 := make([]byte, 64)
	_, err = crand.Read(randomData1)
	require.NoError(t, err)
	randomData2 := make([]byte, 64)
	_, err = crand.Read(randomData2)
	require.NoError(t, err)

	share1 := SilentPaymentShare{
		ScanKey: randomPrivKey1.PubKey().SerializeCompressed(),
		Share:   randomPrivKey2.PubKey().SerializeCompressed(),
	}
	share2 := SilentPaymentShare{
		ScanKey: randomPrivKey3.PubKey().SerializeCompressed(),
		Share:   randomPrivKey2.PubKey().SerializeCompressed(),
	}

	proof1 := SilentPaymentDLEQ{
		ScanKey: share1.ScanKey,
		Proof:   randomData1,
	}
	proof2 := SilentPaymentDLEQ{
		ScanKey: share2.ScanKey,
		Proof:   randomData2,
	}

	info := SilentPaymentInfo{
		ScanKey:  share1.ScanKey,
		SpendKey: share2.ScanKey,
	}
	label := uint32(1234567890)

	// Start with a valid packet from our test vectors (number 9, with the
	// PSBT_GLOBAL_XPUB field set).
	originalBase64 := validPsbtBase64[8]

	// Decode the packet.
	testPacket, err := NewFromRawBytes(
		strings.NewReader(originalBase64), true,
	)
	require.NoError(t, err)

	// We serialize our packet before making any changes to it, so we can
	// compare it to the one we get back after adding our test data.
	originalBytes := serialize(t, testPacket)

	// Now add our test data to the packet.
	testPacket.SilentPaymentShares = append(
		testPacket.SilentPaymentShares, share1,
	)
	testPacket.SilentPaymentDLEQs = append(
		testPacket.SilentPaymentDLEQs, proof1,
	)
	testPacket.Inputs[0].SilentPaymentShares = append(
		testPacket.Inputs[0].SilentPaymentShares, share2,
	)
	testPacket.Inputs[0].SilentPaymentDLEQs = append(
		testPacket.Inputs[0].SilentPaymentDLEQs, proof2,
	)
	testPacket.Outputs[0].SilentPaymentInfo = &info
	testPacket.Outputs[0].SilentPaymentLabel = &label

	// And do another full encoding/decoding round trip.
	newBytes := serialize(t, testPacket)
	newPacket, err := NewFromRawBytes(bytes.NewReader(newBytes), false)
	require.NoError(t, err)

	// Check that the packet we got back is the same as the one we got after
	// adding test data.
	require.Equal(t, testPacket, newPacket)

	// We want to make sure that our new data is much larger than the
	// original packet.
	require.Greater(t, len(newBytes), len(originalBytes))
}

func serialize(t *testing.T, packet *Packet) []byte {
	var buf bytes.Buffer
	err := packet.Serialize(&buf)
	require.NoError(t, err)
	return buf.Bytes()
}

// TestReadSilentPaymentFieldsMalformed verifies that the silent payment field
// readers reject malformed inputs: wrong lengths and (for the pubkey-bearing
// fields) values that aren't valid points on the curve. The global and
// per-input shares/DLEQs share these readers, so this covers both scopes.
func TestReadSilentPaymentFieldsMalformed(t *testing.T) {
	t.Parallel()

	priv, _ := btcec.PrivKeyFromBytes([]byte{0x01})
	validKey := priv.PubKey().SerializeCompressed()
	validProof := make([]byte, 64)
	validInfo := append(append([]byte{}, validKey...), validKey...)

	// A correctly-sized but off-curve compressed public key (x >= p).
	badKey := append([]byte{0x02}, bytes.Repeat([]byte{0xff}, 32)...)

	t.Run("share", func(t *testing.T) {
		t.Parallel()

		_, err := ReadSilentPaymentShare(validKey, validKey)
		require.NoError(t, err)

		_, err = ReadSilentPaymentShare(validKey[:32], validKey)
		require.ErrorIs(t, err, ErrInvalidKeyData)
		_, err = ReadSilentPaymentShare(badKey, validKey)
		require.ErrorIs(t, err, ErrInvalidKeyData)

		_, err = ReadSilentPaymentShare(validKey, validKey[:32])
		require.ErrorIs(t, err, ErrInvalidPsbtFormat)
		_, err = ReadSilentPaymentShare(validKey, badKey)
		require.ErrorIs(t, err, ErrInvalidPsbtFormat)
	})

	t.Run("dleq", func(t *testing.T) {
		t.Parallel()

		_, err := ReadSilentPaymentDLEQ(validKey, validProof)
		require.NoError(t, err)

		_, err = ReadSilentPaymentDLEQ(validKey[:32], validProof)
		require.ErrorIs(t, err, ErrInvalidKeyData)
		_, err = ReadSilentPaymentDLEQ(badKey, validProof)
		require.ErrorIs(t, err, ErrInvalidKeyData)

		_, err = ReadSilentPaymentDLEQ(validKey, validProof[:63])
		require.ErrorIs(t, err, ErrInvalidPsbtFormat)
	})

	t.Run("info", func(t *testing.T) {
		t.Parallel()

		_, err := ReadSilentPaymentInfo(validInfo)
		require.NoError(t, err)

		_, err = ReadSilentPaymentInfo(validInfo[:65])
		require.ErrorIs(t, err, ErrInvalidPsbtFormat)

		badScan := append(append([]byte{}, badKey...), validKey...)
		badSpend := append(append([]byte{}, validKey...), badKey...)
		_, err = ReadSilentPaymentInfo(badScan)
		require.ErrorIs(t, err, ErrInvalidPsbtFormat)
		_, err = ReadSilentPaymentInfo(badSpend)
		require.ErrorIs(t, err, ErrInvalidPsbtFormat)
	})

	t.Run("label", func(t *testing.T) {
		t.Parallel()

		label, err := ReadSilentPaymentLabel([]byte{1, 0, 0, 0})
		require.NoError(t, err)
		require.Equal(t, uint32(1), label)

		_, err = ReadSilentPaymentLabel([]byte{0, 0, 0})
		require.ErrorIs(t, err, ErrInvalidPsbtFormat)
	})
}

// TestSilentPaymentDuplicateKeys verifies that decoding a PSBT with two silent
// payment shares or DLEQ proofs that share the same scan key is rejected, at
// both the global and per-input scopes.
func TestSilentPaymentDuplicateKeys(t *testing.T) {
	t.Parallel()

	priv, _ := btcec.PrivKeyFromBytes([]byte{0x01})
	scanKey := priv.PubKey().SerializeCompressed()
	share := SilentPaymentShare{ScanKey: scanKey, Share: scanKey}
	dleq := SilentPaymentDLEQ{ScanKey: scanKey, Proof: make([]byte, 64)}

	dupShares := []SilentPaymentShare{share, share}
	dupDLEQs := []SilentPaymentDLEQ{dleq, dleq}

	newPacket := func(t *testing.T) *Packet {
		t.Helper()

		pkt, err := NewFromRawBytes(
			strings.NewReader(validPsbtBase64[8]), true,
		)
		require.NoError(t, err)

		return pkt
	}

	// assertDupErr serializes the packet and asserts that decoding it again
	// fails with ErrDuplicateKey.
	assertDupErr := func(t *testing.T, pkt *Packet) {
		t.Helper()

		raw := serialize(t, pkt)
		_, err := NewFromRawBytes(bytes.NewReader(raw), false)
		require.ErrorIs(t, err, ErrDuplicateKey)
	}

	t.Run("global shares", func(t *testing.T) {
		pkt := newPacket(t)
		pkt.SilentPaymentShares = dupShares
		assertDupErr(t, pkt)
	})

	t.Run("global DLEQs", func(t *testing.T) {
		pkt := newPacket(t)
		pkt.SilentPaymentDLEQs = dupDLEQs
		assertDupErr(t, pkt)
	})

	t.Run("input shares", func(t *testing.T) {
		pkt := newPacket(t)
		pkt.Inputs[0].SilentPaymentShares = dupShares
		assertDupErr(t, pkt)
	})

	t.Run("input DLEQs", func(t *testing.T) {
		pkt := newPacket(t)
		pkt.Inputs[0].SilentPaymentDLEQs = dupDLEQs
		assertDupErr(t, pkt)
	})
}

// psbtV2 global key types used to walk the section structure of the BIP-375
// (PSBT v2) test vectors. They aren't defined in this v0-only package; we only
// need them here to learn how many input and output sections follow the global
// one.
// TODO(guggero): Simplify a lot of code in this file once PSBTv2 support is
// merged.
const (
	psbtV2InputCountType  = 0x04
	psbtV2OutputCountType = 0x05
)

// kvPair is a raw key-data / value byte pair extracted from a PSBT field.
type kvPair struct {
	keyData []byte
	value   []byte
}

// rawSPFields collects the raw bytes of every silent-payment-related field
// found while walking a PSBT, grouped by where the field lives and its type.
type rawSPFields struct {
	globalShares []kvPair
	globalDLEQs  []kvPair
	inputShares  []kvPair
	inputDLEQs   []kvPair
	outputInfos  [][]byte
	outputLabels [][]byte
}

// spVector is a single BIP-375 test vector.
type spVector struct {
	Description string `json:"description"`
	Psbt        string `json:"psbt"`
}

// spVectors is the BIP-375 test vector file structure.
type spVectors struct {
	Valid   []spVector `json:"valid"`
	Invalid []spVector `json:"invalid"`
}

// readSPVectors reads and decodes the BIP-375 silent payment test vectors.
func readSPVectors(t *testing.T) spVectors {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(
		"testdata", "bip375_test_vectors.json",
	))
	require.NoError(t, err)

	var vectors spVectors
	require.NoError(t, json.Unmarshal(raw, &vectors))

	return vectors
}

// TestSilentPaymentVectorsValid walks every valid BIP-375 test vector,
// extracts its silent payment fields, and asserts that each one deserializes
// through this package's silent payment readers and round-trips back to the
// same bytes.
//
// NOTE: The BIP-375 vectors are encoded as PSBT v2 packets, which this package
// can't parse yet (v2 support is being added separately). We therefore walk
// the raw PSBT key/value structure manually and only exercise the silent
// payment field (de)serialization that this package actually implements, not
// the full BIP-375 validation sequence.
func TestSilentPaymentVectorsValid(t *testing.T) {
	vectors := readSPVectors(t)
	require.NotEmpty(t, vectors.Valid)

	// We tally how many of each field type we exercised across the whole
	// corpus, so a regression that silently extracts nothing can't make
	// this test vacuously pass.
	var seenShares, seenDLEQs, seenInfos, seenLabels int

	for _, vector := range vectors.Valid {
		t.Run(vector.Description, func(t *testing.T) {
			psbtBytes, err := base64.StdEncoding.DecodeString(
				vector.Psbt,
			)
			require.NoError(t, err)

			f := extractSPFields(t, psbtBytes)

			// All ECDH shares (global and per-input) must decode
			// and round-trip cleanly.
			for _, s := range f.globalShares {
				assertShareRoundTrip(t, s)
			}
			for _, s := range f.inputShares {
				assertShareRoundTrip(t, s)
			}

			// The same goes for all DLEQ proofs.
			for _, d := range f.globalDLEQs {
				assertDLEQRoundTrip(t, d)
			}
			for _, d := range f.inputDLEQs {
				assertDLEQRoundTrip(t, d)
			}

			// And for all silent payment output info fields.
			for _, info := range f.outputInfos {
				assertInfoRoundTrip(t, info)
			}

			// A label is a fixed-size little-endian uint32.
			for _, label := range f.outputLabels {
				require.Len(t, label, uint32Size)
			}

			seenShares += len(f.globalShares) + len(f.inputShares)
			seenDLEQs += len(f.globalDLEQs) + len(f.inputDLEQs)
			seenInfos += len(f.outputInfos)
			seenLabels += len(f.outputLabels)
		})
	}

	require.Positive(t, seenShares, "no ECDH shares exercised")
	require.Positive(t, seenDLEQs, "no DLEQ proofs exercised")
	require.Positive(t, seenInfos, "no SP info fields exercised")
	require.Positive(t, seenLabels, "no SP labels exercised")
}

// TestSilentPaymentVectorsMalformedFields exercises the subset of invalid
// BIP-375 vectors whose defect lives in the silent payment field serialization
// itself (an incorrect byte length). The matching reader must reject the
// malformed field. The remaining invalid vectors fail BIP-375 semantic
// validation (ECDH coverage, input eligibility, output script derivation) that
// this package doesn't implement, so they're out of scope here.
func TestSilentPaymentVectorsMalformedFields(t *testing.T) {
	t.Parallel()

	vectors := readSPVectors(t)

	testCases := []struct {
		name       string
		descPrefix string
		assert     func(t *testing.T, f rawSPFields)
	}{{
		name: "output info wrong length",
		descPrefix: "psbt structure: incorrect byte length for " +
			"PSBT_OUT_SP_V0_INFO",
		assert: assertInfoFieldRejected,
	}, {
		name: "input share wrong length",
		descPrefix: "psbt structure: incorrect byte length for " +
			"PSBT_IN_SP_ECDH_SHARE",
		assert: assertShareFieldRejected,
	}, {
		name: "input DLEQ wrong length",
		descPrefix: "psbt structure: incorrect byte length for " +
			"PSBT_IN_SP_DLEQ",
		assert: assertDLEQFieldRejected,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			vector := findVector(t, vectors.Invalid, tc.descPrefix)

			psbtBytes, err := base64.StdEncoding.DecodeString(
				vector.Psbt,
			)
			require.NoError(t, err)

			tc.assert(t, extractSPFields(t, psbtBytes))
		})
	}
}

// extractSPFields walks the raw bytes of a PSBT (v2) packet and returns every
// silent-payment-related field it finds, grouped by location and type. Because
// the package can't parse PSBT v2 packets yet, we do a minimal manual walk of
// the key/value structure to get at the silent payment fields.
func extractSPFields(t *testing.T, raw []byte) rawSPFields {
	t.Helper()

	r := bytes.NewReader(raw)

	// Every PSBT starts with the magic bytes "psbt" followed by 0xff.
	magic := make([]byte, 5)
	_, err := io.ReadFull(r, magic)
	require.NoError(t, err)
	require.Equal(t, []byte("psbt\xff"), magic)

	var fields rawSPFields

	// The global section carries the input and output counts (PSBT v2),
	// which tell us how many input and output sections follow.
	var inputCount, outputCount uint64
	readSection(t, r, func(keyType byte, keyData, value []byte) {
		switch keyType {
		case psbtV2InputCountType:
			inputCount = decodeCompactSize(t, value)

		case psbtV2OutputCountType:
			outputCount = decodeCompactSize(t, value)

		case byte(SilentPaymentShareType):
			fields.globalShares = append(
				fields.globalShares, kvPair{keyData, value},
			)

		case byte(SilentPaymentDLEQType):
			fields.globalDLEQs = append(
				fields.globalDLEQs, kvPair{keyData, value},
			)
		}
	})

	// Each input section may carry a per-input ECDH share and DLEQ proof.
	for i := uint64(0); i < inputCount; i++ {
		readSection(t, r, func(keyType byte, keyData, value []byte) {
			switch keyType {
			case byte(SilentPaymentShareInputType):
				fields.inputShares = append(
					fields.inputShares,
					kvPair{keyData, value},
				)

			case byte(SilentPaymentDLEQInputType):
				fields.inputDLEQs = append(
					fields.inputDLEQs,
					kvPair{keyData, value},
				)
			}
		})
	}

	// Each output section may carry silent payment info and a label.
	for i := uint64(0); i < outputCount; i++ {
		readSection(t, r, func(keyType byte, keyData, value []byte) {
			switch keyType {
			case byte(SilentPaymentV0InfoOutputType):
				fields.outputInfos = append(
					fields.outputInfos, value,
				)

			case byte(SilentPaymentV0LabelOutputType):
				fields.outputLabels = append(
					fields.outputLabels, value,
				)
			}
		})
	}

	return fields
}

// readSection reads PSBT key/value pairs until the section separator (a
// zero-length key) and invokes handle for each pair. The key type is the
// leading compact-size value of the key; for every silent payment (and count)
// field it fits in a single byte, so we pass the first key byte as the type.
func readSection(t *testing.T, r *bytes.Reader,
	handle func(keyType byte, keyData, value []byte)) {

	t.Helper()

	for {
		keyLen, err := wire.ReadVarInt(r, 0)
		require.NoError(t, err)

		// A zero-length key marks the end of the section.
		if keyLen == 0 {
			return
		}

		key := make([]byte, keyLen)
		_, err = io.ReadFull(r, key)
		require.NoError(t, err)

		valueLen, err := wire.ReadVarInt(r, 0)
		require.NoError(t, err)

		value := make([]byte, valueLen)
		_, err = io.ReadFull(r, value)
		require.NoError(t, err)

		handle(key[0], key[1:], value)
	}
}

// decodeCompactSize decodes a single compact-size integer from the given bytes.
func decodeCompactSize(t *testing.T, b []byte) uint64 {
	t.Helper()

	v, err := wire.ReadVarInt(bytes.NewReader(b), 0)
	require.NoError(t, err)

	return v
}

// findVector returns the first vector whose description starts with the given
// prefix.
func findVector(t *testing.T, vectors []spVector, prefix string) spVector {
	t.Helper()

	for _, v := range vectors {
		if strings.HasPrefix(v.Description, prefix) {
			return v
		}
	}

	require.FailNowf(t, "vector not found", "prefix %q", prefix)

	return spVector{}
}

// assertShareRoundTrip asserts that a raw silent payment share decodes and
// re-serializes to the exact same bytes.
func assertShareRoundTrip(t *testing.T, raw kvPair) {
	t.Helper()

	share, err := ReadSilentPaymentShare(raw.keyData, raw.value)
	require.NoError(t, err)

	keyData, value := SerializeSilentPaymentShare(share)
	require.Equal(t, raw.keyData, keyData)
	require.Equal(t, raw.value, value)
}

// assertDLEQRoundTrip asserts that a raw silent payment DLEQ proof decodes and
// re-serializes to the exact same bytes.
func assertDLEQRoundTrip(t *testing.T, raw kvPair) {
	t.Helper()

	dleq, err := ReadSilentPaymentDLEQ(raw.keyData, raw.value)
	require.NoError(t, err)

	keyData, value := SerializeSilentPaymentDLEQ(dleq)
	require.Equal(t, raw.keyData, keyData)
	require.Equal(t, raw.value, value)
}

// assertInfoRoundTrip asserts that a raw silent payment output info field
// decodes and re-serializes to the exact same bytes.
func assertInfoRoundTrip(t *testing.T, value []byte) {
	t.Helper()

	info, err := ReadSilentPaymentInfo(value)
	require.NoError(t, err)

	require.Equal(t, value, SerializeSilentPaymentInfo(info))
}

// assertInfoFieldRejected asserts that at least one of the extracted output
// info fields is rejected by ReadSilentPaymentInfo.
func assertInfoFieldRejected(t *testing.T, f rawSPFields) {
	t.Helper()

	requireRejected(t, len(f.outputInfos), func(i int) error {
		_, err := ReadSilentPaymentInfo(f.outputInfos[i])
		return err
	})
}

// assertShareFieldRejected asserts that at least one of the extracted input
// shares is rejected by ReadSilentPaymentShare.
func assertShareFieldRejected(t *testing.T, f rawSPFields) {
	t.Helper()

	requireRejected(t, len(f.inputShares), func(i int) error {
		_, err := ReadSilentPaymentShare(
			f.inputShares[i].keyData, f.inputShares[i].value,
		)
		return err
	})
}

// assertDLEQFieldRejected asserts that at least one of the extracted input DLEQ
// proofs is rejected by ReadSilentPaymentDLEQ.
func assertDLEQFieldRejected(t *testing.T, f rawSPFields) {
	t.Helper()

	requireRejected(t, len(f.inputDLEQs), func(i int) error {
		_, err := ReadSilentPaymentDLEQ(
			f.inputDLEQs[i].keyData, f.inputDLEQs[i].value,
		)
		return err
	})
}

// requireRejected asserts that at least one of the n candidate fields is
// rejected (the read function returns an error). It fails if there are no
// candidates or if every candidate is accepted.
func requireRejected(t *testing.T, n int, read func(i int) error) {
	t.Helper()

	require.Positive(t, n, "no candidate fields found in vector")

	for i := 0; i < n; i++ {
		if read(i) != nil {
			return
		}
	}

	require.Fail(t, "expected a malformed silent payment field")
}
