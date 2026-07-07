// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/stretchr/testify/require"
)

// Test data from
// https://github.com/bitcoin/bips/blob/master/bip-0373.mediawiki#test-vectors

const (
	// bip373AggregateKeyHex is the compressed MuSig2 aggregate pubkey used
	// throughout the BIP-373 test vectors.
	bip373AggregateKeyHex = "030b58e337aa4d3852a8c29387c42408d8" +
		"cfbe3a613a5e397e0a9f01a5fb7107d4"

	// bip373AggregateKeyXOnlyHex is the x-only encoding of the same key,
	// used to assert that x-only keydata is rejected by the parser.
	bip373AggregateKeyXOnlyHex = "0b58e337aa4d3852a8c29387c42408d8" +
		"cfbe3a613a5e397e0a9f01a5fb7107d4"

	// bip373TestVectorsPath is the on-disk location of the JSON-encoded
	// BIP-373 test vectors.
	bip373TestVectorsPath = "testdata/bip-373-test-vectors.json"
)

// bip373ParticipantHex are the three participant pubkeys (in aggregation
// order) used by every BIP-373 test vector.
var bip373ParticipantHex = []string{
	"02346b99593357107c9d3459e9deba8d3eaf44e6636c85c7f853eb90ba52e8cd00",
	"024fafd65f8169186fc2bfdb2233c77e630d10be280a24c7165c09a27611775c2c",
	"02f9308a019258c31049344f85f89d5229b531c845836f99b08601f113bce036f9",
}

// mustParsePubKey parses a hex-encoded compressed pubkey or fails the test.
func mustParsePubKey(t *testing.T, hexStr string) *btcec.PublicKey {
	t.Helper()

	raw, err := hex.DecodeString(hexStr)
	require.NoError(t, err)

	pk, err := btcec.ParsePubKey(raw)
	require.NoError(t, err)

	return pk
}

// mustDecodeHex decodes a hex string or fails the test.
func mustDecodeHex(t *testing.T, hexStr string) []byte {
	t.Helper()

	raw, err := hex.DecodeString(hexStr)
	require.NoError(t, err)

	return raw
}

// bip373Vector is one BIP-373 PSBT test vector as encoded in
// testdata/bip-373-test-vectors.json. Valid is true for vectors that must
// parse cleanly and false for vectors that must be rejected.
type bip373Vector struct {
	Name  string
	Hex   string
	Valid bool
}

var (
	bip373VectorsOnce sync.Once
	bip373Vectors     []bip373Vector
)

// loadBIP373Vectors reads and decodes the BIP-373 test vector JSON file
// exactly once per test binary invocation.
func loadBIP373Vectors(t *testing.T) []bip373Vector {
	t.Helper()

	bip373VectorsOnce.Do(func() {
		raw, err := os.ReadFile(bip373TestVectorsPath)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &bip373Vectors))
	})

	return bip373Vectors
}

// bip373ValidVectors returns the subset of BIP-373 test vectors that must
// parse cleanly, in the order they appear in the JSON file.
func bip373ValidVectors(t *testing.T) []bip373Vector {
	t.Helper()

	all := loadBIP373Vectors(t)
	out := make([]bip373Vector, 0, len(all))
	for _, v := range all {
		if v.Valid {
			out = append(out, v)
		}
	}
	return out
}

// bip373InvalidVectors returns the subset of BIP-373 test vectors that must
// be rejected at parse time, in the order they appear in the JSON file.
func bip373InvalidVectors(t *testing.T) []bip373Vector {
	t.Helper()

	all := loadBIP373Vectors(t)
	out := make([]bip373Vector, 0, len(all))
	for _, v := range all {
		if !v.Valid {
			out = append(out, v)
		}
	}
	return out
}

