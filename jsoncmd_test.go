// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// this has to be in the real json subpackage so we can mock up structs
package btcjson

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

var testID = float64(1)

var jsoncmdtests = []struct {
	name   string
	cmd    string
	f      func() (Cmd, error)
	result Cmd // after marshal and unmarshal
}{
	{
		name: "basic",
		cmd:  "addmultisigaddress",
		f: func() (Cmd, error) {
			return NewAddMultisigAddressCmd(testID, 1,
				[]string{"foo", "bar"})
		},
		result: &AddMultisigAddressCmd{
			id:        testID,
			NRequired: 1,
			Keys:      []string{"foo", "bar"},
			Account:   "",
		},
	},
	{
		name: "+ optional",
		cmd:  "addmultisigaddress",
		f: func() (Cmd, error) {
			return NewAddMultisigAddressCmd(testID, 1,
				[]string{"foo", "bar"}, "address")
		},
		result: &AddMultisigAddressCmd{
			id:        testID,
			NRequired: 1,
			Keys:      []string{"foo", "bar"},
			Account:   "address",
		},
	},
	// TODO(oga) Too many arguments to newaddmultisigaddress
	{
		name: "basic add",
		cmd:  "addnode",
		f: func() (Cmd, error) {
			return NewAddNodeCmd(testID, "address",
				"add")
		},
		result: &AddNodeCmd{
			id:     testID,
			Addr:   "address",
			SubCmd: "add",
		},
	},
	{
		name: "basic remove",
		cmd:  "addnode",
		f: func() (Cmd, error) {
			return NewAddNodeCmd(testID, "address",
				"remove")
		},
		result: &AddNodeCmd{
			id:     testID,
			Addr:   "address",
			SubCmd: "remove",
		},
	},
	{
		name: "basic onetry",
		cmd:  "addnode",
		f: func() (Cmd, error) {
			return NewAddNodeCmd(testID, "address",
				"onetry")
		},
		result: &AddNodeCmd{
			id:     testID,
			Addr:   "address",
			SubCmd: "onetry",
		},
	},
	// TODO(oga) try invalid subcmds
	{
		name: "basic",
		cmd:  "backupwallet",
		f: func() (Cmd, error) {
			return NewBackupWalletCmd(testID, "destination")
		},
		result: &BackupWalletCmd{
			id:          testID,
			Destination: "destination",
		},
	},
	{
		name: "basic",
		cmd:  "createmultisig",
		f: func() (Cmd, error) {
			return NewCreateMultisigCmd(testID, 1,
				[]string{"key1", "key2", "key3"})
		},
		result: &CreateMultisigCmd{
			id:        testID,
			NRequired: 1,
			Keys:      []string{"key1", "key2", "key3"},
		},
	},
	{
		name: "basic",
		cmd:  "createrawtransaction",
		f: func() (Cmd, error) {
			return NewCreateRawTransactionCmd(testID,
				[]TransactionInput{
					{Txid: "tx1", Vout: 1},
					{Txid: "tx2", Vout: 3}},
				map[string]int64{"bob": 1, "bill": 2})
		},
		result: &CreateRawTransactionCmd{
			id: testID,
			Inputs: []TransactionInput{
				{Txid: "tx1", Vout: 1},
				{Txid: "tx2", Vout: 3},
			},
			Amounts: map[string]int64{
				"bob":  1,
				"bill": 2,
			},
		},
	},
	{
		name: "basic",
		cmd:  "debuglevel",
		f: func() (Cmd, error) {
			return NewDebugLevelCmd(testID, "debug")
		},
		result: &DebugLevelCmd{
			id:        testID,
			LevelSpec: "debug",
		},
	},
	{
		name: "basic",
		cmd:  "decoderawtransaction",
		f: func() (Cmd, error) {
			return NewDecodeRawTransactionCmd(testID,
				"thisisahexidecimaltransaction")
		},
		result: &DecodeRawTransactionCmd{
			id:    testID,
			HexTx: "thisisahexidecimaltransaction",
		},
	},
	{
		name: "basic",
		cmd:  "decodescript",
		f: func() (Cmd, error) {
			return NewDecodeScriptCmd(testID,
				"a bunch of hex")
		},
		result: &DecodeScriptCmd{
			id:        testID,
			HexScript: "a bunch of hex",
		},
	},
	{
		name: "basic",
		cmd:  "dumpprivkey",
		f: func() (Cmd, error) {
			return NewDumpPrivKeyCmd(testID,
				"address")
		},
		result: &DumpPrivKeyCmd{
			id:      testID,
			Address: "address",
		},
	},
	{
		name: "basic",
		cmd:  "dumpwallet",
		f: func() (Cmd, error) {
			return NewDumpWalletCmd(testID,
				"filename")
		},
		result: &DumpWalletCmd{
			id:       testID,
			Filename: "filename",
		},
	},
	{
		name: "basic",
		cmd:  "encryptwallet",
		f: func() (Cmd, error) {
			return NewEncryptWalletCmd(testID,
				"passphrase")
		},
		result: &EncryptWalletCmd{
			id:         testID,
			Passphrase: "passphrase",
		},
	},
	{
		name: "basic",
		cmd:  "estimatefee",
		f: func() (Cmd, error) {
			return NewEstimateFeeCmd(testID, 1234)
		},
		result: &EstimateFeeCmd{
			id:        testID,
			NumBlocks: 1234,
		},
	},
	{
		name: "basic",
		cmd:  "estimatepriority",
		f: func() (Cmd, error) {
			return NewEstimatePriorityCmd(testID, 1234)
		},
		result: &EstimatePriorityCmd{
			id:        testID,
			NumBlocks: 1234,
		},
	},
	{
		name: "basic",
		cmd:  "getaccount",
		f: func() (Cmd, error) {
			return NewGetAccountCmd(testID,
				"address")
		},
		result: &GetAccountCmd{
			id:      testID,
			Address: "address",
		},
	},
	{
		name: "basic",
		cmd:  "getaccountaddress",
		f: func() (Cmd, error) {
			return NewGetAccountAddressCmd(testID,
				"account")
		},
		result: &GetAccountAddressCmd{
			id:      testID,
			Account: "account",
		},
	},
	{
		name: "basic true",
		cmd:  "getaddednodeinfo",
		f: func() (Cmd, error) {
			return NewGetAddedNodeInfoCmd(testID, true)
		},
		result: &GetAddedNodeInfoCmd{
			id:  testID,
			Dns: true,
		},
	},
	{
		name: "basic false",
		cmd:  "getaddednodeinfo",
		f: func() (Cmd, error) {
			return NewGetAddedNodeInfoCmd(testID, false)
		},
		result: &GetAddedNodeInfoCmd{
			id:  testID,
			Dns: false,
		},
	},
	{
		name: "basic withnode",
		cmd:  "getaddednodeinfo",
		f: func() (Cmd, error) {
			return NewGetAddedNodeInfoCmd(testID, true,
				"thisisanode")
		},
		result: &GetAddedNodeInfoCmd{
			id:   testID,
			Dns:  true,
			Node: "thisisanode",
		},
	},
	{
		name: "basic",
		cmd:  "getaddressesbyaccount",
		f: func() (Cmd, error) {
			return NewGetAddressesByAccountCmd(testID,
				"account")
		},
		result: &GetAddressesByAccountCmd{
			id:      testID,
			Account: "account",
		},
	},
	{
		name: "basic",
		cmd:  "getbalance",
		f: func() (Cmd, error) {
			return NewGetBalanceCmd(testID)
		},
		result: &GetBalanceCmd{
			id:      testID,
			MinConf: 1, // the default
		},
	},
	{
		name: "basic + account",
		cmd:  "getbalance",
		f: func() (Cmd, error) {
			return NewGetBalanceCmd(testID, "account")
		},
		result: &GetBalanceCmd{
			id:      testID,
			Account: "account",
			MinConf: 1, // the default
		},
	},
	{
		name: "basic + minconf",
		cmd:  "getbalance",
		f: func() (Cmd, error) {
			return NewGetBalanceCmd(testID, "", 2)
		},
		result: &GetBalanceCmd{
			id:      testID,
			MinConf: 2,
		},
	},
	{
		name: "basic + account + minconf",
		cmd:  "getbalance",
		f: func() (Cmd, error) {
			return NewGetBalanceCmd(testID, "account", 2)
		},
		result: &GetBalanceCmd{
			id:      testID,
			Account: "account",
			MinConf: 2,
		},
	},
	{
		name: "basic",
		cmd:  "getbestblockhash",
		f: func() (Cmd, error) {
			return NewGetBestBlockHashCmd(testID)
		},
		result: &GetBestBlockHashCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "getblock",
		f: func() (Cmd, error) {
			return NewGetBlockCmd(testID,
				"somehash")
		},
		result: &GetBlockCmd{
			id:      testID,
			Hash:    "somehash",
			Verbose: true,
		},
	},
	{
		name: "basic",
		cmd:  "getblockchaininfo",
		f: func() (Cmd, error) {
			return NewGetBlockChainInfoCmd(testID)
		},
		result: &GetBlockChainInfoCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "getblockcount",
		f: func() (Cmd, error) {
			return NewGetBlockCountCmd(testID)
		},
		result: &GetBlockCountCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "getblockhash",
		f: func() (Cmd, error) {
			return NewGetBlockHashCmd(testID, 1234)
		},
		result: &GetBlockHashCmd{
			id:    testID,
			Index: 1234,
		},
	},
	{
		name: "basic",
		cmd:  "getblocktemplate",
		f: func() (Cmd, error) {
			return NewGetBlockTemplateCmd(testID)
		},
		result: &GetBlockTemplateCmd{
			id: testID,
		},
	},
	{
		name: "basic + request",
		cmd:  "getblocktemplate",
		f: func() (Cmd, error) {
			return NewGetBlockTemplateCmd(testID,
				&TemplateRequest{Mode: "mode",
					Capabilities: []string{"one", "two", "three"}})
		},
		result: &GetBlockTemplateCmd{
			id: testID,
			Request: &TemplateRequest{
				Mode: "mode",
				Capabilities: []string{
					"one",
					"two",
					"three",
				},
			},
		},
	},
	{
		name: "basic + request no mode",
		cmd:  "getblocktemplate",
		f: func() (Cmd, error) {
			return NewGetBlockTemplateCmd(testID,
				&TemplateRequest{
					Capabilities: []string{"one", "two", "three"}})
		},
		result: &GetBlockTemplateCmd{
			id: testID,
			Request: &TemplateRequest{
				Capabilities: []string{
					"one",
					"two",
					"three",
				},
			},
		},
	},
	{
		name: "basic",
		cmd:  "getconnectioncount",
		f: func() (Cmd, error) {
			return NewGetConnectionCountCmd(testID)
		},
		result: &GetConnectionCountCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "getdifficulty",
		f: func() (Cmd, error) {
			return NewGetDifficultyCmd(testID)
		},
		result: &GetDifficultyCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "getgenerate",
		f: func() (Cmd, error) {
			return NewGetGenerateCmd(testID)
		},
		result: &GetGenerateCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "gethashespersec",
		f: func() (Cmd, error) {
			return NewGetHashesPerSecCmd(testID)
		},
		result: &GetHashesPerSecCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "getinfo",
		f: func() (Cmd, error) {
			return NewGetInfoCmd(testID)
		},
		result: &GetInfoCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "getmininginfo",
		f: func() (Cmd, error) {
			return NewGetMiningInfoCmd(testID)
		},
		result: &GetMiningInfoCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "getnettotals",
		f: func() (Cmd, error) {
			return NewGetNetTotalsCmd(testID)
		},
		result: &GetNetTotalsCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "getnetworkhashps",
		f: func() (Cmd, error) {
			return NewGetNetworkHashPSCmd(testID)
		},
		result: &GetNetworkHashPSCmd{
			id:     testID,
			Blocks: 120,
			Height: -1,
		},
	},
	{
		name: "basic + blocks",
		cmd:  "getnetworkhashps",
		f: func() (Cmd, error) {
			return NewGetNetworkHashPSCmd(testID, 5000)
		},
		result: &GetNetworkHashPSCmd{
			id:     testID,
			Blocks: 5000,
			Height: -1,
		},
	},
	{
		name: "basic + blocks + height",
		cmd:  "getnetworkhashps",
		f: func() (Cmd, error) {
			return NewGetNetworkHashPSCmd(testID, 5000, 1000)
		},
		result: &GetNetworkHashPSCmd{
			id:     testID,
			Blocks: 5000,
			Height: 1000,
		},
	},
	{
		name: "basic",
		cmd:  "getnetworkinfo",
		f: func() (Cmd, error) {
			return NewGetNetworkInfoCmd(testID)
		},
		result: &GetNetworkInfoCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "getnewaddress",
		f: func() (Cmd, error) {
			return NewGetNewAddressCmd(testID, "account")
		},
		result: &GetNewAddressCmd{
			id:      testID,
			Account: "account",
		},
	},
	{
		name: "basic",
		cmd:  "getpeerinfo",
		f: func() (Cmd, error) {
			return NewGetPeerInfoCmd(testID)
		},
		result: &GetPeerInfoCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "getrawchangeaddress",
		f: func() (Cmd, error) {
			return NewGetRawChangeAddressCmd(testID)
		},
		result: &GetRawChangeAddressCmd{
			id: testID,
		},
	},
	{
		name: "basic + account",
		cmd:  "getrawchangeaddress",
		f: func() (Cmd, error) {
			return NewGetRawChangeAddressCmd(testID,
				"accountname")
		},
		result: &GetRawChangeAddressCmd{
			id:      testID,
			Account: "accountname",
		},
	},
	{
		name: "basic",
		cmd:  "getrawmempool",
		f: func() (Cmd, error) {
			return NewGetRawMempoolCmd(testID)
		},
		result: &GetRawMempoolCmd{
			id: testID,
		},
	},
	{
		name: "basic noverbose",
		cmd:  "getrawmempool",
		f: func() (Cmd, error) {
			return NewGetRawMempoolCmd(testID, false)
		},
		result: &GetRawMempoolCmd{
			id: testID,
		},
	},
	{
		name: "basic verbose",
		cmd:  "getrawmempool",
		f: func() (Cmd, error) {
			return NewGetRawMempoolCmd(testID, true)
		},
		result: &GetRawMempoolCmd{
			id:      testID,
			Verbose: true,
		},
	},
	{
		name: "basic",
		cmd:  "getrawtransaction",
		f: func() (Cmd, error) {
			return NewGetRawTransactionCmd(testID,
				"sometxid")
		},
		result: &GetRawTransactionCmd{
			id:   testID,
			Txid: "sometxid",
		},
	},
	{
		name: "basic + verbose",
		cmd:  "getrawtransaction",
		f: func() (Cmd, error) {
			return NewGetRawTransactionCmd(testID,
				"sometxid",
				1)
		},
		result: &GetRawTransactionCmd{
			id:      testID,
			Txid:    "sometxid",
			Verbose: 1,
		},
	},
	{
		name: "basic",
		cmd:  "getreceivedbyaccount",
		f: func() (Cmd, error) {
			return NewGetReceivedByAccountCmd(testID,
				"abtcaccount",
				1)
		},
		result: &GetReceivedByAccountCmd{
			id:      testID,
			Account: "abtcaccount",
			MinConf: 1,
		},
	},
	{
		name: "basic + opt",
		cmd:  "getreceivedbyaccount",
		f: func() (Cmd, error) {
			return NewGetReceivedByAccountCmd(testID,
				"abtcaccount",
				2)
		},
		result: &GetReceivedByAccountCmd{
			id:      testID,
			Account: "abtcaccount",
			MinConf: 2,
		},
	},
	{
		name: "basic",
		cmd:  "getreceivedbyaddress",
		f: func() (Cmd, error) {
			return NewGetReceivedByAddressCmd(testID,
				"abtcaddress",
				1)
		},
		result: &GetReceivedByAddressCmd{
			id:      testID,
			Address: "abtcaddress",
			MinConf: 1,
		},
	},
	{
		name: "basic + opt",
		cmd:  "getreceivedbyaddress",
		f: func() (Cmd, error) {
			return NewGetReceivedByAddressCmd(testID,
				"abtcaddress",
				2)
		},
		result: &GetReceivedByAddressCmd{
			id:      testID,
			Address: "abtcaddress",
			MinConf: 2,
		},
	},
	{
		name: "basic",
		cmd:  "gettransaction",
		f: func() (Cmd, error) {
			return NewGetTransactionCmd(testID,
				"atxid")
		},
		result: &GetTransactionCmd{
			id:   testID,
			Txid: "atxid",
		},
	},
	{
		name: "basic",
		cmd:  "gettxout",
		f: func() (Cmd, error) {
			return NewGetTxOutCmd(testID,
				"sometx",
				10)
		},
		result: &GetTxOutCmd{
			id:             testID,
			Txid:           "sometx",
			Output:         10,
			IncludeMempool: true,
		},
	},
	{
		name: "basic + optional",
		cmd:  "gettxout",
		f: func() (Cmd, error) {
			return NewGetTxOutCmd(testID,
				"sometx",
				10,
				false)
		},
		result: &GetTxOutCmd{
			id:             testID,
			Txid:           "sometx",
			Output:         10,
			IncludeMempool: false,
		},
	},
	{
		name: "basic",
		cmd:  "gettxoutsetinfo",
		f: func() (Cmd, error) {
			return NewGetTxOutSetInfoCmd(testID)
		},
		result: &GetTxOutSetInfoCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "getwork",
		f: func() (Cmd, error) {
			return NewGetWorkCmd(testID, "some data")
		},
		result: &GetWorkCmd{
			id:   testID,
			Data: "some data",
		},
	},
	{
		name: "basic",
		cmd:  "help",
		f: func() (Cmd, error) {
			return NewHelpCmd(testID)
		},
		result: &HelpCmd{
			id: testID,
		},
	},
	{
		name: "basic + optional cmd",
		cmd:  "help",
		f: func() (Cmd, error) {
			return NewHelpCmd(testID,
				"getinfo")
		},
		result: &HelpCmd{
			id:      testID,
			Command: "getinfo",
		},
	},
	{
		name: "basic",
		cmd:  "importprivkey",
		f: func() (Cmd, error) {
			return NewImportPrivKeyCmd(testID,
				"somereallongprivatekey")
		},
		result: &ImportPrivKeyCmd{
			id:      testID,
			PrivKey: "somereallongprivatekey",
			Rescan:  true,
		},
	},
	{
		name: "basic + 1 opt",
		cmd:  "importprivkey",
		f: func() (Cmd, error) {
			return NewImportPrivKeyCmd(testID,
				"somereallongprivatekey",
				"some text")
		},
		result: &ImportPrivKeyCmd{
			id:      testID,
			PrivKey: "somereallongprivatekey",
			Label:   "some text",
			Rescan:  true,
		},
	},
	{
		name: "basic + 2 opts",
		cmd:  "importprivkey",
		f: func() (Cmd, error) {
			return NewImportPrivKeyCmd(testID,
				"somereallongprivatekey",
				"some text",
				false)
		},
		result: &ImportPrivKeyCmd{
			id:      testID,
			PrivKey: "somereallongprivatekey",
			Label:   "some text",
			Rescan:  false,
		},
	},
	{
		name: "basic",
		cmd:  "importwallet",
		f: func() (Cmd, error) {
			return NewImportWalletCmd(testID,
				"walletfilename.dat")
		},
		result: &ImportWalletCmd{
			id:       testID,
			Filename: "walletfilename.dat",
		},
	},
	{
		name: "basic",
		cmd:  "keypoolrefill",
		f: func() (Cmd, error) {
			return NewKeyPoolRefillCmd(testID)
		},
		result: &KeyPoolRefillCmd{
			id: testID,
		},
	},
	{
		name: "newsize",
		cmd:  "keypoolrefill",
		f: func() (Cmd, error) {
			return NewKeyPoolRefillCmd(testID, 1000000)
		},
		result: &KeyPoolRefillCmd{
			id:      testID,
			NewSize: 1000000,
		},
	},
	{
		name: "basic",
		cmd:  "listaccounts",
		f: func() (Cmd, error) {
			return NewListAccountsCmd(testID, 1)
		},
		result: &ListAccountsCmd{
			id:      testID,
			MinConf: 1,
		},
	},
	{
		name: "basic + opt",
		cmd:  "listaccounts",
		f: func() (Cmd, error) {
			return NewListAccountsCmd(testID, 2)
		},
		result: &ListAccountsCmd{
			id:      testID,
			MinConf: 2,
		},
	},
	{
		name: "basic",
		cmd:  "listaddressgroupings",
		f: func() (Cmd, error) {
			return NewListAddressGroupingsCmd(testID)
		},
		result: &ListAddressGroupingsCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "listlockunspent",
		f: func() (Cmd, error) {
			return NewListLockUnspentCmd(testID)
		},
		result: &ListLockUnspentCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "listreceivedbyaccount",
		f: func() (Cmd, error) {
			return NewListReceivedByAccountCmd(testID)
		},
		result: &ListReceivedByAccountCmd{
			id:      testID,
			MinConf: 1,
		},
	},
	{
		name: "basic + 1 opt",
		cmd:  "listreceivedbyaccount",
		f: func() (Cmd, error) {
			return NewListReceivedByAccountCmd(testID, 2)
		},
		result: &ListReceivedByAccountCmd{
			id:      testID,
			MinConf: 2,
		},
	},
	{
		name: "basic + 2 opt",
		cmd:  "listreceivedbyaccount",
		f: func() (Cmd, error) {
			return NewListReceivedByAccountCmd(testID, 2, true)
		},
		result: &ListReceivedByAccountCmd{
			id:           testID,
			MinConf:      2,
			IncludeEmpty: true,
		},
	},
	{
		name: "basic",
		cmd:  "listreceivedbyaddress",
		f: func() (Cmd, error) {
			return NewListReceivedByAddressCmd(testID)
		},
		result: &ListReceivedByAddressCmd{
			id:      testID,
			MinConf: 1,
		},
	},
	{
		name: "basic + 1 opt",
		cmd:  "listreceivedbyaddress",
		f: func() (Cmd, error) {
			return NewListReceivedByAddressCmd(testID, 2)
		},
		result: &ListReceivedByAddressCmd{
			id:      testID,
			MinConf: 2,
		},
	},
	{
		name: "basic + 2 opt",
		cmd:  "listreceivedbyaddress",
		f: func() (Cmd, error) {
			return NewListReceivedByAddressCmd(testID, 2, true)
		},
		result: &ListReceivedByAddressCmd{
			id:           testID,
			MinConf:      2,
			IncludeEmpty: true,
		},
	},
	{
		name: "basic",
		cmd:  "listsinceblock",
		f: func() (Cmd, error) {
			return NewListSinceBlockCmd(testID)
		},
		result: &ListSinceBlockCmd{
			id:                  testID,
			TargetConfirmations: 1,
		},
	},
	{
		name: "basic + 1 ops",
		cmd:  "listsinceblock",
		f: func() (Cmd, error) {
			return NewListSinceBlockCmd(testID, "someblockhash")
		},
		result: &ListSinceBlockCmd{
			id:                  testID,
			BlockHash:           "someblockhash",
			TargetConfirmations: 1,
		},
	},
	{
		name: "basic + 2 ops",
		cmd:  "listsinceblock",
		f: func() (Cmd, error) {
			return NewListSinceBlockCmd(testID, "someblockhash", 3)
		},
		result: &ListSinceBlockCmd{
			id:                  testID,
			BlockHash:           "someblockhash",
			TargetConfirmations: 3,
		},
	},
	{
		name: "basic",
		cmd:  "listtransactions",
		f: func() (Cmd, error) {
			return NewListTransactionsCmd(testID)
		},
		result: &ListTransactionsCmd{
			id:      testID,
			Account: "",
			Count:   10,
			From:    0,
		},
	},
	{
		name: "+ 1 optarg",
		cmd:  "listtransactions",
		f: func() (Cmd, error) {
			return NewListTransactionsCmd(testID, "abcde")
		},
		result: &ListTransactionsCmd{
			id:      testID,
			Account: "abcde",
			Count:   10,
			From:    0,
		},
	},
	{
		name: "+ 2 optargs",
		cmd:  "listtransactions",
		f: func() (Cmd, error) {
			return NewListTransactionsCmd(testID, "abcde", 123)
		},
		result: &ListTransactionsCmd{
			id:      testID,
			Account: "abcde",
			Count:   123,
			From:    0,
		},
	},
	{
		name: "+ 3 optargs",
		cmd:  "listtransactions",
		f: func() (Cmd, error) {
			return NewListTransactionsCmd(testID, "abcde", 123, 456)
		},
		result: &ListTransactionsCmd{
			id:      testID,
			Account: "abcde",
			Count:   123,
			From:    456,
		},
	},
	{
		name: "basic",
		cmd:  "listunspent",
		f: func() (Cmd, error) {
			return NewListUnspentCmd(testID)
		},
		result: &ListUnspentCmd{
			id:      testID,
			MinConf: 1,
			MaxConf: 999999,
		},
	},
	{
		name: "basic + opts",
		cmd:  "listunspent",
		f: func() (Cmd, error) {
			return NewListUnspentCmd(testID, 0, 6)
		},
		result: &ListUnspentCmd{
			id:      testID,
			MinConf: 0,
			MaxConf: 6,
		},
	},
	{
		name: "basic + opts + addresses",
		cmd:  "listunspent",
		f: func() (Cmd, error) {
			return NewListUnspentCmd(testID, 0, 6, []string{
				"a", "b", "c",
			})
		},
		result: &ListUnspentCmd{
			id:        testID,
			MinConf:   0,
			MaxConf:   6,
			Addresses: []string{"a", "b", "c"},
		},
	},
	{
		name: "basic",
		cmd:  "lockunspent",
		f: func() (Cmd, error) {
			return NewLockUnspentCmd(testID, true)
		},
		result: &LockUnspentCmd{
			id:     testID,
			Unlock: true,
		},
	},
	{
		name: "basic",
		cmd:  "move",
		f: func() (Cmd, error) {
			return NewMoveCmd(testID,
				"account1",
				"account2",
				12,
				1)
		},
		result: &MoveCmd{
			id:          testID,
			FromAccount: "account1",
			ToAccount:   "account2",
			Amount:      12,
			MinConf:     1, // the default
		},
	},
	{
		name: "basic + optionals",
		cmd:  "move",
		f: func() (Cmd, error) {
			return NewMoveCmd(testID,
				"account1",
				"account2",
				12,
				1,
				"some comment")
		},
		result: &MoveCmd{
			id:          testID,
			FromAccount: "account1",
			ToAccount:   "account2",
			Amount:      12,
			MinConf:     1, // the default
			Comment:     "some comment",
		},
	},
	{
		name: "basic",
		cmd:  "ping",
		f: func() (Cmd, error) {
			return NewPingCmd(testID)
		},
		result: &PingCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "sendfrom",
		f: func() (Cmd, error) {
			return NewSendFromCmd(testID,
				"account",
				"address",
				12,
				1)
		},
		result: &SendFromCmd{
			id:          testID,
			FromAccount: "account",
			ToAddress:   "address",
			Amount:      12,
			MinConf:     1, // the default
		},
	},
	{
		name: "basic + options",
		cmd:  "sendfrom",
		f: func() (Cmd, error) {
			return NewSendFromCmd(testID,
				"account",
				"address",
				12,
				1,
				"a comment",
				"comment to")
		},
		result: &SendFromCmd{
			id:          testID,
			FromAccount: "account",
			ToAddress:   "address",
			Amount:      12,
			MinConf:     1, // the default
			Comment:     "a comment",
			CommentTo:   "comment to",
		},
	},
	{
		name: "basic",
		cmd:  "sendmany",
		f: func() (Cmd, error) {
			pairs := map[string]int64{
				"address A": 1000,
				"address B": 2000,
				"address C": 3000,
			}
			return NewSendManyCmd(testID,
				"account",
				pairs)
		},
		result: &SendManyCmd{
			id:          testID,
			FromAccount: "account",
			Amounts: map[string]int64{
				"address A": 1000,
				"address B": 2000,
				"address C": 3000,
			},
			MinConf: 1, // the default
		},
	},
	{
		name: "+ options",
		cmd:  "sendmany",
		f: func() (Cmd, error) {
			pairs := map[string]int64{
				"address A": 1000,
				"address B": 2000,
				"address C": 3000,
			}
			return NewSendManyCmd(testID,
				"account",
				pairs,
				10,
				"comment")
		},
		result: &SendManyCmd{
			id:          testID,
			FromAccount: "account",
			Amounts: map[string]int64{
				"address A": 1000,
				"address B": 2000,
				"address C": 3000,
			},
			MinConf: 10,
			Comment: "comment",
		},
	},
	{
		name: "basic",
		cmd:  "sendrawtransaction",
		f: func() (Cmd, error) {
			return NewSendRawTransactionCmd(testID,
				"hexstringofatx")
		},
		result: &SendRawTransactionCmd{
			id:    testID,
			HexTx: "hexstringofatx",
		},
	},
	{
		name: "allowhighfees: false",
		cmd:  "sendrawtransaction",
		f: func() (Cmd, error) {
			return NewSendRawTransactionCmd(testID,
				"hexstringofatx", false)
		},
		result: &SendRawTransactionCmd{
			id:    testID,
			HexTx: "hexstringofatx",
		},
	},
	{
		name: "allowhighfees: true",
		cmd:  "sendrawtransaction",
		f: func() (Cmd, error) {
			return NewSendRawTransactionCmd(testID,
				"hexstringofatx", true)
		},
		result: &SendRawTransactionCmd{
			id:            testID,
			HexTx:         "hexstringofatx",
			AllowHighFees: true,
		},
	},
	{
		name: "basic",
		cmd:  "sendtoaddress",
		f: func() (Cmd, error) {
			return NewSendToAddressCmd(testID,
				"somebtcaddress",
				1)
		},
		result: &SendToAddressCmd{
			id:      testID,
			Address: "somebtcaddress",
			Amount:  1,
		},
	},
	{
		name: "basic + optional",
		cmd:  "sendtoaddress",
		f: func() (Cmd, error) {
			return NewSendToAddressCmd(testID,
				"somebtcaddress",
				1,
				"a comment",
				"comment to")
		},
		result: &SendToAddressCmd{
			id:        testID,
			Address:   "somebtcaddress",
			Amount:    1,
			Comment:   "a comment",
			CommentTo: "comment to",
		},
	},
	{
		name: "basic",
		cmd:  "setaccount",
		f: func() (Cmd, error) {
			return NewSetAccountCmd(testID,
				"somebtcaddress",
				"account name")
		},
		result: &SetAccountCmd{
			id:      testID,
			Address: "somebtcaddress",
			Account: "account name",
		},
	},
	{
		name: "basic",
		cmd:  "setgenerate",
		f: func() (Cmd, error) {
			return NewSetGenerateCmd(testID, true)
		},
		result: &SetGenerateCmd{
			id:           testID,
			Generate:     true,
			GenProcLimit: -1,
		},
	},
	{
		name: "basic + optional",
		cmd:  "setgenerate",
		f: func() (Cmd, error) {
			return NewSetGenerateCmd(testID, true, 10)
		},
		result: &SetGenerateCmd{
			id:           testID,
			Generate:     true,
			GenProcLimit: 10,
		},
	},
	{
		name: "basic",
		cmd:  "settxfee",
		f: func() (Cmd, error) {
			return NewSetTxFeeCmd(testID, 10)
		},
		result: &SetTxFeeCmd{
			id:     testID,
			Amount: 10,
		},
	},
	{
		name: "basic",
		cmd:  "signmessage",
		f: func() (Cmd, error) {
			return NewSignMessageCmd(testID,
				"btcaddress",
				"a message")
		},
		result: &SignMessageCmd{
			id:      testID,
			Address: "btcaddress",
			Message: "a message",
		},
	},
	{
		name: "basic",
		cmd:  "signrawtransaction",
		f: func() (Cmd, error) {
			return NewSignRawTransactionCmd(testID,
				"sometxstring")
		},
		result: &SignRawTransactionCmd{
			id:    testID,
			RawTx: "sometxstring",
		},
	},
	/*	{
		name: "basic + optional",
		cmd:  "signrawtransaction",
		f: func() (Cmd, error) {
			return NewSignRawTransactionCmd(testID,
				"sometxstring",
				[]RawTxInput{
					RawTxInput{
						Txid:         "test",
						Vout:         1,
						ScriptPubKey: "test",
						RedeemScript: "test",
					},
				},
				[]string{"aprivatekey", "privkey2"},
				"flags")
		},
		result: &SignRawTransactionCmd{
			id:    testID,
			RawTx: "sometxstring",
			Inputs: []RawTxInput{
				RawTxInput{
					Txid:         "test",
					Vout:         1,
					ScriptPubKey: "test",
					RedeemScript: "test",
				},
			},
			PrivKeys: []string{"aprivatekey", "privkey2"},
			Flags:    "flags",
		},
	},*/
	{
		name: "basic",
		cmd:  "stop",
		f: func() (Cmd, error) {
			return NewStopCmd(testID)
		},
		result: &StopCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "submitblock",
		f: func() (Cmd, error) {
			return NewSubmitBlockCmd(testID,
				"lotsofhex")
		},
		result: &SubmitBlockCmd{
			id:       testID,
			HexBlock: "lotsofhex",
		},
	},
	{
		name: "+ optional object",
		cmd:  "submitblock",
		f: func() (Cmd, error) {
			return NewSubmitBlockCmd(testID,
				"lotsofhex",
				&SubmitBlockOptions{WorkID: "otherstuff"})
		},
		result: &SubmitBlockCmd{
			id:       testID,
			HexBlock: "lotsofhex",
			Options:  &SubmitBlockOptions{WorkID: "otherstuff"},
		},
	},
	{
		name: "basic",
		cmd:  "validateaddress",
		f: func() (Cmd, error) {
			return NewValidateAddressCmd(testID,
				"somebtcaddress")
		},
		result: &ValidateAddressCmd{
			id:      testID,
			Address: "somebtcaddress",
		},
	},
	{
		name: "basic",
		cmd:  "verifychain",
		f: func() (Cmd, error) {
			return NewVerifyChainCmd(testID)
		},
		result: &VerifyChainCmd{
			id:         testID,
			CheckLevel: 3,
			CheckDepth: 288,
		},
	},
	{
		name: "basic + optional",
		cmd:  "verifychain",
		f: func() (Cmd, error) {
			return NewVerifyChainCmd(testID, 4, 1)
		},
		result: &VerifyChainCmd{
			id:         testID,
			CheckLevel: 4,
			CheckDepth: 1,
		},
	},
	{
		name: "basic",
		cmd:  "verifymessage",
		f: func() (Cmd, error) {
			return NewVerifyMessageCmd(testID,
				"someaddress",
				"somesig",
				"a message")
		},
		result: &VerifyMessageCmd{
			id:        testID,
			Address:   "someaddress",
			Signature: "somesig",
			Message:   "a message",
		},
	},
	{
		name: "basic",
		cmd:  "walletlock",
		f: func() (Cmd, error) {
			return NewWalletLockCmd(testID)
		},
		result: &WalletLockCmd{
			id: testID,
		},
	},
	{
		name: "basic",
		cmd:  "walletpassphrase",
		f: func() (Cmd, error) {
			return NewWalletPassphraseCmd(testID,
				"phrase1",
				10)
		},
		result: &WalletPassphraseCmd{
			id:         testID,
			Passphrase: "phrase1",
			Timeout:    10,
		},
	},
	{
		name: "basic",
		cmd:  "walletpassphrasechange",
		f: func() (Cmd, error) {
			return NewWalletPassphraseChangeCmd(testID,
				"phrase1", "phrase2")
		},
		result: &WalletPassphraseChangeCmd{
			id:            testID,
			OldPassphrase: "phrase1",
			NewPassphrase: "phrase2",
		},
	},
}

