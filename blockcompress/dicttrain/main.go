//go:build ignore

// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Command dicttrain trains a zstd dictionary on real Bitcoin mainnet blocks
// for use with the blockcompress codec.
//
// The dictionary is trained on the witness-stripped serialization of blocks,
// matching the cold-tier storage format. The output is a raw zstd dictionary
// file that can be embedded via //go:embed in dictionary_assets.go.
//
// Usage:
//
//	go run blockcompress/dicttrain/main.go -datadir /path/to/praxisd/blocks_ffldb \
//		-maxblocks 5000 -out blockcompress/v1.dict
//
// The datadir must contain ffldb-format block files (*.fdb). The tool streams
// blocks from evenly-spaced files across the chain and reads only the first few
// blocks per file, so it works efficiently even over NFS without reading
// terabytes of data.
//
// The tool does NOT modify the datadir; it only reads from it.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/btcsuite/btcd/wire/v2"
	"github.com/klauspost/compress/dict"
	"github.com/klauspost/compress/zstd"
)

const (
	blockFileExtension = ".fdb"
	recordHeaderLen    = 8 // <network:4><blockLen:4>
	recordTrailerLen   = 4 // <crc:4>
)

func main() {
	datadir := flag.String("datadir", "", "path to ffldb blocks directory containing .fdb files")
	maxBlocks := flag.Int("maxblocks", 200, "maximum number of blocks to sample for training")
	maxDictSize := flag.Int("maxdictsize", 112640, "maximum dictionary size in bytes (110KB is zstd's default)")
	blocksPerFile := flag.Int("blocksperfile", 1, "number of blocks to read from each sampled file (1 = minimal memory)")
	maxSampleLen := flag.Int("maxsamplelen", 4096, "truncate each sample to at most N bytes (keeps memory low)")
	outFile := flag.String("out", "blockcompress/v1.dict", "output dictionary file path")
	flag.Parse()

	if *datadir == "" {
		fmt.Fprintln(os.Stderr, "error: -datadir is required")
		flag.Usage()
		os.Exit(1)
	}

	files, err := filepath.Glob(filepath.Join(*datadir, "*"+blockFileExtension))
	if err != nil {
		fmt.Fprintf(os.Stderr, "glob error: %v\n", err)
		os.Exit(1)
	}
	sort.Strings(files)
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "no block files found in %s\n", *datadir)
		os.Exit(1)
	}
	fmt.Printf("Found %d block files in %s\n", len(files), *datadir)

	// Evenly space file selections across the chain to sample different eras.
	numFilesToSample := *maxBlocks / *blocksPerFile
	if numFilesToSample < 1 {
		numFilesToSample = 1
	}
	if numFilesToSample > len(files) {
		numFilesToSample = len(files)
	}

	var selectedFiles []string
	for i := 0; i < numFilesToSample; i++ {
		idx := i * len(files) / numFilesToSample
		selectedFiles = append(selectedFiles, files[idx])
	}
	fmt.Printf("Sampling %d files (%d blocks each = up to %d blocks)\n",
		len(selectedFiles), *blocksPerFile, len(selectedFiles)**blocksPerFile)

	// Stream-read blocks from selected files.
	samples := readStrippedBlocks(selectedFiles, *blocksPerFile, *maxBlocks, *maxSampleLen)
	fmt.Printf("Read %d stripped block samples (%.1f MB total)\n",
		len(samples), float64(totalBytes(samples))/1024/1024)

	if len(samples) < 100 {
		fmt.Fprintf(os.Stderr, "need at least 100 samples for training, got %d\n",
			len(samples))
		os.Exit(1)
	}

	// Train the dictionary.
	fmt.Printf("Training dictionary (max size %d bytes)...\n", *maxDictSize)
	start := time.Now()
	dictBytes, err := dict.BuildZstdDict(samples, dict.Options{
		MaxDictSize: *maxDictSize,
		HashBytes:   4,
		ZstdDictID:  1, // stable, version-pinned dictionary ID
		Output:      os.Stderr,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "dict build: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Dictionary built: %d bytes in %v\n", len(dictBytes), time.Since(start))

	// Write the dictionary.
	if err := os.WriteFile(*outFile, dictBytes, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write dict: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Dictionary written to %s\n", *outFile)

	// Benchmark: compare compression with and without dictionary on the samples.
	benchmarkDict(samples, dictBytes)
}

// readStrippedBlocks stream-reads blocks from the given files, strips witness
// data, and returns up to maxBlocks samples. It reads only blocksPerFile blocks
// from each file using streaming I/O (no full-file reads).
func readStrippedBlocks(files []string, blocksPerFile, maxBlocks, maxSampleLen int) [][]byte {
	var samples [][]byte
	for _, file := range files {
		if len(samples) >= maxBlocks {
			break
		}
		fileSamples, err := streamBlocksFromFile(file, blocksPerFile, maxSampleLen)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", file, err)
			continue
		}
		samples = append(samples, fileSamples...)
	}
	return samples
}

