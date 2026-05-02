// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/wire/v2"
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

// musig2Field is the interface implemented by all three MuSig2 PSBT field
// types. It allows the round trip assertions below to be expressed once
// instead of once per field type.
type musig2Field interface {
	// Validate asserts the field is sane for being serialized into a PSBT
	// packet.
	Validate() error

	// KeyData returns the key data of the field's PSBT key-value pair.
	KeyData() []byte
}

// bip373Participants returns the aggregate key and the three participant keys
// (in aggregation order) of the BIP-373 test vectors.
func bip373Participants(t *testing.T) (*btcec.PublicKey, []*btcec.PublicKey) {
	t.Helper()

	keys := make([]*btcec.PublicKey, len(bip373ParticipantHex))
	for idx, keyHex := range bip373ParticipantHex {
		keys[idx] = mustParsePubKey(t, keyHex)
	}

	return mustParsePubKey(t, bip373AggregateKeyHex), keys
}

// readKVPair reads back a single key-value pair as written by
// serializeKVPairWithType, returning the key type, the key data and the value.
func readKVPair(t *testing.T, r io.Reader) (int, []byte, []byte) {
	t.Helper()

	keyType, keyData, err := getKey(r)
	require.NoError(t, err)

	value, err := wire.ReadVarBytes(r, 0, MaxPsbtValueLength, "value")
	require.NoError(t, err)

	return keyType, keyData, value
}

// assertMuSig2RoundTrip serializes the given MuSig2 field into a PSBT
// key-value pair, reads that pair back out and then serializes the parsed
// result a second time. Both serializations must produce the exact same bytes,
// which in turn requires the parsed field to pass Validate again. That is the
// property the Validate calls in the Serialize functions exist to uphold: what
// we write must be readable, and what we read must be writable again.
func assertMuSig2RoundTrip[T musig2Field](t *testing.T, typ uint8, field T,
	serialize func(io.Writer, uint8, T) error,
	read func(keyData, value []byte) (T, error)) T {

	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, serialize(&buf, typ, field))
	written := buf.Bytes()

	// The written key must be the type byte followed by exactly the field's
	// key data, and the value must survive the varint framing untouched.
	keyType, keyData, value := readKVPair(t, bytes.NewReader(written))
	require.EqualValues(t, typ, keyType)
	require.Equal(t, field.KeyData(), keyData)

	parsed, err := read(keyData, value)
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())

	var reSerialized bytes.Buffer
	require.NoError(t, serialize(&reSerialized, typ, parsed))
	require.Equal(t, written, reSerialized.Bytes())

	return parsed
}

// assertMuSig2SerializeRejects asserts that serializing an invalid MuSig2 field
// surfaces the expected error from Validate instead of panicking, and that no
// partial record was written to the writer.
func assertMuSig2SerializeRejects[T musig2Field](t *testing.T, typ uint8,
	field T, serialize func(io.Writer, uint8, T) error,
	expectErr error) {

	t.Helper()

	require.ErrorIs(t, field.Validate(), expectErr)

	var (
		buf bytes.Buffer
		err error
	)
	require.NotPanics(t, func() {
		err = serialize(&buf, typ, field)
	})
	require.ErrorIs(t, err, expectErr)
	require.Zero(t, buf.Len())
}

