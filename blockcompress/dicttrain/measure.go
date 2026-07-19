//go:build ignore

// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Command measure samples real mainnet blocks from an ffldb datadir and reports
// witness fraction, compression ratios, and the combined witness-separated
// storage savings across different chain eras. Uses streaming I/O — reads one
// block at a time, never loads an entire file into memory.
//
// Usage:
//
//	go run blockcompress/dicttrain/measure.go -datadir /path/to/btcd/blocks_ffldb
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
	"strconv"

	"github.com/btcsuite/btcd/wire/v2"
	"github.com/klauspost/compress/zstd"
)

const (
	recordHeaderLen  = 8
	recordTrailerLen = 4
)

func main() {
	datadir := flag.String("datadir", "", "path to ffldb blocks directory containing .fdb files")
	sampleEvery := flag.Int("sampleevery", 50, "sample 1 file per N files (spread across chain)")
	dictFile := flag.String("dict", "", "optional zstd dictionary file to benchmark against no-dict baseline")
	flag.Parse()

	if *datadir == "" {
		fmt.Fprintln(os.Stderr, "error: -datadir is required")
		os.Exit(1)
	}

	files, err := filepath.Glob(filepath.Join(*datadir, "*.fdb"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "glob: %v\n", err)
		os.Exit(1)
	}
	sort.Strings(files)
	fmt.Printf("Found %d .fdb files\n", len(files))

	enc, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.EncoderLevel(3)),
		zstd.WithEncoderConcurrency(1),
		zstd.WithZeroFrames(false),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "zstd init: %v\n", err)
		os.Exit(1)
	}
	defer enc.Close()

	var encDict *zstd.Encoder
	var dictBytes []byte
	if *dictFile != "" {
		dictBytes, err = os.ReadFile(*dictFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read dict: %v\n", err)
			os.Exit(1)
		}
		encDict, err = zstd.NewWriter(nil,
			zstd.WithEncoderLevel(zstd.EncoderLevel(3)),
			zstd.WithEncoderConcurrency(1),
			zstd.WithZeroFrames(false),
			zstd.WithEncoderDictRaw(1, dictBytes),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "zstd dict init: %v\n", err)
			os.Exit(1)
		}
		defer encDict.Close()
	}

	var totRaw, totStripped, totCompWhole, totCompStripped, totCompStrippedDict, totWitness, totBlocks int

	for i := 0; i < len(files); i += *sampleEvery {
		file := files[i]
		base := filepath.Base(file)
		fileNum, _ := strconv.Atoi(base[:len(base)-4])

		raw, stripped, witnessBytes, count, compWhole, compStripped, compStrippedDict :=
			measureFile(file, enc, encDict)
		if count == 0 {
			continue
		}

		totRaw += raw
		totStripped += stripped
		totCompWhole += compWhole
		totCompStripped += compStripped
		totCompStrippedDict += compStrippedDict
		totWitness += witnessBytes
		totBlocks += count

		witnessPct := 100.0 * float64(witnessBytes) / float64(raw)
		compOnly := 100.0 * (1 - float64(compWhole)/float64(raw))
		pruneOnly := 100.0 * (1 - float64(stripped)/float64(raw))
		combined := 100.0 * (1 - float64(compStripped)/float64(raw))
		combinedDict := 100.0 * (1 - float64(compStrippedDict)/float64(raw))

		fmt.Printf("file %07d: %4d blocks, raw=%.1fMB witness=%.1f%% | "+
			"compress=%.1f%% prune=%.1f%% combined=%.1f%% combined+dict=%.1f%%\n",
			fileNum, count,
			float64(raw)/1024/1024, witnessPct,
			compOnly, pruneOnly, combined, combinedDict)
	}

	if totBlocks == 0 {
		fmt.Fprintln(os.Stderr, "no blocks sampled")
		os.Exit(1)
	}

	fmt.Printf("\n===== BLENDED across %d blocks, %.1f GB =====\n",
		totBlocks, float64(totRaw)/1024/1024/1024)
	fmt.Printf("total raw:          %d bytes (%.1f GB)\n", totRaw, float64(totRaw)/1e9)
	fmt.Printf("total witness:      %d bytes (%.1f GB, %.1f%%)\n",
		totWitness, float64(totWitness)/1e9,
		100.0*float64(totWitness)/float64(totRaw))
	fmt.Printf("total stripped:     %d bytes (%.1f GB)\n",
		totStripped, float64(totStripped)/1e9)
	fmt.Printf("\ncompress-only (zstd whole block):   %.1f%% reduction -> %.1f GB\n",
		100.0*(1-float64(totCompWhole)/float64(totRaw)),
		float64(totCompWhole)/1e9)
	fmt.Printf("excise-witness-only (no compress):   %.1f%% reduction -> %.1f GB\n",
		100.0*(1-float64(totStripped)/float64(totRaw)),
		float64(totStripped)/1e9)
	fmt.Printf("combined (excise witness + zstd):    %.1f%% reduction -> %.1f GB\n",
		100.0*(1-float64(totCompStripped)/float64(totRaw)),
		float64(totCompStripped)/1e9)
	if totCompStrippedDict > 0 {
		fmt.Printf("combined + dict (prune + zstd+dict): %.1f%% reduction -> %.1f GB\n",
			100.0*(1-float64(totCompStrippedDict)/float64(totRaw)),
			float64(totCompStrippedDict)/1e9)
	}

	// Estimate full-chain savings.
	chainGB := 1005.0
	combinedRatio := float64(totCompStripped) / float64(totRaw)
	combinedDictRatio := float64(totCompStrippedDict) / float64(totRaw)
	fmt.Printf("\n===== FULL-CHAIN ESTIMATE (from %.0f GB datadir) =====\n", chainGB)
	fmt.Printf("current chain on disk:              %.0f GB\n", chainGB)
	fmt.Printf("estimated with witness-separation:  %.0f GB (%.1f%% reduction)\n",
		chainGB*combinedRatio, 100.0*(1-combinedRatio))
	fmt.Printf("estimated disk saved:               %.0f GB\n",
		chainGB*(1-combinedRatio))
	if totCompStrippedDict > 0 {
		fmt.Printf("estimated with witness-sep + dict:  %.0f GB (%.1f%% reduction)\n",
			chainGB*combinedDictRatio, 100.0*(1-combinedDictRatio))
		fmt.Printf("estimated disk saved (with dict):   %.0f GB\n",
			chainGB*(1-combinedDictRatio))
	}
}

