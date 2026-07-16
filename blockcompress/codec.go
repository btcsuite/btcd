// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package blockcompress provides a deterministic, lossless zstd codec for
// Bitcoin block data, with a Bitcoin-script-trained dictionary pinned per
// format version.
//
// Determinism invariants (see docs/ROADMAP.md, M1):
//
//  1. Decompression is a pure function of (compressed stream, dictionary).
//     Given the same bytes and dictionary, every node produces identical
//     output, always, across library versions.
//  2. The dictionary is selected by the format version, not by the node. A
//     block compressed with dictionary v1 is always decoded with v1.
//  3. The encoder level and dictionary bytes are fixed by the format version,
//     so all nodes on the same version produce identical compressed files.
//
// The codec never touches consensus code: it operates on raw block byte slices
// below the database interface.
package blockcompress

import (
	"errors"
	"fmt"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// FormatVersion identifies a compression format version. It selects the
// dictionary and encoder configuration used for both compression and
// decompression. It is stored in the per-file header of compressed block files
// so that a reader always knows which dictionary to use.
//
// Version 0 is reserved for "no compression / legacy uncompressed file" and is
// not a valid Codec version.
type FormatVersion uint8

const (
	// FormatV1 is the first compressed block file format version.
	FormatV1 FormatVersion = 1
)

// EncoderLevel is the zstd compression strength, pinned per format version so
// that all nodes produce identical output.
type EncoderLevel int

const (
	// LevelDefault is a balanced ratio/throughput setting suitable for the
	// IBD write path, where writes are not the bottleneck.
	LevelDefault EncoderLevel = 3
)

// Codec is a deterministic zstd codec for Bitcoin block data.
//
// A Codec is bound to a single FormatVersion and is safe for concurrent use:
// the underlying zstd encoder and decoder are goroutine-safe per the
// klauspost/compress/zstd contract (EncodeAll/DecodeAll do not mutate shared
// encoder/decoder state in a way that conflicts between concurrent calls).
type Codec struct {
	version  FormatVersion
	encoder  *zstd.Encoder
	decoder  *zstd.Decoder
	dict     []byte
	level    EncoderLevel
	initOnce sync.Once
	initErr  error
}

// NewCodec returns a Codec bound to the given format version. The dictionary
// and encoder level are selected from the version's registered configuration.
//
// Construction is eager: a misconfiguration (bad dictionary, etc.) fails here
// rather than mid-IBD.
func NewCodec(v FormatVersion) (*Codec, error) {
	cfg, ok := registeredVersions[v]
	if !ok {
		return nil, fmt.Errorf("blockcompress: unknown format version %d", v)
	}
	if v == 0 {
		return nil, errors.New("blockcompress: version 0 is reserved for " +
			"uncompressed files and is not a valid Codec version")
	}

	c := &Codec{
		version: v,
		dict:    cfg.dict,
		level:   cfg.level,
	}
	if err := c.init(); err != nil {
		return nil, err
	}
	return c, nil
}

// Version returns the format version this codec is bound to.
func (c *Codec) Version() FormatVersion { return c.version }

// init constructs the zstd encoder and decoder with the version-pinned
// dictionary and level. It is idempotent under initOnce.
func (c *Codec) init() error {
	c.initOnce.Do(func() {
		var encOpts []zstd.EOption
		if len(c.dict) > 0 {
			// Use a stable, version-derived dictionary ID so all nodes
			// on the same format version emit identical frames.
			encOpts = append(encOpts,
				zstd.WithEncoderDictRaw(uint32(c.version), c.dict))
		}
		encOpts = append(encOpts,
			zstd.WithEncoderLevel(zstd.EncoderLevel(c.level)),
			// Force deterministic output: single-threaded encoding so no
			// concurrency-adaptive buffering can vary output ordering.
			zstd.WithEncoderConcurrency(1),
			zstd.WithZeroFrames(false),
		)
		enc, err := zstd.NewWriter(nil, encOpts...)
		if err != nil {
			c.initErr = fmt.Errorf("blockcompress: encoder init: %w", err)
			return
		}
		c.encoder = enc

		var decOpts []zstd.DOption
		if len(c.dict) > 0 {
			decOpts = append(decOpts,
				zstd.WithDecoderDictRaw(uint32(c.version), c.dict))
		}
		dec, err := zstd.NewReader(nil, decOpts...)
		if err != nil {
			c.initErr = fmt.Errorf("blockcompress: decoder init: %w", err)
			return
		}
		c.decoder = dec
	})
	return c.initErr
}

// CompressBlock compresses the raw block bytes deterministically. The output
// is a complete zstd frame readable by DecompressBlock with the same format
// version. For a given Codec, the output is identical across nodes and runs.
func (c *Codec) CompressBlock(raw []byte) ([]byte, error) {
	if c == nil {
		return nil, errors.New("blockcompress: nil codec")
	}
	if err := c.init(); err != nil {
		return nil, err
	}
	return c.encoder.EncodeAll(raw, make([]byte, 0, len(raw))), nil
}

// DecompressBlock decompresses a zstd frame produced by CompressBlock with a
// codec of the same format version. The output is byte-identical to the
// original raw block. A corrupted stream or dictionary mismatch yields an
// error from the zstd frame check rather than silent wrong bytes.
func (c *Codec) DecompressBlock(comp []byte) ([]byte, error) {
	if c == nil {
		return nil, errors.New("blockcompress: nil codec")
	}
	if err := c.init(); err != nil {
		return nil, err
	}
	return c.decoder.DecodeAll(comp, make([]byte, 0, len(comp)))
}

// Close releases the underlying zstd encoder/decoder resources. It is safe to
// call multiple times. A Codec must not be used after Close.
func (c *Codec) Close() {
	if c == nil {
		return
	}
	if c.encoder != nil {
		_ = c.encoder.Close()
		c.encoder = nil
	}
	if c.decoder != nil {
		c.decoder.Close()
		c.decoder = nil
	}
}

// MaxEncodedSize returns an upper bound on the compressed size of a block of
// the given uncompressed length. zstd may emit a slightly larger frame for
// incompressible input; this bound accounts for that so callers can preallocate
// safely. The bound is generous: zstd's worst-case overhead is small relative
// to a Bitcoin block but we over-provision to avoid any reallocation surprise.
func MaxEncodedSize(uncompressedLen int) int {
	// zstd worst case frame overhead is ~18 bytes plus a 3-byte block header
	// per incompressible 128KB block. For a 4MB max Bitcoin block this is at
	// most ~96 bytes of headers. We add a flat 1% margin on top.
	if uncompressedLen == 0 {
		return 64
	}
	overhead := uncompressedLen/128_000*3 + 18
	margin := uncompressedLen / 100
	return uncompressedLen + overhead + margin + 64
}
