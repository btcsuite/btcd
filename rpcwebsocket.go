// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/btcsuite/websocket"
	"golang.org/x/crypto/ripemd160"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

const (
	// websocketSendBufferSize is the number of elements the send channel
	// can queue before blocking.  Note that this only applies to requests
	// handled directly in the websocket client input handler or the async
	// handler since notifications have their own queuing mechanism
	// independent of the send channel buffer.
	websocketSendBufferSize = 50
)

type semaphore chan struct{}

func makeSemaphore(n int) semaphore {
	return make(chan struct{}, n)
}

func (s semaphore) acquire() { s <- struct{}{} }
func (s semaphore) release() { <-s }

// timeZeroVal is simply the zero value for a time.Time and is used to avoid
// creating multiple instances.
var timeZeroVal time.Time

// wsCommandHandler describes a callback function used to handle a specific
// command.
type wsCommandHandler func(*wsClient, interface{}) (interface{}, error)

// wsHandlers maps RPC command strings to appropriate websocket handler
// functions.  This is set by init because help references wsHandlers and thus
// causes a dependency loop.
var wsHandlers map[string]wsCommandHandler
var wsHandlersBeforeInit = map[string]wsCommandHandler{
	"loadtxfilter":                handleLoadTxFilter,
	"notifyblocks":                handleNotifyBlocks,
	"notifywinningtickets":        handleWinningTickets,
	"notifyspentandmissedtickets": handleSpentAndMissedTickets,
	"notifynewtickets":            handleNewTickets,
	"notifystakedifficulty":       handleStakeDifficulty,
	"notifynewtransactions":       handleNotifyNewTransactions,
	"session":                     handleSession,
	"help":                        handleWebsocketHelp,
	"rescan":                      handleRescan,
	"stopnotifyblocks":            handleStopNotifyBlocks,
	"stopnotifynewtransactions":   handleStopNotifyNewTransactions,
}

// WebsocketHandler handles a new websocket client by creating a new wsClient,
// starting it, and blocking until the connection closes.  Since it blocks, it
// must be run in a separate goroutine.  It should be invoked from the websocket
// server handler which runs each new connection in a new goroutine thereby
// satisfying the requirement.
func (s *rpcServer) WebsocketHandler(conn *websocket.Conn, remoteAddr string,
	authenticated bool, isAdmin bool) {

	// Clear the read deadline that was set before the websocket hijacked
	// the connection.
	conn.SetReadDeadline(timeZeroVal)

	// Limit max number of websocket clients.
	rpcsLog.Infof("New websocket client %s", remoteAddr)
	if s.ntfnMgr.NumClients()+1 > cfg.RPCMaxWebsockets {
		rpcsLog.Infof("Max websocket clients exceeded [%d] - "+
			"disconnecting client %s", cfg.RPCMaxWebsockets,
			remoteAddr)
		conn.Close()
		return
	}

	// Create a new websocket client to handle the new websocket connection
	// and wait for it to shutdown.  Once it has shutdown (and hence
	// disconnected), remove it and any notifications it registered for.
	client, err := newWebsocketClient(s, conn, remoteAddr, authenticated, isAdmin)
	if err != nil {
		rpcsLog.Errorf("Failed to serve client %s: %v", remoteAddr, err)
		conn.Close()
		return
	}
	s.ntfnMgr.AddClient(client)
	client.Start()
	client.WaitForShutdown()
	s.ntfnMgr.RemoveClient(client)
	rpcsLog.Infof("Disconnected websocket client %s", remoteAddr)
}

// wsNotificationManager is a connection and notification manager used for
// websockets.  It allows websocket clients to register for notifications they
// are interested in.  When an event happens elsewhere in the code such as
// transactions being added to the memory pool or block connects/disconnects,
// the notification manager is provided with the relevant details needed to
// figure out which websocket clients need to be notified based on what they
// have registered for and notifies them accordingly.  It is also used to keep
// track of all connected websocket clients.
type wsNotificationManager struct {
	// server is the RPC server the notification manager is associated with.
	server *rpcServer

	// queueNotification queues a notification for handling.
	queueNotification chan interface{}

	// notificationMsgs feeds notificationHandler with notifications
	// and client (un)registeration requests from a queue as well as
	// registeration and unregisteration requests from clients.
	notificationMsgs chan interface{}

	// Access channel for current number of connected clients.
	numClients chan int

	// Shutdown handling
	wg   sync.WaitGroup
	quit chan struct{}
}

// queueHandler manages a queue of empty interfaces, reading from in and
// sending the oldest unsent to out.  This handler stops when either of the
// in or quit channels are closed, and closes out before returning, without
// waiting to send any variables still remaining in the queue.
func queueHandler(in <-chan interface{}, out chan<- interface{}, quit <-chan struct{}) {
	var q []interface{}
	var dequeue chan<- interface{}
	skipQueue := out
	var next interface{}
out:
	for {
		select {
		case n, ok := <-in:
			if !ok {
				// Sender closed input channel.
				break out
			}

			// Either send to out immediately if skipQueue is
			// non-nil (queue is empty) and reader is ready,
			// or append to the queue and send later.
			select {
			case skipQueue <- n:
			default:
				q = append(q, n)
				dequeue = out
				skipQueue = nil
				next = q[0]
			}

		case dequeue <- next:
			copy(q, q[1:])
			q[len(q)-1] = nil // avoid leak
			q = q[:len(q)-1]
			if len(q) == 0 {
				dequeue = nil
				skipQueue = out
			} else {
				next = q[0]
			}

		case <-quit:
			break out
		}
	}
	close(out)
}

// queueHandler maintains a queue of notifications and notification handler
// control messages.
func (m *wsNotificationManager) queueHandler() {
	queueHandler(m.queueNotification, m.notificationMsgs, m.quit)
	m.wg.Done()
}

// NotifyBlockConnected passes a block newly-connected to the best chain
// to the notification manager for block and transaction notification
// processing.
func (m *wsNotificationManager) NotifyBlockConnected(block *dcrutil.Block) {
	// As NotifyBlockConnected will be called by the block manager
	// and the RPC server may no longer be running, use a select
	// statement to unblock enqueuing the notification once the RPC
	// server has begun shutting down.
	select {
	case m.queueNotification <- (*notificationBlockConnected)(block):
	case <-m.quit:
	}
}

// NotifyBlockDisconnected passes a block disconnected from the best chain
// to the notification manager for block notification processing.
func (m *wsNotificationManager) NotifyBlockDisconnected(block *dcrutil.Block) {
	// As NotifyBlockDisconnected will be called by the block manager
	// and the RPC server may no longer be running, use a select
	// statement to unblock enqueuing the notification once the RPC
	// server has begun shutting down.
	select {
	case m.queueNotification <- (*notificationBlockDisconnected)(block):
	case <-m.quit:
	}
}

// NotifyReorganization passes a blockchain reorganization notification for
// reorganization notification processing.
func (m *wsNotificationManager) NotifyReorganization(rd *blockchain.ReorganizationNtfnsData) {
	// As NotifyReorganization will be called by the block manager
	// and the RPC server may no longer be running, use a select
	// statement to unblock enqueuing the notification once the RPC
	// server has begun shutting down.
	select {
	case m.queueNotification <- (*notificationReorganization)(rd):
	case <-m.quit:
	}
}

// NotifyWinningTickets passes newly winning tickets for an incoming block
// to the notification manager for further processing.
func (m *wsNotificationManager) NotifyWinningTickets(
	wtnd *WinningTicketsNtfnData) {
	// As NotifyWinningTickets will be called by the block manager
	// and the RPC server may no longer be running, use a select
	// statement to unblock enqueuing the notification once the RPC
	// server has begun shutting down.
	select {
	case m.queueNotification <- (*notificationWinningTickets)(wtnd):
	case <-m.quit:
	}
}

// NotifySpentAndMissedTickets passes ticket spend and missing data for an
// incoming block from the best chain to the notification manager for block
// notification processing.
func (m *wsNotificationManager) NotifySpentAndMissedTickets(
	tnd *blockchain.TicketNotificationsData) {
	// As NotifySpentAndMissedTickets will be called by the block manager
	// and the RPC server may no longer be running, use a select
	// statement to unblock enqueuing the notification once the RPC
	// server has begun shutting down.
	select {
	case m.queueNotification <- (*notificationSpentAndMissedTickets)(tnd):
	case <-m.quit:
	}
}

// NotifyNewTickets passes a new ticket data for an incoming block from the best
// chain to the notification manager for block notification processing.
func (m *wsNotificationManager) NotifyNewTickets(
	tnd *blockchain.TicketNotificationsData) {
	// As NotifyNewTickets will be called by the block manager
	// and the RPC server may no longer be running, use a select
	// statement to unblock enqueuing the notification once the RPC
	// server has begun shutting down.
	select {
	case m.queueNotification <- (*notificationNewTickets)(tnd):
	case <-m.quit:
	}
}