func TestCmds(t *testing.T) {
	for _, test := range jsoncmdtests {
		c, err := test.f()
		name := test.cmd + " " + test.name
		if err != nil {
			t.Errorf("%s: failed to run func: %v",
				name, err)
			continue
		}

		msg, err := json.Marshal(c)
		if err != nil {
			t.Errorf("%s: failed to marshal cmd: %v",
				name, err)
			continue
		}

		c2, err := ParseMarshaledCmd(msg)
		if err != nil {
			t.Errorf("%s: failed to ummarshal cmd: %v",
				name, err)
			continue
		}

		id, ok := (c.Id()).(float64)
		if !ok || id != testID {
			t.Errorf("%s: id not returned properly", name)
		}

		if c.Method() != test.cmd {
			t.Errorf("%s: Method name not returned properly: "+
				"%s vs. %s", name, c.Method(), test.cmd)
		}

		if !reflect.DeepEqual(test.result, c2) {
			t.Errorf("%s: unmarshal not as expected. "+
				"got %v wanted %v", name, spew.Sdump(c2),
				spew.Sdump(test.result))
		}
		if !reflect.DeepEqual(c, c2) {
			t.Errorf("%s: unmarshal not as we started with. "+
				"got %v wanted %v", name, spew.Sdump(c2),
				spew.Sdump(c))
		}
	}
}

