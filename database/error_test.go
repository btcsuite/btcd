// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database_test

import (
	"errors"
	"testing"

	"github.com/decred/dcrd/database"
)

// TestErrorCodeStringer tests the stringized output for the ErrorCode type.
func TestErrorCodeStringer(t *testing.T) {
	tests := []struct {
		in   database.ErrorCode
		want string
	}{
		{database.ErrDbTypeRegistered, "ErrDbTypeRegistered"},
		{database.ErrDbUnknownType, "ErrDbUnknownType"},
		{database.ErrDbDoesNotExist, "ErrDbDoesNotExist"},
		{database.ErrDbExists, "ErrDbExists"},
		{database.ErrDbNotOpen, "ErrDbNotOpen"},
		{database.ErrDbAlreadyOpen, "ErrDbAlreadyOpen"},
		{database.ErrInvalid, "ErrInvalid"},
		{database.ErrCorruption, "ErrCorruption"},
		{database.ErrTxClosed, "ErrTxClosed"},
		{database.ErrTxNotWritable, "ErrTxNotWritable"},
		{database.ErrBucketNotFound, "ErrBucketNotFound"},
		{database.ErrBucketExists, "ErrBucketExists"},
		{database.ErrBucketNameRequired, "ErrBucketNameRequired"},
		{database.ErrKeyRequired, "ErrKeyRequired"},
		{database.ErrKeyTooLarge, "ErrKeyTooLarge"},
		{database.ErrValueTooLarge, "ErrValueTooLarge"},
		{database.ErrIncompatibleValue, "ErrIncompatibleValue"},
		{database.ErrBlockNotFound, "ErrBlockNotFound"},
		{database.ErrBlockExists, "ErrBlockExists"},
		{database.ErrBlockRegionInvalid, "ErrBlockRegionInvalid"},
		{database.ErrDriverSpecific, "ErrDriverSpecific"},

		{0xffff, "Unknown ErrorCode (65535)"},
	}

	// Detect additional error codes that don't have the stringer added.
	if len(tests)-1 != int(database.TstNumErrorCodes) {
		t.Errorf("It appears an error code was added without adding " +
			"an associated stringer test")
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.String()
		if result != test.want {
			t.Errorf("String #%d\ngot: %s\nwant: %s", i, result,
				test.want)
			continue
		}
	}
}

// TestError tests the error output for the Error type.
func TestError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   database.Error
		want string
	}{
		{
			database.Error{Description: "some error"},
			"some error",
		},
		{
			database.Error{Description: "human-readable error"},
			"human-readable error",
		},
		{
			database.Error{
				ErrorCode:   database.ErrDriverSpecific,
				Description: "some error",
				Err:         errors.New("driver-specific error"),
			},
			"some error: driver-specific error",
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
