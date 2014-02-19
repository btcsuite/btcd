// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"code.google.com/p/go.net/websocket"
	"container/list"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/conformal/btcws"
	"io"
	"sync"
	"time"
)

const (
	// websocketSendBufferSize is the number of elements the send channel
	// can queue before blocking.  Note that this only applies to requests
	// handled directly in the websocket client input handler or the async
	// handler since notifications have their own queueing mechanism
	// independent of the send channel buffer.
	websocketSendBufferSize = 50
)

// timeZeroVal is simply the zero value for a time.Time and is used to avoid
// creating multiple instances.
var timeZeroVal time.Time

// wsCommandHandler describes a callback function used to handle a specific
// command.
type wsCommandHandler func(*wsClient, btcjson.Cmd) (interface{}, *btcjson.Error)

// wsHandlers maps RPC command strings to appropriate websocket handler
// functions.
var wsHandlers = map[string]wsCommandHandler{
	"getbestblock":    handleGetBestBlock,
	"getcurrentnet":   handleGetCurrentNet,
	"notifyblocks":    handleNotifyBlocks,
	"notifyallnewtxs": handleNotifyAllNewTXs,
	"notifynewtxs":    handleNotifyNewTXs,
	"notifyspent":     handleNotifySpent,
	"rescan":          handleRescan,
}

// wsAsyncHandlers holds the websocket commands which should be run
// asynchronously to the main input handler goroutine.  This allows long-running
// operations to run concurrently (and one at a time) while still responding
// to the majority of normal requests which can be answered quickly.
var wsAsyncHandlers = map[string]bool{
	"rescan": true,
}

// WebsocketHandler handles a new websocket client by creating a new wsClient,
// starting it, and blocking until the connection closes.  Since it blocks, it
// must be run in a separate goroutine.  It should be invoked from the websocket
// server handler which runs each new connection in a new goroutine thereby
// satisfying the requirement.
func (s *rpcServer) WebsocketHandler(conn *websocket.Conn, remoteAddr string,
	authenticated bool) {

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
	client := newWebsocketClient(s, conn, remoteAddr, authenticated)
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
	sync.Mutex

	// server is the RPC server the notification manager is associated with.
	server *rpcServer

	// clients is a map of all currently connected websocket clients.
	clients map[chan bool]*wsClient

	// Maps used to hold lists of websocket clients to be notified on
	// certain events.  Each websocket client also keeps maps for the events
	// which have multiple triggers to make removal from these lists on
	// connection close less horrendously expensive.
	blockNotifications map[chan bool]*wsClient
	txNotifications    map[chan bool]*wsClient
	spentNotifications map[btcwire.OutPoint]map[chan bool]*wsClient
	addrNotifications  map[string]map[chan bool]*wsClient
}

// NumClients returns the number of clients actively being served.
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) NumClients() int {
	m.Lock()
	defer m.Unlock()

	return len(m.clients)
}

// AddBlockUpdateRequest requests block update notifications to the passed
// websocket client.
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) AddBlockUpdateRequest(wsc *wsClient) {
	m.Lock()
	defer m.Unlock()

	// Add the client to the map to notify when block updates are seen.
	// Use the quit channel as a unique id for the client since it is quite
	// a bit more efficient than using the entire struct.
	m.blockNotifications[wsc.quit] = wsc
}

// RemoveBlockUpdateRequest removes block update notifications for the passed
// websocket client.
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) RemoveBlockUpdateRequest(wsc *wsClient) {
	m.Lock()
	defer m.Unlock()

	// Delete the client from the map to notify when block updates are seen.
	// Use the quit channel as a unique id for the client since it is quite
	// a bit more efficient than using the entire struct.
	delete(m.blockNotifications, wsc.quit)
}

// NotifyBlockConnected notifies websocket clients that have registered for
// block updates when a block is connected to the main chain.
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) NotifyBlockConnected(block *btcutil.Block) {
	m.Lock()
	defer m.Unlock()

	// Nothing to do if there are no websocket clients registered to
	// receive notifications that result from a newly connected block.
	if len(m.blockNotifications) == 0 {
		return
	}

	hash, err := block.Sha()
	if err != nil {
		rpcsLog.Error("Bad block; connected block notification dropped")
		return
	}

	// Notify interested websocket clients about the connected block.
	ntfn := btcws.NewBlockConnectedNtfn(hash.String(), int32(block.Height()))
	marshalledJSON, err := json.Marshal(ntfn)
	if err != nil {
		rpcsLog.Error("Failed to marshal block connected notification: "+
			"%v", err)
		return
	}
	for _, wsc := range m.blockNotifications {
		wsc.QueueNotification(marshalledJSON)
	}
}

