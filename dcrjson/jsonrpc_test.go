// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/decred/dcrd/dcrjson"
)

// TestIsValidIDType ensures the IsValidIDType function behaves as expected.
func TestIsValidIDType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		id      interface{}
		isValid bool
	}{
		{"int", int(1), true},
		{"int8", int8(1), true},
		{"int16", int16(1), true},
		{"int32", int32(1), true},
		{"int64", int64(1), true},
		{"uint", uint(1), true},
		{"uint8", uint8(1), true},
		{"uint16", uint16(1), true},
		{"uint32", uint32(1), true},
		{"uint64", uint64(1), true},
		{"string", "1", true},
		{"nil", nil, true},
		{"float32", float32(1), true},
		{"float64", float64(1), true},
		{"bool", true, false},
		{"chan int", make(chan int), false},
		{"complex64", complex64(1), false},
		{"complex128", complex128(1), false},
		{"func", func() {}, false},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		if dcrjson.IsValidIDType(test.id) != test.isValid {
			t.Errorf("Test #%d (%s) valid mismatch - got %v, "+
				"want %v", i, test.name, !test.isValid,
				test.isValid)
			continue
		}
	}
}

// TestMarshalResponse ensures the MarshalResponse function works as expected.
func TestMarshalResponse(t *testing.T) {
	t.Parallel()
	testID := 1
	tests := []struct {
		name     string
		result   interface{}
		jsonErr  *dcrjson.RPCError
		expected []byte
	}{
		{
			name:     "ordinary bool result with no error",
			result:   true,
			jsonErr:  nil,
			expected: []byte(`{"jsonrpc":"1.0","result":true,"error":null,"id":1}`),
		},
		{
			name:   "result with error",
			result: nil,
			jsonErr: func() *dcrjson.RPCError {
				return dcrjson.NewRPCError(dcrjson.ErrRPCBlockNotFound, "123 not found")
			}(),
			expected: []byte(`{"jsonrpc":"1.0","result":null,"error":{"code":-5,"message":"123 not found"},"id":1}`),
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		_, _ = i, test
		marshalled, err := dcrjson.MarshalResponse("1.0", testID, test.result, test.jsonErr)
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !reflect.DeepEqual(marshalled, test.expected) {
			t.Errorf("Test #%d (%s) mismatched result - got %s, "+
				"want %s", i, test.name, marshalled,
				test.expected)
		}
	}
}

// TestMiscErrors tests a few error conditions not covered elsewhere.
func TestMiscErrors(t *testing.T) {
	t.Parallel()
	// Force an error in NewRequest by giving it a parameter type that is
	// not supported.
	_, err := dcrjson.NewRequest("1.0", nil, "test", []interface{}{make(chan int)})
	if err == nil {
		t.Error("NewRequest: did not receive error")
		return
	}

	// Force an error in MarshalResponse by giving it an id type that is not
	// supported.
	wantErr := dcrjson.Error{Code: dcrjson.ErrInvalidType}
	_, err = dcrjson.MarshalResponse("", make(chan int), nil, nil)
	if jerr, ok := err.(dcrjson.Error); !ok || jerr.Code != wantErr.Code {
		t.Errorf("MarshalResult: did not receive expected error - got "+
			"%v (%[1]T), want %v (%[2]T)", err, wantErr)
		return
	}

	// Force an error in MarshalResponse by giving it a result type that
	// can't be marshalled.
	_, err = dcrjson.MarshalResponse("1.0", 1, make(chan int), nil)
	if _, ok := err.(*json.UnsupportedTypeError); !ok {
		wantErr := &json.UnsupportedTypeError{}
		t.Errorf("MarshalResult: did not receive expected error - got "+
			"%v (%[1]T), want %T", err, wantErr)
		return
	}
}

// TestRPCError tests the error output for the RPCError type.
func TestRPCError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   *dcrjson.RPCError
		want string
	}{
		{
			dcrjson.ErrRPCInvalidRequest,
			"-32600: Invalid request",
		},
		{
			dcrjson.ErrRPCMethodNotFound,
			"-32601: Method not found",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("Error #%d\n got: %s want: %s", i, result,
				test.want)
			continue
		}
	}
}
