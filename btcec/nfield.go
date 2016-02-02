// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2013-2014 Dave Collins
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcec

// References:
//   [HAC]: Handbook of Applied Cryptography Menezes, van Oorschot, Vanstone.
//     http://cacr.uwaterloo.ca/hac/

// All elliptic curve operations for secp256k1 are done in a finite field
// characterized by a 256-bit prime.  Given this precision is larger than the
// biggest available native type, obviously some form of bignum math is needed.
// This package implements specialized fixed-precision field arithmetic rather
// than relying on an arbitrary-precision arithmetic package such as math/big
// for dealing with the field math since the size is known.  As a result, rather
// large performance gains are achieved by taking advantage of many
// optimizations not available to arbitrary-precision arithmetic and generic
// modular arithmetic algorithms.
//
// There are various ways to internally represent each finite field element.
// For example, the most obvious representation would be to use an array of 4
// uint64s (64 bits * 4 = 256 bits).  However, that representation suffers from
// a couple of issues.  First, there is no native Go type large enough to handle
// the intermediate results while adding or multiplying two 64-bit numbers, and
// second there is no space left for overflows when performing the intermediate
// arithmetic between each array element which would lead to expensive carry
// propagation.
//
// Given the above, this implementation represents the the field elements as
// 10 uint32s with each word (array entry) treated as base 2^26.  This was
// chosen for the following reasons:
// 1) Most systems at the current time are 64-bit (or at least have 64-bit
//    registers available for specialized purposes such as MMX) so the
//    intermediate results can typically be done using a native register (and
//    using uint64s to avoid the need for additional half-word arithmetic)
// 2) In order to allow addition of the internal words without having to
//    propagate the the carry, the max normalized value for each register must
//    be less than the number of bits available in the register
// 3) Since we're dealing with 32-bit values, 64-bits of overflow is a
//    reasonable choice for #2
// 4) Given the need for 256-bits of precision and the properties stated in #1,
//    #2, and #3, the representation which best accomodates this is 10 uint32s
//    with base 2^26 (26 bits * 10 = 260 bits, so the final word only needs 22
//    bits) which leaves the desired 64 bits (32 * 10 = 320, 320 - 256 = 64) for
//    overflow
//
// Since it is so important that the field arithmetic is extremely fast for
// high performance crypto, this package does not perform any validation where
// it ordinarily would.  For example, some functions only give the correct
// result is the field is normalized and there is no checking to ensure it is.
// While I typically prefer to ensure all state and input is valid for most
// packages, this code is really only used internally and every extra check
// counts.

import (
	"encoding/hex"
	"fmt"
)

// Constants related to the nfield representation.
const (
	// nfieldWords is the number of words used to internally represent the
	// 256-bit value.
	nfieldWords = 10

	// nfieldBase is the exponent used to form the numeric base of each word.
	// 2^(nfieldBase*i) where i is the word position.
	nfieldBase = 26

	// nfieldOverflowBits is the minimum number of "overflow" bits for each
	// word in the nfield value.
	nfieldOverflowBits = 32 - nfieldBase

	// nfieldBaseMask is the mask for the bits in each word needed to
	// represent the numeric base of each word (except the most significant
	// word).
	nfieldBaseMask = (1 << nfieldBase) - 1

	// nfieldMSBBits is the number of bits in the most significant word used
	// to represent the value.
	nfieldMSBBits = 256 - (nfieldBase * (nfieldWords - 1))

	// nfieldMSBMask is the mask for the bits in the most significant word
	// needed to represent the value.
	nfieldMSBMask = (1 << nfieldMSBBits) - 1

	// nfieldPrimeWordZero is word zero of the secp256k1 prime in the
	// internal nfield representation.  It is used during modular reduction
	// and negation.
	nfieldPrimeWordZero = 0x364141

	// nfieldPrimeWordOne is word one of the secp256k1 prime in the
	// internal nfield representation.  It is used during modular reduction
	// and negation.
	nfieldPrimeWordOne = 0x97a334

	// nfieldPrimeWordTwo is word two of the secp256k1 prime in the
	// internal nfield representation.  It is used during modular reduction
	// and negation.
	nfieldPrimeWordTwo = 0x203bbfd

	// nfieldPrimeWordThree is word three of the secp256k1 prime in the
	// internal nfield representation.  It is used during modular reduction
	// and negation.
	nfieldPrimeWordThree = 0x39abd22

	// nfieldPrimeWordFour is word four of the secp256k1 prime in the
	// internal nfield representation.  It is used during modular reduction
	// and negation.
	nfieldPrimeWordFour = 0x2baaedc

	nfieldComplementZero  = 0x3c9bebf
	nfieldComplementOne   = 0x3685ccb
	nfieldComplementTwo   = 0x1fc4402
	nfieldComplementThree = 0x6542dd
	nfieldComplementFour  = 0x1455123
	nfieldReduceZero      = nfieldComplementZero * 16
	nfieldReduceOne       = nfieldComplementOne * 16
	nfieldReduceTwo       = nfieldComplementTwo * 16
	nfieldReduceThree     = nfieldComplementThree * 16
	nfieldReduceFour      = nfieldComplementFour * 16
)

// nfieldVal implements optimized fixed-precision arithmetic over the
// secp256k1 finite nfield.  This means all arithmetic is performed modulo
// 0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffefffffc2f.  It
// represents each 256-bit value as 10 32-bit integers in base 2^26.  This
// provides 6 bits of overflow in each word (10 bits in the most significant
// word) for a total of 64 bits of overflow (9*6 + 10 = 64).  It only implements
// the arithmetic needed for elliptic curve operations.
//
// The following depicts the internal representation:
// 	 -----------------------------------------------------------------
// 	|        n[9]       |        n[8]       | ... |        n[0]       |
// 	| 32 bits available | 32 bits available | ... | 32 bits available |
// 	| 22 bits for value | 26 bits for value | ... | 26 bits for value |
// 	| 10 bits overflow  |  6 bits overflow  | ... |  6 bits overflow  |
// 	| Mult: 2^(26*9)    | Mult: 2^(26*8)    | ... | Mult: 2^(26*0)    |
// 	 -----------------------------------------------------------------
//
// For example, consider the number 2^49 + 1.  It would be represented as:
// 	n[0] = 1
// 	n[1] = 2^23
// 	n[2..9] = 0
//
// The full 256-bit value is then calculated by looping i from 9..0 and
// doing sum(n[i] * 2^(26i)) like so:
// 	n[9] * 2^(26*9) = 0    * 2^234 = 0
// 	n[8] * 2^(26*8) = 0    * 2^208 = 0
// 	...
// 	n[1] * 2^(26*1) = 2^23 * 2^26  = 2^49
// 	n[0] * 2^(26*0) = 1    * 2^0   = 1
// 	Sum: 0 + 0 + ... + 2^49 + 1 = 2^49 + 1
type nfieldVal struct {
	n [10]uint32
}

// String returns the nfield value as a human-readable hex string.
func (f nfieldVal) String() string {
	t := new(nfieldVal).Set(&f).Normalize()
	return hex.EncodeToString(t.Bytes()[:])
}

// Zero sets the nfield value to zero.  A newly created nfield value is already
// set to zero.  This function can be useful to clear an existing nfield value
// for reuse.
func (f *nfieldVal) Zero() {
	f.n[0] = 0
	f.n[1] = 0
	f.n[2] = 0
	f.n[3] = 0
	f.n[4] = 0
	f.n[5] = 0
	f.n[6] = 0
	f.n[7] = 0
	f.n[8] = 0
	f.n[9] = 0
}

// Set sets the nfield value equal to the passed value.
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f := new(nfieldVal).Set(f2).Add(1) so that f = f2 + 1 where f2 is not
// modified.
func (f *nfieldVal) Set(val *nfieldVal) *nfieldVal {
	*f = *val
	return f
}

// SetInt sets the nfield value to the passed integer.  This is a convenience
// function since it is fairly common to perform some arithemetic with small
// native integers.
//
// The nfield value is returned to support chaining.  This enables syntax such
// as f := new(nfieldVal).SetInt(2).Mul(f2) so that f = 2 * f2.
func (f *nfieldVal) SetInt(ui uint) *nfieldVal {
	f.Zero()
	f.n[0] = uint32(ui)
	return f
}

// SetBytes packs the passed 32-byte big-endian value into the internal nfield
// value representation.
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f := new(nfieldVal).SetBytes(byteArray).Mul(f2) so that f = ba * f2.
func (f *nfieldVal) SetBytes(b *[32]byte) *nfieldVal {
	// Pack the 256 total bits across the 10 uint32 words with a max of
	// 26-bits per word.  This could be done with a couple of for loops,
	// but this unrolled version is significantly faster.  Benchmarks show
	// this is about 34 times faster than the variant which uses loops.
	f.n[0] = uint32(b[31]) | uint32(b[30])<<8 | uint32(b[29])<<16 |
		(uint32(b[28])&twoBitsMask)<<24
	f.n[1] = uint32(b[28])>>2 | uint32(b[27])<<6 | uint32(b[26])<<14 |
		(uint32(b[25])&fourBitsMask)<<22
	f.n[2] = uint32(b[25])>>4 | uint32(b[24])<<4 | uint32(b[23])<<12 |
		(uint32(b[22])&sixBitsMask)<<20
	f.n[3] = uint32(b[22])>>6 | uint32(b[21])<<2 | uint32(b[20])<<10 |
		uint32(b[19])<<18
	f.n[4] = uint32(b[18]) | uint32(b[17])<<8 | uint32(b[16])<<16 |
		(uint32(b[15])&twoBitsMask)<<24
	f.n[5] = uint32(b[15])>>2 | uint32(b[14])<<6 | uint32(b[13])<<14 |
		(uint32(b[12])&fourBitsMask)<<22
	f.n[6] = uint32(b[12])>>4 | uint32(b[11])<<4 | uint32(b[10])<<12 |
		(uint32(b[9])&sixBitsMask)<<20
	f.n[7] = uint32(b[9])>>6 | uint32(b[8])<<2 | uint32(b[7])<<10 |
		uint32(b[6])<<18
	f.n[8] = uint32(b[5]) | uint32(b[4])<<8 | uint32(b[3])<<16 |
		(uint32(b[2])&twoBitsMask)<<24
	f.n[9] = uint32(b[2])>>2 | uint32(b[1])<<6 | uint32(b[0])<<14
	return f
}

