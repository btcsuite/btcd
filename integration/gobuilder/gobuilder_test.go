// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gobuilder

import (
	"os"
	"testing"

	"github.com/btcsuite/btcd/integration"
)

// TestGoBuider builds current project executable
func TestGoBuider(t *testing.T) {
	defer integration.VerifyNoAssetsLeaked()
	runExample()
}

func runExample() {
	testWorkingDir := integration.NewTempDir(os.TempDir(), "test-go-builder")

	testWorkingDir.MakeDir()
	defer testWorkingDir.Dispose()

	builder := &GoBuider{
		GoProjectPath:    DetermineProjectPackagePath("btcd"),
		OutputFolderPath: testWorkingDir.Path(),
		BuildFileName:    "btcd",
	}

	builder.Build()
	defer builder.Dispose()
}
