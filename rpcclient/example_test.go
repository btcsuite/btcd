// Copyright (c) 2020 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"fmt"
	"github.com/btcsuite/btcd/btcjson"
)

var connCfg = &ConnConfig{
	Host:         "localhost:8332",
	User:         "user",
	Pass:         "pass",
	HTTPPostMode: true,
	DisableTLS:   true,
}

func ExampleClient_GetDescriptorInfo() {
	client, err := New(connCfg, nil)
	if err != nil {
		panic(err)
	}
	defer client.Shutdown()

	descriptorInfo, err := client.GetDescriptorInfo(
		"wpkh([d34db33f/84h/0h/0h]0279be667ef9dcbbac55a06295Ce870b07029Bfcdb2dce28d959f2815b16f81798)")
	if err != nil {
		panic(err)
	}

	fmt.Printf("%+v\n", descriptorInfo)
	// &{Descriptor:wpkh([d34db33f/84'/0'/0']0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798)#n9g43y4k Checksum:qwlqgth7 IsRange:false IsSolvable:true HasPrivateKeys:false}
}

func ExampleClient_ImportMulti() {
	client, err := New(connCfg, nil)
	if err != nil {
		panic(err)
	}
	defer client.Shutdown()

	requests := []btcjson.ImportMultiRequest{
		{
			Descriptor: btcjson.String(
				"pkh([f34db33f/44'/0'/0']xpub6Cc939fyHvfB9pPLWd3bSyyQFvgKbwhidca49jGCM5Hz5ypEPGf9JVXB4NBuUfPgoHnMjN6oNgdC9KRqM11RZtL8QLW6rFKziNwHDYhZ6Kx/0/*)#ed7px9nu"),
			Range:     &btcjson.DescriptorRange{Value: []int{0, 100}},
			Timestamp: btcjson.TimestampOrNow{Value: 0}, // scan from genesis
			WatchOnly: btcjson.Bool(true),
			KeyPool:   btcjson.Bool(false),
			Internal:  btcjson.Bool(false),
		},
	}
	opts := &btcjson.ImportMultiOptions{Rescan: true}

	resp, err := client.ImportMulti(requests, opts)
	if err != nil {
		panic(err)
	}

	fmt.Println(resp[0].Success)
	// true
}

func ExampleClient_DeriveAddresses() {
	client, err := New(connCfg, nil)
	if err != nil {
		panic(err)
	}
	defer client.Shutdown()

	addrs, err := client.DeriveAddresses(
		"pkh([f34db33f/44'/0'/0']xpub6Cc939fyHvfB9pPLWd3bSyyQFvgKbwhidca49jGCM5Hz5ypEPGf9JVXB4NBuUfPgoHnMjN6oNgdC9KRqM11RZtL8QLW6rFKziNwHDYhZ6Kx/0/*)#ed7px9nu",
		&btcjson.DescriptorRange{Value: []int{0, 2}})
	if err != nil {
		panic(err)
	}

	fmt.Printf("%+v\n", addrs)
	// &[14NjenDKkGGq1McUgoSkeUHJpW3rrKLbPW 1Pn6i3cvdGhqbdgNjXHfbaYfiuviPiymXj 181x1NbgGYKLeMXkDdXEAqepG75EgU8XtG]
}

func ExampleClient_GetAddressInfo() {
	client, err := New(connCfg, nil)
	if err != nil {
		panic(err)
	}
	defer client.Shutdown()

	info, err := client.GetAddressInfo("2NF1FbxtUAsvdU4uW1UC2xkBVatp6cYQuJ3")
	if err != nil {
		panic(err)
	}

	fmt.Println(info.Address)             // 2NF1FbxtUAsvdU4uW1UC2xkBVatp6cYQuJ3
	fmt.Println(info.ScriptType.String()) // witness_v0_keyhash
	fmt.Println(*info.HDKeyPath)          // m/49'/1'/0'/0/4
	fmt.Println(info.Embedded.Address)    // tb1q3x2h2kh57wzg7jz00jhwn0ycvqtdk2ane37j27
}

func ExampleClient_GetWalletInfo() {
	client, err := New(connCfg, nil)
	if err != nil {
		panic(err)
	}
	defer client.Shutdown()

	info, err := client.GetWalletInfo()
	if err != nil {
		panic(err)
	}

	fmt.Println(info.WalletVersion)    // 169900
	fmt.Println(info.TransactionCount) // 22
	fmt.Println(*info.HDSeedID)        // eb44e4e9b864ef17e7ba947da746375b000f5d94
	fmt.Println(info.Scanning.Value)   // false
}

func ExampleClient_GetTxOutSetInfo() {
	client, err := New(connCfg, nil)
	if err != nil {
		panic(err)
	}
	defer client.Shutdown()

	r, err := client.GetTxOutSetInfo()
	if err != nil {
		panic(err)
	}

	fmt.Println(r.TotalAmount.String()) // 20947654.56996054 BTC
	fmt.Println(r.BestBlock.String())   // 000000000000005f94116250e2407310463c0a7cf950f1af9ebe935b1c0687ab
	fmt.Println(r.TxOuts)               // 24280607
	fmt.Println(r.Transactions)         // 9285603
	fmt.Println(r.DiskSize)             // 1320871611
}

func ExampleClient_CreateWallet() {
	client, err := New(connCfg, nil)
	if err != nil {
		panic(err)
	}
	defer client.Shutdown()

	r, err := client.CreateWallet(
		"mywallet",
		WithCreateWalletBlank(),
		WithCreateWalletPassphrase("secret"),
	)
	if err != nil {
		panic(err)
	}

	fmt.Println(r.Name) // mywallet
}
