// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcrpcclient

import (
	"bytes"
	"encoding/hex"
	"errors"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/conformal/btcws"
	"sync"
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
func newNilFutureResult() chan *futureResult {
	responseChan := make(chan *futureResult, 1)
	responseChan <- &futureResult{reply: nil}
	return responseChan
}

// NotificationHandlers defines callback function pointers to invoke with
// notifications.  Since all of the functions are nil by default, all
// notifications are effectively ignored until their handlers are set to a
// concrete callback.
//
// NOTE: These handlers must NOT directly call any blocking calls on the client
// instance since the input reader goroutine blocks until the callback has
// completed.  Doing so will result in a deadlock situation.
type NotificationHandlers struct {
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

	// OnRescanProgress is invoked periodically when a rescan is underway.
	// It will only be invoked if a preceding call to Rescan or
	// RescanEndHeight has been made and the function is non-nil.
	OnRescanProgress func(lastProcessedHeight int32)

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
	OnUnknownNotification func(ntfn interface{})
}

// handleNotification examines the passed notification type, performs
// conversions to get the raw notification types into higher level types and
// delivers the notification to the appropriate On<X> handler registered with
// the client.
func (c *Client) handleNotification(cmd btcjson.Cmd) {
	// Ignore the notification if the client is not interested in any
	// notifications.
	if c.ntfnHandlers == nil {
		return
	}

	switch ntfn := cmd.(type) {
	// OnBlockConnected
	case *btcws.BlockConnectedNtfn:
		// Ignore the notification is the client is not interested in
		// it.
		if c.ntfnHandlers.OnBlockConnected == nil {
			return
		}

		hash, err := btcwire.NewShaHashFromStr(ntfn.Hash)
		if err != nil {
			log.Warnf("Received block connected notification with "+
				"invalid hash string: %q", ntfn.Hash)
			return
		}

		c.ntfnHandlers.OnBlockConnected(hash, ntfn.Height)

	// OnBlockDisconnected
	case *btcws.BlockDisconnectedNtfn:
		// Ignore the notification is the client is not interested in
		// it.
		if c.ntfnHandlers.OnBlockDisconnected == nil {
			return
		}

		hash, err := btcwire.NewShaHashFromStr(ntfn.Hash)
		if err != nil {
			log.Warnf("Received block disconnected notification "+
				"with invalid hash string: %q", ntfn.Hash)
			return
		}

		c.ntfnHandlers.OnBlockDisconnected(hash, ntfn.Height)

	// OnRecvTx
	case *btcws.RecvTxNtfn:
		// Ignore the notification is the client is not interested in
		// it.
		if c.ntfnHandlers.OnRecvTx == nil {
			return
		}

		// Decode the serialized transaction hex to raw bytes.
		serializedTx, err := hex.DecodeString(ntfn.HexTx)
		if err != nil {
			log.Warnf("Received recvtx notification with invalid "+
				"transaction hex '%q': %v", ntfn.HexTx, err)
		}

		// Deserialize the transaction.
		var msgTx btcwire.MsgTx
		err = msgTx.Deserialize(bytes.NewReader(serializedTx))
		if err != nil {
			log.Warnf("Received recvtx notification with "+
				"transaction that failed to deserialize: %v",
				err)
		}

		c.ntfnHandlers.OnRecvTx(btcutil.NewTx(&msgTx), ntfn.Block)

	// OnRedeemingTx
	case *btcws.RedeemingTxNtfn:
		// Ignore the notification is the client is not interested in
		// it.
		if c.ntfnHandlers.OnRedeemingTx == nil {
			return
		}

		// Decode the serialized transaction hex to raw bytes.
		serializedTx, err := hex.DecodeString(ntfn.HexTx)
		if err != nil {
			log.Warnf("Received redeemingtx notification with "+
				"invalid transaction hex '%q': %v", ntfn.HexTx,
				err)
		}

		// Deserialize the transaction.
		var msgTx btcwire.MsgTx
		err = msgTx.Deserialize(bytes.NewReader(serializedTx))
		if err != nil {
			log.Warnf("Received redeemingtx notification with "+
				"transaction that failed to deserialize: %v",
				err)
		}

		c.ntfnHandlers.OnRedeemingTx(btcutil.NewTx(&msgTx), ntfn.Block)

	// OnRescanProgress
	case *btcws.RescanProgressNtfn:
		// Ignore the notification is the client is not interested in
		// it.
		if c.ntfnHandlers.OnRescanProgress == nil {
			return
		}

		c.ntfnHandlers.OnRescanProgress(ntfn.LastProcessed)

	// OnTxAccepted
	case *btcws.TxAcceptedNtfn:
		// Ignore the notification is the client is not interested in
		// it.
		if c.ntfnHandlers.OnTxAccepted == nil {
			return
		}

		hash, err := btcwire.NewShaHashFromStr(ntfn.TxID)
		if err != nil {
			log.Warnf("Received tx accepted notification with "+
				"invalid hash string: %q", ntfn.TxID)
			return
		}

		c.ntfnHandlers.OnTxAccepted(hash, btcutil.Amount(ntfn.Amount))

	// OnTxAcceptedVerbose
	case *btcws.TxAcceptedVerboseNtfn:
		// Ignore the notification is the client is not interested in
		// it.
		if c.ntfnHandlers.OnTxAcceptedVerbose == nil {
			return
		}

		c.ntfnHandlers.OnTxAcceptedVerbose(ntfn.RawTx)

	// OnBtcdConnected
	case *btcws.BtcdConnectedNtfn:
		// Ignore the notification is the client is not interested in
		// it.
		if c.ntfnHandlers.OnBtcdConnected == nil {
			return
		}

		c.ntfnHandlers.OnBtcdConnected(ntfn.Connected)

	// OnAccountBalance
	case *btcws.AccountBalanceNtfn:
		// Ignore the notification is the client is not interested in
		// it.
		if c.ntfnHandlers.OnAccountBalance == nil {
			return
		}

		balance, err := btcjson.JSONToAmount(ntfn.Balance)
		if err != nil {
			log.Warnf("Received account balance notification with "+
				"an amount that does not parse: %v",
				ntfn.Balance)
			return
		}

		c.ntfnHandlers.OnAccountBalance(ntfn.Account,
			btcutil.Amount(balance), ntfn.Confirmed)

	// OnWalletLockState
	case *btcws.WalletLockStateNtfn:
		// Ignore the notification is the client is not interested in
		// it.
		if c.ntfnHandlers.OnWalletLockState == nil {
			return
		}

		c.ntfnHandlers.OnWalletLockState(ntfn.Locked)

	// OnUnknownNotification
	default:
		if c.ntfnHandlers.OnUnknownNotification == nil {
			return
		}

		c.ntfnHandlers.OnUnknownNotification(ntfn)
	}
}

// FutureNotifyBlocksResult is a future promise to deliver the result of a
// NotifyBlocksAsync RPC invocation (or an applicable error).
type FutureNotifyBlocksResult chan *futureResult

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
type FutureNotifySpentResult chan *futureResult

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
type FutureNotifyNewTransactionsResult chan *futureResult

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
type FutureNotifyReceivedResult chan *futureResult

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
		addrs = append(addrs, addr.EncodeAddress())
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
type FutureRescanResult chan *futureResult

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
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) RescanAsync(startHeight int32, addresses []btcutil.Address,
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

	// Convert addresses to strings.
	addrs := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		addrs = append(addrs, addr.EncodeAddress())
	}

	// Convert outpoints.
	ops := make([]btcws.OutPoint, 0, len(outpoints))
	for _, op := range outpoints {
		ops = append(ops, *btcws.NewOutPointFromWire(op))
	}

	id := c.NextID()
	cmd, err := btcws.NewRescanCmd(id, startHeight, addrs, ops)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// Rescan rescans the block chain starting from the provided start height to the
