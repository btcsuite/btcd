// Copyright (c) 2013-2020 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import "testing"

// TestCalcScriptInfoEmptyP2WSHWitness ensures an empty P2WSH witness returns an
// error instead of panicking while trying to access the witness script.
func TestCalcScriptInfoEmptyP2WSHWitness(t *testing.T) {
	t.Parallel()

	pkScript := mustParseShortForm(
		"0 DATA_32 0x00000000000000000000000000000000" +
			"00000000000000000000000000000000",
	)

	_, err := CalcScriptInfo(nil, pkScript, nil, false, true)
	if !IsErrorCode(err, ErrWitnessProgramEmpty) {
		t.Fatalf("unexpected error -- got %v, want %v", err,
			ErrWitnessProgramEmpty)
	}
}