// SetByteSlice packs the passed big-endian value into the internal nfield value
// representation.  Only the first 32-bytes are used.  As a result, it is up to
// the caller to ensure numbers of the appropriate size are used or the value
// will be truncated.
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f := new(nfieldVal).SetByteSlice(byteSlice)
func (f *nfieldVal) SetByteSlice(b []byte) *nfieldVal {
	var b32 [32]byte
	cap := len(b)
	if cap > 32 {
		cap = 32
	}
	for i := 0; i < cap; i++ {
		if i < 32 {
			b32[i+(32-cap)] = b[i]
		}
	}
	return f.SetBytes(&b32)
}

// SetHex decodes the passed big-endian hex string into the internal nfield value
// representation.  Only the first 32-bytes are used.
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f := new(nfieldVal).SetHex("0abc").Add(1) so that f = 0x0abc + 1
func (f *nfieldVal) SetHex(hexString string) *nfieldVal {
	if len(hexString)%2 != 0 {
		hexString = "0" + hexString
	}
	bytes, _ := hex.DecodeString(hexString)
	return f.SetByteSlice(bytes)
}

// Normalize normalizes the internal nfield words into the desired range and
// performs fast modular reduction over the secp256k1 prime by making use of the
// special form of the prime.
func (f *nfieldVal) Normalize() *nfieldVal {
	// The nfield representation leaves 6 bits of overflow in each
	// word so intermediate calculations can be performed without needing
	// to propagate the carry to each higher word during the calculations.
	// In order to normalize, first we need to "compact" the full 256-bit
	// value to the right and treat the additional 64 leftmost bits as
	// the magnitude.
	m := f.n[0]
	t0 := m & nfieldBaseMask
	m = (m >> nfieldBase) + f.n[1]
	t1 := m & nfieldBaseMask
	m = (m >> nfieldBase) + f.n[2]
	t2 := m & nfieldBaseMask
	m = (m >> nfieldBase) + f.n[3]
	t3 := m & nfieldBaseMask
	m = (m >> nfieldBase) + f.n[4]
	t4 := m & nfieldBaseMask
	m = (m >> nfieldBase) + f.n[5]
	t5 := m & nfieldBaseMask
	m = (m >> nfieldBase) + f.n[6]
	t6 := m & nfieldBaseMask
	m = (m >> nfieldBase) + f.n[7]
	t7 := m & nfieldBaseMask
	m = (m >> nfieldBase) + f.n[8]
	t8 := m & nfieldBaseMask
	m = (m >> nfieldBase) + f.n[9]
	t9 := m & nfieldMSBMask
	m = m >> nfieldMSBBits

	// At this point, if the magnitude is greater than 0, the overall value
	// is greater than the max possible 256-bit value.  In particular, it is
	// "how many times larger" than the max value it is.  Since this nfield
	// is doing arithmetic modulo the secp256k1 prime, we need to perform
	// modular reduction over the prime.
	//
	// Per [HAC] section 14.3.4: Reduction method of moduli of special form,
	// when the modulus is of the special form m = b^t - c, highly efficient
	// reduction can be achieved.
	//
	// The secp256k1 N is equivalent to
	// 2^256 - 432420386565659656852420866394968145599, so it fits
	// this criteria.
	//
	// 432420386565659656852420866394968145599 in nfield representation
	// (base 2^26) is:
	// n[0] = 0x3c9bebf (63553215)
	// n[1] = 0x3685ccb (57171147)
	// n[2] = 0x1fc4402 (33309698)
	// n[3] = 0x6542dd  (6636253)
	// n[4] = 0x1455123 (21319971)
	// That is to say
	// 21319971 * 2**104 + 6636253 * 2**78 + 33309698 * 2**52
	// + 57171147 * 2**26 + 63553215 = 432420386565659656852420866394968145599
	//
	//
	// The algorithm presented in the referenced section typically repeats
	// until the quotient is zero.  However, due to our nfield representation
	// we already know at least how many times we would need to repeat as
	// it's the value currently in m.  Thus we can simply multiply the
	// magnitude by the nfield representation of the prime and do a single
	// iteration.  Notice that nothing will be changed when the magnitude is
	// zero, so we could skip this in that case, however always running
	// regardless allows it to run in constant time.
	r := t0 + m*nfieldComplementZero
	t0 = r & nfieldBaseMask
	r = (r >> nfieldBase) + t1 + m*nfieldComplementOne
	t1 = r & nfieldBaseMask
	r = (r >> nfieldBase) + t2 + m*nfieldComplementTwo
	t2 = r & nfieldBaseMask
	r = (r >> nfieldBase) + t3 + m*nfieldComplementThree
	t3 = r & nfieldBaseMask
	r = (r >> nfieldBase) + t4 + m*nfieldComplementFour
	t4 = r & nfieldBaseMask
	r = (r >> nfieldBase) + t5
	t5 = r & nfieldBaseMask
	r = (r >> nfieldBase) + t6
	t6 = r & nfieldBaseMask
	r = (r >> nfieldBase) + t7
	t7 = r & nfieldBaseMask
	r = (r >> nfieldBase) + t8
	t8 = r & nfieldBaseMask
	r = (r >> nfieldBase) + t9
	t9 = r & nfieldMSBMask

	// At this point, one more subtraction of the prime
	// might be needed if the current result is greater than or equal to the
	// n.

	subtractN := false

	if t9 == nfieldMSBMask && t8 == nfieldBaseMask && t7 == nfieldBaseMask && t6 == nfieldBaseMask && t5 == nfieldBaseMask {
		if t4 > nfieldPrimeWordFour {
			subtractN = true
		} else if t4 == nfieldPrimeWordFour {

			if t3 > nfieldPrimeWordThree {
				subtractN = true
			} else if t3 == nfieldPrimeWordThree {
				if t2 > nfieldPrimeWordTwo {
					subtractN = true
				} else if t2 == nfieldPrimeWordTwo {
					if t1 > nfieldPrimeWordOne {
						subtractN = true
					} else if t1 == nfieldPrimeWordOne {
						if t0 >= nfieldPrimeWordZero {
							subtractN = true
						}
					}
				}
			}
		}
	}

	if subtractN {

		borrow := uint32(0)
		if t0 < nfieldPrimeWordZero {
			t0 = (1 << nfieldBase) + t0 - uint32(nfieldPrimeWordZero)
			borrow = 1
		} else {
			t0 = t0 - nfieldPrimeWordZero
			borrow = 0
		}

		if t1-borrow < nfieldPrimeWordOne {
			t1 = (1 << nfieldBase) + t1 - uint32(nfieldPrimeWordOne) - borrow
			borrow = 1
		} else {
			t1 = t1 - nfieldPrimeWordOne - borrow
			borrow = 0
		}

		if t2-borrow < nfieldPrimeWordTwo {
			t2 = (1 << nfieldBase) + t2 - uint32(nfieldPrimeWordTwo) - borrow
			borrow = 1
		} else {
			t2 = t2 - nfieldPrimeWordTwo - borrow
			borrow = 0
		}

		if t3-borrow < nfieldPrimeWordThree {
			t3 = (1 << nfieldBase) + t3 - uint32(nfieldPrimeWordThree) - borrow
			borrow = 1
		} else {
			t3 = t3 - nfieldPrimeWordThree - borrow
			borrow = 0
		}

		t4 = t4 - uint32(nfieldPrimeWordFour) - borrow
		t5 = 0
		t6 = 0
		t7 = 0
		t8 = 0
		t9 = 0
	}

	// Finally, set the normalized and reduced words.
	f.n[0] = t0
	f.n[1] = t1
	f.n[2] = t2
	f.n[3] = t3
	f.n[4] = t4
	f.n[5] = t5
	f.n[6] = t6
	f.n[7] = t7
	f.n[8] = t8
	f.n[9] = t9
	return f
}

// PutBytes unpacks the nfield value to a 32-byte big-endian value using the
// passed byte array.  There is a similar function, Bytes, which unpacks the
// nfield value into a new array and returns that.  This version is provided
// since it can be useful to cut down on the number of allocations by allowing
// the caller to reuse a buffer.
//
// The nfield value must be normalized for this function to return the correct
// result.
func (f *nfieldVal) PutBytes(b *[32]byte) {
	// Unpack the 256 total bits from the 10 uint32 words with a max of
	// 26-bits per word.  This could be done with a couple of for loops,
	// but this unrolled version is a bit faster.  Benchmarks show this is
	// about 10 times faster than the variant which uses loops.
	b[31] = byte(f.n[0] & eightBitsMask)
	b[30] = byte((f.n[0] >> 8) & eightBitsMask)
	b[29] = byte((f.n[0] >> 16) & eightBitsMask)
	b[28] = byte((f.n[0]>>24)&twoBitsMask | (f.n[1]&sixBitsMask)<<2)
	b[27] = byte((f.n[1] >> 6) & eightBitsMask)
	b[26] = byte((f.n[1] >> 14) & eightBitsMask)
	b[25] = byte((f.n[1]>>22)&fourBitsMask | (f.n[2]&fourBitsMask)<<4)
	b[24] = byte((f.n[2] >> 4) & eightBitsMask)
	b[23] = byte((f.n[2] >> 12) & eightBitsMask)
	b[22] = byte((f.n[2]>>20)&sixBitsMask | (f.n[3]&twoBitsMask)<<6)
	b[21] = byte((f.n[3] >> 2) & eightBitsMask)
	b[20] = byte((f.n[3] >> 10) & eightBitsMask)
	b[19] = byte((f.n[3] >> 18) & eightBitsMask)
	b[18] = byte(f.n[4] & eightBitsMask)
	b[17] = byte((f.n[4] >> 8) & eightBitsMask)
	b[16] = byte((f.n[4] >> 16) & eightBitsMask)
	b[15] = byte((f.n[4]>>24)&twoBitsMask | (f.n[5]&sixBitsMask)<<2)
	b[14] = byte((f.n[5] >> 6) & eightBitsMask)
	b[13] = byte((f.n[5] >> 14) & eightBitsMask)
	b[12] = byte((f.n[5]>>22)&fourBitsMask | (f.n[6]&fourBitsMask)<<4)
	b[11] = byte((f.n[6] >> 4) & eightBitsMask)
	b[10] = byte((f.n[6] >> 12) & eightBitsMask)
	b[9] = byte((f.n[6]>>20)&sixBitsMask | (f.n[7]&twoBitsMask)<<6)
	b[8] = byte((f.n[7] >> 2) & eightBitsMask)
	b[7] = byte((f.n[7] >> 10) & eightBitsMask)
	b[6] = byte((f.n[7] >> 18) & eightBitsMask)
	b[5] = byte(f.n[8] & eightBitsMask)
	b[4] = byte((f.n[8] >> 8) & eightBitsMask)
	b[3] = byte((f.n[8] >> 16) & eightBitsMask)
	b[2] = byte((f.n[8]>>24)&twoBitsMask | (f.n[9]&sixBitsMask)<<2)
	b[1] = byte((f.n[9] >> 6) & eightBitsMask)
	b[0] = byte((f.n[9] >> 14) & eightBitsMask)
}

