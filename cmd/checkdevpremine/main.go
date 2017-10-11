// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/rpcclient"
	"github.com/decred/dcrd/wire"
)

// Codes that are returned to the operating system.
const (
	rcNoDevPremineInputs = 0
	rcDevPremineInputs   = 1
	rcError              = 2
)

const (
	// devCoinMaxIndex is the final index in the block 1 premine transaction
	// that involves the original developer premine coins.  All coins after
	// this index are part of the airdrop.
	devCoinMaxIndex = 173
)

var (
	// premineTxHash is the hash of the transaction in block one which
	// creates the premine coins.
	premineTxHash = newHashFromStr("5e29cdb355b3fc7e76c98a9983cd44324b3efdd7815c866e33f6c72292cb8be6")
)

// newHashFromStr converts the passed big-endian hex string into a
// chainhash.Hash.  It only differs from the one available in chainhash in that
// it panics on an error since it will only (and must only) be called with
// hard-coded, and therefore known good, hashes.
func newHashFromStr(hexStr string) *chainhash.Hash {
	hash, err := chainhash.NewHashFromStr(hexStr)
	if err != nil {
		panic(err)
	}
	return hash
}

// usage displays the general usage when the help flag is not displayed and
// and an invalid command was specified.  The commandUsage function is used
// instead when a valid command was specified.
func usage(errorMessage string) {
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	fmt.Fprintln(os.Stderr, errorMessage)
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintf(os.Stderr, "  %s [OPTIONS] <txhash or JSON_array_of_txhashes>\n\n",
		appName)
	fmt.Fprintln(os.Stderr, "Specify -h to show available options")
}

// isDevPremineOut return whether or not the provided outpoint is one of the
// original dev premine coins.
func isDevPremineOut(out wire.OutPoint) bool {
	return out.Hash.IsEqual(premineTxHash) && out.Index <= devCoinMaxIndex &&
		out.Tree == 0
}

// traceDevPremineOuts returns a list of outpoints that are part of the dev
// premine coins and are ancestors of the inputs to the passed transaction hash.
func traceDevPremineOuts(client *rpcclient.Client, txHash *chainhash.Hash) ([]wire.OutPoint, error) {
	// Trace the lineage of all inputs to the provided transaction back to
	// the coinbase outputs that generated them and add those outpoints to
	// a list.  Also, keep track of all of the processed transactions in
	// order to avoid processing duplicates.
	knownCoinbases := make(map[chainhash.Hash]struct{})
	processedHashes := make(map[chainhash.Hash]struct{})
	coinbaseOuts := make([]wire.OutPoint, 0, 10)
	processOuts := []wire.OutPoint{{Hash: *txHash}}
	for len(processOuts) > 0 {
		// Grab the first outpoint to process and skip it if it has
		// already been traced.
		outpoint := processOuts[0]
		processOuts = processOuts[1:]
		if _, exists := processedHashes[outpoint.Hash]; exists {
			if _, exists := knownCoinbases[outpoint.Hash]; exists {
				coinbaseOuts = append(coinbaseOuts, outpoint)
			}
			continue
		}
		processedHashes[outpoint.Hash] = struct{}{}

		// Request the transaction for the outpoint from the server.
		tx, err := client.GetRawTransaction(&outpoint.Hash)
		if err != nil {
			return nil, fmt.Errorf("failed to get transaction %v: %v",
				&outpoint.Hash, err)
		}

		// Add the outpoint to the coinbase outputs list when it is part
		// of a coinbase transaction.  Also, keep track of the fact the
		// transaction is a coinbase to use when avoiding duplicate
		// checks.
		if blockchain.IsCoinBase(tx) {
			knownCoinbases[outpoint.Hash] = struct{}{}
			coinbaseOuts = append(coinbaseOuts, outpoint)
			continue
		}

		// Add the inputs to the transaction to the list of transactions
		// to load and continue tracing.
		//
		// However, skip the first input to stake generation txns since
		// they are creating new coins.  The remaining inputs to a
		// stake generation transaction still need to be traced since
		// they represent the coins that purchased the ticket.
		txIns := tx.MsgTx().TxIn
		isSSGen, _ := stake.IsSSGen(tx.MsgTx())
		if isSSGen {
			txIns = txIns[1:]
		}
		for _, txIn := range txIns {
			processOuts = append(processOuts, txIn.PreviousOutPoint)
		}
	}

	// Add any of the outputs that are dev premine outputs to a list.
	var devPremineOuts []wire.OutPoint
	for _, coinbaseOut := range coinbaseOuts {
		if isDevPremineOut(coinbaseOut) {
			devPremineOuts = append(devPremineOuts, coinbaseOut)
		}
	}

	return devPremineOuts, nil
}

