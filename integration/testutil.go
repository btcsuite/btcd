// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package integration

import (
	"fmt"
	"os"
)

var isInSelfDestructState = false

// ReportTestSetupMalfunction is used to bring
// attention to undesired program behaviour.
// This function is expected to be called never.
// The fact that it is called indicates a serious
// bug in the test setup and requires investigation.
func ReportTestSetupMalfunction(malfunction error) error {
	if malfunction == nil {
		return ReportTestSetupMalfunction(fmt.Errorf("no error provided"))
	}

	fmt.Fprintln(os.Stderr, malfunction.Error())

	if isInSelfDestructState {
		return malfunction
	}
	isInSelfDestructState = true
	selfDestruct()

	panic(fmt.Sprintf("Test setup malfunction: %v", malfunction))
}

// selfDestruct performs routines to dispose test setup framework in
// case of malfunction
func selfDestruct() {
	forceDisposeLeakyAssets()
}

// CheckTestSetupMalfunction reports error when one is present
func CheckTestSetupMalfunction(err error) {
	if err != nil {
		ReportTestSetupMalfunction(err)
	}
}