// findVector returns the BIP-373 valid test vector hex blob whose name
// contains the given case-insensitive substring. Fails the test if no
// matching vector exists.
func findVector(t *testing.T, namePrefix string) string {
	t.Helper()

	for _, tc := range bip373ValidVectors(t) {
		if strings.HasPrefix(tc.Name, namePrefix) {
			return tc.Hex
		}
	}
	t.Fatalf("no BIP-373 valid vector matching %q", namePrefix)
	return ""
}

// TestBIP373ValidPsbts asserts that every valid BIP-373 test vector parses
// and round-trips through serialization without modification.
func TestBIP373ValidPsbts(t *testing.T) {
	for _, tc := range bip373ValidVectors(t) {
		t.Run(tc.Name, func(t *testing.T) {
			raw := mustDecodeHex(t, tc.Hex)

			p, err := NewFromRawBytes(bytes.NewReader(raw), false)
			require.NoError(t, err)

			var b bytes.Buffer
			require.NoError(t, p.Serialize(&b))
			require.Equal(t, raw, b.Bytes())
		})
	}
}

// TestBIP373InvalidPsbts asserts that every invalid BIP-373 test vector is
// rejected at parse time.
func TestBIP373InvalidPsbts(t *testing.T) {
	for _, tc := range bip373InvalidVectors(t) {
		t.Run(tc.Name, func(t *testing.T) {
			raw := mustDecodeHex(t, tc.Hex)

			_, err := NewFromRawBytes(bytes.NewReader(raw), false)
			require.Error(t, err)
		})
	}
}

// TestMuSig2Participants_RoundTrip exercises ReadMuSig2Participants and
// SerializeMuSig2Participants together, asserting the round trip is lossless.
func TestMuSig2Participants_RoundTrip(t *testing.T) {
	agg := mustParsePubKey(t, bip373AggregateKeyHex)

	keys := make([]*btcec.PublicKey, len(bip373ParticipantHex))
	for i, h := range bip373ParticipantHex {
		keys[i] = mustParsePubKey(t, h)
	}

	original := &MuSig2Participants{
		AggregateKey: agg,
		Keys:         keys,
	}

	// Build the value bytes the same way SerializeMuSig2Participants does
	// internally and feed them back through the read path.
	var value bytes.Buffer
	for _, k := range keys {
		value.Write(k.SerializeCompressed())
	}

	parsed, err := ReadMuSig2Participants(original.KeyData(), value.Bytes())
	require.NoError(t, err)
	require.True(t, parsed.AggregateKey.IsEqual(agg))
	require.Equal(t, len(keys), len(parsed.Keys))
	for i := range keys {
		require.True(t, keys[i].IsEqual(parsed.Keys[i]))
	}
}

// TestReadMuSig2Participants_Rejects validates that ReadMuSig2Participants
// rejects malformed keydata and valuedata.
func TestReadMuSig2Participants_Rejects(t *testing.T) {
	validValue := mustDecodeHex(t, bip373ParticipantHex[0])

	tests := []struct {
		name      string
		keyData   []byte
		value     []byte
		expectErr error
	}{
		{
			name:      "x-only aggregate key",
			keyData:   mustDecodeHex(t, bip373AggregateKeyXOnlyHex),
			value:     validValue,
			expectErr: ErrInvalidKeyData,
		},
		{
			name: "uncompressed aggregate key (65 bytes)",
			keyData: append(
				[]byte{0x04}, make([]byte, 64)...,
			),
			value:     validValue,
			expectErr: ErrInvalidKeyData,
		},
		{
			name:      "empty value",
			keyData:   mustDecodeHex(t, bip373AggregateKeyHex),
			value:     []byte{},
			expectErr: ErrInvalidPsbtFormat,
		},
		{
			name:      "value not multiple of 33",
			keyData:   mustDecodeHex(t, bip373AggregateKeyHex),
			value:     append(validValue, 0x00),
			expectErr: ErrInvalidPsbtFormat,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReadMuSig2Participants(tc.keyData, tc.value)
			require.ErrorIs(t, err, tc.expectErr)
		})
	}
}