// Bytes unpacks the nfield value to a 32-byte big-endian value.  See PutBytes
// for a variant that allows the a buffer to be passed which can be useful to
// to cut down on the number of allocations by allowing the caller to reuse a
// buffer.
//
// The nfield value must be normalized for this function to return correct
// result.
func (f *nfieldVal) Bytes() *[32]byte {
	b := new([32]byte)
	f.PutBytes(b)
	return b
}

// IsZero returns whether or not the nfield value is equal to zero.
func (f *nfieldVal) IsZero() bool {
	// The value can only be zero if no bits are set in any of the words.
	// This is a constant time implementation.
	bits := f.n[0] | f.n[1] | f.n[2] | f.n[3] | f.n[4] |
		f.n[5] | f.n[6] | f.n[7] | f.n[8] | f.n[9]

	return bits == 0
}

// IsOdd returns whether or not the nfield value is an odd number.
//
// The nfield value must be normalized for this function to return correct
// result.
func (f *nfieldVal) IsOdd() bool {
	// Only odd numbers have the bottom bit set.
	return f.n[0]&1 == 1
}

// Equals returns whether or not the two nfield values are the same.  Both
// nfield values being compared must be normalized for this function to return
// the correct result.
func (f *nfieldVal) Equals(val *nfieldVal) bool {
	// Xor only sets bits when they are different, so the two nfield values
	// can only be the same if no bits are set after xoring each word.
	// This is a constant time implementation.
	bits := (f.n[0] ^ val.n[0]) | (f.n[1] ^ val.n[1]) | (f.n[2] ^ val.n[2]) |
		(f.n[3] ^ val.n[3]) | (f.n[4] ^ val.n[4]) | (f.n[5] ^ val.n[5]) |
		(f.n[6] ^ val.n[6]) | (f.n[7] ^ val.n[7]) | (f.n[8] ^ val.n[8]) |
		(f.n[9] ^ val.n[9])

	return bits == 0
}

// NegateVal negates the passed value and stores the result in f.
// The result is guaranteed to be normalized
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f.NegateVal(f2).AddInt(1) so that f = -f2 + 1.
func (f *nfieldVal) NegateVal(val *nfieldVal) *nfieldVal {
	// Negation in the nfield is just the prime minus the value.
	// Normalize first
	f.Normalize()
	if f.n[0]+f.n[1]+f.n[2]+f.n[3]+f.n[4]+f.n[5]+f.n[6]+f.n[7]+f.n[8]+f.n[9] == 0 {
		return f
	}
	borrow := uint32(0)
	if val.n[0] > nfieldPrimeWordZero {
		f.n[0] = (1 << nfieldBase) + nfieldPrimeWordZero - val.n[0]
		borrow = 1
	} else {
		f.n[0] = nfieldPrimeWordZero - val.n[0]
		borrow = 0
	}

	if val.n[1] > nfieldPrimeWordOne-borrow {
		f.n[1] = (1 << nfieldBase) + nfieldPrimeWordOne - val.n[1] - borrow
		borrow = 1
	} else {
		f.n[1] = nfieldPrimeWordOne - val.n[1] - borrow
		borrow = 0
	}

	if val.n[2] > nfieldPrimeWordTwo-borrow {
		f.n[2] = (1 << nfieldBase) + nfieldPrimeWordTwo - val.n[2] - borrow
		borrow = 1
	} else {
		f.n[2] = nfieldPrimeWordTwo - val.n[2] - borrow
		borrow = 0
	}

	if val.n[3] > nfieldPrimeWordThree-borrow {
		f.n[3] = (1 << nfieldBase) + nfieldPrimeWordThree - val.n[3] - borrow
		borrow = 1
	} else {
		f.n[3] = nfieldPrimeWordThree - val.n[3] - borrow
		borrow = 0
	}

	if val.n[4] > nfieldPrimeWordFour-borrow {
		f.n[4] = (1 << nfieldBase) + nfieldPrimeWordFour - val.n[4] - borrow
		borrow = 1
	} else {
		f.n[4] = nfieldPrimeWordFour - val.n[4] - borrow
		borrow = 0
	}

	if val.n[5] > nfieldBaseMask-borrow {
		f.n[5] = (1 << nfieldBase) + nfieldBaseMask - val.n[5] - borrow
		borrow = 1
	} else {
		f.n[5] = nfieldBaseMask - val.n[5] - borrow
		borrow = 0
	}

	if val.n[6] > nfieldBaseMask-borrow {
		f.n[6] = (1 << nfieldBase) + nfieldBaseMask - val.n[6] - borrow
		borrow = 1
	} else {
		f.n[6] = nfieldBaseMask - val.n[6] - borrow
		borrow = 0
	}

	if val.n[7] > nfieldBaseMask-borrow {
		f.n[7] = (1 << nfieldBase) + nfieldBaseMask - val.n[7] - borrow
		borrow = 1
	} else {
		f.n[7] = nfieldBaseMask - val.n[7] - borrow
		borrow = 0
	}

	if val.n[8] > nfieldBaseMask-borrow {
		f.n[8] = (1 << nfieldBase) + nfieldBaseMask - val.n[8] - borrow
		borrow = 1
	} else {
		f.n[8] = nfieldBaseMask - val.n[8] - borrow
		borrow = 0
	}

	f.n[9] = nfieldMSBMask - val.n[9] - borrow

	return f
}

// Negate negates the nfield value.  The existing nfield value is modified.  The
// caller must provide the magnitude of the nfield value for a correct result.
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f.Negate().AddInt(1) so that f = -f + 1.
func (f *nfieldVal) Negate() *nfieldVal {
	return f.NegateVal(f)
}

// AddInt adds the passed integer to the existing nfield value and stores the
// result in f.  This is a convenience function since it is fairly common to
// perform some arithemetic with small native integers.
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f.AddInt(1).Add(f2) so that f = f + 1 + f2.
func (f *nfieldVal) AddInt(ui uint) *nfieldVal {
	// Since the nfield representation intentionally provides overflow bits,
	// it's ok to use carryless addition as the carry bit is safely part of
	// the word and will be normalized out.
	f.n[0] += uint32(ui)

	return f
}

// Add adds the passed value to the existing nfield value and stores the result
// in f.
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f.Add(f2).AddInt(1) so that f = f + f2 + 1.
func (f *nfieldVal) Add(val *nfieldVal) *nfieldVal {
	// Since the nfield representation intentionally provides overflow bits,
	// it's ok to use carryless addition as the carry bit is safely part of
	// each word and will be normalized out.  This could obviously be done
	// in a loop, but the unrolled version is faster.
	f.n[0] += val.n[0]
	f.n[1] += val.n[1]
	f.n[2] += val.n[2]
	f.n[3] += val.n[3]
	f.n[4] += val.n[4]
	f.n[5] += val.n[5]
	f.n[6] += val.n[6]
	f.n[7] += val.n[7]
	f.n[8] += val.n[8]
	f.n[9] += val.n[9]

	return f
}

// Add2 adds the passed two nfield values together and stores the result in f.
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f3.Add2(f, f2).AddInt(1) so that f3 = f + f2 + 1.
func (f *nfieldVal) Add2(val *nfieldVal, val2 *nfieldVal) *nfieldVal {
	// Since the nfield representation intentionally provides overflow bits,
	// it's ok to use carryless addition as the carry bit is safely part of
	// each word and will be normalized out.  This could obviously be done
	// in a loop, but the unrolled version is faster.
	f.n[0] = val.n[0] + val2.n[0]
	f.n[1] = val.n[1] + val2.n[1]
	f.n[2] = val.n[2] + val2.n[2]
	f.n[3] = val.n[3] + val2.n[3]
	f.n[4] = val.n[4] + val2.n[4]
	f.n[5] = val.n[5] + val2.n[5]
	f.n[6] = val.n[6] + val2.n[6]
	f.n[7] = val.n[7] + val2.n[7]
	f.n[8] = val.n[8] + val2.n[8]
	f.n[9] = val.n[9] + val2.n[9]

	return f
}

