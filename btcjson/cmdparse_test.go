// Copyright (c) 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson_test

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
)

// TestAssignField tests the assignField function handles supported combinations
// properly.
func TestAssignField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		dest     interface{}
		src      interface{}
		expected interface{}
	}{
		{
			name:     "same types",
			dest:     int8(0),
			src:      int8(100),
			expected: int8(100),
		},
		{
			name: "same types - more source pointers",
			dest: int8(0),
			src: func() interface{} {
				i := int8(100)
				return &i
			}(),
			expected: int8(100),
		},
		{
			name: "same types - more dest pointers",
			dest: func() interface{} {
				i := int8(0)
				return &i
			}(),
			src:      int8(100),
			expected: int8(100),
		},
		{
			name: "convertible types - more source pointers",
			dest: int16(0),
			src: func() interface{} {
				i := int8(100)
				return &i
			}(),
			expected: int16(100),
		},
		{
			name: "convertible types - both pointers",
			dest: func() interface{} {
				i := int8(0)
				return &i
			}(),
			src: func() interface{} {
				i := int16(100)
				return &i
			}(),
			expected: int8(100),
		},
		{
			name:     "convertible types - int16 -> int8",
			dest:     int8(0),
			src:      int16(100),
			expected: int8(100),
		},
		{
			name:     "convertible types - int16 -> uint8",
			dest:     uint8(0),
			src:      int16(100),
			expected: uint8(100),
		},
		{
			name:     "convertible types - uint16 -> int8",
			dest:     int8(0),
			src:      uint16(100),
			expected: int8(100),
		},
		{
			name:     "convertible types - uint16 -> uint8",
			dest:     uint8(0),
			src:      uint16(100),
			expected: uint8(100),
		},
		{
			name:     "convertible types - float32 -> float64",
			dest:     float64(0),
			src:      float32(1.5),
			expected: float64(1.5),
		},
		{
			name:     "convertible types - float64 -> float32",
			dest:     float32(0),
			src:      float64(1.5),
			expected: float32(1.5),
		},
		{
			name:     "convertible types - string -> bool",
			dest:     false,
			src:      "true",
			expected: true,
		},
		{
			name:     "convertible types - string -> int8",
			dest:     int8(0),
			src:      "100",
			expected: int8(100),
		},
		{
			name:     "convertible types - string -> uint8",
			dest:     uint8(0),
			src:      "100",
			expected: uint8(100),
		},
		{
			name:     "convertible types - string -> float32",
			dest:     float32(0),
			src:      "1.5",
			expected: float32(1.5),
		},
		{
			name: "convertible types - typecase string -> string",
			dest: "",
			src: func() interface{} {
				type foo string
				return foo("foo")
			}(),
			expected: "foo",
		},
		{
			name:     "convertible types - string -> array",
			dest:     [2]string{},
			src:      `["test","test2"]`,
			expected: [2]string{"test", "test2"},
		},
		{
			name:     "convertible types - string -> slice",
			dest:     []string{},
			src:      `["test","test2"]`,
			expected: []string{"test", "test2"},
		},
		{
			name:     "convertible types - string -> struct",
			dest:     struct{ A int }{},
			src:      `{"A":100}`,
			expected: struct{ A int }{100},
		},
		{
			name:     "convertible types - string -> map",
			dest:     map[string]float64{},
			src:      `{"1Address":1.5}`,
			expected: map[string]float64{"1Address": 1.5},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		dst := reflect.New(reflect.TypeOf(test.dest)).Elem()
		src := reflect.ValueOf(test.src)
		err := btcjson.TstAssignField(1, "testField", dst, src)
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		// Inidirect through to the base types to ensure their values
		// are the same.
		for dst.Kind() == reflect.Ptr {
			dst = dst.Elem()
		}
		if !reflect.DeepEqual(dst.Interface(), test.expected) {
			t.Errorf("Test #%d (%s) unexpected value - got %v, "+
				"want %v", i, test.name, dst.Interface(),
				test.expected)
			continue
		}
	}
}

// TestAssignFieldErrors tests the assignField function error paths.
func TestAssignFieldErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dest interface{}
		src  interface{}
		err  btcjson.Error
	}{
		{
			name: "general incompatible int -> string",
			dest: string(0),
			src:  int(0),
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "overflow source int -> dest int",
			dest: int8(0),
			src:  int(128),
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "overflow source int -> dest uint",
			dest: uint8(0),
			src:  int(256),
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "int -> float",
			dest: float32(0),
			src:  int(256),
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "overflow source uint64 -> dest int64",
			dest: int64(0),
			src:  uint64(1 << 63),
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "overflow source uint -> dest int",
			dest: int8(0),
			src:  uint(128),
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "overflow source uint -> dest uint",
			dest: uint8(0),
			src:  uint(256),
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "uint -> float",
			dest: float32(0),
			src:  uint(256),
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "float -> int",
			dest: int(0),
			src:  float32(1.0),
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "overflow float64 -> float32",
			dest: float32(0),
			src:  float64(math.MaxFloat64),
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "invalid string -> bool",
			dest: true,
			src:  "foo",
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "invalid string -> int",
			dest: int8(0),
			src:  "foo",
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "overflow string -> int",
			dest: int8(0),
			src:  "128",
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "invalid string -> uint",
			dest: uint8(0),
			src:  "foo",
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "overflow string -> uint",
			dest: uint8(0),
			src:  "256",
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "invalid string -> float",
			dest: float32(0),
			src:  "foo",
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "overflow string -> float",
			dest: float32(0),
			src:  "1.7976931348623157e+308",
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "invalid string -> array",
			dest: [3]int{},
			src:  "foo",
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "invalid string -> slice",
			dest: []int{},
			src:  "foo",
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "invalid string -> struct",
			dest: struct{ A int }{},
			src:  "foo",
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "invalid string -> map",
			dest: map[string]int{},
			src:  "foo",
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		dst := reflect.New(reflect.TypeOf(test.dest)).Elem()
		src := reflect.ValueOf(test.src)
		err := btcjson.TstAssignField(1, "testField", dst, src)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Test #%d (%s) wrong error - got %T (%[3]v), "+
				"want %T", i, test.name, err, test.err)
			continue
		}
		gotErrorCode := err.(btcjson.Error).ErrorCode
		if gotErrorCode != test.err.ErrorCode {
			t.Errorf("Test #%d (%s) mismatched error code - got "+
				"%v (%v), want %v", i, test.name, gotErrorCode,
				err, test.err.ErrorCode)
			continue
		}
	}
}

