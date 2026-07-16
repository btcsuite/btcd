// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockcompress

// This file is the home for embedded dictionary assets. When a zstd dictionary
// trained on real mainnet blocks is available, it is embedded here via
// //go:embed and wired into registeredVersions in registry.go.
//
// Until then, FormatV1 operates without a dictionary. The dictionary training
// tool lives in dicttrain/ (//go:build ignore) and produces a .dict file to
// drop into this directory.
//
// IMPORTANT: once a dictionary is embedded for a version, it must never change.
// Any dictionary improvement ships as a NEW format version with a NEW embedded
// dictionary; the old one stays registered forever so old files keep decoding.
