// Copyright (c) 2014-2020 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcutil"
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
				return btcjson.NewCmd("addmultisigaddress", 2, []string{"031234", "035678"})
			},
			staticCmd: func() interface{} {
				keys := []string{"031234", "035678"}
				return btcjson.NewAddMultisigAddressCmd(2, keys, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"addmultisigaddress","params":[2,["031234","035678"]],"id":1}`,
			unmarshalled: &btcjson.AddMultisigAddressCmd{
				NRequired: 2,
				Keys:      []string{"031234", "035678"},
				Account:   nil,
			},
		},
		{
			name: "addmultisigaddress optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("addmultisigaddress", 2, []string{"031234", "035678"}, "test")
			},
			staticCmd: func() interface{} {
				keys := []string{"031234", "035678"}
				return btcjson.NewAddMultisigAddressCmd(2, keys, btcjson.String("test"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"addmultisigaddress","params":[2,["031234","035678"],"test"],"id":1}`,
			unmarshalled: &btcjson.AddMultisigAddressCmd{
				NRequired: 2,
				Keys:      []string{"031234", "035678"},
				Account:   btcjson.String("test"),
			},
		},
		{
			name: "createwallet",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("createwallet", "mywallet", true, true, "secret", true)
			},
			staticCmd: func() interface{} {
				return btcjson.NewCreateWalletCmd("mywallet",
					btcjson.Bool(true), btcjson.Bool(true),
					btcjson.String("secret"), btcjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"createwallet","params":["mywallet",true,true,"secret",true],"id":1}`,
			unmarshalled: &btcjson.CreateWalletCmd{
				WalletName:         "mywallet",
				DisablePrivateKeys: btcjson.Bool(true),
				Blank:              btcjson.Bool(true),
				Passphrase:         btcjson.String("secret"),
				AvoidReuse:         btcjson.Bool(true),
			},
		},
		{
			name: "createwallet - optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("createwallet", "mywallet")
			},
			staticCmd: func() interface{} {
				return btcjson.NewCreateWalletCmd("mywallet",
					nil, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"createwallet","params":["mywallet"],"id":1}`,
			unmarshalled: &btcjson.CreateWalletCmd{
				WalletName:         "mywallet",
				DisablePrivateKeys: btcjson.Bool(false),
				Blank:              btcjson.Bool(false),
				Passphrase:         btcjson.String(""),
				AvoidReuse:         btcjson.Bool(false),
			},
		},
		{
			name: "createwallet - optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("createwallet", "mywallet", "null", "null", "secret")
			},
			staticCmd: func() interface{} {
				return btcjson.NewCreateWalletCmd("mywallet",
					nil, nil, btcjson.String("secret"), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"createwallet","params":["mywallet",null,null,"secret"],"id":1}`,
			unmarshalled: &btcjson.CreateWalletCmd{
				WalletName:         "mywallet",
				DisablePrivateKeys: nil,
				Blank:              nil,
				Passphrase:         btcjson.String("secret"),
				AvoidReuse:         btcjson.Bool(false),
			},
		},
		{
			name: "addwitnessaddress",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("addwitnessaddress", "1address")
			},
			staticCmd: func() interface{} {
				return btcjson.NewAddWitnessAddressCmd("1address")
			},
			marshalled: `{"jsonrpc":"1.0","method":"addwitnessaddress","params":["1address"],"id":1}`,
			unmarshalled: &btcjson.AddWitnessAddressCmd{
				Address: "1address",
			},
		},
		{
			name: "backupwallet",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("backupwallet", "backup.dat")
			},
			staticCmd: func() interface{} {
				return btcjson.NewBackupWalletCmd("backup.dat")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"backupwallet","params":["backup.dat"],"id":1}`,
			unmarshalled: &btcjson.BackupWalletCmd{Destination: "backup.dat"},
		},
		{
			name: "loadwallet",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("loadwallet", "wallet.dat")
			},
			staticCmd: func() interface{} {
				return btcjson.NewLoadWalletCmd("wallet.dat")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"loadwallet","params":["wallet.dat"],"id":1}`,
			unmarshalled: &btcjson.LoadWalletCmd{WalletName: "wallet.dat"},
		},
		{
			name: "unloadwallet",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("unloadwallet", "default")
			},
			staticCmd: func() interface{} {
				return btcjson.NewUnloadWalletCmd(btcjson.String("default"))
			},
			marshalled:   `{"jsonrpc":"1.0","method":"unloadwallet","params":["default"],"id":1}`,
			unmarshalled: &btcjson.UnloadWalletCmd{WalletName: btcjson.String("default")},
		},
		{name: "unloadwallet - nil arg",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("unloadwallet")
			},
			staticCmd: func() interface{} {
				return btcjson.NewUnloadWalletCmd(nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"unloadwallet","params":[],"id":1}`,
			unmarshalled: &btcjson.UnloadWalletCmd{WalletName: nil},
		},
		{
			name: "createmultisig",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("createmultisig", 2, []string{"031234", "035678"})
			},
			staticCmd: func() interface{} {
				keys := []string{"031234", "035678"}
				return btcjson.NewCreateMultisigCmd(2, keys)
			},
			marshalled: `{"jsonrpc":"1.0","method":"createmultisig","params":[2,["031234","035678"]],"id":1}`,
			unmarshalled: &btcjson.CreateMultisigCmd{
				NRequired: 2,
				Keys:      []string{"031234", "035678"},
			},
		},
		{
			name: "dumpprivkey",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("dumpprivkey", "1Address")
			},
			staticCmd: func() interface{} {
				return btcjson.NewDumpPrivKeyCmd("1Address")
			},
			marshalled: `{"jsonrpc":"1.0","method":"dumpprivkey","params":["1Address"],"id":1}`,
			unmarshalled: &btcjson.DumpPrivKeyCmd{
				Address: "1Address",
			},
		},
		{
			name: "encryptwallet",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("encryptwallet", "pass")
			},
			staticCmd: func() interface{} {
				return btcjson.NewEncryptWalletCmd("pass")
			},
			marshalled: `{"jsonrpc":"1.0","method":"encryptwallet","params":["pass"],"id":1}`,
			unmarshalled: &btcjson.EncryptWalletCmd{
				Passphrase: "pass",
			},
		},
		{
			name: "estimatefee",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("estimatefee", 6)
			},
			staticCmd: func() interface{} {
				return btcjson.NewEstimateFeeCmd(6)
			},
			marshalled: `{"jsonrpc":"1.0","method":"estimatefee","params":[6],"id":1}`,
			unmarshalled: &btcjson.EstimateFeeCmd{
				NumBlocks: 6,
			},
		},
		{
			name: "estimatesmartfee - no mode",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("estimatesmartfee", 6)
			},
			staticCmd: func() interface{} {
				return btcjson.NewEstimateSmartFeeCmd(6, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"estimatesmartfee","params":[6],"id":1}`,
			unmarshalled: &btcjson.EstimateSmartFeeCmd{
				ConfTarget:   6,
				EstimateMode: &btcjson.EstimateModeConservative,
			},
		},
		{
			name: "estimatesmartfee - economical mode",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("estimatesmartfee", 6, btcjson.EstimateModeEconomical)
			},
			staticCmd: func() interface{} {
				return btcjson.NewEstimateSmartFeeCmd(6, &btcjson.EstimateModeEconomical)
			},
			marshalled: `{"jsonrpc":"1.0","method":"estimatesmartfee","params":[6,"ECONOMICAL"],"id":1}`,
			unmarshalled: &btcjson.EstimateSmartFeeCmd{
				ConfTarget:   6,
				EstimateMode: &btcjson.EstimateModeEconomical,
			},
		},
		{
			name: "estimatepriority",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("estimatepriority", 6)
			},
			staticCmd: func() interface{} {
				return btcjson.NewEstimatePriorityCmd(6)
			},
			marshalled: `{"jsonrpc":"1.0","method":"estimatepriority","params":[6],"id":1}`,
			unmarshalled: &btcjson.EstimatePriorityCmd{
				NumBlocks: 6,
			},
		},
		{
			name: "getaccount",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getaccount", "1Address")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetAccountCmd("1Address")
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaccount","params":["1Address"],"id":1}`,
			unmarshalled: &btcjson.GetAccountCmd{
				Address: "1Address",
			},
		},
		{
			name: "getaccountaddress",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getaccountaddress", "acct")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetAccountAddressCmd("acct")
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaccountaddress","params":["acct"],"id":1}`,
			unmarshalled: &btcjson.GetAccountAddressCmd{
				Account: "acct",
			},
		},
		{
			name: "getaddressesbyaccount",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getaddressesbyaccount", "acct")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetAddressesByAccountCmd("acct")
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaddressesbyaccount","params":["acct"],"id":1}`,
			unmarshalled: &btcjson.GetAddressesByAccountCmd{
				Account: "acct",
			},
		},
		{
			name: "getaddressinfo",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getaddressinfo", "1234")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetAddressInfoCmd("1234")
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaddressinfo","params":["1234"],"id":1}`,
			unmarshalled: &btcjson.GetAddressInfoCmd{
				Address: "1234",
			},
		},
		{
			name: "getbalance",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getbalance")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBalanceCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getbalance","params":[],"id":1}`,
			unmarshalled: &btcjson.GetBalanceCmd{
				Account: nil,
				MinConf: btcjson.Int(1),
			},
		},
		{
			name: "getbalance optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getbalance", "acct")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBalanceCmd(btcjson.String("acct"), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getbalance","params":["acct"],"id":1}`,
			unmarshalled: &btcjson.GetBalanceCmd{
				Account: btcjson.String("acct"),
				MinConf: btcjson.Int(1),
			},
		},
		{
			name: "getbalance optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getbalance", "acct", 6)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBalanceCmd(btcjson.String("acct"), btcjson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getbalance","params":["acct",6],"id":1}`,
			unmarshalled: &btcjson.GetBalanceCmd{
				Account: btcjson.String("acct"),
				MinConf: btcjson.Int(6),
			},
		},
		{
			name: "getbalances",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getbalances")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBalancesCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getbalances","params":[],"id":1}`,
			unmarshalled: &btcjson.GetBalancesCmd{},
		},
		{
			name: "getnewaddress",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getnewaddress")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetNewAddressCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnewaddress","params":[],"id":1}`,
			unmarshalled: &btcjson.GetNewAddressCmd{
				Account: nil,
			},
		},
		{
			name: "getnewaddress optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getnewaddress", "acct")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetNewAddressCmd(btcjson.String("acct"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnewaddress","params":["acct"],"id":1}`,
			unmarshalled: &btcjson.GetNewAddressCmd{
				Account: btcjson.String("acct"),
			},
		},
		{
			name: "getrawchangeaddress",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getrawchangeaddress")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetRawChangeAddressCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawchangeaddress","params":[],"id":1}`,
			unmarshalled: &btcjson.GetRawChangeAddressCmd{
				Account: nil,
			},
		},
		{
			name: "getrawchangeaddress optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getrawchangeaddress", "acct")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetRawChangeAddressCmd(btcjson.String("acct"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawchangeaddress","params":["acct"],"id":1}`,
			unmarshalled: &btcjson.GetRawChangeAddressCmd{
				Account: btcjson.String("acct"),
			},
		},
		{
			name: "getreceivedbyaccount",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getreceivedbyaccount", "acct")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetReceivedByAccountCmd("acct", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getreceivedbyaccount","params":["acct"],"id":1}`,
			unmarshalled: &btcjson.GetReceivedByAccountCmd{
				Account: "acct",
				MinConf: btcjson.Int(1),
			},
		},
		{
			name: "getreceivedbyaccount optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getreceivedbyaccount", "acct", 6)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetReceivedByAccountCmd("acct", btcjson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getreceivedbyaccount","params":["acct",6],"id":1}`,
			unmarshalled: &btcjson.GetReceivedByAccountCmd{
				Account: "acct",
				MinConf: btcjson.Int(6),
			},
		},
		{
			name: "getreceivedbyaddress",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getreceivedbyaddress", "1Address")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetReceivedByAddressCmd("1Address", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getreceivedbyaddress","params":["1Address"],"id":1}`,
			unmarshalled: &btcjson.GetReceivedByAddressCmd{
				Address: "1Address",
				MinConf: btcjson.Int(1),
			},
		},
		{
			name: "getreceivedbyaddress optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getreceivedbyaddress", "1Address", 6)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetReceivedByAddressCmd("1Address", btcjson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getreceivedbyaddress","params":["1Address",6],"id":1}`,
			unmarshalled: &btcjson.GetReceivedByAddressCmd{
				Address: "1Address",
				MinConf: btcjson.Int(6),
			},
		},
		{
			name: "gettransaction",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("gettransaction", "123")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetTransactionCmd("123", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettransaction","params":["123"],"id":1}`,
			unmarshalled: &btcjson.GetTransactionCmd{
				Txid:             "123",
				IncludeWatchOnly: btcjson.Bool(false),
			},
		},
		{
			name: "gettransaction optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("gettransaction", "123", true)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetTransactionCmd("123", btcjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettransaction","params":["123",true],"id":1}`,
			unmarshalled: &btcjson.GetTransactionCmd{
				Txid:             "123",
				IncludeWatchOnly: btcjson.Bool(true),
			},
		},
		{
			name: "getwalletinfo",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getwalletinfo")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetWalletInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getwalletinfo","params":[],"id":1}`,
			unmarshalled: &btcjson.GetWalletInfoCmd{},
		},
		{
			name: "importprivkey",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("importprivkey", "abc")
			},
			staticCmd: func() interface{} {
				return btcjson.NewImportPrivKeyCmd("abc", nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importprivkey","params":["abc"],"id":1}`,
			unmarshalled: &btcjson.ImportPrivKeyCmd{
				PrivKey: "abc",
				Label:   nil,
				Rescan:  btcjson.Bool(true),
			},
		},
		{
			name: "importprivkey optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("importprivkey", "abc", "label")
			},
			staticCmd: func() interface{} {
				return btcjson.NewImportPrivKeyCmd("abc", btcjson.String("label"), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importprivkey","params":["abc","label"],"id":1}`,
			unmarshalled: &btcjson.ImportPrivKeyCmd{
				PrivKey: "abc",
				Label:   btcjson.String("label"),
				Rescan:  btcjson.Bool(true),
			},
		},
		{
			name: "importprivkey optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("importprivkey", "abc", "label", false)
			},
			staticCmd: func() interface{} {
				return btcjson.NewImportPrivKeyCmd("abc", btcjson.String("label"), btcjson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"importprivkey","params":["abc","label",false],"id":1}`,
			unmarshalled: &btcjson.ImportPrivKeyCmd{
				PrivKey: "abc",
				Label:   btcjson.String("label"),
				Rescan:  btcjson.Bool(false),
			},
		},
		{
			name: "keypoolrefill",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("keypoolrefill")
			},
			staticCmd: func() interface{} {
				return btcjson.NewKeyPoolRefillCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"keypoolrefill","params":[],"id":1}`,
			unmarshalled: &btcjson.KeyPoolRefillCmd{
				NewSize: btcjson.Uint(100),
			},
		},
		{
			name: "keypoolrefill optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("keypoolrefill", 200)
			},
			staticCmd: func() interface{} {
				return btcjson.NewKeyPoolRefillCmd(btcjson.Uint(200))
			},
			marshalled: `{"jsonrpc":"1.0","method":"keypoolrefill","params":[200],"id":1}`,
			unmarshalled: &btcjson.KeyPoolRefillCmd{
				NewSize: btcjson.Uint(200),
			},
		},
		{
			name: "listaccounts",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listaccounts")
			},
			staticCmd: func() interface{} {
				return btcjson.NewListAccountsCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listaccounts","params":[],"id":1}`,
			unmarshalled: &btcjson.ListAccountsCmd{
				MinConf: btcjson.Int(1),
			},
		},
		{
			name: "listaccounts optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listaccounts", 6)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListAccountsCmd(btcjson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listaccounts","params":[6],"id":1}`,
			unmarshalled: &btcjson.ListAccountsCmd{
				MinConf: btcjson.Int(6),
			},
		},
		{
			name: "listaddressgroupings",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listaddressgroupings")
			},
			staticCmd: func() interface{} {
				return btcjson.NewListAddressGroupingsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"listaddressgroupings","params":[],"id":1}`,
			unmarshalled: &btcjson.ListAddressGroupingsCmd{},
		},
		{
			name: "listlockunspent",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listlockunspent")
			},
			staticCmd: func() interface{} {
				return btcjson.NewListLockUnspentCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"listlockunspent","params":[],"id":1}`,
			unmarshalled: &btcjson.ListLockUnspentCmd{},
		},
		{
			name: "listreceivedbyaccount",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listreceivedbyaccount")
			},
			staticCmd: func() interface{} {
				return btcjson.NewListReceivedByAccountCmd(nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaccount","params":[],"id":1}`,
			unmarshalled: &btcjson.ListReceivedByAccountCmd{
				MinConf:          btcjson.Int(1),
				IncludeEmpty:     btcjson.Bool(false),
				IncludeWatchOnly: btcjson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaccount optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listreceivedbyaccount", 6)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListReceivedByAccountCmd(btcjson.Int(6), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaccount","params":[6],"id":1}`,
			unmarshalled: &btcjson.ListReceivedByAccountCmd{
				MinConf:          btcjson.Int(6),
				IncludeEmpty:     btcjson.Bool(false),
				IncludeWatchOnly: btcjson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaccount optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listreceivedbyaccount", 6, true)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListReceivedByAccountCmd(btcjson.Int(6), btcjson.Bool(true), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaccount","params":[6,true],"id":1}`,
			unmarshalled: &btcjson.ListReceivedByAccountCmd{
				MinConf:          btcjson.Int(6),
				IncludeEmpty:     btcjson.Bool(true),
				IncludeWatchOnly: btcjson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaccount optional3",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listreceivedbyaccount", 6, true, false)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListReceivedByAccountCmd(btcjson.Int(6), btcjson.Bool(true), btcjson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaccount","params":[6,true,false],"id":1}`,
			unmarshalled: &btcjson.ListReceivedByAccountCmd{
				MinConf:          btcjson.Int(6),
				IncludeEmpty:     btcjson.Bool(true),
				IncludeWatchOnly: btcjson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaddress",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listreceivedbyaddress")
			},
			staticCmd: func() interface{} {
				return btcjson.NewListReceivedByAddressCmd(nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaddress","params":[],"id":1}`,
			unmarshalled: &btcjson.ListReceivedByAddressCmd{
				MinConf:          btcjson.Int(1),
				IncludeEmpty:     btcjson.Bool(false),
				IncludeWatchOnly: btcjson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaddress optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listreceivedbyaddress", 6)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListReceivedByAddressCmd(btcjson.Int(6), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaddress","params":[6],"id":1}`,
			unmarshalled: &btcjson.ListReceivedByAddressCmd{
				MinConf:          btcjson.Int(6),
				IncludeEmpty:     btcjson.Bool(false),
				IncludeWatchOnly: btcjson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaddress optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listreceivedbyaddress", 6, true)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListReceivedByAddressCmd(btcjson.Int(6), btcjson.Bool(true), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaddress","params":[6,true],"id":1}`,
			unmarshalled: &btcjson.ListReceivedByAddressCmd{
				MinConf:          btcjson.Int(6),
				IncludeEmpty:     btcjson.Bool(true),
				IncludeWatchOnly: btcjson.Bool(false),
			},
		},
		{
			name: "listreceivedbyaddress optional3",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listreceivedbyaddress", 6, true, false)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListReceivedByAddressCmd(btcjson.Int(6), btcjson.Bool(true), btcjson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listreceivedbyaddress","params":[6,true,false],"id":1}`,
			unmarshalled: &btcjson.ListReceivedByAddressCmd{
				MinConf:          btcjson.Int(6),
				IncludeEmpty:     btcjson.Bool(true),
				IncludeWatchOnly: btcjson.Bool(false),
			},
		},
		{
			name: "listsinceblock",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listsinceblock")
			},
			staticCmd: func() interface{} {
				return btcjson.NewListSinceBlockCmd(nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listsinceblock","params":[],"id":1}`,
			unmarshalled: &btcjson.ListSinceBlockCmd{
				BlockHash:           nil,
				TargetConfirmations: btcjson.Int(1),
				IncludeWatchOnly:    btcjson.Bool(false),
			},
		},
		{
			name: "listsinceblock optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listsinceblock", "123")
			},
			staticCmd: func() interface{} {
				return btcjson.NewListSinceBlockCmd(btcjson.String("123"), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listsinceblock","params":["123"],"id":1}`,
			unmarshalled: &btcjson.ListSinceBlockCmd{
				BlockHash:           btcjson.String("123"),
				TargetConfirmations: btcjson.Int(1),
				IncludeWatchOnly:    btcjson.Bool(false),
			},
		},
		{
			name: "listsinceblock optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listsinceblock", "123", 6)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListSinceBlockCmd(btcjson.String("123"), btcjson.Int(6), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listsinceblock","params":["123",6],"id":1}`,
			unmarshalled: &btcjson.ListSinceBlockCmd{
				BlockHash:           btcjson.String("123"),
				TargetConfirmations: btcjson.Int(6),
				IncludeWatchOnly:    btcjson.Bool(false),
			},
		},
		{
			name: "listsinceblock optional3",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listsinceblock", "123", 6, true)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListSinceBlockCmd(btcjson.String("123"), btcjson.Int(6), btcjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listsinceblock","params":["123",6,true],"id":1}`,
			unmarshalled: &btcjson.ListSinceBlockCmd{
				BlockHash:           btcjson.String("123"),
				TargetConfirmations: btcjson.Int(6),
				IncludeWatchOnly:    btcjson.Bool(true),
			},
		},
		{
			name: "listsinceblock pad null",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listsinceblock", "null", 1, false)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListSinceBlockCmd(nil, btcjson.Int(1), btcjson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listsinceblock","params":[null,1,false],"id":1}`,
			unmarshalled: &btcjson.ListSinceBlockCmd{
				BlockHash:           nil,
				TargetConfirmations: btcjson.Int(1),
				IncludeWatchOnly:    btcjson.Bool(false),
			},
		},
		{
			name: "listtransactions",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listtransactions")
			},
			staticCmd: func() interface{} {
				return btcjson.NewListTransactionsCmd(nil, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":[],"id":1}`,
			unmarshalled: &btcjson.ListTransactionsCmd{
				Account:          nil,
				Count:            btcjson.Int(10),
				From:             btcjson.Int(0),
				IncludeWatchOnly: btcjson.Bool(false),
			},
		},
		{
			name: "listtransactions optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listtransactions", "acct")
			},
			staticCmd: func() interface{} {
				return btcjson.NewListTransactionsCmd(btcjson.String("acct"), nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":["acct"],"id":1}`,
			unmarshalled: &btcjson.ListTransactionsCmd{
				Account:          btcjson.String("acct"),
				Count:            btcjson.Int(10),
				From:             btcjson.Int(0),
				IncludeWatchOnly: btcjson.Bool(false),
			},
		},
		{
			name: "listtransactions optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listtransactions", "acct", 20)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListTransactionsCmd(btcjson.String("acct"), btcjson.Int(20), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":["acct",20],"id":1}`,
			unmarshalled: &btcjson.ListTransactionsCmd{
				Account:          btcjson.String("acct"),
				Count:            btcjson.Int(20),
				From:             btcjson.Int(0),
				IncludeWatchOnly: btcjson.Bool(false),
			},
		},
		{
			name: "listtransactions optional3",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listtransactions", "acct", 20, 1)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListTransactionsCmd(btcjson.String("acct"), btcjson.Int(20),
					btcjson.Int(1), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":["acct",20,1],"id":1}`,
			unmarshalled: &btcjson.ListTransactionsCmd{
				Account:          btcjson.String("acct"),
				Count:            btcjson.Int(20),
				From:             btcjson.Int(1),
				IncludeWatchOnly: btcjson.Bool(false),
			},
		},
		{
			name: "listtransactions optional4",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listtransactions", "acct", 20, 1, true)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListTransactionsCmd(btcjson.String("acct"), btcjson.Int(20),
					btcjson.Int(1), btcjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"listtransactions","params":["acct",20,1,true],"id":1}`,
			unmarshalled: &btcjson.ListTransactionsCmd{
				Account:          btcjson.String("acct"),
				Count:            btcjson.Int(20),
				From:             btcjson.Int(1),
				IncludeWatchOnly: btcjson.Bool(true),
			},
		},
		{
			name: "listunspent",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listunspent")
			},
			staticCmd: func() interface{} {
				return btcjson.NewListUnspentCmd(nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listunspent","params":[],"id":1}`,
			unmarshalled: &btcjson.ListUnspentCmd{
				MinConf:   btcjson.Int(1),
				MaxConf:   btcjson.Int(9999999),
				Addresses: nil,
			},
		},
		{
			name: "listunspent optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listunspent", 6)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListUnspentCmd(btcjson.Int(6), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listunspent","params":[6],"id":1}`,
			unmarshalled: &btcjson.ListUnspentCmd{
				MinConf:   btcjson.Int(6),
				MaxConf:   btcjson.Int(9999999),
				Addresses: nil,
			},
		},
		{
			name: "listunspent optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listunspent", 6, 100)
			},
			staticCmd: func() interface{} {
				return btcjson.NewListUnspentCmd(btcjson.Int(6), btcjson.Int(100), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"listunspent","params":[6,100],"id":1}`,
			unmarshalled: &btcjson.ListUnspentCmd{
				MinConf:   btcjson.Int(6),
				MaxConf:   btcjson.Int(100),
				Addresses: nil,
			},
		},
		{
			name: "listunspent optional3",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("listunspent", 6, 100, []string{"1Address", "1Address2"})
			},
			staticCmd: func() interface{} {
				return btcjson.NewListUnspentCmd(btcjson.Int(6), btcjson.Int(100),
					&[]string{"1Address", "1Address2"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"listunspent","params":[6,100,["1Address","1Address2"]],"id":1}`,
			unmarshalled: &btcjson.ListUnspentCmd{
				MinConf:   btcjson.Int(6),
				MaxConf:   btcjson.Int(100),
				Addresses: &[]string{"1Address", "1Address2"},
			},
		},
		{
			name: "lockunspent",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("lockunspent", true, `[{"txid":"123","vout":1}]`)
			},
			staticCmd: func() interface{} {
				txInputs := []btcjson.TransactionInput{
					{Txid: "123", Vout: 1},
				}
				return btcjson.NewLockUnspentCmd(true, txInputs)
			},
			marshalled: `{"jsonrpc":"1.0","method":"lockunspent","params":[true,[{"txid":"123","vout":1}]],"id":1}`,
			unmarshalled: &btcjson.LockUnspentCmd{
				Unlock: true,
				Transactions: []btcjson.TransactionInput{
					{Txid: "123", Vout: 1},
				},
			},
		},
		{
			name: "move",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("move", "from", "to", 0.5)
			},
			staticCmd: func() interface{} {
				return btcjson.NewMoveCmd("from", "to", 0.5, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"move","params":["from","to",0.5],"id":1}`,
			unmarshalled: &btcjson.MoveCmd{
				FromAccount: "from",
				ToAccount:   "to",
				Amount:      0.5,
				MinConf:     btcjson.Int(1),
				Comment:     nil,
			},
		},
		{
			name: "move optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("move", "from", "to", 0.5, 6)
			},
			staticCmd: func() interface{} {
				return btcjson.NewMoveCmd("from", "to", 0.5, btcjson.Int(6), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"move","params":["from","to",0.5,6],"id":1}`,
			unmarshalled: &btcjson.MoveCmd{
				FromAccount: "from",
				ToAccount:   "to",
				Amount:      0.5,
				MinConf:     btcjson.Int(6),
				Comment:     nil,
			},
		},
		{
			name: "move optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("move", "from", "to", 0.5, 6, "comment")
			},
			staticCmd: func() interface{} {
				return btcjson.NewMoveCmd("from", "to", 0.5, btcjson.Int(6), btcjson.String("comment"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"move","params":["from","to",0.5,6,"comment"],"id":1}`,
			unmarshalled: &btcjson.MoveCmd{
				FromAccount: "from",
				ToAccount:   "to",
				Amount:      0.5,
				MinConf:     btcjson.Int(6),
				Comment:     btcjson.String("comment"),
			},
		},
		{
			name: "sendfrom",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("sendfrom", "from", "1Address", 0.5)
			},
			staticCmd: func() interface{} {
				return btcjson.NewSendFromCmd("from", "1Address", 0.5, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendfrom","params":["from","1Address",0.5],"id":1}`,
			unmarshalled: &btcjson.SendFromCmd{
				FromAccount: "from",
				ToAddress:   "1Address",
				Amount:      0.5,
				MinConf:     btcjson.Int(1),
				Comment:     nil,
				CommentTo:   nil,
			},
		},
		{
			name: "sendfrom optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("sendfrom", "from", "1Address", 0.5, 6)
			},
			staticCmd: func() interface{} {
				return btcjson.NewSendFromCmd("from", "1Address", 0.5, btcjson.Int(6), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendfrom","params":["from","1Address",0.5,6],"id":1}`,
			unmarshalled: &btcjson.SendFromCmd{
				FromAccount: "from",
				ToAddress:   "1Address",
				Amount:      0.5,
				MinConf:     btcjson.Int(6),
				Comment:     nil,
				CommentTo:   nil,
			},
		},
		{
			name: "sendfrom optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("sendfrom", "from", "1Address", 0.5, 6, "comment")
			},
			staticCmd: func() interface{} {
				return btcjson.NewSendFromCmd("from", "1Address", 0.5, btcjson.Int(6),
					btcjson.String("comment"), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendfrom","params":["from","1Address",0.5,6,"comment"],"id":1}`,
			unmarshalled: &btcjson.SendFromCmd{
				FromAccount: "from",
				ToAddress:   "1Address",
				Amount:      0.5,
				MinConf:     btcjson.Int(6),
				Comment:     btcjson.String("comment"),
				CommentTo:   nil,
			},
		},
		{
			name: "sendfrom optional3",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("sendfrom", "from", "1Address", 0.5, 6, "comment", "commentto")
			},
			staticCmd: func() interface{} {
				return btcjson.NewSendFromCmd("from", "1Address", 0.5, btcjson.Int(6),
					btcjson.String("comment"), btcjson.String("commentto"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendfrom","params":["from","1Address",0.5,6,"comment","commentto"],"id":1}`,
			unmarshalled: &btcjson.SendFromCmd{
				FromAccount: "from",
				ToAddress:   "1Address",
				Amount:      0.5,
				MinConf:     btcjson.Int(6),
				Comment:     btcjson.String("comment"),
				CommentTo:   btcjson.String("commentto"),
			},
		},
		{
			name: "sendmany",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("sendmany", "from", `{"1Address":0.5}`)
			},
			staticCmd: func() interface{} {
				amounts := map[string]float64{"1Address": 0.5}
				return btcjson.NewSendManyCmd("from", amounts, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendmany","params":["from",{"1Address":0.5}],"id":1}`,
			unmarshalled: &btcjson.SendManyCmd{
				FromAccount: "from",
				Amounts:     map[string]float64{"1Address": 0.5},
				MinConf:     btcjson.Int(1),
				Comment:     nil,
			},
		},
		{
			name: "sendmany optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("sendmany", "from", `{"1Address":0.5}`, 6)
			},
			staticCmd: func() interface{} {
				amounts := map[string]float64{"1Address": 0.5}
				return btcjson.NewSendManyCmd("from", amounts, btcjson.Int(6), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendmany","params":["from",{"1Address":0.5},6],"id":1}`,
			unmarshalled: &btcjson.SendManyCmd{
				FromAccount: "from",
				Amounts:     map[string]float64{"1Address": 0.5},
				MinConf:     btcjson.Int(6),
				Comment:     nil,
			},
		},
		{
			name: "sendmany optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("sendmany", "from", `{"1Address":0.5}`, 6, "comment")
			},
			staticCmd: func() interface{} {
				amounts := map[string]float64{"1Address": 0.5}
				return btcjson.NewSendManyCmd("from", amounts, btcjson.Int(6), btcjson.String("comment"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendmany","params":["from",{"1Address":0.5},6,"comment"],"id":1}`,
			unmarshalled: &btcjson.SendManyCmd{
				FromAccount: "from",
				Amounts:     map[string]float64{"1Address": 0.5},
				MinConf:     btcjson.Int(6),
				Comment:     btcjson.String("comment"),
			},
		},
		{
			name: "sendtoaddress",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("sendtoaddress", "1Address", 0.5)
			},
			staticCmd: func() interface{} {
				return btcjson.NewSendToAddressCmd("1Address", 0.5, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendtoaddress","params":["1Address",0.5],"id":1}`,
			unmarshalled: &btcjson.SendToAddressCmd{
				Address:   "1Address",
				Amount:    0.5,
				Comment:   nil,
				CommentTo: nil,
			},
		},
		{
			name: "sendtoaddress optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("sendtoaddress", "1Address", 0.5, "comment", "commentto")
			},
			staticCmd: func() interface{} {
				return btcjson.NewSendToAddressCmd("1Address", 0.5, btcjson.String("comment"),
					btcjson.String("commentto"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendtoaddress","params":["1Address",0.5,"comment","commentto"],"id":1}`,
			unmarshalled: &btcjson.SendToAddressCmd{
				Address:   "1Address",
				Amount:    0.5,
				Comment:   btcjson.String("comment"),
				CommentTo: btcjson.String("commentto"),
			},
		},
		{
			name: "setaccount",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("setaccount", "1Address", "acct")
			},
			staticCmd: func() interface{} {
				return btcjson.NewSetAccountCmd("1Address", "acct")
			},
			marshalled: `{"jsonrpc":"1.0","method":"setaccount","params":["1Address","acct"],"id":1}`,
			unmarshalled: &btcjson.SetAccountCmd{
				Address: "1Address",
				Account: "acct",
			},
		},
		{
			name: "settxfee",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("settxfee", 0.0001)
			},
			staticCmd: func() interface{} {
				return btcjson.NewSetTxFeeCmd(0.0001)
			},
			marshalled: `{"jsonrpc":"1.0","method":"settxfee","params":[0.0001],"id":1}`,
			unmarshalled: &btcjson.SetTxFeeCmd{
				Amount: 0.0001,
			},
		},
		{
			name: "signmessage",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("signmessage", "1Address", "message")
			},
			staticCmd: func() interface{} {
				return btcjson.NewSignMessageCmd("1Address", "message")
			},
			marshalled: `{"jsonrpc":"1.0","method":"signmessage","params":["1Address","message"],"id":1}`,
			unmarshalled: &btcjson.SignMessageCmd{
				Address: "1Address",
				Message: "message",
			},
		},
		{
			name: "signrawtransaction",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("signrawtransaction", "001122")
			},
			staticCmd: func() interface{} {
				return btcjson.NewSignRawTransactionCmd("001122", nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransaction","params":["001122"],"id":1}`,
			unmarshalled: &btcjson.SignRawTransactionCmd{
				RawTx:    "001122",
				Inputs:   nil,
				PrivKeys: nil,
				Flags:    btcjson.String("ALL"),
			},
		},
		{
			name: "signrawtransaction optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("signrawtransaction", "001122", `[{"txid":"123","vout":1,"scriptPubKey":"00","redeemScript":"01"}]`)
			},
			staticCmd: func() interface{} {
				txInputs := []btcjson.RawTxInput{
					{
						Txid:         "123",
						Vout:         1,
						ScriptPubKey: "00",
						RedeemScript: "01",
					},
				}

				return btcjson.NewSignRawTransactionCmd("001122", &txInputs, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransaction","params":["001122",[{"txid":"123","vout":1,"scriptPubKey":"00","redeemScript":"01"}]],"id":1}`,
			unmarshalled: &btcjson.SignRawTransactionCmd{
				RawTx: "001122",
				Inputs: &[]btcjson.RawTxInput{
					{
						Txid:         "123",
						Vout:         1,
						ScriptPubKey: "00",
						RedeemScript: "01",
					},
				},
				PrivKeys: nil,
				Flags:    btcjson.String("ALL"),
			},
		},
		{
			name: "signrawtransaction optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("signrawtransaction", "001122", `[]`, `["abc"]`)
			},
			staticCmd: func() interface{} {
				txInputs := []btcjson.RawTxInput{}
				privKeys := []string{"abc"}
				return btcjson.NewSignRawTransactionCmd("001122", &txInputs, &privKeys, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransaction","params":["001122",[],["abc"]],"id":1}`,
			unmarshalled: &btcjson.SignRawTransactionCmd{
				RawTx:    "001122",
				Inputs:   &[]btcjson.RawTxInput{},
				PrivKeys: &[]string{"abc"},
				Flags:    btcjson.String("ALL"),
			},
		},
		{
			name: "signrawtransaction optional3",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("signrawtransaction", "001122", `[]`, `[]`, "ALL")
			},
			staticCmd: func() interface{} {
				txInputs := []btcjson.RawTxInput{}
				privKeys := []string{}
				return btcjson.NewSignRawTransactionCmd("001122", &txInputs, &privKeys,
					btcjson.String("ALL"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransaction","params":["001122",[],[],"ALL"],"id":1}`,
			unmarshalled: &btcjson.SignRawTransactionCmd{
				RawTx:    "001122",
				Inputs:   &[]btcjson.RawTxInput{},
				PrivKeys: &[]string{},
				Flags:    btcjson.String("ALL"),
			},
		},
		{
			name: "signrawtransactionwithwallet",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("signrawtransactionwithwallet", "001122")
			},
			staticCmd: func() interface{} {
				return btcjson.NewSignRawTransactionWithWalletCmd("001122", nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransactionwithwallet","params":["001122"],"id":1}`,
			unmarshalled: &btcjson.SignRawTransactionWithWalletCmd{
				RawTx:       "001122",
				Inputs:      nil,
				SigHashType: btcjson.String("ALL"),
			},
		},
		{
			name: "signrawtransactionwithwallet optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("signrawtransactionwithwallet", "001122", `[{"txid":"123","vout":1,"scriptPubKey":"00","redeemScript":"01","witnessScript":"02","amount":1.5}]`)
			},
			staticCmd: func() interface{} {
				txInputs := []btcjson.RawTxWitnessInput{
					{
						Txid:          "123",
						Vout:          1,
						ScriptPubKey:  "00",
						RedeemScript:  btcjson.String("01"),
						WitnessScript: btcjson.String("02"),
						Amount:        btcjson.Float64(1.5),
					},
				}

				return btcjson.NewSignRawTransactionWithWalletCmd("001122", &txInputs, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransactionwithwallet","params":["001122",[{"txid":"123","vout":1,"scriptPubKey":"00","redeemScript":"01","witnessScript":"02","amount":1.5}]],"id":1}`,
			unmarshalled: &btcjson.SignRawTransactionWithWalletCmd{
				RawTx: "001122",
				Inputs: &[]btcjson.RawTxWitnessInput{
					{
						Txid:          "123",
						Vout:          1,
						ScriptPubKey:  "00",
						RedeemScript:  btcjson.String("01"),
						WitnessScript: btcjson.String("02"),
						Amount:        btcjson.Float64(1.5),
					},
				},
				SigHashType: btcjson.String("ALL"),
			},
		},
		{
			name: "signrawtransactionwithwallet optional1 with blank fields in input",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("signrawtransactionwithwallet", "001122", `[{"txid":"123","vout":1,"scriptPubKey":"00","redeemScript":"01"}]`)
			},
			staticCmd: func() interface{} {
				txInputs := []btcjson.RawTxWitnessInput{
					{
						Txid:         "123",
						Vout:         1,
						ScriptPubKey: "00",
						RedeemScript: btcjson.String("01"),
					},
				}

				return btcjson.NewSignRawTransactionWithWalletCmd("001122", &txInputs, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransactionwithwallet","params":["001122",[{"txid":"123","vout":1,"scriptPubKey":"00","redeemScript":"01"}]],"id":1}`,
			unmarshalled: &btcjson.SignRawTransactionWithWalletCmd{
				RawTx: "001122",
				Inputs: &[]btcjson.RawTxWitnessInput{
					{
						Txid:         "123",
						Vout:         1,
						ScriptPubKey: "00",
						RedeemScript: btcjson.String("01"),
					},
				},
				SigHashType: btcjson.String("ALL"),
			},
		},
		{
			name: "signrawtransactionwithwallet optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("signrawtransactionwithwallet", "001122", `[]`, "ALL")
			},
			staticCmd: func() interface{} {
				txInputs := []btcjson.RawTxWitnessInput{}
				return btcjson.NewSignRawTransactionWithWalletCmd("001122", &txInputs, btcjson.String("ALL"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"signrawtransactionwithwallet","params":["001122",[],"ALL"],"id":1}`,
			unmarshalled: &btcjson.SignRawTransactionWithWalletCmd{
				RawTx:       "001122",
				Inputs:      &[]btcjson.RawTxWitnessInput{},
				SigHashType: btcjson.String("ALL"),
			},
		},
		{
			name: "walletlock",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("walletlock")
			},
			staticCmd: func() interface{} {
				return btcjson.NewWalletLockCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"walletlock","params":[],"id":1}`,
			unmarshalled: &btcjson.WalletLockCmd{},
		},
		{
			name: "walletpassphrase",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("walletpassphrase", "pass", 60)
			},
			staticCmd: func() interface{} {
				return btcjson.NewWalletPassphraseCmd("pass", 60)
			},
			marshalled: `{"jsonrpc":"1.0","method":"walletpassphrase","params":["pass",60],"id":1}`,
			unmarshalled: &btcjson.WalletPassphraseCmd{
				Passphrase: "pass",
				Timeout:    60,
			},
		},
		{
			name: "walletpassphrasechange",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("walletpassphrasechange", "old", "new")
			},
			staticCmd: func() interface{} {
				return btcjson.NewWalletPassphraseChangeCmd("old", "new")
			},
			marshalled: `{"jsonrpc":"1.0","method":"walletpassphrasechange","params":["old","new"],"id":1}`,
			unmarshalled: &btcjson.WalletPassphraseChangeCmd{
				OldPassphrase: "old",
				NewPassphrase: "new",
			},
		},
		{
			name: "importmulti with descriptor + options",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"importmulti",
					// Cannot use a native string, due to special types like timestamp.
					[]btcjson.ImportMultiRequest{
						{Descriptor: btcjson.String("123"), Timestamp: btcjson.TimestampOrNow{Value: 0}},
					},
					`{"rescan": true}`,
				)
			},
			staticCmd: func() interface{} {
				requests := []btcjson.ImportMultiRequest{
					{Descriptor: btcjson.String("123"), Timestamp: btcjson.TimestampOrNow{Value: 0}},
				}
				options := btcjson.ImportMultiOptions{Rescan: true}
				return btcjson.NewImportMultiCmd(requests, &options)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importmulti","params":[[{"desc":"123","timestamp":0}],{"rescan":true}],"id":1}`,
			unmarshalled: &btcjson.ImportMultiCmd{
				Requests: []btcjson.ImportMultiRequest{
					{
						Descriptor: btcjson.String("123"),
						Timestamp:  btcjson.TimestampOrNow{Value: 0},
					},
				},
				Options: &btcjson.ImportMultiOptions{Rescan: true},
			},
		},
		{
			name: "importmulti with descriptor + no options",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"importmulti",
					// Cannot use a native string, due to special types like timestamp.
					[]btcjson.ImportMultiRequest{
						{
							Descriptor: btcjson.String("123"),
							Timestamp:  btcjson.TimestampOrNow{Value: 0},
							WatchOnly:  btcjson.Bool(false),
							Internal:   btcjson.Bool(true),
							Label:      btcjson.String("aaa"),
							KeyPool:    btcjson.Bool(false),
						},
					},
				)
			},
			staticCmd: func() interface{} {
				requests := []btcjson.ImportMultiRequest{
					{
						Descriptor: btcjson.String("123"),
						Timestamp:  btcjson.TimestampOrNow{Value: 0},
						WatchOnly:  btcjson.Bool(false),
						Internal:   btcjson.Bool(true),
						Label:      btcjson.String("aaa"),
						KeyPool:    btcjson.Bool(false),
					},
				}
				return btcjson.NewImportMultiCmd(requests, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importmulti","params":[[{"desc":"123","timestamp":0,"internal":true,"watchonly":false,"label":"aaa","keypool":false}]],"id":1}`,
			unmarshalled: &btcjson.ImportMultiCmd{
				Requests: []btcjson.ImportMultiRequest{
					{
						Descriptor: btcjson.String("123"),
						Timestamp:  btcjson.TimestampOrNow{Value: 0},
						WatchOnly:  btcjson.Bool(false),
						Internal:   btcjson.Bool(true),
						Label:      btcjson.String("aaa"),
						KeyPool:    btcjson.Bool(false),
					},
				},
			},
		},
		{
			name: "importmulti with descriptor + string timestamp",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"importmulti",
					// Cannot use a native string, due to special types like timestamp.
					[]btcjson.ImportMultiRequest{
						{
							Descriptor: btcjson.String("123"),
							Timestamp:  btcjson.TimestampOrNow{Value: "now"},
						},
					},
				)
			},
			staticCmd: func() interface{} {
				requests := []btcjson.ImportMultiRequest{
					{Descriptor: btcjson.String("123"), Timestamp: btcjson.TimestampOrNow{Value: "now"}},
				}
				return btcjson.NewImportMultiCmd(requests, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importmulti","params":[[{"desc":"123","timestamp":"now"}]],"id":1}`,
			unmarshalled: &btcjson.ImportMultiCmd{
				Requests: []btcjson.ImportMultiRequest{
					{Descriptor: btcjson.String("123"), Timestamp: btcjson.TimestampOrNow{Value: "now"}},
				},
			},
		},
		{
			name: "importmulti with scriptPubKey script",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"importmulti",
					// Cannot use a native string, due to special types like timestamp and scriptPubKey
					[]btcjson.ImportMultiRequest{
						{
							ScriptPubKey: &btcjson.ScriptPubKey{Value: "script"},
							RedeemScript: btcjson.String("123"),
							Timestamp:    btcjson.TimestampOrNow{Value: 0},
							PubKeys:      &[]string{"aaa"},
						},
					},
				)
			},
			staticCmd: func() interface{} {
				requests := []btcjson.ImportMultiRequest{
					{
						ScriptPubKey: &btcjson.ScriptPubKey{Value: "script"},
						RedeemScript: btcjson.String("123"),
						Timestamp:    btcjson.TimestampOrNow{Value: 0},
						PubKeys:      &[]string{"aaa"},
					},
				}
				return btcjson.NewImportMultiCmd(requests, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importmulti","params":[[{"scriptPubKey":"script","timestamp":0,"redeemscript":"123","pubkeys":["aaa"]}]],"id":1}`,
			unmarshalled: &btcjson.ImportMultiCmd{
				Requests: []btcjson.ImportMultiRequest{
					{
						ScriptPubKey: &btcjson.ScriptPubKey{Value: "script"},
						RedeemScript: btcjson.String("123"),
						Timestamp:    btcjson.TimestampOrNow{Value: 0},
						PubKeys:      &[]string{"aaa"},
					},
				},
			},
		},
		{
			name: "importmulti with scriptPubKey address",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"importmulti",
					// Cannot use a native string, due to special types like timestamp and scriptPubKey
					[]btcjson.ImportMultiRequest{
						{
							ScriptPubKey:  &btcjson.ScriptPubKey{Value: btcjson.ScriptPubKeyAddress{Address: "addr"}},
							WitnessScript: btcjson.String("123"),
							Timestamp:     btcjson.TimestampOrNow{Value: 0},
							Keys:          &[]string{"aaa"},
						},
					},
				)
			},
			staticCmd: func() interface{} {
				requests := []btcjson.ImportMultiRequest{
					{
						ScriptPubKey:  &btcjson.ScriptPubKey{Value: btcjson.ScriptPubKeyAddress{Address: "addr"}},
						WitnessScript: btcjson.String("123"),
						Timestamp:     btcjson.TimestampOrNow{Value: 0},
						Keys:          &[]string{"aaa"},
					},
				}
				return btcjson.NewImportMultiCmd(requests, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importmulti","params":[[{"scriptPubKey":{"address":"addr"},"timestamp":0,"witnessscript":"123","keys":["aaa"]}]],"id":1}`,
			unmarshalled: &btcjson.ImportMultiCmd{
				Requests: []btcjson.ImportMultiRequest{
					{
						ScriptPubKey:  &btcjson.ScriptPubKey{Value: btcjson.ScriptPubKeyAddress{Address: "addr"}},
						WitnessScript: btcjson.String("123"),
						Timestamp:     btcjson.TimestampOrNow{Value: 0},
						Keys:          &[]string{"aaa"},
					},
				},
			},
		},
		{
			name: "importmulti with ranged (int) descriptor",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"importmulti",
					// Cannot use a native string, due to special types like timestamp.
					[]btcjson.ImportMultiRequest{
						{
							Descriptor: btcjson.String("123"),
							Timestamp:  btcjson.TimestampOrNow{Value: 0},
							Range:      &btcjson.DescriptorRange{Value: 7},
						},
					},
				)
			},
			staticCmd: func() interface{} {
				requests := []btcjson.ImportMultiRequest{
					{
						Descriptor: btcjson.String("123"),
						Timestamp:  btcjson.TimestampOrNow{Value: 0},
						Range:      &btcjson.DescriptorRange{Value: 7},
					},
				}
				return btcjson.NewImportMultiCmd(requests, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importmulti","params":[[{"desc":"123","timestamp":0,"range":7}]],"id":1}`,
			unmarshalled: &btcjson.ImportMultiCmd{
				Requests: []btcjson.ImportMultiRequest{
					{
						Descriptor: btcjson.String("123"),
						Timestamp:  btcjson.TimestampOrNow{Value: 0},
						Range:      &btcjson.DescriptorRange{Value: 7},
					},
				},
			},
		},
		{
			name: "importmulti with ranged (slice) descriptor",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"importmulti",
					// Cannot use a native string, due to special types like timestamp.
					[]btcjson.ImportMultiRequest{
						{
							Descriptor: btcjson.String("123"),
							Timestamp:  btcjson.TimestampOrNow{Value: 0},
							Range:      &btcjson.DescriptorRange{Value: []int{1, 7}},
						},
					},
				)
			},
			staticCmd: func() interface{} {
				requests := []btcjson.ImportMultiRequest{
					{
						Descriptor: btcjson.String("123"),
						Timestamp:  btcjson.TimestampOrNow{Value: 0},
						Range:      &btcjson.DescriptorRange{Value: []int{1, 7}},
					},
				}
				return btcjson.NewImportMultiCmd(requests, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"importmulti","params":[[{"desc":"123","timestamp":0,"range":[1,7]}]],"id":1}`,
			unmarshalled: &btcjson.ImportMultiCmd{
				Requests: []btcjson.ImportMultiRequest{
					{
						Descriptor: btcjson.String("123"),
						Timestamp:  btcjson.TimestampOrNow{Value: 0},
						Range:      &btcjson.DescriptorRange{Value: []int{1, 7}},
					},
				},
			},
		},
		{
			name: "walletcreatefundedpsbt",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"walletcreatefundedpsbt",
					[]btcjson.PsbtInput{
						{
							Txid:     "1234",
							Vout:     0,
							Sequence: 0,
						},
					},
					[]btcjson.PsbtOutput{
						btcjson.NewPsbtOutput("1234", btcutil.Amount(1234)),
						btcjson.NewPsbtDataOutput([]byte{1, 2, 3, 4}),
					},
					btcjson.Uint32(1),
					btcjson.WalletCreateFundedPsbtOpts{},
					btcjson.Bool(true),
				)
			},
			staticCmd: func() interface{} {
				return btcjson.NewWalletCreateFundedPsbtCmd(
					[]btcjson.PsbtInput{
						{
							Txid:     "1234",
							Vout:     0,
							Sequence: 0,
						},
					},
					[]btcjson.PsbtOutput{
						btcjson.NewPsbtOutput("1234", btcutil.Amount(1234)),
						btcjson.NewPsbtDataOutput([]byte{1, 2, 3, 4}),
					},
					btcjson.Uint32(1),
					&btcjson.WalletCreateFundedPsbtOpts{},
					btcjson.Bool(true),
				)
			},
			marshalled: `{"jsonrpc":"1.0","method":"walletcreatefundedpsbt","params":[[{"txid":"1234","vout":0,"sequence":0}],[{"1234":0.00001234},{"data":"01020304"}],1,{},true],"id":1}`,
			unmarshalled: &btcjson.WalletCreateFundedPsbtCmd{
				Inputs: []btcjson.PsbtInput{
					{
						Txid:     "1234",
						Vout:     0,
						Sequence: 0,
					},
				},
				Outputs: []btcjson.PsbtOutput{
					btcjson.NewPsbtOutput("1234", btcutil.Amount(1234)),
					btcjson.NewPsbtDataOutput([]byte{1, 2, 3, 4}),
				},
				Locktime:    btcjson.Uint32(1),
				Options:     &btcjson.WalletCreateFundedPsbtOpts{},
				Bip32Derivs: btcjson.Bool(true),
			},
		},
		{
			name: "walletprocesspsbt",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"walletprocesspsbt", "1234", btcjson.Bool(true), btcjson.String("ALL"), btcjson.Bool(true))
			},
			staticCmd: func() interface{} {
				return btcjson.NewWalletProcessPsbtCmd(
					"1234", btcjson.Bool(true), btcjson.String("ALL"), btcjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"walletprocesspsbt","params":["1234",true,"ALL",true],"id":1}`,
			unmarshalled: &btcjson.WalletProcessPsbtCmd{
				Psbt:        "1234",
				Sign:        btcjson.Bool(true),
				SighashType: btcjson.String("ALL"),
				Bip32Derivs: btcjson.Bool(true),
			},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Marshal the command as created by the new static command
		// creation function.
		marshalled, err := btcjson.MarshalCmd(btcjson.RpcVersion1, testID, test.staticCmd())
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
		marshalled, err = btcjson.MarshalCmd(btcjson.RpcVersion1, testID, cmd)
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

		var request btcjson.Request
		if err := json.Unmarshal(marshalled, &request); err != nil {
			t.Errorf("Test #%d (%s) unexpected error while "+
				"unmarshalling JSON-RPC request: %v", i,
				test.name, err)
			continue
		}

		cmd, err = btcjson.UnmarshalCmd(&request)
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
