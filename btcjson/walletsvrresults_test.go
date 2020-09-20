// Copyright (c) 2020 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/txscript"
	"github.com/davecgh/go-spew/spew"
)

// TestGetAddressInfoResult ensures that custom unmarshalling of
// GetAddressInfoResult works as intended.
func TestGetAddressInfoResult(t *testing.T) {
	t.Parallel()

	// arbitrary script class to use in tests
	nonStandard, _ := txscript.NewScriptClass("nonstandard")

	tests := []struct {
		name    string
		result  string
		want    GetAddressInfoResult
		wantErr error
	}{
		{
			name:   "GetAddressInfoResult - no ScriptType",
			result: `{}`,
			want:   GetAddressInfoResult{},
		},
		{
			name:   "GetAddressInfoResult - ScriptType",
			result: `{"script":"nonstandard","address":"1abc"}`,
			want: GetAddressInfoResult{
				embeddedAddressInfo: embeddedAddressInfo{
					Address:    "1abc",
					ScriptType: nonStandard,
				},
			},
		},
		{
			name:   "GetAddressInfoResult - embedded ScriptType",
			result: `{"embedded": {"script":"nonstandard","address":"121313"}}`,
			want: GetAddressInfoResult{
				Embedded: &embeddedAddressInfo{
					Address:    "121313",
					ScriptType: nonStandard,
				},
			},
		},
		{
			name:    "GetAddressInfoResult - invalid ScriptType",
			result:  `{"embedded": {"script":"foo","address":"121313"}}`,
			wantErr: txscript.ErrUnsupportedScriptType,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		var out GetAddressInfoResult
		err := json.Unmarshal([]byte(test.result), &out)
		if err != nil && !errors.Is(err, test.wantErr) {
			t.Errorf("Test #%d (%s) unexpected error: %v, want: %v", i,
				test.name, err, test.wantErr)
			continue
		}

		if !reflect.DeepEqual(out, test.want) {
			t.Errorf("Test #%d (%s) unexpected unmarshalled data - "+
				"got %v, want %v", i, test.name, spew.Sdump(out),
				spew.Sdump(test.want))
			continue
		}
	}
}

// TestGetWalletInfoResult ensures that custom unmarshalling of
// GetWalletInfoResult works as intended.
func TestGetWalletInfoResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result string
		want   GetWalletInfoResult
	}{
		{
			name:   "GetWalletInfoResult - not scanning",
			result: `{"scanning":false}`,
			want: GetWalletInfoResult{
				Scanning: ScanningOrFalse{Value: false},
			},
		},
		{
			name:   "GetWalletInfoResult - scanning",
			result: `{"scanning":{"duration":10,"progress":1.0}}`,
			want: GetWalletInfoResult{
				Scanning: ScanningOrFalse{
					Value: ScanProgress{Duration: 10, Progress: 1.0},
				},
			},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		var out GetWalletInfoResult
		err := json.Unmarshal([]byte(test.result), &out)
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !reflect.DeepEqual(out, test.want) {
			t.Errorf("Test #%d (%s) unexpected unmarshalled data - "+
				"got %v, want %v", i, test.name, spew.Sdump(out),
				spew.Sdump(test.want))
			continue
		}
	}
}