// TestMuSig2Participants_Validate asserts that Validate accepts every
// participants record that survives an encode/decode round trip, and rejects
// every record that would either panic during serialization or produce a
// key-value pair the read path no longer accepts.
func TestMuSig2Participants_Validate(t *testing.T) {
	const typ = uint8(MuSig2ParticipantsInputType)

	agg, keys := bip373Participants(t)

	tests := []struct {
		name         string
		participants *MuSig2Participants
		expectErr    error
	}{
		{
			name: "three participants",
			participants: &MuSig2Participants{
				AggregateKey: agg,
				Keys:         keys,
			},
		},
		{
			name: "single participant",
			participants: &MuSig2Participants{
				AggregateKey: agg,
				Keys:         keys[:1],
			},
		},
		{
			name: "aggregate key repeated as participant",
			participants: &MuSig2Participants{
				AggregateKey: agg,
				Keys: append(
					[]*btcec.PublicKey{agg}, keys...,
				),
			},
		},
		{
			name: "missing aggregate key",
			participants: &MuSig2Participants{
				Keys: keys,
			},
			expectErr: ErrMissingKey,
		},
		{
			name: "no participant keys",
			participants: &MuSig2Participants{
				AggregateKey: agg,
			},
			expectErr: ErrMissingKey,
		},
		{
			name: "empty (non-nil) participant keys",
			participants: &MuSig2Participants{
				AggregateKey: agg,
				Keys:         []*btcec.PublicKey{},
			},
			expectErr: ErrMissingKey,
		},
		{
			name: "nil participant key",
			participants: &MuSig2Participants{
				AggregateKey: agg,
				Keys: []*btcec.PublicKey{
					keys[0], nil, keys[2],
				},
			},
			expectErr: ErrMissingKey,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.expectErr != nil {
				assertMuSig2SerializeRejects(
					t, typ, tc.participants,
					SerializeMuSig2Participants,
					tc.expectErr,
				)

				return
			}

			parsed := assertMuSig2RoundTrip(
				t, typ, tc.participants,
				SerializeMuSig2Participants,
				ReadMuSig2Participants,
			)

			require.True(t, parsed.AggregateKey.IsEqual(
				tc.participants.AggregateKey,
			))
			require.Len(t, parsed.Keys, len(tc.participants.Keys))
			for idx, key := range tc.participants.Keys {
				require.True(t, key.IsEqual(parsed.Keys[idx]))
			}
		})
	}
}

// TestMuSig2PubNonce_Validate asserts that Validate accepts every public nonce
// record that survives an encode/decode round trip, and rejects every record
// that would either panic during serialization or produce key data the read
// path no longer accepts.
func TestMuSig2PubNonce_Validate(t *testing.T) {
	const typ = uint8(MuSig2PubNoncesInputType)

	agg, keys := bip373Participants(t)

	var pubNonce [musig2.PubNonceSize]byte
	for idx := range pubNonce {
		pubNonce[idx] = byte(idx + 1)
	}

	tapLeaf := bytes.Repeat([]byte{0xab}, sha256.Size)

	tests := []struct {
		name      string
		nonce     *MuSig2PubNonce
		expectErr error
	}{
		{
			name: "without tap leaf hash",
			nonce: &MuSig2PubNonce{
				PubKey:       keys[0],
				AggregateKey: agg,
				PubNonce:     pubNonce,
			},
		},
		{
			name: "with tap leaf hash",
			nonce: &MuSig2PubNonce{
				PubKey:       keys[0],
				AggregateKey: agg,
				TapLeafHash:  tapLeaf,
				PubNonce:     pubNonce,
			},
		},
		{
			// An empty but non-nil tap leaf hash is treated the
			// same as an absent one, and comes back as nil.
			name: "empty (non-nil) tap leaf hash",
			nonce: &MuSig2PubNonce{
				PubKey:       keys[0],
				AggregateKey: agg,
				TapLeafHash:  []byte{},
				PubNonce:     pubNonce,
			},
		},
		{
			// The nonce bytes themselves are opaque to the codec,
			// so an all-zero nonce still round trips cleanly.
			name: "zero pub nonce",
			nonce: &MuSig2PubNonce{
				PubKey:       keys[0],
				AggregateKey: agg,
			},
		},
		{
			name: "aggregate key as participant key",
			nonce: &MuSig2PubNonce{
				PubKey:       agg,
				AggregateKey: agg,
				PubNonce:     pubNonce,
			},
		},
		{
			name: "missing participant key",
			nonce: &MuSig2PubNonce{
				AggregateKey: agg,
				PubNonce:     pubNonce,
			},
			expectErr: ErrMissingKey,
		},
		{
			name: "missing aggregate key",
			nonce: &MuSig2PubNonce{
				PubKey:   keys[0],
				PubNonce: pubNonce,
			},
			expectErr: ErrMissingKey,
		},
		{
			name: "tap leaf hash too short",
			nonce: &MuSig2PubNonce{
				PubKey:       keys[0],
				AggregateKey: agg,
				TapLeafHash:  tapLeaf[:sha256.Size-1],
				PubNonce:     pubNonce,
			},
			expectErr: ErrInvalidTapLeafHash,
		},
		{
			name: "tap leaf hash too long",
			nonce: &MuSig2PubNonce{
				PubKey:       keys[0],
				AggregateKey: agg,
				TapLeafHash: bytes.Repeat(
					[]byte{0xab}, sha256.Size+1,
				),
				PubNonce: pubNonce,
			},
			expectErr: ErrInvalidTapLeafHash,
		},
		{
			name: "single byte tap leaf hash",
			nonce: &MuSig2PubNonce{
				PubKey:       keys[0],
				AggregateKey: agg,
				TapLeafHash:  []byte{0xab},
				PubNonce:     pubNonce,
			},
			expectErr: ErrInvalidTapLeafHash,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.expectErr != nil {
				assertMuSig2SerializeRejects(
					t, typ, tc.nonce,
					SerializeMuSig2PubNonce, tc.expectErr,
				)

				return
			}

			parsed := assertMuSig2RoundTrip(
				t, typ, tc.nonce, SerializeMuSig2PubNonce,
				ReadMuSig2PubNonce,
			)

			require.True(t, parsed.PubKey.IsEqual(tc.nonce.PubKey))
			require.True(t, parsed.AggregateKey.IsEqual(
				tc.nonce.AggregateKey,
			))
			require.Equal(t, tc.nonce.PubNonce, parsed.PubNonce)

			if len(tc.nonce.TapLeafHash) == 0 {
				require.Empty(t, parsed.TapLeafHash)
			} else {
				require.Equal(
					t, tc.nonce.TapLeafHash,
					parsed.TapLeafHash,
				)
			}
		})
	}
}