// MulInt multiplies the nfield value by the passed int and stores the result in
// f.  Note that this function can overflow if multiplying the value by any of
// the individual words exceeds a max uint32.  Therefore it is important that
// the caller ensures no overflows will occur before using this function.
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f.MulInt(2).Add(f2) so that f = 2 * f + f2.
func (f *nfieldVal) MulInt(val uint) *nfieldVal {
	// Since each word of the nfield representation can hold up to
	// nfieldOverflowBits extra bits which will be normalized out, it's safe
	// to multiply each word without using a larger type or carry
	// propagation so long as the values won't overflow a uint32.  This
	// could obviously be done in a loop, but the unrolled version is
	// faster.
	ui := uint32(val)
	f.n[0] *= ui
	f.n[1] *= ui
	f.n[2] *= ui
	f.n[3] *= ui
	f.n[4] *= ui
	f.n[5] *= ui
	f.n[6] *= ui
	f.n[7] *= ui
	f.n[8] *= ui
	f.n[9] *= ui

	return f
}

// Mul multiplies the passed value to the existing nfield value and stores the
// result in f.  Note that this function can overflow if multiplying any
// of the individual words exceeds a max uint32.  In practice, this means the
// magnitude of either value involved in the multiplication must be a max of
// 8.
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f.Mul(f2).AddInt(1) so that f = (f * f2) + 1.
func (f *nfieldVal) Mul(val *nfieldVal) *nfieldVal {
	return f.Mul2(f, val)
}

// Mul2 multiplies the passed two nfield values together and stores the result
// result in f.  Note that this function can overflow if multiplying any of
// the individual words exceeds a max uint32.  In practice, this means the
// magnitude of either value involved in the multiplication must be a max of
// 8.
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f3.Mul2(f, f2).AddInt(1) so that f3 = (f * f2) + 1.
func (f *nfieldVal) Mul2(val *nfieldVal, val2 *nfieldVal) *nfieldVal {
	// This could be done with a couple of for loops and an array to store
	// the intermediate terms, but this unrolled version is significantly
	// faster.

	// Terms for 2^(nfieldBase*0).
	m := uint64(val.n[0]) * uint64(val2.n[0])
	t0 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*1).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[1]) +
		uint64(val.n[1])*uint64(val2.n[0])
	t1 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*2).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[2]) +
		uint64(val.n[1])*uint64(val2.n[1]) +
		uint64(val.n[2])*uint64(val2.n[0])
	t2 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*3).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[3]) +
		uint64(val.n[1])*uint64(val2.n[2]) +
		uint64(val.n[2])*uint64(val2.n[1]) +
		uint64(val.n[3])*uint64(val2.n[0])
	t3 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*4).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[4]) +
		uint64(val.n[1])*uint64(val2.n[3]) +
		uint64(val.n[2])*uint64(val2.n[2]) +
		uint64(val.n[3])*uint64(val2.n[1]) +
		uint64(val.n[4])*uint64(val2.n[0])
	t4 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*5).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[5]) +
		uint64(val.n[1])*uint64(val2.n[4]) +
		uint64(val.n[2])*uint64(val2.n[3]) +
		uint64(val.n[3])*uint64(val2.n[2]) +
		uint64(val.n[4])*uint64(val2.n[1]) +
		uint64(val.n[5])*uint64(val2.n[0])
	t5 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*6).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[6]) +
		uint64(val.n[1])*uint64(val2.n[5]) +
		uint64(val.n[2])*uint64(val2.n[4]) +
		uint64(val.n[3])*uint64(val2.n[3]) +
		uint64(val.n[4])*uint64(val2.n[2]) +
		uint64(val.n[5])*uint64(val2.n[1]) +
		uint64(val.n[6])*uint64(val2.n[0])
	t6 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*7).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[7]) +
		uint64(val.n[1])*uint64(val2.n[6]) +
		uint64(val.n[2])*uint64(val2.n[5]) +
		uint64(val.n[3])*uint64(val2.n[4]) +
		uint64(val.n[4])*uint64(val2.n[3]) +
		uint64(val.n[5])*uint64(val2.n[2]) +
		uint64(val.n[6])*uint64(val2.n[1]) +
		uint64(val.n[7])*uint64(val2.n[0])
	t7 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*8).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[8]) +
		uint64(val.n[1])*uint64(val2.n[7]) +
		uint64(val.n[2])*uint64(val2.n[6]) +
		uint64(val.n[3])*uint64(val2.n[5]) +
		uint64(val.n[4])*uint64(val2.n[4]) +
		uint64(val.n[5])*uint64(val2.n[3]) +
		uint64(val.n[6])*uint64(val2.n[2]) +
		uint64(val.n[7])*uint64(val2.n[1]) +
		uint64(val.n[8])*uint64(val2.n[0])
	t8 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*9).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[9]) +
		uint64(val.n[1])*uint64(val2.n[8]) +
		uint64(val.n[2])*uint64(val2.n[7]) +
		uint64(val.n[3])*uint64(val2.n[6]) +
		uint64(val.n[4])*uint64(val2.n[5]) +
		uint64(val.n[5])*uint64(val2.n[4]) +
		uint64(val.n[6])*uint64(val2.n[3]) +
		uint64(val.n[7])*uint64(val2.n[2]) +
		uint64(val.n[8])*uint64(val2.n[1]) +
		uint64(val.n[9])*uint64(val2.n[0])
	t9 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*10).
	m = (m >> nfieldBase) +
		uint64(val.n[1])*uint64(val2.n[9]) +
		uint64(val.n[2])*uint64(val2.n[8]) +
		uint64(val.n[3])*uint64(val2.n[7]) +
		uint64(val.n[4])*uint64(val2.n[6]) +
		uint64(val.n[5])*uint64(val2.n[5]) +
		uint64(val.n[6])*uint64(val2.n[4]) +
		uint64(val.n[7])*uint64(val2.n[3]) +
		uint64(val.n[8])*uint64(val2.n[2]) +
		uint64(val.n[9])*uint64(val2.n[1])
	t10 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*11).
	m = (m >> nfieldBase) +
		uint64(val.n[2])*uint64(val2.n[9]) +
		uint64(val.n[3])*uint64(val2.n[8]) +
		uint64(val.n[4])*uint64(val2.n[7]) +
		uint64(val.n[5])*uint64(val2.n[6]) +
		uint64(val.n[6])*uint64(val2.n[5]) +
		uint64(val.n[7])*uint64(val2.n[4]) +
		uint64(val.n[8])*uint64(val2.n[3]) +
		uint64(val.n[9])*uint64(val2.n[2])
	t11 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*12).
	m = (m >> nfieldBase) +
		uint64(val.n[3])*uint64(val2.n[9]) +
		uint64(val.n[4])*uint64(val2.n[8]) +
		uint64(val.n[5])*uint64(val2.n[7]) +
		uint64(val.n[6])*uint64(val2.n[6]) +
		uint64(val.n[7])*uint64(val2.n[5]) +
		uint64(val.n[8])*uint64(val2.n[4]) +
		uint64(val.n[9])*uint64(val2.n[3])
	t12 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*13).
	m = (m >> nfieldBase) +
		uint64(val.n[4])*uint64(val2.n[9]) +
		uint64(val.n[5])*uint64(val2.n[8]) +
		uint64(val.n[6])*uint64(val2.n[7]) +
		uint64(val.n[7])*uint64(val2.n[6]) +
		uint64(val.n[8])*uint64(val2.n[5]) +
		uint64(val.n[9])*uint64(val2.n[4])
	t13 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*14).
	m = (m >> nfieldBase) +
		uint64(val.n[5])*uint64(val2.n[9]) +
		uint64(val.n[6])*uint64(val2.n[8]) +
		uint64(val.n[7])*uint64(val2.n[7]) +
		uint64(val.n[8])*uint64(val2.n[6]) +
		uint64(val.n[9])*uint64(val2.n[5])
	t14 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*15).
	m = (m >> nfieldBase) +
		uint64(val.n[6])*uint64(val2.n[9]) +
		uint64(val.n[7])*uint64(val2.n[8]) +
		uint64(val.n[8])*uint64(val2.n[7]) +
		uint64(val.n[9])*uint64(val2.n[6])
	t15 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*16).
	m = (m >> nfieldBase) +
		uint64(val.n[7])*uint64(val2.n[9]) +
		uint64(val.n[8])*uint64(val2.n[8]) +
		uint64(val.n[9])*uint64(val2.n[7])
	t16 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*17).
	m = (m >> nfieldBase) +
		uint64(val.n[8])*uint64(val2.n[9]) +
		uint64(val.n[9])*uint64(val2.n[8])
	t17 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*18).
	m = (m >> nfieldBase) + uint64(val.n[9])*uint64(val2.n[9])
	t18 := m & nfieldBaseMask

	// What's left is for 2^(nfieldBase*19).
	t19 := m >> nfieldBase

	// At this point, all of the terms are grouped into their respective
	// base.
	//
	// Per [HAC] section 14.3.4: Reduction method of moduli of special form,
	// when the modulus is of the special form m = b^t - c, highly efficient
	// reduction can be achieved per the provided algorithm.
	//
	// The secp256k1 N is equivalent to
	// 2^256 - 432420386565659656852420866394968145599, so it fits
	// this criteria.
	//
	// 432420386565659656852420866394968145599 in nfield representation
	// (base 2^26) is:
	// n[0] = 0x3c9bebf (63553215)
	// n[1] = 0x3685ccb (57171147)
	// n[2] = 0x1fc4402 (33309698)
	// n[3] = 0x6542dd  (6636253)
	// n[4] = 0x1455123 (21319971)
	// That is to say
	// 21319971 * 2**104 + 6636253 * 2**78 + 33309698 * 2**52
	// + 57171147 * 2**26 + 63553215 = 432420386565659656852420866394968145599
	//
	// Since each word is in base 2**26, the upper terms (t10 and up) start
	// at 260 bits (versus the final desired range of 256 bits), so the
	// nfield representation of 'c' from above needs to be adjusted for the
	// extra 4 bits by multiplying it by 2^4 = 16.
	// Thus, the adjusted nfield representation of 'c' is:
	// n[0] = 0x3c9bebf00 (16269623040)
	// n[1] = 0x3685ccb00 (14635813632)
	// n[2] = 0x1fc440200 (8527282688)
	// n[3] = 0x6542dd00 (1698880768)
	// n[4] = 0x145512300 (5457912576)
	//
	// Since the numbers are going to be too large for an int64, we're going to
	// start by getting rid of the t15-19 by reducing them into t5-t14

	m = t5 + t15*nfieldReduceZero
	t5 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t6 + t16*nfieldReduceZero + t15*nfieldReduceOne
	t6 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t7 + t17*nfieldReduceZero + t16*nfieldReduceOne + t15*nfieldReduceTwo
	t7 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t8 + t18*nfieldReduceZero + t17*nfieldReduceOne + t16*nfieldReduceTwo + t15*nfieldReduceThree
	t8 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t9 + t19*nfieldReduceZero + t18*nfieldReduceOne + t17*nfieldReduceTwo + t16*nfieldReduceThree + t15*nfieldReduceFour
	t9 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t10 + t19*nfieldReduceOne + t18*nfieldReduceTwo + t17*nfieldReduceThree + t16*nfieldReduceFour
	t10 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t11 + t19*nfieldReduceTwo + t18*nfieldReduceThree + t17*nfieldReduceFour
	t11 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t12 + t19*nfieldReduceThree + t18*nfieldReduceFour
	t12 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t13 + t19*nfieldReduceFour
	t13 = m & nfieldBaseMask
	t14 = (m >> nfieldBase) + t14

	// now we need to reduce t10-t14 into t0-t9

	m = t0 + t10*nfieldReduceZero
	t0 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t1 + t11*nfieldReduceZero + t10*nfieldReduceOne
	t1 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t2 + t12*nfieldReduceZero + t11*nfieldReduceOne + t10*nfieldReduceTwo
	t2 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t3 + t13*nfieldReduceZero + t12*nfieldReduceOne + t11*nfieldReduceTwo + t10*nfieldReduceThree
	t3 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t4 + t14*nfieldReduceZero + t13*nfieldReduceOne + t12*nfieldReduceTwo + t11*nfieldReduceThree + t10*nfieldReduceFour
	t4 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t5 + t14*nfieldReduceOne + t13*nfieldReduceTwo + t12*nfieldReduceThree + t11*nfieldReduceFour
	t5 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t6 + t14*nfieldReduceTwo + t13*nfieldReduceThree + t12*nfieldReduceFour
	t6 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t7 + t14*nfieldReduceThree + t13*nfieldReduceFour
	t7 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t8 + t14*nfieldReduceFour
	t8 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t9
	t9 = m & nfieldMSBMask
	m = m >> nfieldMSBBits

	// At this point, if the magnitude is greater than 0, the overall value
	// is greater than the max possible 256-bit value.  In particular, it is
	// "how many times larger" than the max value it is.
	//
	// The algorithm presented in [HAC] section 14.3.4 repeats until the
	// quotient is zero.  However, due to the above, we already know at
	// least how many times we would need to repeat as it's the value
	// currently in m.  Thus we can simply multiply the magnitude by the
	// nfield representation of the prime and do a single iteration.  Notice
	// that nothing will be changed when the magnitude is zero, so we could
	// skip this in that case, however always running regardless allows it
	// to run in constant time.  The final result will be in the range
	// 0 <= result <= prime + (2^64 - c), so it is guaranteed to have a
	// magnitude of 1, but it is denormalized.
	d := t0 + m*nfieldComplementZero
	f.n[0] = uint32(d & nfieldBaseMask)
	d = (d >> nfieldBase) + t1 + m*nfieldComplementOne
	f.n[1] = uint32(d & nfieldBaseMask)
	d = (d >> nfieldBase) + t2 + m*nfieldComplementTwo
	f.n[2] = uint32(d & nfieldBaseMask)
	d = (d >> nfieldBase) + t3 + m*nfieldComplementThree
	f.n[3] = uint32(d & nfieldBaseMask)
	d = (d >> nfieldBase) + t4 + m*nfieldComplementFour
	f.n[4] = uint32(d & nfieldBaseMask)
	f.n[5] = uint32((d >> fieldBase) + t5)
	f.n[6] = uint32(t6)
	f.n[7] = uint32(t7)
	f.n[8] = uint32(t8)
	f.n[9] = uint32(t9)

	return f
}

