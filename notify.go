// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcrpcclient

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/conformal/btcjson"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/conformal/btcws"
)

var (
	// ErrNotificationsNotSupported is an error to describe the condition
	// where the caller is trying to request notifications when they are
	// not supported due to the client being configured to run in HTTP POST
	// mode.
	ErrNotificationsNotSupported = errors.New("notifications are not " +
		"supported when running in HTTP POST mode")
)

// notificationState is used to track the current state of successfuly
// registered notification so the state can be automatically re-established on
// reconnect.
type notificationState struct {
	sync.Mutex
	notifyBlocks       bool
	notifyNewTx        bool
	notifyNewTxVerbose bool
	notifyReceived     map[string]struct{}
	notifySpent        map[btcws.OutPoint]struct{}
}

// Copy returns a deep copy of the receiver.
//
// This function is safe for concurrent access.
func (s *notificationState) Copy() *notificationState {
	s.Lock()
	defer s.Unlock()

	stateCopy := *s
	stateCopy.notifyReceived = make(map[string]struct{})
	for addr := range s.notifyReceived {
		stateCopy.notifyReceived[addr] = struct{}{}
	}
	stateCopy.notifySpent = make(map[btcws.OutPoint]struct{})
	for op := range s.notifySpent {
		stateCopy.notifySpent[op] = struct{}{}
	}

	return &stateCopy
}

// newNotificationState returns a new notification state ready to be populated.
func newNotificationState() *notificationState {
	return &notificationState{
		notifyReceived: make(map[string]struct{}),
		notifySpent:    make(map[btcws.OutPoint]struct{}),
	}
}

// newNilFutureResult returns a new future result channel that already has the
// result waiting on the channel with the reply set to nil.  This is useful
// to ignore things such as notifications when the caller didn't specify any
// notification handlers.
func newNilFutureResult() chan *response {
	responseChan := make(chan *response, 1)
	responseChan <- &response{result: nil, err: nil}
	return responseChan
}