// TestMuSig2PartialSig_Validate asserts that Validate accepts every partial
// signature record that survives an encode/decode round trip, and rejects
// every record that would either panic during serialization or produce key
// data the read path no longer accepts.
//
// The R value of a partial signature is deliberately not required: it is not
// part of the 32-byte serialization, so a parsed record never has it set.
// Requiring it would make any partial signature read from an existing PSBT
// impossible to write back out again.
func TestMuSig2PartialSig_Validate(t *testing.T) {
	const typ = uint8(MuSig2PartialSigsInputType)

	agg, keys := bip373Participants(t)
	tapLeaf := bytes.Repeat([]byte{0xab}, sha256.Size)

	// A small but non-zero scalar, once paired with the R value that is
	// dropped by the serialization and once without it.
	sigWithR := musig2.NewPartialSignature(
		new(btcec.ModNScalar).SetInt(1), keys[1],
	)
	sigWithoutR := musig2.PartialSignature{
		S: new(btcec.ModNScalar).SetInt(1),
	}

	tests := []struct {
		name       string
		partialSig *MuSig2PartialSig
		expectErr  error
	}{
		{
			name: "without tap leaf hash",
			partialSig: &MuSig2PartialSig{
				PubKey:       keys[0],
				AggregateKey: agg,
				PartialSig:   sigWithR,
			},
		},
		{
			name: "with tap leaf hash",
			partialSig: &MuSig2PartialSig{
				PubKey:       keys[0],
				AggregateKey: agg,
				TapLeafHash:  tapLeaf,
				PartialSig:   sigWithR,
			},
		},
		{
			// This is what a record parsed out of a PSBT looks
			// like, so it must be serializable again.
			name: "partial signature without R",
			partialSig: &MuSig2PartialSig{
				PubKey:       keys[0],
				AggregateKey: agg,
				PartialSig:   sigWithoutR,
			},
		},
		{
			// A zero scalar is not a usable signature, but it is a
			// valid serialization that round trips, so Validate
			// has no reason to reject it.
			name: "zero scalar",
			partialSig: &MuSig2PartialSig{
				PubKey:       keys[0],
				AggregateKey: agg,
				PartialSig: musig2.PartialSignature{
					S: new(btcec.ModNScalar),
				},
			},
		},
		{
			name: "empty (non-nil) tap leaf hash",
			partialSig: &MuSig2PartialSig{
				PubKey:       keys[0],
				AggregateKey: agg,
				TapLeafHash:  []byte{},
				PartialSig:   sigWithR,
			},
		},
		{
			name: "missing participant key",
			partialSig: &MuSig2PartialSig{
				AggregateKey: agg,
				PartialSig:   sigWithR,
			},
			expectErr: ErrMissingKey,
		},
		{
			name: "missing aggregate key",
			partialSig: &MuSig2PartialSig{
				PubKey:     keys[0],
				PartialSig: sigWithR,
			},
			expectErr: ErrMissingKey,
		},
		{
			name: "missing partial signature",
			partialSig: &MuSig2PartialSig{
				PubKey:       keys[0],
				AggregateKey: agg,
			},
			expectErr: ErrMissingPartialSignature,
		},
		{
			// R alone is not enough, S is what gets serialized.
			name: "partial signature with R but without S",
			partialSig: &MuSig2PartialSig{
				PubKey:       keys[0],
				AggregateKey: agg,
				PartialSig: musig2.PartialSignature{
					R: keys[1],
				},
			},
			expectErr: ErrMissingPartialSignature,
		},
		{
			name: "tap leaf hash too short",
			partialSig: &MuSig2PartialSig{
				PubKey:       keys[0],
				AggregateKey: agg,
				TapLeafHash:  tapLeaf[:sha256.Size-1],
				PartialSig:   sigWithR,
			},
			expectErr: ErrInvalidTapLeafHash,
		},
		{
			name: "tap leaf hash too long",
			partialSig: &MuSig2PartialSig{
				PubKey:       keys[0],
				AggregateKey: agg,
				TapLeafHash: bytes.Repeat(
					[]byte{0xab}, sha256.Size+1,
				),
				PartialSig: sigWithR,
			},
			expectErr: ErrInvalidTapLeafHash,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.expectErr != nil {
				assertMuSig2SerializeRejects(
					t, typ, tc.partialSig,
					SerializeMuSig2PartialSig, tc.expectErr,
				)

				return
			}

			parsed := assertMuSig2RoundTrip(
				t, typ, tc.partialSig,
				SerializeMuSig2PartialSig,
				ReadMuSig2PartialSig,
			)

			require.True(t, parsed.PubKey.IsEqual(
				tc.partialSig.PubKey,
			))
			require.True(t, parsed.AggregateKey.IsEqual(
				tc.partialSig.AggregateKey,
			))

			if len(tc.partialSig.TapLeafHash) == 0 {
				require.Empty(t, parsed.TapLeafHash)
			} else {
				require.Equal(
					t, tc.partialSig.TapLeafHash,
					parsed.TapLeafHash,
				)
			}

			// Only S makes it into the serialization, so R is
			// always nil on a parsed record.
			require.Equal(
				t, tc.partialSig.PartialSig.S,
				parsed.PartialSig.S,
			)
			require.Nil(t, parsed.PartialSig.R)
		})
	}
}

