// Copyright (c) 2016-2017 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson_test

import (
	"encoding/json"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
)

// TestBtcdExtCustomResults ensures any results that have custom marshalling
// work as inteded.
// and unmarshal code of results are as expected.
func TestBtcdExtCustomResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		result   interface{}
		expected string
	}{
		{
			name: "versionresult",
			result: &btcjson.VersionResult{
				VersionString: "1.0.0",
				Major:         1,
				Minor:         0,
				Patch:         0,
				Prerelease:    "pr",
				BuildMetadata: "bm",
			},
			expected: `{"versionstring":"1.0.0","major":1,"minor":0,"patch":0,"prerelease":"pr","buildmetadata":"bm"}`,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		marshalled, err := json.Marshal(test.result)
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}
		if string(marshalled) != test.expected {
			t.Errorf("Test #%d (%s) unexpected marhsalled data - "+
				"got %s, want %s", i, test.name, marshalled,
				test.expected)
			continue
		}
	}
}
