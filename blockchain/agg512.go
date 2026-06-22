// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"encoding/binary"
	"fmt"
	"math/bits"
)

// agg512Limbs is the number of 64-bit limbs in the swift sync accumulator.  A
// salted outpoint occupies the low 68 bytes and the running sum needs room
// above that for carries.
const agg512Limbs = 10

// agg512Size is the number of bytes in the little-endian serialization of an
// Agg512.
const agg512Size = agg512Limbs * 8

// Agg512 is the accumulator used to validate swift sync.  It holds a running
// sum of salted outpoints as little-endian 64-bit limbs, with limb 0 the least
// significant.  Add and Sub are inverse operations modulo 2^(8*agg512Size), so
// adding and subtracting the same value cancels exactly and the order of
// operations does not matter.
//
// Swift sync adds the salted outpoint of every output spent within the synced
// range (once, when the output is created) and subtracts it (once, when it is
// spent), so a correct chain leaves the accumulator at zero.  A salted outpoint
// fills only the low 68 bytes, so the upper limbs absorb carries and the
// running sum never overflows the register.
type Agg512 [agg512Limbs]uint64

// Add adds the value v to the accumulator, propagating the carry through all
// limbs.
func (a *Agg512) Add(v *[agg512Size]byte) {
	var carry uint64
	a[0], carry = bits.Add64(a[0], binary.LittleEndian.Uint64(v[0:8]), carry)
	a[1], carry = bits.Add64(a[1], binary.LittleEndian.Uint64(v[8:16]), carry)
	a[2], carry = bits.Add64(a[2], binary.LittleEndian.Uint64(v[16:24]), carry)
	a[3], carry = bits.Add64(a[3], binary.LittleEndian.Uint64(v[24:32]), carry)
	a[4], carry = bits.Add64(a[4], binary.LittleEndian.Uint64(v[32:40]), carry)
	a[5], carry = bits.Add64(a[5], binary.LittleEndian.Uint64(v[40:48]), carry)
	a[6], carry = bits.Add64(a[6], binary.LittleEndian.Uint64(v[48:56]), carry)
	a[7], carry = bits.Add64(a[7], binary.LittleEndian.Uint64(v[56:64]), carry)
	a[8], carry = bits.Add64(a[8], binary.LittleEndian.Uint64(v[64:72]), carry)
	a[9], _ = bits.Add64(a[9], binary.LittleEndian.Uint64(v[72:80]), carry)
}

// Sub subtracts the value v from the accumulator, propagating the borrow
// through all limbs (wrapping modulo 2^(8*agg512Size)).
func (a *Agg512) Sub(v *[agg512Size]byte) {
	var borrow uint64
	a[0], borrow = bits.Sub64(a[0], binary.LittleEndian.Uint64(v[0:8]), borrow)
	a[1], borrow = bits.Sub64(a[1], binary.LittleEndian.Uint64(v[8:16]), borrow)
	a[2], borrow = bits.Sub64(a[2], binary.LittleEndian.Uint64(v[16:24]), borrow)
	a[3], borrow = bits.Sub64(a[3], binary.LittleEndian.Uint64(v[24:32]), borrow)
	a[4], borrow = bits.Sub64(a[4], binary.LittleEndian.Uint64(v[32:40]), borrow)
	a[5], borrow = bits.Sub64(a[5], binary.LittleEndian.Uint64(v[40:48]), borrow)
	a[6], borrow = bits.Sub64(a[6], binary.LittleEndian.Uint64(v[48:56]), borrow)
	a[7], borrow = bits.Sub64(a[7], binary.LittleEndian.Uint64(v[56:64]), borrow)
	a[8], borrow = bits.Sub64(a[8], binary.LittleEndian.Uint64(v[64:72]), borrow)
	a[9], _ = bits.Sub64(a[9], binary.LittleEndian.Uint64(v[72:80]), borrow)
}

// IsZero reports whether every limb of the accumulator is zero.
func (a *Agg512) IsZero() bool {
	for _, limb := range a {
		if limb != 0 {
			return false
		}
	}
	return true
}

// Bytes returns the little-endian serialization of the accumulator.
func (a *Agg512) Bytes() [agg512Size]byte {
	var out [agg512Size]byte
	for i, limb := range a {
		binary.LittleEndian.PutUint64(out[i*8:], limb)
	}
	return out
}

// SetBytes sets the accumulator from the little-endian serialization produced
// by Bytes.  It returns an error if b is not exactly agg512Size bytes.
func (a *Agg512) SetBytes(b []byte) error {
	if len(b) != agg512Size {
		return fmt.Errorf("agg512: invalid length %d, want %d", len(b),
			agg512Size)
	}
	for i := range a {
		a[i] = binary.LittleEndian.Uint64(b[i*8:])
	}
	return nil
}
