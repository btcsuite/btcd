// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"encoding/binary"
	"fmt"
	"math"
)

// SwiftSyncData holds a parsed swift sync hintsfile for a network.
// The hintsfile encodes, for each block at heights 1..Height, the indices of
// the outputs that remain unspent at Height — restricted to outputs the
// hintsfile considers spendable (i.e. not OP_RETURN-leading, not oversized,
// and not the BIP-30 duplicate coinbases).  During initial sync, btcd uses
// these hints to construct the UTXO set without replaying historical spends.
//
// The on-disk format is:
//
//	+----------+-----------+----------+----------------------------+
//	| Magic    | Version   | Height   | Per-block hints            |
//	| 4 bytes  | 1 byte    | u32 LE   | Hints[1], Hints[2], ...    |
//	+----------+-----------+----------+----------------------------+
//
// Each per-block hints entry is a CompactSize byte length followed by that
// many bytes of CompactSize values encoding the block's strictly increasing
// unspent output indices: the first value is the first index, and every
// subsequent value is the gap to the previous index minus one (consecutive
// indices therefore encode as 0x00).
type SwiftSyncData struct {
	// Height is the block height up to which the hintsfile covers.  Blocks 1
	// through Height (inclusive) can be processed via swift sync.
	Height int32

	// body is the per-block hints section of the hintsfile (i.e. the file
	// bytes immediately after the header).
	body []byte

	// offsets[i] is the byte offset within body of the hints entry for
	// block height i+1.  len(offsets) == int(Height).
	offsets []uint32
}

// HintsForBlock returns the unspent spendable output indices for the block at
// the given 1-indexed height.  The returned slice is freshly allocated and
// owned by the caller.
func (s *SwiftSyncData) HintsForBlock(height int32) ([]uint32, error) {
	if height < 1 || height > s.Height {
		return nil, fmt.Errorf("hintsfile: height %d out of range [1, %d]",
			height, s.Height)
	}
	start := s.offsets[height-1]
	indices, _, err := decodeHintsBlock(s.body[start:])
	if err != nil {
		return nil, fmt.Errorf("hintsfile: block %d: %w", height, err)
	}
	return indices, nil
}

const (
	// swiftSyncHintsHeaderSize is 4 (magic) + 1 (version) + 4 (height LE).
	swiftSyncHintsHeaderSize = 9
	swiftSyncHintsVersion    = 0x01
)

// swiftSyncHintsMagic is the hintsfile magic, the ASCII bytes "UTXO".
var swiftSyncHintsMagic = [4]byte{0x55, 0x54, 0x58, 0x4f}

// ParseSwiftSyncHints validates a hintsfile blob and returns a SwiftSyncData
// whose body aliases (zero-copy) the per-block hints section and whose offsets
// index every block.  It enforces that the file's recorded height equals
// expectedHeight.
//
// btcd loads the hintsfile from disk at runtime (it is too large to embed) and
// validates it against the boundary returned by Params.SwiftSyncCheckpoint.
func ParseSwiftSyncHints(raw []byte, expectedHeight int32) (*SwiftSyncData, error) {
	if len(raw) < swiftSyncHintsHeaderSize {
		return nil, fmt.Errorf("header truncated (%d < %d)",
			len(raw), swiftSyncHintsHeaderSize)
	}
	var magic [4]byte
	copy(magic[:], raw[:4])
	if magic != swiftSyncHintsMagic {
		return nil, fmt.Errorf("bad magic %x", magic)
	}
	if raw[4] != swiftSyncHintsVersion {
		return nil, fmt.Errorf("unsupported version 0x%02x", raw[4])
	}
	fileHeight := binary.LittleEndian.Uint32(raw[5:9])
	if fileHeight > math.MaxInt32 {
		return nil, fmt.Errorf("height %d exceeds int32", fileHeight)
	}
	if int32(fileHeight) != expectedHeight {
		return nil, fmt.Errorf("height %d does not match expected %d",
			fileHeight, expectedHeight)
	}

	body := raw[swiftSyncHintsHeaderSize:]
	offsets := make([]uint32, fileHeight)
	var pos uint64
	for h := uint32(0); h < fileHeight; h++ {
		if pos > math.MaxUint32 {
			return nil, fmt.Errorf("body exceeds 4 GiB")
		}
		offsets[h] = uint32(pos)
		size, err := skipHintsBlock(body[pos:])
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", h+1, err)
		}
		pos += uint64(size)
		if pos > uint64(len(body)) {
			return nil, fmt.Errorf("block %d overflows body", h+1)
		}
	}
	if pos != uint64(len(body)) {
		return nil, fmt.Errorf("%d trailing bytes after last block",
			uint64(len(body))-pos)
	}

	return &SwiftSyncData{
		Height:  int32(fileHeight),
		body:    body,
		offsets: offsets,
	}, nil
}

