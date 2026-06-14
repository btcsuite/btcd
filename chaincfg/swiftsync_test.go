// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math"
	"math/rand"
	"testing"

	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestDecodeHintsBlock verifies the per-block hints decoder against known
// vectors.  Each entry pairs a sorted slice of indices with the hex of its
// serialized hints entry.  Decoding the bytes must produce the original
// slice.
func TestDecodeHintsBlock(t *testing.T) {
	tests := []struct {
		name      string
		encodedHx string
		want      []uint32
	}{
		{
			name:      "empty",
			encodedHx: "00",
			want:      nil,
		},
		{
			name:      "consecutive from zero",
			encodedHx: "0400000000",
			want:      []uint32{0, 1, 2, 3},
		},
		{
			name:      "regular gaps",
			encodedHx: "0a0d020202020202020202",
			want: []uint32{
				13, 16, 19, 22, 25, 28, 31, 34, 37, 40,
			},
		},
		{
			name:      "single multi-byte index",
			encodedHx: "03fd2c01",
			want:      []uint32{300},
		},
		{
			name:      "mixed gap widths",
			encodedHx: "0405fdee01",
			want:      []uint32{5, 500},
		},
		{
			name:      "max index",
			encodedHx: "05feffffffff",
			want:      []uint32{math.MaxUint32},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := hex.DecodeString(tt.encodedHx)
			require.NoError(t, err)

			got, consumed, err := decodeHintsBlock(raw)
			require.NoError(t, err)
			require.Equal(t, len(raw), consumed,
				"decoder did not consume the whole block")
			require.Equal(t, tt.want, got)
		})
	}
}

// TestDecodeHintsBlockErrors exercises the malformed-input paths of the
// per-block hints decoder.
func TestDecodeHintsBlockErrors(t *testing.T) {
	tests := []struct {
		name      string
		encodedHx string
		wantErr   string
	}{
		{
			name:      "empty buffer",
			encodedHx: "",
			wantErr:   "read byte length",
		},
		{
			name:      "declared length exceeds buffer",
			encodedHx: "05ff",
			wantErr:   "truncated",
		},
		{
			name:      "value crosses payload boundary",
			encodedHx: "01fd",
			wantErr:   "read index 0",
		},
		{
			name: "value must not borrow bytes past the payload",
			// Declared 1-byte payload holding a 0xfd prefix whose
			// continuation bytes sit beyond the payload boundary.
			encodedHx: "01fd0001",
			wantErr:   "read index 0",
		},
		{
			name: "index overflows uint32",
			// A single 9-byte value encoding 2^32.
			encodedHx: "09ff0000000001000000",
			wantErr:   "overflows uint32",
		},
		{
			name: "accumulated index overflows uint32",
			// Index 4294967295 followed by another value.
			encodedHx: "06feffffffff00",
			wantErr:   "overflows uint32",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := hex.DecodeString(tt.encodedHx)
			require.NoError(t, err)

			_, _, err = decodeHintsBlock(raw)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// encodeHintsBlock serializes indices using the same per-block hints encoding
// the decoder expects: a CompactSize byte length followed by CompactSize
// values where the first value is the first index and every subsequent value
// is the gap to the previous index minus one.
func encodeHintsBlock(t *testing.T, indices []uint32) []byte {
	t.Helper()

	var payload bytes.Buffer
	var next uint64
	for _, index := range indices {
		err := wire.WriteVarInt(&payload, 0, uint64(index)-next)
		require.NoError(t, err)
		next = uint64(index) + 1
	}

	var out bytes.Buffer
	err := wire.WriteVarInt(&out, 0, uint64(payload.Len()))
	require.NoError(t, err)
	_, err = out.Write(payload.Bytes())
	require.NoError(t, err)

	return out.Bytes()
}

// roundTripCase is a single TestSwiftSyncHintsRoundTrip scenario: the unspent
// index list for each block of a hintsfile.
type roundTripCase struct {
	name   string
	blocks [][]uint32
}

// TestSwiftSyncHintsRoundTrip builds a hintsfile from per-block unspent index
// lists, parses it back, and verifies that HintsForBlock returns the original
// indices for every block and errors out of range.  It covers fixed vectors
// and randomly generated index sets, exercising the encoder, parser, and
// decoder (HintsForBlock decodes each block) together.
func TestSwiftSyncHintsRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1))

	// randomBlock returns a strictly increasing index list mixing small gaps
	// with occasionally huge ones.
	randomBlock := func() []uint32 {
		indices := make([]uint32, 0, 200)
		next := uint64(0)
		for j := rng.Intn(200); j > 0; j-- {
			var gap uint64
			switch rng.Intn(4) {
			case 0:
				gap = 0
			case 1:
				gap = uint64(rng.Intn(300))
			default:
				gap = uint64(rng.Intn(1 << 20))
			}
			next += gap
			if next > math.MaxUint32 {
				break
			}
			indices = append(indices, uint32(next))
			next++
		}
		return indices
	}

	tests := []roundTripCase{
		{
			name: "mixed gaps, consecutive, and empty",
			blocks: [][]uint32{
				{13, 16, 19, 22, 25, 28, 31, 34, 37, 40},
				{0, 1, 2, 3},
				{5, 500},
				nil,
			},
		},
		{name: "single empty block", blocks: [][]uint32{nil}},
		{name: "max index", blocks: [][]uint32{{math.MaxUint32}}},
	}

	// Append randomly generated multi-block scenarios to fuzz the round trip.
	for i := 0; i < 200; i++ {
		blocks := make([][]uint32, 1+rng.Intn(4))
		for j := range blocks {
			blocks[j] = randomBlock()
		}
		tests = append(tests, roundTripCase{name: "random", blocks: blocks})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			height := int32(len(tt.blocks))

			raw := append([]byte{}, swiftSyncHintsMagic[:]...)
			raw = append(raw, swiftSyncHintsVersion)
			var heightLE [4]byte
			binary.LittleEndian.PutUint32(heightLE[:], uint32(height))
			raw = append(raw, heightLE[:]...)
			for _, indices := range tt.blocks {
				raw = append(raw, encodeHintsBlock(t, indices)...)
			}

			data, err := ParseSwiftSyncHints(raw, height)
			require.NoError(t, err)
			require.Equal(t, height, data.Height)
			require.Len(t, data.offsets, int(height))

			for i, want := range tt.blocks {
				got, err := data.HintsForBlock(int32(i + 1))
				require.NoError(t, err)
				if len(want) == 0 {
					require.Empty(t, got)
				} else {
					require.Equal(t, want, got, "block %d", i+1)
				}
			}

			// Out-of-range heights must error.
			_, err = data.HintsForBlock(0)
			require.Error(t, err)
			_, err = data.HintsForBlock(height + 1)
			require.Error(t, err)
		})
	}
}

