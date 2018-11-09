// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package simpleregtest

import (
	"flag"
	"os"
	"testing"

	"github.com/btcsuite/btcd/integration"
	"github.com/btcsuite/btcd/integration/harness"
)

// ObtainHarness manages access to the Pool for test cases
func ObtainHarness(tag string) *harness.Harness {
	s := testSetup.harnessPool.ObtainSpawnableConcurrentSafe(tag)
	return s.(*harness.Harness)
}

var testSetup *SimpleTestSetup

// TestMain is executed by go-test, and is
// responsible for setting up and disposing test environment.
func TestMain(m *testing.M) {
	flag.Parse()

	testSetup = Setup()

	if !testing.Short() {
		// Initialize harnesses before running any tests
		// otherwise they will be created on request.
		tagsList := []string{
			mainHarnessName,
		}
		testSetup.harnessPool.InitTags(tagsList)
	}

	// Run tests
	exitCode := m.Run()

	testSetup.TearDown()

	integration.VerifyNoAssetsLeaked()

	os.Exit(exitCode)
}
