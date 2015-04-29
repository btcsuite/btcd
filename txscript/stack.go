// Copyright (c) 2013-2015 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"encoding/hex"
	"math/big"
)

// asInt converts a byte array to a bignum by treating it as a little endian
// number with sign bit.
func asInt(v []byte) (*big.Int, error) {
	// Only 32bit numbers allowed.
	if len(v) > 4 {
		return nil, ErrStackNumberTooBig
	}
	if len(v) == 0 {
		return big.NewInt(0), nil
	}
	negative := false
	origlen := len(v)
	msb := v[len(v)-1]
	if msb&0x80 == 0x80 {
		negative = true
		// remove sign bit
		msb &= 0x7f
	}
	// trim leading 0 bytes
	for ; msb == 0; msb = v[len(v)-1] {
		v = v[:len(v)-1]
		if len(v) == 0 {
			break
		}
	}
	// reverse bytes with a copy since stack is immutable.
	intArray := make([]byte, len(v))
	for i := range v {
		intArray[len(v)-i-1] = v[i]
	}
	// IFF the value is negative and no 0 bytes were trimmed,
	// the leading byte needs to be sign corrected
	if negative && len(intArray) == origlen {
		intArray[0] &= 0x7f
	}

	num := new(big.Int).SetBytes(intArray)
	if negative {
		num = num.Neg(num)
	}
	return num, nil
}

// fromInt provies a Big.Int in little endian format with the high bit of the
// msb donating sign.
func fromInt(v *big.Int) []byte {
	negative := false
	if v.Sign() == -1 {
		negative = true
	}
	// Int.Bytes() trims leading zeros for us, so we don't have to.
	b := v.Bytes()
	if len(b) == 0 {
		return []byte{}
	}
	arr := make([]byte, len(b))
	for i := range b {
		arr[len(b)-i-1] = b[i]
	}
	// if would otherwise be negative, add a zero byte
	if arr[len(arr)-1]&0x80 == 0x80 {
		arr = append(arr, 0)
	}
	if negative {
		arr[len(arr)-1] |= 0x80
	}
	return arr
}

// asBool gets the boolean value of the byte array.
func asBool(t []byte) bool {
	for i := range t {
		if t[i] != 0 {
			return true
		}
	}
	return false
}

// fromBool converts a boolean into the appropriate byte array.
func fromBool(v bool) []byte {
	if v {
		return []byte{1}
	}
	return []byte{0}
}

// stack represents a stack of immutable objects to be used with bitcoin
// scripts.  Objects may be shared, therefore in usage if a value is to be
// changed it *must* be deep-copied first to avoid changing other values on the
// stack.
type stack struct {
	stk               [][]byte
	verifyMinimalData bool
}

// checkMinimalData returns whether or not the passed byte array adheres to
// the minimal encoding requirements, if enabled.
func (s *stack) checkMinimalData(so []byte) error {
	if !s.verifyMinimalData || len(so) == 0 {
		return nil
	}

	// Check that the number is encoded with the minimum possible
	// number of bytes.
	//
	// If the most-significant-byte - excluding the sign bit - is zero
	// then we're not minimal. Note how this test also rejects the
	// negative-zero encoding, 0x80.
	if so[len(so)-1]&0x7f == 0 {
		// One exception: if there's more than one byte and the most
		// significant bit of the second-most-significant-byte is set
		// it would conflict with the sign bit. An example of this case
		// is +-255, which encode to 0xff00 and 0xff80 respectively.
		// (big-endian).
		if len(so) == 1 || so[len(so)-2]&0x80 == 0 {
			return ErrStackMinimalData
		}
	}
	return nil
}

// Depth returns the number of items on the stack.
func (s *stack) Depth() int {
	return len(s.stk)
}

// PushByteArray adds the given back array to the top of the stack.
//
// Stack transformation: [... x1 x2] -> [... x1 x2 data]
func (s *stack) PushByteArray(so []byte) {
	s.stk = append(s.stk, so)
}

// PushInt converts the provided bignum to a suitable byte array then pushes
// it onto the top of the stack.
//
// Stack transformation: [... x1 x2] -> [... x1 x2 int]
func (s *stack) PushInt(val *big.Int) {
	s.PushByteArray(fromInt(val))
}