// NotifyBlockDisconnected notifies websocket clients that have registered for
// block updates when a block is disconnected from the main chain (due to a
// reorganize).
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) NotifyBlockDisconnected(block *btcutil.Block) {
	m.Lock()
	defer m.Unlock()

	// Nothing to do if there are no websocket clients registered to
	// receive notifications that result from a newly connected block.
	if len(m.blockNotifications) == 0 {
		return
	}

	hash, err := block.Sha()
	if err != nil {
		rpcsLog.Error("Bad block; disconnected block notification " +
			"dropped")
		return
	}

	// Notify interested websocket clients about the disconnected block.
	ntfn := btcws.NewBlockDisconnectedNtfn(hash.String(),
		int32(block.Height()))
	marshalledJSON, err := json.Marshal(ntfn)
	if err != nil {
		rpcsLog.Error("Failed to marshal block disconnected "+
			"notification: %v", err)
		return
	}
	for _, wsc := range m.blockNotifications {
		wsc.QueueNotification(marshalledJSON)
	}
}

// AddNewTxRequest requests notifications to the passed websocket client when
// new transactions are added to the memory pool.
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) AddNewTxRequest(wsc *wsClient) {
	m.Lock()
	defer m.Unlock()

	// Add the client to the map to notify when a new transaction is added
	// to the memory pool.  Use the quit channel as a unique id for the
	// client since it is quite a bit more efficient than using the entire
	// struct.
	m.txNotifications[wsc.quit] = wsc
}

// RemoveNewTxRequest removes notifications to the passed websocket client when
// new transaction are added to the memory pool.
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) RemoveNewTxRequest(wsc *wsClient) {
	m.Lock()
	defer m.Unlock()

	// Delete the client from the map to notify when a new transaction is
	// seen in the memory pool.  Use the quit channel as a unique id for the
	// client since it is quite a bit more efficient than using the entire
	// struct.
	delete(m.txNotifications, wsc.quit)
}

// NotifyForNewTx notifies websocket clients that have registerd for updates
// when a new transaction is added to the memory pool.
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) NotifyForNewTx(tx *btcutil.Tx) {
	m.Lock()
	defer m.Unlock()

	// Nothing to do if there are no websocket clients registered to
	// receive notifications about transactions added to the memory pool.
	if len(m.txNotifications) == 0 {
		return
	}

	txID := tx.Sha().String()
	mtx := tx.MsgTx()

	var amount int64
	for _, txOut := range mtx.TxOut {
		amount += txOut.Value
	}

	ntfn := btcws.NewAllTxNtfn(txID, amount)
	marshalledJSON, err := json.Marshal(ntfn)
	if err != nil {
		rpcsLog.Errorf("Failed to marshal tx notification: %s", err.Error())
		return
	}

	var verboseNtfn *btcws.AllVerboseTxNtfn
	var marshalledJSONVerbose []byte
	for _, wsc := range m.txNotifications {
		if wsc.verboseTxUpdates {
			if verboseNtfn == nil {
				rawTx, err := createTxRawResult(m.server.server.btcnet, txID, mtx, nil, 0, nil)
				if err != nil {
					return
				}
				verboseNtfn = btcws.NewAllVerboseTxNtfn(rawTx)
				marshalledJSONVerbose, err = json.Marshal(verboseNtfn)
				if err != nil {
					rpcsLog.Errorf("Failed to marshal verbose tx notification: %s", err.Error())
					return
				}

			}
			wsc.QueueNotification(marshalledJSONVerbose)
		} else {
			wsc.QueueNotification(marshalledJSON)
		}
	}
}

// AddSpentRequest requests an notification when the passed outpoint is
// confirmed spent (contained in a block connected to the main chain) for the
// passed websocket client.  The request is automatically removed once the
// notification has been sent.
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) AddSpentRequest(wsc *wsClient, op *btcwire.OutPoint) {
	m.Lock()
	defer m.Unlock()

	// Track the request in the client as well so it can be quickly be
	// removed on disconnect.
	wsc.spentRequests[*op] = struct{}{}

	// Add the client to the list to notify when the outpoint is seen.
	// Create the list as needed.
	cmap, ok := m.spentNotifications[*op]
	if !ok {
		cmap = make(map[chan bool]*wsClient)
		m.spentNotifications[*op] = cmap
	}
	cmap[wsc.quit] = wsc
}

// removeSpentRequest is the internal function which implements the public
// RemoveSpentRequest.  See the comment for RemoveSpentRequest for more details.
//
// This function MUST be called with the notification manager lock held.
func (m *wsNotificationManager) removeSpentRequest(wsc *wsClient, op *btcwire.OutPoint) {
	// Remove the request tracking from the client.
	delete(wsc.spentRequests, *op)

	// Remove the client from the list to notify.
	notifyMap, ok := m.spentNotifications[*op]
	if !ok {
		rpcsLog.Warnf("Attempt to remove nonexistent spent request "+
			"for websocket client %s", wsc.addr)
		return
	}
	delete(notifyMap, wsc.quit)

	// Remove the map entry altogether if there are no more clients
	// interested in it.
	if len(notifyMap) == 0 {
		delete(m.spentNotifications, *op)
	}
}