// TestNewCmdErrors ensures the error paths of NewCmd behave as expected.
func TestNewCmdErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		args   []interface{}
		err    btcjson.Error
	}{
		{
			name:   "unregistered command",
			method: "boguscommand",
			args:   []interface{}{},
			err:    btcjson.Error{ErrorCode: btcjson.ErrUnregisteredMethod},
		},
		{
			name:   "too few parameters to command with required + optional",
			method: "getblock",
			args:   []interface{}{},
			err:    btcjson.Error{ErrorCode: btcjson.ErrNumParams},
		},
		{
			name:   "too many parameters to command with no optional",
			method: "getblockcount",
			args:   []interface{}{"123"},
			err:    btcjson.Error{ErrorCode: btcjson.ErrNumParams},
		},
		{
			name:   "incorrect parameter type",
			method: "getblock",
			args:   []interface{}{1},
			err:    btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		_, err := btcjson.NewCmd(test.method, test.args...)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Test #%d (%s) wrong error - got %T (%v), "+
				"want %T", i, test.name, err, err, test.err)
			continue
		}
		gotErrorCode := err.(btcjson.Error).ErrorCode
		if gotErrorCode != test.err.ErrorCode {
			t.Errorf("Test #%d (%s) mismatched error code - got "+
				"%v (%v), want %v", i, test.name, gotErrorCode,
				err, test.err.ErrorCode)
			continue
		}
	}
}

// TestMarshalCmdErrors  tests the error paths of the MarshalCmd function.
func TestMarshalCmdErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   interface{}
		cmd  interface{}
		err  btcjson.Error
	}{
		{
			name: "unregistered type",
			id:   1,
			cmd:  (*int)(nil),
			err:  btcjson.Error{ErrorCode: btcjson.ErrUnregisteredMethod},
		},
		{
			name: "nil instance of registered type",
			id:   1,
			cmd:  (*btcjson.GetBlockCmd)(nil),
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "nil instance of registered type",
			id:   []int{0, 1},
			cmd:  &btcjson.GetBlockCountCmd{},
			err:  btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		_, err := btcjson.MarshalCmd(test.id, test.cmd)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Test #%d (%s) wrong error - got %T (%v), "+
				"want %T", i, test.name, err, err, test.err)
			continue
		}
		gotErrorCode := err.(btcjson.Error).ErrorCode
		if gotErrorCode != test.err.ErrorCode {
			t.Errorf("Test #%d (%s) mismatched error code - got "+
				"%v (%v), want %v", i, test.name, gotErrorCode,
				err, test.err.ErrorCode)
			continue
		}
	}
}

// TestUnmarshalCmdErrors  tests the error paths of the UnmarshalCmd function.
func TestUnmarshalCmdErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request btcjson.Request
		err     btcjson.Error
	}{
		{
			name: "unregistered type",
			request: btcjson.Request{
				Jsonrpc: "1.0",
				Method:  "bogusmethod",
				Params:  nil,
				ID:      nil,
			},
			err: btcjson.Error{ErrorCode: btcjson.ErrUnregisteredMethod},
		},
		{
			name: "incorrect number of params",
			request: btcjson.Request{
				Jsonrpc: "1.0",
				Method:  "getblockcount",
				Params:  []json.RawMessage{[]byte(`"bogusparam"`)},
				ID:      nil,
			},
			err: btcjson.Error{ErrorCode: btcjson.ErrNumParams},
		},
		{
			name: "invalid type for a parameter",
			request: btcjson.Request{
				Jsonrpc: "1.0",
				Method:  "getblock",
				Params:  []json.RawMessage{[]byte("1")},
				ID:      nil,
			},
			err: btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name: "invalid JSON for a parameter",
			request: btcjson.Request{
				Jsonrpc: "1.0",
				Method:  "getblock",
				Params:  []json.RawMessage{[]byte(`"1`)},
				ID:      nil,
			},
			err: btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		_, err := btcjson.UnmarshalCmd(&test.request)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Test #%d (%s) wrong error - got %T (%v), "+
				"want %T", i, test.name, err, err, test.err)
			continue
		}
		gotErrorCode := err.(btcjson.Error).ErrorCode
		if gotErrorCode != test.err.ErrorCode {
			t.Errorf("Test #%d (%s) mismatched error code - got "+
				"%v (%v), want %v", i, test.name, gotErrorCode,
				err, test.err.ErrorCode)
			continue
		}
	}
}
