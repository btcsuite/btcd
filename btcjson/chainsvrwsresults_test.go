// Copyright (c) 2017 The btcsuite developers
// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson_test

import (
	"encoding/json"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
)

// TestChainSvrWsResults ensures any results that have custom marshalling
// work as inteded.
func TestChainSvrWsResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		result   interface{}
		expected string
	}{
		{
			name: "RescannedBlock",
			result: &btcjson.RescannedBlock{
				Hash:         "blockhash",
				Transactions: []string{"serializedtx"},
			},
			expected: `{"hash":"blockhash","transactions":["serializedtx"]}`,
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
