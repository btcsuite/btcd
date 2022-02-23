// Copyright (c) 2016-2017 The btcsuite developers
// Copyright (c) 2016-2017 The Lightning Network Developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gcs_test

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"testing"

	"github.com/btcsuite/btcd/btcutil/gcs"
)

var (
	// No need to allocate an err variable in every test
	err error

	// Collision probability for the tests (1/2**19)
	P = uint8(19)

	// Modulus value for the tests.
	M uint64 = 784931

	// Filters are conserved between tests but we must define with an
	// interface which functions we're testing because the gcsFilter type
	// isn't exported
	filter, filter2, filter3 *gcs.Filter

	// We need to use the same key for building and querying the filters
	key [gcs.KeySize]byte

	// List of values for building a filter
	contents = [][]byte{
		[]byte("Alex"),
		[]byte("Bob"),
		[]byte("Charlie"),
		[]byte("Dick"),
		[]byte("Ed"),
		[]byte("Frank"),
		[]byte("George"),
		[]byte("Harry"),
		[]byte("Ilya"),
		[]byte("John"),
		[]byte("Kevin"),
		[]byte("Larry"),
		[]byte("Michael"),
		[]byte("Nate"),
		[]byte("Owen"),
		[]byte("Paul"),
		[]byte("Quentin"),
	}

	// List of values for querying a filter using MatchAny()
	contents2 = [][]byte{
		[]byte("Alice"),
		[]byte("Betty"),
		[]byte("Charmaine"),
		[]byte("Donna"),
		[]byte("Edith"),
		[]byte("Faina"),
		[]byte("Georgia"),
		[]byte("Hannah"),
		[]byte("Ilsbeth"),
		[]byte("Jennifer"),
		[]byte("Kayla"),
		[]byte("Lena"),
		[]byte("Michelle"),
		[]byte("Natalie"),
		[]byte("Ophelia"),
		[]byte("Peggy"),
		[]byte("Queenie"),
	}
)

// TestGCSFilterBuild builds a test filter with a randomized key. For Bitcoin
// use, deterministic filter generation is desired. Therefore, a key that's
// derived deterministically would be required.
func TestGCSFilterBuild(t *testing.T) {
	for i := 0; i < gcs.KeySize; i += 4 {
		binary.BigEndian.PutUint32(key[i:], rand.Uint32())
	}
	filter, err = gcs.BuildGCSFilter(P, M, key, contents)
	if err != nil {
		t.Fatalf("Filter build failed: %s", err.Error())
	}
}

// TestGCSMatchZeroHash ensures that Match and MatchAny properly match an item
// if it's hash after the reduction is zero. This is accomplished by brute
// forcing a specific target whose hash is zero given a certain (P, M, key,
// len(elements)) combination. In this case, P and M are the default, key was
// chosen randomly, and len(elements) is 13. The target 4-byte value of 16060032
// is the first such 32-bit value, thus we use the number 0-11 as the other
// elements in the filter since we know they won't collide. We test both the
// positive and negative cases, when the zero hash item is in the filter and
// when it is excluded. In the negative case, the 32-bit value of 12 is added to
// the filter instead of the target.
func TestGCSMatchZeroHash(t *testing.T) {
	t.Run("include zero", func(t *testing.T) {
		testGCSMatchZeroHash(t, true)
	})
	t.Run("exclude zero", func(t *testing.T) {
		testGCSMatchZeroHash(t, false)
	})
}

