// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// this has to be in the real package so we can mock up structs
package btcws

import (
	"github.com/conformal/btcdb"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcwire"
	"github.com/davecgh/go-spew/spew"
	"reflect"
	"testing"
)

var cmdtests = []struct {
	name   string
	f      func() (btcjson.Cmd, error)
	result btcjson.Cmd // after marshal and unmarshal
}{
	{
		name: "createencryptedwallet",
		f: func() (btcjson.Cmd, error) {
			return NewCreateEncryptedWalletCmd(
				float64(1),
				"abcde",
				"description",
				"banana"), nil
		},
		result: &CreateEncryptedWalletCmd{
			id:          float64(1),
			Account:     "abcde",
			Description: "description",
			Passphrase:  "banana",
		},
	},
	{
		name: "getaddressbalance no optargs",
		f: func() (btcjson.Cmd, error) {
			return NewGetAddressBalanceCmd(
				float64(1),
				"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH")
		},
		result: &GetAddressBalanceCmd{
			id:      float64(1),
			Address: "17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH",
			Minconf: 1,
		},
	},
	{
		name: "getaddressbalance one optarg",
		f: func() (btcjson.Cmd, error) {
			return NewGetAddressBalanceCmd(
				float64(1),
				"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH",
				0)
		},
		result: &GetAddressBalanceCmd{
			id:      float64(1),
			Address: "17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH",
			Minconf: 0,
		},
	},
	{
		name: "getbestblock",
		f: func() (btcjson.Cmd, error) {
			return NewGetBestBlockCmd(float64(1)), nil
		},
		result: &GetBestBlockCmd{
			id: float64(1),
		},
	},
	{
		name: "getcurrentnet",
		f: func() (btcjson.Cmd, error) {
			return NewGetCurrentNetCmd(float64(1)), nil
		},
		result: &GetCurrentNetCmd{
			id: float64(1),
		},
	},
	{
		name: "getunconfirmedbalance no optargs",
		f: func() (btcjson.Cmd, error) {
			return NewGetUnconfirmedBalanceCmd(float64(1))
		},
		result: &GetUnconfirmedBalanceCmd{
			id:      float64(1),
			Account: "",
		},
	},
	{
		name: "getunconfirmedbalance one optarg",
		f: func() (btcjson.Cmd, error) {
			return NewGetUnconfirmedBalanceCmd(float64(1),
				"abcde")
		},
		result: &GetUnconfirmedBalanceCmd{
			id:      float64(1),
			Account: "abcde",
		},
	},
	{
		name: "listaddresstransactions no optargs",
		f: func() (btcjson.Cmd, error) {
			addrs := []string{
				"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH",
			}
			return NewListAddressTransactionsCmd(
				float64(1),
				addrs)
		},
		result: &ListAddressTransactionsCmd{
			id:      float64(1),
			Account: "",
			Addresses: []string{
				"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH",
			},
		},
	},
	{
		name: "listaddresstransactions one optarg",
		f: func() (btcjson.Cmd, error) {
			addrs := []string{
				"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH",
			}
			return NewListAddressTransactionsCmd(
				float64(1),
				addrs,
				"abcde")
		},
		result: &ListAddressTransactionsCmd{
			id:      float64(1),
			Account: "abcde",
			Addresses: []string{
				"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH",
			},
		},
	},
	{
		name: "listalltransactions no optargs",
		f: func() (btcjson.Cmd, error) {
			return NewListAllTransactionsCmd(float64(1))
		},
		result: &ListAllTransactionsCmd{
			id:      float64(1),
			Account: "",
		},
	},
	{
		name: "listalltransactions one optarg",
		f: func() (btcjson.Cmd, error) {
			return NewListAllTransactionsCmd(
				float64(1),
				"abcde")
		},
		result: &ListAllTransactionsCmd{
			id:      float64(1),
			Account: "abcde",
		},
	},
	{
		name: "notifynewtxs",
		f: func() (btcjson.Cmd, error) {
			addrs := []string{
				"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH",
			}
			return NewNotifyNewTXsCmd(
				float64(1),
				addrs), nil
		},
		result: &NotifyNewTXsCmd{
			id: float64(1),
			Addresses: []string{
				"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH",
			},
		},
	},
	{
		name: "notifyspent",
		f: func() (btcjson.Cmd, error) {
			op := &btcwire.OutPoint{
				Hash: btcwire.ShaHash{0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
					10, 11, 12, 13, 14, 15, 16, 17, 18, 19,
					20, 21, 22, 23, 24, 25, 26, 27, 28, 29,
					30, 31},
				Index: 1,
			}
			return NewNotifySpentCmd(float64(1), op), nil
		},
		result: &NotifySpentCmd{
			id: float64(1),
			OutPoint: &btcwire.OutPoint{
				Hash: btcwire.ShaHash{0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
					10, 11, 12, 13, 14, 15, 16, 17, 18, 19,
					20, 21, 22, 23, 24, 25, 26, 27, 28, 29,
					30, 31},
				Index: 1,
			},
		},
	},
	{
		name: "rescan no optargs",
		f: func() (btcjson.Cmd, error) {
			addrs := map[string]struct{}{
				"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH": struct{}{},
			}
			return NewRescanCmd(
				float64(1),
				270000,
				addrs)
		},
		result: &RescanCmd{
			id:         float64(1),
			BeginBlock: 270000,
			Addresses: map[string]struct{}{
				"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH": struct{}{},
			},
			EndBlock: btcdb.AllShas,
		},
	},
	{
		name: "rescan one optarg",
		f: func() (btcjson.Cmd, error) {
			addrs := map[string]struct{}{
				"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH": struct{}{},
			}
			return NewRescanCmd(
				float64(1),
				270000,
				addrs,
				280000)
		},
		result: &RescanCmd{
			id:         float64(1),
			BeginBlock: 270000,
			Addresses: map[string]struct{}{
				"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH": struct{}{},
			},
			EndBlock: 280000,
		},
	},
	{
		name: "walletislocked no optargs",
		f: func() (btcjson.Cmd, error) {
			return NewWalletIsLockedCmd(float64(1))
		},
		result: &WalletIsLockedCmd{
			id:      float64(1),
			Account: "",
		},
	},
	{
		name: "walletislocked one optarg",
		f: func() (btcjson.Cmd, error) {
			return NewWalletIsLockedCmd(
				float64(1),
				"abcde")
		},
		result: &WalletIsLockedCmd{
			id:      float64(1),
			Account: "abcde",
		},
	},
}