// streamBlocksFromFile reads up to maxBlocks blocks from a single ffldb flat
// file using streaming I/O. Each record is:
//
//	<network:4><blockLen:4><rawBlock:blockLen><crc:4>
//
// Only the first maxBlocks records are read; the rest of the file is never
// touched, so this is efficient even on 512MB files over NFS.
func streamBlocksFromFile(path string, maxBlocks, maxSampleLen int) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var samples [][]byte
	for len(samples) < maxBlocks {
		// Read the 8-byte record header.
		var header [recordHeaderLen]byte
		if _, err := io.ReadFull(f, header[:]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break // end of file
			}
			return samples, fmt.Errorf("read header: %w", err)
		}

		network := binary.LittleEndian.Uint32(header[0:4])
		blockLen := binary.LittleEndian.Uint32(header[4:8])

		// Validate network magic.
		if !isValidNetwork(network) {
			break // not a valid record; might be cold-tier or corrupt
		}
		if blockLen == 0 || blockLen > 8<<20 { // 8MiB sanity cap
			break
		}

		// Read the block payload.
		rawBlock := make([]byte, blockLen)
		if _, err := io.ReadFull(f, rawBlock); err != nil {
			break
		}

		// Skip the 4-byte CRC.
		if _, err := f.Seek(recordTrailerLen, io.SeekCurrent); err != nil {
			break
		}

		// Strip witness data.
		var blk wire.MsgBlock
		if err := blk.Deserialize(bytes.NewReader(rawBlock)); err != nil {
			continue
		}
		var stripped bytes.Buffer
		if err := blk.SerializeNoWitness(&stripped); err != nil {
			continue
		}
		s := stripped.Bytes()
		if maxSampleLen > 0 && len(s) > maxSampleLen {
			s = s[:maxSampleLen]
		}
		// Copy to avoid pinning the larger buffer.
		sample := make([]byte, len(s))
		copy(sample, s)
		samples = append(samples, sample)
	}

	return samples, nil
}

// isValidNetwork returns true if the magic matches a known Bitcoin network.
func isValidNetwork(net uint32) bool {
	return net == uint32(wire.MainNet) ||
		net == uint32(wire.TestNet3) ||
		net == uint32(wire.TestNet) ||
		net == uint32(wire.SimNet) ||
		net == uint32(wire.TestNet4) ||
		net == uint32(wire.SigNet)
}

// totalBytes returns the total byte count across all samples.
func totalBytes(samples [][]byte) int {
	total := 0
	for _, s := range samples {
		total += len(s)
	}
	return total
}

// benchmarkDict compares compression ratios with and without the dictionary
// on the training samples, reporting per-block and blended ratios.
func benchmarkDict(samples [][]byte, dictBytes []byte) {
	fmt.Println("\n--- Compression Benchmark ---")

	encNoDict, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.EncoderLevel(3)),
		zstd.WithEncoderConcurrency(1),
		zstd.WithZeroFrames(false),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "enc no dict: %v\n", err)
		return
	}
	defer encNoDict.Close()

	encWithDict, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.EncoderLevel(3)),
		zstd.WithEncoderConcurrency(1),
		zstd.WithZeroFrames(false),
		zstd.WithEncoderDictRaw(1, dictBytes),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "enc with dict: %v\n", err)
		return
	}
	defer encWithDict.Close()

	var totalRaw, totalNoDict, totalWithDict int
	for i, sample := range samples {
		compNoDict := encNoDict.EncodeAll(sample, make([]byte, 0, len(sample)))
		compWithDict := encWithDict.EncodeAll(sample, make([]byte, 0, len(sample)))
		totalRaw += len(sample)
		totalNoDict += len(compNoDict)
		totalWithDict += len(compWithDict)
		if i < 5 || i%1000 == 0 {
			fmt.Printf("  block %d: raw=%d noDict=%d (%.1f%%) withDict=%d (%.1f%%)\n",
				i, len(sample), len(compNoDict),
				ratio(len(sample), len(compNoDict)),
				len(compWithDict), ratio(len(sample), len(compWithDict)))
		}
	}

	fmt.Printf("\nBlended (%d blocks, %.1f MB):\n", len(samples),
		float64(totalRaw)/1024/1024)
	fmt.Printf("  no dict:   %d bytes (%.1f%% reduction)\n",
		totalNoDict, ratio(totalRaw, totalNoDict))
	fmt.Printf("  with dict: %d bytes (%.1f%% reduction)\n",
		totalWithDict, ratio(totalRaw, totalWithDict))
	improvement := ratio(totalRaw, totalWithDict) - ratio(totalRaw, totalNoDict)
	fmt.Printf("  dict improvement: %.1f percentage points\n", improvement)
}

// ratio returns the percentage reduction: 100 * (1 - compressed/raw).
func ratio(raw, compressed int) float64 {
	if raw == 0 {
		return 0
	}
	return 100.0 * (1 - float64(compressed)/float64(raw))
}