func testGCSMatchZeroHash(t *testing.T, includeZeroHash bool) {
	key := [gcs.KeySize]byte{
		0x25, 0x28, 0x0d, 0x25, 0x26, 0xe1, 0xd3, 0xc7,
		0xa5, 0x71, 0x85, 0x34, 0x92, 0xa5, 0x7e, 0x68,
	}

	// Construct the target data to match, whose hash is zero after applying
	// the reduction with the parameters in the test.
	target := make([]byte, 4)
	binary.BigEndian.PutUint32(target, 16060032)

	// Construct the set of 13 items including the target, using the 32-bit
	// values of 0 through 11 as the first 12 items. We known none of these
	// hash to zero since the brute force ended well beyond them.
	elements := make([][]byte, 0, 13)
	for i := 0; i < 12; i++ {
		data := make([]byte, 4)
		binary.BigEndian.PutUint32(data, uint32(i))
		elements = append(elements, data)
	}

	// If the filter should include the zero hash element, add the target
	// which we know hashes to zero. Otherwise add 32-bit value of 12 which
	// we know does not hash to zero.
	if includeZeroHash {
		elements = append(elements, target)
	} else {
		data := make([]byte, 4)
		binary.BigEndian.PutUint32(data, 12)
		elements = append(elements, data)
	}

	filter, err := gcs.BuildGCSFilter(P, M, key, elements)
	if err != nil {
		t.Fatalf("unable to build filter: %v", err)
	}

	match, err := filter.Match(key, target)
	if err != nil {
		t.Fatalf("unable to match: %v", err)
	}

	// We should only get a match iff the target was included.
	if match != includeZeroHash {
		t.Fatalf("expected match from Match: %t, got %t",
			includeZeroHash, match)
	}

	match, err = filter.MatchAny(key, [][]byte{target})
	if err != nil {
		t.Fatalf("unable to match any: %v", err)
	}

	// We should only get a match iff the target was included.
	if match != includeZeroHash {
		t.Fatalf("expected match from MatchAny: %t, got %t",
			includeZeroHash, match)
	}
}

// TestGCSFilterCopy deserializes and serializes a filter to create a copy.
func TestGCSFilterCopy(t *testing.T) {
	serialized2, err := filter.Bytes()
	if err != nil {
		t.Fatalf("Filter Bytes() failed: %v", err)
	}
	filter2, err = gcs.FromBytes(filter.N(), P, M, serialized2)
	if err != nil {
		t.Fatalf("Filter copy failed: %s", err.Error())
	}
	serialized3, err := filter.NBytes()
	if err != nil {
		t.Fatalf("Filter NBytes() failed: %v", err)
	}
	filter3, err = gcs.FromNBytes(filter.P(), M, serialized3)
	if err != nil {
		t.Fatalf("Filter copy failed: %s", err.Error())
	}
}

// TestGCSFilterMetadata checks that the filter metadata is built and copied
// correctly.
func TestGCSFilterMetadata(t *testing.T) {
	if filter.P() != P {
		t.Fatal("P not correctly stored in filter metadata")
	}
	if filter.N() != uint32(len(contents)) {
		t.Fatal("N not correctly stored in filter metadata")
	}
	if filter.P() != filter2.P() {
		t.Fatal("P doesn't match between copied filters")
	}
	if filter.P() != filter3.P() {
		t.Fatal("P doesn't match between copied filters")
	}
	if filter.N() != filter2.N() {
		t.Fatal("N doesn't match between copied filters")
	}
	if filter.N() != filter3.N() {
		t.Fatal("N doesn't match between copied filters")
	}
	serialized, err := filter.Bytes()
	if err != nil {
		t.Fatalf("Filter Bytes() failed: %v", err)
	}
	serialized2, err := filter2.Bytes()
	if err != nil {
		t.Fatalf("Filter Bytes() failed: %v", err)
	}
	if !bytes.Equal(serialized, serialized2) {
		t.Fatal("Bytes don't match between copied filters")
	}
	serialized3, err := filter3.Bytes()
	if err != nil {
		t.Fatalf("Filter Bytes() failed: %v", err)
	}
	if !bytes.Equal(serialized, serialized3) {
		t.Fatal("Bytes don't match between copied filters")
	}
	serialized4, err := filter3.Bytes()
	if err != nil {
		t.Fatalf("Filter Bytes() failed: %v", err)
	}
	if !bytes.Equal(serialized, serialized4) {
		t.Fatal("Bytes don't match between copied filters")
	}
}

