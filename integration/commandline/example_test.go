// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package commandline

import (
	"testing"

	"github.com/btcsuite/btcd/integration"
)

// TestGoExample launches example `go help` process
func TestGoExample(t *testing.T) {

	{
		proc := &ExternalProcess{
			CommandName: "go",
			WaitForExit: true,
		}
		proc.Arguments = append(proc.Arguments, "help")

		debugOutput := true
		proc.Launch(debugOutput)
	}

	// Verify proper disposal
	integration.VerifyNoAssetsLeaked()
}
