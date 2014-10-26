// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcrpcclient

import (
	"encoding/json"
	"strconv"

	"github.com/conformal/btcjson"
	"github.com/conformal/btcnet"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/conformal/btcws"
)

// *****************************
// Transaction Listing Functions
// *****************************

// FutureGetTransactionResult is a future promise to deliver the result
// of a GetTransactionAsync RPC invocation (or an applicable error).
type FutureGetTransactionResult chan *response

// Receive waits for the response promised by the future and returns detailed
// information about a wallet transaction.
func (r FutureGetTransactionResult) Receive() (*btcjson.GetTransactionResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a gettransaction result object
	var getTx btcjson.GetTransactionResult
	err = json.Unmarshal(res, &getTx)
	if err != nil {
		return nil, err
	}

	return &getTx, nil
}

// GetTransactionAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetTransaction for the blocking version and more details.
func (c *Client) GetTransactionAsync(txHash *btcwire.ShaHash) FutureGetTransactionResult {
	hash := ""
	if txHash != nil {
		hash = txHash.String()
	}

	id := c.NextID()
	cmd, err := btcjson.NewGetTransactionCmd(id, hash)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetTransaction returns detailed information about a wallet transaction.
//
// See GetRawTransaction to return the raw transaction instead.
func (c *Client) GetTransaction(txHash *btcwire.ShaHash) (*btcjson.GetTransactionResult, error) {
	return c.GetTransactionAsync(txHash).Receive()
}

// FutureListTransactionsResult is a future promise to deliver the result of a
// ListTransactionsAsync, ListTransactionsCountAsync, or
// ListTransactionsCountFromAsync RPC invocation (or an applicable error).
type FutureListTransactionsResult chan *response

// Receive waits for the response promised by the future and returns a list of
// the most recent transactions.
func (r FutureListTransactionsResult) Receive() ([]btcjson.ListTransactionsResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as an array of listtransaction result objects.
	var transactions []btcjson.ListTransactionsResult
	err = json.Unmarshal(res, &transactions)
	if err != nil {
		return nil, err
	}

	return transactions, nil
}

// ListTransactionsAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See ListTransactions for the blocking version and more details.
func (c *Client) ListTransactionsAsync(account string) FutureListTransactionsResult {
	id := c.NextID()
	cmd, err := btcjson.NewListTransactionsCmd(id, account)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListTransactions returns a list of the most recent transactions.
//
// See the ListTransactionsCount and ListTransactionsCountFrom to control the
// number of transactions returned and starting point, respectively.
func (c *Client) ListTransactions(account string) ([]btcjson.ListTransactionsResult, error) {
	return c.ListTransactionsAsync(account).Receive()
}

// ListTransactionsCountAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See ListTransactionsCount for the blocking version and more details.
func (c *Client) ListTransactionsCountAsync(account string, count int) FutureListTransactionsResult {
	id := c.NextID()
	cmd, err := btcjson.NewListTransactionsCmd(id, account, count)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListTransactionsCount returns a list of the most recent transactions up
// to the passed count.
//
// See the ListTransactions and ListTransactionsCountFrom functions for
// different options.
func (c *Client) ListTransactionsCount(account string, count int) ([]btcjson.ListTransactionsResult, error) {
	return c.ListTransactionsCountAsync(account, count).Receive()
}

// ListTransactionsCountFromAsync returns an instance of a type that can be used
// to get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See ListTransactionsCountFrom for the blocking version and more details.
func (c *Client) ListTransactionsCountFromAsync(account string, count, from int) FutureListTransactionsResult {
	id := c.NextID()
	cmd, err := btcjson.NewListTransactionsCmd(id, account, count, from)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListTransactionsCountFrom returns a list of the most recent transactions up
// to the passed count while skipping the first 'from' transactions.
//
// See the ListTransactions and ListTransactionsCount functions to use defaults.
func (c *Client) ListTransactionsCountFrom(account string, count, from int) ([]btcjson.ListTransactionsResult, error) {
	return c.ListTransactionsCountFromAsync(account, count, from).Receive()
}

// FutureListUnspentResult is a future promise to deliver the result of a
// ListUnspentAsync, ListUnspentMinAsync, ListUnspentMinMaxAsync, or
// ListUnspentMinMaxAddressesAsync RPC invocation (or an applicable error).
type FutureListUnspentResult chan *response

// Receive waits for the response promised by the future and returns all
// unspent wallet transaction outputs returned by the RPC call.  If the
// future wac returnd by a call to ListUnspentMinAsync, ListUnspentMinMaxAsync,
// or ListUnspentMinMaxAddressesAsync, the range may be limited by the
// parameters of the RPC invocation.
func (r FutureListUnspentResult) Receive() ([]btcjson.ListUnspentResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as an array of listunspent results.
	var unspent []btcjson.ListUnspentResult
	err = json.Unmarshal(res, &unspent)
	if err != nil {
		return nil, err
	}

	return unspent, nil
}

// ListUnspentAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function
// on the returned instance.
//
// See ListUnspent for the blocking version and more details.
func (c *Client) ListUnspentAsync() FutureListUnspentResult {
	id := c.NextID()
	cmd, err := btcjson.NewListUnspentCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListUnspentMinAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function
// on the returned instance.
//
// See ListUnspentMin for the blocking version and more details.
func (c *Client) ListUnspentMinAsync(minConf int) FutureListUnspentResult {
	id := c.NextID()
	cmd, err := btcjson.NewListUnspentCmd(id, minConf)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListUnspentMinMaxAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function
// on the returned instance.
//
// See ListUnspentMinMax for the blocking version and more details.
func (c *Client) ListUnspentMinMaxAsync(minConf, maxConf int) FutureListUnspentResult {
	id := c.NextID()
	cmd, err := btcjson.NewListUnspentCmd(id, minConf, maxConf)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListUnspentMinMaxAddressesAsync returns an instance of a type that can be
// used to get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See ListUnspentMinMaxAddresses for the blocking version and more details.
func (c *Client) ListUnspentMinMaxAddressesAsync(minConf, maxConf int, addrs []btcutil.Address) FutureListUnspentResult {
	addrStrs := make([]string, 0, len(addrs))
	for _, a := range addrs {
		addrStrs = append(addrStrs, a.EncodeAddress())
	}

	id := c.NextID()
	cmd, err := btcjson.NewListUnspentCmd(id, minConf, maxConf, addrStrs)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListUnspent returns all unspent transaction outputs known to a wallet, using
// the default number of minimum and maximum number of confirmations as a
// filter (1 and 999999, respectively).
func (c *Client) ListUnspent() ([]btcjson.ListUnspentResult, error) {
	return c.ListUnspentAsync().Receive()
}

// ListUnspentMin returns all unspent transaction outputs known to a wallet,
// using the specified number of minimum conformations and default number of
// maximum confiramtions (999999) as a filter.
func (c *Client) ListUnspentMin(minConf int) ([]btcjson.ListUnspentResult, error) {
	return c.ListUnspentMinAsync(minConf).Receive()
}

// ListUnspentMinMax returns all unspent transaction outputs known to a wallet,
// using the specified number of minimum and maximum number of confirmations as
// a filter.
func (c *Client) ListUnspentMinMax(minConf, maxConf int) ([]btcjson.ListUnspentResult, error) {
	return c.ListUnspentMinMaxAsync(minConf, maxConf).Receive()
}

// ListUnspentMinMaxAddresses returns all unspent transaction outputs that pay
// to any of specified addresses in a wallet using the specified number of
// minimum and maximum number of confirmations as a filter.
func (c *Client) ListUnspentMinMaxAddresses(minConf, maxConf int, addrs []btcutil.Address) ([]btcjson.ListUnspentResult, error) {
	return c.ListUnspentMinMaxAddressesAsync(minConf, maxConf, addrs).Receive()
}

// FutureListSinceBlockResult is a future promise to deliver the result of a
// ListSinceBlockAsync or ListSinceBlockMinConfAsync RPC invocation (or an
// applicable error).
type FutureListSinceBlockResult chan *response

// Receive waits for the response promised by the future and returns all
// transactions added in blocks since the specified block hash, or all
// transactions if it is nil.
func (r FutureListSinceBlockResult) Receive() (*btcjson.ListSinceBlockResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a listsinceblock result object.
	var listResult btcjson.ListSinceBlockResult
	err = json.Unmarshal(res, &listResult)
	if err != nil {
		return nil, err
	}

	return &listResult, nil
}

// ListSinceBlockAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See ListSinceBlock for the blocking version and more details.
func (c *Client) ListSinceBlockAsync(blockHash *btcwire.ShaHash) FutureListSinceBlockResult {
	hash := ""
	if blockHash != nil {
		hash = blockHash.String()
	}

	id := c.NextID()
	cmd, err := btcjson.NewListSinceBlockCmd(id, hash)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListSinceBlock returns all transactions added in blocks since the specified
// block hash, or all transactions if it is nil, using the default number of
// minimum confirmations as a filter.
//
// See ListSinceBlockMinConf to override the minimum number of confirmations.
func (c *Client) ListSinceBlock(blockHash *btcwire.ShaHash) (*btcjson.ListSinceBlockResult, error) {
	return c.ListSinceBlockAsync(blockHash).Receive()
}

// ListSinceBlockMinConfAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See ListSinceBlockMinConf for the blocking version and more details.
func (c *Client) ListSinceBlockMinConfAsync(blockHash *btcwire.ShaHash, minConfirms int) FutureListSinceBlockResult {
	hash := ""
	if blockHash != nil {
		hash = blockHash.String()
	}

	id := c.NextID()
	cmd, err := btcjson.NewListSinceBlockCmd(id, hash, minConfirms)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListSinceBlockMinConf returns all transactions added in blocks since the
// specified block hash, or all transactions if it is nil, using the specified
// number of minimum confirmations as a filter.
//
// See ListSinceBlock to use the default minimum number of confirmations.
func (c *Client) ListSinceBlockMinConf(blockHash *btcwire.ShaHash, minConfirms int) (*btcjson.ListSinceBlockResult, error) {
	return c.ListSinceBlockMinConfAsync(blockHash, minConfirms).Receive()
}

// **************************
// Transaction Send Functions
// **************************

// FutureLockUnspentResult is a future promise to deliver the error result of a
// LockUnspentAsync RPC invocation.
type FutureLockUnspentResult chan *response

// Receive waits for the response promised by the future and returns the result
// of locking or unlocking the unspent output(s).
func (r FutureLockUnspentResult) Receive() error {
	_, err := receiveFuture(r)
	return err
}

// LockUnspentAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See LockUnspent for the blocking version and more details.
func (c *Client) LockUnspentAsync(unlock bool, ops []*btcwire.OutPoint) FutureLockUnspentResult {
	id := c.NextID()
	outputs := make([]btcjson.TransactionInput, len(ops))
	for i, op := range ops {
		outputs[i] = btcjson.TransactionInput{
			Txid: op.Hash.String(),
			Vout: op.Index,
		}
	}
	cmd, err := btcjson.NewLockUnspentCmd(id, unlock, outputs)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// LockUnspent marks outputs as locked or unlocked, depending on the value of
// the unlock bool.  When locked, the unspent output will not be selected as
// input for newly created, non-raw transactions, and will not be returned in
// future ListUnspent results, until the output is marked unlocked again.
//
// If unlock is false, each outpoint in ops will be marked locked.  If unlocked
// is true and specific outputs are specified in ops (len != 0), exactly those
// outputs will be marked unlocked.  If unlocked is true and no outpoints are
// specified, all previous locked outputs are marked unlocked.
//
// The locked or unlocked state of outputs are not written to disk and after
// restarting a wallet process, this data will be reset (every output unlocked).
//
// NOTE: While this method would be a bit more readable if the unlock bool was
// reversed (that is, LockUnspent(true, ...) locked the outputs), it has been
// left as unlock to keep compatibility with the reference client API and to
// avoid confusion for those who are already familiar with the lockunspent RPC.
func (c *Client) LockUnspent(unlock bool, ops []*btcwire.OutPoint) error {
	return c.LockUnspentAsync(unlock, ops).Receive()
}

// FutureListLockUnspentResult is a future promise to deliver the result of a
// ListLockUnspentAsync RPC invocation (or an applicable error).
type FutureListLockUnspentResult chan *response

// Receive waits for the response promised by the future and returns the result
// of all currently locked unspent outputs.
func (r FutureListLockUnspentResult) Receive() ([]*btcwire.OutPoint, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal as an array of transaction inputs.
	var inputs []btcjson.TransactionInput
	err = json.Unmarshal(res, &inputs)
	if err != nil {
		return nil, err
	}

	// Create a slice of outpoints from the transaction input structs.
	ops := make([]*btcwire.OutPoint, len(inputs))
	for i, input := range inputs {
		sha, err := btcwire.NewShaHashFromStr(input.Txid)
		if err != nil {
			return nil, err
		}
		ops[i] = btcwire.NewOutPoint(sha, input.Vout)
	}

	return ops, nil
}

// ListLockUnspentAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See ListLockUnspent for the blocking version and more details.
func (c *Client) ListLockUnspentAsync() FutureListLockUnspentResult {
	id := c.NextID()
	cmd, err := btcjson.NewListLockUnspentCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListLockUnspent returns a slice of outpoints for all unspent outputs marked
// as locked by a wallet.  Unspent outputs may be marked locked using
// LockOutput.
func (c *Client) ListLockUnspent() ([]*btcwire.OutPoint, error) {
	return c.ListLockUnspentAsync().Receive()
}

// FutureSetTxFeeResult is a future promise to deliver the result of a
// SetTxFeeAsync RPC invocation (or an applicable error).
type FutureSetTxFeeResult chan *response

// Receive waits for the response promised by the future and returns the result
// of setting an optional transaction fee per KB that helps ensure transactions
// are processed quickly.  Most transaction are 1KB.
func (r FutureSetTxFeeResult) Receive() error {
	_, err := receiveFuture(r)
	if err != nil {
		return err
	}

	return nil
}

// SetTxFeeAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See SetTxFee for the blocking version and more details.
func (c *Client) SetTxFeeAsync(fee btcutil.Amount) FutureSetTxFeeResult {
	id := c.NextID()
	cmd, err := btcjson.NewSetTxFeeCmd(id, int64(fee))
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SetTxFee sets an optional transaction fee per KB that helps ensure
// transactions are processed quickly.  Most transaction are 1KB.
func (c *Client) SetTxFee(fee btcutil.Amount) error {
	return c.SetTxFeeAsync(fee).Receive()
}

// FutureSendToAddressResult is a future promise to deliver the result of a
// SendToAddressAsync RPC invocation (or an applicable error).
type FutureSendToAddressResult chan *response

// Receive waits for the response promised by the future and returns the hash
// of the transaction sending the passed amount to the given address.
func (r FutureSendToAddressResult) Receive() (*btcwire.ShaHash, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var txHash string
	err = json.Unmarshal(res, &txHash)
	if err != nil {
		return nil, err
	}

	return btcwire.NewShaHashFromStr(txHash)
}

// SendToAddressAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See SendToAddress for the blocking version and more details.
func (c *Client) SendToAddressAsync(address btcutil.Address, amount btcutil.Amount) FutureSendToAddressResult {
	id := c.NextID()
	addr := address.EncodeAddress()
	cmd, err := btcjson.NewSendToAddressCmd(id, addr, int64(amount))
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SendToAddress sends the passed amount to the given address.
//
// See SendToAddressComment to associate comments with the transaction in the
// wallet.  The comments are not part of the transaction and are only internal
// to the wallet.
//
// NOTE: This function requires to the wallet to be unlocked.  See the
// WalletPassphrase function for more details.
func (c *Client) SendToAddress(address btcutil.Address, amount btcutil.Amount) (*btcwire.ShaHash, error) {
	return c.SendToAddressAsync(address, amount).Receive()
}

// SendToAddressCommentAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See SendToAddressComment for the blocking version and more details.
func (c *Client) SendToAddressCommentAsync(address btcutil.Address,
	amount btcutil.Amount, comment,
	commentTo string) FutureSendToAddressResult {

	id := c.NextID()
	addr := address.EncodeAddress()
	cmd, err := btcjson.NewSendToAddressCmd(id, addr, int64(amount),
		comment, commentTo)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SendToAddressComment sends the passed amount to the given address and stores
// the provided comment and comment to in the wallet.  The comment parameter is
// intended to be used for the purpose of the transaction while the commentTo
// parameter is indended to be used for who the transaction is being sent to.
//
// The comments are not part of the transaction and are only internal
// to the wallet.
//
// See SendToAddress to avoid using comments.
//
// NOTE: This function requires to the wallet to be unlocked.  See the
// WalletPassphrase function for more details.
func (c *Client) SendToAddressComment(address btcutil.Address, amount btcutil.Amount, comment, commentTo string) (*btcwire.ShaHash, error) {
	return c.SendToAddressCommentAsync(address, amount, comment,
		commentTo).Receive()
}

// FutureSendFromResult is a future promise to deliver the result of a
// SendFromAsync, SendFromMinConfAsync, or SendFromCommentAsync RPC invocation
// (or an applicable error).
type FutureSendFromResult chan *response

// Receive waits for the response promised by the future and returns the hash
// of the transaction sending amount to the given address using the provided
// account as a source of funds.
func (r FutureSendFromResult) Receive() (*btcwire.ShaHash, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var txHash string
	err = json.Unmarshal(res, &txHash)
	if err != nil {
		return nil, err
	}

	return btcwire.NewShaHashFromStr(txHash)
}

// SendFromAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See SendFrom for the blocking version and more details.
func (c *Client) SendFromAsync(fromAccount string, toAddress btcutil.Address, amount btcutil.Amount) FutureSendFromResult {
	id := c.NextID()
	addr := toAddress.EncodeAddress()
	cmd, err := btcjson.NewSendFromCmd(id, fromAccount, addr, int64(amount))
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SendFrom sends the passed amount to the given address using the provided
// account as a source of funds.  Only funds with the default number of minimum
// confirmations will be used.
//
// See SendFromMinConf and SendFromComment for different options.
//
// NOTE: This function requires to the wallet to be unlocked.  See the
// WalletPassphrase function for more details.
func (c *Client) SendFrom(fromAccount string, toAddress btcutil.Address, amount btcutil.Amount) (*btcwire.ShaHash, error) {
	return c.SendFromAsync(fromAccount, toAddress, amount).Receive()
}

// SendFromMinConfAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See SendFromMinConf for the blocking version and more details.
func (c *Client) SendFromMinConfAsync(fromAccount string, toAddress btcutil.Address, amount btcutil.Amount, minConfirms int) FutureSendFromResult {
	id := c.NextID()
	addr := toAddress.EncodeAddress()
	cmd, err := btcjson.NewSendFromCmd(id, fromAccount, addr, int64(amount),
		minConfirms)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SendFromMinConf sends the passed amount to the given address using the
// provided account as a source of funds.  Only funds with the passed number of
// minimum confirmations will be used.
//
// See SendFrom to use the default number of minimum confirmations and
// SendFromComment for additional options.
//
// NOTE: This function requires to the wallet to be unlocked.  See the
// WalletPassphrase function for more details.
func (c *Client) SendFromMinConf(fromAccount string, toAddress btcutil.Address, amount btcutil.Amount, minConfirms int) (*btcwire.ShaHash, error) {
	return c.SendFromMinConfAsync(fromAccount, toAddress, amount,
		minConfirms).Receive()
}

// SendFromCommentAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See SendFromComment for the blocking version and more details.
func (c *Client) SendFromCommentAsync(fromAccount string,
	toAddress btcutil.Address, amount btcutil.Amount, minConfirms int,
	comment, commentTo string) FutureSendFromResult {

	id := c.NextID()
	addr := toAddress.EncodeAddress()
	cmd, err := btcjson.NewSendFromCmd(id, fromAccount, addr, int64(amount),
		minConfirms, comment, commentTo)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SendFromComment sends the passed amount to the given address using the
// provided account as a source of funds and stores the provided comment and
// comment to in the wallet.  The comment parameter is intended to be used for
// the purpose of the transaction while the commentTo parameter is indended to
// be used for who the transaction is being sent to.  Only funds with the passed
// number of minimum confirmations will be used.
//
// See SendFrom and SendFromMinConf to use defaults.
//
// NOTE: This function requires to the wallet to be unlocked.  See the
// WalletPassphrase function for more details.
func (c *Client) SendFromComment(fromAccount string, toAddress btcutil.Address,
	amount btcutil.Amount, minConfirms int,
	comment, commentTo string) (*btcwire.ShaHash, error) {

	return c.SendFromCommentAsync(fromAccount, toAddress, amount,
		minConfirms, comment, commentTo).Receive()
}

// FutureSendManyResult is a future promise to deliver the result of a
// SendManyAsync, SendManyMinConfAsync, or SendManyCommentAsync RPC invocation
// (or an applicable error).
type FutureSendManyResult chan *response

// Receive waits for the response promised by the future and returns the hash
// of the transaction sending multiple amounts to multiple addresses using the
// provided account as a source of funds.
func (r FutureSendManyResult) Receive() (*btcwire.ShaHash, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmashal result as a string.
	var txHash string
	err = json.Unmarshal(res, &txHash)
	if err != nil {
		return nil, err
	}

	return btcwire.NewShaHashFromStr(txHash)
}

// SendManyAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See SendMany for the blocking version and more details.
func (c *Client) SendManyAsync(fromAccount string, amounts map[btcutil.Address]btcutil.Amount) FutureSendManyResult {
	convertedAmounts := make(map[string]int64, len(amounts))
	for addr, amount := range amounts {
		convertedAmounts[addr.EncodeAddress()] = int64(amount)
	}
	id := c.NextID()
	cmd, err := btcjson.NewSendManyCmd(id, fromAccount, convertedAmounts)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SendMany sends multiple amounts to multiple addresses using the provided
// account as a source of funds in a single transaction.  Only funds with the
// default number of minimum confirmations will be used.
//
// See SendManyMinConf and SendManyComment for different options.
//
// NOTE: This function requires to the wallet to be unlocked.  See the
// WalletPassphrase function for more details.
func (c *Client) SendMany(fromAccount string, amounts map[btcutil.Address]btcutil.Amount) (*btcwire.ShaHash, error) {
	return c.SendManyAsync(fromAccount, amounts).Receive()
}

// SendManyMinConfAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See SendManyMinConf for the blocking version and more details.
func (c *Client) SendManyMinConfAsync(fromAccount string,
	amounts map[btcutil.Address]btcutil.Amount,
	minConfirms int) FutureSendManyResult {

	convertedAmounts := make(map[string]int64, len(amounts))
	for addr, amount := range amounts {
		convertedAmounts[addr.EncodeAddress()] = int64(amount)
	}
	id := c.NextID()
	cmd, err := btcjson.NewSendManyCmd(id, fromAccount, convertedAmounts,
		minConfirms)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SendManyMinConf sends multiple amounts to multiple addresses using the
// provided account as a source of funds in a single transaction.  Only funds
// with the passed number of minimum confirmations will be used.
//
// See SendMany to use the default number of minimum confirmations and
// SendManyComment for additional options.
//
// NOTE: This function requires to the wallet to be unlocked.  See the
// WalletPassphrase function for more details.
func (c *Client) SendManyMinConf(fromAccount string,
	amounts map[btcutil.Address]btcutil.Amount,
	minConfirms int) (*btcwire.ShaHash, error) {

	return c.SendManyMinConfAsync(fromAccount, amounts, minConfirms).Receive()
}

// SendManyCommentAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See SendManyComment for the blocking version and more details.
func (c *Client) SendManyCommentAsync(fromAccount string,
	amounts map[btcutil.Address]btcutil.Amount, minConfirms int,
	comment string) FutureSendManyResult {

	convertedAmounts := make(map[string]int64, len(amounts))
	for addr, amount := range amounts {
		convertedAmounts[addr.EncodeAddress()] = int64(amount)
	}
	id := c.NextID()
	cmd, err := btcjson.NewSendManyCmd(id, fromAccount, convertedAmounts,
		minConfirms, comment)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SendManyComment sends multiple amounts to multiple addresses using the
// provided account as a source of funds in a single transaction and stores the
// provided comment in the wallet.  The comment parameter is intended to be used
// for the purpose of the transaction   Only funds with the passed number of
// minimum confirmations will be used.
//
// See SendMany and SendManyMinConf to use defaults.
//
// NOTE: This function requires to the wallet to be unlocked.  See the
// WalletPassphrase function for more details.
func (c *Client) SendManyComment(fromAccount string,
	amounts map[btcutil.Address]btcutil.Amount, minConfirms int,
	comment string) (*btcwire.ShaHash, error) {

	return c.SendManyCommentAsync(fromAccount, amounts, minConfirms,
		comment).Receive()
}

// *************************
// Address/Account Functions
// *************************

// FutureAddMultisigAddressResult is a future promise to deliver the result of a
// AddMultisigAddressAsync RPC invocation (or an applicable error).
type FutureAddMultisigAddressResult chan *response

// Receive waits for the response promised by the future and returns the
// multisignature address that requires the specified number of signatures for
// the provided addresses.
func (r FutureAddMultisigAddressResult) Receive() (btcutil.Address, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var addr string
	err = json.Unmarshal(res, &addr)
	if err != nil {
		return nil, err
	}

	return btcutil.DecodeAddress(addr, &btcnet.MainNetParams)
}

// AddMultisigAddressAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See AddMultisigAddress for the blocking version and more details.
func (c *Client) AddMultisigAddressAsync(requiredSigs int, addresses []btcutil.Address, account string) FutureAddMultisigAddressResult {
	id := c.NextID()

	addrs := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		addrs = append(addrs, addr.String())
	}

	cmd, err := btcjson.NewAddMultisigAddressCmd(id, requiredSigs, addrs, account)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// AddMultisigAddress adds a multisignature address that requires the specified
// number of signatures for the provided addresses to the wallet.
func (c *Client) AddMultisigAddress(requiredSigs int, addresses []btcutil.Address, account string) (btcutil.Address, error) {
	return c.AddMultisigAddressAsync(requiredSigs, addresses,
		account).Receive()
}

// FutureCreateMultisigResult is a future promise to deliver the result of a
// CreateMultisigAsync RPC invocation (or an applicable error).
type FutureCreateMultisigResult chan *response

// Receive waits for the response promised by the future and returns the
// multisignature address and script needed to redeem it.
func (r FutureCreateMultisigResult) Receive() (*btcjson.CreateMultiSigResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a createmultisig result object.
	var multisigRes btcjson.CreateMultiSigResult
	err = json.Unmarshal(res, &multisigRes)
	if err != nil {
		return nil, err
	}

	return &multisigRes, nil
}

// CreateMultisigAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See CreateMultisig for the blocking version and more details.
func (c *Client) CreateMultisigAsync(requiredSigs int, addresses []btcutil.Address) FutureCreateMultisigResult {
	id := c.NextID()

	addrs := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		addrs = append(addrs, addr.String())
	}

	cmd, err := btcjson.NewCreateMultisigCmd(id, requiredSigs, addrs)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// CreateMultisig creates a multisignature address that requires the specified
// number of signatures for the provided addresses and returns the
// multisignature address and script needed to redeem it.
func (c *Client) CreateMultisig(requiredSigs int, addresses []btcutil.Address) (*btcjson.CreateMultiSigResult, error) {
	return c.CreateMultisigAsync(requiredSigs, addresses).Receive()
}

// FutureGetNewAddressResult is a future promise to deliver the result of a
// GetNewAddressAsync RPC invocation (or an applicable error).
type FutureGetNewAddressResult chan *response

// Receive waits for the response promised by the future and returns a new
// address.
func (r FutureGetNewAddressResult) Receive() (btcutil.Address, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var addr string
	err = json.Unmarshal(res, &addr)
	if err != nil {
		return nil, err
	}

	return btcutil.DecodeAddress(addr, &btcnet.MainNetParams)
}

// GetNewAddressAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetNewAddress for the blocking version and more details.
func (c *Client) GetNewAddressAsync() FutureGetNewAddressResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetNewAddressCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetNewAddress returns a new address.
func (c *Client) GetNewAddress() (btcutil.Address, error) {
	return c.GetNewAddressAsync().Receive()
}

// FutureGetRawChangeAddressResult is a future promise to deliver the result of
// a GetRawChangeAddressAsync RPC invocation (or an applicable error).
type FutureGetRawChangeAddressResult chan *response

// Receive waits for the response promised by the future and returns a new
// address for receiving change that will be associated with the provided
// account.  Note that this is only for raw transactions and NOT for normal use.
func (r FutureGetRawChangeAddressResult) Receive() (btcutil.Address, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var addr string
	err = json.Unmarshal(res, &addr)
	if err != nil {
		return nil, err
	}

	return btcutil.DecodeAddress(addr, &btcnet.MainNetParams)
}

// GetRawChangeAddressAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetRawChangeAddress for the blocking version and more details.
func (c *Client) GetRawChangeAddressAsync(account string) FutureGetRawChangeAddressResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetRawChangeAddressCmd(id, account)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetRawChangeAddress returns a new address for receiving change that will be
// associated with the provided account.  Note that this is only for raw
// transactions and NOT for normal use.
func (c *Client) GetRawChangeAddress(account string) (btcutil.Address, error) {
	return c.GetRawChangeAddressAsync(account).Receive()
}

// FutureGetAccountAddressResult is a future promise to deliver the result of a
// GetAccountAddressAsync RPC invocation (or an applicable error).
type FutureGetAccountAddressResult chan *response

// Receive waits for the response promised by the future and returns the current
// Bitcoin address for receiving payments to the specified account.
func (r FutureGetAccountAddressResult) Receive() (btcutil.Address, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var addr string
	err = json.Unmarshal(res, &addr)
	if err != nil {
		return nil, err
	}

	return btcutil.DecodeAddress(addr, &btcnet.MainNetParams)
}

// GetAccountAddressAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetAccountAddress for the blocking version and more details.
func (c *Client) GetAccountAddressAsync(account string) FutureGetAccountAddressResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetAccountAddressCmd(id, account)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetAccountAddress returns the current Bitcoin address for receiving payments
// to the specified account.
func (c *Client) GetAccountAddress(account string) (btcutil.Address, error) {
	return c.GetAccountAddressAsync(account).Receive()
}

// FutureGetAccountResult is a future promise to deliver the result of a
// GetAccountAsync RPC invocation (or an applicable error).
type FutureGetAccountResult chan *response

// Receive waits for the response promised by the future and returns the account
// associated with the passed address.
func (r FutureGetAccountResult) Receive() (string, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return "", err
	}

	// Unmarshal result as a string.
	var account string
	err = json.Unmarshal(res, &account)
	if err != nil {
		return "", err
	}

	return account, nil
}

// GetAccountAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetAccount for the blocking version and more details.
func (c *Client) GetAccountAsync(address btcutil.Address) FutureGetAccountResult {
	id := c.NextID()
	addr := address.EncodeAddress()
	cmd, err := btcjson.NewGetAccountCmd(id, addr)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetAccount returns the account associated with the passed address.
func (c *Client) GetAccount(address btcutil.Address) (string, error) {
	return c.GetAccountAsync(address).Receive()
}

// FutureSetAccountResult is a future promise to deliver the result of a
// SetAccountAsync RPC invocation (or an applicable error).
type FutureSetAccountResult chan *response

// Receive waits for the response promised by the future and returns the result
// of setting the account to be associated with the passed address.
func (r FutureSetAccountResult) Receive() error {
	_, err := receiveFuture(r)
	if err != nil {
		return err
	}

	return nil
}

// SetAccountAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See SetAccount for the blocking version and more details.
func (c *Client) SetAccountAsync(address btcutil.Address, account string) FutureSetAccountResult {
	id := c.NextID()
	addr := address.EncodeAddress()
	cmd, err := btcjson.NewSetAccountCmd(id, addr, account)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SetAccount sets the account associated with the passed address.
func (c *Client) SetAccount(address btcutil.Address, account string) error {
	return c.SetAccountAsync(address, account).Receive()
}

// FutureGetAddressesByAccountResult is a future promise to deliver the result
// of a GetAddressesByAccountAsync RPC invocation (or an applicable error).
type FutureGetAddressesByAccountResult chan *response

// Receive waits for the response promised by the future and returns the list of
// addresses associated with the passed account.
func (r FutureGetAddressesByAccountResult) Receive() ([]btcutil.Address, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmashal result as an array of string.
	var addrStrings []string
	err = json.Unmarshal(res, &addrStrings)
	if err != nil {
		return nil, err
	}

	addrs := make([]btcutil.Address, 0, len(addrStrings))
	for _, addrStr := range addrStrings {
		addr, err := btcutil.DecodeAddress(addrStr, &btcnet.MainNetParams)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, addr)
	}

	return addrs, nil
}

// GetAddressesByAccountAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetAddressesByAccount for the blocking version and more details.
func (c *Client) GetAddressesByAccountAsync(account string) FutureGetAddressesByAccountResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetAddressesByAccountCmd(id, account)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetAddressesByAccount returns the list of addresses associated with the
// passed account.
func (c *Client) GetAddressesByAccount(account string) ([]btcutil.Address, error) {
	return c.GetAddressesByAccountAsync(account).Receive()
}

// FutureMoveResult is a future promise to deliver the result of a MoveAsync,
// MoveMinConfAsync, or MoveCommentAsync RPC invocation (or an applicable
// error).
type FutureMoveResult chan *response

// Receive waits for the response promised by the future and returns the result
// of the move operation.
func (r FutureMoveResult) Receive() (bool, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return false, err
	}

	// Unmarshal result as a boolean.
	var moveResult bool
	err = json.Unmarshal(res, &moveResult)
	if err != nil {
		return false, err
	}

	return moveResult, nil
}

// MoveAsync returns an instance of a type that can be used to get the result of
// the RPC at some future time by invoking the Receive function on the returned
// instance.
//
// See Move for the blocking version and more details.
func (c *Client) MoveAsync(fromAccount, toAccount string, amount btcutil.Amount) FutureMoveResult {
	id := c.NextID()
	cmd, err := btcjson.NewMoveCmd(id, fromAccount, toAccount, int64(amount))
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// Move moves specified amount from one account in your wallet to another.  Only
// funds with the default number of minimum confirmations will be used.
//
// See MoveMinConf and MoveComment for different options.
func (c *Client) Move(fromAccount, toAccount string, amount btcutil.Amount) (bool, error) {
	return c.MoveAsync(fromAccount, toAccount, amount).Receive()
}

// MoveMinConfAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See MoveMinConf for the blocking version and more details.
func (c *Client) MoveMinConfAsync(fromAccount, toAccount string,
	amount btcutil.Amount, minConfirms int) FutureMoveResult {

	id := c.NextID()
	cmd, err := btcjson.NewMoveCmd(id, fromAccount, toAccount, int64(amount),
		minConfirms)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// MoveMinConf moves specified amount from one account in your wallet to
// another.  Only funds with the passed number of minimum confirmations will be
// used.
//
// See Move to use the default number of minimum confirmations and MoveComment
// for additional options.
func (c *Client) MoveMinConf(fromAccount, toAccount string, amount btcutil.Amount, minConf int) (bool, error) {
	return c.MoveMinConfAsync(fromAccount, toAccount, amount, minConf).Receive()
}

// MoveCommentAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See MoveComment for the blocking version and more details.
func (c *Client) MoveCommentAsync(fromAccount, toAccount string,
	amount btcutil.Amount, minConfirms int, comment string) FutureMoveResult {

	id := c.NextID()
	cmd, err := btcjson.NewMoveCmd(id, fromAccount, toAccount, int64(amount),
		minConfirms, comment)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// MoveComment moves specified amount from one account in your wallet to
// another and stores the provided comment in the wallet.  The comment
// parameter is only available in the wallet.  Only funds with the passed number
// of minimum confirmations will be used.
//
// See Move and MoveMinConf to use defaults.
func (c *Client) MoveComment(fromAccount, toAccount string, amount btcutil.Amount,
	minConf int, comment string) (bool, error) {

	return c.MoveCommentAsync(fromAccount, toAccount, amount, minConf,
		comment).Receive()
}

// FutureValidateAddressResult is a future promise to deliver the result of a
// ValidateAddressAsync RPC invocation (or an applicable error).
type FutureValidateAddressResult chan *response

// Receive waits for the response promised by the future and returns information
// about the given bitcoin address.
func (r FutureValidateAddressResult) Receive() (*btcjson.ValidateAddressResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a validateaddress result object.
	var addrResult btcjson.ValidateAddressResult
	err = json.Unmarshal(res, &addrResult)
	if err != nil {
		return nil, err
	}

	return &addrResult, nil
}

// ValidateAddressAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See ValidateAddress for the blocking version and more details.
func (c *Client) ValidateAddressAsync(address btcutil.Address) FutureValidateAddressResult {
	id := c.NextID()
	addr := address.EncodeAddress()
	cmd, err := btcjson.NewValidateAddressCmd(id, addr)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ValidateAddress returns information about the given bitcoin address.
func (c *Client) ValidateAddress(address btcutil.Address) (*btcjson.ValidateAddressResult, error) {
	return c.ValidateAddressAsync(address).Receive()
}

// FutureKeyPoolRefillResult is a future promise to deliver the result of a
// KeyPoolRefillAsync RPC invocation (or an applicable error).
type FutureKeyPoolRefillResult chan *response

// Receive waits for the response promised by the future and returns the result
// of refilling the key pool.
func (r FutureKeyPoolRefillResult) Receive() error {
	_, err := receiveFuture(r)
	if err != nil {
		return err
	}

	return nil
}

// KeyPoolRefillAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See KeyPoolRefill for the blocking version and more details.
func (c *Client) KeyPoolRefillAsync() FutureKeyPoolRefillResult {
	id := c.NextID()
	cmd, err := btcjson.NewKeyPoolRefillCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// KeyPoolRefill fills the key pool as necessary to reach the default size.
//
// See KeyPoolRefillSize to override the size of the key pool.
func (c *Client) KeyPoolRefill() error {
	return c.KeyPoolRefillAsync().Receive()
}

// KeyPoolRefillSizeAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See KeyPoolRefillSize for the blocking version and more details.
func (c *Client) KeyPoolRefillSizeAsync(newSize uint) FutureKeyPoolRefillResult {
	id := c.NextID()
	cmd, err := btcjson.NewKeyPoolRefillCmd(id, newSize)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// KeyPoolRefillSize fills the key pool as necessary to reach the specified
// size.
func (c *Client) KeyPoolRefillSize(newSize uint) error {
	return c.KeyPoolRefillSizeAsync(newSize).Receive()
}

// ************************
// Amount/Balance Functions
// ************************

// FutureListAccountsResult is a future promise to deliver the result of a
// ListAccountsAsync or ListAccountsMinConfAsync RPC invocation (or an
// applicable error).
type FutureListAccountsResult chan *response

// Receive waits for the response promised by the future and returns returns a
// map of account names and their associated balances.
func (r FutureListAccountsResult) Receive() (map[string]btcutil.Amount, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a json object.
	var accounts map[string]float64
	err = json.Unmarshal(res, &accounts)
	if err != nil {
		return nil, err
	}

	accountsMap := make(map[string]btcutil.Amount)
	for k, v := range accounts {
		satoshi, err := btcjson.JSONToAmount(v)
		if err != nil {
			return nil, err
		}

		accountsMap[k] = btcutil.Amount(satoshi)
	}

	return accountsMap, nil
}

// ListAccountsAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See ListAccounts for the blocking version and more details.
func (c *Client) ListAccountsAsync() FutureListAccountsResult {
	id := c.NextID()
	cmd, err := btcjson.NewListAccountsCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListAccounts returns a map of account names and their associated balances
// using the default number of minimum confirmations.
//
// See ListAccountsMinConf to override the minimum number of confirmations.
func (c *Client) ListAccounts() (map[string]btcutil.Amount, error) {
	return c.ListAccountsAsync().Receive()
}

// ListAccountsMinConfAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See ListAccountsMinConf for the blocking version and more details.
func (c *Client) ListAccountsMinConfAsync(minConfirms int) FutureListAccountsResult {
	id := c.NextID()
	cmd, err := btcjson.NewListAccountsCmd(id, minConfirms)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListAccountsMinConf returns a map of account names and their associated
// balances using the specified number of minimum confirmations.
//
// See ListAccounts to use the default minimum number of confirmations.
func (c *Client) ListAccountsMinConf(minConfirms int) (map[string]btcutil.Amount, error) {
	return c.ListAccountsMinConfAsync(minConfirms).Receive()
}

// FutureGetBalanceResult is a future promise to deliver the result of a
// GetBalanceAsync or GetBalanceMinConfAsync RPC invocation (or an applicable
// error).
type FutureGetBalanceResult chan *response

// Receive waits for the response promised by the future and returns the
// available balance from the server for the specified account.
func (r FutureGetBalanceResult) Receive() (btcutil.Amount, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return 0, err
	}

	// Unmarshal result as a floating point number.
	var balance float64
	err = json.Unmarshal(res, &balance)
	if err != nil {
		return 0, err
	}

	satoshi, err := btcjson.JSONToAmount(balance)
	if err != nil {
		return 0, err
	}

	return btcutil.Amount(satoshi), nil
}

// FutureGetBalanceParseResult is same as FutureGetBalanceResult except
// that the result is expected to be a string which is then parsed into
// a float64 value
// This is required for compatiblity with servers like blockchain.info
type FutureGetBalanceParseResult chan *response

// Receive waits for the response promised by the future and returns the
// available balance from the server for the specified account.
func (r FutureGetBalanceParseResult) Receive() (btcutil.Amount, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return 0, err
	}

	// Unmarshal result as a string
	var balanceString string
	err = json.Unmarshal(res, &balanceString)
	if err != nil {
		return 0, err
	}

	balance, err := strconv.ParseFloat(balanceString, 64)
	if err != nil {
		return 0, err
	}
	satoshi, err := btcjson.JSONToAmount(balance)
	if err != nil {
		return 0, err
	}

	return btcutil.Amount(satoshi), nil
}

// GetBalanceAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBalance for the blocking version and more details.
func (c *Client) GetBalanceAsync(account string) FutureGetBalanceResult {
	// TODO(davec): Remove this hack once btcwallet is fixed.
	if account == "*" {
		account = ""
	}
	id := c.NextID()
	cmd, err := btcjson.NewGetBalanceCmd(id, account)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetBalance returns the available balance from the server for the specified
// account using the default number of minimum confirmations.  The account may
// be "*" for all accounts.
//
// See GetBalanceMinConf to override the minimum number of confirmations.
func (c *Client) GetBalance(account string) (btcutil.Amount, error) {
	return c.GetBalanceAsync(account).Receive()
}

// GetBalanceMinConfAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetBalanceMinConf for the blocking version and more details.
func (c *Client) GetBalanceMinConfAsync(account string, minConfirms int) FutureGetBalanceResult {
	// TODO(davec): Remove this hack once btcwallet is fixed.
	if account == "*" {
		account = ""
	}
	id := c.NextID()
	cmd, err := btcjson.NewGetBalanceCmd(id, account, minConfirms)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetBalanceMinConf returns the available balance from the server for the
// specified account using the specified number of minimum confirmations.  The
// account may be "*" for all accounts.
//
// See GetBalance to use the default minimum number of confirmations.
func (c *Client) GetBalanceMinConf(account string, minConfirms int) (btcutil.Amount, error) {
	if c.config.EnableBCInfoHacks {
		response := c.GetBalanceMinConfAsync(account, minConfirms)
		return FutureGetBalanceParseResult(response).Receive()
	}
	return c.GetBalanceMinConfAsync(account, minConfirms).Receive()
}

// FutureGetReceivedByAccountResult is a future promise to deliver the result of
// a GetReceivedByAccountAsync or GetReceivedByAccountMinConfAsync RPC
// invocation (or an applicable error).
type FutureGetReceivedByAccountResult chan *response

// Receive waits for the response promised by the future and returns the total
// amount received with the specified account.
func (r FutureGetReceivedByAccountResult) Receive() (btcutil.Amount, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return 0, err
	}

	// Unmarshal result as a floating point number.
	var balance float64
	err = json.Unmarshal(res, &balance)
	if err != nil {
		return 0, err
	}

	satoshi, err := btcjson.JSONToAmount(balance)
	if err != nil {
		return 0, err
	}

	return btcutil.Amount(satoshi), nil
}

// GetReceivedByAccountAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetReceivedByAccount for the blocking version and more details.
func (c *Client) GetReceivedByAccountAsync(account string) FutureGetReceivedByAccountResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetReceivedByAccountCmd(id, account)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetReceivedByAccount returns the total amount received with the specified
// account with at least the default number of minimum confirmations.
//
// See GetReceivedByAccountMinConf to override the minimum number of
// confirmations.
func (c *Client) GetReceivedByAccount(account string) (btcutil.Amount, error) {
	return c.GetReceivedByAccountAsync(account).Receive()
}

// GetReceivedByAccountMinConfAsync returns an instance of a type that can be
// used to get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetReceivedByAccountMinConf for the blocking version and more details.
func (c *Client) GetReceivedByAccountMinConfAsync(account string, minConfirms int) FutureGetReceivedByAccountResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetReceivedByAccountCmd(id, account, minConfirms)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetReceivedByAccountMinConf returns the total amount received with the
// specified account with at least the specified number of minimum
// confirmations.
//
// See GetReceivedByAccount to use the default minimum number of confirmations.
func (c *Client) GetReceivedByAccountMinConf(account string, minConfirms int) (btcutil.Amount, error) {
	return c.GetReceivedByAccountMinConfAsync(account, minConfirms).Receive()
}

// FutureGetUnconfirmedBalanceResult is a future promise to deliver the result
// of a GetUnconfirmedBalanceAsync RPC invocation (or an applicable error).
type FutureGetUnconfirmedBalanceResult chan *response

// Receive waits for the response promised by the future and returns returns the
// unconfirmed balance from the server for the specified account.
func (r FutureGetUnconfirmedBalanceResult) Receive() (btcutil.Amount, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return 0, err
	}

	// Unmarshal result as a floating point number.
	var balance float64
	err = json.Unmarshal(res, &balance)
	if err != nil {
		return 0, err
	}

	satoshi, err := btcjson.JSONToAmount(balance)
	if err != nil {
		return 0, err
	}

	return btcutil.Amount(satoshi), nil
}

// GetUnconfirmedBalanceAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetUnconfirmedBalance for the blocking version and more details.
func (c *Client) GetUnconfirmedBalanceAsync(account string) FutureGetUnconfirmedBalanceResult {
	id := c.NextID()
	cmd, err := btcws.NewGetUnconfirmedBalanceCmd(id, account)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetUnconfirmedBalance returns the unconfirmed balance from the server for
// the specified account.
func (c *Client) GetUnconfirmedBalance(account string) (btcutil.Amount, error) {
	return c.GetUnconfirmedBalanceAsync(account).Receive()
}

// FutureGetReceivedByAddressResult is a future promise to deliver the result of
// a GetReceivedByAddressAsync or GetReceivedByAddressMinConfAsync RPC
// invocation (or an applicable error).
type FutureGetReceivedByAddressResult chan *response

// Receive waits for the response promised by the future and returns the total
// amount received by the specified address.
func (r FutureGetReceivedByAddressResult) Receive() (btcutil.Amount, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return 0, err
	}

	// Unmarshal result as a floating point number.
	var balance float64
	err = json.Unmarshal(res, &balance)
	if err != nil {
		return 0, err
	}

	satoshi, err := btcjson.JSONToAmount(balance)
	if err != nil {
		return 0, err
	}

	return btcutil.Amount(satoshi), nil
}

// GetReceivedByAddressAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetReceivedByAddress for the blocking version and more details.
func (c *Client) GetReceivedByAddressAsync(address btcutil.Address) FutureGetReceivedByAddressResult {
	id := c.NextID()
	addr := address.EncodeAddress()
	cmd, err := btcjson.NewGetReceivedByAddressCmd(id, addr)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)

}

// GetReceivedByAddress returns the total amount received by the specified
// address with at least the default number of minimum confirmations.
//
// See GetReceivedByAddressMinConf to override the minimum number of
// confirmations.
func (c *Client) GetReceivedByAddress(address btcutil.Address) (btcutil.Amount, error) {
	return c.GetReceivedByAddressAsync(address).Receive()
}

// GetReceivedByAddressMinConfAsync returns an instance of a type that can be
// used to get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetReceivedByAddressMinConf for the blocking version and more details.
func (c *Client) GetReceivedByAddressMinConfAsync(address btcutil.Address, minConfirms int) FutureGetReceivedByAddressResult {
	id := c.NextID()
	addr := address.EncodeAddress()
	cmd, err := btcjson.NewGetReceivedByAddressCmd(id, addr, minConfirms)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetReceivedByAddressMinConf returns the total amount received by the specified
// address with at least the specified number of minimum confirmations.
//
// See GetReceivedByAddress to use the default minimum number of confirmations.
func (c *Client) GetReceivedByAddressMinConf(address btcutil.Address, minConfirms int) (btcutil.Amount, error) {
	return c.GetReceivedByAddressMinConfAsync(address, minConfirms).Receive()
}

// FutureListReceivedByAccountResult is a future promise to deliver the result
// of a ListReceivedByAccountAsync, ListReceivedByAccountMinConfAsync, or
// ListReceivedByAccountIncludeEmptyAsync RPC invocation (or an applicable
// error).
type FutureListReceivedByAccountResult chan *response

// Receive waits for the response promised by the future and returns a list of
// balances by account.
func (r FutureListReceivedByAccountResult) Receive() ([]btcjson.ListReceivedByAccountResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal as an array of listreceivedbyaccount result objects.
	var received []btcjson.ListReceivedByAccountResult
	err = json.Unmarshal(res, &received)
	if err != nil {
		return nil, err
	}

	return received, nil
}

// ListReceivedByAccountAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See ListReceivedByAccount for the blocking version and more details.
func (c *Client) ListReceivedByAccountAsync() FutureListReceivedByAccountResult {
	id := c.NextID()
	cmd, err := btcjson.NewListReceivedByAccountCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListReceivedByAccount lists balances by account using the default number
// of minimum confirmations and including accounts that haven't received any
// payments.
//
// See ListReceivedByAccountMinConf to override the minimum number of
// confirmations and ListReceivedByAccountIncludeEmpty to filter accounts that
// haven't received any payments from the results.
func (c *Client) ListReceivedByAccount() ([]btcjson.ListReceivedByAccountResult, error) {
	return c.ListReceivedByAccountAsync().Receive()
}

// ListReceivedByAccountMinConfAsync returns an instance of a type that can be
// used to get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See ListReceivedByAccountMinConf for the blocking version and more details.
func (c *Client) ListReceivedByAccountMinConfAsync(minConfirms int) FutureListReceivedByAccountResult {
	id := c.NextID()
	cmd, err := btcjson.NewListReceivedByAccountCmd(id, minConfirms)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListReceivedByAccountMinConf lists balances by account using the specified
// number of minimum confirmations not including accounts that haven't received
// any payments.
//
// See ListReceivedByAccount to use the default minimum number of confirmations
// and ListReceivedByAccountIncludeEmpty to also include accounts that haven't
// received any payments in the results.
func (c *Client) ListReceivedByAccountMinConf(minConfirms int) ([]btcjson.ListReceivedByAccountResult, error) {
	return c.ListReceivedByAccountMinConfAsync(minConfirms).Receive()
}

// ListReceivedByAccountIncludeEmptyAsync returns an instance of a type that can
// be used to get the result of the RPC at some future time by invoking the
// Receive function on the returned instance.
//
// See ListReceivedByAccountIncludeEmpty for the blocking version and more details.
func (c *Client) ListReceivedByAccountIncludeEmptyAsync(minConfirms int, includeEmpty bool) FutureListReceivedByAccountResult {
	id := c.NextID()
	cmd, err := btcjson.NewListReceivedByAccountCmd(id, minConfirms, includeEmpty)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListReceivedByAccountIncludeEmpty lists balances by account using the
// specified number of minimum confirmations and including accounts that
// haven't received any payments depending on specified flag.
//
// See ListReceivedByAccount and ListReceivedByAccountMinConf to use defaults.
func (c *Client) ListReceivedByAccountIncludeEmpty(minConfirms int, includeEmpty bool) ([]btcjson.ListReceivedByAccountResult, error) {
	return c.ListReceivedByAccountIncludeEmptyAsync(minConfirms,
		includeEmpty).Receive()
}

// FutureListReceivedByAddressResult is a future promise to deliver the result
// of a ListReceivedByAddressAsync, ListReceivedByAddressMinConfAsync, or
// ListReceivedByAddressIncludeEmptyAsync RPC invocation (or an applicable
// error).
type FutureListReceivedByAddressResult chan *response

// Receive waits for the response promised by the future and returns a list of
// balances by address.
func (r FutureListReceivedByAddressResult) Receive() ([]btcjson.ListReceivedByAddressResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal as an array of listreceivedbyaddress result objects.
	var received []btcjson.ListReceivedByAddressResult
	err = json.Unmarshal(res, &received)
	if err != nil {
		return nil, err
	}

	return received, nil
}

// ListReceivedByAddressAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See ListReceivedByAddress for the blocking version and more details.
func (c *Client) ListReceivedByAddressAsync() FutureListReceivedByAddressResult {
	id := c.NextID()
	cmd, err := btcjson.NewListReceivedByAddressCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListReceivedByAddress lists balances by address using the default number
// of minimum confirmations not including addresses that haven't received any
// payments or watching only addresses.
//
// See ListReceivedByAddressMinConf to override the minimum number of
// confirmations and ListReceivedByAddressIncludeEmpty to also include addresses
// that haven't received any payments in the results.
func (c *Client) ListReceivedByAddress() ([]btcjson.ListReceivedByAddressResult, error) {
	return c.ListReceivedByAddressAsync().Receive()
}

// ListReceivedByAddressMinConfAsync returns an instance of a type that can be
// used to get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See ListReceivedByAddressMinConf for the blocking version and more details.
func (c *Client) ListReceivedByAddressMinConfAsync(minConfirms int) FutureListReceivedByAddressResult {
	id := c.NextID()
	cmd, err := btcjson.NewListReceivedByAddressCmd(id, minConfirms)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListReceivedByAddressMinConf lists balances by address using the specified
// number of minimum confirmations not including addresses that haven't received
// any payments.
//
// See ListReceivedByAddress to use the default minimum number of confirmations
// and ListReceivedByAddressIncludeEmpty to also include addresses that haven't
// received any payments in the results.
func (c *Client) ListReceivedByAddressMinConf(minConfirms int) ([]btcjson.ListReceivedByAddressResult, error) {
	return c.ListReceivedByAddressMinConfAsync(minConfirms).Receive()
}

// ListReceivedByAccountIncludeEmptyAsync returns an instance of a type that can
// be used to get the result of the RPC at some future time by invoking the
// Receive function on the returned instance.
//
// See ListReceivedByAccountIncludeEmpty for the blocking version and more details.
func (c *Client) ListReceivedByAddressIncludeEmptyAsync(minConfirms int, includeEmpty bool) FutureListReceivedByAddressResult {
	id := c.NextID()
	cmd, err := btcjson.NewListReceivedByAddressCmd(id, minConfirms,
		includeEmpty)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ListReceivedByAddressIncludeEmpty lists balances by address using the
// specified number of minimum confirmations and including addresses that
// haven't received any payments depending on specified flag.
//
// See ListReceivedByAddress and ListReceivedByAddressMinConf to use defaults.
func (c *Client) ListReceivedByAddressIncludeEmpty(minConfirms int, includeEmpty bool) ([]btcjson.ListReceivedByAddressResult, error) {
	return c.ListReceivedByAddressIncludeEmptyAsync(minConfirms,
		includeEmpty).Receive()
}

// ************************
// Wallet Locking Functions
// ************************

// FutureWalletLockResult is a future promise to deliver the result of a
// WalletLockAsync RPC invocation (or an applicable error).
type FutureWalletLockResult chan *response

// Receive waits for the response promised by the future and returns the result
// of locking the wallet.
func (r FutureWalletLockResult) Receive() error {
	_, err := receiveFuture(r)
	if err != nil {
		return err
	}

	return nil
}

// WalletLockAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See WalletLock for the blocking version and more details.
func (c *Client) WalletLockAsync() FutureWalletLockResult {
	id := c.NextID()
	cmd, err := btcjson.NewWalletLockCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// WalletLock locks the wallet by removing the encryption key from memory.
//
// After calling this function, the WalletPassphrase function must be used to
// unlock the wallet prior to calling any other function which requires the
// wallet to be unlocked.
func (c *Client) WalletLock() error {
	return c.WalletLockAsync().Receive()
}

// WalletPassphrase unlocks the wallet by using the passphrase to derive the
// decryption key which is then stored in memory for the specified timeout
// (in seconds).
func (c *Client) WalletPassphrase(passphrase string, timeoutSecs int64) error {
	id := c.NextID()
	cmd, err := btcjson.NewWalletPassphraseCmd(id, passphrase, timeoutSecs)
	if err != nil {
		return err
	}

	_, err = c.sendCmdAndWait(cmd)
	if err != nil {
		return err
	}

	return nil
}

// FutureWalletPassphraseChangeResult is a future promise to deliver the result
// of a WalletPassphraseChangeAsync RPC invocation (or an applicable error).
type FutureWalletPassphraseChangeResult chan *response

// Receive waits for the response promised by the future and returns the result
// of changing the wallet passphrase.
func (r FutureWalletPassphraseChangeResult) Receive() error {
	_, err := receiveFuture(r)
	if err != nil {
		return err
	}

	return nil
}

// WalletPassphraseChangeAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See WalletPassphraseChange for the blocking version and more details.
func (c *Client) WalletPassphraseChangeAsync(old, new string) FutureWalletPassphraseChangeResult {
	id := c.NextID()
	cmd, err := btcjson.NewWalletPassphraseChangeCmd(id, old, new)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// WalletPassphraseChange changes the wallet passphrase from the specified old
// to new passphrase.
func (c *Client) WalletPassphraseChange(old, new string) error {
	return c.WalletPassphraseChangeAsync(old, new).Receive()
}

// *************************
// Message Signing Functions
// *************************

// FutureSignMessageResult is a future promise to deliver the result of a
// SignMessageAsync RPC invocation (or an applicable error).
type FutureSignMessageResult chan *response

// Receive waits for the response promised by the future and returns the message
// signed with the private key of the specified address.
func (r FutureSignMessageResult) Receive() (string, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return "", err
	}

	// Unmarshal result as a string.
	var b64 string
	err = json.Unmarshal(res, &b64)
	if err != nil {
		return "", err
	}

	return b64, nil
}

// SignMessageAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See SignMessage for the blocking version and more details.
func (c *Client) SignMessageAsync(address btcutil.Address, message string) FutureSignMessageResult {
	id := c.NextID()
	addr := address.EncodeAddress()
	cmd, err := btcjson.NewSignMessageCmd(id, addr, message)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SignMessage signs a message with the private key of the specified address.
//
// NOTE: This function requires to the wallet to be unlocked.  See the
// WalletPassphrase function for more details.
func (c *Client) SignMessage(address btcutil.Address, message string) (string, error) {
	return c.SignMessageAsync(address, message).Receive()
}

// FutureVerifyMessageResult is a future promise to deliver the result of a
// VerifyMessageAsync RPC invocation (or an applicable error).
type FutureVerifyMessageResult chan *response

// Receive waits for the response promised by the future and returns whether or
// not the message was successfully verified.
func (r FutureVerifyMessageResult) Receive() (bool, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return false, err
	}

	// Unmarshal result as a boolean.
	var verified bool
	err = json.Unmarshal(res, &verified)
	if err != nil {
		return false, err
	}

	return verified, nil
}

// VerifyMessageAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See VerifyMessage for the blocking version and more details.
func (c *Client) VerifyMessageAsync(address btcutil.Address, signature, message string) FutureVerifyMessageResult {
	id := c.NextID()
	addr := address.EncodeAddress()
	cmd, err := btcjson.NewVerifyMessageCmd(id, addr, signature, message)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// VerifyMessage verifies a signed message.
//
// NOTE: This function requires to the wallet to be unlocked.  See the
// WalletPassphrase function for more details.
func (c *Client) VerifyMessage(address btcutil.Address, signature, message string) (bool, error) {
	return c.VerifyMessageAsync(address, signature, message).Receive()
}

// *********************
// Dump/Import Functions
// *********************

// FutureDumpPrivKeyResult is a future promise to deliver the result of a
// DumpPrivKeyAsync RPC invocation (or an applicable error).
type FutureDumpPrivKeyResult chan *response

// Receive waits for the response promised by the future and returns the private
// key corresponding to the passed address encoded in the wallet import format
// (WIF)
func (r FutureDumpPrivKeyResult) Receive() (*btcutil.WIF, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var privKeyWIF string
	err = json.Unmarshal(res, &privKeyWIF)
	if err != nil {
		return nil, err
	}

	return btcutil.DecodeWIF(privKeyWIF)
}

// DumpPrivKeyAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See DumpPrivKey for the blocking version and more details.
func (c *Client) DumpPrivKeyAsync(address btcutil.Address) FutureDumpPrivKeyResult {
	id := c.NextID()
	addr := address.EncodeAddress()
	cmd, err := btcjson.NewDumpPrivKeyCmd(id, addr)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// DumpPrivKey gets the private key corresponding to the passed address encoded
// in the wallet import format (WIF).
//
// NOTE: This function requires to the wallet to be unlocked.  See the
// WalletPassphrase function for more details.
func (c *Client) DumpPrivKey(address btcutil.Address) (*btcutil.WIF, error) {
	return c.DumpPrivKeyAsync(address).Receive()
}

// FutureImportPrivKeyResult is a future promise to deliver the result of an
// ImportPrivKeyAsync RPC invocation (or an applicable error).
type FutureImportPrivKeyResult chan *response

// Receive waits for the response promised by the future and returns the result
// of importing the passed private key which must be the wallet import format
// (WIF).
func (r FutureImportPrivKeyResult) Receive() error {
	_, err := receiveFuture(r)
	if err != nil {
		return err
	}

	return nil
}

// ImportPrivKeyAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See ImportPrivKey for the blocking version and more details.
func (c *Client) ImportPrivKeyAsync(privKeyWIF *btcutil.WIF) FutureImportPrivKeyResult {
	wif := ""
	if privKeyWIF != nil {
		wif = privKeyWIF.String()
	}

	id := c.NextID()
	cmd, err := btcjson.NewImportPrivKeyCmd(id, wif)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ImportPrivKey imports the passed private key which must be the wallet import
// format (WIF).
func (c *Client) ImportPrivKey(privKeyWIF *btcutil.WIF) error {
	return c.ImportPrivKeyAsync(privKeyWIF).Receive()
}

// ImportPrivKeyLabelAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See ImportPrivKey for the blocking version and more details.
func (c *Client) ImportPrivKeyLabelAsync(privKeyWIF *btcutil.WIF, label string) FutureImportPrivKeyResult {
	wif := ""
	if privKeyWIF != nil {
		wif = privKeyWIF.String()
	}

	id := c.NextID()
	cmd, err := btcjson.NewImportPrivKeyCmd(id, wif, label)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ImportPrivKeyLabel imports the passed private key which must be the wallet import
// format (WIF). It sets the account label to the one provided.
func (c *Client) ImportPrivKeyLabel(privKeyWIF *btcutil.WIF, label string) error {
	return c.ImportPrivKeyLabelAsync(privKeyWIF, label).Receive()
}

// ImportPrivKeyRescanAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See ImportPrivKey for the blocking version and more details.
func (c *Client) ImportPrivKeyRescanAsync(privKeyWIF *btcutil.WIF, label string, rescan bool) FutureImportPrivKeyResult {
	wif := ""
	if privKeyWIF != nil {
		wif = privKeyWIF.String()
	}

	id := c.NextID()
	cmd, err := btcjson.NewImportPrivKeyCmd(id, wif, label, rescan)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// ImportPrivKeyRescan imports the passed private key which must be the wallet import
// format (WIF). It sets the account label to the one provided. When rescan is true,
// the block history is scanned for transactions addressed to provided privKey.
func (c *Client) ImportPrivKeyRescan(privKeyWIF *btcutil.WIF, label string, rescan bool) error {
	return c.ImportPrivKeyRescanAsync(privKeyWIF, label, rescan).Receive()
}

// ***********************
// Miscellaneous Functions
// ***********************

// NOTE: While getinfo is implemented here (in wallet.go), a btcd chain server
// will respond to getinfo requests as well, excluding any wallet information.

// FutureGetInfoResult is a future promise to deliver the result of a
// GetInfoAsync RPC invocation (or an applicable error).
type FutureGetInfoResult chan *response

// Receive waits for the response promised by the future and returns the info
// provided by the server.
func (r FutureGetInfoResult) Receive() (*btcjson.InfoResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a getinfo result object.
	var infoRes btcjson.InfoResult
	err = json.Unmarshal(res, &infoRes)
	if err != nil {
		return nil, err
	}

	return &infoRes, nil
}

// GetInfoAsync returns an instance of a type that can be used to get the result
// of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetInfo for the blocking version and more details.
func (c *Client) GetInfoAsync() FutureGetInfoResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetInfoCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetInfo returns miscellaneous info regarding the RPC server.  The returned
// info object may be void of wallet information if the remote server does
// not include wallet functionality.
func (c *Client) GetInfo() (*btcjson.InfoResult, error) {
	return c.GetInfoAsync().Receive()
}

// TODO(davec): Implement
// backupwallet (NYI in btcwallet)
// encryptwallet (Won't be supported by btcwallet since it's always encrypted)
// getwalletinfo (NYI in btcwallet or btcjson)
// listaddressgroupings (NYI in btcwallet)
// listreceivedbyaccount (NYI in btcwallet)

// DUMP
// importwallet (NYI in btcwallet)
// dumpwallet (NYI in btcwallet)