func TestCmds(t *testing.T) {
	for _, test := range cmdtests {
		c, err := test.f()
		if err != nil {
			t.Errorf("%s: failed to run func: %v",
				test.name, err)
			continue
		}

		mc, err := c.MarshalJSON()
		if err != nil {
			t.Errorf("%s: failed to marshal cmd: %v",
				test.name, err)
			continue
		}

		c2, err := btcjson.ParseMarshaledCmd(mc)
		if err != nil {
			t.Errorf("%s: failed to ummarshal cmd: %v",
				test.name, err)
			continue
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

		// id from Id func must match result.
		if c.Id() != test.result.Id() {
			t.Errorf("%s: Id returned incorrect id. "+
				"got %v wanted %v", test.name, c.Id(),
				test.result.Id())
		}

		// method from Method func must match result.
		if c.Method() != test.result.Method() {
			t.Errorf("%s: Method returned incorrect method. "+
				"got %v wanted %v", test.name, c.Method(),
				test.result.Method())
		}

		// Read marshaled command back into c.  Should still
		// match result.
		c.UnmarshalJSON(mc)
		if !reflect.DeepEqual(test.result, c) {
			t.Errorf("%s: unmarshal not as expected. "+
				"got %v wanted %v", test.name, spew.Sdump(c),
				spew.Sdump(test.result))
		}
	}
}
