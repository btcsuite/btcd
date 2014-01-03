// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// this has to be in the real json subpackage so we can mock up structs
package btcjson

import (
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"reflect"
	"testing"
)

var testId = float64(1)

var jsoncmdtests = []struct {
	name   string
	f      func() (Cmd, error)
	result Cmd // after marshal and unmarshal
}{
	{
		name: "basic addmultisigaddress",
		f: func() (Cmd, error) {
			return NewAddMultisigAddressCmd(testId, 1,
				[]string{"foo", "bar"})
		},
		result: &AddMultisigAddressCmd{
			id:        testId,
			NRequired: 1,
			Keys:      []string{"foo", "bar"},
			Account:   "",
		},
	},
	{
		name: "addmultisigaddress + optional",
		f: func() (Cmd, error) {
			return NewAddMultisigAddressCmd(testId, 1,
				[]string{"foo", "bar"}, "address")
		},
		result: &AddMultisigAddressCmd{
			id:        testId,
			NRequired: 1,
			Keys:      []string{"foo", "bar"},
			Account:   "address",
		},
	},
	// TODO(oga) Too many arguments to newaddmultisigaddress
	{
		name: "basic addnode add",
		f: func() (Cmd, error) {
			return NewAddNodeCmd(testId, "address",
				"add")
		},
		result: &AddNodeCmd{
			id:     testId,
			Addr:   "address",
			SubCmd: "add",
		},
	},
	{
		name: "basic addnode remove",
		f: func() (Cmd, error) {
			return NewAddNodeCmd(testId, "address",
				"remove")
		},
		result: &AddNodeCmd{
			id:     testId,
			Addr:   "address",
			SubCmd: "remove",
		},
	},
	{
		name: "basic addnode onetry",
		f: func() (Cmd, error) {
			return NewAddNodeCmd(testId, "address",
				"onetry")
		},
		result: &AddNodeCmd{
			id:     testId,
			Addr:   "address",
			SubCmd: "onetry",
		},
	},
	// TODO(oga) try invalid subcmds
	{
		name: "basic backupwallet",
		f: func() (Cmd, error) {
			return NewBackupWalletCmd(testId, "destination")
		},
		result: &BackupWalletCmd{
			id:          testId,
			Destination: "destination",
		},
	},
	{
		name: "basic createmultisig",
		f: func() (Cmd, error) {
			return NewCreateMultisigCmd(testId, 1,
				[]string{"key1", "key2", "key3"})
		},
		result: &CreateMultisigCmd{
			id:        testId,
			NRequired: 1,
			Keys:      []string{"key1", "key2", "key3"},
		},
	},
	{
		name: "basic createrawtransaction",
		f: func() (Cmd, error) {
			return NewCreateRawTransactionCmd(testId,
				[]TransactionInput{
					TransactionInput{Txid: "tx1", Vout: 1},
					TransactionInput{Txid: "tx2", Vout: 3}},
				map[string]int64{"bob": 1, "bill": 2})
		},
		result: &CreateRawTransactionCmd{
			id: testId,
			Inputs: []TransactionInput{
				TransactionInput{Txid: "tx1", Vout: 1},
				TransactionInput{Txid: "tx2", Vout: 3},
			},
			Amounts: map[string]int64{
				"bob":  1,
				"bill": 2,
			},
		},
	},
	{
		name: "basic debuglevel",
		f: func() (Cmd, error) {
			return NewDebugLevelCmd(testId, "debug")
		},
		result: &DebugLevelCmd{
			id:        testId,
			LevelSpec: "debug",
		},
	},
	{
		name: "basic decoderawtransaction",
		f: func() (Cmd, error) {
			return NewDecodeRawTransactionCmd(testId,
				"thisisahexidecimaltransaction")
		},
		result: &DecodeRawTransactionCmd{
			id:    testId,
			HexTx: "thisisahexidecimaltransaction",
		},
	},
	{
		name: "basic decodescript",
		f: func() (Cmd, error) {
			return NewDecodeScriptCmd(testId,
				"a bunch of hex")
		},
		result: &DecodeScriptCmd{
			id:        testId,
			HexScript: "a bunch of hex",
		},
	},
	{
		name: "basic dumpprivkey",
		f: func() (Cmd, error) {
			return NewDumpPrivKeyCmd(testId,
				"address")
		},
		result: &DumpPrivKeyCmd{
			id:      testId,
			Address: "address",
		},
	},
	{
		name: "basic dumpwallet",
		f: func() (Cmd, error) {
			return NewDumpWalletCmd(testId,
				"filename")
		},
		result: &DumpWalletCmd{
			id:       testId,
			Filename: "filename",
		},
	},
	{
		name: "basic encryptwallet",
		f: func() (Cmd, error) {
			return NewEncryptWalletCmd(testId,
				"passphrase")
		},
		result: &EncryptWalletCmd{
			id:         testId,
			Passphrase: "passphrase",
		},
	},
	{
		name: "basic getaccount",
		f: func() (Cmd, error) {
			return NewGetAccountCmd(testId,
				"address")
		},
		result: &GetAccountCmd{
			id:      testId,
			Address: "address",
		},
	},
	{
		name: "basic getaccountaddress",
		f: func() (Cmd, error) {
			return NewGetAccountAddressCmd(testId,
				"account")
		},
		result: &GetAccountAddressCmd{
			id:      testId,
			Account: "account",
		},
	},
	{
		name: "basic getaddednodeinfo true",
		f: func() (Cmd, error) {
			return NewGetAddedNodeInfoCmd(testId, true)
		},
		result: &GetAddedNodeInfoCmd{
			id:  testId,
			Dns: true,
		},
	},
	{
		name: "basic getaddednodeinfo false",
		f: func() (Cmd, error) {
			return NewGetAddedNodeInfoCmd(testId, false)
		},
		result: &GetAddedNodeInfoCmd{
			id:  testId,
			Dns: false,
		},
	},
	{
		name: "basic getaddednodeinfo withnode",
		f: func() (Cmd, error) {
			return NewGetAddedNodeInfoCmd(testId, true,
				"thisisanode")
		},
		result: &GetAddedNodeInfoCmd{
			id:   testId,
			Dns:  true,
			Node: "thisisanode",
		},
	},
	{
		name: "basic getaddressesbyaccount",
		f: func() (Cmd, error) {
			return NewGetAddressesByAccountCmd(testId,
				"account")
		},
		result: &GetAddressesByAccountCmd{
			id:      testId,
			Account: "account",
		},
	},
	{
		name: "basic getbalance",
		f: func() (Cmd, error) {
			return NewGetBalanceCmd(testId)
		},
		result: &GetBalanceCmd{
			id:      testId,
			MinConf: 1, // the default
		},
	},
	{
		name: "basic getbalance + account",
		f: func() (Cmd, error) {
			return NewGetBalanceCmd(testId, "account")
		},
		result: &GetBalanceCmd{
			id:      testId,
			Account: "account",
			MinConf: 1, // the default
		},
	},
	{
		name: "basic getbalance + minconf",
		f: func() (Cmd, error) {
			return NewGetBalanceCmd(testId, "", 2)
		},
		result: &GetBalanceCmd{
			id:      testId,
			MinConf: 2,
		},
	},
	{
		name: "basic getbalance + account + minconf",
		f: func() (Cmd, error) {
			return NewGetBalanceCmd(testId, "account", 2)
		},
		result: &GetBalanceCmd{
			id:      testId,
			Account: "account",
			MinConf: 2,
		},
	},
	{
		name: "basic getbestblockhash",
		f: func() (Cmd, error) {
			return NewGetBestBlockHashCmd(testId)
		},
		result: &GetBestBlockHashCmd{
			id: testId,
		},
	},
	{
		name: "basic getblock",
		f: func() (Cmd, error) {
			return NewGetBlockCmd(testId,
				"somehash")
		},
		result: &GetBlockCmd{
			id:      testId,
			Hash:    "somehash",
			Verbose: true,
		},
	},
	{
		name: "basic getblockcount",
		f: func() (Cmd, error) {
			return NewGetBlockCountCmd(testId)
		},
		result: &GetBlockCountCmd{
			id: testId,
		},
	},
	{
		name: "basic getblockhash",
		f: func() (Cmd, error) {
			return NewGetBlockHashCmd(testId, 1234)
		},
		result: &GetBlockHashCmd{
			id:    testId,
			Index: 1234,
		},
	},
	{
		name: "basic getblocktemplate",
		f: func() (Cmd, error) {
			return NewGetBlockTemplateCmd(testId)
		},
		result: &GetBlockTemplateCmd{
			id: testId,
		},
	},
	{
		name: "basic getblocktemplate + request",
		f: func() (Cmd, error) {
			return NewGetBlockTemplateCmd(testId,
				&TemplateRequest{Mode: "mode",
					Capabilities: []string{"one", "two", "three"}})
		},
		result: &GetBlockTemplateCmd{
			id: testId,
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
		name: "basic getblocktemplate + request no mode",
		f: func() (Cmd, error) {
			return NewGetBlockTemplateCmd(testId,
				&TemplateRequest{
					Capabilities: []string{"one", "two", "three"}})
		},
		result: &GetBlockTemplateCmd{
			id: testId,
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
		name: "basic getconectioncount",
		f: func() (Cmd, error) {
			return NewGetConnectionCountCmd(testId)
		},
		result: &GetConnectionCountCmd{
			id: testId,
		},
	},
	{
		name: "basic getdifficulty",
		f: func() (Cmd, error) {
			return NewGetDifficultyCmd(testId)
		},
		result: &GetDifficultyCmd{
			id: testId,
		},
	},
	{
		name: "basic getgeneratecmd",
		f: func() (Cmd, error) {
			return NewGetGenerateCmd(testId)
		},
		result: &GetGenerateCmd{
			id: testId,
		},
	},
	{
		name: "basic gethashespersec",
		f: func() (Cmd, error) {
			return NewGetHashesPerSecCmd(testId)
		},
		result: &GetHashesPerSecCmd{
			id: testId,
		},
	},
	{
		name: "basic getinfo",
		f: func() (Cmd, error) {
			return NewGetInfoCmd(testId)
		},
		result: &GetInfoCmd{
			id: testId,
		},
	},
	{
		name: "basic getinfo",
		f: func() (Cmd, error) {
			return NewGetInfoCmd(testId)
		},
		result: &GetInfoCmd{
			id: testId,
		},
	},
	{
		name: "basic getmininginfo",
		f: func() (Cmd, error) {
			return NewGetMiningInfoCmd(testId)
		},
		result: &GetMiningInfoCmd{
			id: testId,
		},
	},
	{
		name: "basic getnettotals",
		f: func() (Cmd, error) {
			return NewGetNetTotalsCmd(testId)
		},
		result: &GetNetTotalsCmd{
			id: testId,
		},
	},
	{
		name: "basic getnetworkhashps",
		f: func() (Cmd, error) {
			return NewGetNetworkHashPSCmd(testId)
		},
		result: &GetNetworkHashPSCmd{
			id:     testId,
			Blocks: 120,
			Height: -1,
		},
	},
	{
		name: "basic getnetworkhashps + blocks",
		f: func() (Cmd, error) {
			return NewGetNetworkHashPSCmd(testId, 5000)
		},
		result: &GetNetworkHashPSCmd{
			id:     testId,
			Blocks: 5000,
			Height: -1,
		},
	},
	{
		name: "basic getnetworkhashps + blocks + height",
		f: func() (Cmd, error) {
			return NewGetNetworkHashPSCmd(testId, 5000, 1000)
		},
		result: &GetNetworkHashPSCmd{
			id:     testId,
			Blocks: 5000,
			Height: 1000,
		},
	},
	{
		name: "basic getnewaddress",
		f: func() (Cmd, error) {
			return NewGetNewAddressCmd(testId, "account")
		},
		result: &GetNewAddressCmd{
			id:      testId,
			Account: "account",
		},
	},
	{
		name: "basic getpeerinfo",
		f: func() (Cmd, error) {
			return NewGetPeerInfoCmd(testId)
		},
		result: &GetPeerInfoCmd{
			id: testId,
		},
	},
	{
		name: "basic getrawmchangeaddress",
		f: func() (Cmd, error) {
			return NewGetRawChangeAddressCmd(testId)
		},
		result: &GetRawChangeAddressCmd{
			id: testId,
		},
	},
	{
		name: "basic getrawmchangeaddress + account",
		f: func() (Cmd, error) {
			return NewGetRawChangeAddressCmd(testId,
				"accountname")
		},
		result: &GetRawChangeAddressCmd{
			id:      testId,
			Account: "accountname",
		},
	},
	{
		name: "basic getrawmempool",
		f: func() (Cmd, error) {
			return NewGetRawMempoolCmd(testId)
		},
		result: &GetRawMempoolCmd{
			id: testId,
		},
	},
	{
		name: "basic getrawmempool noverbose",
		f: func() (Cmd, error) {
			return NewGetRawMempoolCmd(testId, false)
		},
		result: &GetRawMempoolCmd{
			id: testId,
		},
	},
	{
		name: "basic getrawmempool verbose",
		f: func() (Cmd, error) {
			return NewGetRawMempoolCmd(testId, true)
		},
		result: &GetRawMempoolCmd{
			id:      testId,
			Verbose: true,
		},
	},
	{
		name: "basic getrawtransaction",
		f: func() (Cmd, error) {
			return NewGetRawTransactionCmd(testId,
				"sometxid")
		},
		result: &GetRawTransactionCmd{
			id:   testId,
			Txid: "sometxid",
		},
	},
	{
		name: "basic getrawtransaction + verbose",
		f: func() (Cmd, error) {
			return NewGetRawTransactionCmd(testId,
				"sometxid",
				true)
		},
		result: &GetRawTransactionCmd{
			id:      testId,
			Txid:    "sometxid",
			Verbose: true,
		},
	},
	{
		name: "basic getreceivedbyaccount",
		f: func() (Cmd, error) {
			return NewGetReceivedByAccountCmd(testId,
				"abtcaccount",
				1)
		},
		result: &GetReceivedByAccountCmd{
			id:      testId,
			Account: "abtcaccount",
			MinConf: 1,
		},
	},
	{
		name: "basic getreceivedbyaddress",
		f: func() (Cmd, error) {
			return NewGetReceivedByAddressCmd(testId,
				"abtcaddress",
				1)
		},
		result: &GetReceivedByAddressCmd{
			id:      testId,
			Address: "abtcaddress",
			MinConf: 1,
		},
	},
	{
		name: "basic gettransaction",
		f: func() (Cmd, error) {
			return NewGetTransactionCmd(testId,
				"atxid")
		},
		result: &GetTransactionCmd{
			id:   testId,
			Txid: "atxid",
		},
	},
	{
		name: "basic gettxout",
		f: func() (Cmd, error) {
			return NewGetTxOutCmd(testId,
				"sometx",
				10)
		},
		result: &GetTxOutCmd{
			id:     testId,
			Txid:   "sometx",
			Output: 10,
		},
	},
	{
		name: "basic gettxout + optional",
		f: func() (Cmd, error) {
			return NewGetTxOutCmd(testId,
				"sometx",
				10,
				false)
		},
		result: &GetTxOutCmd{
			id:             testId,
			Txid:           "sometx",
			Output:         10,
			IncludeMempool: false,
		},
	},
	{
		name: "basic gettxsetoutinfo",
		f: func() (Cmd, error) {
			return NewGetTxOutSetInfoCmd(testId)
		},
		result: &GetTxOutSetInfoCmd{
			id: testId,
		},
	},
	{
		name: "basic getwork",
		f: func() (Cmd, error) {
			return NewGetWorkCmd(testId,
				WorkRequest{
					Data:      "some data",
					Target:    "our target",
					Algorithm: "algo",
				})
		},
		result: &GetWorkCmd{
			id: testId,
			Request: WorkRequest{
				Data:      "some data",
				Target:    "our target",
				Algorithm: "algo",
			},
		},
	},
	{
		name: "basic help",
		f: func() (Cmd, error) {
			return NewHelpCmd(testId)
		},
		result: &HelpCmd{
			id: testId,
		},
	},
	{
		name: "basic help + optional cmd",
		f: func() (Cmd, error) {
			return NewHelpCmd(testId,
				"getinfo")
		},
		result: &HelpCmd{
			id:      testId,
			Command: "getinfo",
		},
	},
	{
		name: "basic importprivkey",
		f: func() (Cmd, error) {
			return NewImportPrivKeyCmd(testId,
				"somereallongprivatekey")
		},
		result: &ImportPrivKeyCmd{
			id:      testId,
			PrivKey: "somereallongprivatekey",
			Rescan:  true,
		},
	},
	{
		name: "basic importprivkey + 1 opt",
		f: func() (Cmd, error) {
			return NewImportPrivKeyCmd(testId,
				"somereallongprivatekey",
				"some text")
		},
		result: &ImportPrivKeyCmd{
			id:      testId,
			PrivKey: "somereallongprivatekey",
			Label:   "some text",
			Rescan:  true,
		},
	},
	{
		name: "basic importprivkey + 2 opts",
		f: func() (Cmd, error) {
			return NewImportPrivKeyCmd(testId,
				"somereallongprivatekey",
				"some text",
				false)
		},
		result: &ImportPrivKeyCmd{
			id:      testId,
			PrivKey: "somereallongprivatekey",
			Label:   "some text",
			Rescan:  false,
		},
	},
	{
		name: "basic importwallet",
		f: func() (Cmd, error) {
			return NewImportWalletCmd(testId,
				"walletfilename.dat")
		},
		result: &ImportWalletCmd{
			id:       testId,
			Filename: "walletfilename.dat",
		},
	},
	{
		name: "basic keypoolrefill",
		f: func() (Cmd, error) {
			return NewKeyPoolRefillCmd(testId)
		},
		result: &KeyPoolRefillCmd{
			id: testId,
		},
	},
	{
		name: "basic listaccounts",
		f: func() (Cmd, error) {
			return NewListAccountsCmd(testId, 1)
		},
		result: &ListAccountsCmd{
			id:      testId,
			MinConf: 1,
		},
	},
	{
		name: "basic listaddressgroupings",
		f: func() (Cmd, error) {
			return NewListAddressGroupingsCmd(testId)
		},
		result: &ListAddressGroupingsCmd{
			id: testId,
		},
	},
	{
		name: "basic listlockunspent",
		f: func() (Cmd, error) {
			return NewListLockUnspentCmd(testId)
		},
		result: &ListLockUnspentCmd{
			id: testId,
		},
	},
	{
		name: "basic listreceivedbyaccount",
		f: func() (Cmd, error) {
			return NewListReceivedByAccountCmd(testId)
		},
		result: &ListReceivedByAccountCmd{
			id:      testId,
			MinConf: 1,
		},
	},
	{
		name: "basic listtransactions",
		f: func() (Cmd, error) {
			return NewListTransactionsCmd(testId)
		},
		result: &ListTransactionsCmd{
			id:      testId,
			Account: "",
			Count:   10,
			From:    0,
		},
	},
	{
		name: "listtransactions 1 optarg",
		f: func() (Cmd, error) {
			return NewListTransactionsCmd(testId, "abcde")
		},
		result: &ListTransactionsCmd{
			id:      testId,
			Account: "abcde",
			Count:   10,
			From:    0,
		},
	},
	{
		name: "listtransactions 2 optargs",
		f: func() (Cmd, error) {
			return NewListTransactionsCmd(testId, "abcde", 123)
		},
		result: &ListTransactionsCmd{
			id:      testId,
			Account: "abcde",
			Count:   123,
			From:    0,
		},
	},
	{
		name: "listtransactions 3 optargs",
		f: func() (Cmd, error) {
			return NewListTransactionsCmd(testId, "abcde", 123, 456)
		},
		result: &ListTransactionsCmd{
			id:      testId,
			Account: "abcde",
			Count:   123,
			From:    456,
		},
	},
	{
		name: "basic listunspent",
		f: func() (Cmd, error) {
			return NewListUnspentCmd(testId)
		},
		result: &ListUnspentCmd{
			id:      testId,
			MinConf: 1,
			MaxConf: 999999,
		},
	},
	{
		name: "basic listunspent + opts",
		f: func() (Cmd, error) {
			return NewListUnspentCmd(testId, 0, 6)
		},
		result: &ListUnspentCmd{
			id:      testId,
			MinConf: 0,
			MaxConf: 6,
		},
	},
	{
		name: "basic lockunspent",
		f: func() (Cmd, error) {
			return NewLockUnspentCmd(testId, true)
		},
		result: &LockUnspentCmd{
			id:     testId,
			Unlock: true,
		},
	},
	{
		name: "basic move",
		f: func() (Cmd, error) {
			return NewMoveCmd(testId,
				"account1",
				"account2",
				12,
				1)
		},
		result: &MoveCmd{
			id:          testId,
			FromAccount: "account1",
			ToAccount:   "account2",
			Amount:      12,
			MinConf:     1, // the default
		},
	},
	{
		name: "basic move + optionals",
		f: func() (Cmd, error) {
			return NewMoveCmd(testId,
				"account1",
				"account2",
				12,
				1,
				"some comment")
		},
		result: &MoveCmd{
			id:          testId,
			FromAccount: "account1",
			ToAccount:   "account2",
			Amount:      12,
			MinConf:     1, // the default
			Comment:     "some comment",
		},
	},
	{
		name: "basic ping",
		f: func() (Cmd, error) {
			return NewPingCmd(testId)
		},
		result: &PingCmd{
			id: testId,
		},
	},
	{
		name: "basic sendfrom",
		f: func() (Cmd, error) {
			return NewSendFromCmd(testId,
				"account",
				"address",
				12,
				1)
		},
		result: &SendFromCmd{
			id:          testId,
			FromAccount: "account",
			ToAddress:   "address",
			Amount:      12,
			MinConf:     1, // the default
		},
	},
	{
		name: "basic sendfrom + options",
		f: func() (Cmd, error) {
			return NewSendFromCmd(testId,
				"account",
				"address",
				12,
				1,
				"a comment",
				"comment to")
		},
		result: &SendFromCmd{
			id:          testId,
			FromAccount: "account",
			ToAddress:   "address",
			Amount:      12,
			MinConf:     1, // the default
			Comment:     "a comment",
			CommentTo:   "comment to",
		},
	},
	{
		name: "basic sendmany",
		f: func() (Cmd, error) {
			pairs := map[string]int64{
				"address A": 1000,
				"address B": 2000,
				"address C": 3000,
			}
			return NewSendManyCmd(testId,
				"account",
				pairs)
		},
		result: &SendManyCmd{
			id:          testId,
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
		name: "sendmany + options",
		f: func() (Cmd, error) {
			pairs := map[string]int64{
				"address A": 1000,
				"address B": 2000,
				"address C": 3000,
			}
			return NewSendManyCmd(testId,
				"account",
				pairs,
				10,
				"comment")
		},
		result: &SendManyCmd{
			id:          testId,
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
		name: "basic sendrawtransaction",
		f: func() (Cmd, error) {
			return NewSendRawTransactionCmd(testId,
				"hexstringofatx")
		},
		result: &SendRawTransactionCmd{
			id:    testId,
			HexTx: "hexstringofatx",
		},
	},
	{
		name: "basic sendtoaddress",
		f: func() (Cmd, error) {
			return NewSendToAddressCmd(testId,
				"somebtcaddress",
				1)
		},
		result: &SendToAddressCmd{
			id:      testId,
			Address: "somebtcaddress",
			Amount:  1,
		},
	},
	{
		name: "basic sendtoaddress plus optional",
		f: func() (Cmd, error) {
			return NewSendToAddressCmd(testId,
				"somebtcaddress",
				1,
				"a comment",
				"comment to")
		},
		result: &SendToAddressCmd{
			id:        testId,
			Address:   "somebtcaddress",
			Amount:    1,
			Comment:   "a comment",
			CommentTo: "comment to",
		},
	},
	{
		name: "basic setaccount",
		f: func() (Cmd, error) {
			return NewSetAccountCmd(testId,
				"somebtcaddress",
				"account name")
		},
		result: &SetAccountCmd{
			id:      testId,
			Address: "somebtcaddress",
			Account: "account name",
		},
	},
	{
		name: "basic setgenerate",
		f: func() (Cmd, error) {
			return NewSetGenerateCmd(testId, true)
		},
		result: &SetGenerateCmd{
			id:       testId,
			Generate: true,
		},
	},
	{
		name: "basic setgenerate + optional",
		f: func() (Cmd, error) {
			return NewSetGenerateCmd(testId, true, 10)
		},
		result: &SetGenerateCmd{
			id:           testId,
			Generate:     true,
			GenProcLimit: 10,
		},
	},
	{
		name: "basic settxfee",
		f: func() (Cmd, error) {
			return NewSetTxFeeCmd(testId, 10)
		},
		result: &SetTxFeeCmd{
			id:     testId,
			Amount: 10,
		},
	},
	{
		name: "basic signmessage",
		f: func() (Cmd, error) {
			return NewSignMessageCmd(testId,
				"btcaddress",
				"a message")
		},
		result: &SignMessageCmd{
			id:      testId,
			Address: "btcaddress",
			Message: "a message",
		},
	},
	{
		name: "basic signrawtransaction",
		f: func() (Cmd, error) {
			return NewSignRawTransactionCmd(testId,
				"sometxstring")
		},
		result: &SignRawTransactionCmd{
			id:    testId,
			RawTx: "sometxstring",
		},
	},
	/*	{
		name: "basic signrawtransaction with optional",
		f: func() (Cmd, error) {
			return NewSignRawTransactionCmd(testId,
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
			id:    testId,
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
		name: "basic stop",
		f: func() (Cmd, error) {
			return NewStopCmd(testId)
		},
		result: &StopCmd{
			id: testId,
		},
	},
	{
		name: "basic submitblock",
		f: func() (Cmd, error) {
			return NewSubmitBlockCmd(testId,
				"lotsofhex")
		},
		result: &SubmitBlockCmd{
			id:       testId,
			HexBlock: "lotsofhex",
		},
	},
	{
		name: "submitblock with optional object",
		f: func() (Cmd, error) {
			return NewSubmitBlockCmd(testId,
				"lotsofhex",
				&SubmitBlockOptions{WorkId: "otherstuff"})
		},
		result: &SubmitBlockCmd{
			id:       testId,
			HexBlock: "lotsofhex",
			Options:  &SubmitBlockOptions{WorkId: "otherstuff"},
		},
	},
	{
		name: "basic validateaddress",
		f: func() (Cmd, error) {
			return NewValidateAddressCmd(testId,
				"somebtcaddress")
		},
		result: &ValidateAddressCmd{
			id:      testId,
			Address: "somebtcaddress",
		},
	},
	{
		name: "basic verifychain",
		f: func() (Cmd, error) {
			return NewVerifyChainCmd(testId)
		},
		result: &VerifyChainCmd{
			id:         testId,
			CheckLevel: 3,
			CheckDepth: 288,
		},
	},
	{
		name: "basic verifychain + optional",
		f: func() (Cmd, error) {
			return NewVerifyChainCmd(testId, 4, 1)
		},
		result: &VerifyChainCmd{
			id:         testId,
			CheckLevel: 4,
			CheckDepth: 1,
		},
	},
	{
		name: "basic verifymessage",
		f: func() (Cmd, error) {
			return NewVerifyMessageCmd(testId,
				"someaddress",
				"somesig",
				"a message")
		},
		result: &VerifyMessageCmd{
			id:        testId,
			Address:   "someaddress",
			Signature: "somesig",
			Message:   "a message",
		},
	},
	{
		name: "basic walletlock",
		f: func() (Cmd, error) {
			return NewWalletLockCmd(testId)
		},
		result: &WalletLockCmd{
			id: testId,
		},
	},
	{
		name: "basic walletpassphrase",
		f: func() (Cmd, error) {
			return NewWalletPassphraseCmd(testId,
				"phrase1",
				10)
		},
		result: &WalletPassphraseCmd{
			id:         testId,
			Passphrase: "phrase1",
			Timeout:    10,
		},
	},
	{
		name: "basic walletpassphrasechange",
		f: func() (Cmd, error) {
			return NewWalletPassphraseChangeCmd(testId,
				"phrase1", "phrase2")
		},
		result: &WalletPassphraseChangeCmd{
			id:            testId,
			OldPassphrase: "phrase1",
			NewPassphrase: "phrase2",
		},
	},
}

func TestCmds(t *testing.T) {
	for _, test := range jsoncmdtests {
		c, err := test.f()
		if err != nil {
			t.Errorf("%s: failed to run func: %v",
				test.name, err)
			continue
		}

		msg, err := json.Marshal(c)
		if err != nil {
			t.Errorf("%s: failed to marshal cmd: %v",
				test.name, err)
			continue
		}

		c2, err := ParseMarshaledCmd(msg)
		if err != nil {
			t.Errorf("%s: failed to ummarshal cmd: %v",
				test.name, err)
			continue
		}

		id, ok := (c.Id()).(float64)
		if !ok || id != testId {
			t.Errorf("%s: id not returned properly", test.name)
		}

		if !reflect.DeepEqual(test.result, c2) {
			t.Errorf("%s: unmarshal not as expected. "+
				"got %v wanted %v", test.name, spew.Sdump(c2),
				spew.Sdump(test.result))
		}
		if !reflect.DeepEqual(c, c2) {
			t.Errorf("%s: unmarshal not as we started with. "+
				"got %v wanted %v", test.name, spew.Sdump(c2),
				spew.Sdump(c))
		}

	}
}