// NotifyNewTickets passes a new ticket data for an incoming block from the best
// chain to the notification manager for block notification processing.
func (m *wsNotificationManager) NotifyStakeDifficulty(
	stnd *StakeDifficultyNtfnData) {
	// As NotifyNewTickets will be called by the block manager
	// and the RPC server may no longer be running, use a select
	// statement to unblock enqueuing the notification once the RPC
	// server has begun shutting down.
	select {
	case m.queueNotification <- (*notificationStakeDifficulty)(stnd):
	case <-m.quit:
	}
}

// NotifyMempoolTx passes a transaction accepted by mempool to the
// notification manager for transaction notification processing.  If
// isNew is true, the tx is is a new transaction, rather than one
// added to the mempool during a reorg.
func (m *wsNotificationManager) NotifyMempoolTx(tx *dcrutil.Tx, isNew bool) {
	n := &notificationTxAcceptedByMempool{
		isNew: isNew,
		tx:    tx,
	}

	// As NotifyMempoolTx will be called by mempool and the RPC server
	// may no longer be running, use a select statement to unblock
	// enqueuing the notification once the RPC server has begun
	// shutting down.
	select {
	case m.queueNotification <- n:
	case <-m.quit:
	}
}

// WinningTicketsNtfnData is the data that is used to generate
// winning ticket notifications (which indicate a block and
// the tickets eligible to vote on it).
type WinningTicketsNtfnData struct {
	BlockHash   chainhash.Hash
	BlockHeight int64
	Tickets     []chainhash.Hash
}

// StakeDifficultyNtfnData is the data that is used to generate
// stake difficulty notifications.
type StakeDifficultyNtfnData struct {
	BlockHash       chainhash.Hash
	BlockHeight     int64
	StakeDifficulty int64
}

type wsClientFilter struct {
	mu sync.Mutex

	// Implemented fast paths for address lookup.
	pubKeyHashes        map[[ripemd160.Size]byte]struct{}
	scriptHashes        map[[ripemd160.Size]byte]struct{}
	compressedPubKeys   map[[33]byte]struct{}
	uncompressedPubKeys map[[65]byte]struct{}

	// A fallback address lookup map in case a fast path doesn't exist.
	// Only exists for completeness.  If using this shows up in a profile,
	// there's a good chance a fast path should be added.
	otherAddresses map[string]struct{}

	// Outpoints of unspent outputs.
	unspent map[wire.OutPoint]struct{}
}

func makeWSClientFilter(addresses []string, unspentOutPoints []*wire.OutPoint) *wsClientFilter {
	filter := &wsClientFilter{
		pubKeyHashes:        map[[ripemd160.Size]byte]struct{}{},
		scriptHashes:        map[[ripemd160.Size]byte]struct{}{},
		compressedPubKeys:   map[[33]byte]struct{}{},
		uncompressedPubKeys: map[[65]byte]struct{}{},
		otherAddresses:      map[string]struct{}{},
		unspent:             make(map[wire.OutPoint]struct{}, len(unspentOutPoints)),
	}

	for _, s := range addresses {
		filter.addAddressStr(s)
	}
	for _, op := range unspentOutPoints {
		filter.addUnspentOutPoint(op)
	}

	return filter
}

func (f *wsClientFilter) addAddress(a dcrutil.Address) {
	switch a := a.(type) {
	case *dcrutil.AddressPubKeyHash:
		f.pubKeyHashes[*a.Hash160()] = struct{}{}
		return
	case *dcrutil.AddressScriptHash:
		f.scriptHashes[*a.Hash160()] = struct{}{}
		return
	case *dcrutil.AddressSecpPubKey:
		serializedPubKey := a.ScriptAddress()
		switch len(serializedPubKey) {
		case 33: // compressed
			var compressedPubKey [33]byte
			copy(compressedPubKey[:], serializedPubKey)
			f.compressedPubKeys[compressedPubKey] = struct{}{}
			return
		case 65: // uncompressed
			var uncompressedPubKey [65]byte
			copy(uncompressedPubKey[:], serializedPubKey)
			f.uncompressedPubKeys[uncompressedPubKey] = struct{}{}
			return
		}
	}

	f.otherAddresses[a.EncodeAddress()] = struct{}{}
}

func (f *wsClientFilter) addAddressStr(s string) {
	a, err := dcrutil.DecodeAddress(s)
	// If address can't be decoded, no point in saving it since it should also
	// impossible to create the address from an inspected transaction output
	// script.
	if err != nil {
		return
	}
	f.addAddress(a)
}

func (f *wsClientFilter) existsAddress(a dcrutil.Address) bool {
	switch a := a.(type) {
	case *dcrutil.AddressPubKeyHash:
		_, ok := f.pubKeyHashes[*a.Hash160()]
		return ok
	case *dcrutil.AddressScriptHash:
		_, ok := f.scriptHashes[*a.Hash160()]
		return ok
	case *dcrutil.AddressSecpPubKey:
		serializedPubKey := a.ScriptAddress()
		switch len(serializedPubKey) {
		case 33: // compressed
			var compressedPubKey [33]byte
			copy(compressedPubKey[:], serializedPubKey)
			_, ok := f.compressedPubKeys[compressedPubKey]
			if !ok {
				_, ok = f.pubKeyHashes[*a.AddressPubKeyHash().Hash160()]
			}
			return ok
		case 65: // uncompressed
			var uncompressedPubKey [65]byte
			copy(uncompressedPubKey[:], serializedPubKey)
			_, ok := f.uncompressedPubKeys[uncompressedPubKey]
			if !ok {
				_, ok = f.pubKeyHashes[*a.AddressPubKeyHash().Hash160()]
			}
			return ok
		}
	}

	_, ok := f.otherAddresses[a.EncodeAddress()]
	return ok
}

func (f *wsClientFilter) addUnspentOutPoint(op *wire.OutPoint) {
	f.unspent[*op] = struct{}{}
}

func (f *wsClientFilter) existsUnspentOutPoint(op *wire.OutPoint) bool {
	_, ok := f.unspent[*op]
	return ok
}

// Notification types
type notificationBlockConnected dcrutil.Block
type notificationBlockDisconnected dcrutil.Block
type notificationReorganization blockchain.ReorganizationNtfnsData
type notificationWinningTickets WinningTicketsNtfnData
type notificationSpentAndMissedTickets blockchain.TicketNotificationsData
type notificationNewTickets blockchain.TicketNotificationsData
type notificationStakeDifficulty StakeDifficultyNtfnData
type notificationTxAcceptedByMempool struct {
	isNew bool
	tx    *dcrutil.Tx
}

// Notification control requests
type notificationRegisterClient wsClient
type notificationUnregisterClient wsClient
type notificationRegisterBlocks wsClient
type notificationUnregisterBlocks wsClient
type notificationRegisterWinningTickets wsClient
type notificationUnregisterWinningTickets wsClient
type notificationRegisterSpentAndMissedTickets wsClient
type notificationUnregisterSpentAndMissedTickets wsClient
type notificationRegisterNewTickets wsClient
type notificationUnregisterNewTickets wsClient
type notificationRegisterStakeDifficulty wsClient
type notificationUnregisterStakeDifficulty wsClient
type notificationRegisterNewMempoolTxs wsClient
type notificationUnregisterNewMempoolTxs wsClient

