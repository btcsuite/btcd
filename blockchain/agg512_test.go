// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// aggValue returns a deterministic agg512Size-byte value derived from seed.
func aggValue(seed byte) [agg512Size]byte {
	var v [agg512Size]byte
	for i := range v {
		v[i] = seed + byte(i)
	}
	return v
}

// TestAgg512AddSubCancel verifies that adding then subtracting the same value
// (in either order) returns the accumulator to zero.
func TestAgg512AddSubCancel(t *testing.T) {
	v := aggValue(0x11)

	var a Agg512
	a.Add(&v)
	require.False(t, a.IsZero())
	a.Sub(&v)
	require.True(t, a.IsZero())

	// Subtract first (underflow wraps modulo 2^(8*agg512Size)), then add back.
	var b Agg512
	b.Sub(&v)
	require.False(t, b.IsZero())
	b.Add(&v)
	require.True(t, b.IsZero())
}

// TestAgg512Commutative verifies that a balanced multiset of adds and subs nets
// to zero regardless of the order they are applied in.
func TestAgg512Commutative(t *testing.T) {
	values := make([]*[agg512Size]byte, 64)
	for i := range values {
		v := aggValue(byte(i))
		values[i] = &v
	}

	// Add every value twice and subtract it twice, in a shuffled order.
	type op struct {
		v   *[agg512Size]byte
		add bool
	}
	ops := make([]op, 0, len(values)*4)
	for _, v := range values {
		ops = append(ops, op{v, true}, op{v, true}, op{v, false}, op{v, false})
	}
	rng := rand.New(rand.NewSource(1))
	rng.Shuffle(len(ops), func(i, j int) { ops[i], ops[j] = ops[j], ops[i] })

	var a Agg512
	for _, o := range ops {
		if o.add {
			a.Add(o.v)
		} else {
			a.Sub(o.v)
		}
	}
	require.True(t, a.IsZero(), "balanced ops did not cancel: %v", a)
}

// TestAgg512Carry verifies that carries and borrows propagate up into the top
// limb and still cancel exactly.
func TestAgg512Carry(t *testing.T) {
	// Every limb but the top is all ones, so two adds carry exactly 1 into the
	// top limb.
	var ones [agg512Size]byte
	for i := 0; i < agg512Size-8; i++ {
		ones[i] = 0xff
	}

	var a Agg512
	a.Add(&ones)
	a.Add(&ones)
	require.Equal(t, uint64(1), a[agg512Limbs-1], "carry did not reach the top limb")
	require.False(t, a.IsZero())

	a.Sub(&ones)
	a.Sub(&ones)
	require.True(t, a.IsZero(), "carry/borrow did not cancel: %v", a)
}

// TestAgg512Bytes verifies the serialization round-trips, including non-zero
// high limbs, and that SetBytes rejects the wrong length.
func TestAgg512Bytes(t *testing.T) {
	a := Agg512{1, 2, 3, 4, 5, 6, 7, 8, 9, 0xfedcba9876543210}

	serialized := a.Bytes()
	var b Agg512
	require.NoError(t, b.SetBytes(serialized[:]))
	require.Equal(t, a, b)

	require.Error(t, b.SetBytes(make([]byte, agg512Size-1)))
	require.Error(t, b.SetBytes(make([]byte, agg512Size+1)))
}

// TestAgg512IsZero checks IsZero for zero and non-zero accumulators, including
// when only a high limb is set.
func TestAgg512IsZero(t *testing.T) {
	var a Agg512
	require.True(t, a.IsZero())

	a[agg512Limbs-1] = 1
	require.False(t, a.IsZero())
}