// TestMuSig2PubNonce_RoundTrip exercises the nonce read/write path with and
// without a tap leaf hash.
func TestMuSig2PubNonce_RoundTrip(t *testing.T) {
	pubKey := mustParsePubKey(t, bip373ParticipantHex[0])
	agg := mustParsePubKey(t, bip373AggregateKeyHex)

	var pubNonce [musig2.PubNonceSize]byte
	for i := range pubNonce {
		pubNonce[i] = byte(i + 1)
	}

	tapLeaf := bytes.Repeat([]byte{0xab}, sha256.Size)

	tests := []struct {
		name string
		leaf []byte
	}{
		{name: "without tap leaf hash", leaf: nil},
		{name: "with tap leaf hash", leaf: tapLeaf},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := &MuSig2PubNonce{
				PubKey:       pubKey,
				AggregateKey: agg,
				TapLeafHash:  tc.leaf,
				PubNonce:     pubNonce,
			}

			parsed, err := ReadMuSig2PubNonce(
				original.KeyData(), pubNonce[:],
			)
			require.NoError(t, err)
			require.True(t, parsed.PubKey.IsEqual(pubKey))
			require.True(t, parsed.AggregateKey.IsEqual(agg))
			require.Equal(t, tc.leaf, parsed.TapLeafHash)
			require.Equal(t, pubNonce, parsed.PubNonce)
		})
	}
}

// TestReadMuSig2PubNonce_Rejects validates input rejection for malformed
// keydata and valuedata.
func TestReadMuSig2PubNonce_Rejects(t *testing.T) {
	pubKey := mustDecodeHex(t, bip373ParticipantHex[0])
	agg := mustDecodeHex(t, bip373AggregateKeyHex)
	leaf := bytes.Repeat([]byte{0xab}, sha256.Size)

	validKey := append(append([]byte{}, pubKey...), agg...)
	validValue := make([]byte, musig2.PubNonceSize)

	tests := []struct {
		name      string
		keyData   []byte
		value     []byte
		expectErr error
	}{
		{
			name:      "x-only participant key (32+33 bytes)",
			keyData:   append(pubKey[1:], agg...),
			value:     validValue,
			expectErr: ErrInvalidKeyData,
		},
		{
			name: "x-only aggregate key (33+32 bytes)",
			keyData: append(
				append([]byte{}, pubKey...), agg[1:]...,
			),
			value:     validValue,
			expectErr: ErrInvalidKeyData,
		},
		{
			name: "keydata with bad leaf hash length (97 bytes)",
			keyData: append(
				append([]byte{}, validKey...), leaf[:31]...,
			),
			value:     validValue,
			expectErr: ErrInvalidKeyData,
		},
		{
			name:      "value too short (65 bytes)",
			keyData:   validKey,
			value:     make([]byte, musig2.PubNonceSize-1),
			expectErr: ErrInvalidPsbtFormat,
		},
		{
			name:      "value too long (67 bytes)",
			keyData:   validKey,
			value:     make([]byte, musig2.PubNonceSize+1),
			expectErr: ErrInvalidPsbtFormat,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReadMuSig2PubNonce(tc.keyData, tc.value)
			require.ErrorIs(t, err, tc.expectErr)
		})
	}
}

// TestMuSig2PartialSig_RoundTrip exercises the partial-sig read/write path
// with and without a tap leaf hash.
func TestMuSig2PartialSig_RoundTrip(t *testing.T) {
	pubKey := mustParsePubKey(t, bip373ParticipantHex[0])
	agg := mustParsePubKey(t, bip373AggregateKeyHex)

	// Use a small but non-zero scalar so PartialSignature.Decode succeeds.
	value := make([]byte, 32)
	value[31] = 0x01

	tapLeaf := bytes.Repeat([]byte{0xab}, sha256.Size)

	tests := []struct {
		name string
		leaf []byte
	}{
		{name: "without tap leaf hash", leaf: nil},
		{name: "with tap leaf hash", leaf: tapLeaf},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			template := &MuSig2PartialSig{
				PubKey:       pubKey,
				AggregateKey: agg,
				TapLeafHash:  tc.leaf,
			}

			parsed, err := ReadMuSig2PartialSig(
				template.KeyData(), value,
			)
			require.NoError(t, err)
			require.True(t, parsed.PubKey.IsEqual(pubKey))
			require.True(t, parsed.AggregateKey.IsEqual(agg))
			require.Equal(t, tc.leaf, parsed.TapLeafHash)

			// Re-serialize the partial sig and compare against the
			// original 32-byte input.
			var buf bytes.Buffer
			require.NoError(t, parsed.PartialSig.Encode(&buf))
			require.Equal(t, value, buf.Bytes())
		})
	}
}