// notificationHandler reads notifications and control messages from the queue
// handler and processes one at a time.
func (m *wsNotificationManager) notificationHandler() {
	// clients is a map of all currently connected websocket clients.
	clients := make(map[chan struct{}]*wsClient)

	// Maps used to hold lists of websocket clients to be notified on
	// certain events.  Each websocket client also keeps maps for the events
	// which have multiple triggers to make removal from these lists on
	// connection close less horrendously expensive.
	//
	// Where possible, the quit channel is used as the unique id for a client
	// since it is quite a bit more efficient than using the entire struct.
	blockNotifications := make(map[chan struct{}]*wsClient)
	winningTicketNotifications := make(map[chan struct{}]*wsClient)
	ticketSMNotifications := make(map[chan struct{}]*wsClient)
	ticketNewNotifications := make(map[chan struct{}]*wsClient)
	stakeDifficultyNotifications := make(map[chan struct{}]*wsClient)
	txNotifications := make(map[chan struct{}]*wsClient)

out:
	for {
		select {
		case n, ok := <-m.notificationMsgs:
			if !ok {
				// queueHandler quit.
				break out
			}
			switch n := n.(type) {
			case *notificationBlockConnected:
				block := (*dcrutil.Block)(n)

				// Skip iterating through all txs if no tx
				// notification requests exist.
				if len(blockNotifications) == 0 {
					continue
				}

				m.notifyBlockConnected(blockNotifications, block)

			case *notificationBlockDisconnected:
				m.notifyBlockDisconnected(blockNotifications,
					(*dcrutil.Block)(n))

			case *notificationReorganization:
				m.notifyReorganization(blockNotifications,
					(*blockchain.ReorganizationNtfnsData)(n))

			case *notificationWinningTickets:
				m.notifyWinningTickets(winningTicketNotifications,
					(*WinningTicketsNtfnData)(n))

			case *notificationSpentAndMissedTickets:
				m.notifySpentAndMissedTickets(ticketSMNotifications,
					(*blockchain.TicketNotificationsData)(n))

			case *notificationNewTickets:
				m.notifyNewTickets(ticketNewNotifications,
					(*blockchain.TicketNotificationsData)(n))

			case *notificationStakeDifficulty:
				m.notifyStakeDifficulty(stakeDifficultyNotifications,
					(*StakeDifficultyNtfnData)(n))

			case *notificationTxAcceptedByMempool:
				if n.isNew && len(txNotifications) != 0 {
					m.notifyForNewTx(txNotifications, n.tx)
				}
				m.notifyRelevantTxAccepted(n.tx, clients)

			case *notificationRegisterBlocks:
				wsc := (*wsClient)(n)
				blockNotifications[wsc.quit] = wsc

			case *notificationUnregisterBlocks:
				wsc := (*wsClient)(n)
				delete(blockNotifications, wsc.quit)

			case *notificationRegisterWinningTickets:
				wsc := (*wsClient)(n)
				winningTicketNotifications[wsc.quit] = wsc

			case *notificationUnregisterWinningTickets:
				wsc := (*wsClient)(n)
				delete(winningTicketNotifications, wsc.quit)

			case *notificationRegisterSpentAndMissedTickets:
				wsc := (*wsClient)(n)
				ticketSMNotifications[wsc.quit] = wsc

			case *notificationUnregisterSpentAndMissedTickets:
				wsc := (*wsClient)(n)
				delete(ticketSMNotifications, wsc.quit)

			case *notificationRegisterNewTickets:
				wsc := (*wsClient)(n)
				ticketNewNotifications[wsc.quit] = wsc

			case *notificationUnregisterNewTickets:
				wsc := (*wsClient)(n)
				delete(ticketNewNotifications, wsc.quit)

			case *notificationRegisterStakeDifficulty:
				wsc := (*wsClient)(n)
				stakeDifficultyNotifications[wsc.quit] = wsc

			case *notificationUnregisterStakeDifficulty:
				wsc := (*wsClient)(n)
				delete(stakeDifficultyNotifications, wsc.quit)

			case *notificationRegisterClient:
				wsc := (*wsClient)(n)
				clients[wsc.quit] = wsc

			case *notificationUnregisterClient:
				wsc := (*wsClient)(n)
				// Remove any requests made by the client as well as
				// the client itself.
				delete(blockNotifications, wsc.quit)
				delete(txNotifications, wsc.quit)
				delete(clients, wsc.quit)

			case *notificationRegisterNewMempoolTxs:
				wsc := (*wsClient)(n)
				txNotifications[wsc.quit] = wsc

			case *notificationUnregisterNewMempoolTxs:
				wsc := (*wsClient)(n)
				delete(txNotifications, wsc.quit)

			default:
				rpcsLog.Warn("Unhandled notification type")
			}

		case m.numClients <- len(clients):

		case <-m.quit:
			// RPC server shutting down.
			break out
		}
	}

	for _, c := range clients {
		c.Disconnect()
	}
	m.wg.Done()
}

// NumClients returns the number of clients actively being served.
func (m *wsNotificationManager) NumClients() (n int) {
	select {
	case n = <-m.numClients:
	case <-m.quit: // Use default n (0) if server has shut down.
	}
	return
}

// RegisterBlockUpdates requests block update notifications to the passed
// websocket client.
func (m *wsNotificationManager) RegisterBlockUpdates(wsc *wsClient) {
	m.queueNotification <- (*notificationRegisterBlocks)(wsc)
}

// UnregisterBlockUpdates removes block update notifications for the passed
// websocket client.
func (m *wsNotificationManager) UnregisterBlockUpdates(wsc *wsClient) {
	m.queueNotification <- (*notificationUnregisterBlocks)(wsc)
}

// subscribedClients returns the set of all websocket client quit channels that
// are registered to receive notifications regarding tx, either due to tx
// spending a watched output or outputting to a watched address.  Matching
// client's filters are updated based on this transaction's outputs and output
// addresses that may be relevant for a client.
func (m *wsNotificationManager) subscribedClients(tx *dcrutil.Tx,
	clients map[chan struct{}]*wsClient) map[chan struct{}]struct{} {

	// Use a map of client quit channels as keys to prevent duplicates when
	// multiple inputs and/or outputs are relevant to the client.
	subscribed := make(map[chan struct{}]struct{})

	msgTx := tx.MsgTx()
	for q, c := range clients {
		c.Lock()
		f := c.filterData
		c.Unlock()
		if f == nil {
			continue
		}
		f.mu.Lock()

		for _, input := range msgTx.TxIn {
			if f.existsUnspentOutPoint(&input.PreviousOutPoint) {
				subscribed[q] = struct{}{}
			}
		}

		for i, output := range msgTx.TxOut {
			_, addrs, _, err := txscript.ExtractPkScriptAddrs(
				txscript.DefaultScriptVersion,
				output.PkScript, m.server.server.chainParams)
			if err != nil {
				// Clients are not able to subscribe to
				// nonstandard or non-address outputs.
				continue
			}
			for _, a := range addrs {
				if f.existsAddress(a) {
					subscribed[q] = struct{}{}
					op := wire.OutPoint{
						Hash:  *tx.Hash(),
						Index: uint32(i),
						Tree:  tx.Tree(),
					}
					f.addUnspentOutPoint(&op)
				}
			}
		}

		f.mu.Unlock()
	}

	return subscribed
}

// notifyBlockConnected notifies websocket clients that have registered for
// block updates when a block is connected to the main chain.
func (m *wsNotificationManager) notifyBlockConnected(clients map[chan struct{}]*wsClient,
	block *dcrutil.Block) {

	// Create the common portion of the notification that is the same for
	// every client.
	headerBytes, err := block.MsgBlock().Header.Bytes()
	if err != nil {
		// This should never error.  The header is written to an
		// in-memory expandable buffer, and given that the block was
		// just accepted, there should be no issues serializing it.
		panic(err)
	}
	ntfn := dcrjson.BlockConnectedNtfn{
		Header:        hex.EncodeToString(headerBytes),
		SubscribedTxs: nil, // Set individually for each client
	}

	// Search for relevant transactions for each client and save them
	// serialized in hex encoding for the notification.
	subscribedTxs := make(map[chan struct{}][]string)
	for _, tx := range block.STransactions() {
		var txHex string
		for quitChan := range m.subscribedClients(tx, clients) {
			if txHex == "" {
				txHex = txHexString(tx.MsgTx())
			}
			subscribedTxs[quitChan] = append(subscribedTxs[quitChan], txHex)
		}
	}
	for _, tx := range block.Transactions() {
		var txHex string
		for quitChan := range m.subscribedClients(tx, clients) {
			if txHex == "" {
				txHex = txHexString(tx.MsgTx())
			}
			subscribedTxs[quitChan] = append(subscribedTxs[quitChan], txHex)
		}
	}

	for quitChan, client := range clients {
		// Add all previously discovered relevant transactions for this client,
		// if any.
		ntfn.SubscribedTxs = subscribedTxs[quitChan]

		// Marshal and queue notification.
		marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, &ntfn)
		if err != nil {
			rpcsLog.Errorf("Failed to marshal block connected "+
				"notification: %v", err)
			continue
		}
		client.QueueNotification(marshalledJSON)
	}
}

// notifyBlockDisconnected notifies websocket clients that have registered for
// block updates when a block is disconnected from the main chain (due to a
// reorganize).
func (*wsNotificationManager) notifyBlockDisconnected(clients map[chan struct{}]*wsClient, block *dcrutil.Block) {
	// Skip notification creation if no clients have requested block
	// connected/disconnected notifications.
	if len(clients) == 0 {
		return
	}

	// Notify interested websocket clients about the disconnected block.
	headerBytes, err := block.MsgBlock().Header.Bytes()
	if err != nil {
		// This should never error.  The header is written to an
		// in-memory expandable buffer, and given that the block was
		// previously accepted, there should be no issues serializing
		// it.
		panic(err)
	}
	ntfn := dcrjson.BlockDisconnectedNtfn{
		Header: hex.EncodeToString(headerBytes),
	}
	marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, &ntfn)
	if err != nil {
		rpcsLog.Errorf("Failed to marshal block disconnected "+
			"notification: %v", err)
		return
	}
	for _, wsc := range clients {
		wsc.QueueNotification(marshalledJSON)
	}
}

// notifyReorganization notifies websocket clients that have registered for
// block updates when the blockchain is beginning a reorganization.
func (m *wsNotificationManager) notifyReorganization(clients map[chan struct{}]*wsClient, rd *blockchain.ReorganizationNtfnsData) {
	// Skip notification creation if no clients have requested block
	// connected/disconnected notifications.
	if len(clients) == 0 {
		return
	}

	// Notify interested websocket clients about the disconnected block.
	ntfn := dcrjson.NewReorganizationNtfn(rd.OldHash.String(),
		int32(rd.OldHeight),
		rd.NewHash.String(),
		int32(rd.NewHeight))
	marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, ntfn)
	if err != nil {
		rpcsLog.Errorf("Failed to marshal reorganization "+
			"notification: %v", err)
		return
	}
	for _, wsc := range clients {
		wsc.QueueNotification(marshalledJSON)
	}
}

