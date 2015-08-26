// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson_test

import (
	"testing"

	"github.com/decred/dcrd/dcrjson"
)

// TestErrorCodeStringer tests the stringized output for the ErrorCode type.
func TestErrorCodeStringer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   dcrjson.ErrorCode
		want string
	}{
		{dcrjson.ErrDuplicateMethod, "ErrDuplicateMethod"},
		{dcrjson.ErrInvalidUsageFlags, "ErrInvalidUsageFlags"},
		{dcrjson.ErrInvalidType, "ErrInvalidType"},
		{dcrjson.ErrEmbeddedType, "ErrEmbeddedType"},
		{dcrjson.ErrUnexportedField, "ErrUnexportedField"},
		{dcrjson.ErrUnsupportedFieldType, "ErrUnsupportedFieldType"},
		{dcrjson.ErrNonOptionalField, "ErrNonOptionalField"},
		{dcrjson.ErrNonOptionalDefault, "ErrNonOptionalDefault"},
		{dcrjson.ErrMismatchedDefault, "ErrMismatchedDefault"},
		{dcrjson.ErrUnregisteredMethod, "ErrUnregisteredMethod"},
		{dcrjson.ErrNumParams, "ErrNumParams"},
		{dcrjson.ErrMissingDescription, "ErrMissingDescription"},
		{0xffff, "Unknown ErrorCode (65535)"},
	}

	// Detect additional error codes that don't have the stringer added.
	if len(tests)-1 != int(dcrjson.TstNumErrorCodes) {
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
		in   dcrjson.Error
		want string
	}{
		{
			dcrjson.Error{Message: "some error"},
			"some error",
		},
		{
			dcrjson.Error{Message: "human-readable error"},
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