// Square squares the nfield value.  The existing nfield value is modified.  Note
// that this function can overflow if multiplying any of the individual words
// exceeds a max uint32.  In practice, this means the magnitude of the nfield
// must be a max of 8 to prevent overflow.
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f.Square().Mul(f2) so that f = f^2 * f2.
func (f *nfieldVal) Square() *nfieldVal {
	return f.SquareVal(f)
}

// SquareVal squares the passed value and stores the result in f.  Note that
// this function can overflow if multiplying any of the individual words
// exceeds a max uint32.  In practice, this means the magnitude of the nfield
// being squred must be a max of 8 to prevent overflow.
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f3.SquareVal(f).Mul(f) so that f3 = f^2 * f = f^3.
func (f *nfieldVal) SquareVal(val *nfieldVal) *nfieldVal {
	// This could be done with a couple of for loops and an array to store
	// the intermediate terms, but this unrolled version is significantly
	// faster.

	// Terms for 2^(nfieldBase*0).
	m := uint64(val.n[0]) * uint64(val.n[0])
	t0 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*1).
	m = (m >> nfieldBase) + 2*uint64(val.n[0])*uint64(val.n[1])
	t1 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*2).
	m = (m >> nfieldBase) +
		2*uint64(val.n[0])*uint64(val.n[2]) +
		uint64(val.n[1])*uint64(val.n[1])
	t2 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*3).
	m = (m >> nfieldBase) +
		2*uint64(val.n[0])*uint64(val.n[3]) +
		2*uint64(val.n[1])*uint64(val.n[2])
	t3 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*4).
	m = (m >> nfieldBase) +
		2*uint64(val.n[0])*uint64(val.n[4]) +
		2*uint64(val.n[1])*uint64(val.n[3]) +
		uint64(val.n[2])*uint64(val.n[2])
	t4 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*5).
	m = (m >> nfieldBase) +
		2*uint64(val.n[0])*uint64(val.n[5]) +
		2*uint64(val.n[1])*uint64(val.n[4]) +
		2*uint64(val.n[2])*uint64(val.n[3])
	t5 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*6).
	m = (m >> nfieldBase) +
		2*uint64(val.n[0])*uint64(val.n[6]) +
		2*uint64(val.n[1])*uint64(val.n[5]) +
		2*uint64(val.n[2])*uint64(val.n[4]) +
		uint64(val.n[3])*uint64(val.n[3])
	t6 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*7).
	m = (m >> nfieldBase) +
		2*uint64(val.n[0])*uint64(val.n[7]) +
		2*uint64(val.n[1])*uint64(val.n[6]) +
		2*uint64(val.n[2])*uint64(val.n[5]) +
		2*uint64(val.n[3])*uint64(val.n[4])
	t7 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*8).
	m = (m >> nfieldBase) +
		2*uint64(val.n[0])*uint64(val.n[8]) +
		2*uint64(val.n[1])*uint64(val.n[7]) +
		2*uint64(val.n[2])*uint64(val.n[6]) +
		2*uint64(val.n[3])*uint64(val.n[5]) +
		uint64(val.n[4])*uint64(val.n[4])
	t8 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*9).
	m = (m >> nfieldBase) +
		2*uint64(val.n[0])*uint64(val.n[9]) +
		2*uint64(val.n[1])*uint64(val.n[8]) +
		2*uint64(val.n[2])*uint64(val.n[7]) +
		2*uint64(val.n[3])*uint64(val.n[6]) +
		2*uint64(val.n[4])*uint64(val.n[5])
	t9 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*10).
	m = (m >> nfieldBase) +
		2*uint64(val.n[1])*uint64(val.n[9]) +
		2*uint64(val.n[2])*uint64(val.n[8]) +
		2*uint64(val.n[3])*uint64(val.n[7]) +
		2*uint64(val.n[4])*uint64(val.n[6]) +
		uint64(val.n[5])*uint64(val.n[5])
	t10 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*11).
	m = (m >> nfieldBase) +
		2*uint64(val.n[2])*uint64(val.n[9]) +
		2*uint64(val.n[3])*uint64(val.n[8]) +
		2*uint64(val.n[4])*uint64(val.n[7]) +
		2*uint64(val.n[5])*uint64(val.n[6])
	t11 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*12).
	m = (m >> nfieldBase) +
		2*uint64(val.n[3])*uint64(val.n[9]) +
		2*uint64(val.n[4])*uint64(val.n[8]) +
		2*uint64(val.n[5])*uint64(val.n[7]) +
		uint64(val.n[6])*uint64(val.n[6])
	t12 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*13).
	m = (m >> nfieldBase) +
		2*uint64(val.n[4])*uint64(val.n[9]) +
		2*uint64(val.n[5])*uint64(val.n[8]) +
		2*uint64(val.n[6])*uint64(val.n[7])
	t13 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*14).
	m = (m >> nfieldBase) +
		2*uint64(val.n[5])*uint64(val.n[9]) +
		2*uint64(val.n[6])*uint64(val.n[8]) +
		uint64(val.n[7])*uint64(val.n[7])
	t14 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*15).
	m = (m >> nfieldBase) +
		2*uint64(val.n[6])*uint64(val.n[9]) +
		2*uint64(val.n[7])*uint64(val.n[8])
	t15 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*16).
	m = (m >> nfieldBase) +
		2*uint64(val.n[7])*uint64(val.n[9]) +
		uint64(val.n[8])*uint64(val.n[8])
	t16 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*17).
	m = (m >> nfieldBase) + 2*uint64(val.n[8])*uint64(val.n[9])
	t17 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*18).
	m = (m >> nfieldBase) + uint64(val.n[9])*uint64(val.n[9])
	t18 := m & nfieldBaseMask

	// What's left is for 2^(nfieldBase*19).
	t19 := m >> nfieldBase

	// At this point, all of the terms are grouped into their respective
	// base.
	//
	// Per [HAC] section 14.3.4: Reduction method of moduli of special form,
	// when the modulus is of the special form m = b^t - c, highly efficient
	// reduction can be achieved per the provided algorithm.
	//
	// The secp256k1 N is equivalent to
	// 2^256 - 432420386565659656852420866394968145599, so it fits
	// this criteria.
	//
	// 432420386565659656852420866394968145599 in nfield representation
	// (base 2^26) is:
	// n[0] = 0x3c9bebf (63553215)
	// n[1] = 0x3685ccb (57171147)
	// n[2] = 0x1fc4402 (33309698)
	// n[3] = 0x6542dd  (6636253)
	// n[4] = 0x1455123 (21319971)
	// That is to say
	// 21319971 * 2**104 + 6636253 * 2**78 + 33309698 * 2**52
	// + 57171147 * 2**26 + 63553215 = 432420386565659656852420866394968145599
	//
	// Since each word is in base 2**26, the upper terms (t10 and up) start
	// at 260 bits (versus the final desired range of 256 bits), so the
	// nfield representation of 'c' from above needs to be adjusted for the
	// extra 4 bits by multiplying it by 2^4 = 16.
	// Thus, the adjusted nfield representation of 'c' is:
	// n[0] = 0x3c9bebf00 (16269623040)
	// n[1] = 0x3685ccb00 (14635813632)
	// n[2] = 0x1fc440200 (8527282688)
	// n[3] = 0x6542dd00 (1698880768)
	// n[4] = 0x145512300 (5457912576)
	//
	// Since the numbers are going to be too large for an int64, we're going to
	// start by getting rid of the t15-19 by reducing them into t5-t14

	m = t5 + t15*nfieldReduceZero
	t5 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t6 + t16*nfieldReduceZero + t15*nfieldReduceOne
	t6 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t7 + t17*nfieldReduceZero + t16*nfieldReduceOne + t15*nfieldReduceTwo
	t7 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t8 + t18*nfieldReduceZero + t17*nfieldReduceOne + t16*nfieldReduceTwo + t15*nfieldReduceThree
	t8 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t9 + t19*nfieldReduceZero + t18*nfieldReduceOne + t17*nfieldReduceTwo + t16*nfieldReduceThree + t15*nfieldReduceFour
	t9 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t10 + t19*nfieldReduceOne + t18*nfieldReduceTwo + t17*nfieldReduceThree + t16*nfieldReduceFour
	t10 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t11 + t19*nfieldReduceTwo + t18*nfieldReduceThree + t17*nfieldReduceFour
	t11 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t12 + t19*nfieldReduceThree + t18*nfieldReduceFour
	t12 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t13 + t19*nfieldReduceFour
	t13 = m & nfieldBaseMask
	t14 = (m >> nfieldBase) + t14

	// now we need to reduce t10-t14 into t0-t9

	m = t0 + t10*nfieldReduceZero
	t0 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t1 + t11*nfieldReduceZero + t10*nfieldReduceOne
	t1 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t2 + t12*nfieldReduceZero + t11*nfieldReduceOne + t10*nfieldReduceTwo
	t2 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t3 + t13*nfieldReduceZero + t12*nfieldReduceOne + t11*nfieldReduceTwo + t10*nfieldReduceThree
	t3 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t4 + t14*nfieldReduceZero + t13*nfieldReduceOne + t12*nfieldReduceTwo + t11*nfieldReduceThree + t10*nfieldReduceFour
	t4 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t5 + t14*nfieldReduceOne + t13*nfieldReduceTwo + t12*nfieldReduceThree + t11*nfieldReduceFour
	t5 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t6 + t14*nfieldReduceTwo + t13*nfieldReduceThree + t12*nfieldReduceFour
	t6 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t7 + t14*nfieldReduceThree + t13*nfieldReduceFour
	t7 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t8 + t14*nfieldReduceFour
	t8 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t9
	t9 = m & nfieldMSBMask
	m = m >> nfieldMSBBits

	// At this point, if the magnitude is greater than 0, the overall value
	// is greater than the max possible 256-bit value.  In particular, it is
	// "how many times larger" than the max value it is.
	//
	// The algorithm presented in [HAC] section 14.3.4 repeats until the
	// quotient is zero.  However, due to the above, we already know at
	// least how many times we would need to repeat as it's the value
	// currently in m.  Thus we can simply multiply the magnitude by the
	// nfield representation of the prime and do a single iteration.  Notice
	// that nothing will be changed when the magnitude is zero, so we could
	// skip this in that case, however always running regardless allows it
	// to run in constant time.  The final result will be in the range
	// 0 <= result <= prime + (2^64 - c), so it is guaranteed to have a
	// magnitude of 1, but it is denormalized.
	d := t0 + m*nfieldComplementZero
	f.n[0] = uint32(d & nfieldBaseMask)
	d = (d >> nfieldBase) + t1 + m*nfieldComplementOne
	f.n[1] = uint32(d & nfieldBaseMask)
	d = (d >> nfieldBase) + t2 + m*nfieldComplementTwo
	f.n[2] = uint32(d & nfieldBaseMask)
	d = (d >> nfieldBase) + t3 + m*nfieldComplementThree
	f.n[3] = uint32(d & nfieldBaseMask)
	d = (d >> nfieldBase) + t4 + m*nfieldComplementFour
	f.n[4] = uint32(d & nfieldBaseMask)
	f.n[5] = uint32((d >> fieldBase) + t5)
	f.n[6] = uint32(t6)
	f.n[7] = uint32(t7)
	f.n[8] = uint32(t8)
	f.n[9] = uint32(t9)

	return f
}