func TestHelps(t *testing.T) {
	helpTests := []string{
		"addmultisigaddress",
		"addnode",
		"backupwallet",
		"createmultisig",
		"createrawtransaction",
		"debuglevel",
		"decoderawtransaction",
		"decodescript",
		"dumpprivkey",
		"dumpwallet",
		"encryptwallet",
		"estimatefee",
		"estimatepriority",
		"getaccount",
		"getaccountaddress",
		"getaddednodeinfo",
		"getaddressesbyaccount",
		"getbalance",
		"getbestblockhash",
		"getblock",
		"getblockcount",
		"getblockhash",
		"getblocktemplate",
		"getconnectioncount",
		"getdifficulty",
		"getgenerate",
		"gethashespersec",
		"getinfo",
		"getmininginfo",
		"getnettotals",
		"getnetworkhashps",
		"getnewaddress",
		"getpeerinfo",
		"getrawchangeaddress",
		"getrawmempool",
		"getrawtransaction",
		"getreceivedbyaccount",
		"getreceivedbyaddress",
		"gettransaction",
		"gettxout",
		"gettxoutsetinfo",
		"getwork",
		"help",
		"importprivkey",
		"importwallet",
		"keypoolrefill",
		"listaccounts",
		"listaddressgroupings",
		"listlockunspent",
		"listreceivedbyaccount",
		"listreceivedbyaddress",
		"listsinceblock",
		"listtransactions",
		"listunspent",
		"lockunspent",
		"move",
		"ping",
		"sendfrom",
		"sendmany",
		"sendrawtransaction",
		"sendtoaddress",
		"setaccount",
		"setgenerate",
		"settxfee",
		"signmessage",
		"signrawtransaction",
		"stop",
		"submitblock",
		"validateaddress",
		"verifychain",
		"verifymessage",
		"walletlock",
		"walletpassphrase",
		"walletpassphrasechange",
	}

	for _, test := range helpTests {
		helpStr, err := GetHelpString(test)
		if err != nil {
			t.Errorf("%s: failed to get help string: %v",
				test, err)
			continue
		}

		if !strings.HasPrefix(helpStr, test) {
			t.Errorf("%s: help string doesn't begin with command "+
				"name: \"%s\"", test, helpStr)
			continue
		}
	}
}