// NotificationHandlers defines callback function pointers to invoke with
// notifications.  Since all of the functions are nil by default, all
// notifications are effectively ignored until their handlers are set to a
// concrete callback.
//
// NOTE: Unless otherwise documented, these handlers must NOT directly call any
// blocking calls on the client instance since the input reader goroutine blocks
// until the callback has completed.  Doing so will result in a deadlock
// situation.
type NotificationHandlers struct {
	// OnClientConnected is invoked when the client connects or reconnects
	// to the RPC server.  This callback is run async with the rest of the
	// notification handlers, and is safe for blocking client requests.
	OnClientConnected func()

	// OnBlockConnected is invoked when a block is connected to the longest
	// (best) chain.  It will only be invoked if a preceding call to
	// NotifyBlocks has been made to register for the notification and the
	// function is non-nil.
	OnBlockConnected func(hash *btcwire.ShaHash, height int32)

	// OnBlockDisconnected is invoked when a block is disconnected from the
	// longest (best) chain.  It will only be invoked if a preceding call to
	// NotifyBlocks has been made to register for the notification and the
	// function is non-nil.
	OnBlockDisconnected func(hash *btcwire.ShaHash, height int32)

	// OnRecvTx is invoked when a transaction that receives funds to a
	// registered address is received into the memory pool and also
	// connected to the longest (best) chain.  It will only be invoked if a
	// preceding call to NotifyReceived, Rescan, or RescanEndHeight has been
	// made to register for the notification and the function is non-nil.
	OnRecvTx func(transaction *btcutil.Tx, details *btcws.BlockDetails)

	// OnRedeemingTx is invoked when a transaction that spends a registered
	// outpoint is received into the memory pool and also connected to the
	// longest (best) chain.  It will only be invoked if a preceding call to
	// NotifySpent, Rescan, or RescanEndHeight has been made to register for
	// the notification and the function is non-nil.
	//
	// NOTE: The NotifyReceived will automatically register notifications
	// for the outpoints that are now "owned" as a result of receiving
	// funds to the registered addresses.  This means it is possible for
	// this to invoked indirectly as the result of a NotifyReceived call.
	OnRedeemingTx func(transaction *btcutil.Tx, details *btcws.BlockDetails)

	// OnRescanFinished is invoked after a rescan finishes due to a previous
	// call to Rescan or RescanEndHeight.  Finished rescans should be
	// signaled on this notification, rather than relying on the return
	// result of a rescan request, due to how btcd may send various rescan
	// notifications after the rescan request has already returned.
	OnRescanFinished func(hash *btcwire.ShaHash, height int32, blkTime time.Time)

	// OnRescanProgress is invoked periodically when a rescan is underway.
	// It will only be invoked if a preceding call to Rescan or
	// RescanEndHeight has been made and the function is non-nil.
	OnRescanProgress func(hash *btcwire.ShaHash, height int32, blkTime time.Time)

	// OnTxAccepted is invoked when a transaction is accepted into the
	// memory pool.  It will only be invoked if a preceding call to
	// NotifyNewTransactions with the verbose flag set to false has been
	// made to register for the notification and the function is non-nil.
	OnTxAccepted func(hash *btcwire.ShaHash, amount btcutil.Amount)

	// OnTxAccepted is invoked when a transaction is accepted into the
	// memory pool.  It will only be invoked if a preceding call to
	// NotifyNewTransactions with the verbose flag set to true has been
	// made to register for the notification and the function is non-nil.
	OnTxAcceptedVerbose func(txDetails *btcjson.TxRawResult)

	// OnBtcdConnected is invoked when a wallet connects or disconnects from
	// btcd.
	//
	// This will only be available when client is connected to a wallet
	// server such as btcwallet.
	OnBtcdConnected func(connected bool)

	// OnAccountBalance is invoked with account balance updates.
	//
	// This will only be available when speaking to a wallet server
	// such as btcwallet.
	OnAccountBalance func(account string, balance btcutil.Amount, confirmed bool)

	// OnWalletLockState is invoked when a wallet is locked or unlocked.
	//
	// This will only be available when client is connected to a wallet
	// server such as btcwallet.
	OnWalletLockState func(locked bool)

	// OnUnknownNotification is invoked when an unrecognized notification
	// is received.  This typically means the notification handling code
	// for this package needs to be updated for a new notification type or
	// the caller is using a custom notification this package does not know
	// about.
	OnUnknownNotification func(method string, params []json.RawMessage)
}

// handleNotification examines the passed notification type, performs
// conversions to get the raw notification types into higher level types and
// delivers the notification to the appropriate On<X> handler registered with
// the client.
func (c *Client) handleNotification(ntfn *rawNotification) {
	// Ignore the notification if the client is not interested in any
	// notifications.
	if c.ntfnHandlers == nil {
		return
	}

	switch ntfn.Method {
	// OnBlockConnected
	case btcws.BlockConnectedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnBlockConnected == nil {
			return
		}

		blockSha, blockHeight, err := parseChainNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid block connected "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnBlockConnected(blockSha, blockHeight)

	// OnBlockDisconnected
	case btcws.BlockDisconnectedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnBlockDisconnected == nil {
			return
		}

		blockSha, blockHeight, err := parseChainNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid block connected "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnBlockDisconnected(blockSha, blockHeight)

	// OnRecvTx
	case btcws.RecvTxNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnRecvTx == nil {
			return
		}

		tx, block, err := parseChainTxNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid recvtx notification: %v",
				err)
			return
		}

		c.ntfnHandlers.OnRecvTx(tx, block)

	// OnRedeemingTx
	case btcws.RedeemingTxNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnRedeemingTx == nil {
			return
		}

		tx, block, err := parseChainTxNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid redeemingtx "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnRedeemingTx(tx, block)

	// OnRescanFinished
	case btcws.RescanFinishedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnRescanFinished == nil {
			return
		}

		hash, height, blkTime, err := parseRescanProgressParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid rescanfinished "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnRescanFinished(hash, height, blkTime)

	// OnRescanProgress
	case btcws.RescanProgressNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnRescanProgress == nil {
			return
		}

		hash, height, blkTime, err := parseRescanProgressParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid rescanprogress "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnRescanProgress(hash, height, blkTime)

	// OnTxAccepted
	case btcws.TxAcceptedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnTxAccepted == nil {
			return
		}

		hash, amt, err := parseTxAcceptedNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid tx accepted "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnTxAccepted(hash, amt)

	// OnTxAcceptedVerbose
	case btcws.TxAcceptedVerboseNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnTxAcceptedVerbose == nil {
			return
		}

		rawTx, err := parseTxAcceptedVerboseNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid tx accepted verbose "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnTxAcceptedVerbose(rawTx)

	// OnBtcdConnected
	case btcws.BtcdConnectedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnBtcdConnected == nil {
			return
		}

		connected, err := parseBtcdConnectedNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid btcd connected "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnBtcdConnected(connected)

	// OnAccountBalance
	case btcws.AccountBalanceNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnAccountBalance == nil {
			return
		}

		account, bal, conf, err := parseAccountBalanceNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid account balance "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnAccountBalance(account, bal, conf)

	// OnWalletLockState
	case btcws.WalletLockStateNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnWalletLockState == nil {
			return
		}

		// The account name is not notified, so the return value is
		// discarded.
		_, locked, err := parseWalletLockStateNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid wallet lock state "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnWalletLockState(locked)

	// OnUnknownNotification
	default:
		if c.ntfnHandlers.OnUnknownNotification == nil {
			return
		}

		c.ntfnHandlers.OnUnknownNotification(ntfn.Method, ntfn.Params)
	}
}