// PushBool converts the provided boolean to a suitable byte array then pushes
// it onto the top of the stack.
//
// Stack transformation: [... x1 x2] -> [... x1 x2 bool]
func (s *stack) PushBool(val bool) {
	s.PushByteArray(fromBool(val))
}

// PopByteArray pops the value off the top of the stack and returns it.
//
// Stack transformation: [... x1 x2 x3] -> [... x1 x2]
func (s *stack) PopByteArray() ([]byte, error) {
	return s.nipN(0)
}

// PopInt pops the value off the top of the stack, converts it into a bignum and
// returns it.
//
// Stack transformation: [... x1 x2 x3] -> [... x1 x2]
func (s *stack) PopInt() (*big.Int, error) {
	so, err := s.PopByteArray()
	if err != nil {
		return nil, err
	}

	if err := s.checkMinimalData(so); err != nil {
		return nil, err
	}

	return asInt(so)
}

// PopBool pops the value off the top of the stack, converts it into a bool, and
// returns it.
//
// Stack transformation: [... x1 x2 x3] -> [... x1 x2]
func (s *stack) PopBool() (bool, error) {
	so, err := s.PopByteArray()
	if err != nil {
		return false, err
	}

	return asBool(so), nil
}

// PeekByteArray returns the nth item on the stack without removing it.
func (s *stack) PeekByteArray(idx int) ([]byte, error) {
	sz := len(s.stk)
	if idx < 0 || idx >= sz {
		return nil, ErrStackUnderflow
	}

	return s.stk[sz-idx-1], nil
}

// PeekInt returns the Nth item on the stack as a bignum without removing it.
func (s *stack) PeekInt(idx int) (*big.Int, error) {
	so, err := s.PeekByteArray(idx)
	if err != nil {
		return nil, err
	}

	if err := s.checkMinimalData(so); err != nil {
		return nil, err
	}

	return asInt(so)
}

// PeekBool returns the Nth item on the stack as a bool without removing it.
func (s *stack) PeekBool(idx int) (i bool, err error) {
	so, err := s.PeekByteArray(idx)
	if err != nil {
		return false, err
	}

	return asBool(so), nil
}

// nipN is an internal function that removes the nth item on the stack and
// returns it.
//
// Stack transformation:
// nipN(0): [... x1 x2 x3] -> [... x1 x2]
// nipN(1): [... x1 x2 x3] -> [... x1 x3]
// nipN(2): [... x1 x2 x3] -> [... x2 x3]
func (s *stack) nipN(idx int) ([]byte, error) {
	sz := len(s.stk)
	if idx < 0 || idx > sz-1 {
		return nil, ErrStackUnderflow
	}

	so := s.stk[sz-idx-1]
	if idx == 0 {
		s.stk = s.stk[:sz-1]
	} else if idx == sz-1 {
		s1 := make([][]byte, sz-1, sz-1)
		copy(s1, s.stk[1:])
		s.stk = s1
	} else {
		s1 := s.stk[sz-idx : sz]
		s.stk = s.stk[:sz-idx-1]
		s.stk = append(s.stk, s1...)
	}
	return so, nil
}

// NipN removes the Nth object on the stack
//
// Stack transformation:
// NipN(0): [... x1 x2 x3] -> [... x1 x2]
// NipN(1): [... x1 x2 x3] -> [... x1 x3]
// NipN(2): [... x1 x2 x3] -> [... x2 x3]
func (s *stack) NipN(idx int) error {
	_, err := s.nipN(idx)
	return err
}

// Tuck copies the item at the top of the stack and inserts it before the 2nd
// to top item.
//
// Stack transformation: [... x1 x2] -> [... x2 x1 x2]
func (s *stack) Tuck() error {
	so2, err := s.PopByteArray()
	if err != nil {
		return err
	}
	so1, err := s.PopByteArray()
	if err != nil {
		return err
	}
	s.PushByteArray(so2) // stack [... x2]
	s.PushByteArray(so1) // stack [... x2 x1]
	s.PushByteArray(so2) // stack [... x2 x1 x2]

	return nil
}