// realMain is the real main function for the utility.  It is necessary to work
// around the fact that deferred functions do not run when os.Exit() is called.
func realMain() int {
	// Load configuration and parse command line.
	cfg, args, err := loadConfig()
	if err != nil {
		return rcError
	}

	// Ensure the user specified a single argument.
	if len(args) < 1 {
		usage("Transaction hash not specified")
		return rcError
	}
	if len(args) > 1 {
		usage("Too many arguments specified")
		return rcError
	}

	// Read the argument from a stdin pipe when it is '-'.
	arg0 := args[0]
	if arg0 == "-" {
		bio := bufio.NewReader(os.Stdin)
		param, err := bio.ReadString('\n')
		if err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "Failed to read data from "+
				"stdin: %v\n", err)
			return rcError
		}
		if err == io.EOF && len(param) == 0 {
			fmt.Fprintln(os.Stderr, "Not enough lines provided on "+
				"stdin")
			return rcError
		}
		arg0 = param
	}
	arg0 = strings.TrimSpace(arg0)

	// Attempt to unmarshal the parameter as a JSON array of strings if it
	// looks like JSON input.  This allows multiple transactions to be
	// specified via the argument.  Treat the argument as a single hash if
	// it fails to unmarshal.
	var txHashes []*chainhash.Hash
	if strings.Contains(arg0, "[") && strings.Contains(arg0, "]") {
		var txHashStrs []string
		if err := json.Unmarshal([]byte(arg0), &txHashStrs); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to unmarshal JSON "+
				"string array of transaction hashes: %v\n", err)
			return rcError
		}
		for _, txHashStr := range txHashStrs {
			txHash, err := chainhash.NewHashFromStr(txHashStr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to parse "+
					"transaction hash %q: %v\n", txHashStr, err)
				return rcError
			}
			txHashes = append(txHashes, txHash)
		}
	} else {
		// Parse the provided transaction hash string.
		arg0 = strings.Trim(arg0, `"`)
		txHash, err := chainhash.NewHashFromStr(arg0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to parse transaction "+
				"hash %q: %v\n", arg0, err)
			return rcError
		}
		txHashes = append(txHashes, txHash)
	}

	// Connect to dcrd RPC server using websockets.
	certs, err := ioutil.ReadFile(cfg.RPCCert)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read RPC server TLS cert: %v\n",
			err)
		return rcError
	}
	connCfg := &rpcclient.ConnConfig{
		Host:         cfg.RPCServer,
		Endpoint:     "ws",
		User:         cfg.RPCUser,
		Pass:         cfg.RPCPassword,
		Certificates: certs,
	}
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to dcrd RPC server: "+
			"%v\n", err)
		return rcError
	}
	defer client.Shutdown()

	// Check all of the provided transactions.
	var hasDevPremineOuts bool
	for _, txHash := range txHashes {
		// Get a list of all dev premine outpoints the are ancestors of
		// all inputs to the provided transaction.
		devPremineOuts, err := traceDevPremineOuts(client, txHash)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return rcError
		}

		// List outputs which are dev premine outputs.
		if len(devPremineOuts) > 0 {
			hasDevPremineOuts = true

			// Don't print anything in quiet mode.
			if cfg.Quiet {
				continue
			}
			fmt.Printf("Transaction %v contains inputs which "+
				"trace back to the following original dev "+
				"premine outpoints:\n", txHash)
			for _, out := range devPremineOuts {
				fmt.Println(out)
			}
		}
	}

	// Return the approriate code depending on whether or not any of the
	// inputs trace back to a dev premine outpoint.
	if hasDevPremineOuts {
		return rcDevPremineInputs
	}
	return rcNoDevPremineInputs
}

func main() {
	os.Exit(realMain())
}