// RegisterWinningTickets requests winning tickets update notifications
// to the passed websocket client.
func (m *wsNotificationManager) RegisterWinningTickets(wsc *wsClient) {
	m.queueNotification <- (*notificationRegisterWinningTickets)(wsc)
}

// UnregisterWinningTickets removes winning ticket notifications for
// the passed websocket client.
func (m *wsNotificationManager) UnregisterWinningTickets(wsc *wsClient) {
	m.queueNotification <- (*notificationUnregisterWinningTickets)(wsc)
}

// notifyWinningTickets notifies websocket clients that have registered for
// winning ticket updates.
func (*wsNotificationManager) notifyWinningTickets(
	clients map[chan struct{}]*wsClient, wtnd *WinningTicketsNtfnData) {

	// Create a ticket map to export as JSON.
	ticketMap := make(map[string]string)
	for i, ticket := range wtnd.Tickets {
		ticketMap[strconv.Itoa(i)] = ticket.String()
	}

	// Notify interested websocket clients about the connected block.
	ntfn := dcrjson.NewWinningTicketsNtfn(wtnd.BlockHash.String(),
		int32(wtnd.BlockHeight), ticketMap)

	marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, ntfn)
	if err != nil {
		rpcsLog.Errorf("Failed to marshal winning tickets notification: "+
			"%v", err)
		return
	}

	for _, wsc := range clients {
		wsc.QueueNotification(marshalledJSON)
	}
}

// RegisterSpentAndMissedTickets requests spent/missed tickets update notifications
// to the passed websocket client.
func (m *wsNotificationManager) RegisterSpentAndMissedTickets(wsc *wsClient) {
	m.queueNotification <- (*notificationRegisterSpentAndMissedTickets)(wsc)
}

// UnregisterSpentAndMissedTickets removes spent/missed ticket notifications for
// the passed websocket client.
func (m *wsNotificationManager) UnregisterSpentAndMissedTickets(wsc *wsClient) {
	m.queueNotification <- (*notificationUnregisterSpentAndMissedTickets)(wsc)
}

// notifySpentAndMissedTickets notifies websocket clients that have registered for
// spent and missed ticket updates.
func (*wsNotificationManager) notifySpentAndMissedTickets(
	clients map[chan struct{}]*wsClient, tnd *blockchain.TicketNotificationsData) {

	// Create a ticket map to export as JSON.
	ticketMap := make(map[string]string)
	for _, ticket := range tnd.TicketsMissed {
		ticketMap[ticket.String()] = "missed"
	}
	for _, ticket := range tnd.TicketsSpent {
		ticketMap[ticket.String()] = "spent"
	}

	// Notify interested websocket clients about the connected block.
	ntfn := dcrjson.NewSpentAndMissedTicketsNtfn(tnd.Hash.String(),
		int32(tnd.Height), tnd.StakeDifficulty, ticketMap)

	marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, ntfn)
	if err != nil {
		rpcsLog.Errorf("Failed to marshal spent and missed tickets "+
			"notification: %v", err)
		return
	}

	for _, wsc := range clients {
		wsc.QueueNotification(marshalledJSON)
	}
}

// RegisterNewTickets requests spent/missed tickets update notifications
// to the passed websocket client.
func (m *wsNotificationManager) RegisterNewTickets(wsc *wsClient) {
	m.queueNotification <- (*notificationRegisterNewTickets)(wsc)
}

// UnregisterNewTickets removes spent/missed ticket notifications for
// the passed websocket client.
func (m *wsNotificationManager) UnregisterNewTickets(wsc *wsClient) {
	m.queueNotification <- (*notificationUnregisterNewTickets)(wsc)
}

// RegisterStakeDifficulty requests stake difficulty notifications
// to the passed websocket client.
func (m *wsNotificationManager) RegisterStakeDifficulty(wsc *wsClient) {
	m.queueNotification <- (*notificationRegisterStakeDifficulty)(wsc)
}

// UnregisterStakeDifficulty removes stake difficulty notifications for
// the passed websocket client.
func (m *wsNotificationManager) UnregisterStakeDifficulty(wsc *wsClient) {
	m.queueNotification <- (*notificationUnregisterStakeDifficulty)(wsc)
}

// notifyNewTickets notifies websocket clients that have registered for
// maturing ticket updates.
func (*wsNotificationManager) notifyNewTickets(clients map[chan struct{}]*wsClient,
	tnd *blockchain.TicketNotificationsData) {

	// Create a ticket map to export as JSON.
	var tickets []string
	for _, h := range tnd.TicketsNew {
		tickets = append(tickets, h.String())
	}

	// Notify interested websocket clients about the connected block.
	ntfn := dcrjson.NewNewTicketsNtfn(tnd.Hash.String(), int32(tnd.Height),
		tnd.StakeDifficulty, tickets)

	marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, ntfn)
	if err != nil {
		rpcsLog.Errorf("Failed to marshal new tickets notification: "+
			"%v", err)
		return
	}
	for _, wsc := range clients {
		wsc.QueueNotification(marshalledJSON)
	}
}

// notifyStakeDifficulty notifies websocket clients that have registered for
// maturing ticket updates.
func (*wsNotificationManager) notifyStakeDifficulty(
	clients map[chan struct{}]*wsClient,
	sdnd *StakeDifficultyNtfnData) {

	// Notify interested websocket clients about the connected block.
	ntfn := dcrjson.NewStakeDifficultyNtfn(sdnd.BlockHash.String(),
		int32(sdnd.BlockHeight),
		sdnd.StakeDifficulty)

	marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, ntfn)
	if err != nil {
		rpcsLog.Errorf("Failed to marshal stake difficulty notification: "+
			"%v", err)
		return
	}
	for _, wsc := range clients {
		wsc.QueueNotification(marshalledJSON)
	}
}

// RegisterNewMempoolTxsUpdates requests notifications to the passed websocket
// client when new transactions are added to the memory pool.
func (m *wsNotificationManager) RegisterNewMempoolTxsUpdates(wsc *wsClient) {
	m.queueNotification <- (*notificationRegisterNewMempoolTxs)(wsc)
}

// UnregisterNewMempoolTxsUpdates removes notifications to the passed websocket
// client when new transaction are added to the memory pool.
func (m *wsNotificationManager) UnregisterNewMempoolTxsUpdates(wsc *wsClient) {
	m.queueNotification <- (*notificationUnregisterNewMempoolTxs)(wsc)
}

// notifyForNewTx notifies websocket clients that have registered for updates
// when a new transaction is added to the memory pool.
func (m *wsNotificationManager) notifyForNewTx(clients map[chan struct{}]*wsClient, tx *dcrutil.Tx) {
	txHashStr := tx.Hash().String()
	mtx := tx.MsgTx()

	var amount int64
	for _, txOut := range mtx.TxOut {
		amount += txOut.Value
	}

	ntfn := dcrjson.NewTxAcceptedNtfn(txHashStr,
		dcrutil.Amount(amount).ToCoin())
	marshalledJSON, err := dcrjson.MarshalCmd("1.0", nil, ntfn)
	if err != nil {
		rpcsLog.Errorf("Failed to marshal tx notification: %s",
			err.Error())
		return
	}

	var verboseNtfn *dcrjson.TxAcceptedVerboseNtfn
	var marshalledJSONVerbose []byte
	for _, wsc := range clients {
		if wsc.verboseTxUpdates {
			if marshalledJSONVerbose != nil {
				wsc.QueueNotification(marshalledJSONVerbose)
				continue
			}

			net := m.server.server.chainParams
			rawTx, err := createTxRawResult(net, mtx, txHashStr,
				wire.NullBlockIndex, nil, "", 0, 0)
			if err != nil {
				return
			}

			verboseNtfn = dcrjson.NewTxAcceptedVerboseNtfn(*rawTx)
			marshalledJSONVerbose, err = dcrjson.MarshalCmd("1.0", nil,
				verboseNtfn)
			if err != nil {
				rpcsLog.Errorf("Failed to marshal verbose tx "+
					"notification: %s", err.Error())
				return
			}
			wsc.QueueNotification(marshalledJSONVerbose)
		} else {
			wsc.QueueNotification(marshalledJSON)
		}
	}
}

// txHexString returns the serialized transaction encoded in hexadecimal.
func txHexString(tx *wire.MsgTx) string {
	buf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
	// Ignore Serialize's error, as writing to a bytes.buffer cannot fail.
	tx.Serialize(buf)
	return hex.EncodeToString(buf.Bytes())
}