// RemoveSpentRequest removes a request from the passed websocket client to be
// notified when the passed outpoint is confirmed spent (contained in a block
// connected to the main chain).
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) RemoveSpentRequest(wsc *wsClient, op *btcwire.OutPoint) {
	m.Lock()
	defer m.Unlock()

	m.removeSpentRequest(wsc, op)
}

// notifyForTxOuts is the internal function which implements the public
// NotifyForTxOuts.  See the comment for NotifyForTxOuts for more details.
//
// This function MUST be called with the notification manager lock held.
func (m *wsNotificationManager) notifyForTxOuts(tx *btcutil.Tx, block *btcutil.Block) {
	// Nothing to do if nobody is listening for address notifications.
	if len(m.addrNotifications) == 0 {
		return
	}

	for i, txout := range tx.MsgTx().TxOut {
		_, addrs, _, err := btcscript.ExtractPkScriptAddrs(
			txout.PkScript, m.server.server.btcnet)
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			encodedAddr := addr.EncodeAddress()
			cmap, ok := m.addrNotifications[encodedAddr]
			if !ok {
				continue
			}

			ntfn := &btcws.ProcessedTxNtfn{
				Receiver:   encodedAddr,
				TxID:       tx.Sha().String(),
				TxOutIndex: uint32(i),
				Amount:     txout.Value,
				PkScript:   hex.EncodeToString(txout.PkScript),
				// TODO(jrick): hardcoding unspent is WRONG and needs
				// to be either calculated from other block txs, or dropped.
				Spent: false,
			}

			if block != nil {
				blkhash, err := block.Sha()
				if err != nil {
					rpcsLog.Error("Error getting block sha; dropping Tx notification")
					break
				}
				ntfn.BlockHeight = int32(block.Height())
				ntfn.BlockHash = blkhash.String()
				ntfn.BlockIndex = tx.Index()
				ntfn.BlockTime = block.MsgBlock().Header.Timestamp.Unix()
			} else {
				ntfn.BlockHeight = -1
				ntfn.BlockIndex = -1
			}

			marshalledJSON, err := json.Marshal(ntfn)
			if err != nil {
				rpcsLog.Errorf("Failed to marshal processedtx notification: %v", err)
			}

			for _, wsc := range cmap {
				wsc.QueueNotification(marshalledJSON)
			}
		}
	}
}

// NotifyForTxOuts examines the outputs of the passed transaction and sends a
// notification to any websocket clients that are interested in an address the
// transaction pays to.
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) NotifyForTxOuts(tx *btcutil.Tx, block *btcutil.Block) {
	m.Lock()
	defer m.Unlock()

	m.notifyForTxOuts(tx, block)
}

// newSpentNotification returns a new marshalled spent notification with the
// passed parameters.
func newSpentNotification(prevOut *btcwire.OutPoint, spender *btcutil.Tx) []byte {
	// Ignore Serialize's error, as writing to a bytes.buffer cannot fail.
	var serializedTx bytes.Buffer
	spender.MsgTx().Serialize(&serializedTx)
	txHex := hex.EncodeToString(serializedTx.Bytes())

	// Create and marsh the notification.
	ntfn := btcws.NewTxSpentNtfn(prevOut.Hash.String(), int(prevOut.Index),
		txHex)
	marshalledJSON, err := json.Marshal(ntfn)
	if err != nil {
		rpcsLog.Errorf("Failed to marshal spent notification: %v", err)
		return nil
	}
	return marshalledJSON
}

// notifySpent examines the inputs of the passed transaction and sends
// interested websocket clients a notification.
//
// This function MUST be called with the notification manager lock held.
func (m *wsNotificationManager) notifySpent(tx *btcutil.Tx) {
	// Nothing to do if nobody is listening for spent notifications.
	if len(m.spentNotifications) == 0 {
		return
	}

	for _, txIn := range tx.MsgTx().TxIn {
		prevOut := &txIn.PreviousOutpoint
		if cmap, ok := m.spentNotifications[*prevOut]; ok {
			marshalledJSON := newSpentNotification(prevOut, tx)
			if marshalledJSON == nil {
				continue
			}
			for _, wsc := range cmap {
				wsc.QueueNotification(marshalledJSON)
				m.removeSpentRequest(wsc, prevOut)
			}
		}
	}
}