// DropN removes the top N items from the stack.
//
// Stack transformation:
// DropN(1): [... x1 x2] -> [... x1]
// DropN(2): [... x1 x2] -> [...]
func (s *stack) DropN(n int) error {
	if n < 1 {
		return ErrStackInvalidArgs
	}

	for ; n > 0; n-- {
		_, err := s.PopByteArray()
		if err != nil {
			return err
		}
	}
	return nil
}

// DupN duplicates the top N items on the stack.
//
// Stack transformation:
// DupN(1): [... x1 x2] -> [... x1 x2 x2]
// DupN(2): [... x1 x2] -> [... x1 x2 x1 x2]
func (s *stack) DupN(n int) error {
	if n < 1 {
		return ErrStackInvalidArgs
	}

	// Iteratively duplicate the value n-1 down the stack n times.
	// This leaves an in-order duplicate of the top n items on the stack.
	for i := n; i > 0; i-- {
		so, err := s.PeekByteArray(n - 1)
		if err != nil {
			return err
		}
		s.PushByteArray(so)
	}
	return nil
}

// RotN rotates the top 3N items on the stack to the left N times.
//
// Stack transformation:
// RotN(1): [... x1 x2 x3] -> [... x2 x3 x1]
// RotN(2): [... x1 x2 x3 x4 x5 x6] -> [... x3 x4 x5 x6 x1 x2]
func (s *stack) RotN(n int) error {
	if n < 1 {
		return ErrStackInvalidArgs
	}

	// Nip the 3n-1th item from the stack to the top n times to rotate
	// them up to the head of the stack.
	entry := 3*n - 1
	for i := n; i > 0; i-- {
		so, err := s.nipN(entry)
		if err != nil {
			return err
		}

		s.PushByteArray(so)
	}
	return nil
}

// SwapN swaps the top N items on the stack with those below them.
//
// Stack transformation:
// SwapN(1): [... x1 x2] -> [... x2 x1]
// SwapN(2): [... x1 x2 x3 x4] -> [... x3 x4 x1 x2]
func (s *stack) SwapN(n int) error {
	if n < 1 {
		return ErrStackInvalidArgs
	}

	entry := 2*n - 1
	for i := n; i > 0; i-- {
		// Swap 2n-1th entry to top.
		so, err := s.nipN(entry)
		if err != nil {
			return err
		}

		s.PushByteArray(so)
	}
	return nil
}

// OverN copies N items N items back to the top of the stack.
//
// Stack transformation:
// OverN(1): [... x1 x2 x3] -> [... x1 x2 x3 x2]
// OverN(2): [... x1 x2 x3 x4] -> [... x1 x2 x3 x4 x1 x2]
func (s *stack) OverN(n int) error {
	if n < 1 {
		return ErrStackInvalidArgs
	}

	// Copy 2n-1th entry to top of the stack.
	entry := 2*n - 1
	for ; n > 0; n-- {
		so, err := s.PeekByteArray(entry)
		if err != nil {
			return err
		}
		s.PushByteArray(so)
	}

	return nil
}

// PickN copies the item N items back in the stack to the top.
//
// Stack transformation:
// PickN(0): [x1 x2 x3] -> [x1 x2 x3 x3]
// PickN(1): [x1 x2 x3] -> [x1 x2 x3 x2]
// PickN(2): [x1 x2 x3] -> [x1 x2 x3 x1]
func (s *stack) PickN(n int) error {
	so, err := s.PeekByteArray(n)
	if err != nil {
		return err
	}
	s.PushByteArray(so)

	return nil
}

// RollN moves the item N items back in the stack to the top.
//
// Stack transformation:
// RollN(0): [x1 x2 x3] -> [x1 x2 x3]
// RollN(1): [x1 x2 x3] -> [x1 x3 x2]
// RollN(2): [x1 x2 x3] -> [x2 x3 x1]
func (s *stack) RollN(n int) error {
	so, err := s.nipN(n)
	if err != nil {
		return err
	}

	s.PushByteArray(so)

	return nil
}

// String returns the stack in a readable format.
func (s *stack) String() string {
	var result string
	for _, stack := range s.stk {
		result += hex.Dump(stack)
	}

	return result
}