// notifyRelevantTxAccepted examines the inputs and outputs of the passed
// transaction, notifying websocket clients of outputs spending to a watched
// address and inputs spending a watched outpoint.  Any outputs paying to a
// watched address result in the output being watched as well for future
// notifications.
func (m *wsNotificationManager) notifyRelevantTxAccepted(tx *dcrutil.Tx,
	clients map[chan struct{}]*wsClient) {

	var clientsToNotify map[chan struct{}]*wsClient

	msgTx := tx.MsgTx()
	for q, c := range clients {
		c.Lock()
		f := c.filterData
		c.Unlock()
		if f == nil {
			continue
		}
		f.mu.Lock()

		for _, input := range msgTx.TxIn {
			if f.existsUnspentOutPoint(&input.PreviousOutPoint) {
				if clientsToNotify == nil {
					clientsToNotify = make(map[chan struct{}]*wsClient)
				}
				clientsToNotify[q] = c
			}
		}

		for i, output := range msgTx.TxOut {
			_, addrs, _, err := txscript.ExtractPkScriptAddrs(
				output.Version, output.PkScript,
				m.server.server.chainParams)
			if err != nil {
				continue
			}
			for _, a := range addrs {
				if f.existsAddress(a) {
					if clientsToNotify == nil {
						clientsToNotify = make(map[chan struct{}]*wsClient)
					}
					clientsToNotify[q] = c

					op := wire.OutPoint{
						Hash:  *tx.Hash(),
						Index: uint32(i),
						Tree:  tx.Tree(),
					}
					f.addUnspentOutPoint(&op)
				}
			}
		}

		f.mu.Unlock()
	}

	if len(clientsToNotify) != 0 {
		n := dcrjson.NewRelevantTxAcceptedNtfn(txHexString(msgTx))
		marshalled, err := dcrjson.MarshalCmd("1.0", nil, n)
		if err != nil {
			rpcsLog.Errorf("Failed to marshal notification: %v", err)
			return
		}
		for _, c := range clientsToNotify {
			c.QueueNotification(marshalled)
		}
	}
}

// AddClient adds the passed websocket client to the notification manager.
func (m *wsNotificationManager) AddClient(wsc *wsClient) {
	m.queueNotification <- (*notificationRegisterClient)(wsc)
}

// RemoveClient removes the passed websocket client and all notifications
// registered for it.
func (m *wsNotificationManager) RemoveClient(wsc *wsClient) {
	select {
	case m.queueNotification <- (*notificationUnregisterClient)(wsc):
	case <-m.quit:
	}
}

// Start starts the goroutines required for the manager to queue and process
// websocket client notifications.
func (m *wsNotificationManager) Start() {
	m.wg.Add(2)
	go m.queueHandler()
	go m.notificationHandler()
}

// WaitForShutdown blocks until all notification manager goroutines have
// finished.
func (m *wsNotificationManager) WaitForShutdown() {
	m.wg.Wait()
}

// Shutdown shuts down the manager, stopping the notification queue and
// notification handler goroutines.
func (m *wsNotificationManager) Shutdown() {
	close(m.quit)
}

// newWsNotificationManager returns a new notification manager ready for use.
// See wsNotificationManager for more details.
func newWsNotificationManager(server *rpcServer) *wsNotificationManager {
	return &wsNotificationManager{
		server:            server,
		queueNotification: make(chan interface{}),
		notificationMsgs:  make(chan interface{}),
		numClients:        make(chan int),
		quit:              make(chan struct{}),
	}
}

// wsResponse houses a message to send to a connected websocket client as
// well as a channel to reply on when the message is sent.
type wsResponse struct {
	msg      []byte
	doneChan chan bool
}

// wsClient provides an abstraction for handling a websocket client. The overall
// data flow is split into 3 main goroutines. A websocket manager is used to
// allow things such as broadcasting requested notifications to all connected
// websocket clients. Inbound messages are read via the inHandler goroutine and
// generally dispatched to their own handler. There are two outbound message
// types - one for responding to client requests and another for async
// notifications. Responses to client requests use SendMessage which employs a
// buffered channel thereby limiting the number of outstanding requests that can
// be made. Notifications are sent via QueueNotification which implements a
// queue via notificationQueueHandler to ensure sending notifications from other
// subsystems can't block.  Ultimately, all messages are sent via the
// outHandler.
type wsClient struct {
	sync.Mutex

	// server is the RPC server that is servicing the client.
	server *rpcServer

	// conn is the underlying websocket connection.
	conn *websocket.Conn

	// disconnected indicated whether or not the websocket client is
	// disconnected.
	disconnected bool

	// addr is the remote address of the client.
	addr string

	// authenticated specifies whether a client has been authenticated
	// and therefore is allowed to communicated over the websocket.
	authenticated bool

	// isAdmin specifies whether a client may change the state of the server;
	// false means its access is only to the limited set of RPC calls.
	isAdmin bool

	// sessionID is a random ID generated for each client when connected.
	// These IDs may be queried by a client using the session RPC.  A change
	// to the session ID indicates that the client reconnected.
	sessionID uint64

	// verboseTxUpdates specifies whether a client has requested verbose
	// information about all new transactions.
	verboseTxUpdates bool

	filterData *wsClientFilter

	// Networking infrastructure.
	serviceRequestSem semaphore
	ntfnChan          chan []byte
	sendChan          chan wsResponse
	quit              chan struct{}
	wg                sync.WaitGroup
}

