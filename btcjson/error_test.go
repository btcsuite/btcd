// Copyright (c) 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson_test

import (
	"testing"

	"github.com/btcsuite/btcd/btcjson"
)

// TestErrorCodeStringer tests the stringized output for the ErrorCode type.
func TestErrorCodeStringer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   btcjson.ErrorCode
		want string
	}{
		{btcjson.ErrDuplicateMethod, "ErrDuplicateMethod"},
		{btcjson.ErrInvalidUsageFlags, "ErrInvalidUsageFlags"},
		{btcjson.ErrInvalidType, "ErrInvalidType"},
		{btcjson.ErrEmbeddedType, "ErrEmbeddedType"},
		{btcjson.ErrUnexportedField, "ErrUnexportedField"},
		{btcjson.ErrUnsupportedFieldType, "ErrUnsupportedFieldType"},
		{btcjson.ErrNonOptionalField, "ErrNonOptionalField"},
		{btcjson.ErrNonOptionalDefault, "ErrNonOptionalDefault"},
		{btcjson.ErrMismatchedDefault, "ErrMismatchedDefault"},
		{btcjson.ErrUnregisteredMethod, "ErrUnregisteredMethod"},
		{btcjson.ErrNumParams, "ErrNumParams"},
		{btcjson.ErrMissingDescription, "ErrMissingDescription"},
		{0xffff, "Unknown ErrorCode (65535)"},
	}

	// Detect additional error codes that don't have the stringer added.
	if len(tests)-1 != int(btcjson.TstNumErrorCodes) {
		t.Errorf("It appears an error code was added without adding an " +
			"associated stringer test")
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.String()
		if result != test.want {
			t.Errorf("String #%d\n got: %s want: %s", i, result,
				test.want)
			continue
		}
	}
}

// TestError tests the error output for the Error type.
func TestError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   btcjson.Error
		want string
	}{
		{
			btcjson.Error{Description: "some error"},
			"some error",
		},
		{
			btcjson.Error{Description: "human-readable error"},
			"human-readable error",
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