// TestReadMuSig2PartialSig_Rejects validates input rejection for malformed
// keydata and valuedata.
func TestReadMuSig2PartialSig_Rejects(t *testing.T) {
	pubKey := mustDecodeHex(t, bip373ParticipantHex[0])
	agg := mustDecodeHex(t, bip373AggregateKeyHex)

	validKey := append(append([]byte{}, pubKey...), agg...)
	validValue := make([]byte, 32)
	validValue[31] = 0x01

	tests := []struct {
		name      string
		keyData   []byte
		value     []byte
		expectErr error
	}{
		{
			name:      "x-only participant key",
			keyData:   append(pubKey[1:], agg...),
			value:     validValue,
			expectErr: ErrInvalidKeyData,
		},
		{
			name: "x-only aggregate key",
			keyData: append(
				append([]byte{}, pubKey...), agg[1:]...,
			),
			value:     validValue,
			expectErr: ErrInvalidKeyData,
		},
		{
			name:      "value too short (31 bytes)",
			keyData:   validKey,
			value:     make([]byte, 31),
			expectErr: ErrInvalidPsbtFormat,
		},
		{
			name:      "value too long (33 bytes)",
			keyData:   validKey,
			value:     make([]byte, 33),
			expectErr: ErrInvalidPsbtFormat,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReadMuSig2PartialSig(tc.keyData, tc.value)
			require.ErrorIs(t, err, tc.expectErr)
		})
	}
}

// TestPInput_DuplicateMuSig2Fields asserts that the deserializer rejects
// duplicate MuSig2 keys (participants, nonces, partial sigs) in the same
// input. We synthesize a malformed input by re-using one of the BIP-373
// valid hex vectors, doubling the participants field, and parsing the
// result.
func TestPInput_DuplicateMuSig2Fields(t *testing.T) {
	// Take case 1a (smallest vector with a participants field) and inject
	// a duplicate. The participants record starts with the byte sequence
	// 0x22 0x1a (varint key length 0x22 = 34, key type 0x1a). Find that
	// record and append a copy.
	src := bip373ValidVectors(t)[0].Hex

	// The participants record in case 1a, found by searching for the
	// known prefix once.
	const recordPrefix = "221a030b58e337aa4d3852a8c29387c42408d8" +
		"cfbe3a613a5e397e0a9f01a5fb7107d463"
	idx := strings.Index(src, recordPrefix)
	require.GreaterOrEqual(t, idx, 0)

	// The record extends from idx through the end of the value (3
	// concatenated 33-byte pubkeys = 99 bytes plus 1 varint length byte
	// for the value). 0x63 right after the keydata is the value length
	// (0x63 = 99 == 3*33). So total record length = 1 (keylen) + 0x22
	// (34 = key type + 33 byte agg key) + 1 (value len) + 99 = 135 bytes
	// = 270 hex chars.
	const recordHexLen = 1*2 + 0x22*2 + 1*2 + 0x63*2
	require.GreaterOrEqual(t, len(src)-idx, recordHexLen)

	dup := src[:idx+recordHexLen] + src[idx:idx+recordHexLen] +
		src[idx+recordHexLen:]

	raw := mustDecodeHex(t, dup)

	_, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.ErrorIs(t, err, ErrDuplicateKey)
}