// TestMuSig2Validate_PreventsBrokenRoundTrip asserts that the records rejected
// by Validate would in fact serialize into PSBT key-value pairs that can no
// longer be parsed. The Serialize functions are bypassed here (the pair is
// written directly) to demonstrate what the Validate calls inside of them
// protect against.
func TestMuSig2Validate_PreventsBrokenRoundTrip(t *testing.T) {
	agg, keys := bip373Participants(t)

	readParticipants := func(keyData, value []byte) error {
		_, err := ReadMuSig2Participants(keyData, value)
		return err
	}
	readPubNonce := func(keyData, value []byte) error {
		_, err := ReadMuSig2PubNonce(keyData, value)
		return err
	}
	readPartialSig := func(keyData, value []byte) error {
		_, err := ReadMuSig2PartialSig(keyData, value)
		return err
	}

	shortLeaf := bytes.Repeat([]byte{0xab}, sha256.Size-1)
	longLeaf := bytes.Repeat([]byte{0xab}, sha256.Size+1)

	nonceValue := make([]byte, musig2.PubNonceSize)
	partialSigValue := make([]byte, 32)
	partialSigValue[31] = 0x01

	tests := []struct {
		name      string
		typ       uint8
		field     musig2Field
		value     []byte
		read      func(keyData, value []byte) error
		expectErr error
	}{
		{
			// Zero participant keys means a zero-length value,
			// which the read path rejects outright.
			name: "empty participant list",
			typ:  uint8(MuSig2ParticipantsInputType),
			field: &MuSig2Participants{
				AggregateKey: agg,
			},
			value:     []byte{},
			read:      readParticipants,
			expectErr: ErrInvalidPsbtFormat,
		},
		{
			name: "pub nonce with short tap leaf hash",
			typ:  uint8(MuSig2PubNoncesInputType),
			field: &MuSig2PubNonce{
				PubKey:       keys[0],
				AggregateKey: agg,
				TapLeafHash:  shortLeaf,
			},
			value:     nonceValue,
			read:      readPubNonce,
			expectErr: ErrInvalidKeyData,
		},
		{
			name: "pub nonce with long tap leaf hash",
			typ:  uint8(MuSig2PubNoncesInputType),
			field: &MuSig2PubNonce{
				PubKey:       keys[0],
				AggregateKey: agg,
				TapLeafHash:  longLeaf,
			},
			value:     nonceValue,
			read:      readPubNonce,
			expectErr: ErrInvalidKeyData,
		},
		{
			name: "partial sig with short tap leaf hash",
			typ:  uint8(MuSig2PartialSigsInputType),
			field: &MuSig2PartialSig{
				PubKey:       keys[0],
				AggregateKey: agg,
				TapLeafHash:  shortLeaf,
			},
			value:     partialSigValue,
			read:      readPartialSig,
			expectErr: ErrInvalidKeyData,
		},
		{
			name: "partial sig with long tap leaf hash",
			typ:  uint8(MuSig2PartialSigsInputType),
			field: &MuSig2PartialSig{
				PubKey:       keys[0],
				AggregateKey: agg,
				TapLeafHash:  longLeaf,
			},
			value:     partialSigValue,
			read:      readPartialSig,
			expectErr: ErrInvalidKeyData,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Validate must catch this before anything is written.
			require.Error(t, tc.field.Validate())

			// Now bypass Validate and write the key-value pair the
			// field would have produced, then read it back again.
			var buf bytes.Buffer
			require.NoError(t, serializeKVPairWithType(
				&buf, tc.typ, tc.field.KeyData(), tc.value,
			))

			_, keyData, value := readKVPair(t, &buf)
			require.ErrorIs(
				t, tc.read(keyData, value), tc.expectErr,
			)
		})
	}
}

