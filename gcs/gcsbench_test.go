// Copyright (c) 2016-2017 The btcsuite developers
// Copyright (c) 2016-2017 The Lightning Network Developers
// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gcs

import (
	"encoding/binary"
	"math/rand"
	"testing"
)

var globalMatch bool

func genRandFilterElements(numElements uint) ([][]byte, error) {
	testContents := make([][]byte, numElements)
	for i := range contents {
		randElem := make([]byte, 32)
		if _, err := rand.Read(randElem); err != nil {
			return nil, err
		}
		testContents[i] = randElem
	}

	return testContents, nil
}

// BenchmarkGCSFilterBuild benchmarks building a filter.
func BenchmarkGCSFilterBuild50000(b *testing.B) {
	b.StopTimer()
	var testKey [KeySize]byte
	for i := 0; i < KeySize; i += 4 {
		binary.BigEndian.PutUint32(testKey[i:], rand.Uint32())
	}
	randFilterElems, genErr := genRandFilterElements(50000)
	if err != nil {
		b.Fatalf("unable to generate random item: %v", genErr)
	}
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		_, err := NewFilter(P, key, randFilterElems)
		if err != nil {
			b.Fatalf("unable to generate filter: %v", err)
		}
	}
}

// BenchmarkGCSFilterBuild benchmarks building a filter.
func BenchmarkGCSFilterBuild100000(b *testing.B) {
	var testKey [KeySize]byte
	for i := 0; i < KeySize; i += 4 {
		binary.BigEndian.PutUint32(testKey[i:], rand.Uint32())
	}
	randFilterElems, genErr := genRandFilterElements(100000)
	if err != nil {
		b.Fatalf("unable to generate random item: %v", genErr)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := NewFilter(P, key, randFilterElems)
		if err != nil {
			b.Fatalf("unable to generate filter: %v", err)
		}
	}
}

// BenchmarkGCSFilterMatch benchmarks querying a filter for a single value.
func BenchmarkGCSFilterMatch(b *testing.B) {
	filter, err := NewFilter(P, key, contents)
	if err != nil {
		b.Fatalf("Failed to build filter")
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		globalMatch = filter.Match(key, []byte("Nate"))
		globalMatch = filter.Match(key, []byte("Nates"))
	}
}

// BenchmarkGCSFilterMatchAny benchmarks querying a filter for a list of
// values.
func BenchmarkGCSFilterMatchAny(b *testing.B) {
	filter, err := NewFilter(P, key, contents)
	if err != nil {
		b.Fatalf("Failed to build filter")
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		globalMatch = filter.MatchAny(key, contents2)
	}
}
