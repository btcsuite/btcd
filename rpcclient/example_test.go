package rpcclient

import (
	"fmt"
)

func ExampleClient_GetDescriptorInfo() {
	connCfg := &ConnConfig{
		Host:         "localhost:8332",
		User:         "yourrpcuser",
		Pass:         "yourrpcpass",
		HTTPPostMode: true,
		DisableTLS:   true,
	}
	client, err := New(connCfg, nil)
	if err != nil {
		log.Error(err)
		return
	}
	defer client.Shutdown()

	descriptorInfo, err := client.GetDescriptorInfo(
		"wpkh([d34db33f/84h/0h/0h]0279be667ef9dcbbac55a06295Ce870b07029Bfcdb2dce28d959f2815b16f81798)")
	if err != nil {
		log.Error(err)
		return
	}

	fmt.Printf("%+v\n", descriptorInfo)
	// &{Descriptor:wpkh([d34db33f/84'/0'/0']0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798)#n9g43y4k Checksum:qwlqgth7 IsRange:false IsSolvable:true HasPrivateKeys:false}
}