// wrongNumParams is an error type describing an unparseable JSON-RPC
// notificiation due to an incorrect number of parameters for the
// expected notification type.  The value is the number of parameters
// of the invalid notification.
type wrongNumParams int

// Error satisifies the builtin error interface.
func (e wrongNumParams) Error() string {
	return fmt.Sprintf("wrong number of parameters (%d)", e)
}

// parseChainNtfnParams parses out the block hash and height from the parameters
// of blockconnected and blockdisconnected notifications.
func parseChainNtfnParams(params []json.RawMessage) (*btcwire.ShaHash,
	int32, error) {

	if len(params) != 2 {
		return nil, 0, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	var blockShaStr string
	err := json.Unmarshal(params[0], &blockShaStr)
	if err != nil {
		return nil, 0, err
	}

	// Unmarshal second parameter as an integer.
	var blockHeight int32
	err = json.Unmarshal(params[1], &blockHeight)
	if err != nil {
		return nil, 0, err
	}

	// Create ShaHash from block sha string.
	blockSha, err := btcwire.NewShaHashFromStr(blockShaStr)
	if err != nil {
		return nil, 0, err
	}

	return blockSha, blockHeight, nil
}

// parseChainTxNtfnParams parses out the transaction and optional details about
// the block it's mined in from the parameters of recvtx and redeemingtx
// notifications.
func parseChainTxNtfnParams(params []json.RawMessage) (*btcutil.Tx,
	*btcws.BlockDetails, error) {

	if len(params) == 0 || len(params) > 2 {
		return nil, nil, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	var txHex string
	err := json.Unmarshal(params[0], &txHex)
	if err != nil {
		return nil, nil, err
	}

	// If present, unmarshal second optional parameter as the block details
	// JSON object.
	var block *btcws.BlockDetails
	if len(params) > 1 {
		err = json.Unmarshal(params[1], &block)
		if err != nil {
			return nil, nil, err
		}
	}

	// Hex decode and deserialize the transaction.
	serializedTx, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, nil, err
	}
	var msgTx btcwire.MsgTx
	err = msgTx.Deserialize(bytes.NewReader(serializedTx))
	if err != nil {
		return nil, nil, err
	}

	// TODO: Change recvtx and redeemingtx callback signatures to use
	// nicer types for details about the block (block sha as a
	// btcwire.ShaHash, block time as a time.Time, etc.).
	return btcutil.NewTx(&msgTx), block, nil
}

// parseRescanProgressParams parses out the height of the last rescanned block
// from the parameters of rescanfinished and rescanprogress notifications.
func parseRescanProgressParams(params []json.RawMessage) (*btcwire.ShaHash, int32, time.Time, error) {
	if len(params) != 3 {
		return nil, 0, time.Time{}, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as an string.
	var hashStr string
	err := json.Unmarshal(params[0], &hashStr)
	if err != nil {
		return nil, 0, time.Time{}, err
	}

	// Unmarshal second parameter as an integer.
	var height int32
	err = json.Unmarshal(params[1], &height)
	if err != nil {
		return nil, 0, time.Time{}, err
	}

	// Unmarshal third parameter as an integer.
	var blkTime int64
	err = json.Unmarshal(params[2], &blkTime)
	if err != nil {
		return nil, 0, time.Time{}, err
	}

	// Decode string encoding of block hash.
	hash, err := btcwire.NewShaHashFromStr(hashStr)
	if err != nil {
		return nil, 0, time.Time{}, err
	}

	return hash, height, time.Unix(blkTime, 0), nil
}

// parseTxAcceptedNtfnParams parses out the transaction hash and total amount
// from the parameters of a txaccepted notification.
func parseTxAcceptedNtfnParams(params []json.RawMessage) (*btcwire.ShaHash,
	btcutil.Amount, error) {

	if len(params) != 2 {
		return nil, 0, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	var txShaStr string
	err := json.Unmarshal(params[0], &txShaStr)
	if err != nil {
		return nil, 0, err
	}

	// Unmarshal second parameter as an integer.
	var amt int64
	err = json.Unmarshal(params[1], &amt)
	if err != nil {
		return nil, 0, err
	}

	// Decode string encoding of transaction sha.
	txSha, err := btcwire.NewShaHashFromStr(txShaStr)
	if err != nil {
		return nil, 0, err
	}

	return txSha, btcutil.Amount(amt), nil
}

// parseTxAcceptedVerboseNtfnParams parses out details about a raw transaction
// from the parameters of a txacceptedverbose notification.
func parseTxAcceptedVerboseNtfnParams(params []json.RawMessage) (*btcjson.TxRawResult,
	error) {

	if len(params) != 1 {
		return nil, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a raw transaction result object.
	var rawTx btcjson.TxRawResult
	err := json.Unmarshal(params[0], &rawTx)
	if err != nil {
		return nil, err
	}

	// TODO: change txacceptedverbose notifiation callbacks to use nicer
	// types for all details about the transaction (i.e. decoding hashes
	// from their string encoding).
	return &rawTx, nil
}

// parseBtcdConnectedNtfnParams parses out the connection status of btcd
// and btcwallet from the parameters of a btcdconnected notification.
func parseBtcdConnectedNtfnParams(params []json.RawMessage) (bool, error) {
	if len(params) != 1 {
		return false, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a boolean.
	var connected bool
	err := json.Unmarshal(params[0], &connected)
	if err != nil {
		return false, err
	}

	return connected, nil
}

// parseAccountBalanceNtfnParams parses out the account name, total balance,
// and whether or not the balance is confirmed or unconfirmed from the
// parameters of an accountbalance notification.
func parseAccountBalanceNtfnParams(params []json.RawMessage) (account string,
	balance btcutil.Amount, confirmed bool, err error) {

	if len(params) != 3 {
		return "", 0, false, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	err = json.Unmarshal(params[0], &account)
	if err != nil {
		return "", 0, false, err
	}

	// Unmarshal second parameter as a floating point number.
	var fbal float64
	err = json.Unmarshal(params[1], &fbal)
	if err != nil {
		return "", 0, false, err
	}

	// Unmarshal third parameter as a boolean.
	err = json.Unmarshal(params[2], &confirmed)
	if err != nil {
		return "", 0, false, err
	}

	// Bounds check amount.
	bal, err := btcjson.JSONToAmount(fbal)
	if err != nil {
		return "", 0, false, err
	}

	return account, btcutil.Amount(bal), confirmed, nil
}

// parseWalletLockStateNtfnParams parses out the account name and locked
// state of an account from the parameters of a walletlockstate notification.
func parseWalletLockStateNtfnParams(params []json.RawMessage) (account string,
	locked bool, err error) {

	if len(params) != 2 {
		return "", false, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	err = json.Unmarshal(params[0], &account)
	if err != nil {
		return "", false, err
	}

	// Unmarshal second parameter as a boolean.
	err = json.Unmarshal(params[1], &locked)
	if err != nil {
		return "", false, err
	}

	return account, locked, nil
}

// FutureNotifyBlocksResult is a future promise to deliver the result of a
// NotifyBlocksAsync RPC invocation (or an applicable error).
type FutureNotifyBlocksResult chan *response

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r FutureNotifyBlocksResult) Receive() error {
	_, err := receiveFuture(r)
	if err != nil {
		return err
	}

	return nil
}

// NotifyBlocksAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See NotifyBlocks for the blocking version and more details.
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) NotifyBlocksAsync() FutureNotifyBlocksResult {
	// Not supported in HTTP POST mode.
	if c.config.HttpPostMode {
		return newFutureError(ErrNotificationsNotSupported)
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return newNilFutureResult()
	}

	id := c.NextID()
	cmd := btcws.NewNotifyBlocksCmd(id)

	return c.sendCmd(cmd)
}

// NotifyBlocks registers the client to receive notifications when blocks are
// connected and disconnected from the main chain.  The notifications are
// delivered to the notification handlers associated with the client.  Calling
// this function has no effect if there are no notification handlers and will
// result in an error if the client is configured to run in HTTP POST mode.
//
// The notifications delivered as a result of this call will be via one of
// OnBlockConnected or OnBlockDisconnected.
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) NotifyBlocks() error {
	return c.NotifyBlocksAsync().Receive()
}

// FutureNotifySpentResult is a future promise to deliver the result of a
// NotifySpentAsync RPC invocation (or an applicable error).
type FutureNotifySpentResult chan *response

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r FutureNotifySpentResult) Receive() error {
	_, err := receiveFuture(r)
	if err != nil {
		return err
	}

	return nil
}

// notifySpentInternal is the same as notifySpentAsync except it accepts
// the converted outpoints as a parameter so the client can more efficiently
// recreate the previous notification state on reconnect.
func (c *Client) notifySpentInternal(outpoints []btcws.OutPoint) FutureNotifySpentResult {
	// Not supported in HTTP POST mode.
	if c.config.HttpPostMode {
		return newFutureError(ErrNotificationsNotSupported)
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return newNilFutureResult()
	}

	id := c.NextID()
	cmd := btcws.NewNotifySpentCmd(id, outpoints)

	return c.sendCmd(cmd)
}

// NotifySpentAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See NotifySpent for the blocking version and more details.
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) NotifySpentAsync(outpoints []*btcwire.OutPoint) FutureNotifySpentResult {
	// Not supported in HTTP POST mode.
	if c.config.HttpPostMode {
		return newFutureError(ErrNotificationsNotSupported)
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return newNilFutureResult()
	}

	id := c.NextID()
	ops := make([]btcws.OutPoint, 0, len(outpoints))
	for _, outpoint := range outpoints {
		ops = append(ops, *btcws.NewOutPointFromWire(outpoint))
	}
	cmd := btcws.NewNotifySpentCmd(id, ops)

	return c.sendCmd(cmd)
}

// NotifySpent registers the client to receive notifications when the passed
// transaction outputs are spent.  The notifications are delivered to the
// notification handlers associated with the client.  Calling this function has
// no effect if there are no notification handlers and will result in an error
// if the client is configured to run in HTTP POST mode.
//
// The notifications delivered as a result of this call will be via
// OnRedeemingTx.
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) NotifySpent(outpoints []*btcwire.OutPoint) error {
	return c.NotifySpentAsync(outpoints).Receive()
}

// FutureNotifyNewTransactionsResult is a future promise to deliver the result
// of a NotifyNewTransactionsAsync RPC invocation (or an applicable error).
type FutureNotifyNewTransactionsResult chan *response

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r FutureNotifyNewTransactionsResult) Receive() error {
	_, err := receiveFuture(r)
	if err != nil {
		return err
	}

	return nil
}

// NotifyNewTransactionsAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See NotifyNewTransactionsAsync for the blocking version and more details.
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) NotifyNewTransactionsAsync(verbose bool) FutureNotifyNewTransactionsResult {
	// Not supported in HTTP POST mode.
	if c.config.HttpPostMode {
		return newFutureError(ErrNotificationsNotSupported)
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return newNilFutureResult()
	}

	id := c.NextID()
	cmd, err := btcws.NewNotifyNewTransactionsCmd(id, verbose)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// NotifyNewTransactions registers the client to receive notifications every
// time a new transaction is accepted to the memory pool.  The notifications are
// delivered to the notification handlers associated with the client.  Calling
// this function has no effect if there are no notification handlers and will
// result in an error if the client is configured to run in HTTP POST mode.
//
// The notifications delivered as a result of this call will be via one of
// OnTxAccepted (when verbose is false) or OnTxAcceptedVerbose (when verbose is
// true).
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) NotifyNewTransactions(verbose bool) error {
	return c.NotifyNewTransactionsAsync(verbose).Receive()
}

// FutureNotifyReceivedResult is a future promise to deliver the result of a
// NotifyReceivedAsync RPC invocation (or an applicable error).
type FutureNotifyReceivedResult chan *response

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r FutureNotifyReceivedResult) Receive() error {
	_, err := receiveFuture(r)
	if err != nil {
		return err
	}

	return nil
}

