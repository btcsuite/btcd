package btcjson_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/dashevo/dashd-go/btcjson"
)

// TestDashMasternodeCmds tests all of the btcd extended commands marshal and unmarshal
// into valid results include handling of optional fields being omitted in the
// marshalled command, while optional fields with defaults have the default
// assigned on unmarshalled commands.
func TestDashMasternodeCmds(t *testing.T) {
	t.Parallel()

	testID := int(1)
	tests := []struct {
		name         string
		newCmd       func() (interface{}, error)
		staticCmd    func() interface{}
		marshalled   string
		unmarshalled interface{}
	}{
		{
			name: "masternode status",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("masternode", "status")
			},
			staticCmd: func() interface{} {
				return btcjson.NewMasternodeCmd(btcjson.MasternodeStatus)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"masternode","params":["status"],"id":1}`,
			unmarshalled: &btcjson.MasternodeCmd{SubCmd: btcjson.MasternodeStatus},
		},
		{
			name: "masternode count",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("masternode", "count")
			},
			staticCmd: func() interface{} {
				return btcjson.NewMasternodeCmd(btcjson.MasternodeCount)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"masternode","params":["count"],"id":1}`,
			unmarshalled: &btcjson.MasternodeCmd{SubCmd: btcjson.MasternodeCount},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Marshal the command as created by the new static command
			// creation function.
			marshalled, err := btcjson.MarshalCmd(testID, test.staticCmd())
			if err != nil {
				t.Fatalf("MarshalCmd unexpected error: %v", err)
			}

			if !bytes.Equal(marshalled, []byte(test.marshalled)) {
				t.Fatalf("Test unexpected marshalled data - "+
					"got %s, want %s", marshalled, test.marshalled)
			}

			// Ensure the command is created without error via the generic
			// new command creation function.
			cmd, err := test.newCmd()
			if err != nil {
				t.Fatalf("Test unexpected NewCmd error: %v ", err)
			}

			// Marshal the command as created by the generic new command
			// creation function.
			marshalled, err = btcjson.MarshalCmd(testID, cmd)
			if err != nil {
				t.Fatalf("MarshalCmd unexpected error: %v", err)
			}

			if !bytes.Equal(marshalled, []byte(test.marshalled)) {
				t.Fatalf("Unexpected marshalled data - "+
					"got %s, want %s", marshalled, test.marshalled)
			}

			var request btcjson.Request
			if err := json.Unmarshal(marshalled, &request); err != nil {
				t.Fatalf("Unexpected error while "+
					"unmarshalling JSON-RPC request: %v", err)
			}

			cmd, err = btcjson.UnmarshalCmd(&request)
			if err != nil {
				t.Fatalf("UnmarshalCmd unexpected error: %v", err)
			}

			if !reflect.DeepEqual(cmd, test.unmarshalled) {
				t.Fatalf("Unexpected unmarshalled command "+
					"- got %s, want %s",
					fmt.Sprintf("(%T) %+[1]v", cmd),
					fmt.Sprintf("(%T) %+[1]v\n", test.unmarshalled))
			}
		})
	}
}
