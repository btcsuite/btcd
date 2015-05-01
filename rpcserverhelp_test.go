// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import "testing"

// TestHelp ensures the help is reasonably accurate by checking that every
// command specified also has result types defined and the one-line usage and
// help text can be generated for them.
func TestHelp(t *testing.T) {
	// Ensure there are result types specified for every handler.
	for k := range rpcHandlers {
		if _, ok := rpcResultTypes[k]; !ok {
			t.Errorf("RPC handler defined for method '%v' without "+
				"also specifying result types", k)
			continue
		}

	}
	for k := range wsHandlers {
		if _, ok := rpcResultTypes[k]; !ok {
			t.Errorf("RPC handler defined for method '%v' without "+
				"also specifying result types", k)
			continue
		}

	}

	// Ensure the usage for every command can be generated without errors.
	helpCacher := newHelpCacher()
	if _, err := helpCacher.rpcUsage(true); err != nil {
		t.Fatalf("Failed to generate one-line usage: %v", err)
	}
	if _, err := helpCacher.rpcUsage(true); err != nil {
		t.Fatalf("Failed to generate one-line usage (cached): %v", err)
	}

	// Ensure the help for every command can be generated without errors.
	for k := range rpcHandlers {
		if _, err := helpCacher.rpcMethodHelp(k); err != nil {
			t.Errorf("Failed to generate help for method '%v': %v",
				k, err)
			continue
		}
		if _, err := helpCacher.rpcMethodHelp(k); err != nil {
			t.Errorf("Failed to generate help for method '%v'"+
				"(cached): %v", k, err)
			continue
		}
	}
	for k := range wsHandlers {
		if _, err := helpCacher.rpcMethodHelp(k); err != nil {
			t.Errorf("Failed to generate help for method '%v': %v",
				k, err)
			continue
		}
		if _, err := helpCacher.rpcMethodHelp(k); err != nil {
			t.Errorf("Failed to generate help for method '%v'"+
				"(cached): %v", k, err)
			continue
		}
	}
}