// notifyReceivedInternal is the same as notifyReceivedAsync except it accepts
// the converted addresses as a parameter so the client can more efficiently
// recreate the previous notification state on reconnect.
func (c *Client) notifyReceivedInternal(addresses []string) FutureNotifyReceivedResult {
	// Not supported in HTTP POST mode.
	if c.config.HttpPostMode {
		return newFutureError(ErrNotificationsNotSupported)
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return newNilFutureResult()
	}

	// Convert addresses to strings.
	id := c.NextID()
	cmd := btcws.NewNotifyReceivedCmd(id, addresses)

	return c.sendCmd(cmd)
}

// NotifyReceivedAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See NotifyReceived for the blocking version and more details.
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) NotifyReceivedAsync(addresses []btcutil.Address) FutureNotifyReceivedResult {
	// Not supported in HTTP POST mode.
	if c.config.HttpPostMode {
		return newFutureError(ErrNotificationsNotSupported)
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return newNilFutureResult()
	}

	// Convert addresses to strings.
	addrs := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		addrs = append(addrs, addr.String())
	}
	id := c.NextID()
	cmd := btcws.NewNotifyReceivedCmd(id, addrs)

	return c.sendCmd(cmd)
}

// NotifyReceived registers the client to receive notifications every time a
// new transaction which pays to one of the passed addresses is accepted to
// memory pool or in a block connected to the block chain.  In addition, when
// one of these transactions is detected, the client is also automatically
// registered for notifications when the new transaction outpoints the address
// now has available are spent (See NotifySpent).  The notifications are
// delivered to the notification handlers associated with the client.  Calling
// this function has no effect if there are no notification handlers and will
// result in an error if the client is configured to run in HTTP POST mode.
//
// The notifications delivered as a result of this call will be via one of
// *OnRecvTx (for transactions that receive funds to one of the passed
// addresses) or OnRedeemingTx (for transactions which spend from one
// of the outpoints which are automatically registered upon receipt of funds to
// the address).
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) NotifyReceived(addresses []btcutil.Address) error {
	return c.NotifyReceivedAsync(addresses).Receive()
}

