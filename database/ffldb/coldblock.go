// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Cold-tier block storage: witness-stripped, zstd-compressed block files.
//
// Design (see docs/ROADMAP.md, M1): blocks older than a rolling 2016-block hot
// window are age-out compacted into the cold tier. The cold tier stores each
// block as its non-witness (stripped) serialization, compressed via the
// blockcompress package, and discards the witness. The hot tier (recent blocks)
// is stored unchanged — full block, with witness, uncompressed — so the
// acceptance write path is untouched.
//
// Routing: the high bit of blockLocation.blockFileNum (coldFlag) marks a block
// as cold. This keeps the serialized block location at 12 bytes — no block-index
// migration — and lets readBlock route without opening the file first. Real cold
// file numbers are blockFileNum &^ coldFlag, which cannot approach 2^31 for any
// realistic chain (512 MiB files × 2^31 = 1 EiB).
//
// Cold files live in a separate subdirectory so scanBlockFiles (which only globs
// the base directory) ignores them and the hot write cursor never touches them.
// Cold READS reuse blockStore's open-file/LRU machinery: blockFilePath routes
// cold-flagged numbers to the cold subdirectory, and the openBlockFiles map is
// keyed by the flagged number, so cold and hot entries never collide.
//
// Cold file layout:
//
//	[coldFileMagic:8][formatVersion:1]   <- per-file header, written once
//	<record per block>
//
// where each record is:
//
//	<network:4><origStrippedLen:4><compressedStrippedBlock><crc:4>
//
// identical 12-byte overhead to the hot tier. origStrippedLen is the
// UNCOMPRESSED stripped-block length; the CRC covers the original uncompressed
// stripped bytes (crc32(network || origStrippedLen || strippedBlock)), so a
// codec bug that yields wrong bytes is caught the same way hot-tier corruption
// is. The blockcompress codec's zstd frame checksum validates the compressed
// stream additionally.
//
// blockLocation.blockLen for cold blocks is the on-disk record length (the header
// is per-file, not per-record), matching the hot tier's semantics.
package ffldb

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/blockcompress"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire/v2"
)

const (
	// coldFileSubdir is the subdirectory under the block store base path that
	// holds cold-tier block files. scanBlockFiles only globs the base
	// directory, so cold files are invisible to the hot write cursor.
	coldFileSubdir = "cold"

	// coldFlag is the high bit of blockLocation.blockFileNum. When set, the
	// block is in the cold tier and the real file number is
	// blockFileNum &^ coldFlag. It is stripped before building paths.
	coldFlag uint32 = 1 << 31

	// coldFileMagic is the 8-byte magic at the start of every cold block file,
	// used to identify the format on read.
	coldFileMagic = "BTCDCOLD"

	// coldHeaderLen is the length of the per-file cold header: magic + version.
	coldHeaderLen uint32 = uint32(len(coldFileMagic) + 1) // 9
)

// coldStore manages cold-tier block writes. It has its own write cursor,
// independent of the hot-tier write cursor, so age-out compaction never
// interferes with the acceptance write path. Cold reads are handled by
// blockStore.readBlock, which routes on the cold flag and reuses the shared
// open-file/LRU machinery.
type coldStore struct {
	basePath string
	network  wire.BitcoinNet

	// maxBlockFileSize caps each cold file size, matching the hot tier so file
	// counts stay comparable. Defined on the store for whitebox overrides.
	maxBlockFileSize uint32

	// codec is the deterministic zstd codec for the stripped-block stream,
	// bound to a single blockcompress.FormatVersion (pinned dictionary +
	// encoder level).
	codec *blockcompress.Codec

	// writeCursor tracks the current cold write file and offset. It is only
	// advanced during cold writes, which are serialized by the caller (the
	// age-out job).
	writeCursor *writeCursor

	// openWriteFileFunc opens (creating if needed) a cold file for read/write,
	// writes the per-file header if the file is new, and returns the file along
	// with the byte offset at which the next record should be written (the
	// header length for a new file, or the existing file size for an append).
	// Exposed for whitebox tests to substitute mock files.
	openWriteFileFunc func(fileNum uint32) (filer, uint32, error)
}

