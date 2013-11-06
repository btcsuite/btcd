// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire_test

import (
	"github.com/conformal/btcwire"
	"io/ioutil"
	"testing"
)

// BenchmarkWriteVarInt1 performs a benchmark on how long it takes to write
// a single byte variable length integer.
func BenchmarkWriteVarInt1(b *testing.B) {
	for i := 0; i < b.N; i++ {
		btcwire.TstWriteVarInt(ioutil.Discard, 0, 1)
	}
}

// BenchmarkWriteVarInt3 performs a benchmark on how long it takes to write
// a three byte variable length integer.
func BenchmarkWriteVarInt3(b *testing.B) {
	for i := 0; i < b.N; i++ {
		btcwire.TstWriteVarInt(ioutil.Discard, 0, 65535)
	}
}

// BenchmarkWriteVarInt5 performs a benchmark on how long it takes to write
// a five byte variable length integer.
func BenchmarkWriteVarInt5(b *testing.B) {
	for i := 0; i < b.N; i++ {
		btcwire.TstWriteVarInt(ioutil.Discard, 0, 4294967295)
	}
}

// BenchmarkWriteVarInt9 performs a benchmark on how long it takes to write
// a nine byte variable length integer.
func BenchmarkWriteVarInt9(b *testing.B) {
	for i := 0; i < b.N; i++ {
		btcwire.TstWriteVarInt(ioutil.Discard, 0, 18446744073709551615)
	}
}