// readCompactSize parses a Bitcoin CompactSize (varint) from buf, returning
// the parsed value and the number of bytes consumed.
func readCompactSize(buf []byte) (uint64, int, error) {
	if len(buf) < 1 {
		return 0, 0, fmt.Errorf("CompactSize: empty")
	}
	first := buf[0]
	switch {
	case first < 0xfd:
		return uint64(first), 1, nil
	case first == 0xfd:
		if len(buf) < 3 {
			return 0, 0, fmt.Errorf("CompactSize 0xfd: truncated")
		}
		return uint64(binary.LittleEndian.Uint16(buf[1:3])), 3, nil
	case first == 0xfe:
		if len(buf) < 5 {
			return 0, 0, fmt.Errorf("CompactSize 0xfe: truncated")
		}
		return uint64(binary.LittleEndian.Uint32(buf[1:5])), 5, nil
	default: // 0xff
		if len(buf) < 9 {
			return 0, 0, fmt.Errorf("CompactSize 0xff: truncated")
		}
		return binary.LittleEndian.Uint64(buf[1:9]), 9, nil
	}
}

// skipHintsBlock returns the number of bytes the next per-block hints entry
// occupies in buf without decoding its indices.
func skipHintsBlock(buf []byte) (int, error) {
	byteLen, lenSize, err := readCompactSize(buf)
	if err != nil {
		return 0, fmt.Errorf("read byte length: %w", err)
	}
	if byteLen > uint64(len(buf)-lenSize) {
		return 0, fmt.Errorf("hints block truncated")
	}
	return lenSize + int(byteLen), nil
}

// decodeHintsBlock decodes the next per-block hints entry from buf, returning
// the recovered indices and the number of bytes consumed.
//
// The entry is a CompactSize byte length followed by that many bytes of
// CompactSize values: the first value is the first index, and every
// subsequent value is the gap to the previous index minus one.
func decodeHintsBlock(buf []byte) ([]uint32, int, error) {
	byteLen, lenSize, err := readCompactSize(buf)
	if err != nil {
		return nil, 0, fmt.Errorf("read byte length: %w", err)
	}
	if byteLen > uint64(len(buf)-lenSize) {
		return nil, 0, fmt.Errorf("hints block truncated")
	}
	if byteLen == 0 {
		return nil, lenSize, nil
	}

	payload := buf[lenSize : lenSize+int(byteLen)]

	// Every value occupies at least one byte, so len(payload) bounds the
	// number of indices.
	indices := make([]uint32, 0, len(payload))

	// next is the smallest value the upcoming index may take: 0 for the
	// first index, previous index + 1 afterwards.
	var next uint64
	var pos int
	for pos < len(payload) {
		gap, gapSize, err := readCompactSize(payload[pos:])
		if err != nil {
			return nil, 0, fmt.Errorf("read index %d: %w",
				len(indices), err)
		}
		pos += gapSize

		if gap > math.MaxUint32 || next+gap > math.MaxUint32 {
			return nil, 0, fmt.Errorf("index %d overflows uint32",
				len(indices))
		}
		next += gap
		indices = append(indices, uint32(next))
		next++
	}

	return indices, lenSize + int(byteLen), nil
}