// NotifyBlockTXs examines the input and outputs of the passed transaction
// and sends websocket clients notifications they are interested in.
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) NotifyBlockTXs(block *btcutil.Block) {
	m.Lock()
	defer m.Unlock()

	// Nothing to do if there are no websocket clients registered to receive
	// notifications about spent outpoints or payments to addresses.
	if len(m.spentNotifications) == 0 && len(m.addrNotifications) == 0 {
		return
	}

	for _, tx := range block.Transactions() {
		m.notifySpent(tx)
		m.notifyForTxOuts(tx, block)
	}
}

// AddAddrRequest requests notifications to the passed websocket client when
// a transaction pays to the passed address.
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) AddAddrRequest(wsc *wsClient, addr string) {
	m.Lock()
	defer m.Unlock()

	// Track the request in the client as well so it can be quickly be
	// removed on disconnect.
	wsc.addrRequests[addr] = struct{}{}

	// Add the client to the list to notify when the outpoint is seen.
	// Create the list as needed.
	cmap, ok := m.addrNotifications[addr]
	if !ok {
		cmap = make(map[chan bool]*wsClient)
		m.addrNotifications[addr] = cmap
	}
	cmap[wsc.quit] = wsc
}

// removeAddrRequest is the internal function which implements the public
// RemoveAddrRequest.  See the comment for RemoveAddrRequest for more details.
//
// This function MUST be called with the notification manager lock held.
func (m *wsNotificationManager) removeAddrRequest(wsc *wsClient, addr string) {
	// Remove the request tracking from the client.
	delete(wsc.addrRequests, addr)

	// Remove the client from the list to notify.
	notifyMap, ok := m.addrNotifications[addr]
	if !ok {
		rpcsLog.Warnf("Attempt to remove nonexistent addr request "+
			"<%s> for websocket client %s", addr, wsc.addr)
		return
	}
	delete(notifyMap, wsc.quit)

	// Remove the map entry altogether if there are no more clients
	// interested in it.
	if len(notifyMap) == 0 {
		delete(m.addrNotifications, addr)
	}
}

// RemoveAddrRequest removes a request from the passed websocket client to be
// notified when a transaction pays to the passed address.
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) RemoveAddrRequest(wsc *wsClient, addr string) {
	m.Lock()
	defer m.Unlock()

	m.removeAddrRequest(wsc, addr)
}

// AddClient adds the passed websocket client to the notification manager.
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) AddClient(wsc *wsClient) {
	m.Lock()
	defer m.Unlock()

	m.clients[wsc.quit] = wsc
}

// RemoveClient removes the passed websocket client and all notifications
// registered for it.
//
// This function is safe for concurrent access.
func (m *wsNotificationManager) RemoveClient(wsc *wsClient) {
	m.Lock()
	defer m.Unlock()

	// Remove any requests made by the client as well as the client itself.
	delete(m.blockNotifications, wsc.quit)
	delete(m.txNotifications, wsc.quit)
	for k := range wsc.spentRequests {
		op := k
		m.removeSpentRequest(wsc, &op)
	}
	for addr := range wsc.addrRequests {
		m.removeAddrRequest(wsc, addr)
	}
	delete(m.clients, wsc.quit)
}

// Shutdown disconnects all websocket clients the manager knows about.
func (m *wsNotificationManager) Shutdown() {
	for _, wsc := range m.clients {
		wsc.Disconnect()
	}
}

// newWsNotificationManager returns a new notification manager ready for use.
// See wsNotificationManager for more details.
func newWsNotificationManager(server *rpcServer) *wsNotificationManager {
	return &wsNotificationManager{
		server:             server,
		clients:            make(map[chan bool]*wsClient),
		blockNotifications: make(map[chan bool]*wsClient),
		txNotifications:    make(map[chan bool]*wsClient),
		spentNotifications: make(map[btcwire.OutPoint]map[chan bool]*wsClient),
		addrNotifications:  make(map[string]map[chan bool]*wsClient),
	}
}

// wsResponse houses a message to send to the a connected websocket client as
// well as a channel to reply on when the message is sent.
type wsResponse struct {
	msg      []byte
	doneChan chan bool
}

// createMarshalledReply returns a new marshalled btcjson.Reply given the
// passed parameters.  It will automatically convert errors that are not of
// the type *btcjson.Error to the appropriate type as needed.
func createMarshalledReply(id, result interface{}, replyErr error) ([]byte, error) {
	var jsonErr *btcjson.Error
	if replyErr != nil {
		if jErr, ok := replyErr.(*btcjson.Error); !ok {
			jsonErr = &btcjson.Error{
				Code:    btcjson.ErrInternal.Code,
				Message: jErr.Error(),
			}
		}
	}

	response := btcjson.Reply{
		Id:     &id,
		Result: result,
		Error:  jsonErr,
	}

	marshalledJSON, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	return marshalledJSON, nil
}