// TestUpdater_AddInMuSig2 exercises the AddIn* MuSig2 updater helpers,
// asserting that values are appended on the input and that duplicate keys
// are rejected.
func TestUpdater_AddInMuSig2(t *testing.T) {
	// Start from BIP-373 case 1a (smallest vector with MuSig2 fields)
	// and clear the input's MuSig2 fields so we can re-populate them via
	// the updater helpers.
	raw := mustDecodeHex(t, bip373ValidVectors(t)[0].Hex)
	p, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.NoError(t, err)

	p.Inputs[0].MuSig2Participants = nil
	p.Inputs[0].MuSig2PubNonces = nil
	p.Inputs[0].MuSig2PartialSigs = nil

	updater, err := NewUpdater(p)
	require.NoError(t, err)

	agg := mustParsePubKey(t, bip373AggregateKeyHex)
	pub0 := mustParsePubKey(t, bip373ParticipantHex[0])
	pub1 := mustParsePubKey(t, bip373ParticipantHex[1])
	pub2 := mustParsePubKey(t, bip373ParticipantHex[2])
	keys := []*btcec.PublicKey{pub0, pub1, pub2}

	participants := &MuSig2Participants{
		AggregateKey: agg, Keys: keys,
	}
	require.NoError(t, updater.AddInMuSig2Participants(0, participants))
	require.Len(t, p.Inputs[0].MuSig2Participants, 1)

	// Re-adding the same record fails with ErrDuplicateKey.
	require.ErrorIs(t,
		updater.AddInMuSig2Participants(0, participants),
		ErrDuplicateKey,
	)

	// Out-of-range input index is rejected.
	require.ErrorIs(t,
		updater.AddInMuSig2Participants(99, participants),
		ErrInvalidPsbtFormat,
	)

	// Add three nonces and three partial sigs.
	var dummyNonce [musig2.PubNonceSize]byte
	for i := range dummyNonce {
		dummyNonce[i] = byte(i + 1)
	}
	for _, k := range keys {
		require.NoError(t, updater.AddInMuSig2PubNonce(
			0, &MuSig2PubNonce{
				PubKey: k, AggregateKey: agg,
				PubNonce: dummyNonce,
			},
		))
	}
	require.Len(t, p.Inputs[0].MuSig2PubNonces, 3)

	// Duplicate nonce keydata is rejected.
	require.ErrorIs(t,
		updater.AddInMuSig2PubNonce(
			0, &MuSig2PubNonce{
				PubKey: pub0, AggregateKey: agg,
				PubNonce: dummyNonce,
			},
		),
		ErrDuplicateKey,
	)
}

// TestUpdater_AddOutMuSig2Participants exercises the AddOutMuSig2Participants
// helper.
func TestUpdater_AddOutMuSig2Participants(t *testing.T) {
	// Build a minimal one-output PSBT.
	raw := mustDecodeHex(t, bip373ValidVectors(t)[0].Hex)
	p, err := NewFromRawBytes(bytes.NewReader(raw), false)
	require.NoError(t, err)

	updater, err := NewUpdater(p)
	require.NoError(t, err)

	agg := mustParsePubKey(t, bip373AggregateKeyHex)
	keys := []*btcec.PublicKey{
		mustParsePubKey(t, bip373ParticipantHex[0]),
		mustParsePubKey(t, bip373ParticipantHex[1]),
		mustParsePubKey(t, bip373ParticipantHex[2]),
	}

	require.NoError(t, updater.AddOutMuSig2Participants(
		0, &MuSig2Participants{AggregateKey: agg, Keys: keys},
	))
	require.Len(t, p.Outputs[0].MuSig2Participants, 1)

	// Duplicate aggregate is rejected.
	require.ErrorIs(t, updater.AddOutMuSig2Participants(
		0, &MuSig2Participants{AggregateKey: agg, Keys: keys},
	), ErrDuplicateKey)
}