// Inverse finds the modular multiplicative inverse of the nfield value.  The
// existing nfield value is modified.
//
// The nfield value is returned to support chaining.  This enables syntax like:
// f.Inverse().Mul(f2) so that f = f^-1 * f2.
func (f *nfieldVal) Inverse() *nfieldVal {
	// Fermat's little theorem states that for a nonzero number a and prime
	// prime p, a^(p-1) = 1 (mod p).  Since the multipliciative inverse is
	// a*b = 1 (mod p), it follows that b = a*a^(p-2) = a^(p-1) = 1 (mod p).
	// Thus, a^(p-2) is the multiplicative inverse.
	//
	// In order to efficiently compute a^(p-2), p-2 needs to be split into
	// a sequence of squares and multipications that minimizes the number of
	// multiplications needed (since they are more costly than squarings).
	// Intermediate results are saved and reused as well.
	//
	// The secp256k1 n - 2 is
	// 2^256 - 432420386565659656852420866394968145601.
	//
	// This has a cost of 258 nfield squarings and 33 nfield multiplications.
	var a2, a3, a4, a5, a6, a8, a9, a10, a11, a21, a42, a63, a1019, a1023 nfieldVal
	a2.SquareVal(f)
	a3.Mul2(&a2, f)
	a4.SquareVal(&a2)
	a5.Mul2(&a2, &a3)
	a6.SquareVal(&a3)
	a8.SquareVal(&a4)
	a9.Mul2(&a6, &a3)
	a10.Mul2(&a8, &a2)
	a11.Mul2(&a10, f)
	a21.Mul2(&a10, &a11)
	a42.SquareVal(&a21)
	a63.Mul2(&a42, &a21)
	a1019.SquareVal(&a63).Square().Square().Square().Mul(&a11)
	a1023.Mul2(&a1019, &a4)
	f.Set(&a63)                                    // f = a^(2^6 - 1)
	f.Square().Square().Square().Square().Square() // f = a^(2^11 - 32)
	f.Square().Square().Square().Square().Square() // f = a^(2^16 - 1024)
	f.Mul(&a1023)                                  // f = a^(2^16 - 1)
	f.Square().Square().Square().Square().Square() // f = a^(2^21 - 32)
	f.Square().Square().Square().Square().Square() // f = a^(2^26 - 1024)
	f.Mul(&a1023)                                  // f = a^(2^26 - 1)
	f.Square().Square().Square().Square().Square() // f = a^(2^31 - 32)
	f.Square().Square().Square().Square().Square() // f = a^(2^36 - 1024)
	f.Mul(&a1023)                                  // f = a^(2^36 - 1)
	f.Square().Square().Square().Square().Square() // f = a^(2^41 - 32)
	f.Square().Square().Square().Square().Square() // f = a^(2^46 - 1024)
	f.Mul(&a1023)                                  // f = a^(2^46 - 1)
	f.Square().Square().Square().Square().Square() // f = a^(2^51 - 32)
	f.Square().Square().Square().Square().Square() // f = a^(2^56 - 1024)
	f.Mul(&a1023)                                  // f = a^(2^56 - 1)
	f.Square().Square().Square().Square().Square() // f = a^(2^61 - 32)
	f.Square().Square().Square().Square().Square() // f = a^(2^66 - 1024)
	f.Mul(&a1023)                                  // f = a^(2^66 - 1)
	f.Square().Square().Square().Square().Square() // f = a^(2^71 - 32)
	f.Square().Square().Square().Square().Square() // f = a^(2^76 - 1024)
	f.Mul(&a1023)                                  // f = a^(2^76 - 1)
	f.Square().Square().Square().Square().Square() // f = a^(2^81 - 32)
	f.Square().Square().Square().Square().Square() // f = a^(2^86 - 1024)
	f.Mul(&a1023)                                  // f = a^(2^86 - 1)
	f.Square().Square().Square().Square().Square() // f = a^(2^91 - 32)
	f.Square().Square().Square().Square().Square() // f = a^(2^96 - 1024)
	f.Mul(&a1023)                                  // f = a^(2^96 - 1)
	f.Square().Square().Square().Square().Square() // f = a^(2^101 - 32)
	f.Square().Square().Square().Square().Square() // f = a^(2^106 - 1024)
	f.Mul(&a1023)                                  // f = a^(2^106 - 1)
	f.Square().Square().Square().Square().Square() // f = a^(2^111 - 32)
	f.Square().Square().Square().Square().Square() // f = a^(2^116 - 1024)
	f.Mul(&a1023)                                  // f = a^(2^116 - 1)
	f.Square().Square().Square().Square().Square() // f = a^(2^121 - 32)
	f.Square().Square().Square().Square().Square() // f = a^(2^126 - 1024)
	f.Mul(&a1023)                                  // f = a^(2^126 - 1)
	f.Square().Square().Square().Square().Square() // f = a^(2^131 - 32)
	f.Mul(&a21)                                    // f = a^(2^131 - 11)
	f.Square().Square().Square().Square().Square() // f = a^(2^136 - 352)
	f.Mul(&a21)                                    // f = a^(2^136 - 331)
	f.Mul(&a5)                                     // f = a^(2^136 - 326)
	f.Square().Square().Square().Square().Square() // f = a^(2^141 - 10432)
	f.Mul(&a21)                                    // f = a^(2^141 - 10411)
	f.Square().Square().Square().Square().Square() // f = a^(2^146 - 333152)
	f.Mul(&a21)                                    // f = a^(2^146 - 333131)
	f.Mul(&a6)                                     // f = a^(2^146 - 333125)
	f.Square().Square().Square().Square().Square() // f = a^(2^151 - 10660000)
	f.Mul(&a11)                                    // f = a^(2^151 - 10659989)
	f.Mul(&a3)                                     // f = a^(2^151 - 10659986)
	f.Square().Square().Square().Square().Square() // f = a^(2^156 - 341119552)
	f.Mul(&a11)                                    // f = a^(2^156 - 341119541)
	f.Mul(&a3)                                     // f = a^(2^156 - 341119538)
	f.Square().Square().Square().Square().Square() // f = a^(2^161 - 10915825216)
	f.Mul(&a11)                                    // f = a^(2^161 - 10915825205)
	f.Mul(&a2)                                     // f = a^(2^161 - 10915825203)
	f.Square().Square().Square().Square().Square() // f = a^(2^166 - 349306406496)
	f.Mul(&a11)                                    // f = a^(2^166 - 349306406485)
	f.Square().Square().Square().Square().Square() // f = a^(2^171 - 11177805007520)
	f.Mul(&a21)                                    // f = a^(2^171 - 11177805007499)
	f.Mul(&a5)                                     // f = a^(2^171 - 11177805007494)
	f.Square().Square().Square().Square().Square() // f = a^(2^176 - 357689760239808)
	f.Mul(&a8)                                     // f = a^(2^176 - 357689760239800)
	f.Square().Square().Square().Square().Square() // f = a^(2^181 - 11446072327673600)
	f.Mul(&a10)                                    // f = a^(2^181 - 11446072327673590)
	f.Mul(&a10)                                    // f = a^(2^181 - 11446072327673580)
	f.Square().Square().Square().Square().Square() // f = a^(2^186 - 1102180114514098667840)
	f.Square().Square().Square().Square().Square() // f = a^(2^191 - 11720778063537745920)
	f.Mul(&a21)                                    // f = a^(2^191 - 11720778063537745899)
	f.Mul(&a8)                                     // f = a^(2^191 - 11720778063537745895)
	f.Square().Square().Square().Square().Square() // f = a^(2^196 - 375064898033207868512)
	f.Mul(&a21)                                    // f = a^(2^196 - 375064898033207868491)
	f.Mul(&a6)                                     // f = a^(2^196 - 375064898033207868485)
	f.Square().Square().Square().Square().Square() // f = a^(2^201 - 12002076737062651791520)
	f.Mul(&a21)                                    // f = a^(2^201 - 12002076737062651791499)
	f.Mul(&a10)                                    // f = a^(2^201 - 12002076737062651791489)
	f.Square().Square().Square().Square().Square() // f = a^(2^206 - 384066455586004857327648)
	f.Mul(&a10)                                    // f = a^(2^206 - 384066455586004857327638)
	f.Mul(&a10)                                    // f = a^(2^206 - 384066455586004857327628)
	f.Square().Square().Square().Square().Square() // f = a^(2^211 - 12290126578752155434484096)
	f.Mul(&a10)                                    // f = a^(2^211 - 12290126578752155434484086)
	f.Mul(&a8)                                     // f = a^(2^211 - 12290126578752155434484078)
	f.Square().Square().Square().Square().Square() // f = a^(2^216 - 393284050520068973903490496)
	f.Mul(&a21)                                    // f = a^(2^216 - 393284050520068973903490475)
	f.Mul(&a9)                                     // f = a^(2^216 - 393284050520068973903490466)
	f.Square().Square().Square().Square().Square() // f = a^(2^221 - 12585089616642207164911694912)
	f.Mul(&a11)                                    // f = a^(2^221 - 12585089616642207164911694901)
	f.Mul(&a6)                                     // f = a^(2^221 - 12585089616642207164911694895)
	f.Square().Square().Square().Square().Square() // f = a^(2^226 - 402722867732550629277174236640)
	f.Mul(&a11)                                    // f = a^(2^226 - 402722867732550629277174236629)
	f.Mul(&a8)                                     // f = a^(2^226 - 402722867732550629277174236621)
	f.Square().Square().Square().Square().Square() // f = a^(2^231 - 12887131767441620136869575571872)
	f.Mul(&a8)                                     // f = a^(2^231 - 12887131767441620136869575571868)
	f.Square().Square().Square().Square().Square() // f = a^(2^236 - 412388216558131844379826418299648)
	f.Mul(&a3)                                     // f = a^(2^236 - 412388216558131844379826418299645)
	f.Square().Square().Square().Square().Square() // f = a^(2^241 - 13196422929860219020154445385588640)
	f.Mul(&a10)                                    // f = a^(2^241 - 13196422929860219020154445385588630)
	f.Mul(&a2)                                     // f = a^(2^241 - 13196422929860219020154445385588628)
	f.Square().Square().Square().Square().Square() // f = a^(2^246 - 422285533755527008644942252338836096)
	f.Mul(&a10)                                    // f = a^(2^246 - 422285533755527008644942252338836086)
	f.Mul(&a6)                                     // f = a^(2^246 - 422285533755527008644942252338836080)
	f.Square().Square().Square().Square().Square() // f = a^(2^251 - 13513137080176864276638152074842754560)
	f.Mul(&a9)                                     // f = a^(2^251 - 13513137080176864276638152074842754551)
	f.Square().Square().Square().Square().Square() // f = a^(2^256 - 432420386565659656852420866394968145632)
	f.Mul(&a21)                                    // f = a^(2^256 - 432420386565659656852420866394968145611)
	return f.Mul(&a10)                             // f = a^(2^256 - 432420386565659656852420866394968145601) = a^(p-2)
}