// newColdStore returns a cold store backed by the given base path's cold
// subdirectory, using codec version v for compression. It does not scan for
// existing cold files; the write cursor starts at file 0 offset coldHeaderLen
// (a fresh cold file with its header) and is advanced as blocks are written.
// A-3 (age-out compaction) wires up scan/resume of existing cold files.
func newColdStore(basePath string, network wire.BitcoinNet,
	v blockcompress.FormatVersion) (*coldStore, error) {

	codec, err := blockcompress.NewCodec(v)
	if err != nil {
		return nil, err
	}
	cs := &coldStore{
		basePath:         basePath,
		network:          network,
		maxBlockFileSize: maxBlockFileSize,
		codec:            codec,
		writeCursor: &writeCursor{
			curFile:    &lockableFile{},
			curFileNum: 0,
			curOffset:  0, // advanced to coldHeaderLen on first write
		},
	}
	cs.openWriteFileFunc = cs.openWriteFile
	return cs, nil
}

// Close releases the cold store's codec and any open write file.
func (cs *coldStore) Close() {
	if cs == nil {
		return
	}
	if cs.codec != nil {
		cs.codec.Close()
		cs.codec = nil
	}
	wc := cs.writeCursor
	if wc != nil && wc.curFile != nil {
		wc.curFile.Lock()
		if wc.curFile.file != nil {
			_ = wc.curFile.file.Close()
			wc.curFile.file = nil
		}
		wc.curFile.Unlock()
	}
}

// coldBlockFilePath returns the path of the cold block file for the given raw
// (unflagged) file number. Cold files live in a subdirectory so scanBlockFiles
// and the hot write cursor never see them.
func coldBlockFilePath(basePath string, fileNum uint32) string {
	fileName := fmt.Sprintf(blockFilenameTemplate, fileNum)
	return filepath.Join(basePath, coldFileSubdir, fileName)
}

// readColdHeader reads the per-file cold header from an open file and returns
// the blockcompress format version it selects. It validates the magic. The
// file's read position is not used; this reads via ReadAt at offset 0 so it is
// safe regardless of concurrent positional reads.
func readColdHeader(file filer) (blockcompress.FormatVersion, error) {
	hdr := make([]byte, int(coldHeaderLen))
	if _, err := file.ReadAt(hdr, 0); err != nil {
		return 0, fmt.Errorf("read header: %w", err)
	}
	if string(hdr[:len(coldFileMagic)]) != coldFileMagic {
		return 0, fmt.Errorf("not a cold file (bad magic)")
	}
	return blockcompress.FormatVersion(hdr[len(coldFileMagic)]), nil
}

// openWriteFile opens the cold block file for the given number in read/write
// mode, creating it if needed. If the file is new (zero size), the per-file
// cold header (magic + format version) is written so the file is self-
// identifying on read. It returns the file and the byte offset at which the
// next record should be written.
func (cs *coldStore) openWriteFile(fileNum uint32) (filer, uint32, error) {
	filePath := coldBlockFilePath(cs.basePath, fileNum)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, 0, makeDbErr(database.ErrDriverSpecific,
			fmt.Sprintf("failed to create cold block dir: %v", err), err)
	}
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0o666)
	if err != nil {
		return nil, 0, makeDbErr(database.ErrDriverSpecific,
			fmt.Sprintf("failed to open cold file %q: %v", filePath, err), err)
	}

	st, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, makeDbErr(database.ErrDriverSpecific,
			fmt.Sprintf("stat cold file %q: %v", filePath, err), err)
	}
	if st.Size() == 0 {
		// New file: write the per-file header.
		hdr := make([]byte, int(coldHeaderLen))
		copy(hdr, []byte(coldFileMagic))
		hdr[len(coldFileMagic)] = byte(cs.codec.Version())
		if _, err := file.WriteAt(hdr, 0); err != nil {
			_ = file.Close()
			return nil, 0, makeDbErr(database.ErrDriverSpecific,
				fmt.Sprintf("failed to write cold header %q: %v",
					filePath, err), err)
		}
		return file, coldHeaderLen, nil
	}
	return file, uint32(st.Size()), nil
}

