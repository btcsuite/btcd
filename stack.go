// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcscript

import (
	"math/big"
)

// asInt converts a byte array to a bignum by treating it as a little endian
// number with sign bit.
func asInt(v []byte) (*big.Int, error) {
	// Only 32bit numbers allowed.
	if len(v) > 4 {
		return nil, StackErrNumberTooBig
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

// Stack represents a stack of immutable objects to be used with bitcoin scripts
// Objects may be shared,  therefore in usage if a value is to be changed it
// *must* be deep-copied first to avoid changing other values on the stack.
type Stack struct {
	stk [][]byte
}

// PushByteArray adds the given back array to the top of the stack.
func (s *Stack) PushByteArray(so []byte) {
	s.stk = append(s.stk, so)
}

// PushInt converts the provided bignum to a suitable byte array then pushes
// it onto the top of the stack.
func (s *Stack) PushInt(val *big.Int) {
	s.PushByteArray(fromInt(val))
}

// PushBool converts the provided boolean to a suitable byte array then pushes
// it onto the top of the stack.
func (s *Stack) PushBool(val bool) {
	s.PushByteArray(fromBool(val))
}

// PopByteArray pops the value off the top of the stack and returns it.
func (s *Stack) PopByteArray() ([]byte, error) {
	return s.nipN(0)
}

// PopInt pops the value off the top of the stack, converts it into a bignum and
// returns it.
func (s *Stack) PopInt() (*big.Int, error) {
	so, err := s.PopByteArray()
	if err != nil {
		return nil, err
	}
	return asInt(so)
}

// PopBool pops the value off the top of the stack, converts it into a bool and
// returns it.
func (s *Stack) PopBool() (bool, error) {
	so, err := s.PopByteArray()
	if err != nil {
		return false, err
	}
	return asBool(so), nil
}

// PeekByteArray returns the nth item on the stack without removing it.
func (s *Stack) PeekByteArray(idx int) (so []byte, err error) {
	sz := len(s.stk)
	if idx < 0 || idx >= sz {
		return nil, StackErrUnderflow
	}
	return s.stk[sz-idx-1], nil
}

// PeekInt returns the nth item on the stack as a bignum without removing it.
func (s *Stack) PeekInt(idx int) (i *big.Int, err error) {
	so, err := s.PeekByteArray(idx)
	if err != nil {
		return nil, err
	}
	return asInt(so)
}

// PeekBool returns the nth item on the stack as a bool without removing it.
func (s *Stack) PeekBool(idx int) (i bool, err error) {
	so, err := s.PeekByteArray(idx)
	if err != nil {
		return false, err
	}
	return asBool(so), nil
}

// nipN is an internal function that removes the nth item on the stack and
// returns it.
func (s *Stack) nipN(idx int) (so []byte, err error) {
	sz := len(s.stk)
	if idx < 0 || idx > sz-1 {
		err = StackErrUnderflow
		return
	}
	so = s.stk[sz-idx-1]
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
	return
}

// NipN removes the Nth object on the stack
func (s *Stack) NipN(idx int) error {
	_, err := s.nipN(idx)
	return err
}

// Tuck copies the item at the top of the stack and inserts it before the 2nd
// to top item. e.g.: 2,1 -> 2,1,2
func (s *Stack) Tuck() error {
	so2, err := s.PopByteArray()
	if err != nil {
		return err
	}
	so1, err := s.PopByteArray()
	if err != nil {
		return err
	}
	s.PushByteArray(so2) // stack 2
	s.PushByteArray(so1) // stack 1,2
	s.PushByteArray(so2) // stack 2,1,2

	return nil
}

// Depth returns the number of items on the stack.
func (s *Stack) Depth() (sz int) {
	sz = len(s.stk)
	return
}

// DropN removes the top N items from the stack.
// e.g.
// DropN(1): 1,2,3 -> 1,2
// DropN(2): 1,2,3 -> 1
func (s *Stack) DropN(n int) error {
	if n < 1 {
		return StackErrInvalidArgs
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
// e.g.
// DupN(1): 1,2,3 -> 1,2,3,3
// DupN(2): 1,2,3 -> 1,2,3,2,3
func (s *Stack) DupN(n int) error {
	if n < 1 {
		return StackErrInvalidArgs
	}
	// Iteratively duplicate the value n-1 down the stack n times.
	// this leaves us with an in-order duplicate of the top N items on the
	// stack.
	for i := n; i > 0; i-- {
		so, err := s.PeekByteArray(n - 1)
		if err != nil {
			return err
		}
		s.PushByteArray(so)
	}
	return nil
}

// RotN rotates the top 3N items on the stack to the left
// e.g.
// RotN(1): 1,2,3 -> 2,3,1
func (s *Stack) RotN(n int) error {
	if n < 1 {
		return StackErrInvalidArgs
	}
	entry := 3*n - 1
	// Nip the 3n-1th item from the stack to the top n times to rotate
	// them up to the head of the stack.
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
// E.g.:
// SwapN(1): 1,2 -> 2,1
// SwapN(2): 1,2,3,4 -> 3,4,1,2
func (s *Stack) SwapN(n int) error {
	if n < 1 {
		return StackErrInvalidArgs
	}
	entry := 2*n - 1
	for i := n; i > 0; i-- {
		// swap 2n-1th entry to topj
		so, err := s.nipN(entry)
		if err != nil {
			return err
		}

		s.PushByteArray(so)
	}
	return nil
}

// OverN copies N items N spaces back to the top of the stack.
// e.g.:
// OverN(1): 1,2 -> 1,2,1
// OverN(2): 1,2,3,4 -> 1,2,3,4,1,2
func (s *Stack) OverN(n int) error {
	if n < 1 {
		return StackErrInvalidArgs
	}
	// Copy 2n-1th entry to top of the stack
	entry := 2*n - 1
	for ; n > 0; n-- {
		so, err := s.PeekByteArray(entry)
		if err != nil {
			return err
		}
		s.PushByteArray(so)
		// 4,1,2,3,4, now code original 3rd entry to top.
	}

	return nil
}

// PickN copies the item N items back in the stack to the top.
// e.g.:
// PickN(1): 1,2,3 -> 1,2,3,2
// PickN(2): 1,2,3 -> 1,2,3,1
func (s *Stack) PickN(n int) error {
	so, err := s.PeekByteArray(n)
	if err != nil {
		return err
	}

	s.PushByteArray(so)

	return nil
}

// RollN moves the item N items back in the stack to the top.
// e.g.:
// RollN(1): 1,2,3 -> 1,3,2
// RollN(2): 1,2,3 -> 2,3,1
func (s *Stack) RollN(n int) error {
	so, err := s.nipN(n)
	if err != nil {
		return err
	}

	s.PushByteArray(so)

	return nil
}