// end of the longest chain for transactions that pay to the passed addresses
// and transactions which spend the passed outpoints.
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
// See RescanEndHeight to also specify a block height at which to stop the
// rescan if a bounded rescan is desired instead.
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) Rescan(startHeight int32, addresses []btcutil.Address,
	outpoints []*btcwire.OutPoint) error {

	return c.RescanAsync(startHeight, addresses, outpoints).Receive()
}

// RescanEndHeightAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See RescanEndHeight for the blocking version and more details.
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) RescanEndHeightAsync(startHeight int32,
	addresses []btcutil.Address, outpoints []*btcwire.OutPoint,
	endHeight int64) FutureRescanResult {

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
		addrs = append(addrs, addr.EncodeAddress())
	}

	// Convert outpoints.
	ops := make([]btcws.OutPoint, 0, len(outpoints))
	for _, op := range outpoints {
		ops = append(ops, *btcws.NewOutPointFromWire(op))
	}

	id := c.NextID()
	cmd, err := btcws.NewRescanCmd(id, startHeight, addrs, ops, endHeight)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// RescanEndHeight rescans the block chain starting from the provided start
// height up to the provided end height for transactions that pay to the passed
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
// See Rescan to also perform a rescan through current end of the longest chain.
//
// NOTE: This is a btcd extension and requires a websocket connection.
func (c *Client) RescanEndHeight(startHeight int32, addresses []btcutil.Address,
	outpoints []*btcwire.OutPoint, endHeight int64) error {

	return c.RescanEndHeightAsync(startHeight, addresses, outpoints,
		endHeight).Receive()
}