// Cmp returns -1 if f < val, 0 if f == val, 1 if f > val
func (f *nfieldVal) Cmp(val *nfieldVal) int {
	f.Normalize()
	val.Normalize()
	if f.n[9] < val.n[9] {
		return -1
	} else if f.n[9] > val.n[9] {
		return 1
	} else if f.n[8] < val.n[8] {
		return -1
	} else if f.n[8] > val.n[8] {
		return 1
	} else if f.n[7] < val.n[7] {
		return -1
	} else if f.n[7] > val.n[7] {
		return 1
	} else if f.n[6] < val.n[6] {
		return -1
	} else if f.n[6] > val.n[6] {
		return 1
	} else if f.n[5] < val.n[5] {
		return -1
	} else if f.n[5] > val.n[5] {
		return 1
	} else if f.n[4] < val.n[4] {
		return -1
	} else if f.n[4] > val.n[4] {
		return 1
	} else if f.n[3] < val.n[3] {
		return -1
	} else if f.n[3] > val.n[3] {
		return 1
	} else if f.n[2] < val.n[2] {
		return -1
	} else if f.n[2] > val.n[2] {
		return 1
	} else if f.n[1] < val.n[1] {
		return -1
	} else if f.n[1] > val.n[1] {
		return 1
	} else if f.n[0] < val.n[0] {
		return -1
	} else if f.n[0] > val.n[0] {
		return 1
	}
	return 0
}