// inHandler handles all incoming messages for the websocket connection.  It
// must be run as a goroutine.
func (c *wsClient) inHandler() {
out:
	for {
		// Break out of the loop once the quit channel has been closed.
		// Use a non-blocking select here so we fall through otherwise.
		select {
		case <-c.quit:
			break out
		default:
		}

		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			// Log the error if it's not due to disconnecting.
			if err != io.EOF {
				rpcsLog.Errorf("Websocket receive error from "+
					"%s: %v", c.addr, err)
			}
			break out
		}

		var batchedRequest bool

		// Determine request type
		if bytes.HasPrefix(msg, batchedRequestPrefix) {
			batchedRequest = true
		}

		// Process a single request
		if !batchedRequest {
			var req dcrjson.Request
			var reply json.RawMessage
			err = json.Unmarshal(msg, &req)
			if err != nil {
				// only process requests from authenticated clients
				if !c.authenticated {
					break out
				}

				jsonErr := &dcrjson.RPCError{
					Code:    dcrjson.ErrRPCParse.Code,
					Message: "Failed to parse request: " + err.Error(),
				}
				reply, err = createMarshalledReply("1.0", nil, nil, jsonErr)
				if err != nil {
					rpcsLog.Errorf("Failed to marshal reply: %v", err)
					continue
				}
				c.SendMessage(reply, nil)
				continue
			}

			if req.Method == "" || req.Params == nil {
				jsonErr := &dcrjson.RPCError{
					Code:    dcrjson.ErrRPCInvalidRequest.Code,
					Message: fmt.Sprintf("Invalid request: malformed"),
				}
				reply, err := createMarshalledReply(req.Jsonrpc, req.ID, nil, jsonErr)
				if err != nil {
					rpcsLog.Errorf("Failed to marshal reply: %v", err)
					continue
				}
				c.SendMessage(reply, nil)
				continue
			}

			// Valid requests with no ID (notifications) must not have a response
			// per the JSON-RPC spec.
			if req.ID == nil {
				if !c.authenticated {
					break out
				}
				continue
			}

			cmd := parseCmd(&req)
			if cmd.err != nil {
				// Only process requests from authenticated clients
				if !c.authenticated {
					break out
				}

				reply, err = createMarshalledReply(cmd.jsonrpc, cmd.id, nil, cmd.err)
				if err != nil {
					rpcsLog.Errorf("Failed to marshal reply: %v", err)
					continue
				}
				c.SendMessage(reply, nil)
				continue
			}

			rpcsLog.Debugf("Received command <%s> from %s", cmd.method, c.addr)

			// Check auth.  The client is immediately disconnected if the
			// first request of an unauthentiated websocket client is not
			// the authenticate request, an authenticate request is received
			// when the client is already authenticated, or incorrect
			// authentication credentials are provided in the request.
			switch authCmd, ok := cmd.cmd.(*dcrjson.AuthenticateCmd); {
			case c.authenticated && ok:
				rpcsLog.Warnf("Websocket client %s is already authenticated",
					c.addr)
				break out
			case !c.authenticated && !ok:
				rpcsLog.Warnf("Unauthenticated websocket message " +
					"received")
				break out
			case !c.authenticated:
				// Check credentials.
				login := authCmd.Username + ":" + authCmd.Passphrase
				auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(login))
				authSha := sha256.Sum256([]byte(auth))
				cmp := subtle.ConstantTimeCompare(authSha[:], c.server.authsha[:])
				limitcmp := subtle.ConstantTimeCompare(authSha[:], c.server.limitauthsha[:])
				if cmp != 1 && limitcmp != 1 {
					rpcsLog.Warnf("Auth failure.")
					break out
				}
				c.authenticated = true
				c.isAdmin = cmp == 1

				// Marshal and send response.
				reply, err = createMarshalledReply(cmd.jsonrpc, cmd.id, nil, nil)
				if err != nil {
					rpcsLog.Errorf("Failed to marshal authenticate reply: "+
						"%v", err.Error())
					continue
				}
				c.SendMessage(reply, nil)
				continue
			}

			// Check if the client is using limited RPC credentials and
			// error when not authorized to call the supplied RPC.
			if !c.isAdmin {
				if _, ok := rpcLimited[req.Method]; !ok {
					jsonErr := &dcrjson.RPCError{
						Code:    dcrjson.ErrRPCInvalidParams.Code,
						Message: "limited user not authorized for this method",
					}
					// Marshal and send response.
					reply, err = createMarshalledReply("", req.ID, nil, jsonErr)
					if err != nil {
						rpcsLog.Errorf("Failed to marshal parse failure "+
							"reply: %v", err)
						continue
					}
					c.SendMessage(reply, nil)
					continue
				}
			}

			// Asynchronously handle the request.  A semaphore is used to
			// limit the number of concurrent requests currently being
			// serviced.  If the semaphore can not be acquired, simply wait
			// until a request finished before reading the next RPC request
			// from the websocket client.
			//
			// This could be a little fancier by timing out and erroring
			// when it takes too long to service the request, but if that is
			// done, the read of the next request should not be blocked by
			// this semaphore, otherwise the next request will be read and
			// will probably sit here for another few seconds before timing
			// out as well.  This will cause the total timeout duration for
			// later requests to be much longer than the check here would
			// imply.
			//
			// If a timeout is added, the semaphore acquiring should be
			// moved inside of the new goroutine with a select statement
			// that also reads a time.After channel.  This will unblock the
			// read of the next request from the websocket client and allow
			// many requests to be waited on concurrently.
			c.serviceRequestSem.acquire()
			go func() {
				c.serviceRequest(cmd)
				c.serviceRequestSem.release()
			}()
		}

		// Process a batched request
		if batchedRequest {
			var batchedRequests []interface{}
			var results []json.RawMessage
			var batchSize int
			var reply json.RawMessage
			c.serviceRequestSem.acquire()
			err = json.Unmarshal(msg, &batchedRequests)
			if err != nil {
				// Only process requests from authenticated clients
				if !c.authenticated {
					break out
				}

				jsonErr := &dcrjson.RPCError{
					Code: dcrjson.ErrRPCParse.Code,
					Message: fmt.Sprintf("Failed to parse request: %v",
						err),
				}
				reply, err = dcrjson.MarshalResponse("2.0", nil, nil, jsonErr)
				if err != nil {
					rpcsLog.Errorf("Failed to create reply: %v", err)
				}

				if reply != nil {
					results = append(results, reply)
				}
			}

			if err == nil {
				// Response with an empty batch error if the batch size is zero
				if len(batchedRequests) == 0 {
					if !c.authenticated {
						break out
					}

					jsonErr := &dcrjson.RPCError{
						Code:    dcrjson.ErrRPCInvalidRequest.Code,
						Message: fmt.Sprint("Invalid request: empty batch"),
					}
					reply, err = dcrjson.MarshalResponse("2.0", nil, nil, jsonErr)
					if err != nil {
						rpcsLog.Errorf("Failed to marshal reply: %v", err)
					}

					if reply != nil {
						results = append(results, reply)
					}
				}

				// Process each batch entry individually
				if len(batchedRequests) > 0 {
					batchSize = len(batchedRequests)
					for _, entry := range batchedRequests {
						var reqBytes []byte
						reqBytes, err = json.Marshal(entry)
						if err != nil {
							// Only process requests from authenticated clients
							if !c.authenticated {
								break out
							}

							jsonErr := &dcrjson.RPCError{
								Code: dcrjson.ErrRPCInvalidRequest.Code,
								Message: fmt.Sprintf("Invalid request: %v",
									err),
							}
							reply, err = dcrjson.MarshalResponse("2.0", nil, nil, jsonErr)
							if err != nil {
								rpcsLog.Errorf("Failed to create reply: %v", err)
								continue
							}

							if reply != nil {
								results = append(results, reply)
							}
							continue
						}

						var req dcrjson.Request
						err := json.Unmarshal(reqBytes, &req)
						if err != nil {
							// Only process requests from authenticated clients
							if !c.authenticated {
								break out
							}

							jsonErr := &dcrjson.RPCError{
								Code: dcrjson.ErrRPCInvalidRequest.Code,
								Message: fmt.Sprintf("Invalid request: %v",
									err),
							}
							reply, err = dcrjson.MarshalResponse("2.0", nil, nil, jsonErr)
							if err != nil {
								rpcsLog.Errorf("Failed to create reply: %v", err)
								continue
							}

							if reply != nil {
								results = append(results, reply)
							}
							continue
						}

						if req.Method == "" || req.Params == nil {
							jsonErr := &dcrjson.RPCError{
								Code:    dcrjson.ErrRPCInvalidRequest.Code,
								Message: fmt.Sprintf("Invalid request: malformed"),
							}
							reply, err := createMarshalledReply(req.Jsonrpc, req.ID, nil, jsonErr)
							if err != nil {
								rpcsLog.Errorf("Failed to marshal reply: %v", err)
								continue
							}

							if reply != nil {
								results = append(results, reply)
							}
							continue
						}

						// Valid requests with no ID (notifications) must not have a response
						// per the JSON-RPC spec.
						if req.ID == nil {
							if !c.authenticated {
								break out
							}
							continue
						}

						cmd := parseCmd(&req)
						if cmd.err != nil {
							// Only process requests from authenticated clients
							if !c.authenticated {
								break out
							}

							reply, err = createMarshalledReply(cmd.jsonrpc, cmd.id, nil, cmd.err)
							if err != nil {
								rpcsLog.Errorf("Failed to marshal reply: %v", err)
								continue
							}

							if reply != nil {
								results = append(results, reply)
							}
							continue
						}

						rpcsLog.Debugf("Received command <%s> from %s", cmd.method, c.addr)

						// Check auth.  The client is immediately disconnected if the
						// first request of an unauthentiated websocket client is not
						// the authenticate request, an authenticate request is received
						// when the client is already authenticated, or incorrect
						// authentication credentials are provided in the request.
						switch authCmd, ok := cmd.cmd.(*dcrjson.AuthenticateCmd); {
						case c.authenticated && ok:
							rpcsLog.Warnf("Websocket client %s is already authenticated",
								c.addr)
							break out
						case !c.authenticated && !ok:
							rpcsLog.Warnf("Unauthenticated websocket message " +
								"received")
							break out
						case !c.authenticated:
							// Check credentials.
							login := authCmd.Username + ":" + authCmd.Passphrase
							auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(login))
							authSha := sha256.Sum256([]byte(auth))
							cmp := subtle.ConstantTimeCompare(authSha[:], c.server.authsha[:])
							limitcmp := subtle.ConstantTimeCompare(authSha[:], c.server.limitauthsha[:])
							if cmp != 1 && limitcmp != 1 {
								rpcsLog.Warnf("Auth failure.")
								break out
							}

							c.authenticated = true
							c.isAdmin = cmp == 1

							// Marshal and send response.
							reply, err = createMarshalledReply(cmd.jsonrpc, cmd.id, nil, nil)
							if err != nil {
								rpcsLog.Errorf("Failed to marshal authenticate reply: "+
									"%v", err.Error())
								continue
							}

							if reply != nil {
								results = append(results, reply)
							}
							continue
						}

						// Check if the client is using limited RPC credentials and
						// error when not authorized to call the supplied RPC.
						if !c.isAdmin {
							if _, ok := rpcLimited[req.Method]; !ok {
								jsonErr := &dcrjson.RPCError{
									Code:    dcrjson.ErrRPCInvalidParams.Code,
									Message: "limited user not authorized for this method",
								}
								// Marshal and send response.
								reply, err = createMarshalledReply(req.Jsonrpc, req.ID, nil, jsonErr)
								if err != nil {
									rpcsLog.Errorf("Failed to marshal parse failure "+
										"reply: %v", err)
									continue
								}

								if reply != nil {
									results = append(results, reply)
								}
								continue
							}
						}

						// Lookup the websocket extension for the command, if it doesn't
						// exist fallback to handling the command as a standard command.
						var resp interface{}
						wsHandler, ok := wsHandlers[cmd.method]
						if ok {
							resp, err = wsHandler(c, cmd.cmd)
						} else {
							resp, err = c.server.standardCmdResult(cmd, nil)
						}

						// Marshal request output.
						reply, err := createMarshalledReply(cmd.jsonrpc, cmd.id, resp, err)
						if err != nil {
							rpcsLog.Errorf("Failed to marshal reply for <%s> "+
								"command: %v", cmd.method, err)
							return
						}

						if reply != nil {
							results = append(results, reply)
						}
					}
				}
			}

			// generate reply
			var payload = []byte{}
			if batchedRequest && batchSize > 0 {
				if len(results) > 0 {
					// Form the batched response json
					var buffer bytes.Buffer
					buffer.WriteByte('[')
					for idx, marshalledReply := range results {
						if idx == len(results)-1 {
							buffer.Write(marshalledReply)
							buffer.WriteByte(']')
							break
						}
						buffer.Write(marshalledReply)
						buffer.WriteByte(',')
					}
					payload = buffer.Bytes()
				}
			}

			if !batchedRequest || batchSize == 0 {
				// Respond with the first results entry for single requests
				if len(results) > 0 {
					payload = results[0]
				}
			}

			c.SendMessage(payload, nil)
			c.serviceRequestSem.release()
		}
	}

	// Ensure the connection is closed.
	c.Disconnect()
	c.wg.Done()
	rpcsLog.Tracef("Websocket client input handler done for %s", c.addr)
}