// wsClient provides an abstraction for handling a websocket client.  The
// overall data flow is split into 3 main goroutines, a possible 4th goroutine
// for long-running operations (only started if request is made), and a
// websocket manager which is used to allow things such as broadcasting
// requested notifications to all connected websocket clients.   Inbound
// messages are read via the inHandler goroutine and generally dispatched to
// their own handler.  However, certain potentially long-running operations such
// as rescans, are sent to the asyncHander goroutine and are limited to one at a
// time.  There are two outbound message types - one for responding to client
// requests and another for async notifications.  Responses to client requests
// use SendMessage which employs a buffered channel thereby limiting the number
// of outstanding requests that can be made.  Notifications are sent via
// QueueNotification which implements a queue via notificationQueueHandler to
// ensure sending notifications from other subsystems can't block.  Ultimately,
// all messages are sent via the outHandler.
type wsClient struct {
	// server is the RPC server that is servicing the client.
	server *rpcServer

	// conn is the underlying websocket connection.
	conn *websocket.Conn

	// addr is the remote address of the client.
	addr string

	// authenticated specifies whether a client has been authenticated
	// and therefore is allowed to communicated over the websocket.
	authenticated bool

	// verboseTxUpdates specifies whether a client has requested verbose
	// information about all new transactions.
	verboseTxUpdates bool

	// addrRequests is a set of addresses the caller has requested to be
	// notified about.  It is maintained here so all requests can be removed
	// when a wallet disconnects.  Owned by the notification manager.
	addrRequests map[string]struct{}

	// spentRequests is a set of unspent Outpoints a wallet has requested
	// notifications for when they are spent by a processed transaction.
	// Owned by the notification manager.
	spentRequests map[btcwire.OutPoint]struct{}

	// Networking infrastructure.
	asyncStarted bool
	asyncChan    chan btcjson.Cmd
	ntfnChan     chan []byte
	sendChan     chan wsResponse
	quit         chan bool
	wg           sync.WaitGroup
}

// handleMessage is the main handler for incoming requests.  It enforces
// authentication, parses the incoming json, looks up and executes handlers
// (including pass through for standard RPC commands), sends the appropriate
// response.  It also detects commands which are marked as long-running and
// sends them off to the asyncHander for processing.
func (c *wsClient) handleMessage(msg string) {
	if !c.authenticated {
		// Disconnect immediately if the provided command fails to
		// parse when the client is not already authenticated.
		cmd, jsonErr := parseCmd([]byte(msg))
		if jsonErr != nil {
			c.Disconnect()
			return
		}

		// Disconnect immediately if the first command is not
		// authenticate when not already authenticated.
		authCmd, ok := cmd.(*btcws.AuthenticateCmd)
		if !ok {
			rpcsLog.Warnf("Unauthenticated websocket message " +
				"received")
			c.Disconnect()
			return
		}

		// Check credentials.
		login := authCmd.Username + ":" + authCmd.Passphrase
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(login))
		authSha := sha256.Sum256([]byte(auth))
		cmp := subtle.ConstantTimeCompare(authSha[:], c.server.authsha[:])
		if cmp != 1 {
			rpcsLog.Warnf("Auth failure.")
			c.Disconnect()
			return
		}
		c.authenticated = true

		// Marshal and send response.
		reply, err := createMarshalledReply(authCmd.Id(), nil, nil)
		if err != nil {
			rpcsLog.Errorf("Failed to marshal authenticate reply: "+
				"%v", err.Error())
			return
		}
		c.SendMessage(reply, nil)
		return
	}

	// Attmpt to parse the raw json into a known btcjson.Cmd.
	cmd, jsonErr := parseCmd([]byte(msg))
	if jsonErr != nil {
		// Use the provided id for errors when a valid JSON-RPC message
		// was parsed.  Requests with no IDs are ignored.
		var id interface{}
		if cmd != nil {
			id = cmd.Id()
			if id == nil {
				return
			}
		}

		// Marshal and send response.
		reply, err := createMarshalledReply(id, nil, jsonErr)
		if err != nil {
			rpcsLog.Errorf("Failed to marshal parse failure "+
				"reply: %v", err)
			return
		}
		c.SendMessage(reply, nil)
		return
	}
	rpcsLog.Debugf("Received command <%s> from %s", cmd.Method(), c.addr)

	// Disconnect if already authenticated and another authenticate command
	// is received.
	if _, ok := cmd.(*btcws.AuthenticateCmd); ok {
		rpcsLog.Warnf("Websocket client %s is already authenticated",
			c.addr)
		c.Disconnect()
		return
	}

	// When the command is marked as a long-running command, send it off
	// to the asyncHander goroutine for processing.
	if _, ok := wsAsyncHandlers[cmd.Method()]; ok {
		// Start up the async goroutine for handling long-running
		// requests asynchonrously if needed.
		if !c.asyncStarted {
			rpcsLog.Tracef("Starting async handler for %s", c.addr)
			c.wg.Add(1)
			go c.asyncHandler()
			c.asyncStarted = true
		}
		c.asyncChan <- cmd
		return
	}

	// Lookup the websocket extension for the command and if it doesn't
	// exist fallback to handling the command as a standard command.
	wsHandler, ok := wsHandlers[cmd.Method()]
	if !ok {
		// No websocket-specific handler so handle like a legacy
		// RPC connection.
		response := standardCmdReply(cmd, c.server)
		reply, err := json.Marshal(response)
		if err != nil {
			rpcsLog.Errorf("Failed to marshal reply for <%s> "+
				"command: %v", cmd.Method(), err)

			return
		}
		c.SendMessage(reply, nil)
		return
	}

	// Invoke the handler and marshal and send response.
	result, jsonErr := wsHandler(c, cmd)
	reply, err := createMarshalledReply(cmd.Id(), result, jsonErr)
	if err != nil {
		rpcsLog.Errorf("Failed to marshal reply for <%s> command: %v",
			cmd.Method(), err)
		return
	}
	c.SendMessage(reply, nil)
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

		var msg string
		if err := websocket.Message.Receive(c.conn, &msg); err != nil {
			// Log the error if it's not due to disconnecting.
			if err != io.EOF {
				rpcsLog.Errorf("Websocket receive error from "+
					"%s: %v", c.addr, err)
			}
			break out
		}
		c.handleMessage(msg)
	}

	// Ensure the connection is closed.
	c.Disconnect()
	c.wg.Done()
	rpcsLog.Tracef("Websocket client input handler done for %s", c.addr)
}

