// Copyright (c) 2016-2017 The btcsuite developers
// Copyright (c) 2016-2017 The Lightning Network Developers
// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gcs

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"testing"
)

var (
	// No need to allocate an err variable in every test
	err error

	// Collision probability for the tests (1/2**20)
	P = uint8(20)

	// Filters are conserved between tests but we must define with an
	// interface which functions we're testing because the gcsFilter type
	// isn't exported
	filter, filter2, filter3, filter4, filter5 *Filter

	// We need to use the same key for building and querying the filters
	key [KeySize]byte

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
	for i := 0; i < KeySize; i += 4 {
		binary.BigEndian.PutUint32(key[i:], rand.Uint32())
	}
	filter, err = NewFilter(P, key, contents)
	if err != nil {
		t.Fatalf("Filter build failed: %s", err.Error())
	}
}

// TestGCSFilterCopy deserializes and serializes a filter to create a copy.
func TestGCSFilterCopy(t *testing.T) {
	filter2, err = FromBytes(filter.N(), P, filter.Bytes())
	if err != nil {
		t.Fatalf("Filter copy failed: %s", err.Error())
	}
	filter3, err = FromNBytes(filter.P(), filter.NBytes())
	if err != nil {
		t.Fatalf("Filter copy failed: %s", err.Error())
	}
	filter4, err = FromPBytes(filter.N(), filter.PBytes())
	if err != nil {
		t.Fatalf("Filter copy failed: %s", err.Error())
	}
	filter5, err = FromNPBytes(filter.NPBytes())
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
	if filter.P() != filter4.P() {
		t.Fatal("P doesn't match between copied filters")
	}
	if filter.P() != filter5.P() {
		t.Fatal("P doesn't match between copied filters")
	}
	if filter.N() != filter2.N() {
		t.Fatal("N doesn't match between copied filters")
	}
	if filter.N() != filter3.N() {
		t.Fatal("N doesn't match between copied filters")
	}
	if filter.N() != filter4.N() {
		t.Fatal("N doesn't match between copied filters")
	}
	if filter.N() != filter5.N() {
		t.Fatal("N doesn't match between copied filters")
	}
	if !bytes.Equal(filter.Bytes(), filter2.Bytes()) {
		t.Fatal("Bytes don't match between copied filters")
	}
	if !bytes.Equal(filter.Bytes(), filter3.Bytes()) {
		t.Fatal("Bytes don't match between copied filters")
	}
	if !bytes.Equal(filter.Bytes(), filter4.Bytes()) {
		t.Fatal("Bytes don't match between copied filters")
	}
	if !bytes.Equal(filter.Bytes(), filter5.Bytes()) {
		t.Fatal("Bytes don't match between copied filters")
	}
}

// TestGCSFilterMatch checks that both the built and copied filters match
// correctly, logging any false positives without failing on them.
func TestGCSFilterMatch(t *testing.T) {
	if !filter.Match(key, []byte("Nate")) {
		t.Fatal("Filter didn't match when it should have!")
	}
	if !filter2.Match(key, []byte("Nate")) {
		t.Fatal("Filter didn't match when it should have!")
	}
	if !filter.Match(key, []byte("Quentin")) {
		t.Fatal("Filter didn't match when it should have!")
	}
	if !filter2.Match(key, []byte("Quentin")) {
		t.Fatal("Filter didn't match when it should have!")
	}
	if filter.Match(key, []byte("Nates")) {
		t.Logf("False positive match, should be 1 in 2**%d!", P)
	}
	if filter2.Match(key, []byte("Nates")) {
		t.Logf("False positive match, should be 1 in 2**%d!", P)
	}
	if filter.Match(key, []byte("Quentins")) {
		t.Logf("False positive match, should be 1 in 2**%d!", P)
	}
	if filter2.Match(key, []byte("Quentins")) {
		t.Logf("False positive match, should be 1 in 2**%d!", P)
	}
}

// TestGCSFilterMatchAny checks that both the built and copied filters match a
// list correctly, logging any false positives without failing on them.
func TestGCSFilterMatchAny(t *testing.T) {
	if filter.MatchAny(key, contents2) {
		t.Logf("False positive match, should be 1 in 2**%d!", P)
	}
	if filter2.MatchAny(key, contents2) {
		t.Logf("False positive match, should be 1 in 2**%d!", P)
	}
	contents2 = append(contents2, []byte("Nate"))
	if !filter.MatchAny(key, contents2) {
		t.Fatal("Filter didn't match any when it should have!")
	}
	if !filter2.MatchAny(key, contents2) {
		t.Fatal("Filter didn't match any when it should have!")
	}
}