// serviceRequest services a parsed RPC request by looking up and executing the
// appropriate RPC handler.  The response is marshalled and sent to the websocket
// client.
func (c *wsClient) serviceRequest(r *parsedRPCCmd) {
	var (
		result interface{}
		err    error
	)

	// Lookup the websocket extension for the command and if it doesn't
	// exist fallback to handling the command as a standard command.
	wsHandler, ok := wsHandlers[r.method]
	if ok {
		result, err = wsHandler(c, r.cmd)
	} else {
		result, err = c.server.standardCmdResult(r, nil)
	}
	reply, err := createMarshalledReply(r.jsonrpc, r.id, result, err)
	if err != nil {
		rpcsLog.Errorf("Failed to marshal reply for <%s> "+
			"command: %v", r.method, err)
		return
	}

	c.SendMessage(reply, nil)
}

// notificationQueueHandler handles the queuing of outgoing notifications for
// the websocket client.  This runs as a muxer for various sources of input to
// ensure that queuing up notifications to be sent will not block.  Otherwise,
// slow clients could bog down the other systems (such as the mempool or block
// manager) which are queuing the data.  The data is passed on to outHandler to
// actually be written.  It must be run as a goroutine.
func (c *wsClient) notificationQueueHandler() {
	ntfnSentChan := make(chan bool, 1) // nonblocking sync

	// pendingNtfns is used as a queue for notifications that are ready to
	// be sent once there are no outstanding notifications currently being
	// sent.  The waiting flag is used over simply checking for items in the
	// pending list to ensure cleanup knows what has and hasn't been sent
	// to the outHandler.  Currently no special cleanup is needed, however
	// if something like a done channel is added to notifications in the
	// future, not knowing what has and hasn't been sent to the outHandler
	// (and thus who should respond to the done channel) would be
	// problematic without using this approach.
	pendingNtfns := list.New()
	waiting := false
out:
	for {
		select {
		// This channel is notified when a message is being queued to
		// be sent across the network socket.  It will either send the
		// message immediately if a send is not already in progress, or
		// queue the message to be sent once the other pending messages
		// are sent.
		case msg := <-c.ntfnChan:
			if !waiting {
				c.SendMessage(msg, ntfnSentChan)
			} else {
				pendingNtfns.PushBack(msg)
			}
			waiting = true

		// This channel is notified when a notification has been sent
		// across the network socket.
		case <-ntfnSentChan:
			// No longer waiting if there are no more messages in
			// the pending messages queue.
			next := pendingNtfns.Front()
			if next == nil {
				waiting = false
				continue
			}

			// Notify the outHandler about the next item to
			// asynchronously send.
			msg := pendingNtfns.Remove(next).([]byte)
			c.SendMessage(msg, ntfnSentChan)

		case <-c.quit:
			break out
		}
	}

	// Drain any wait channels before exiting so nothing is left waiting
	// around to send.
cleanup:
	for {
		select {
		case <-c.ntfnChan:
		case <-ntfnSentChan:
		default:
			break cleanup
		}
	}
	c.wg.Done()
	rpcsLog.Tracef("Websocket client notification queue handler done "+
		"for %s", c.addr)
}

// outHandler handles all outgoing messages for the websocket connection.  It
// must be run as a goroutine.  It uses a buffered channel to serialize output
// messages while allowing the sender to continue running asynchronously.  It
// must be run as a goroutine.
func (c *wsClient) outHandler() {
out:
	for {
		// Send any messages ready for send until the quit channel is
		// closed.
		select {
		case r := <-c.sendChan:
			err := c.conn.WriteMessage(websocket.TextMessage, r.msg)
			if err != nil {
				c.Disconnect()
				break out
			}
			if r.doneChan != nil {
				r.doneChan <- true
			}

		case <-c.quit:
			break out
		}
	}

	// Drain any wait channels before exiting so nothing is left waiting
	// around to send.
cleanup:
	for {
		select {
		case r := <-c.sendChan:
			if r.doneChan != nil {
				r.doneChan <- false
			}
		default:
			break cleanup
		}
	}
	c.wg.Done()
	rpcsLog.Tracef("Websocket client output handler done for %s", c.addr)
}

// SendMessage sends the passed json to the websocket client.  It is backed
// by a buffered channel, so it will not block until the send channel is full.
// Note however that QueueNotification must be used for sending async
// notifications instead of the this function.  This approach allows a limit to
// the number of outstanding requests a client can make without preventing or
// blocking on async notifications.
func (c *wsClient) SendMessage(marshalledJSON []byte, doneChan chan bool) {
	// Don't send the message if disconnected.
	if c.Disconnected() {
		if doneChan != nil {
			doneChan <- false
		}
		return
	}

	c.sendChan <- wsResponse{msg: marshalledJSON, doneChan: doneChan}
}

// ErrClientQuit describes the error where a client send is not processed due
// to the client having already been disconnected or dropped.
var ErrClientQuit = errors.New("client quit")

// QueueNotification queues the passed notification to be sent to the websocket
// client.  This function, as the name implies, is only intended for
// notifications since it has additional logic to prevent other subsystems, such
// as the memory pool and block manager, from blocking even when the send
// channel is full.
//
// If the client is in the process of shutting down, this function returns
// ErrClientQuit.  This is intended to be checked by long-running notification
// handlers to stop processing if there is no more work needed to be done.
func (c *wsClient) QueueNotification(marshalledJSON []byte) error {
	// Don't queue the message if disconnected.
	if c.Disconnected() {
		return ErrClientQuit
	}

	c.ntfnChan <- marshalledJSON
	return nil
}

// Disconnected returns whether or not the websocket client is disconnected.
func (c *wsClient) Disconnected() bool {
	c.Lock()
	isDisconnected := c.disconnected
	c.Unlock()

	return isDisconnected
}

// Disconnect disconnects the websocket client.
func (c *wsClient) Disconnect() {
	c.Lock()
	defer c.Unlock()

	// Nothing to do if already disconnected.
	if c.disconnected {
		return
	}

	rpcsLog.Tracef("Disconnecting websocket client %s", c.addr)
	close(c.quit)
	c.conn.Close()
	c.disconnected = true
}

// Start begins processing input and output messages.
func (c *wsClient) Start() {
	rpcsLog.Tracef("Starting websocket client %s", c.addr)

	// Start processing input and output.
	c.wg.Add(3)
	go c.inHandler()
	go c.notificationQueueHandler()
	go c.outHandler()
}

// WaitForShutdown blocks until the websocket client goroutines are stopped
// and the connection is closed.
func (c *wsClient) WaitForShutdown() {
	c.wg.Wait()
}

// newWebsocketClient returns a new websocket client given the notification
// manager, websocket connection, remote address, and whether or not the client
// has already been authenticated (via HTTP Basic access authentication).  The
// returned client is ready to start.  Once started, the client will process
// incoming and outgoing messages in separate goroutines complete with queuing
// and asynchrous handling for long-running operations.
func newWebsocketClient(server *rpcServer, conn *websocket.Conn,
	remoteAddr string, authenticated bool, isAdmin bool) (*wsClient, error) {

	sessionID, err := wire.RandomUint64()
	if err != nil {
		return nil, err
	}

	client := &wsClient{
		conn:              conn,
		addr:              remoteAddr,
		authenticated:     authenticated,
		isAdmin:           isAdmin,
		sessionID:         sessionID,
		server:            server,
		serviceRequestSem: makeSemaphore(cfg.RPCMaxConcurrentReqs),
		ntfnChan:          make(chan []byte, 1), // nonblocking sync
		sendChan:          make(chan wsResponse, websocketSendBufferSize),
		quit:              make(chan struct{}),
	}
	return client, nil
}

// handleWebsocketHelp implements the help command for websocket connections.
func handleWebsocketHelp(wsc *wsClient, icmd interface{}) (interface{}, error) {
	cmd, ok := icmd.(*dcrjson.HelpCmd)
	if !ok {
		return nil, dcrjson.ErrRPCInternal
	}

	// Provide a usage overview of all commands when no specific command
	// was specified.
	var command string
	if cmd.Command != nil {
		command = *cmd.Command
	}
	if command == "" {
		usage, err := wsc.server.helpCacher.rpcUsage(true)
		if err != nil {
			context := "Failed to generate RPC usage"
			return nil, rpcInternalError(err.Error(), context)
		}
		return usage, nil
	}

	// Check that the command asked for is supported and implemented.
	// Search the list of websocket handlers as well as the main list of
	// handlers since help should only be provided for those cases.
	valid := true
	if _, ok := rpcHandlers[command]; !ok {
		if _, ok := wsHandlers[command]; !ok {
			valid = false
		}
	}
	if !valid {
		return nil, &dcrjson.RPCError{
			Code:    dcrjson.ErrRPCInvalidParameter,
			Message: "Unknown command: " + command,
		}
	}

	// Get the help for the command.
	help, err := wsc.server.helpCacher.rpcMethodHelp(command)
	if err != nil {
		context := "Failed to generate help"
		return nil, rpcInternalError(err.Error(), context)
	}
	return help, nil
}