// Magnitude computes val*val2 / N, rounded down.
// This isn't precise for some reason, but will be within 2 of the big.Int
func (f *nfieldVal) Magnitude(val, val2 *nfieldVal) *nfieldVal {
	// This could be done with a couple of for loops and an array to store
	// the intermediate terms, but this unrolled version is significantly
	// faster.

	// Terms for 2^(nfieldBase*0).
	m := uint64(val.n[0]) * uint64(val2.n[0])
	t0 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*1).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[1]) +
		uint64(val.n[1])*uint64(val2.n[0])
	t1 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*2).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[2]) +
		uint64(val.n[1])*uint64(val2.n[1]) +
		uint64(val.n[2])*uint64(val2.n[0])
	t2 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*3).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[3]) +
		uint64(val.n[1])*uint64(val2.n[2]) +
		uint64(val.n[2])*uint64(val2.n[1]) +
		uint64(val.n[3])*uint64(val2.n[0])
	t3 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*4).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[4]) +
		uint64(val.n[1])*uint64(val2.n[3]) +
		uint64(val.n[2])*uint64(val2.n[2]) +
		uint64(val.n[3])*uint64(val2.n[1]) +
		uint64(val.n[4])*uint64(val2.n[0])
	t4 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*5).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[5]) +
		uint64(val.n[1])*uint64(val2.n[4]) +
		uint64(val.n[2])*uint64(val2.n[3]) +
		uint64(val.n[3])*uint64(val2.n[2]) +
		uint64(val.n[4])*uint64(val2.n[1]) +
		uint64(val.n[5])*uint64(val2.n[0])
	t5 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*6).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[6]) +
		uint64(val.n[1])*uint64(val2.n[5]) +
		uint64(val.n[2])*uint64(val2.n[4]) +
		uint64(val.n[3])*uint64(val2.n[3]) +
		uint64(val.n[4])*uint64(val2.n[2]) +
		uint64(val.n[5])*uint64(val2.n[1]) +
		uint64(val.n[6])*uint64(val2.n[0])
	t6 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*7).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[7]) +
		uint64(val.n[1])*uint64(val2.n[6]) +
		uint64(val.n[2])*uint64(val2.n[5]) +
		uint64(val.n[3])*uint64(val2.n[4]) +
		uint64(val.n[4])*uint64(val2.n[3]) +
		uint64(val.n[5])*uint64(val2.n[2]) +
		uint64(val.n[6])*uint64(val2.n[1]) +
		uint64(val.n[7])*uint64(val2.n[0])
	t7 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*8).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[8]) +
		uint64(val.n[1])*uint64(val2.n[7]) +
		uint64(val.n[2])*uint64(val2.n[6]) +
		uint64(val.n[3])*uint64(val2.n[5]) +
		uint64(val.n[4])*uint64(val2.n[4]) +
		uint64(val.n[5])*uint64(val2.n[3]) +
		uint64(val.n[6])*uint64(val2.n[2]) +
		uint64(val.n[7])*uint64(val2.n[1]) +
		uint64(val.n[8])*uint64(val2.n[0])
	t8 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*9).
	m = (m >> nfieldBase) +
		uint64(val.n[0])*uint64(val2.n[9]) +
		uint64(val.n[1])*uint64(val2.n[8]) +
		uint64(val.n[2])*uint64(val2.n[7]) +
		uint64(val.n[3])*uint64(val2.n[6]) +
		uint64(val.n[4])*uint64(val2.n[5]) +
		uint64(val.n[5])*uint64(val2.n[4]) +
		uint64(val.n[6])*uint64(val2.n[3]) +
		uint64(val.n[7])*uint64(val2.n[2]) +
		uint64(val.n[8])*uint64(val2.n[1]) +
		uint64(val.n[9])*uint64(val2.n[0])
	t9 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*10).
	m = (m >> nfieldBase) +
		uint64(val.n[1])*uint64(val2.n[9]) +
		uint64(val.n[2])*uint64(val2.n[8]) +
		uint64(val.n[3])*uint64(val2.n[7]) +
		uint64(val.n[4])*uint64(val2.n[6]) +
		uint64(val.n[5])*uint64(val2.n[5]) +
		uint64(val.n[6])*uint64(val2.n[4]) +
		uint64(val.n[7])*uint64(val2.n[3]) +
		uint64(val.n[8])*uint64(val2.n[2]) +
		uint64(val.n[9])*uint64(val2.n[1])
	t10 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*11).
	m = (m >> nfieldBase) +
		uint64(val.n[2])*uint64(val2.n[9]) +
		uint64(val.n[3])*uint64(val2.n[8]) +
		uint64(val.n[4])*uint64(val2.n[7]) +
		uint64(val.n[5])*uint64(val2.n[6]) +
		uint64(val.n[6])*uint64(val2.n[5]) +
		uint64(val.n[7])*uint64(val2.n[4]) +
		uint64(val.n[8])*uint64(val2.n[3]) +
		uint64(val.n[9])*uint64(val2.n[2])
	t11 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*12).
	m = (m >> nfieldBase) +
		uint64(val.n[3])*uint64(val2.n[9]) +
		uint64(val.n[4])*uint64(val2.n[8]) +
		uint64(val.n[5])*uint64(val2.n[7]) +
		uint64(val.n[6])*uint64(val2.n[6]) +
		uint64(val.n[7])*uint64(val2.n[5]) +
		uint64(val.n[8])*uint64(val2.n[4]) +
		uint64(val.n[9])*uint64(val2.n[3])
	t12 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*13).
	m = (m >> nfieldBase) +
		uint64(val.n[4])*uint64(val2.n[9]) +
		uint64(val.n[5])*uint64(val2.n[8]) +
		uint64(val.n[6])*uint64(val2.n[7]) +
		uint64(val.n[7])*uint64(val2.n[6]) +
		uint64(val.n[8])*uint64(val2.n[5]) +
		uint64(val.n[9])*uint64(val2.n[4])
	t13 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*14).
	m = (m >> nfieldBase) +
		uint64(val.n[5])*uint64(val2.n[9]) +
		uint64(val.n[6])*uint64(val2.n[8]) +
		uint64(val.n[7])*uint64(val2.n[7]) +
		uint64(val.n[8])*uint64(val2.n[6]) +
		uint64(val.n[9])*uint64(val2.n[5])
	t14 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*15).
	m = (m >> nfieldBase) +
		uint64(val.n[6])*uint64(val2.n[9]) +
		uint64(val.n[7])*uint64(val2.n[8]) +
		uint64(val.n[8])*uint64(val2.n[7]) +
		uint64(val.n[9])*uint64(val2.n[6])
	t15 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*16).
	m = (m >> nfieldBase) +
		uint64(val.n[7])*uint64(val2.n[9]) +
		uint64(val.n[8])*uint64(val2.n[8]) +
		uint64(val.n[9])*uint64(val2.n[7])
	t16 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*17).
	m = (m >> nfieldBase) +
		uint64(val.n[8])*uint64(val2.n[9]) +
		uint64(val.n[9])*uint64(val2.n[8])
	t17 := m & nfieldBaseMask

	// Terms for 2^(nfieldBase*18).
	m = (m >> nfieldBase) + uint64(val.n[9])*uint64(val2.n[9])
	t18 := m & nfieldBaseMask

	// What's left is for 2^(nfieldBase*19).
	t19 := m >> nfieldBase

	// so far, we're really close, but since 2^256 and N are not the same
	// (gap of about 2^128), we need to account for additional magnitudes
	// based on the reducers
	// We start by getting accounting for the t15-19 terms by adding
	// the reduction terms into t5-t14

	m = t5 + t15*nfieldReduceZero
	t5 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t6 + t16*nfieldReduceZero + t15*nfieldReduceOne
	t6 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t7 + t17*nfieldReduceZero + t16*nfieldReduceOne + t15*nfieldReduceTwo
	t7 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t8 + t18*nfieldReduceZero + t17*nfieldReduceOne + t16*nfieldReduceTwo + t15*nfieldReduceThree
	t8 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t9 + t19*nfieldReduceZero + t18*nfieldReduceOne + t17*nfieldReduceTwo + t16*nfieldReduceThree + t15*nfieldReduceFour
	t9 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t10 + t19*nfieldReduceOne + t18*nfieldReduceTwo + t17*nfieldReduceThree + t16*nfieldReduceFour
	t10 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t11 + t19*nfieldReduceTwo + t18*nfieldReduceThree + t17*nfieldReduceFour
	t11 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t12 + t19*nfieldReduceThree + t18*nfieldReduceFour
	t12 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t13 + t19*nfieldReduceFour
	t13 = m & nfieldBaseMask
	t14 = (m >> nfieldBase) + t14

	// now we need to add the reduction terms of t10-t14 into t0-t9

	m = t0 + t10*nfieldReduceZero
	t0 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t1 + t11*nfieldReduceZero + t10*nfieldReduceOne
	t1 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t2 + t12*nfieldReduceZero + t11*nfieldReduceOne + t10*nfieldReduceTwo
	t2 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t3 + t13*nfieldReduceZero + t12*nfieldReduceOne + t11*nfieldReduceTwo + t10*nfieldReduceThree
	t3 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t4 + t14*nfieldReduceZero + t13*nfieldReduceOne + t12*nfieldReduceTwo + t11*nfieldReduceThree + t10*nfieldReduceFour
	t4 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t5 + t14*nfieldReduceOne + t13*nfieldReduceTwo + t12*nfieldReduceThree + t11*nfieldReduceFour
	t5 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t6 + t14*nfieldReduceTwo + t13*nfieldReduceThree + t12*nfieldReduceFour
	t6 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t7 + t14*nfieldReduceThree + t13*nfieldReduceFour
	t7 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t8 + t14*nfieldReduceFour
	t8 = m & nfieldBaseMask
	m = (m >> nfieldBase) + t9
	t9 = m & nfieldBaseMask
	t10 = (m >> nfieldBase) + t10

	// Everything gets shifted over 4 since t10 starts at bit 260
	// whereas we want to start at bit256
	f.n[0] = uint32((t9 >> (nfieldBase - 4)) + (t10<<4)&nfieldBaseMask)
	f.n[1] = uint32((t10 >> (nfieldBase - 4)) + (t11<<4)&nfieldBaseMask)
	f.n[2] = uint32((t11 >> (nfieldBase - 4)) + (t12<<4)&nfieldBaseMask)
	f.n[3] = uint32((t12 >> (nfieldBase - 4)) + (t13<<4)&nfieldBaseMask)
	f.n[4] = uint32((t13 >> (nfieldBase - 4)) + (t14<<4)&nfieldBaseMask)
	f.n[5] = uint32((t14 >> (nfieldBase - 4)) + (t15<<4)&nfieldBaseMask)
	f.n[6] = uint32((t15 >> (nfieldBase - 4)) + (t16<<4)&nfieldBaseMask)
	f.n[7] = uint32((t16 >> (nfieldBase - 4)) + (t17<<4)&nfieldBaseMask)
	f.n[8] = uint32((t17 >> (nfieldBase - 4)) + (t18<<4)&nfieldBaseMask)
	f.n[9] = uint32((t18 >> (nfieldBase - 4)) + (t19 << 4))

	return f
}

func (f *nfieldVal) DebugPrint() {

	fmt.Printf("%x %x %x %x %x\n", f.n[0], f.n[1], f.n[2], f.n[3], f.n[4])
	fmt.Printf("%x %x %x %x %x\n", f.n[5], f.n[6], f.n[7], f.n[8], f.n[9])
}

func (f *nfieldVal) BitLen() int {
	f.Normalize()
	// find the highest bit

	for i, v := range f.n {
		if v != 0 {
			r := 0
			for x := v; x > 0; x >>= 1 {
				r++
			}
			return r + (9-i)*26
		}
	}
	return 0
}
