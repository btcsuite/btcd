// Copyright (c) 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// TestChainSvrCmds tests all of the chain server commands marshal and unmarshal
// into valid results include handling of optional fields being omitted in the
// marshalled command, while optional fields with defaults have the default
// assigned on unmarshalled commands.
func TestChainSvrCmds(t *testing.T) {
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
			name: "addnode",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("addnode", "127.0.0.1", btcjson.ANRemove)
			},
			staticCmd: func() interface{} {
				return btcjson.NewAddNodeCmd("127.0.0.1", btcjson.ANRemove)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"addnode","params":["127.0.0.1","remove"],"id":1}`,
			unmarshalled: &btcjson.AddNodeCmd{Addr: "127.0.0.1", SubCmd: btcjson.ANRemove},
		},
		{
			name: "createrawtransaction",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("createrawtransaction", `[{"txid":"123","vout":1}]`,
					`{"456":0.0123}`)
			},
			staticCmd: func() interface{} {
				txInputs := []btcjson.TransactionInput{
					{Txid: "123", Vout: 1},
				}
				amounts := map[string]float64{"456": .0123}
				return btcjson.NewCreateRawTransactionCmd(txInputs, amounts, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"createrawtransaction","params":[[{"txid":"123","vout":1}],{"456":0.0123}],"id":1}`,
			unmarshalled: &btcjson.CreateRawTransactionCmd{
				Inputs:  []btcjson.TransactionInput{{Txid: "123", Vout: 1}},
				Amounts: map[string]float64{"456": .0123},
			},
		},
		{
			name: "createrawtransaction - no inputs",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("createrawtransaction", `[]`, `{"456":0.0123}`)
			},
			staticCmd: func() interface{} {
				amounts := map[string]float64{"456": .0123}
				return btcjson.NewCreateRawTransactionCmd(nil, amounts, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"createrawtransaction","params":[[],{"456":0.0123}],"id":1}`,
			unmarshalled: &btcjson.CreateRawTransactionCmd{
				Inputs:  []btcjson.TransactionInput{},
				Amounts: map[string]float64{"456": .0123},
			},
		},
		{
			name: "createrawtransaction optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("createrawtransaction", `[{"txid":"123","vout":1}]`,
					`{"456":0.0123}`, int64(12312333333))
			},
			staticCmd: func() interface{} {
				txInputs := []btcjson.TransactionInput{
					{Txid: "123", Vout: 1},
				}
				amounts := map[string]float64{"456": .0123}
				return btcjson.NewCreateRawTransactionCmd(txInputs, amounts, btcjson.Int64(12312333333))
			},
			marshalled: `{"jsonrpc":"1.0","method":"createrawtransaction","params":[[{"txid":"123","vout":1}],{"456":0.0123},12312333333],"id":1}`,
			unmarshalled: &btcjson.CreateRawTransactionCmd{
				Inputs:   []btcjson.TransactionInput{{Txid: "123", Vout: 1}},
				Amounts:  map[string]float64{"456": .0123},
				LockTime: btcjson.Int64(12312333333),
			},
		},
		{
			name: "fundrawtransaction - empty opts",
			newCmd: func() (i interface{}, e error) {
				return btcjson.NewCmd("fundrawtransaction", "deadbeef", "{}")
			},
			staticCmd: func() interface{} {
				deadbeef, err := hex.DecodeString("deadbeef")
				if err != nil {
					panic(err)
				}
				return btcjson.NewFundRawTransactionCmd(deadbeef, btcjson.FundRawTransactionOpts{}, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"fundrawtransaction","params":["deadbeef",{}],"id":1}`,
			unmarshalled: &btcjson.FundRawTransactionCmd{
				HexTx:     "deadbeef",
				Options:   btcjson.FundRawTransactionOpts{},
				IsWitness: nil,
			},
		},
		{
			name: "fundrawtransaction - full opts",
			newCmd: func() (i interface{}, e error) {
				return btcjson.NewCmd("fundrawtransaction", "deadbeef", `{"changeAddress":"bcrt1qeeuctq9wutlcl5zatge7rjgx0k45228cxez655","changePosition":1,"change_type":"legacy","includeWatching":true,"lockUnspents":true,"feeRate":0.7,"subtractFeeFromOutputs":[0],"replaceable":true,"conf_target":8,"estimate_mode":"ECONOMICAL"}`)
			},
			staticCmd: func() interface{} {
				deadbeef, err := hex.DecodeString("deadbeef")
				if err != nil {
					panic(err)
				}
				changeAddress := "bcrt1qeeuctq9wutlcl5zatge7rjgx0k45228cxez655"
				change := 1
				changeType := btcjson.ChangeTypeLegacy
				watching := true
				lockUnspents := true
				feeRate := 0.7
				replaceable := true
				confTarget := 8

				return btcjson.NewFundRawTransactionCmd(deadbeef, btcjson.FundRawTransactionOpts{
					ChangeAddress:          &changeAddress,
					ChangePosition:         &change,
					ChangeType:             &changeType,
					IncludeWatching:        &watching,
					LockUnspents:           &lockUnspents,
					FeeRate:                &feeRate,
					SubtractFeeFromOutputs: []int{0},
					Replaceable:            &replaceable,
					ConfTarget:             &confTarget,
					EstimateMode:           &btcjson.EstimateModeEconomical,
				}, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"fundrawtransaction","params":["deadbeef",{"changeAddress":"bcrt1qeeuctq9wutlcl5zatge7rjgx0k45228cxez655","changePosition":1,"change_type":"legacy","includeWatching":true,"lockUnspents":true,"feeRate":0.7,"subtractFeeFromOutputs":[0],"replaceable":true,"conf_target":8,"estimate_mode":"ECONOMICAL"}],"id":1}`,
			unmarshalled: func() interface{} {
				changeAddress := "bcrt1qeeuctq9wutlcl5zatge7rjgx0k45228cxez655"
				change := 1
				changeType := btcjson.ChangeTypeLegacy
				watching := true
				lockUnspents := true
				feeRate := 0.7
				replaceable := true
				confTarget := 8
				return &btcjson.FundRawTransactionCmd{
					HexTx: "deadbeef",
					Options: btcjson.FundRawTransactionOpts{
						ChangeAddress:          &changeAddress,
						ChangePosition:         &change,
						ChangeType:             &changeType,
						IncludeWatching:        &watching,
						LockUnspents:           &lockUnspents,
						FeeRate:                &feeRate,
						SubtractFeeFromOutputs: []int{0},
						Replaceable:            &replaceable,
						ConfTarget:             &confTarget,
						EstimateMode:           &btcjson.EstimateModeEconomical,
					},
					IsWitness: nil,
				}
			}(),
		},
		{
			name: "fundrawtransaction - iswitness",
			newCmd: func() (i interface{}, e error) {
				return btcjson.NewCmd("fundrawtransaction", "deadbeef", "{}", true)
			},
			staticCmd: func() interface{} {
				deadbeef, err := hex.DecodeString("deadbeef")
				if err != nil {
					panic(err)
				}
				t := true
				return btcjson.NewFundRawTransactionCmd(deadbeef, btcjson.FundRawTransactionOpts{}, &t)
			},
			marshalled: `{"jsonrpc":"1.0","method":"fundrawtransaction","params":["deadbeef",{},true],"id":1}`,
			unmarshalled: &btcjson.FundRawTransactionCmd{
				HexTx:   "deadbeef",
				Options: btcjson.FundRawTransactionOpts{},
				IsWitness: func() *bool {
					t := true
					return &t
				}(),
			},
		},
		{
			name: "decoderawtransaction",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("decoderawtransaction", "123")
			},
			staticCmd: func() interface{} {
				return btcjson.NewDecodeRawTransactionCmd("123")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"decoderawtransaction","params":["123"],"id":1}`,
			unmarshalled: &btcjson.DecodeRawTransactionCmd{HexTx: "123"},
		},
		{
			name: "decodescript",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("decodescript", "00")
			},
			staticCmd: func() interface{} {
				return btcjson.NewDecodeScriptCmd("00")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"decodescript","params":["00"],"id":1}`,
			unmarshalled: &btcjson.DecodeScriptCmd{HexScript: "00"},
		},
		{
			name: "deriveaddresses no range",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("deriveaddresses", "00")
			},
			staticCmd: func() interface{} {
				return btcjson.NewDeriveAddressesCmd("00", nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"deriveaddresses","params":["00"],"id":1}`,
			unmarshalled: &btcjson.DeriveAddressesCmd{Descriptor: "00"},
		},
		{
			name: "deriveaddresses int range",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"deriveaddresses", "00", btcjson.DescriptorRange{Value: 2})
			},
			staticCmd: func() interface{} {
				return btcjson.NewDeriveAddressesCmd(
					"00", &btcjson.DescriptorRange{Value: 2})
			},
			marshalled: `{"jsonrpc":"1.0","method":"deriveaddresses","params":["00",2],"id":1}`,
			unmarshalled: &btcjson.DeriveAddressesCmd{
				Descriptor: "00",
				Range:      &btcjson.DescriptorRange{Value: 2},
			},
		},
		{
			name: "deriveaddresses slice range",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"deriveaddresses", "00",
					btcjson.DescriptorRange{Value: []int{0, 2}},
				)
			},
			staticCmd: func() interface{} {
				return btcjson.NewDeriveAddressesCmd(
					"00", &btcjson.DescriptorRange{Value: []int{0, 2}})
			},
			marshalled: `{"jsonrpc":"1.0","method":"deriveaddresses","params":["00",[0,2]],"id":1}`,
			unmarshalled: &btcjson.DeriveAddressesCmd{
				Descriptor: "00",
				Range:      &btcjson.DescriptorRange{Value: []int{0, 2}},
			},
		},
		{
			name: "getaddednodeinfo",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getaddednodeinfo", true)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetAddedNodeInfoCmd(true, nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getaddednodeinfo","params":[true],"id":1}`,
			unmarshalled: &btcjson.GetAddedNodeInfoCmd{DNS: true, Node: nil},
		},
		{
			name: "getaddednodeinfo optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getaddednodeinfo", true, "127.0.0.1")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetAddedNodeInfoCmd(true, btcjson.String("127.0.0.1"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getaddednodeinfo","params":[true,"127.0.0.1"],"id":1}`,
			unmarshalled: &btcjson.GetAddedNodeInfoCmd{
				DNS:  true,
				Node: btcjson.String("127.0.0.1"),
			},
		},
		{
			name: "getbestblockhash",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getbestblockhash")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBestBlockHashCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getbestblockhash","params":[],"id":1}`,
			unmarshalled: &btcjson.GetBestBlockHashCmd{},
		},
		{
			name: "getblock",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblock", "123", btcjson.Int(0))
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockCmd("123", btcjson.Int(0))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblock","params":["123",0],"id":1}`,
			unmarshalled: &btcjson.GetBlockCmd{
				Hash:      "123",
				Verbosity: btcjson.Int(0),
			},
		},
		{
			name: "getblock default verbosity",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblock", "123")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockCmd("123", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblock","params":["123"],"id":1}`,
			unmarshalled: &btcjson.GetBlockCmd{
				Hash:      "123",
				Verbosity: btcjson.Int(1),
			},
		},
		{
			name: "getblock required optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblock", "123", btcjson.Int(1))
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockCmd("123", btcjson.Int(1))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblock","params":["123",1],"id":1}`,
			unmarshalled: &btcjson.GetBlockCmd{
				Hash:      "123",
				Verbosity: btcjson.Int(1),
			},
		},
		{
			name: "getblock required optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblock", "123", btcjson.Int(2))
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockCmd("123", btcjson.Int(2))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblock","params":["123",2],"id":1}`,
			unmarshalled: &btcjson.GetBlockCmd{
				Hash:      "123",
				Verbosity: btcjson.Int(2),
			},
		},
		{
			name: "getblockchaininfo",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblockchaininfo")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockChainInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockchaininfo","params":[],"id":1}`,
			unmarshalled: &btcjson.GetBlockChainInfoCmd{},
		},
		{
			name: "getblockcount",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblockcount")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockCountCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockcount","params":[],"id":1}`,
			unmarshalled: &btcjson.GetBlockCountCmd{},
		},
		{
			name: "getblockfilter",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblockfilter", "0000afaf")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockFilterCmd("0000afaf", nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockfilter","params":["0000afaf"],"id":1}`,
			unmarshalled: &btcjson.GetBlockFilterCmd{"0000afaf", nil},
		},
		{
			name: "getblockfilter optional filtertype",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblockfilter", "0000afaf", "basic")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockFilterCmd("0000afaf", btcjson.NewFilterTypeName(btcjson.FilterTypeBasic))
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockfilter","params":["0000afaf","basic"],"id":1}`,
			unmarshalled: &btcjson.GetBlockFilterCmd{"0000afaf", btcjson.NewFilterTypeName(btcjson.FilterTypeBasic)},
		},
		{
			name: "getblockhash",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblockhash", 123)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockHashCmd(123)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblockhash","params":[123],"id":1}`,
			unmarshalled: &btcjson.GetBlockHashCmd{Index: 123},
		},
		{
			name: "getblockheader",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblockheader", "123")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockHeaderCmd("123", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblockheader","params":["123"],"id":1}`,
			unmarshalled: &btcjson.GetBlockHeaderCmd{
				Hash:    "123",
				Verbose: btcjson.Bool(true),
			},
		},
		{
			name: "getblockstats height",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblockstats", btcjson.HashOrHeight{Value: 123})
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockStatsCmd(btcjson.HashOrHeight{Value: 123}, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblockstats","params":[123],"id":1}`,
			unmarshalled: &btcjson.GetBlockStatsCmd{
				HashOrHeight: btcjson.HashOrHeight{Value: 123},
			},
		},
		{
			name: "getblockstats hash",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblockstats", btcjson.HashOrHeight{Value: "deadbeef"})
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockStatsCmd(btcjson.HashOrHeight{Value: "deadbeef"}, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblockstats","params":["deadbeef"],"id":1}`,
			unmarshalled: &btcjson.GetBlockStatsCmd{
				HashOrHeight: btcjson.HashOrHeight{Value: "deadbeef"},
			},
		},
		{
			name: "getblockstats height optional stats",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblockstats", btcjson.HashOrHeight{Value: 123}, []string{"avgfee", "maxfee"})
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockStatsCmd(btcjson.HashOrHeight{Value: 123}, &[]string{"avgfee", "maxfee"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblockstats","params":[123,["avgfee","maxfee"]],"id":1}`,
			unmarshalled: &btcjson.GetBlockStatsCmd{
				HashOrHeight: btcjson.HashOrHeight{Value: 123},
				Stats:        &[]string{"avgfee", "maxfee"},
			},
		},
		{
			name: "getblockstats hash optional stats",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblockstats", btcjson.HashOrHeight{Value: "deadbeef"}, []string{"avgfee", "maxfee"})
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockStatsCmd(btcjson.HashOrHeight{Value: "deadbeef"}, &[]string{"avgfee", "maxfee"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblockstats","params":["deadbeef",["avgfee","maxfee"]],"id":1}`,
			unmarshalled: &btcjson.GetBlockStatsCmd{
				HashOrHeight: btcjson.HashOrHeight{Value: "deadbeef"},
				Stats:        &[]string{"avgfee", "maxfee"},
			},
		},
		{
			name: "getblocktemplate",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblocktemplate")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetBlockTemplateCmd(nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getblocktemplate","params":[],"id":1}`,
			unmarshalled: &btcjson.GetBlockTemplateCmd{Request: nil},
		},
		{
			name: "getblocktemplate optional - template request",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblocktemplate", `{"mode":"template","capabilities":["longpoll","coinbasetxn"]}`)
			},
			staticCmd: func() interface{} {
				template := btcjson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
				}
				return btcjson.NewGetBlockTemplateCmd(&template)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblocktemplate","params":[{"mode":"template","capabilities":["longpoll","coinbasetxn"]}],"id":1}`,
			unmarshalled: &btcjson.GetBlockTemplateCmd{
				Request: &btcjson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
				},
			},
		},
		{
			name: "getblocktemplate optional - template request with tweaks",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblocktemplate", `{"mode":"template","capabilities":["longpoll","coinbasetxn"],"sigoplimit":500,"sizelimit":100000000,"maxversion":2}`)
			},
			staticCmd: func() interface{} {
				template := btcjson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
					SigOpLimit:   500,
					SizeLimit:    100000000,
					MaxVersion:   2,
				}
				return btcjson.NewGetBlockTemplateCmd(&template)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblocktemplate","params":[{"mode":"template","capabilities":["longpoll","coinbasetxn"],"sigoplimit":500,"sizelimit":100000000,"maxversion":2}],"id":1}`,
			unmarshalled: &btcjson.GetBlockTemplateCmd{
				Request: &btcjson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
					SigOpLimit:   int64(500),
					SizeLimit:    int64(100000000),
					MaxVersion:   2,
				},
			},
		},
		{
			name: "getblocktemplate optional - template request with tweaks 2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getblocktemplate", `{"mode":"template","capabilities":["longpoll","coinbasetxn"],"sigoplimit":true,"sizelimit":100000000,"maxversion":2}`)
			},
			staticCmd: func() interface{} {
				template := btcjson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
					SigOpLimit:   true,
					SizeLimit:    100000000,
					MaxVersion:   2,
				}
				return btcjson.NewGetBlockTemplateCmd(&template)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getblocktemplate","params":[{"mode":"template","capabilities":["longpoll","coinbasetxn"],"sigoplimit":true,"sizelimit":100000000,"maxversion":2}],"id":1}`,
			unmarshalled: &btcjson.GetBlockTemplateCmd{
				Request: &btcjson.TemplateRequest{
					Mode:         "template",
					Capabilities: []string{"longpoll", "coinbasetxn"},
					SigOpLimit:   true,
					SizeLimit:    int64(100000000),
					MaxVersion:   2,
				},
			},
		},
		{
			name: "getcfilter",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getcfilter", "123",
					wire.GCSFilterRegular)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetCFilterCmd("123",
					wire.GCSFilterRegular)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getcfilter","params":["123",0],"id":1}`,
			unmarshalled: &btcjson.GetCFilterCmd{
				Hash:       "123",
				FilterType: wire.GCSFilterRegular,
			},
		},
		{
			name: "getcfilterheader",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getcfilterheader", "123",
					wire.GCSFilterRegular)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetCFilterHeaderCmd("123",
					wire.GCSFilterRegular)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getcfilterheader","params":["123",0],"id":1}`,
			unmarshalled: &btcjson.GetCFilterHeaderCmd{
				Hash:       "123",
				FilterType: wire.GCSFilterRegular,
			},
		},
		{
			name: "getchaintips",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getchaintips")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetChainTipsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getchaintips","params":[],"id":1}`,
			unmarshalled: &btcjson.GetChainTipsCmd{},
		},
		{
			name: "getchaintxstats",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getchaintxstats")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetChainTxStatsCmd(nil, nil)
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getchaintxstats","params":[],"id":1}`,
			unmarshalled: &btcjson.GetChainTxStatsCmd{},
		},
		{
			name: "getchaintxstats optional nblocks",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getchaintxstats", btcjson.Int32(1000))
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetChainTxStatsCmd(btcjson.Int32(1000), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getchaintxstats","params":[1000],"id":1}`,
			unmarshalled: &btcjson.GetChainTxStatsCmd{
				NBlocks: btcjson.Int32(1000),
			},
		},
		{
			name: "getchaintxstats optional nblocks and blockhash",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getchaintxstats", btcjson.Int32(1000), btcjson.String("0000afaf"))
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetChainTxStatsCmd(btcjson.Int32(1000), btcjson.String("0000afaf"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getchaintxstats","params":[1000,"0000afaf"],"id":1}`,
			unmarshalled: &btcjson.GetChainTxStatsCmd{
				NBlocks:   btcjson.Int32(1000),
				BlockHash: btcjson.String("0000afaf"),
			},
		},
		{
			name: "getconnectioncount",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getconnectioncount")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetConnectionCountCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getconnectioncount","params":[],"id":1}`,
			unmarshalled: &btcjson.GetConnectionCountCmd{},
		},
		{
			name: "getdifficulty",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getdifficulty")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetDifficultyCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getdifficulty","params":[],"id":1}`,
			unmarshalled: &btcjson.GetDifficultyCmd{},
		},
		{
			name: "getgenerate",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getgenerate")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetGenerateCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getgenerate","params":[],"id":1}`,
			unmarshalled: &btcjson.GetGenerateCmd{},
		},
		{
			name: "gethashespersec",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("gethashespersec")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetHashesPerSecCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"gethashespersec","params":[],"id":1}`,
			unmarshalled: &btcjson.GetHashesPerSecCmd{},
		},
		{
			name: "getinfo",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getinfo")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getinfo","params":[],"id":1}`,
			unmarshalled: &btcjson.GetInfoCmd{},
		},
		{
			name: "getmempoolentry",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getmempoolentry", "txhash")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetMempoolEntryCmd("txhash")
			},
			marshalled: `{"jsonrpc":"1.0","method":"getmempoolentry","params":["txhash"],"id":1}`,
			unmarshalled: &btcjson.GetMempoolEntryCmd{
				TxID: "txhash",
			},
		},
		{
			name: "getmempoolinfo",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getmempoolinfo")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetMempoolInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getmempoolinfo","params":[],"id":1}`,
			unmarshalled: &btcjson.GetMempoolInfoCmd{},
		},
		{
			name: "getmininginfo",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getmininginfo")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetMiningInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getmininginfo","params":[],"id":1}`,
			unmarshalled: &btcjson.GetMiningInfoCmd{},
		},
		{
			name: "getnetworkinfo",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getnetworkinfo")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetNetworkInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getnetworkinfo","params":[],"id":1}`,
			unmarshalled: &btcjson.GetNetworkInfoCmd{},
		},
		{
			name: "getnettotals",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getnettotals")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetNetTotalsCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getnettotals","params":[],"id":1}`,
			unmarshalled: &btcjson.GetNetTotalsCmd{},
		},
		{
			name: "getnetworkhashps",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getnetworkhashps")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetNetworkHashPSCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnetworkhashps","params":[],"id":1}`,
			unmarshalled: &btcjson.GetNetworkHashPSCmd{
				Blocks: btcjson.Int(120),
				Height: btcjson.Int(-1),
			},
		},
		{
			name: "getnetworkhashps optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getnetworkhashps", 200)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetNetworkHashPSCmd(btcjson.Int(200), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnetworkhashps","params":[200],"id":1}`,
			unmarshalled: &btcjson.GetNetworkHashPSCmd{
				Blocks: btcjson.Int(200),
				Height: btcjson.Int(-1),
			},
		},
		{
			name: "getnetworkhashps optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getnetworkhashps", 200, 123)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetNetworkHashPSCmd(btcjson.Int(200), btcjson.Int(123))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnetworkhashps","params":[200,123],"id":1}`,
			unmarshalled: &btcjson.GetNetworkHashPSCmd{
				Blocks: btcjson.Int(200),
				Height: btcjson.Int(123),
			},
		},
		{
			name: "getnodeaddresses",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getnodeaddresses")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetNodeAddressesCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnodeaddresses","params":[],"id":1}`,
			unmarshalled: &btcjson.GetNodeAddressesCmd{
				Count: btcjson.Int32(1),
			},
		},
		{
			name: "getnodeaddresses optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getnodeaddresses", 10)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetNodeAddressesCmd(btcjson.Int32(10))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getnodeaddresses","params":[10],"id":1}`,
			unmarshalled: &btcjson.GetNodeAddressesCmd{
				Count: btcjson.Int32(10),
			},
		},
		{
			name: "getpeerinfo",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getpeerinfo")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetPeerInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getpeerinfo","params":[],"id":1}`,
			unmarshalled: &btcjson.GetPeerInfoCmd{},
		},
		{
			name: "getrawmempool",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getrawmempool")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetRawMempoolCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawmempool","params":[],"id":1}`,
			unmarshalled: &btcjson.GetRawMempoolCmd{
				Verbose: btcjson.Bool(false),
			},
		},
		{
			name: "getrawmempool optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getrawmempool", false)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetRawMempoolCmd(btcjson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawmempool","params":[false],"id":1}`,
			unmarshalled: &btcjson.GetRawMempoolCmd{
				Verbose: btcjson.Bool(false),
			},
		},
		{
			name: "getrawtransaction",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getrawtransaction", "123")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetRawTransactionCmd("123", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawtransaction","params":["123"],"id":1}`,
			unmarshalled: &btcjson.GetRawTransactionCmd{
				Txid:    "123",
				Verbose: btcjson.Int(0),
			},
		},
		{
			name: "getrawtransaction optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getrawtransaction", "123", 1)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetRawTransactionCmd("123", btcjson.Int(1))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getrawtransaction","params":["123",1],"id":1}`,
			unmarshalled: &btcjson.GetRawTransactionCmd{
				Txid:    "123",
				Verbose: btcjson.Int(1),
			},
		},
		{
			name: "gettxout",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("gettxout", "123", 1)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetTxOutCmd("123", 1, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettxout","params":["123",1],"id":1}`,
			unmarshalled: &btcjson.GetTxOutCmd{
				Txid:           "123",
				Vout:           1,
				IncludeMempool: btcjson.Bool(true),
			},
		},
		{
			name: "gettxout optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("gettxout", "123", 1, true)
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetTxOutCmd("123", 1, btcjson.Bool(true))
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettxout","params":["123",1,true],"id":1}`,
			unmarshalled: &btcjson.GetTxOutCmd{
				Txid:           "123",
				Vout:           1,
				IncludeMempool: btcjson.Bool(true),
			},
		},
		{
			name: "gettxoutproof",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("gettxoutproof", []string{"123", "456"})
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetTxOutProofCmd([]string{"123", "456"}, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettxoutproof","params":[["123","456"]],"id":1}`,
			unmarshalled: &btcjson.GetTxOutProofCmd{
				TxIDs: []string{"123", "456"},
			},
		},
		{
			name: "gettxoutproof optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("gettxoutproof", []string{"123", "456"},
					btcjson.String("000000000000034a7dedef4a161fa058a2d67a173a90155f3a2fe6fc132e0ebf"))
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetTxOutProofCmd([]string{"123", "456"},
					btcjson.String("000000000000034a7dedef4a161fa058a2d67a173a90155f3a2fe6fc132e0ebf"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettxoutproof","params":[["123","456"],` +
				`"000000000000034a7dedef4a161fa058a2d67a173a90155f3a2fe6fc132e0ebf"],"id":1}`,
			unmarshalled: &btcjson.GetTxOutProofCmd{
				TxIDs:     []string{"123", "456"},
				BlockHash: btcjson.String("000000000000034a7dedef4a161fa058a2d67a173a90155f3a2fe6fc132e0ebf"),
			},
		},
		{
			name: "gettxoutsetinfo",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("gettxoutsetinfo")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetTxOutSetInfoCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"gettxoutsetinfo","params":[],"id":1}`,
			unmarshalled: &btcjson.GetTxOutSetInfoCmd{},
		},
		{
			name: "getwork",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getwork")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetWorkCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"getwork","params":[],"id":1}`,
			unmarshalled: &btcjson.GetWorkCmd{
				Data: nil,
			},
		},
		{
			name: "getwork optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getwork", "00112233")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetWorkCmd(btcjson.String("00112233"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"getwork","params":["00112233"],"id":1}`,
			unmarshalled: &btcjson.GetWorkCmd{
				Data: btcjson.String("00112233"),
			},
		},
		{
			name: "help",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("help")
			},
			staticCmd: func() interface{} {
				return btcjson.NewHelpCmd(nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"help","params":[],"id":1}`,
			unmarshalled: &btcjson.HelpCmd{
				Command: nil,
			},
		},
		{
			name: "help optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("help", "getblock")
			},
			staticCmd: func() interface{} {
				return btcjson.NewHelpCmd(btcjson.String("getblock"))
			},
			marshalled: `{"jsonrpc":"1.0","method":"help","params":["getblock"],"id":1}`,
			unmarshalled: &btcjson.HelpCmd{
				Command: btcjson.String("getblock"),
			},
		},
		{
			name: "invalidateblock",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("invalidateblock", "123")
			},
			staticCmd: func() interface{} {
				return btcjson.NewInvalidateBlockCmd("123")
			},
			marshalled: `{"jsonrpc":"1.0","method":"invalidateblock","params":["123"],"id":1}`,
			unmarshalled: &btcjson.InvalidateBlockCmd{
				BlockHash: "123",
			},
		},
		{
			name: "ping",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("ping")
			},
			staticCmd: func() interface{} {
				return btcjson.NewPingCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"ping","params":[],"id":1}`,
			unmarshalled: &btcjson.PingCmd{},
		},
		{
			name: "preciousblock",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("preciousblock", "0123")
			},
			staticCmd: func() interface{} {
				return btcjson.NewPreciousBlockCmd("0123")
			},
			marshalled: `{"jsonrpc":"1.0","method":"preciousblock","params":["0123"],"id":1}`,
			unmarshalled: &btcjson.PreciousBlockCmd{
				BlockHash: "0123",
			},
		},
		{
			name: "reconsiderblock",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("reconsiderblock", "123")
			},
			staticCmd: func() interface{} {
				return btcjson.NewReconsiderBlockCmd("123")
			},
			marshalled: `{"jsonrpc":"1.0","method":"reconsiderblock","params":["123"],"id":1}`,
			unmarshalled: &btcjson.ReconsiderBlockCmd{
				BlockHash: "123",
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("searchrawtransactions", "1Address")
			},
			staticCmd: func() interface{} {
				return btcjson.NewSearchRawTransactionsCmd("1Address", nil, nil, nil, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address"],"id":1}`,
			unmarshalled: &btcjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     btcjson.Int(1),
				Skip:        btcjson.Int(0),
				Count:       btcjson.Int(100),
				VinExtra:    btcjson.Int(0),
				Reverse:     btcjson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("searchrawtransactions", "1Address", 0)
			},
			staticCmd: func() interface{} {
				return btcjson.NewSearchRawTransactionsCmd("1Address",
					btcjson.Int(0), nil, nil, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0],"id":1}`,
			unmarshalled: &btcjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     btcjson.Int(0),
				Skip:        btcjson.Int(0),
				Count:       btcjson.Int(100),
				VinExtra:    btcjson.Int(0),
				Reverse:     btcjson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("searchrawtransactions", "1Address", 0, 5)
			},
			staticCmd: func() interface{} {
				return btcjson.NewSearchRawTransactionsCmd("1Address",
					btcjson.Int(0), btcjson.Int(5), nil, nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5],"id":1}`,
			unmarshalled: &btcjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     btcjson.Int(0),
				Skip:        btcjson.Int(5),
				Count:       btcjson.Int(100),
				VinExtra:    btcjson.Int(0),
				Reverse:     btcjson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("searchrawtransactions", "1Address", 0, 5, 10)
			},
			staticCmd: func() interface{} {
				return btcjson.NewSearchRawTransactionsCmd("1Address",
					btcjson.Int(0), btcjson.Int(5), btcjson.Int(10), nil, nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5,10],"id":1}`,
			unmarshalled: &btcjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     btcjson.Int(0),
				Skip:        btcjson.Int(5),
				Count:       btcjson.Int(10),
				VinExtra:    btcjson.Int(0),
				Reverse:     btcjson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("searchrawtransactions", "1Address", 0, 5, 10, 1)
			},
			staticCmd: func() interface{} {
				return btcjson.NewSearchRawTransactionsCmd("1Address",
					btcjson.Int(0), btcjson.Int(5), btcjson.Int(10), btcjson.Int(1), nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5,10,1],"id":1}`,
			unmarshalled: &btcjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     btcjson.Int(0),
				Skip:        btcjson.Int(5),
				Count:       btcjson.Int(10),
				VinExtra:    btcjson.Int(1),
				Reverse:     btcjson.Bool(false),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("searchrawtransactions", "1Address", 0, 5, 10, 1, true)
			},
			staticCmd: func() interface{} {
				return btcjson.NewSearchRawTransactionsCmd("1Address",
					btcjson.Int(0), btcjson.Int(5), btcjson.Int(10), btcjson.Int(1), btcjson.Bool(true), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5,10,1,true],"id":1}`,
			unmarshalled: &btcjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     btcjson.Int(0),
				Skip:        btcjson.Int(5),
				Count:       btcjson.Int(10),
				VinExtra:    btcjson.Int(1),
				Reverse:     btcjson.Bool(true),
				FilterAddrs: nil,
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("searchrawtransactions", "1Address", 0, 5, 10, 1, true, []string{"1Address"})
			},
			staticCmd: func() interface{} {
				return btcjson.NewSearchRawTransactionsCmd("1Address",
					btcjson.Int(0), btcjson.Int(5), btcjson.Int(10), btcjson.Int(1), btcjson.Bool(true), &[]string{"1Address"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5,10,1,true,["1Address"]],"id":1}`,
			unmarshalled: &btcjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     btcjson.Int(0),
				Skip:        btcjson.Int(5),
				Count:       btcjson.Int(10),
				VinExtra:    btcjson.Int(1),
				Reverse:     btcjson.Bool(true),
				FilterAddrs: &[]string{"1Address"},
			},
		},
		{
			name: "searchrawtransactions",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("searchrawtransactions", "1Address", 0, 5, 10, "null", true, []string{"1Address"})
			},
			staticCmd: func() interface{} {
				return btcjson.NewSearchRawTransactionsCmd("1Address",
					btcjson.Int(0), btcjson.Int(5), btcjson.Int(10), nil, btcjson.Bool(true), &[]string{"1Address"})
			},
			marshalled: `{"jsonrpc":"1.0","method":"searchrawtransactions","params":["1Address",0,5,10,null,true,["1Address"]],"id":1}`,
			unmarshalled: &btcjson.SearchRawTransactionsCmd{
				Address:     "1Address",
				Verbose:     btcjson.Int(0),
				Skip:        btcjson.Int(5),
				Count:       btcjson.Int(10),
				VinExtra:    nil,
				Reverse:     btcjson.Bool(true),
				FilterAddrs: &[]string{"1Address"},
			},
		},
		{
			name: "sendrawtransaction",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("sendrawtransaction", "1122", &btcjson.AllowHighFeesOrMaxFeeRate{})
			},
			staticCmd: func() interface{} {
				return btcjson.NewSendRawTransactionCmd("1122", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendrawtransaction","params":["1122",false],"id":1}`,
			unmarshalled: &btcjson.SendRawTransactionCmd{
				HexTx: "1122",
				FeeSetting: &btcjson.AllowHighFeesOrMaxFeeRate{
					Value: btcjson.Bool(false),
				},
			},
		},
		{
			name: "sendrawtransaction optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("sendrawtransaction", "1122", &btcjson.AllowHighFeesOrMaxFeeRate{Value: btcjson.Bool(false)})
			},
			staticCmd: func() interface{} {
				return btcjson.NewSendRawTransactionCmd("1122", btcjson.Bool(false))
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendrawtransaction","params":["1122",false],"id":1}`,
			unmarshalled: &btcjson.SendRawTransactionCmd{
				HexTx: "1122",
				FeeSetting: &btcjson.AllowHighFeesOrMaxFeeRate{
					Value: btcjson.Bool(false),
				},
			},
		},
		{
			name: "sendrawtransaction optional, bitcoind >= 0.19.0",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("sendrawtransaction", "1122", &btcjson.AllowHighFeesOrMaxFeeRate{Value: btcjson.Int32(1234)})
			},
			staticCmd: func() interface{} {
				return btcjson.NewBitcoindSendRawTransactionCmd("1122", 1234)
			},
			marshalled: `{"jsonrpc":"1.0","method":"sendrawtransaction","params":["1122",1234],"id":1}`,
			unmarshalled: &btcjson.SendRawTransactionCmd{
				HexTx: "1122",
				FeeSetting: &btcjson.AllowHighFeesOrMaxFeeRate{
					Value: btcjson.Int32(1234),
				},
			},
		},
		{
			name: "setgenerate",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("setgenerate", true)
			},
			staticCmd: func() interface{} {
				return btcjson.NewSetGenerateCmd(true, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"setgenerate","params":[true],"id":1}`,
			unmarshalled: &btcjson.SetGenerateCmd{
				Generate:     true,
				GenProcLimit: btcjson.Int(-1),
			},
		},
		{
			name: "setgenerate optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("setgenerate", true, 6)
			},
			staticCmd: func() interface{} {
				return btcjson.NewSetGenerateCmd(true, btcjson.Int(6))
			},
			marshalled: `{"jsonrpc":"1.0","method":"setgenerate","params":[true,6],"id":1}`,
			unmarshalled: &btcjson.SetGenerateCmd{
				Generate:     true,
				GenProcLimit: btcjson.Int(6),
			},
		},
		{
			name: "signmessagewithprivkey",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("signmessagewithprivkey", "5Hue", "Hey")
			},
			staticCmd: func() interface{} {
				return btcjson.NewSignMessageWithPrivKey("5Hue", "Hey")
			},
			marshalled: `{"jsonrpc":"1.0","method":"signmessagewithprivkey","params":["5Hue","Hey"],"id":1}`,
			unmarshalled: &btcjson.SignMessageWithPrivKeyCmd{
				PrivKey: "5Hue",
				Message: "Hey",
			},
		},
		{
			name: "stop",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("stop")
			},
			staticCmd: func() interface{} {
				return btcjson.NewStopCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"stop","params":[],"id":1}`,
			unmarshalled: &btcjson.StopCmd{},
		},
		{
			name: "submitblock",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("submitblock", "112233")
			},
			staticCmd: func() interface{} {
				return btcjson.NewSubmitBlockCmd("112233", nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"submitblock","params":["112233"],"id":1}`,
			unmarshalled: &btcjson.SubmitBlockCmd{
				HexBlock: "112233",
				Options:  nil,
			},
		},
		{
			name: "submitblock optional",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("submitblock", "112233", `{"workid":"12345"}`)
			},
			staticCmd: func() interface{} {
				options := btcjson.SubmitBlockOptions{
					WorkID: "12345",
				}
				return btcjson.NewSubmitBlockCmd("112233", &options)
			},
			marshalled: `{"jsonrpc":"1.0","method":"submitblock","params":["112233",{"workid":"12345"}],"id":1}`,
			unmarshalled: &btcjson.SubmitBlockCmd{
				HexBlock: "112233",
				Options: &btcjson.SubmitBlockOptions{
					WorkID: "12345",
				},
			},
		},
		{
			name: "uptime",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("uptime")
			},
			staticCmd: func() interface{} {
				return btcjson.NewUptimeCmd()
			},
			marshalled:   `{"jsonrpc":"1.0","method":"uptime","params":[],"id":1}`,
			unmarshalled: &btcjson.UptimeCmd{},
		},
		{
			name: "validateaddress",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("validateaddress", "1Address")
			},
			staticCmd: func() interface{} {
				return btcjson.NewValidateAddressCmd("1Address")
			},
			marshalled: `{"jsonrpc":"1.0","method":"validateaddress","params":["1Address"],"id":1}`,
			unmarshalled: &btcjson.ValidateAddressCmd{
				Address: "1Address",
			},
		},
		{
			name: "verifychain",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("verifychain")
			},
			staticCmd: func() interface{} {
				return btcjson.NewVerifyChainCmd(nil, nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifychain","params":[],"id":1}`,
			unmarshalled: &btcjson.VerifyChainCmd{
				CheckLevel: btcjson.Int32(3),
				CheckDepth: btcjson.Int32(288),
			},
		},
		{
			name: "verifychain optional1",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("verifychain", 2)
			},
			staticCmd: func() interface{} {
				return btcjson.NewVerifyChainCmd(btcjson.Int32(2), nil)
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifychain","params":[2],"id":1}`,
			unmarshalled: &btcjson.VerifyChainCmd{
				CheckLevel: btcjson.Int32(2),
				CheckDepth: btcjson.Int32(288),
			},
		},
		{
			name: "verifychain optional2",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("verifychain", 2, 500)
			},
			staticCmd: func() interface{} {
				return btcjson.NewVerifyChainCmd(btcjson.Int32(2), btcjson.Int32(500))
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifychain","params":[2,500],"id":1}`,
			unmarshalled: &btcjson.VerifyChainCmd{
				CheckLevel: btcjson.Int32(2),
				CheckDepth: btcjson.Int32(500),
			},
		},
		{
			name: "verifymessage",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("verifymessage", "1Address", "301234", "test")
			},
			staticCmd: func() interface{} {
				return btcjson.NewVerifyMessageCmd("1Address", "301234", "test")
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifymessage","params":["1Address","301234","test"],"id":1}`,
			unmarshalled: &btcjson.VerifyMessageCmd{
				Address:   "1Address",
				Signature: "301234",
				Message:   "test",
			},
		},
		{
			name: "verifytxoutproof",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("verifytxoutproof", "test")
			},
			staticCmd: func() interface{} {
				return btcjson.NewVerifyTxOutProofCmd("test")
			},
			marshalled: `{"jsonrpc":"1.0","method":"verifytxoutproof","params":["test"],"id":1}`,
			unmarshalled: &btcjson.VerifyTxOutProofCmd{
				Proof: "test",
			},
		},
		{
			name: "getdescriptorinfo",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getdescriptorinfo", "123")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetDescriptorInfoCmd("123")
			},
			marshalled:   `{"jsonrpc":"1.0","method":"getdescriptorinfo","params":["123"],"id":1}`,
			unmarshalled: &btcjson.GetDescriptorInfoCmd{Descriptor: "123"},
		},
		{
			name: "getzmqnotifications",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("getzmqnotifications")
			},
			staticCmd: func() interface{} {
				return btcjson.NewGetZmqNotificationsCmd()
			},

			marshalled:   `{"jsonrpc":"1.0","method":"getzmqnotifications","params":[],"id":1}`,
			unmarshalled: &btcjson.GetZmqNotificationsCmd{},
		},
		{
			name: "testmempoolaccept",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("testmempoolaccept", []string{"rawhex"}, 0.1)
			},
			staticCmd: func() interface{} {
				return btcjson.NewTestMempoolAcceptCmd([]string{"rawhex"}, 0.1)
			},
			marshalled: `{"jsonrpc":"1.0","method":"testmempoolaccept","params":[["rawhex"],0.1],"id":1}`,
			unmarshalled: &btcjson.TestMempoolAcceptCmd{
				RawTxns:    []string{"rawhex"},
				MaxFeeRate: 0.1,
			},
		},
		{
			name: "testmempoolaccept with maxfeerate",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd("testmempoolaccept", []string{"rawhex"}, 0.01)
			},
			staticCmd: func() interface{} {
				return btcjson.NewTestMempoolAcceptCmd([]string{"rawhex"}, 0.01)
			},
			marshalled: `{"jsonrpc":"1.0","method":"testmempoolaccept","params":[["rawhex"],0.01],"id":1}`,
			unmarshalled: &btcjson.TestMempoolAcceptCmd{
				RawTxns:    []string{"rawhex"},
				MaxFeeRate: 0.01,
			},
		},
		{
			name: "gettxspendingprevout",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"gettxspendingprevout",
					[]*btcjson.GetTxSpendingPrevOutCmdOutput{
						{Txid: "0000000000000000000000000000000000000000000000000000000000000001", Vout: 0},
					})
			},
			staticCmd: func() interface{} {
				outputs := []wire.OutPoint{
					{Hash: chainhash.Hash{1}, Index: 0},
				}
				return btcjson.NewGetTxSpendingPrevOutCmd(outputs)
			},
			marshalled: `{"jsonrpc":"1.0","method":"gettxspendingprevout","params":[[{"txid":"0000000000000000000000000000000000000000000000000000000000000001","vout":0}]],"id":1}`,
			unmarshalled: &btcjson.GetTxSpendingPrevOutCmd{
				Outputs: []*btcjson.GetTxSpendingPrevOutCmdOutput{{
					Txid: "0000000000000000000000000000000000000000000000000000000000000001",
					Vout: 0,
				}},
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
			t.Errorf("\n%s\n%s", marshalled, test.marshalled)
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

// TestChainSvrCmdErrors ensures any errors that occur in the command during
// custom mashal and unmarshal are as expected.
func TestChainSvrCmdErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		result     interface{}
		marshalled string
		err        error
	}{
		{
			name:       "template request with invalid type",
			result:     &btcjson.TemplateRequest{},
			marshalled: `{"mode":1}`,
			err:        &json.UnmarshalTypeError{},
		},
		{
			name:       "invalid template request sigoplimit field",
			result:     &btcjson.TemplateRequest{},
			marshalled: `{"sigoplimit":"invalid"}`,
			err:        btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
		{
			name:       "invalid template request sizelimit field",
			result:     &btcjson.TemplateRequest{},
			marshalled: `{"sizelimit":"invalid"}`,
			err:        btcjson.Error{ErrorCode: btcjson.ErrInvalidType},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		err := json.Unmarshal([]byte(test.marshalled), &test.result)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("Test #%d (%s) wrong error - got %T (%v), "+
				"want %T", i, test.name, err, err, test.err)
			continue
		}

		if terr, ok := test.err.(btcjson.Error); ok {
			gotErrorCode := err.(btcjson.Error).ErrorCode
			if gotErrorCode != terr.ErrorCode {
				t.Errorf("Test #%d (%s) mismatched error code "+
					"- got %v (%v), want %v", i, test.name,
					gotErrorCode, terr, terr.ErrorCode)
				continue
			}
		}
	}
}