// TestGCSFilterMatch checks that both the built and copied filters match
// correctly, logging any false positives without failing on them.
func TestGCSFilterMatch(t *testing.T) {
	match, err := filter.Match(key, []byte("Nate"))
	if err != nil {
		t.Fatalf("Filter match failed: %s", err.Error())
	}
	if !match {
		t.Fatal("Filter didn't match when it should have!")
	}
	match, err = filter2.Match(key, []byte("Nate"))
	if err != nil {
		t.Fatalf("Filter match failed: %s", err.Error())
	}
	if !match {
		t.Fatal("Filter didn't match when it should have!")
	}
	match, err = filter.Match(key, []byte("Quentin"))
	if err != nil {
		t.Fatalf("Filter match failed: %s", err.Error())
	}
	if !match {
		t.Fatal("Filter didn't match when it should have!")
	}
	match, err = filter2.Match(key, []byte("Quentin"))
	if err != nil {
		t.Fatalf("Filter match failed: %s", err.Error())
	}
	if !match {
		t.Fatal("Filter didn't match when it should have!")
	}
	match, err = filter.Match(key, []byte("Nates"))
	if err != nil {
		t.Fatalf("Filter match failed: %s", err.Error())
	}
	if match {
		t.Logf("False positive match, should be 1 in 2**%d!", P)
	}
	match, err = filter2.Match(key, []byte("Nates"))
	if err != nil {
		t.Fatalf("Filter match failed: %s", err.Error())
	}
	if match {
		t.Logf("False positive match, should be 1 in 2**%d!", P)
	}
	match, err = filter.Match(key, []byte("Quentins"))
	if err != nil {
		t.Fatalf("Filter match failed: %s", err.Error())
	}
	if match {
		t.Logf("False positive match, should be 1 in 2**%d!", P)
	}
	match, err = filter2.Match(key, []byte("Quentins"))
	if err != nil {
		t.Fatalf("Filter match failed: %s", err.Error())
	}
	if match {
		t.Logf("False positive match, should be 1 in 2**%d!", P)
	}
}

// AnyMatcher is the function signature of our matching algorithms.
type AnyMatcher func(key [gcs.KeySize]byte, data [][]byte) (bool, error)

// TestGCSFilterMatchAnySuite checks that all of our matching algorithms
// properly match a list correctly when using built or copied filters, logging
// any false positives without failing on them.
func TestGCSFilterMatchAnySuite(t *testing.T) {
	funcs := []struct {
		name     string
		matchAny func(*gcs.Filter) AnyMatcher
	}{
		{
			"default",
			func(f *gcs.Filter) AnyMatcher {
				return f.MatchAny
			},
		},
		{
			"hash",
			func(f *gcs.Filter) AnyMatcher {
				return f.HashMatchAny
			},
		},
		{
			"zip",
			func(f *gcs.Filter) AnyMatcher {
				return f.ZipMatchAny
			},
		},
	}

	for _, test := range funcs {
		test := test

		t.Run(test.name, func(t *testing.T) {
			contentsCopy := make([][]byte, len(contents2))
			copy(contentsCopy, contents2)

			match, err := test.matchAny(filter)(key, contentsCopy)
			if err != nil {
				t.Fatalf("Filter match any failed: %s", err.Error())
			}
			if match {
				t.Logf("False positive match, should be 1 in 2**%d!", P)
			}
			match, err = test.matchAny(filter2)(key, contentsCopy)
			if err != nil {
				t.Fatalf("Filter match any failed: %s", err.Error())
			}
			if match {
				t.Logf("False positive match, should be 1 in 2**%d!", P)
			}
			contentsCopy = append(contentsCopy, []byte("Nate"))
			match, err = test.matchAny(filter)(key, contentsCopy)
			if err != nil {
				t.Fatalf("Filter match any failed: %s", err.Error())
			}
			if !match {
				t.Fatal("Filter didn't match any when it should have!")
			}
			match, err = test.matchAny(filter2)(key, contentsCopy)
			if err != nil {
				t.Fatalf("Filter match any failed: %s", err.Error())
			}
			if !match {
				t.Fatal("Filter didn't match any when it should have!")
			}
		})
	}
}
