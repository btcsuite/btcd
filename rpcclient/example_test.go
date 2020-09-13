package rpcclient

import (
	"fmt"

	"github.com/btcsuite/btcd/btcjson"
)

var connCfg = &ConnConfig{
	Host:         "localhost:8332",
	User:         "yourrpcuser",
	Pass:         "yourrpcpass",
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
