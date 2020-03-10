// Copyright (c) 2014-2019 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/btcsuite/btcd/rpcclient"
	"log"
)

func main() {
	// Connect to local bitcoin core RPC server using HTTP POST mode.
	connCfg := &rpcclient.ConnConfig{
		Host:                "localhost:8332",
		User:                "yourrpcuser",
		Pass:                "yourrpcpass",
		DisableConnectOnNew: true,
		HTTPPostMode:        true, // Bitcoin core only supports HTTP POST mode
		DisableTLS:          true, // Bitcoin core does not provide TLS by default
	}
	// Notice the notification parameter is nil since notifications are
	// not supported in HTTP POST mode.
	client, err := rpcclient.New(connCfg, nil)
	defer client.Shutdown()

	if err != nil {
		log.Fatal(err)
	}

	// Get the current block count.
	batchClient := client.Batch()

	// batch mode requires async requests
	blockCount := batchClient.GetBlockCountAsync()
	block1 := batchClient.GetBlockHashAsync(1)
	batchClient.GetBlockHashAsync(2)
	batchClient.GetBlockHashAsync(3)
	block4 := batchClient.GetBlockHashAsync(4)
	difficulty := batchClient.GetDifficultyAsync()

	// sends all queued batch requests
	batchClient.Send()

	fmt.Println(blockCount.Receive())
	fmt.Println(block1.Receive())
	fmt.Println(block4.Receive())
	fmt.Println(difficulty.Receive())
}
