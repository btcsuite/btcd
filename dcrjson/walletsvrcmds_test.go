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

// TestWalletSvrCmds tests all of the wallet server commands marshal and
// unmarshal into valid results include handling of optional fields being
// omitted in the marshalled command, while optional fields with defaults have
// the default assigned on unmarshalled commands.
func TestWalletSvrCmds(t *testing.T) {
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
			name: "addmultisigaddress",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("addmultisigaddress", 2, []string{"031234", "035678"})
			},
			staticCmd: func() interface{} {
				keys := []string{"031234", "035678"}
				return dcrjson.NewAddMultisigAddressCmd(2, keys, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"addmultisigaddress","params":[2,["031234","035678"]],"id":1}`,
			unmarshalled: &dcrjson.AddMultisigAddressCmd{
				NRequired: 2,
				Keys:      []string{"031234", "035678"},
				Account:   nil,
			},
		},
		{
			name: "addmultisigaddress optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("addmultisigaddress", 2, []string{"031234", "035678"}, "test")
			},
			staticCmd: func() interface{} {
				keys := []string{"031234", "035678"}
				return dcrjson.NewAddMultisigAddressCmd(2, keys, dcrjson.String("test"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"addmultisigaddress","params":[2,["031234","035678"],"test"],"id":1}`,
			unmarshalled: &dcrjson.AddMultisigAddressCmd{
				NRequired: 2,
				Keys:      []string{"031234", "035678"},
				Account:   dcrjson.String("test"),
			},
		},
		{
			name: "createmultisig",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("createmultisig", 2, []string{"031234", "035678"})
			},
			staticCmd: func() interface{} {
				keys := []string{"031234", "035678"}
				return dcrjson.NewCreateMultisigCmd(2, keys)
			},
			marshalled: `{"jsonrpc":"1.0","method":"createmultisig","params":[2,["031234","035678"]],"id":1}`,
			unmarshalled: &dcrjson.CreateMultisigCmd{
				NRequired: 2,
				Keys:      []string{"031234", "035678"},
			},
		},
		{
			name: "dumpprivkey",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("dumpprivkey", "1Address")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewDumpPrivKeyCmd("1Address")
			},
			marshalled: `{"jsonrpc":"1.0","method":"dumpprivkey","params":["1Address"],"id":1}`,
			unmarshalled: &dcrjson.DumpPrivKeyCmd{
				Address: "1Address",
			},
		},
		{
			name: "estimatefee",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("estimatefee", 6)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewEstimateFeeCmd(6)
			},
			marshalled: `{"jsonrpc":"1.0","method":"estimatefee","params":[6],"id":1}`,
			unmarshalled: &dcrjson.EstimateFeeCmd{
				NumBlocks: 6,
			},
		},
		{
			name: "estimatepriority",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("estimatepriority", 6)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewEstimatePriorityCmd(6)
			},
			marshalled: `{"jsonrpc":"1.0","method":"estimatepriority","params":[6],"id":1}`,
			unmarshalled: &dcrjson.EstimatePriorityCmd{
				NumBlocks: 6,
			},
		},
		{
			name: "getaccount",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getaccount", "1Address")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetAccountCmd("1Address")
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaccount","params":["1Address"],"id":1}`,
			unmarshalled: &dcrjson.GetAccountCmd{
				Address: "1Address",
			},
		},
		{
			name: "getaccountaddress",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getaccountaddress", "acct")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetAccountAddressCmd("acct")
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaccountaddress","params":["acct"],"id":1}`,
			unmarshalled: &dcrjson.GetAccountAddressCmd{
				Account: "acct",
			},
		},
		{
			name: "getaddressesbyaccount",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getaddressesbyaccount", "acct")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetAddressesByAccountCmd("acct")
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaddressesbyaccount","params":["acct"],"id":1}`,
			unmarshalled: &dcrjson.GetAddressesByAccountCmd{
				Account: "acct",
			},
		},
		{
			name: "getbalance",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getbalance")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetBalanceCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getbalance","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetBalanceCmd{
				Account: nil,
				MinConf: dcrjson.Int(1),
			},
		},
		{
			name: "getbalance optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getbalance", "acct")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetBalanceCmd(dcrjson.String("acct"), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getbalance","params":["acct"],"id":1}`,
			unmarshalled: &dcrjson.GetBalanceCmd{
				Account: dcrjson.String("acct"),
				MinConf: dcrjson.Int(1),
			},
		},
		{
			name: "getbalance optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getbalance", "acct", 6)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetBalanceCmd(dcrjson.String("acct"), dcrjson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getbalance","params":["acct",6],"id":1}`,
			unmarshalled: &dcrjson.GetBalanceCmd{
				Account: dcrjson.String("acct"),
				MinConf: dcrjson.Int(6),
			},
		},
		{
			name: "getnewaddress",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getnewaddress")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetNewAddressCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnewaddress","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetNewAddressCmd{
				Account:   nil,
				GapPolicy: nil,
			},
		},
		{
			name: "getnewaddress optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getnewaddress", "acct", "ignore")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetNewAddressCmd(dcrjson.String("acct"), dcrjson.String("ignore"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnewaddress","params":["acct","ignore"],"id":1}`,
			unmarshalled: &dcrjson.GetNewAddressCmd{
				Account:   dcrjson.String("acct"),
				GapPolicy: dcrjson.String("ignore"),
			},
		},
		{
			name: "getrawchangeaddress",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getrawchangeaddress")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetRawChangeAddressCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawchangeaddress","params":[],"id":1}`,
			unmarshalled: &dcrjson.GetRawChangeAddressCmd{
				Account: nil,
			},
		},
		{
			name: "getrawchangeaddress optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getrawchangeaddress", "acct")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetRawChangeAddressCmd(dcrjson.String("acct"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawchangeaddress","params":["acct"],"id":1}`,
			unmarshalled: &dcrjson.GetRawChangeAddressCmd{
				Account: dcrjson.String("acct"),
			},
		},
		{
			name: "getreceivedbyaccount",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getreceivedbyaccount", "acct")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetReceivedByAccountCmd("acct", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getreceivedbyaccount","params":["acct"],"id":1}`,
			unmarshalled: &dcrjson.GetReceivedByAccountCmd{
				Account: "acct",
				MinConf: dcrjson.Int(1),
			},
		},
		{
			name: "getreceivedbyaccount optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getreceivedbyaccount", "acct", 6)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetReceivedByAccountCmd("acct", dcrjson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getreceivedbyaccount","params":["acct",6],"id":1}`,
			unmarshalled: &dcrjson.GetReceivedByAccountCmd{
				Account: "acct",
				MinConf: dcrjson.Int(6),
			},
		},
		{
			name: "getreceivedbyaddress",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getreceivedbyaddress", "1Address")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetReceivedByAddressCmd("1Address", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getreceivedbyaddress","params":["1Address"],"id":1}`,
			unmarshalled: &dcrjson.GetReceivedByAddressCmd{
				Address: "1Address",
				MinConf: dcrjson.Int(1),
			},
		},
		{
			name: "getreceivedbyaddress optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("getreceivedbyaddress", "1Address", 6)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetReceivedByAddressCmd("1Address", dcrjson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getreceivedbyaddress","params":["1Address",6],"id":1}`,
			unmarshalled: &dcrjson.GetReceivedByAddressCmd{
				Address: "1Address",
				MinConf: dcrjson.Int(6),
			},
		},
		{
			name: "gettransaction",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("gettransaction", "123")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetTransactionCmd("123", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettransaction","params":["123"],"id":1}`,
			unmarshalled: &dcrjson.GetTransactionCmd{
				Txid:             "123",
				IncludeWatchOnly: dcrjson.Bool(false),
			},
		},
		{
			name: "gettransaction optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("gettransaction", "123", true)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewGetTransactionCmd("123", dcrjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettransaction","params":["123",true],"id":1}`,
			unmarshalled: &dcrjson.GetTransactionCmd{
				Txid:             "123",
				IncludeWatchOnly: dcrjson.Bool(true),
			},
		},
		{
			name: "importprivkey",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("importprivkey", "abc")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewImportPrivKeyCmd("abc", nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importprivkey","params":["abc"],"id":1}`,
			unmarshalled: &dcrjson.ImportPrivKeyCmd{
				PrivKey: "abc",
				Label:   nil,
				Rescan:  dcrjson.Bool(true),
			},
		},
		{
			name: "importprivkey optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("importprivkey", "abc", "label")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewImportPrivKeyCmd("abc", dcrjson.String("label"), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importprivkey","params":["abc","label"],"id":1}`,
			unmarshalled: &dcrjson.ImportPrivKeyCmd{
				PrivKey: "abc",
				Label:   dcrjson.String("label"),
				Rescan:  dcrjson.Bool(true),
			},
		},
		{
			name: "importprivkey optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("importprivkey", "abc", "label", false)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewImportPrivKeyCmd("abc", dcrjson.String("label"), dcrjson.Bool(false), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importprivkey","params":["abc","label",false],"id":1}`,
			unmarshalled: &dcrjson.ImportPrivKeyCmd{
				PrivKey: "abc",
				Label:   dcrjson.String("label"),
				Rescan:  dcrjson.Bool(false),
			},
		},
		{
			name: "importprivkey optional3",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("importprivkey", "abc", "label", false, 12345)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewImportPrivKeyCmd("abc", dcrjson.String("label"), dcrjson.Bool(false), dcrjson.Int(12345))
			},
			marshalled: `{"jsonrpc":"1.0","method":"importprivkey","params":["abc","label",false,12345],"id":1}`,
			unmarshalled: &dcrjson.ImportPrivKeyCmd{
				PrivKey:  "abc",
				Label:    dcrjson.String("label"),
				Rescan:   dcrjson.Bool(false),
				ScanFrom: dcrjson.Int(12345),
			},
		},
		{
			name: "keypoolrefill",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("keypoolrefill")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewKeyPoolRefillCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"keypoolrefill","params":[],"id":1}`,
			unmarshalled: &dcrjson.KeyPoolRefillCmd{
				NewSize: dcrjson.Uint(100),
			},
		},
		{
			name: "keypoolrefill optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("keypoolrefill", 200)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewKeyPoolRefillCmd(dcrjson.Uint(200))
			},
			marshalled: `{"jsonrpc":"1.0","method":"keypoolrefill","params":[200],"id":1}`,
			unmarshalled: &dcrjson.KeyPoolRefillCmd{
				NewSize: dcrjson.Uint(200),
			},
		},
		{
			name: "listaccounts",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listaccounts")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListAccountsCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listaccounts","params":[],"id":1}`,
			unmarshalled: &dcrjson.ListAccountsCmd{
				MinConf: dcrjson.Int(1),
			},
		},
		{
			name: "listaccounts optional",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listaccounts", 6)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListAccountsCmd(dcrjson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listaccounts","params":[6],"id":1}`,
			unmarshalled: &dcrjson.ListAccountsCmd{
				MinConf: dcrjson.Int(6),
			},
		},
		{
			name: "listlockunspent",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listlockunspent")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListLockUnspentCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"listlockunspent","params":[],"id":1}`,
			unmarshalled: &dcrjson.ListLockUnspentCmd{},
		},
		{
			name: "listreceivedbyaccount",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listreceivedbyaccount")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListReceivedByAccountCmd(nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaccount","params":[],"id":1}`,
			unmarshalled: &dcrjson.ListReceivedByAccountCmd{
				MinConf:          dcrjson.Int(1),
				IncludeEmpty:     dcrjson.Bool(false),
				IncludeWatchOnly: dcrjson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaccount optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listreceivedbyaccount", 6)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListReceivedByAccountCmd(dcrjson.Int(6), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaccount","params":[6],"id":1}`,
			unmarshalled: &dcrjson.ListReceivedByAccountCmd{
				MinConf:          dcrjson.Int(6),
				IncludeEmpty:     dcrjson.Bool(false),
				IncludeWatchOnly: dcrjson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaccount optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listreceivedbyaccount", 6, true)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListReceivedByAccountCmd(dcrjson.Int(6), dcrjson.Bool(true), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaccount","params":[6,true],"id":1}`,
			unmarshalled: &dcrjson.ListReceivedByAccountCmd{
				MinConf:          dcrjson.Int(6),
				IncludeEmpty:     dcrjson.Bool(true),
				IncludeWatchOnly: dcrjson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaccount optional3",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listreceivedbyaccount", 6, true, false)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListReceivedByAccountCmd(dcrjson.Int(6), dcrjson.Bool(true), dcrjson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaccount","params":[6,true,false],"id":1}`,
			unmarshalled: &dcrjson.ListReceivedByAccountCmd{
				MinConf:          dcrjson.Int(6),
				IncludeEmpty:     dcrjson.Bool(true),
				IncludeWatchOnly: dcrjson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaddress",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listreceivedbyaddress")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListReceivedByAddressCmd(nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaddress","params":[],"id":1}`,
			unmarshalled: &dcrjson.ListReceivedByAddressCmd{
				MinConf:          dcrjson.Int(1),
				IncludeEmpty:     dcrjson.Bool(false),
				IncludeWatchOnly: dcrjson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaddress optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listreceivedbyaddress", 6)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListReceivedByAddressCmd(dcrjson.Int(6), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaddress","params":[6],"id":1}`,
			unmarshalled: &dcrjson.ListReceivedByAddressCmd{
				MinConf:          dcrjson.Int(6),
				IncludeEmpty:     dcrjson.Bool(false),
				IncludeWatchOnly: dcrjson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaddress optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listreceivedbyaddress", 6, true)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListReceivedByAddressCmd(dcrjson.Int(6), dcrjson.Bool(true), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaddress","params":[6,true],"id":1}`,
			unmarshalled: &dcrjson.ListReceivedByAddressCmd{
				MinConf:          dcrjson.Int(6),
				IncludeEmpty:     dcrjson.Bool(true),
				IncludeWatchOnly: dcrjson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaddress optional3",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listreceivedbyaddress", 6, true, false)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListReceivedByAddressCmd(dcrjson.Int(6), dcrjson.Bool(true), dcrjson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaddress","params":[6,true,false],"id":1}`,
			unmarshalled: &dcrjson.ListReceivedByAddressCmd{
				MinConf:          dcrjson.Int(6),
				IncludeEmpty:     dcrjson.Bool(true),
				IncludeWatchOnly: dcrjson.Bool(false),
			},
		},
		{
			name: "listsinceblock",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listsinceblock")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListSinceBlockCmd(nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listsinceblock","params":[],"id":1}`,
			unmarshalled: &dcrjson.ListSinceBlockCmd{
				BlockHash:           nil,
				TargetConfirmations: dcrjson.Int(1),
				IncludeWatchOnly:    dcrjson.Bool(false),
			},
		},
		{
			name: "listsinceblock optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listsinceblock", "123")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListSinceBlockCmd(dcrjson.String("123"), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listsinceblock","params":["123"],"id":1}`,
			unmarshalled: &dcrjson.ListSinceBlockCmd{
				BlockHash:           dcrjson.String("123"),
				TargetConfirmations: dcrjson.Int(1),
				IncludeWatchOnly:    dcrjson.Bool(false),
			},
		},
		{
			name: "listsinceblock optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listsinceblock", "123", 6)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListSinceBlockCmd(dcrjson.String("123"), dcrjson.Int(6), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listsinceblock","params":["123",6],"id":1}`,
			unmarshalled: &dcrjson.ListSinceBlockCmd{
				BlockHash:           dcrjson.String("123"),
				TargetConfirmations: dcrjson.Int(6),
				IncludeWatchOnly:    dcrjson.Bool(false),
			},
		},
		{
			name: "listsinceblock optional3",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listsinceblock", "123", 6, true)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListSinceBlockCmd(dcrjson.String("123"), dcrjson.Int(6), dcrjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listsinceblock","params":["123",6,true],"id":1}`,
			unmarshalled: &dcrjson.ListSinceBlockCmd{
				BlockHash:           dcrjson.String("123"),
				TargetConfirmations: dcrjson.Int(6),
				IncludeWatchOnly:    dcrjson.Bool(true),
			},
		},
		{
			name: "listtransactions",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listtransactions")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListTransactionsCmd(nil, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":[],"id":1}`,
			unmarshalled: &dcrjson.ListTransactionsCmd{
				Account:          nil,
				Count:            dcrjson.Int(10),
				From:             dcrjson.Int(0),
				IncludeWatchOnly: dcrjson.Bool(false),
			},
		},
		{
			name: "listtransactions optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listtransactions", "acct")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListTransactionsCmd(dcrjson.String("acct"), nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":["acct"],"id":1}`,
			unmarshalled: &dcrjson.ListTransactionsCmd{
				Account:          dcrjson.String("acct"),
				Count:            dcrjson.Int(10),
				From:             dcrjson.Int(0),
				IncludeWatchOnly: dcrjson.Bool(false),
			},
		},
		{
			name: "listtransactions optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listtransactions", "acct", 20)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListTransactionsCmd(dcrjson.String("acct"), dcrjson.Int(20), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":["acct",20],"id":1}`,
			unmarshalled: &dcrjson.ListTransactionsCmd{
				Account:          dcrjson.String("acct"),
				Count:            dcrjson.Int(20),
				From:             dcrjson.Int(0),
				IncludeWatchOnly: dcrjson.Bool(false),
			},
		},
		{
			name: "listtransactions optional3",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listtransactions", "acct", 20, 1)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListTransactionsCmd(dcrjson.String("acct"), dcrjson.Int(20),
					dcrjson.Int(1), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":["acct",20,1],"id":1}`,
			unmarshalled: &dcrjson.ListTransactionsCmd{
				Account:          dcrjson.String("acct"),
				Count:            dcrjson.Int(20),
				From:             dcrjson.Int(1),
				IncludeWatchOnly: dcrjson.Bool(false),
			},
		},
		{
			name: "listtransactions optional4",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listtransactions", "acct", 20, 1, true)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListTransactionsCmd(dcrjson.String("acct"), dcrjson.Int(20),
					dcrjson.Int(1), dcrjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":["acct",20,1,true],"id":1}`,
			unmarshalled: &dcrjson.ListTransactionsCmd{
				Account:          dcrjson.String("acct"),
				Count:            dcrjson.Int(20),
				From:             dcrjson.Int(1),
				IncludeWatchOnly: dcrjson.Bool(true),
			},
		},
		{
			name: "listunspent",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listunspent")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListUnspentCmd(nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listunspent","params":[],"id":1}`,
			unmarshalled: &dcrjson.ListUnspentCmd{
				MinConf:   dcrjson.Int(1),
				MaxConf:   dcrjson.Int(9999999),
				Addresses: nil,
			},
		},
		{
			name: "listunspent optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listunspent", 6)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListUnspentCmd(dcrjson.Int(6), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listunspent","params":[6],"id":1}`,
			unmarshalled: &dcrjson.ListUnspentCmd{
				MinConf:   dcrjson.Int(6),
				MaxConf:   dcrjson.Int(9999999),
				Addresses: nil,
			},
		},
		{
			name: "listunspent optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listunspent", 6, 100)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListUnspentCmd(dcrjson.Int(6), dcrjson.Int(100), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listunspent","params":[6,100],"id":1}`,
			unmarshalled: &dcrjson.ListUnspentCmd{
				MinConf:   dcrjson.Int(6),
				MaxConf:   dcrjson.Int(100),
				Addresses: nil,
			},
		},
		{
			name: "listunspent optional3",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("listunspent", 6, 100, []string{"1Address", "1Address2"})
			},
			staticCmd: func() interface{} {
				return dcrjson.NewListUnspentCmd(dcrjson.Int(6), dcrjson.Int(100),
					&[]string{"1Address", "1Address2"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"listunspent","params":[6,100,["1Address","1Address2"]],"id":1}`,
			unmarshalled: &dcrjson.ListUnspentCmd{
				MinConf:   dcrjson.Int(6),
				MaxConf:   dcrjson.Int(100),
				Addresses: &[]string{"1Address", "1Address2"},
			},
		},
		{
			name: "lockunspent",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("lockunspent", true, `[{"txid":"123","vout":1}]`)
			},
			staticCmd: func() interface{} {
				txInputs := []dcrjson.TransactionInput{
					{Txid: "123", Vout: 1},
				}
				return dcrjson.NewLockUnspentCmd(true, txInputs)
			},
			marshalled: `{"jsonrpc":"1.0","method":"lockunspent","params":[true,[{"txid":"123","vout":1,"tree":0}]],"id":1}`,
			unmarshalled: &dcrjson.LockUnspentCmd{
				Unlock: true,
				Transactions: []dcrjson.TransactionInput{
					{Txid: "123", Vout: 1},
				},
			},
		},
		{
			name: "sendfrom",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("sendfrom", "from", "1Address", 0.5)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSendFromCmd("from", "1Address", 0.5, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendfrom","params":["from","1Address",0.5],"id":1}`,
			unmarshalled: &dcrjson.SendFromCmd{
				FromAccount: "from",
				ToAddress:   "1Address",
				Amount:      0.5,
				MinConf:     dcrjson.Int(1),
				Comment:     nil,
				CommentTo:   nil,
			},
		},
		{
			name: "sendfrom optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("sendfrom", "from", "1Address", 0.5, 6)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSendFromCmd("from", "1Address", 0.5, dcrjson.Int(6), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendfrom","params":["from","1Address",0.5,6],"id":1}`,
			unmarshalled: &dcrjson.SendFromCmd{
				FromAccount: "from",
				ToAddress:   "1Address",
				Amount:      0.5,
				MinConf:     dcrjson.Int(6),
				Comment:     nil,
				CommentTo:   nil,
			},
		},
		{
			name: "sendfrom optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("sendfrom", "from", "1Address", 0.5, 6, "comment")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSendFromCmd("from", "1Address", 0.5, dcrjson.Int(6),
					dcrjson.String("comment"), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendfrom","params":["from","1Address",0.5,6,"comment"],"id":1}`,
			unmarshalled: &dcrjson.SendFromCmd{
				FromAccount: "from",
				ToAddress:   "1Address",
				Amount:      0.5,
				MinConf:     dcrjson.Int(6),
				Comment:     dcrjson.String("comment"),
				CommentTo:   nil,
			},
		},
		{
			name: "sendfrom optional3",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("sendfrom", "from", "1Address", 0.5, 6, "comment", "commentto")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSendFromCmd("from", "1Address", 0.5, dcrjson.Int(6),
					dcrjson.String("comment"), dcrjson.String("commentto"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendfrom","params":["from","1Address",0.5,6,"comment","commentto"],"id":1}`,
			unmarshalled: &dcrjson.SendFromCmd{
				FromAccount: "from",
				ToAddress:   "1Address",
				Amount:      0.5,
				MinConf:     dcrjson.Int(6),
				Comment:     dcrjson.String("comment"),
				CommentTo:   dcrjson.String("commentto"),
			},
		},
		{
			name: "sendmany",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("sendmany", "from", `{"1Address":0.5}`)
			},
			staticCmd: func() interface{} {
				amounts := map[string]float64{"1Address": 0.5}
				return dcrjson.NewSendManyCmd("from", amounts, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendmany","params":["from",{"1Address":0.5}],"id":1}`,
			unmarshalled: &dcrjson.SendManyCmd{
				FromAccount: "from",
				Amounts:     map[string]float64{"1Address": 0.5},
				MinConf:     dcrjson.Int(1),
				Comment:     nil,
			},
		},
		{
			name: "sendmany optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("sendmany", "from", `{"1Address":0.5}`, 6)
			},
			staticCmd: func() interface{} {
				amounts := map[string]float64{"1Address": 0.5}
				return dcrjson.NewSendManyCmd("from", amounts, dcrjson.Int(6), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendmany","params":["from",{"1Address":0.5},6],"id":1}`,
			unmarshalled: &dcrjson.SendManyCmd{
				FromAccount: "from",
				Amounts:     map[string]float64{"1Address": 0.5},
				MinConf:     dcrjson.Int(6),
				Comment:     nil,
			},
		},
		{
			name: "sendmany optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("sendmany", "from", `{"1Address":0.5}`, 6, "comment")
			},
			staticCmd: func() interface{} {
				amounts := map[string]float64{"1Address": 0.5}
				return dcrjson.NewSendManyCmd("from", amounts, dcrjson.Int(6), dcrjson.String("comment"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendmany","params":["from",{"1Address":0.5},6,"comment"],"id":1}`,
			unmarshalled: &dcrjson.SendManyCmd{
				FromAccount: "from",
				Amounts:     map[string]float64{"1Address": 0.5},
				MinConf:     dcrjson.Int(6),
				Comment:     dcrjson.String("comment"),
			},
		},
		{
			name: "sendtoaddress",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("sendtoaddress", "1Address", 0.5)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSendToAddressCmd("1Address", 0.5, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendtoaddress","params":["1Address",0.5],"id":1}`,
			unmarshalled: &dcrjson.SendToAddressCmd{
				Address:   "1Address",
				Amount:    0.5,
				Comment:   nil,
				CommentTo: nil,
			},
		},
		{
			name: "sendtoaddress optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("sendtoaddress", "1Address", 0.5, "comment", "commentto")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSendToAddressCmd("1Address", 0.5, dcrjson.String("comment"),
					dcrjson.String("commentto"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendtoaddress","params":["1Address",0.5,"comment","commentto"],"id":1}`,
			unmarshalled: &dcrjson.SendToAddressCmd{
				Address:   "1Address",
				Amount:    0.5,
				Comment:   dcrjson.String("comment"),
				CommentTo: dcrjson.String("commentto"),
			},
		},
		{
			name: "settxfee",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("settxfee", 0.0001)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSetTxFeeCmd(0.0001)
			},
			marshalled: `{"jsonrpc":"1.0","method":"settxfee","params":[0.0001],"id":1}`,
			unmarshalled: &dcrjson.SetTxFeeCmd{
				Amount: 0.0001,
			},
		},
		{
			name: "signmessage",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("signmessage", "1Address", "message")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSignMessageCmd("1Address", "message")
			},
			marshalled: `{"jsonrpc":"1.0","method":"signmessage","params":["1Address","message"],"id":1}`,
			unmarshalled: &dcrjson.SignMessageCmd{
				Address: "1Address",
				Message: "message",
			},
		},
		{
			name: "signrawtransaction",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("signrawtransaction", "001122")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewSignRawTransactionCmd("001122", nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransaction","params":["001122"],"id":1}`,
			unmarshalled: &dcrjson.SignRawTransactionCmd{
				RawTx:    "001122",
				Inputs:   nil,
				PrivKeys: nil,
				Flags:    dcrjson.String("ALL"),
			},
		},
		{
			name: "signrawtransaction optional1",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("signrawtransaction", "001122", `[{"txid":"123","vout":1,"tree":0,"scriptPubKey":"00","redeemScript":"01"}]`)
			},
			staticCmd: func() interface{} {
				txInputs := []dcrjson.RawTxInput{
					{
						Txid:         "123",
						Vout:         1,
						ScriptPubKey: "00",
						RedeemScript: "01",
					},
				}

				return dcrjson.NewSignRawTransactionCmd("001122", &txInputs, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransaction","params":["001122",[{"txid":"123","vout":1,"tree":0,"scriptPubKey":"00","redeemScript":"01"}]],"id":1}`,
			unmarshalled: &dcrjson.SignRawTransactionCmd{
				RawTx: "001122",
				Inputs: &[]dcrjson.RawTxInput{
					{
						Txid:         "123",
						Vout:         1,
						ScriptPubKey: "00",
						RedeemScript: "01",
					},
				},
				PrivKeys: nil,
				Flags:    dcrjson.String("ALL"),
			},
		},
		{
			name: "signrawtransaction optional2",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("signrawtransaction", "001122", `[]`, `["abc"]`)
			},
			staticCmd: func() interface{} {
				txInputs := []dcrjson.RawTxInput{}
				privKeys := []string{"abc"}
				return dcrjson.NewSignRawTransactionCmd("001122", &txInputs, &privKeys, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransaction","params":["001122",[],["abc"]],"id":1}`,
			unmarshalled: &dcrjson.SignRawTransactionCmd{
				RawTx:    "001122",
				Inputs:   &[]dcrjson.RawTxInput{},
				PrivKeys: &[]string{"abc"},
				Flags:    dcrjson.String("ALL"),
			},
		},
		{
			name: "signrawtransaction optional3",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("signrawtransaction", "001122", `[]`, `[]`, "ALL")
			},
			staticCmd: func() interface{} {
				txInputs := []dcrjson.RawTxInput{}
				privKeys := []string{}
				return dcrjson.NewSignRawTransactionCmd("001122", &txInputs, &privKeys,
					dcrjson.String("ALL"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransaction","params":["001122",[],[],"ALL"],"id":1}`,
			unmarshalled: &dcrjson.SignRawTransactionCmd{
				RawTx:    "001122",
				Inputs:   &[]dcrjson.RawTxInput{},
				PrivKeys: &[]string{},
				Flags:    dcrjson.String("ALL"),
			},
		},
		{
			name: "walletlock",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("walletlock")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewWalletLockCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"walletlock","params":[],"id":1}`,
			unmarshalled: &dcrjson.WalletLockCmd{},
		},
		{
			name: "walletpassphrase",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("walletpassphrase", "pass", 60)
			},
			staticCmd: func() interface{} {
				return dcrjson.NewWalletPassphraseCmd("pass", 60)
			},
			marshalled: `{"jsonrpc":"1.0","method":"walletpassphrase","params":["pass",60],"id":1}`,
			unmarshalled: &dcrjson.WalletPassphraseCmd{
				Passphrase: "pass",
				Timeout:    60,
			},
		},
		{
			name: "walletpassphrasechange",
			newCmd: func() (interface{}, error) {
				return dcrjson.NewCmd("walletpassphrasechange", "old", "new")
			},
			staticCmd: func() interface{} {
				return dcrjson.NewWalletPassphraseChangeCmd("old", "new")
			},
			marshalled: `{"jsonrpc":"1.0","method":"walletpassphrasechange","params":["old","new"],"id":1}`,
			unmarshalled: &dcrjson.WalletPassphraseChangeCmd{
				OldPassphrase: "old",
				NewPassphrase: "new",
			},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Marshal the command as created by the new static command
		// creation function.
		marshalled, err := dcrjson.MarshalCmd("1.0", testID, test.staticCmd())
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

		// Ensure the command is created without error via the generic
		// new command creation function.
		cmd, err := test.newCmd()
		if err != nil {
			t.Errorf("Test #%d (%s) unexpected NewCmd error: %v ",
				i, test.name, err)
		}

		// Marshal the command as created by the generic new command
		// creation function.
		marshalled, err = dcrjson.MarshalCmd("1.0", testID, cmd)
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
