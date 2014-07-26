// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// this has to be in the real package so we can mock up structs
package btcws

import (
	"reflect"
	"testing"

	"github.com/conformal/btcjson"
	"github.com/davecgh/go-spew/spew"
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
				"banana"), nil
		},
		result: &CreateEncryptedWalletCmd{
			id:         float64(1),
			Passphrase: "banana",
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
		name: "notifyreceived",
		f: func() (btcjson.Cmd, error) {
			addrs := []string{
				"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH",
			}
			return NewNotifyReceivedCmd(
				float64(1),
				addrs), nil
		},
		result: &NotifyReceivedCmd{
			id: float64(1),
			Addresses: []string{
				"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH",
			},
		},
	},
	{
		name: "notifynewtransactions",
		f: func() (btcjson.Cmd, error) {
			return NewNotifyNewTransactionsCmd(
				float64(1),
				true)
		},
		result: &NotifyNewTransactionsCmd{
			id:      float64(1),
			Verbose: true,
		},
	},
	{
		name: "notifyspent",
		f: func() (btcjson.Cmd, error) {
			ops := []OutPoint{
				{
					Hash: "000102030405060708091011121314" +
						"1516171819202122232425262728" +
						"293031",
					Index: 1,
				},
			}
			return NewNotifySpentCmd(float64(1), ops), nil
		},
		result: &NotifySpentCmd{
			id: float64(1),
			OutPoints: []OutPoint{
				{
					Hash: "000102030405060708091011121314" +
						"1516171819202122232425262728" +
						"293031",
					Index: 1,
				},
			},
		},
	},
	{
		name: "rescan no optargs",
		f: func() (btcjson.Cmd, error) {
			addrs := []string{"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH"}
			ops := []OutPoint{
				{
					Hash: "000102030405060708091011121314" +
						"1516171819202122232425262728" +
						"293031",
					Index: 1,
				},
			}
			return NewRescanCmd(
				float64(1),
				"0000000000000002a775aec59dc6a9e4bb1c025cf1b8c2195dd9dc3998c827c5",
				addrs,
				ops)
		},
		result: &RescanCmd{
			id:         float64(1),
			BeginBlock: "0000000000000002a775aec59dc6a9e4bb1c025cf1b8c2195dd9dc3998c827c5",
			Addresses:  []string{"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH"},
			OutPoints: []OutPoint{
				{
					Hash: "000102030405060708091011121314" +
						"1516171819202122232425262728" +
						"293031",
					Index: 1,
				},
			},
			EndBlock: "",
		},
	},
	{
		name: "rescan one optarg",
		f: func() (btcjson.Cmd, error) {
			addrs := []string{"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH"}
			ops := []OutPoint{
				{
					Hash: "000102030405060708091011121314" +
						"1516171819202122232425262728" +
						"293031",
					Index: 1,
				},
			}
			return NewRescanCmd(
				float64(1),
				"0000000000000002a775aec59dc6a9e4bb1c025cf1b8c2195dd9dc3998c827c5",
				addrs,
				ops,
				"0000000000000001c091ada69f444dc0282ecaabe4808ddbb2532e5555db0c03")
		},
		result: &RescanCmd{
			id:         float64(1),
			BeginBlock: "0000000000000002a775aec59dc6a9e4bb1c025cf1b8c2195dd9dc3998c827c5",
			Addresses:  []string{"17XhEvq9Nahdj7Xe1nv6oRe1tEmaHUuynH"},
			OutPoints: []OutPoint{
				{
					Hash: "000102030405060708091011121314" +
						"1516171819202122232425262728" +
						"293031",
					Index: 1,
				},
			},
			EndBlock: "0000000000000001c091ada69f444dc0282ecaabe4808ddbb2532e5555db0c03",
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
		if err := c.UnmarshalJSON(mc); err != nil {
			t.Errorf("%s: error while unmarshalling: %v", test.name,
				err)
		}
		if !reflect.DeepEqual(test.result, c) {
			t.Errorf("%s: unmarshal not as expected. "+
				"got %v wanted %v", test.name, spew.Sdump(c),
				spew.Sdump(test.result))
		}
	}
}
