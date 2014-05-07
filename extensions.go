// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcrpcclient

import (
	"encoding/base64"
	"fmt"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/conformal/btcws"
)

// FutureDebugLevelResult is a future promise to deliver the result of a
// DebugLevelAsync RPC invocation (or an applicable error).
type FutureDebugLevelResult chan *futureResult

// Receive waits for the response promised by the future and returns the result
// of setting the debug logging level to the passed level specification or the
// list of of the available subsystems for the special keyword 'show'.
func (r FutureDebugLevelResult) Receive() (string, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return "", err
	}

	// Ensure the returned data is the expected type.
	result, ok := reply.(string)
	if !ok {
		return "", fmt.Errorf("unexpected response type for "+
			"debuglevel: %T\n", reply)
	}

	return result, nil
}

// DebugLevelAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See DebugLevel for the blocking version and more details.
//
// NOTE: This is a btcd extension.
func (c *Client) DebugLevelAsync(levelSpec string) FutureDebugLevelResult {
	id := c.NextID()
	cmd, err := btcjson.NewDebugLevelCmd(id, levelSpec)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// DebugLevel dynamically sets the debug logging level to the passed level
// specification.
//
// The levelspec can either a debug level or of the form:
// 	<subsystem>=<level>,<subsystem2>=<level2>,...
//
// Additionally, the special keyword 'show' can be used to get a list of the
// available subsystems.
//
// NOTE: This is a btcd extension.
func (c *Client) DebugLevel(levelSpec string) (string, error) {
	return c.DebugLevelAsync(levelSpec).Receive()
}

// FutureListAddressTransactionsResult is a future promise to deliver the result
// of a ListAddressTransactionsAsync RPC invocation (or an applicable error).
type FutureListAddressTransactionsResult chan *futureResult

// Receive waits for the response promised by the future and returns information
// about all transactions associated with the provided addresses.
func (r FutureListAddressTransactionsResult) Receive() ([]btcjson.ListTransactionsResult, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// No transactions.
	if reply == nil {
		return nil, nil
	}

	// Ensure the returned data is the expected type.
	transactions, ok := reply.([]btcjson.ListTransactionsResult)
	if !ok {
		return nil, fmt.Errorf("unexpected response type for "+
			"listaddresstransactions: %T\n", reply)
	}

	return transactions, nil
}

// ListAddressTransactionsAsync returns an instance of a type that can be used
// to get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See ListAddressTransactions for the blocking version and more details.
//
// NOTE: This is a btcd extension.
func (c *Client) ListAddressTransactionsAsync(addresses []btcutil.Address, account string) FutureListAddressTransactionsResult {
	// Convert addresses to strings.
	addrs := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		addrs = append(addrs, addr.EncodeAddress())
	}
	id := c.NextID()
	cmd, err := btcws.NewListAddressTransactionsCmd(id, addrs, account)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListAddressTransactions returns information about all transactions associated
// with the provided addresses.
//
// NOTE: This is a btcwallet extension.
func (c *Client) ListAddressTransactions(addresses []btcutil.Address, account string) ([]btcjson.ListTransactionsResult, error) {
	return c.ListAddressTransactionsAsync(addresses, account).Receive()
}

// FutureGetBestBlockResult is a future promise to deliver the result of a
// GetBestBlockAsync RPC invocation (or an applicable error).
type FutureGetBestBlockResult chan *futureResult

// Receive waits for the response promised by the future and returns the hash
// and height of the block in the longest (best) chain.
func (r FutureGetBestBlockResult) Receive() (*btcwire.ShaHash, int32, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return nil, 0, err
	}

	// Ensure the returned data is the expected type.
	result, ok := reply.(btcws.GetBestBlockResult)
	if !ok {
		return nil, 0, fmt.Errorf("unexpected response type for "+
			"listaddresstransactions: %T\n", reply)
	}

	// Convert hash string.
	hash, err := btcwire.NewShaHashFromStr(result.Hash)
	if err != nil {
		return nil, 0, err
	}

	return hash, result.Height, nil
}

// GetBestBlockAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBestBlock for the blocking version and more details.
//
// NOTE: This is a btcd extension.
func (c *Client) GetBestBlockAsync() FutureGetBestBlockResult {
	id := c.NextID()
	cmd := btcws.NewGetBestBlockCmd(id)

	return c.sendCmd(cmd)
}

// GetBestBlock returns the hash and height of the block in the longest (best)
// chain.
//
// NOTE: This is a btcd extension.
func (c *Client) GetBestBlock() (*btcwire.ShaHash, int32, error) {
	return c.GetBestBlockAsync().Receive()
}

// FutureGetCurrentNetResult is a future promise to deliver the result of a
// GetCurrentNetAsync RPC invocation (or an applicable error).
type FutureGetCurrentNetResult chan *futureResult

// Receive waits for the response promised by the future and returns the network
// the server is running on.
func (r FutureGetCurrentNetResult) Receive() (btcwire.BitcoinNet, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return 0, err
	}

	// Ensure the returned data is the expected type.
	fnet, ok := reply.(float64)
	if !ok {
		return 0, fmt.Errorf("unexpected response type for "+
			"getcurrentnet: %T\n", reply)
	}

	return btcwire.BitcoinNet(fnet), nil
}

// GetCurrentNetAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetCurrentNet for the blocking version and more details.
//
// NOTE: This is a btcd extension.
func (c *Client) GetCurrentNetAsync() FutureGetCurrentNetResult {
	id := c.NextID()
	cmd := btcws.NewGetCurrentNetCmd(id)

	return c.sendCmd(cmd)
}

// GetCurrentNet returns the network the server is running on.
//
// NOTE: This is a btcd extension.
func (c *Client) GetCurrentNet() (btcwire.BitcoinNet, error) {
	return c.GetCurrentNetAsync().Receive()
}

// FutureExportWatchingWalletResult is a future promise to deliver the result of
// an ExportWatchingWalletAsync RPC invocation (or an applicable error).
type FutureExportWatchingWalletResult chan *futureResult

// Receive waits for the response promised by the future and returns the
// exported wallet.
func (r FutureExportWatchingWalletResult) Receive() ([]byte, []byte, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return nil, nil, err
	}

	// Ensure the returned data is the expected type.
	result, ok := reply.(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("unexpected response type for "+
			"exportwatchingwallet: %T\n", reply)
	}

	base64Wallet, ok := result["wallet"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected response type for "+
			"exportwatchingwallet 'wallet' field: %T\n",
			result["wallet"])
	}
	base64TxStore, ok := result["tx"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected response type for "+
			"exportwatchingwallet 'tx' field: %T\n",
			result["tx"])
	}

	walletBytes, err := base64.StdEncoding.DecodeString(base64Wallet)
	if err != nil {
		return nil, nil, err
	}

	txStoreBytes, err := base64.StdEncoding.DecodeString(base64TxStore)
	if err != nil {
		return nil, nil, err
	}

	return walletBytes, txStoreBytes, nil

}

// ExportWatchingWalletAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See ExportWatchingWallet for the blocking version and more details.
//
// NOTE: This is a btcwallet extension.
func (c *Client) ExportWatchingWalletAsync(account string) FutureExportWatchingWalletResult {
	id := c.NextID()
	cmd, err := btcws.NewExportWatchingWalletCmd(id, account, true)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ExportWatchingWallet returns the raw bytes for a watching-only version of
// wallet.bin and tx.bin, respectively, for the specified account that can be
// used by btcwallet to enable a wallet which does not have the private keys
// necessary to spend funds.
//
// NOTE: This is a btcwallet extension.
func (c *Client) ExportWatchingWallet(account string) ([]byte, []byte, error) {
	return c.ExportWatchingWalletAsync(account).Receive()
}