// writeColdBlock appends the specified raw FULL block to the cold tier as a
// compressed stripped record: it deserializes the block, re-serializes without
// witness via SerializeNoWitness, compresses the stripped bytes, and writes the
// cold record. It returns a blockLocation with the cold flag set in
// blockFileNum. The caller (age-out job) updates the block index to point at
// this location.
//
// The raw block must be a full serialized block (with witness) as produced by
// wire.MsgBlock.Serialize.
func (cs *coldStore) writeColdBlock(rawFullBlock []byte) (blockLocation, error) {
	// Split: deserialize the full block, then re-serialize without witness.
	var blk wire.MsgBlock
	if err := blk.Deserialize(bytes.NewReader(rawFullBlock)); err != nil {
		return blockLocation{}, makeDbErr(database.ErrDriverSpecific,
			fmt.Sprintf("cold write: deserialize block: %v", err), err)
	}
	var stripped bytes.Buffer
	if err := blk.SerializeNoWitness(&stripped); err != nil {
		return blockLocation{}, makeDbErr(database.ErrDriverSpecific,
			fmt.Sprintf("cold write: serialize stripped: %v", err), err)
	}
	strippedBytes := stripped.Bytes()

	// Compress the stripped block.
	compressed, err := cs.codec.CompressBlock(strippedBytes)
	if err != nil {
		return blockLocation{}, makeDbErr(database.ErrDriverSpecific,
			fmt.Sprintf("cold write: compress: %v", err), err)
	}

	origStrippedLen := uint32(len(strippedBytes))
	fullLen := uint32(len(compressed)) + 12 // network + origLen + crc

	wc := cs.writeCursor

	// Move to the next cold file if this record would exceed the max size.
	// For a brand-new cold file (curOffset 0), account for the per-file header.
	finalOffset := wc.curOffset + fullLen
	if wc.curOffset == 0 {
		finalOffset += coldHeaderLen
	}
	if finalOffset < wc.curOffset || finalOffset > cs.maxBlockFileSize {
		wc.Lock()
		wc.curFile.Lock()
		if wc.curFile.file != nil {
			_ = wc.curFile.file.Close()
			wc.curFile.file = nil
		}
		wc.curFile.Unlock()
		wc.curFileNum++
		wc.curOffset = 0
		wc.Unlock()
	}

	wc.curFile.Lock()
	defer wc.curFile.Unlock()

	if wc.curFile.file == nil {
		file, startOffset, err := cs.openWriteFileFunc(wc.curFileNum)
		if err != nil {
			return blockLocation{}, err
		}
		wc.curFile.file = file
		wc.curOffset = startOffset
	}

	origOffset := wc.curOffset
	hasher := crc32.New(castagnoli)
	var scratch [4]byte

	// Network.
	byteOrder.PutUint32(scratch[:], uint32(cs.network))
	if err := writeAt(wc, scratch[:], "network"); err != nil {
		return blockLocation{}, err
	}
	_, _ = hasher.Write(scratch[:])

	// Original (uncompressed) stripped length.
	byteOrder.PutUint32(scratch[:], origStrippedLen)
	if err := writeAt(wc, scratch[:], "orig stripped len"); err != nil {
		return blockLocation{}, err
	}
	_, _ = hasher.Write(scratch[:])

	// Compressed stripped block.
	if err := writeAt(wc, compressed, "compressed stripped block"); err != nil {
		return blockLocation{}, err
	}
	_, _ = hasher.Write(strippedBytes) // CRC over the ORIGINAL uncompressed bytes

	// CRC.
	if err := writeAt(wc, hasher.Sum(nil), "checksum"); err != nil {
		return blockLocation{}, err
	}

	loc := blockLocation{
		blockFileNum: wc.curFileNum | coldFlag,
		fileOffset:   origOffset,
		blockLen:     fullLen,
	}
	return loc, nil
}

// writeAt writes data at the cold write cursor's current offset and advances
// the offset. It is the cold analog of blockStore.writeData.
func writeAt(wc *writeCursor, data []byte, fieldName string) error {
	n, err := wc.curFile.file.WriteAt(data, int64(wc.curOffset))
	wc.curOffset += uint32(n)
	if err != nil {
		str := fmt.Sprintf("failed to write %s to cold file %d at "+
			"offset %d: %v", fieldName, wc.curFileNum,
			wc.curOffset-uint32(n), err)
		return makeDbErr(database.ErrDriverSpecific, str, err)
	}
	return nil
}
