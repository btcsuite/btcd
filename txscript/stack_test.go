// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"testing"
)

// TestStack tests that all of the stack operations work as expected.
func TestStack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		before         [][]byte
		operation      func(*stack) error
		expectedReturn error
		after          [][]byte
	}{
		{
			"noop",
			[][]byte{{1}, {2}, {3}, {4}, {5}},
			func(s *stack) error {
				return nil
			},
			nil,
			[][]byte{{1}, {2}, {3}, {4}, {5}},
		},
		{
			"peek underflow (byte)",
			[][]byte{{1}, {2}, {3}, {4}, {5}},
			func(s *stack) error {
				_, err := s.PeekByteArray(5)
				return err
			},
			ErrStackUnderflow,
			[][]byte{},
		},
		{
			"peek underflow (int)",
			[][]byte{{1}, {2}, {3}, {4}, {5}},
			func(s *stack) error {
				_, err := s.PeekInt(5)
				return err
			},
			ErrStackUnderflow,
			[][]byte{},
		},
		{
			"peek underflow (bool)",
			[][]byte{{1}, {2}, {3}, {4}, {5}},
			func(s *stack) error {
				_, err := s.PeekBool(5)
				return err
			},
			ErrStackUnderflow,
			[][]byte{},
		},
		{
			"pop",
			[][]byte{{1}, {2}, {3}, {4}, {5}},
			func(s *stack) error {
				val, err := s.PopByteArray()
				if err != nil {
					return err
				}
				if !bytes.Equal(val, []byte{5}) {
					return errors.New("not equal")
				}
				return err
			},
			nil,
			[][]byte{{1}, {2}, {3}, {4}},
		},
		{
			"pop",
			[][]byte{{1}, {2}, {3}, {4}, {5}},
			func(s *stack) error {
				val, err := s.PopByteArray()
				if err != nil {
					return err
				}
				if !bytes.Equal(val, []byte{5}) {
					return errors.New("not equal")
				}
				return err
			},
			nil,
			[][]byte{{1}, {2}, {3}, {4}},
		},
		{
			"pop everything",
			[][]byte{{1}, {2}, {3}, {4}, {5}},
			func(s *stack) error {
				for i := 0; i < 5; i++ {
					_, err := s.PopByteArray()
					if err != nil {
						return err
					}
				}
				return nil
			},
			nil,
			[][]byte{},
		},
		{
			"pop underflow",
			[][]byte{{1}, {2}, {3}, {4}, {5}},
			func(s *stack) error {
				for i := 0; i < 6; i++ {
					_, err := s.PopByteArray()
					if err != nil {
						return err
					}
				}
				return nil
			},
			ErrStackUnderflow,
			[][]byte{},
		},
		{
			"pop bool",
			[][]byte{{0}},
			func(s *stack) error {
				val, err := s.PopBool()
				if err != nil {
					return err
				}

				if val != false {
					return errors.New("unexpected value")
				}
				return nil
			},
			nil,
			[][]byte{},
		},
		{
			"pop bool",
			[][]byte{{1}},
			func(s *stack) error {
				val, err := s.PopBool()
				if err != nil {
					return err
				}

				if val != true {
					return errors.New("unexpected value")
				}
				return nil
			},
			nil,
			[][]byte{},
		},
		{
			"pop bool",
			[][]byte{},
			func(s *stack) error {
				_, err := s.PopBool()
				if err != nil {
					return err
				}

				return nil
			},
			ErrStackUnderflow,
			[][]byte{},
		},
		{
			"popInt 0",
			[][]byte{{0x0}},
			func(s *stack) error {
				v, err := s.PopInt()
				if err != nil {
					return err
				}
				if v.Sign() != 0 {
					return errors.New("0 != 0 on popInt")
				}
				return nil
			},
			nil,
			[][]byte{},
		},
		{
			"popInt -0",
			[][]byte{{0x80}},
			func(s *stack) error {
				v, err := s.PopInt()
				if err != nil {
					return err
				}
				if v.Sign() != 0 {
					return errors.New("-0 != 0 on popInt")
				}
				return nil
			},
			nil,
			[][]byte{},
		},
		{
			"popInt 1",
			[][]byte{{0x01}},
			func(s *stack) error {
				v, err := s.PopInt()
				if err != nil {
					return err
				}
				if v.Cmp(big.NewInt(1)) != 0 {
					return errors.New("1 != 1 on popInt")
				}
				return nil
			},
			nil,
			[][]byte{},
		},
		{
			"popInt 1 leading 0",
			[][]byte{{0x01, 0x00, 0x00, 0x00}},
			func(s *stack) error {
				v, err := s.PopInt()
				if err != nil {
					return err
				}
				if v.Cmp(big.NewInt(1)) != 0 {
					fmt.Printf("%v != %v\n", v, big.NewInt(1))
					return errors.New("1 != 1 on popInt")
				}
				return nil
			},
			nil,
			[][]byte{},
		},
		{
			"popInt -1",
			[][]byte{{0x81}},
			func(s *stack) error {
				v, err := s.PopInt()
				if err != nil {
					return err
				}
				if v.Cmp(big.NewInt(-1)) != 0 {
					return errors.New("1 != 1 on popInt")
				}
				return nil
			},
			nil,
			[][]byte{},
		},
		{
			"popInt -1 leading 0",
			[][]byte{{0x01, 0x00, 0x00, 0x80}},
			func(s *stack) error {
				v, err := s.PopInt()
				if err != nil {
					return err
				}
				if v.Cmp(big.NewInt(-1)) != 0 {
					fmt.Printf("%v != %v\n", v, big.NewInt(-1))
					return errors.New("-1 != -1 on popInt")
				}
				return nil
			},
			nil,
			[][]byte{},
		},
		// Triggers the multibyte case in asInt
		{
			"popInt -513",
			[][]byte{{0x1, 0x82}},
			func(s *stack) error {
				v, err := s.PopInt()
				if err != nil {
					return err
				}
				if v.Cmp(big.NewInt(-513)) != 0 {
					fmt.Printf("%v != %v\n", v, big.NewInt(-513))
					return errors.New("1 != 1 on popInt")
				}
				return nil
			},
			nil,
			[][]byte{},
		},
		// Confirm that the asInt code doesn't modify the base data.
		{
			"peekint nomodify -1",
			[][]byte{{0x01, 0x00, 0x00, 0x80}},
			func(s *stack) error {
				v, err := s.PeekInt(0)
				if err != nil {
					return err
				}
				if v.Cmp(big.NewInt(-1)) != 0 {
					fmt.Printf("%v != %v\n", v, big.NewInt(-1))
					return errors.New("-1 != -1 on popInt")
				}
				return nil
			},
			nil,
			[][]byte{{0x01, 0x00, 0x00, 0x80}},
		},
		{
			"PushInt 0",
			[][]byte{},
			func(s *stack) error {
				s.PushInt(big.NewInt(0))
				return nil
			},
			nil,
			[][]byte{{}},
		},
		{
			"PushInt 1",
			[][]byte{},
			func(s *stack) error {
				s.PushInt(big.NewInt(1))
				return nil
			},
			nil,
			[][]byte{{0x1}},
		},
		{
			"PushInt -1",
			[][]byte{},
			func(s *stack) error {
				s.PushInt(big.NewInt(-1))
				return nil
			},
			nil,
			[][]byte{{0x81}},
		},
		{
			"PushInt two bytes",
			[][]byte{},
			func(s *stack) error {
				s.PushInt(big.NewInt(256))
				return nil
			},
			nil,
			// little endian.. *sigh*
			[][]byte{{0x00, 0x01}},
		},
		{
			"PushInt leading zeros",
			[][]byte{},
			func(s *stack) error {
				// this will have the highbit set
				s.PushInt(big.NewInt(128))
				return nil
			},
			nil,
			[][]byte{{0x80, 0x00}},
		},
		{
			"dup",
			[][]byte{{1}},
			func(s *stack) error {
				err := s.DupN(1)
				if err != nil {
					return err
				}

				return nil
			},
			nil,
			[][]byte{{1}, {1}},
		},
		{
			"dup2",
			[][]byte{{1}, {2}},
			func(s *stack) error {
				err := s.DupN(2)
				if err != nil {
					return err
				}

				return nil
			},
			nil,
			[][]byte{{1}, {2}, {1}, {2}},
		},
		{
			"dup3",
			[][]byte{{1}, {2}, {3}},
			func(s *stack) error {
				err := s.DupN(3)
				if err != nil {
					return err
				}

				return nil
			},
			nil,
			[][]byte{{1}, {2}, {3}, {1}, {2}, {3}},
		},
		{
			"dup0",
			[][]byte{{1}},
			func(s *stack) error {
				err := s.DupN(0)
				if err != nil {
					return err
				}

				return nil
			},
			ErrStackInvalidArgs,
			[][]byte{},
		},
		{
			"dup-1",
			[][]byte{{1}},
			func(s *stack) error {
				err := s.DupN(-1)
				if err != nil {
					return err
				}

				return nil
			},
			ErrStackInvalidArgs,
			[][]byte{},
		},
		{
			"dup too much",
			[][]byte{{1}},
			func(s *stack) error {
				err := s.DupN(2)
				if err != nil {
					return err
				}

				return nil
			},
			ErrStackUnderflow,
			[][]byte{},
		},
		{
			"dup-1",
			[][]byte{{1}},
			func(s *stack) error {
				err := s.DupN(-1)
				if err != nil {
					return err
				}

				return nil
			},
			ErrStackInvalidArgs,
			[][]byte{},
		},
		{
			"PushBool true",
			[][]byte{},
			func(s *stack) error {
				s.PushBool(true)

				return nil
			},
			nil,
			[][]byte{{1}},
		},
		{
			"PushBool false",
			[][]byte{},
			func(s *stack) error {
				s.PushBool(false)

				return nil
			},
			nil,
			[][]byte{{0}},
		},
		{
			"PushBool PopBool",
			[][]byte{},
			func(s *stack) error {
				s.PushBool(true)
				val, err := s.PopBool()
				if err != nil {
					return err
				}
				if val != true {
					return errors.New("unexpected value")
				}

				return nil
			},
			nil,
			[][]byte{},
		},
		{
			"PushBool PopBool 2",
			[][]byte{},
			func(s *stack) error {
				s.PushBool(false)
				val, err := s.PopBool()
				if err != nil {
					return err
				}
				if val != false {
					return errors.New("unexpected value")
				}

				return nil
			},
			nil,
			[][]byte{},
		},
		{
			"PushInt PopBool",
			[][]byte{},
			func(s *stack) error {
				s.PushInt(big.NewInt(1))
				val, err := s.PopBool()
				if err != nil {
					return err
				}
				if val != true {
					return errors.New("unexpected value")
				}

				return nil
			},
			nil,
			[][]byte{},
		},
		{
			"PushInt PopBool 2",
			[][]byte{},
			func(s *stack) error {
				s.PushInt(big.NewInt(0))
				val, err := s.PopBool()
				if err != nil {
					return err
				}
				if val != false {
					return errors.New("unexpected value")
				}

				return nil
			},
			nil,
			[][]byte{},
		},
		{
			"PushInt PopBool 2",
			[][]byte{},
			func(s *stack) error {
				s.PushInt(big.NewInt(0))
				val, err := s.PopBool()
				if err != nil {
					return err
				}
				if val != false {
					return errors.New("unexpected value")
				}

				return nil
			},
			nil,
			[][]byte{},
		},
		{
			"Nip top",
			[][]byte{{1}, {2}, {3}},
			func(s *stack) error {
				return s.NipN(0)
			},
			nil,
			[][]byte{{1}, {2}},
		},
		{
			"Nip middle",
			[][]byte{{1}, {2}, {3}},
			func(s *stack) error {
				return s.NipN(1)
			},
			nil,
			[][]byte{{1}, {3}},
		},
		{
			"Nip low",
			[][]byte{{1}, {2}, {3}},
			func(s *stack) error {
				return s.NipN(2)
			},
			nil,
			[][]byte{{2}, {3}},
		},
		{
			"Nip too much",
			[][]byte{{1}, {2}, {3}},
			func(s *stack) error {
				// bite off more than we can chew
				return s.NipN(3)
			},
			ErrStackUnderflow,
			[][]byte{{2}, {3}},
		},
		{
			"keep on tucking",
			[][]byte{{1}, {2}, {3}},
			func(s *stack) error {
				return s.Tuck()
			},
			nil,
			[][]byte{{1}, {3}, {2}, {3}},
		},
		{
			"a little tucked up",
			[][]byte{{1}}, // too few arguments for tuck
			func(s *stack) error {
				return s.Tuck()
			},
			ErrStackUnderflow,
			[][]byte{},
		},
		{
			"all tucked up",
			[][]byte{}, // too few arguments  for tuck
			func(s *stack) error {
				return s.Tuck()
			},
			ErrStackUnderflow,
			[][]byte{},
		},
		{
			"drop 1",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.DropN(1)
			},
			nil,
			[][]byte{{1}, {2}, {3}},
		},
		{
			"drop 2",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.DropN(2)
			},
			nil,
			[][]byte{{1}, {2}},
		},
		{
			"drop 3",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.DropN(3)
			},
			nil,
			[][]byte{{1}},
		},
		{
			"drop 4",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.DropN(4)
			},
			nil,
			[][]byte{},
		},
		{
			"drop 4/5",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.DropN(5)
			},
			ErrStackUnderflow,
			[][]byte{},
		},
		{
			"drop invalid",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.DropN(0)
			},
			ErrStackInvalidArgs,
			[][]byte{},
		},
		{
			"Rot1",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.RotN(1)
			},
			nil,
			[][]byte{{1}, {3}, {4}, {2}},
		},
		{
			"Rot2",
			[][]byte{{1}, {2}, {3}, {4}, {5}, {6}},
			func(s *stack) error {
				return s.RotN(2)
			},
			nil,
			[][]byte{{3}, {4}, {5}, {6}, {1}, {2}},
		},
		{
			"Rot too little",
			[][]byte{{1}, {2}},
			func(s *stack) error {
				return s.RotN(1)
			},
			ErrStackUnderflow,
			[][]byte{},
		},
		{
			"Rot0",
			[][]byte{{1}, {2}, {3}},
			func(s *stack) error {
				return s.RotN(0)
			},
			ErrStackInvalidArgs,
			[][]byte{},
		},
		{
			"Swap1",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.SwapN(1)
			},
			nil,
			[][]byte{{1}, {2}, {4}, {3}},
		},
		{
			"Swap2",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.SwapN(2)
			},
			nil,
			[][]byte{{3}, {4}, {1}, {2}},
		},
		{
			"Swap too little",
			[][]byte{{1}},
			func(s *stack) error {
				return s.SwapN(1)
			},
			ErrStackUnderflow,
			[][]byte{},
		},
		{
			"Swap0",
			[][]byte{{1}, {2}, {3}},
			func(s *stack) error {
				return s.SwapN(0)
			},
			ErrStackInvalidArgs,
			[][]byte{},
		},
		{
			"Over1",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.OverN(1)
			},
			nil,
			[][]byte{{1}, {2}, {3}, {4}, {3}},
		},
		{
			"Over2",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.OverN(2)
			},
			nil,
			[][]byte{{1}, {2}, {3}, {4}, {1}, {2}},
		},
		{
			"Over too little",
			[][]byte{{1}},
			func(s *stack) error {
				return s.OverN(1)
			},
			ErrStackUnderflow,
			[][]byte{},
		},
		{
			"Over0",
			[][]byte{{1}, {2}, {3}},
			func(s *stack) error {
				return s.OverN(0)
			},
			ErrStackInvalidArgs,
			[][]byte{},
		},
		{
			"Pick1",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.PickN(1)
			},
			nil,
			[][]byte{{1}, {2}, {3}, {4}, {3}},
		},
		{
			"Pick2",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.PickN(2)
			},
			nil,
			[][]byte{{1}, {2}, {3}, {4}, {2}},
		},
		{
			"Pick too little",
			[][]byte{{1}},
			func(s *stack) error {
				return s.PickN(1)
			},
			ErrStackUnderflow,
			[][]byte{},
		},
		{
			"Roll1",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.RollN(1)
			},
			nil,
			[][]byte{{1}, {2}, {4}, {3}},
		},
		{
			"Roll2",
			[][]byte{{1}, {2}, {3}, {4}},
			func(s *stack) error {
				return s.RollN(2)
			},
			nil,
			[][]byte{{1}, {3}, {4}, {2}},
		},
		{
			"Roll too little",
			[][]byte{{1}},
			func(s *stack) error {
				return s.RollN(1)
			},
			ErrStackUnderflow,
			[][]byte{},
		},
		{
			"Peek bool",
			[][]byte{{1}},
			func(s *stack) error {
				// Peek bool is otherwise pretty well tested,
				// just check it works.
				val, err := s.PeekBool(0)
				if err != nil {
					return err
				}
				if val != true {
					return errors.New("invalid result")
				}
				return nil
			},
			nil,
			[][]byte{{1}},
		},
		{
			"Peek bool 2",
			[][]byte{{0}},
			func(s *stack) error {
				// Peek bool is otherwise pretty well tested,
				// just check it works.
				val, err := s.PeekBool(0)
				if err != nil {
					return err
				}
				if val != false {
					return errors.New("invalid result")
				}
				return nil
			},
			nil,
			[][]byte{{0}},
		},
		{
			"Peek int",
			[][]byte{{1}},
			func(s *stack) error {
				// Peek int is otherwise pretty well tested,
				// just check it works.
				val, err := s.PeekInt(0)
				if err != nil {
					return err
				}
				if val.Cmp(big.NewInt(1)) != 0 {
					return errors.New("invalid result")
				}
				return nil
			},
			nil,
			[][]byte{{1}},
		},
		{
			"Peek int 2",
			[][]byte{{0}},
			func(s *stack) error {
				// Peek int is otherwise pretty well tested,
				// just check it works.
				val, err := s.PeekInt(0)
				if err != nil {
					return err
				}
				if val.Cmp(big.NewInt(0)) != 0 {
					return errors.New("invalid result")
				}
				return nil
			},
			nil,
			[][]byte{{0}},
		},
		{
			"pop int",
			[][]byte{},
			func(s *stack) error {
				s.PushInt(big.NewInt(1))
				// Peek int is otherwise pretty well tested,
				// just check it works.
				val, err := s.PopInt()
				if err != nil {
					return err
				}
				if val.Cmp(big.NewInt(1)) != 0 {
					return errors.New("invalid result")
				}
				return nil
			},
			nil,
			[][]byte{},
		},
		{
			"pop empty",
			[][]byte{},
			func(s *stack) error {
				// Peek int is otherwise pretty well tested,
				// just check it works.
				_, err := s.PopInt()
				return err
			},
			ErrStackUnderflow,
			[][]byte{},
		},
	}

	for _, test := range tests {
		s := stack{}

		for i := range test.before {
			s.PushByteArray(test.before[i])
		}
		err := test.operation(&s)
		if err != test.expectedReturn {
			t.Errorf("%s: operation return not what expected: %v "+
				"vs %v", test.name, err, test.expectedReturn)
		}
		if err != nil {
			continue
		}

		if len(test.after) != s.Depth() {
			t.Errorf("%s: stack depth doesn't match expected: %v "+
				"vs %v", test.name, len(test.after),
				s.Depth())
		}

		for i := range test.after {
			val, err := s.PeekByteArray(s.Depth() - i - 1)
			if err != nil {
				t.Errorf("%s: can't peek %dth stack entry: %v",
					test.name, i, err)
				break
			}

			if !bytes.Equal(val, test.after[i]) {
				t.Errorf("%s: %dth stack entry doesn't match "+
					"expected: %v vs %v", test.name, i, val,
					test.after[i])
				break
			}
		}
	}
}
