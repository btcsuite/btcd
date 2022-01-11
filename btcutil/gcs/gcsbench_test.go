// Copyright (c) 2016-2017 The btcsuite developers
// Copyright (c) 2016-2017 The Lightning Network Developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gcs_test

import (
	"encoding/binary"
	"math/rand"
	"testing"

	"github.com/btcsuite/btcd/btcutil/gcs"
)

func genRandFilterElements(numElements uint) ([][]byte, error) {
	testContents := make([][]byte, numElements)
	for i := range testContents {
		randElem := make([]byte, 32)
		if _, err := rand.Read(randElem); err != nil {
			return nil, err
		}
		testContents[i] = randElem
	}

	return testContents, nil
}

var (
	generatedFilter *gcs.Filter
)

// BenchmarkGCSFilterBuild benchmarks building a filter.
func BenchmarkGCSFilterBuild50000(b *testing.B) {
	var testKey [gcs.KeySize]byte
	for i := 0; i < gcs.KeySize; i += 4 {
		binary.BigEndian.PutUint32(testKey[i:], rand.Uint32())
	}

	randFilterElems, genErr := genRandFilterElements(50000)
	if err != nil {
		b.Fatalf("unable to generate random item: %v", genErr)
	}

	b.ReportAllocs()
	b.ResetTimer()

	var localFilter *gcs.Filter
	for i := 0; i < b.N; i++ {
		localFilter, err = gcs.BuildGCSFilter(
			P, M, key, randFilterElems,
		)
		if err != nil {
			b.Fatalf("unable to generate filter: %v", err)
		}
	}
	generatedFilter = localFilter
}

// BenchmarkGCSFilterBuild benchmarks building a filter.
func BenchmarkGCSFilterBuild100000(b *testing.B) {
	var testKey [gcs.KeySize]byte
	for i := 0; i < gcs.KeySize; i += 4 {
		binary.BigEndian.PutUint32(testKey[i:], rand.Uint32())
	}

	randFilterElems, genErr := genRandFilterElements(100000)
	if err != nil {
		b.Fatalf("unable to generate random item: %v", genErr)
	}

	b.ReportAllocs()
	b.ResetTimer()

	var localFilter *gcs.Filter
	for i := 0; i < b.N; i++ {
		localFilter, err = gcs.BuildGCSFilter(
			P, M, key, randFilterElems,
		)
		if err != nil {
			b.Fatalf("unable to generate filter: %v", err)
		}
	}
	generatedFilter = localFilter
}

var (
	match bool
)

// BenchmarkGCSFilterMatch benchmarks querying a filter for a single value.
func BenchmarkGCSFilterMatch(b *testing.B) {
	filter, err := gcs.BuildGCSFilter(P, M, key, contents)
	if err != nil {
		b.Fatalf("Failed to build filter")
	}

	b.ReportAllocs()
	b.ResetTimer()

	var localMatch bool
	for i := 0; i < b.N; i++ {
		_, err = filter.Match(key, []byte("Nate"))
		if err != nil {
			b.Fatalf("unable to match filter: %v", err)
		}

		localMatch, err = filter.Match(key, []byte("Nates"))
		if err != nil {
			b.Fatalf("unable to match filter: %v", err)
		}
	}
	match = localMatch
}

var (
	randElems100, _      = genRandFilterElements(100)
	randElems1000, _     = genRandFilterElements(1000)
	randElems10000, _    = genRandFilterElements(10000)
	randElems100000, _   = genRandFilterElements(100000)
	randElems1000000, _  = genRandFilterElements(1000000)
	randElems10000000, _ = genRandFilterElements(10000000)

	filterElems1000, _  = genRandFilterElements(1000)
	filter1000, _       = gcs.BuildGCSFilter(P, M, key, filterElems1000)
	filterElems5000, _  = genRandFilterElements(5000)
	filter5000, _       = gcs.BuildGCSFilter(P, M, key, filterElems5000)
	filterElems10000, _ = genRandFilterElements(10000)
	filter10000, _      = gcs.BuildGCSFilter(P, M, key, filterElems10000)
)

// matchAnyBenchmarks contains combinations of random filters and queries used
// to measure performance of various MatchAny implementations.
var matchAnyBenchmarks = []struct {
	name   string
	query  [][]byte
	filter *gcs.Filter
}{
	{"q100-f1K", randElems100, filter1000},
	{"q1K-f1K", randElems1000, filter1000},
	{"q10K-f1K", randElems10000, filter1000},
	{"q100K-f1K", randElems100000, filter1000},
	{"q1M-f1K", randElems1000000, filter1000},
	{"q10M-f1K", randElems10000000, filter1000},

	{"q100-f5K", randElems100, filter5000},
	{"q1K-f5K", randElems1000, filter5000},
	{"q10K-f5K", randElems10000, filter5000},
	{"q100K-f5K", randElems100000, filter5000},
	{"q1M-f5K", randElems1000000, filter5000},
	{"q10M-f5K", randElems10000000, filter5000},

	{"q100-f10K", randElems100, filter10000},
	{"q1K-f10K", randElems1000, filter10000},
	{"q10K-f10K", randElems10000, filter10000},
	{"q100K-f10K", randElems100000, filter10000},
	{"q1M-f10K", randElems1000000, filter10000},
	{"q10M-f10K", randElems10000000, filter10000},
}

// BenchmarkGCSFilterMatchAny benchmarks the sort-and-zip MatchAny impl.
func BenchmarkGCSFilterZipMatchAny(b *testing.B) {
	for _, test := range matchAnyBenchmarks {
		test := test

		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()

			var (
				localMatch bool
				err        error
			)

			for i := 0; i < b.N; i++ {
				localMatch, err = test.filter.ZipMatchAny(
					key, test.query,
				)
				if err != nil {
					b.Fatalf("unable to match filter: %v", err)
				}
			}
			match = localMatch
		})
	}
}

// BenchmarkGCSFilterMatchAny benchmarks the hash-join MatchAny impl.
func BenchmarkGCSFilterHashMatchAny(b *testing.B) {
	for _, test := range matchAnyBenchmarks {
		test := test

		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()

			var (
				localMatch bool
				err        error
			)

			for i := 0; i < b.N; i++ {
				localMatch, err = test.filter.HashMatchAny(
					key, test.query,
				)
				if err != nil {
					b.Fatalf("unable to match filter: %v", err)
				}
			}
			match = localMatch
		})
	}
}

// BenchmarkGCSFilterMatchAny benchmarks the hybrid MatchAny impl.
func BenchmarkGCSFilterMatchAny(b *testing.B) {
	for _, test := range matchAnyBenchmarks {
		test := test

		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()

			var (
				localMatch bool
				err        error
			)

			for i := 0; i < b.N; i++ {
				localMatch, err = test.filter.MatchAny(
					key, test.query,
				)
				if err != nil {
					b.Fatalf("unable to match filter: %v", err)
				}
			}
			match = localMatch
		})
	}
}
