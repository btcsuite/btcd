// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"reflect"
)

/*
 Assertion checks help the test setup to detect its own defects.
*/

// AssertNotNil checks and reports if given variable is nil
func AssertNotNil(tag string, value interface{}) {
	if value == nil || (reflect.ValueOf(value).Kind() == reflect.Ptr && reflect.ValueOf(value).IsNil()) {
		ReportTestSetupMalfunction(
			fmt.Errorf("invalid state: <%v> is nil", tag))
	}
}

// AssertNotEmpty checks and reports if given string is empty
func AssertNotEmpty(tag string, value string) {
	if value == "" {
		ReportTestSetupMalfunction(
			fmt.Errorf("invalid state: string <%v> is empty", tag))
	}
}

// AssertTrue checks and reports if given variable is false
func AssertTrue(tag string, value bool) {
	if !value {
		ReportTestSetupMalfunction(
			fmt.Errorf("invalid state: string <%v> is %v", tag, value))
	}
}