// FutureRescanResult is a future promise to deliver the result of a RescanAsync
// or RescanEndHeightAsync RPC invocation (or an applicable error).
type FutureRescanResult chan *response

// Receive waits for the response promised by the future and returns an error
// if the rescan was not successful.
func (r FutureRescanResult) Receive() error {
	_, err := receiveFuture(r)
	if err != nil {
		return err
	}

	return nil
}

// RescanAsync returns an instance of a type that can be used to get the result
// of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See Rescan for the blocking version and more details.
//
// NOTE: Rescan requests are not issued on client reconnect and must be
// performed manually (ideally with a new start height based on the last
// rescan progress notification).  See the OnClientConnected notification
// callback for a good callsite to reissue rescan requests on connect and
// reconnect.
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) RescanAsync(startBlock *btcwire.ShaHash,
	addresses []btcutil.Address,
	outpoints []*btcwire.OutPoint) FutureRescanResult {

	// Not supported in HTTP POST mode.
	if c.config.HttpPostMode {
		return newFutureError(ErrNotificationsNotSupported)
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return newNilFutureResult()
	}

	// Convert block hashes to strings.
	var startBlockShaStr string
	if startBlock != nil {
		startBlockShaStr = startBlock.String()
	}

	// Convert addresses to strings.
	addrs := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		addrs = append(addrs, addr.String())
	}

	// Convert outpoints.
	ops := make([]btcws.OutPoint, 0, len(outpoints))
	for _, op := range outpoints {
		ops = append(ops, *btcws.NewOutPointFromWire(op))
	}

	id := c.NextID()
	cmd, err := btcws.NewRescanCmd(id, startBlockShaStr, addrs, ops)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// Rescan rescans the block chain starting from the provided starting block to