// measureFile stream-reads all blocks from one .fdb file one at a time,
// strips witness, compresses both whole and stripped versions, and returns
// aggregate metrics. Never holds more than one block in memory.
func measureFile(path string, enc, encDict *zstd.Encoder) (raw, stripped, witnessBytes, count, compWhole, compStripped, compStrippedDict int) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: skip %s: %v\n", path, err)
		return
	}
	defer f.Close()

	for {
		var header [recordHeaderLen]byte
		if _, err := io.ReadFull(f, header[:]); err != nil {
			break
		}
		network := binary.LittleEndian.Uint32(header[0:4])
		blockLen := binary.LittleEndian.Uint32(header[4:8])

		if !isValidNetwork(network) {
			break
		}
		if blockLen == 0 || blockLen > 8<<20 {
			break
		}

		rawBlock := make([]byte, blockLen)
		if _, err := io.ReadFull(f, rawBlock); err != nil {
			break
		}
		if _, err := f.Seek(recordTrailerLen, io.SeekCurrent); err != nil {
			break
		}

		var blk wire.MsgBlock
		if err := blk.Deserialize(bytes.NewReader(rawBlock)); err != nil {
			continue
		}

		var whole bytes.Buffer
		if err := blk.Serialize(&whole); err != nil {
			continue
		}
		var strippedBuf bytes.Buffer
		if err := blk.SerializeNoWitness(&strippedBuf); err != nil {
			continue
		}

		wholeBytes := whole.Bytes()
		strippedBytes := strippedBuf.Bytes()

		compW := enc.EncodeAll(wholeBytes, make([]byte, 0, len(wholeBytes)))
		compS := enc.EncodeAll(strippedBytes, make([]byte, 0, len(strippedBytes)))
		var compSD []byte
		if encDict != nil {
			compSD = encDict.EncodeAll(strippedBytes, make([]byte, 0, len(strippedBytes)))
		}

		raw += len(wholeBytes)
		stripped += len(strippedBytes)
		witnessBytes += len(wholeBytes) - len(strippedBytes)
		compWhole += len(compW)
		compStripped += len(compS)
		compStrippedDict += len(compSD)
		count++
	}
	return
}

func isValidNetwork(net uint32) bool {
	return net == uint32(wire.MainNet) ||
		net == uint32(wire.TestNet3) ||
		net == uint32(wire.TestNet) ||
		net == uint32(wire.SimNet) ||
		net == uint32(wire.TestNet4) ||
		net == uint32(wire.SigNet)
}