// notificationQueueHandler handles the queueing of outgoing notifications for
// the websocket client.  This runs as a muxer for various sources of input to
// ensure that queueing up notifications to be sent will not block.  Otherwise,
// slow clients could bog down the other systems (such as the mempool or block
// manager) which are queueing the data.  The data is passed on to outHandler to
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
			err := websocket.Message.Send(c.conn, string(r.msg))
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

// asyncHandler handles all long-running requests such as rescans which are
// not run directly in the inHandler routine unlike most requests.  This allows
// normal quick requests to continue to be processed and responded to even while
// lengthy operations are underway.  Only one long-running operation is
// permitted at a time, so multiple long-running requests are queued and
// serialized.  It must be run as a goroutine.  Also, this goroutine is not
// started until/if the first long-running request is made.
func (c *wsClient) asyncHandler() {
	asyncHandlerDoneChan := make(chan bool, 1) // nonblocking sync
	pendingCmds := list.New()
	waiting := false

	// runHandler runs the handler for the passed command and sends the
	// reply.
	runHandler := func(cmd btcjson.Cmd) {
		wsHandler, ok := wsHandlers[cmd.Method()]
		if !ok {
			rpcsLog.Warnf("No handler for command <%s>",
				cmd.Method())
			return
		}

		// Invoke the handler and marshal and send response.
		result, jsonErr := wsHandler(c, cmd)
		reply, err := createMarshalledReply(cmd.Id(), result, jsonErr)
		if err != nil {
			rpcsLog.Errorf("Failed to marshal reply for <%s> "+
				"command: %v", cmd.Method(), err)
			return
		}
		c.SendMessage(reply, nil)
	}

out:
	for {
		select {
		case cmd := <-c.asyncChan:
			if !waiting {
				c.wg.Add(1)
				go func(cmd btcjson.Cmd) {
					runHandler(cmd)
					asyncHandlerDoneChan <- true
					c.wg.Done()
				}(cmd)
			} else {
				pendingCmds.PushBack(cmd)
			}
			waiting = true

		case <-asyncHandlerDoneChan:
			// No longer waiting if there are no more messages in
			// the pending messages queue.
			next := pendingCmds.Front()
			if next == nil {
				waiting = false
				continue
			}

			// Notify the outHandler about the next item to
			// asynchronously send.
			element := pendingCmds.Remove(next)
			c.wg.Add(1)
			go func(cmd btcjson.Cmd) {
				runHandler(cmd)
				asyncHandlerDoneChan <- true
				c.wg.Done()
			}(element.(btcjson.Cmd))

		case <-c.quit:
			break out
		}
	}

	// Drain any wait channels before exiting so nothing is left waiting
	// around to send.
cleanup:
	for {
		select {
		case <-c.asyncChan:
		case <-asyncHandlerDoneChan:
		default:
			break cleanup
		}
	}

	c.wg.Done()
	rpcsLog.Tracef("Websocket client async handler done for %s", c.addr)
}

