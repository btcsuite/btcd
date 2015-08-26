// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/decred/dcrd/dcrjson"
)

// TestChainSvrWsNtfns tests all of the chain server websocket-specific
// notifications marshal and unmarshal into valid results include handling of
// optional fields being omitted in the marshalled command, while optional
// fields with defaults have the default assigned on unmarshalled commands.
func TestChainSvrWsNtfns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		newNtfn      func() (interface{}, error)
		staticNtfn   func() interface{}
		marshalled   string
		unmarshalled interface{}
	}{
		{
			name: "blockconnected",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("blockconnected", "123", 100000, 123456789, 0)
			},
			staticNtfn: func() interface{} {
				return dcrjson.NewBlockConnectedNtfn("123", 100000, 123456789, 0)
			},
			marshalled: `{"jsonrpc":"1.0","method":"blockconnected","params":["123",100000,123456789,0],"id":null}`,
			unmarshalled: &dcrjson.BlockConnectedNtfn{
				Hash:     "123",
				Height:   100000,
				Time:     123456789,
				VoteBits: 0,
			},
		},
		{
			name: "blockdisconnected",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("blockdisconnected", "123", 100000, 123456789, 0)
			},
			staticNtfn: func() interface{} {
				return dcrjson.NewBlockDisconnectedNtfn("123", 100000, 123456789, 0)
			},
			marshalled: `{"jsonrpc":"1.0","method":"blockdisconnected","params":["123",100000,123456789,0],"id":null}`,
			unmarshalled: &dcrjson.BlockDisconnectedNtfn{
				Hash:     "123",
				Height:   100000,
				Time:     123456789,
				VoteBits: 0,
			},
		},
		{
			name: "recvtx",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("recvtx", "001122", `{"height":100000,"tree":0,"hash":"123","index":0,"time":12345678,"votebits":0}`)
			},
			staticNtfn: func() interface{} {
				blockDetails := dcrjson.BlockDetails{
					Height:   100000,
					Tree:     0,
					Hash:     "123",
					Index:    0,
					Time:     12345678,
					VoteBits: 0,
				}
				return dcrjson.NewRecvTxNtfn("001122", &blockDetails)
			},
			marshalled: `{"jsonrpc":"1.0","method":"recvtx","params":["001122",{"height":100000,"tree":0,"hash":"123","index":0,"time":12345678,"votebits":0}],"id":null}`,
			unmarshalled: &dcrjson.RecvTxNtfn{
				HexTx: "001122",
				Block: &dcrjson.BlockDetails{
					Height:   100000,
					Tree:     0,
					Hash:     "123",
					Index:    0,
					Time:     12345678,
					VoteBits: 0,
				},
			},
		},
		{
			name: "redeemingtx",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("redeemingtx", "001122", `{"height":100000,"tree":0,"hash":"123","index":0,"time":12345678,"votebits":0}`)
			},
			staticNtfn: func() interface{} {
				blockDetails := dcrjson.BlockDetails{
					Height:   100000,
					Hash:     "123",
					Index:    0,
					Time:     12345678,
					VoteBits: 0,
				}
				return dcrjson.NewRedeemingTxNtfn("001122", &blockDetails)
			},
			marshalled: `{"jsonrpc":"1.0","method":"redeemingtx","params":["001122",{"height":100000,"tree":0,"hash":"123","index":0,"time":12345678,"votebits":0}],"id":null}`,
			unmarshalled: &dcrjson.RedeemingTxNtfn{
				HexTx: "001122",
				Block: &dcrjson.BlockDetails{
					Height:   100000,
					Hash:     "123",
					Index:    0,
					Time:     12345678,
					VoteBits: 0,
				},
			},
		},
		{
			name: "rescanfinished",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("rescanfinished", "123", 100000, 12345678)
			},
			staticNtfn: func() interface{} {
				return dcrjson.NewRescanFinishedNtfn("123", 100000, 12345678)
			},
			marshalled: `{"jsonrpc":"1.0","method":"rescanfinished","params":["123",100000,12345678],"id":null}`,
			unmarshalled: &dcrjson.RescanFinishedNtfn{
				Hash:   "123",
				Height: 100000,
				Time:   12345678,
			},
		},
		{
			name: "rescanprogress",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("rescanprogress", "123", 100000, 12345678)
			},
			staticNtfn: func() interface{} {
				return dcrjson.NewRescanProgressNtfn("123", 100000, 12345678)
			},
			marshalled: `{"jsonrpc":"1.0","method":"rescanprogress","params":["123",100000,12345678],"id":null}`,
			unmarshalled: &dcrjson.RescanProgressNtfn{
				Hash:   "123",
				Height: 100000,
				Time:   12345678,
			},
		},
		{
			name: "txaccepted",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("txaccepted", "123", 1.5)
			},
			staticNtfn: func() interface{} {
				return dcrjson.NewTxAcceptedNtfn("123", 1.5)
			},
			marshalled: `{"jsonrpc":"1.0","method":"txaccepted","params":["123",1.5],"id":null}`,
			unmarshalled: &dcrjson.TxAcceptedNtfn{
				TxID:   "123",
				Amount: 1.5,
			},
		},
		{
			name: "txacceptedverbose",
			newNtfn: func() (interface{}, error) {
				return dcrjson.NewCmd("txacceptedverbose", `{"hex":"001122","txid":"123","version":1,"locktime":4294967295,"vin":null,"vout":null,"confirmations":0}`)
			},
			staticNtfn: func() interface{} {
				txResult := dcrjson.TxRawResult{
					Hex:           "001122",
					Txid:          "123",
					Version:       1,
					LockTime:      4294967295,
					Vin:           nil,
					Vout:          nil,
					Confirmations: 0,
				}
				return dcrjson.NewTxAcceptedVerboseNtfn(txResult)
			},
			marshalled: `{"jsonrpc":"1.0","method":"txacceptedverbose","params":[{"hex":"001122","txid":"123","version":1,"locktime":4294967295,"expiry":0,"vin":null,"vout":null,"blockheight":0}],"id":null}`,
			unmarshalled: &dcrjson.TxAcceptedVerboseNtfn{
				RawTx: dcrjson.TxRawResult{
					Hex:           "001122",
					Txid:          "123",
					Version:       1,
					LockTime:      4294967295,
					Vin:           nil,
					Vout:          nil,
					Confirmations: 0,
				},
			},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Marshal the notification as created by the new static
		// creation function.  The ID is nil for notifications.
		marshalled, err := dcrjson.MarshalCmd(nil, test.staticNtfn())
		if err != nil {
			t.Errorf("MarshalCmd #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !bytes.Equal(marshalled, []byte(test.marshalled)) {
			t.Errorf("Test #%d (%s) unexpected marshalled data - "+
				"got %s, want %s", i, test.name, marshalled,
				test.marshalled)
			continue
		}

		// Ensure the notification is created without error via the
		// generic new notification creation function.
		cmd, err := test.newNtfn()
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected NewCmd error: %v ",
				i, test.name, err)
		}

		// Marshal the notification as created by the generic new
		// notification creation function.    The ID is nil for
		// notifications.
		marshalled, err = dcrjson.MarshalCmd(nil, cmd)
		if err != nil {
			t.Errorf("MarshalCmd #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !bytes.Equal(marshalled, []byte(test.marshalled)) {
			t.Errorf("Test #%d (%s) unexpected marshalled data - "+
				"got %s, want %s", i, test.name, marshalled,
				test.marshalled)
			continue
		}

		var request dcrjson.Request
		if err := json.Unmarshal(marshalled, &request); err != nil {
			t.Errorf("Test #%d (%s) unexpected error while "+
				"unmarshalling JSON-RPC request: %v", i,
				test.name, err)
			continue
		}

		cmd, err = dcrjson.UnmarshalCmd(&request)
		if err != nil {
			t.Errorf("UnmarshalCmd #%d (%s) unexpected error: %v", i,
				test.name, err)
			continue
		}

		if !reflect.DeepEqual(cmd, test.unmarshalled) {
			t.Errorf("Test #%d (%s) unexpected unmarshalled command "+
				"- got %s, want %s", i, test.name,
				fmt.Sprintf("(%T) %+[1]v", cmd),
				fmt.Sprintf("(%T) %+[1]v\n", test.unmarshalled))
			continue
		}
	}
}
