// Copyright (c) 2014-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"golang.org/x/sync/errgroup"
	"log"
	"time"
)

var client = CreateClient()

// issue happens most around this spot
const initialValue = 390000

func main() {
	height, err := client.GetBlockCount()

	var i int64
	for i = initialValue; i <= height; i++ {
		block := GetBlock(i)

		g, ctx := errgroup.WithContext(context.Background())
		transactions := make(chan rpcclient.FutureGetRawTransactionVerboseResult)

		hashes := block.Tx

		g.Go(func() error {
			defer close(transactions)
			for _, hash := range hashes {
				txHash, err := chainhash.NewHashFromStr(hash)
				if err != nil {
					log.Fatal(err)
				}
				select {
				case transactions <- client.GetRawTransactionVerboseAsync(txHash):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})

		for i := 0; i < 10; i++ {
			g.Go(func() error {
				for transactionFuture := range transactions {
					_, err := transactionFuture.Receive()
					if (err != nil) {
						log.Fatal(err)
					}
					select {
					default:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			panic(err)
		}
	}

	// Register for block connect and disconnect notifications.
	if err := client.NotifyBlocks(); err != nil {
		log.Fatal(err)
	}
	log.Println("NotifyBlocks: Registration Complete")

	// Get the current block count.
	blockCount, err := client.GetBlockCount()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Block count: %d", blockCount)

	// For this example gracefully shutdown the client after 10 seconds.
	// Ordinarily when to shutdown the client is highly application
	// specific.
	log.Println("Client shutdown in 10 seconds...")
	time.AfterFunc(time.Second*10, func() {
		log.Println("Client shutting down...")
		client.Shutdown()
		log.Println("Client shutdown complete.")
	})

	// Wait until the client either shuts down gracefully (or the user
	// terminates the process with Ctrl+C).
	client.WaitForShutdown()
}