// handleLoadTxFilter implements the loadtxfilter command extension for
// websocket connections.
func handleLoadTxFilter(wsc *wsClient, icmd interface{}) (interface{}, error) {
	cmd := icmd.(*dcrjson.LoadTxFilterCmd)

	outPoints := make([]*wire.OutPoint, len(cmd.OutPoints))
	for i := range cmd.OutPoints {
		hash, err := chainhash.NewHashFromStr(cmd.OutPoints[i].Hash)
		if err != nil {
			return nil, &dcrjson.RPCError{
				Code:    dcrjson.ErrRPCInvalidParameter,
				Message: err.Error(),
			}
		}
		outPoints[i] = &wire.OutPoint{
			Hash:  *hash,
			Index: cmd.OutPoints[i].Index,
			Tree:  cmd.OutPoints[i].Tree,
		}
	}

	wsc.Lock()
	if cmd.Reload || wsc.filterData == nil {
		wsc.filterData = makeWSClientFilter(cmd.Addresses, outPoints)
		wsc.Unlock()
	} else {
		filter := wsc.filterData
		wsc.Unlock()

		filter.mu.Lock()
		for _, a := range cmd.Addresses {
			filter.addAddressStr(a)
		}
		for _, op := range outPoints {
			filter.addUnspentOutPoint(op)
		}
		filter.mu.Unlock()
	}

	return nil, nil
}

// handleNotifyBlocks implements the notifyblocks command extension for
// websocket connections.
func handleNotifyBlocks(wsc *wsClient, icmd interface{}) (interface{}, error) {
	wsc.server.ntfnMgr.RegisterBlockUpdates(wsc)
	return nil, nil
}

// handleSession implements the session command extension for websocket
// connections.
func handleSession(wsc *wsClient, icmd interface{}) (interface{}, error) {
	return &dcrjson.SessionResult{SessionID: wsc.sessionID}, nil
}

// handleWinningTickets implements the notifywinningtickets command
// extension for websocket connections.
func handleWinningTickets(wsc *wsClient, icmd interface{}) (interface{},
	error) {
	wsc.server.ntfnMgr.RegisterWinningTickets(wsc)
	return nil, nil
}

// handleSpentAndMissedTickets implements the notifyspentandmissedtickets command
// extension for websocket connections.
func handleSpentAndMissedTickets(wsc *wsClient, icmd interface{}) (interface{},
	error) {
	wsc.server.ntfnMgr.RegisterSpentAndMissedTickets(wsc)
	return nil, nil
}

// handleNewTickets implements the notifynewtickets command extension for
// websocket connections.
func handleNewTickets(wsc *wsClient, icmd interface{}) (interface{},
	error) {
	wsc.server.ntfnMgr.RegisterNewTickets(wsc)
	return nil, nil
}

// handleStakeDifficulty implements the notifystakedifficulty command extension
// for websocket connections.
func handleStakeDifficulty(wsc *wsClient, icmd interface{}) (interface{},
	error) {
	wsc.server.ntfnMgr.RegisterStakeDifficulty(wsc)
	return nil, nil
}

// handleStopNotifyBlocks implements the stopnotifyblocks command extension for
// websocket connections.
func handleStopNotifyBlocks(wsc *wsClient, icmd interface{}) (interface{}, error) {
	wsc.server.ntfnMgr.UnregisterBlockUpdates(wsc)
	return nil, nil
}

// handleNotifyNewTransations implements the notifynewtransactions command
// extension for websocket connections.
func handleNotifyNewTransactions(wsc *wsClient, icmd interface{}) (interface{}, error) {
	cmd, ok := icmd.(*dcrjson.NotifyNewTransactionsCmd)
	if !ok {
		return nil, dcrjson.ErrRPCInternal
	}

	wsc.verboseTxUpdates = cmd.Verbose != nil && *cmd.Verbose
	wsc.server.ntfnMgr.RegisterNewMempoolTxsUpdates(wsc)
	return nil, nil
}

// handleStopNotifyNewTransations implements the stopnotifynewtransactions
// command extension for websocket connections.
func handleStopNotifyNewTransactions(wsc *wsClient, icmd interface{}) (interface{}, error) {
	wsc.server.ntfnMgr.UnregisterNewMempoolTxsUpdates(wsc)
	return nil, nil
}

// rescanBlock rescans a block for any relevant transactions for the passed
// lookup keys.  Any discovered transactions are returned hex encoded as a
// string slice.
func rescanBlock(filter *wsClientFilter, block *dcrutil.Block) []string {
	var transactions []string

	// Need to iterate over both the stake and regular transactions in a
	// block, but these are two different slices in the MsgTx.  To avoid
	// another allocation to create a single slice to range over, the loop
	// body logic is run from a closure.
	//
	// This makes unsynchronized calls to the filter and thus must only be
	// called with the filter mutex held.
	checkTransaction := func(tx *wire.MsgTx, tree int8) {
		// Keep track of whether the transaction has already been added
		// to the result.  It shouldn't be added twice.
		added := false

		inputs := tx.TxIn
		if tree == wire.TxTreeRegular {
			// Skip previous output checks for coinbase inputs.  These do
			// not reference a previous output.
			if blockchain.IsCoinBaseTx(tx) {
				goto LoopOutputs
			}
		} else {
			if stake.DetermineTxType(tx) == stake.TxTypeSSGen {
				// Skip the first stakebase input.  These do not
				// reference a previous output.
				inputs = inputs[1:]
			}
		}
		for _, input := range inputs {
			if !filter.existsUnspentOutPoint(&input.PreviousOutPoint) {
				continue
			}
			if !added {
				transactions = append(transactions, txHexString(tx))
				added = true
			}
		}

	LoopOutputs:
		for i, output := range tx.TxOut {
			_, addrs, _, err := txscript.ExtractPkScriptAddrs(
				output.Version, output.PkScript,
				activeNetParams.Params)
			if err != nil {
				continue
			}
			for _, a := range addrs {
				if !filter.existsAddress(a) {
					continue
				}

				op := wire.OutPoint{
					Hash:  tx.TxHash(),
					Index: uint32(i),
					Tree:  tree,
				}
				filter.addUnspentOutPoint(&op)

				if !added {
					transactions = append(transactions, txHexString(tx))
					added = true
				}
			}
		}
	}

	msgBlock := block.MsgBlock()
	filter.mu.Lock()
	for _, tx := range msgBlock.STransactions {
		checkTransaction(tx, wire.TxTreeStake)
	}
	for _, tx := range msgBlock.Transactions {
		checkTransaction(tx, wire.TxTreeRegular)
	}
	filter.mu.Unlock()

	return transactions
}

// handleRescan implements the rescan command extension for websocket
// connections.
func handleRescan(wsc *wsClient, icmd interface{}) (interface{}, error) {
	cmd, ok := icmd.(*dcrjson.RescanCmd)
	if !ok {
		return nil, dcrjson.ErrRPCInternal
	}

	// Load client's transaction filter.  Must exist in order to continue.
	wsc.Lock()
	filter := wsc.filterData
	wsc.Unlock()
	if filter == nil {
		return nil, &dcrjson.RPCError{
			Code:    dcrjson.ErrRPCMisc,
			Message: "Transaction filter must be loaded before rescanning",
		}
	}

	blockHashes, err := dcrjson.DecodeConcatenatedHashes(cmd.BlockHashes)
	if err != nil {
		return nil, err
	}

	discoveredData := make([]dcrjson.RescannedBlock, 0, len(blockHashes))

	// Iterate over each block in the request and rescan.  When a block
	// contains relevant transactions, add it to the response.
	bc := wsc.server.server.blockManager.chain
	var lastBlockHash *chainhash.Hash
	for i := range blockHashes {
		block, err := bc.BlockByHash(&blockHashes[i])
		if err != nil {
			return nil, &dcrjson.RPCError{
				Code:    dcrjson.ErrRPCBlockNotFound,
				Message: "Failed to fetch block: " + err.Error(),
			}
		}
		if lastBlockHash != nil && block.MsgBlock().Header.PrevBlock != *lastBlockHash {
			return nil, &dcrjson.RPCError{
				Code: dcrjson.ErrRPCInvalidParameter,
				Message: fmt.Sprintf("Block %v is not a child of %v",
					&blockHashes[i], lastBlockHash),
			}
		}
		lastBlockHash = &blockHashes[i]

		transactions := rescanBlock(filter, block)
		if len(transactions) != 0 {
			discoveredData = append(discoveredData, dcrjson.RescannedBlock{
				Hash:         blockHashes[i].String(),
				Transactions: transactions,
			})
		}
	}

	return &dcrjson.RescanResult{DiscoveredData: discoveredData}, nil
}

func init() {
	wsHandlers = wsHandlersBeforeInit
}