// TestParseSwiftSyncHintsRejects verifies that ParseSwiftSyncHints refuses
// malformed hintsfiles.
func TestParseSwiftSyncHintsRejects(t *testing.T) {
	// header returns a hintsfile header (magic, version, height LE).
	header := func(version byte, height uint32) []byte {
		raw := append([]byte{}, swiftSyncHintsMagic[:]...)
		raw = append(raw, version)
		var heightLE [4]byte
		binary.LittleEndian.PutUint32(heightLE[:], height)
		return append(raw, heightLE[:]...)
	}

	tests := []struct {
		name           string
		raw            []byte
		expectedHeight int32
		wantErrs       []string
	}{
		{
			name:           "bad magic",
			raw:            make([]byte, swiftSyncHintsHeaderSize),
			expectedHeight: 0,
			wantErrs:       []string{"bad magic"},
		},
		{
			name:           "version below supported",
			raw:            header(swiftSyncHintsVersion-1, 0),
			expectedHeight: 0,
			wantErrs:       []string{"unsupported version"},
		},
		{
			name:           "version above supported",
			raw:            header(swiftSyncHintsVersion+1, 0),
			expectedHeight: 0,
			wantErrs:       []string{"unsupported version"},
		},
		{
			name:           "height mismatch",
			raw:            header(swiftSyncHintsVersion, 7),
			expectedHeight: 99,
			wantErrs:       []string{"does not match expected"},
		},
		{
			name: "truncated body",
			// Block 1 is an empty entry; block 2 declares 5 bytes but
			// only has 1.
			raw: append(header(swiftSyncHintsVersion, 2),
				0x00, 0x05, 0xff),
			expectedHeight: 2,
			wantErrs:       []string{"block 2", "truncated"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSwiftSyncHints(tt.raw, tt.expectedHeight)
			for _, want := range tt.wantErrs {
				require.ErrorContains(t, err, want)
			}
		})
	}
}

// TestSigNetSwiftSyncCheckpoint verifies signet's swift sync boundary and the
// hintsfile hash the loader validates against.
func TestSigNetSwiftSyncCheckpoint(t *testing.T) {
	cp := SigNetParams.SwiftSync
	require.NotNil(t, cp)
	require.Equal(t, int32(285205), cp.Height)
	require.Equal(t,
		"df2a00fe29905bbc1897d23a04f169ea2b5e909b53e55dacc41eb1ab9d04edde",
		cp.HintsHash)
}

// TestSwiftSyncBoundaryIsCheckpoint enforces, for every network that enables
// swift sync, that the boundary is a checkpoint.  Swift sync skips signature
// validation up to the boundary, so the boundary must be a checkpoint that
// freezes the block and supplies its trusted hash.
func TestSwiftSyncBoundaryIsCheckpoint(t *testing.T) {
	for _, p := range []*Params{
		&MainNetParams, &TestNet3Params, &TestNet4Params,
		&SigNetParams, &SimNetParams, &RegressionNetParams,
	} {
		if p.SwiftSync == nil {
			continue // swift sync not supported on this network
		}
		var found bool
		for _, cp := range p.Checkpoints {
			if cp.Height == p.SwiftSync.Height {
				found = true
				break
			}
		}
		require.Truef(t, found,
			"%s: swift sync boundary height %d is not a checkpoint",
			p.Name, p.SwiftSync.Height)
	}
}