// the end of the longest chain for transactions that pay to the passed
// addresses and transactions which spend the passed outpoints.
//
// The notifications of found transactions are delivered to the notification
// handlers associated with client and this call will not return until the
// rescan has completed.  Calling this function has no effect if there are no
// notification handlers and will result in an error if the client is configured
// to run in HTTP POST mode.
//
// The notifications delivered as a result of this call will be via one of
// OnRedeemingTx (for transactions which spend from the one of the
// passed outpoints), OnRecvTx (for transactions that receive funds
// to one of the passed addresses), and OnRescanProgress (for rescan progress
// updates).
//
// See RescanEndBlock to also specify an ending block to finish the rescan
// without continuing through the best block on the main chain.
//
// NOTE: Rescan requests are not issued on client reconnect and must be
// performed manually (ideally with a new start height based on the last
// rescan progress notification).  See the OnClientConnected notification
// callback for a good callsite to reissue rescan requests on connect and
// reconnect.
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) Rescan(startBlock *btcwire.ShaHash,
	addresses []btcutil.Address,
	outpoints []*btcwire.OutPoint) error {

	return c.RescanAsync(startBlock, addresses, outpoints).Receive()
}

// RescanEndBlockAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See RescanEndBlock for the blocking version and more details.
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) RescanEndBlockAsync(startBlock *btcwire.ShaHash,
	addresses []btcutil.Address, outpoints []*btcwire.OutPoint,
	endBlock *btcwire.ShaHash) FutureRescanResult {

	// Not supported in HTTP POST mode.
	if c.config.HttpPostMode {
		return newFutureError(ErrNotificationsNotSupported)
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return newNilFutureResult()
	}

	// Convert block hashes to strings.
	var startBlockShaStr, endBlockShaStr string
	if startBlock != nil {
		startBlockShaStr = startBlock.String()
	}
	if endBlock != nil {
		endBlockShaStr = endBlock.String()
	}

	// Convert addresses to strings.
	addrs := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		addrs = append(addrs, addr.String())
	}

	// Convert outpoints.
	ops := make([]btcws.OutPoint, 0, len(outpoints))
	for _, op := range outpoints {
		ops = append(ops, *btcws.NewOutPointFromWire(op))
	}

	id := c.NextID()
	cmd, err := btcws.NewRescanCmd(id, startBlockShaStr, addrs, ops,
		endBlockShaStr)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// RescanEndBlock rescans the block chain starting from the provided starting
// block up to the provided ending block for transactions that pay to the
// passed addresses and transactions which spend the passed outpoints.
//
// The notifications of found transactions are delivered to the notification
// handlers associated with client and this call will not return until the
// rescan has completed.  Calling this function has no effect if there are no
// notification handlers and will result in an error if the client is configured
// to run in HTTP POST mode.
//
// The notifications delivered as a result of this call will be via one of
// OnRedeemingTx (for transactions which spend from the one of the
// passed outpoints), OnRecvTx (for transactions that receive funds
// to one of the passed addresses), and OnRescanProgress (for rescan progress
// updates).
//
// See Rescan to also perform a rescan through current end of the longest chain.
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) RescanEndHeight(startBlock *btcwire.ShaHash,
	addresses []btcutil.Address, outpoints []*btcwire.OutPoint,
	endBlock *btcwire.ShaHash) error {

	return c.RescanEndBlockAsync(startBlock, addresses, outpoints,
		endBlock).Receive()
}