// SendMessage sends the passed json to the websocket client.  It is backed
// by a buffered channel, so it will not block until the send channel is full.
// Note however that QueueNotification must be used for sending async
// notifications instead of the this function.  This approach allows a limit to
// the number of outstanding requests a client can make without preventing or
// blocking on async notifications.
func (c *wsClient) SendMessage(marshalledJSON []byte, doneChan chan bool) {
	// Don't queue the message if in the process of shutting down.
	select {
	case <-c.quit:
		if doneChan != nil {
			doneChan <- false
		}
		return
	default:
	}

	c.sendChan <- wsResponse{msg: marshalledJSON, doneChan: doneChan}
}

// QueueMessage queues the passed notification to be sent to the websocket
// client.  This function, as the name implies, is only intended for
// notifications since it has additional logic to prevent other subsystems, such
// as the memory pool and block manager, from blocking even when the send
// channel is full.
func (c *wsClient) QueueNotification(marshalledJSON []byte) {
	// Don't queue the message if in the process of shutting down.
	select {
	case <-c.quit:
		return
	default:
	}

	c.ntfnChan <- marshalledJSON
}

// Disconnect disconnects the websocket client.
func (c *wsClient) Disconnect() {
	// Don't try to disconnect again if in the process of shutting down.
	select {
	case <-c.quit:
		return
	default:
	}

	rpcsLog.Tracef("Disconnecting websocket client %s", c.addr)
	close(c.quit)
	c.conn.Close()
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
// incoming and outgoing messages in separate goroutines complete with queueing
// and asynchrous handling for long-running operations.
func newWebsocketClient(server *rpcServer, conn *websocket.Conn,
	remoteAddr string, authenticated bool) *wsClient {

	return &wsClient{
		conn:          conn,
		addr:          remoteAddr,
		authenticated: authenticated,
		server:        server,
		addrRequests:  make(map[string]struct{}),
		spentRequests: make(map[btcwire.OutPoint]struct{}),
		ntfnChan:      make(chan []byte, 1),      // nonblocking sync
		asyncChan:     make(chan btcjson.Cmd, 1), // nonblocking sync
		sendChan:      make(chan wsResponse, websocketSendBufferSize),
		quit:          make(chan bool),
	}
}

// handleGetBestBlock implements the getbestblock command extension
// for websocket connections.
func handleGetBestBlock(wsc *wsClient, icmd btcjson.Cmd) (interface{}, *btcjson.Error) {
	// All other "get block" commands give either the height, the
	// hash, or both but require the block SHA.  This gets both for
	// the best block.
	sha, height, err := wsc.server.server.db.NewestSha()
	if err != nil {
		return nil, &btcjson.ErrBestBlockHash
	}

	// TODO(jrick): need a btcws type for the result.
	result := map[string]interface{}{
		"hash":   sha.String(),
		"height": height,
	}
	return result, nil
}

// handleGetCurrentNet implements the getcurrentnet command extension
// for websocket connections.
func handleGetCurrentNet(wsc *wsClient, icmd btcjson.Cmd) (interface{}, *btcjson.Error) {
	return wsc.server.server.btcnet, nil
}

// handleNotifyBlocks implements the notifyblocks command extension for
// websocket connections.
func handleNotifyBlocks(wsc *wsClient, icmd btcjson.Cmd) (interface{}, *btcjson.Error) {
	wsc.server.ntfnMgr.AddBlockUpdateRequest(wsc)
	return nil, nil
}

// handleNotifySpent implements the notifyspent command extension for
// websocket connections.
func handleNotifySpent(wsc *wsClient, icmd btcjson.Cmd) (interface{}, *btcjson.Error) {
	cmd, ok := icmd.(*btcws.NotifySpentCmd)
	if !ok {
		return nil, &btcjson.ErrInternal
	}

	wsc.server.ntfnMgr.AddSpentRequest(wsc, cmd.OutPoint)
	return nil, nil
}

// handleNotifyAllNewTXs implements the notifyallnewtxs command extension for
// websocket connections.
func handleNotifyAllNewTXs(wsc *wsClient, icmd btcjson.Cmd) (interface{}, *btcjson.Error) {
	cmd, ok := icmd.(*btcws.NotifyAllNewTXsCmd)
	if !ok {
		return nil, &btcjson.ErrInternal
	}

	wsc.verboseTxUpdates = cmd.Verbose
	wsc.server.ntfnMgr.AddNewTxRequest(wsc)
	return nil, nil
}

// handleNotifyNewTXs implements the notifynewtxs command extension for
// websocket connections.
func handleNotifyNewTXs(wsc *wsClient, icmd btcjson.Cmd) (interface{}, *btcjson.Error) {
	cmd, ok := icmd.(*btcws.NotifyNewTXsCmd)
	if !ok {
		return nil, &btcjson.ErrInternal
	}

	for _, addrStr := range cmd.Addresses {
		addr, err := btcutil.DecodeAddr(addrStr)
		if err != nil {
			e := btcjson.Error{
				Code:    btcjson.ErrInvalidAddressOrKey.Code,
				Message: fmt.Sprintf("Invalid address or key: %v", addrStr),
			}
			return nil, &e
		}

		wsc.server.ntfnMgr.AddAddrRequest(wsc, addr.EncodeAddress())
	}

	return nil, nil
}

// rescanBlock rescans all transactions in a single block.  This is a helper
// function for handleRescan.
func rescanBlock(wsc *wsClient, cmd *btcws.RescanCmd, blk *btcutil.Block) {
	db := wsc.server.server.db

	for _, tx := range blk.Transactions() {
		var txReply *btcdb.TxListReply
	txouts:
		for txOutIdx, txout := range tx.MsgTx().TxOut {
			_, addrs, _, err := btcscript.ExtractPkScriptAddrs(
				txout.PkScript, wsc.server.server.btcnet)
			if err != nil {
				continue txouts
			}

			for _, addr := range addrs {
				encodedAddr := addr.EncodeAddress()
				if _, ok := cmd.Addresses[encodedAddr]; !ok {
					continue
				}
				// TODO(jrick): This lookup is expensive and can be avoided
				// if the wallet is sent the previous outpoints for all inputs
				// of the tx, so any can removed from the utxo set (since
				// they are, as of this tx, now spent).
				if txReply == nil {
					txReplyList, err := db.FetchTxBySha(tx.Sha())
					if err != nil {
						rpcsLog.Errorf("Tx Sha %v not found by db", tx.Sha())
						continue txouts
					}
					for i := range txReplyList {
						if txReplyList[i].Height == blk.Height() {
							txReply = txReplyList[i]
							break
						}
					}

				}

				// Sha never errors.
				blksha, _ := blk.Sha()

				ntfn := &btcws.ProcessedTxNtfn{
					Receiver:    encodedAddr,
					Amount:      txout.Value,
					TxID:        tx.Sha().String(),
					TxOutIndex:  uint32(txOutIdx),
					PkScript:    hex.EncodeToString(txout.PkScript),
					BlockHash:   blksha.String(),
					BlockHeight: int32(blk.Height()),
					BlockIndex:  tx.Index(),
					BlockTime:   blk.MsgBlock().Header.Timestamp.Unix(),
					Spent:       txReply.TxSpent[txOutIdx],
				}
				marshalledJSON, err := json.Marshal(ntfn)
				if err != nil {
					rpcsLog.Errorf("Failed to marshal processedtx notification: %v", err)
					return
				}

				// Stop the rescan early if the websocket client
				// disconnected.
				select {
				case <-wsc.quit:
					return

				default:
					wsc.SendMessage(marshalledJSON, nil)
				}
			}
		}
	}
}

// handleRescan implements the rescan command extension for websocket
// connections.
func handleRescan(wsc *wsClient, icmd btcjson.Cmd) (interface{}, *btcjson.Error) {
	cmd, ok := icmd.(*btcws.RescanCmd)
	if !ok {
		return nil, &btcjson.ErrInternal
	}

	numAddrs := len(cmd.Addresses)
	if numAddrs == 1 {
		rpcsLog.Info("Beginning rescan for 1 address")
	} else {
		rpcsLog.Infof("Beginning rescan for %d addresses", numAddrs)
	}

	minBlock := int64(cmd.BeginBlock)
	maxBlock := int64(cmd.EndBlock)

	// FetchHeightRange may not return a complete list of block shas for
	// the given range, so fetch range as many times as necessary.
	db := wsc.server.server.db
	for {
		hashList, err := db.FetchHeightRange(minBlock, maxBlock)
		if err != nil {
			rpcsLog.Errorf("Error looking up block range: %v", err)
			return nil, &btcjson.ErrDatabase
		}
		if len(hashList) == 0 {
			break
		}

		for i := range hashList {
			blk, err := db.FetchBlockBySha(&hashList[i])
			if err != nil {
				rpcsLog.Errorf("Error looking up block sha: %v", err)
				return nil, &btcjson.ErrDatabase
			}

			// A select statement is used to stop rescans if the
			// client requesting the rescan has disconnected.
			select {
			case <-wsc.quit:
				rpcsLog.Debugf("Stopped rescan at height %v for disconnected client",
					blk.Height())
				return nil, nil
			default:
				rescanBlock(wsc, cmd, blk)
			}
		}

		if maxBlock-minBlock > int64(len(hashList)) {
			minBlock += int64(len(hashList))
		} else {
			break
		}
	}

	rpcsLog.Info("Finished rescan")
	return nil, nil
}