// TestMuSig2Validate_PreventsPanic asserts that the missing key and missing
// signature cases rejected by Validate would otherwise panic deep inside the
// serialization code, where the nil pointers are dereferenced. That the
// Serialize functions return a clean error for those inputs instead is
// asserted by the per-field Validate tests above.
func TestMuSig2Validate_PreventsPanic(t *testing.T) {
	agg, keys := bip373Participants(t)

	var nilKey *btcec.PublicKey

	tests := []struct {
		name string
		call func()
	}{
		{
			name: "participants without aggregate key",
			call: func() {
				p := &MuSig2Participants{Keys: keys}
				_ = p.KeyData()
			},
		},
		{
			// This is the call SerializeMuSig2Participants makes
			// for every participant key when building the value.
			name: "participants with nil participant key",
			call: func() {
				_ = nilKey.SerializeCompressed()
			},
		},
		{
			name: "pub nonce without participant key",
			call: func() {
				n := &MuSig2PubNonce{AggregateKey: agg}
				_ = n.KeyData()
			},
		},
		{
			name: "pub nonce without aggregate key",
			call: func() {
				n := &MuSig2PubNonce{PubKey: keys[0]}
				_ = n.KeyData()
			},
		},
		{
			name: "partial sig without participant key",
			call: func() {
				s := &MuSig2PartialSig{AggregateKey: agg}
				_ = s.KeyData()
			},
		},
		{
			name: "partial sig without aggregate key",
			call: func() {
				s := &MuSig2PartialSig{PubKey: keys[0]}
				_ = s.KeyData()
			},
		},
		{
			name: "partial sig without S value",
			call: func() {
				var sig musig2.PartialSignature
				_ = sig.Encode(io.Discard)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Panics(t, tc.call)
		})
	}
}
