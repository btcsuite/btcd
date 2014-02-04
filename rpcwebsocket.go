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
	"errors"
	"fmt"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/conformal/btcws"
	"sync"
	"time"
)

var timeZeroVal time.Time

// ErrBadAuth describes an error that was the result of invalid credentials.
var ErrBadAuth = errors.New("invalid credentials")

type ntfnChan chan btcjson.Cmd

type handlerChans struct {
	n            ntfnChan        // channel to send notifications
	disconnected <-chan struct{} // closed when a client has disconnected.
}

type wsCommandHandler func(*rpcServer, btcjson.Cmd, handlerChans) (interface{}, *btcjson.Error)

// wsHandlers maps RPC command strings to appropriate websocket handler
// functions.
var wsHandlers = map[string]wsCommandHandler{
	"getcurrentnet":      handleGetCurrentNet,
	"getbestblock":       handleGetBestBlock,
	"notifyblocks":       handleNotifyBlocks,
	"notifynewtxs":       handleNotifyNewTXs,
	"notifyspent":        handleNotifySpent,
	"rescan":             handleRescan,
	"sendrawtransaction": handleWalletSendRawTransaction,
}

// wsContext holds the items the RPC server needs to handle websocket
// connections for wallets.
type wsContext struct {
	sync.RWMutex

	// connections holds a map of requests for each wallet using the
	// wallet channel as the key.
	connections map[ntfnChan]*requestContexts

	// Map of address hash to list of notificationCtx. This is the global
	// list we actually use for notifications, we also keep a list in the
	// requestContexts to make removal from this list on connection close
	// less horrendously expensive.
	txNotifications map[string]*list.List

	// Map of outpoint to list of notificationCtx.
	spentNotifications map[btcwire.OutPoint]*list.List

	// Map of shas to list of notificationCtx.
	minedTxNotifications map[btcwire.ShaHash]*list.List
}

// AddBlockUpdateRequest adds the request context to mark a wallet as
// having requested updates for connected and disconnected blocks.
func (r *wsContext) AddBlockUpdateRequest(n ntfnChan) {
	r.Lock()
	defer r.Unlock()

	rc := r.connections[n]
	rc.blockUpdates = true
}

// AddTxRequest adds the request context for new transaction notifications.
func (r *wsContext) AddTxRequest(n ntfnChan, addr string) {
	r.Lock()
	defer r.Unlock()

	clist, ok := r.txNotifications[addr]
	if !ok {
		clist = list.New()
		r.txNotifications[addr] = clist
	}

	clist.PushBack(n)

	rc := r.connections[n]
	rc.txRequests[addr] = struct{}{}
}

func (r *wsContext) removeGlobalTxRequest(n ntfnChan, addr string) {
	clist := r.txNotifications[addr]
	var enext *list.Element
	for e := clist.Front(); e != nil; e = enext {
		enext = e.Next()
		c := e.Value.(ntfnChan)
		if c == n {
			clist.Remove(e)
			break
		}
	}

	if clist.Len() == 0 {
		delete(r.txNotifications, addr)
	}
}

// AddSpentRequest adds a request context for notifications of a spent
// Outpoint.
func (r *wsContext) AddSpentRequest(n ntfnChan, op *btcwire.OutPoint) {
	r.Lock()
	defer r.Unlock()

	clist, ok := r.spentNotifications[*op]
	if !ok {
		clist = list.New()
		r.spentNotifications[*op] = clist
	}
	clist.PushBack(n)

	rc := r.connections[n]
	rc.spentRequests[*op] = struct{}{}
}

func (r *wsContext) removeGlobalSpentRequest(n ntfnChan, op *btcwire.OutPoint) {
	clist := r.spentNotifications[*op]
	var enext *list.Element
	for e := clist.Front(); e != nil; e = enext {
		enext = e.Next()
		c := e.Value.(ntfnChan)
		if c == n {
			clist.Remove(e)
			break
		}
	}

	if clist.Len() == 0 {
		delete(r.spentNotifications, *op)
	}
}

// RemoveSpentRequest removes a request context for notifications of a
// spent Outpoint.
func (r *wsContext) RemoveSpentRequest(n ntfnChan, op *btcwire.OutPoint) {
	r.Lock()
	defer r.Unlock()

	r.removeGlobalSpentRequest(n, op)
	rc := r.connections[n]
	delete(rc.spentRequests, *op)
}

// AddMinedTxRequest adds request contexts for notifications of a
// mined transaction.
func (r *wsContext) AddMinedTxRequest(n ntfnChan, txID *btcwire.ShaHash) {
	r.Lock()
	defer r.Unlock()

	clist, ok := r.minedTxNotifications[*txID]
	if !ok {
		clist = list.New()
		r.minedTxNotifications[*txID] = clist
	}
	clist.PushBack(n)

	rc := r.connections[n]
	rc.minedTxRequests[*txID] = struct{}{}
}

func (r *wsContext) removeGlobalMinedTxRequest(n ntfnChan, txID *btcwire.ShaHash) {
	clist := r.minedTxNotifications[*txID]
	var enext *list.Element
	for e := clist.Front(); e != nil; e = enext {
		enext = e.Next()
		c := e.Value.(ntfnChan)
		if c == n {
			clist.Remove(e)
			break
		}
	}

	if clist.Len() == 0 {
		delete(r.minedTxNotifications, *txID)
	}
}

// RemoveMinedTxRequest removes request contexts for notifications of a
// mined transaction.
func (r *wsContext) RemoveMinedTxRequest(n ntfnChan, txID *btcwire.ShaHash) {
	r.Lock()
	defer r.Unlock()

	r.removeMinedTxRequest(n, txID)
}

// removeMinedTxRequest removes request contexts for notifications of a
// mined transaction without grabbing any locks.
func (r *wsContext) removeMinedTxRequest(n ntfnChan, txID *btcwire.ShaHash) {
	r.removeGlobalMinedTxRequest(n, txID)
	rc := r.connections[n]
	delete(rc.minedTxRequests, *txID)
}

// CloseListeners removes all request contexts for notifications sent
// to a wallet notification channel and closes the channel to stop all
// goroutines currently serving that wallet.
func (r *wsContext) CloseListeners(n ntfnChan) {
	r.Lock()
	defer r.Unlock()

	delete(r.connections, n)
	close(n)
}

// newWebsocketContext returns a new websocket context that is used
// for handling websocket requests and notifications.
func newWebsocketContext() *wsContext {
	return &wsContext{
		connections:          make(map[ntfnChan]*requestContexts),
		txNotifications:      make(map[string]*list.List),
		spentNotifications:   make(map[btcwire.OutPoint]*list.List),
		minedTxNotifications: make(map[btcwire.ShaHash]*list.List),
	}
}

// requestContexts holds all requests for a single wallet connection.
type requestContexts struct {
	// disconnecting indicates the websocket is in the process of
	// disconnecting.  This is used to prevent trying to handle any more
	// commands in the interim.
	disconnecting bool

	// authenticated specifies whether a client has been authenticated
	// and therefore is allowed to communicated over the websocket.
	authenticated bool

	// blockUpdates specifies whether a client has requested notifications
	// for whenever blocks are connected or disconnected from the main
	// chain.
	blockUpdates bool

	// txRequests is a set of addresses a wallet has requested transactions
	// updates for.  It is maintained here so all requests can be removed
	// when a wallet disconnects.
	txRequests map[string]struct{}

	// spentRequests is a set of unspent Outpoints a wallet has requested
	// notifications for when they are spent by a processed transaction.
	spentRequests map[btcwire.OutPoint]struct{}

	// minedTxRequests holds a set of transaction IDs (tx hashes) of
	// transactions created by a wallet. A wallet may request
	// notifications of when a tx it created is mined into a block and
	// removed from the mempool.  Once a tx has been mined into a
	// block, wallet may remove the raw transaction from its unmined tx
	// pool.
	minedTxRequests map[btcwire.ShaHash]struct{}
}

// respondToAnyCmd checks that a parsed command is a standard or
// extension JSON-RPC command and runs the proper handler to reply to
// the command.  Any and all responses are sent to the wallet from
// this function.
func respondToAnyCmd(cmd btcjson.Cmd, s *rpcServer, c handlerChans) *btcjson.Reply {
	// Lookup the websocket extension for the command and if it doesn't
	// exist fallback to handling the command as a standard command.
	wsHandler, ok := wsHandlers[cmd.Method()]
	if !ok {
		// No websocket-specific handler so handle like a legacy
		// RPC connection.
		response := standardCmdReply(cmd, s)
		return &response
	}
	result, jsonErr := wsHandler(s, cmd, c)
	id := cmd.Id()
	response := btcjson.Reply{
		Id:     &id,
		Result: result,
		Error:  jsonErr,
	}
	return &response
}

// handleGetCurrentNet implements the getcurrentnet command extension
// for websocket connections.
func handleGetCurrentNet(s *rpcServer, icmd btcjson.Cmd, c handlerChans) (interface{}, *btcjson.Error) {
	if cfg.TestNet3 {
		return btcwire.TestNet3, nil
	}
	return btcwire.MainNet, nil
}

// handleGetBestBlock implements the getbestblock command extension
// for websocket connections.
func handleGetBestBlock(s *rpcServer, icmd btcjson.Cmd, c handlerChans) (interface{}, *btcjson.Error) {
	// All other "get block" commands give either the height, the
	// hash, or both but require the block SHA.  This gets both for
	// the best block.
	sha, height, err := s.server.db.NewestSha()
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

// handleNotifyBlocks implements the notifyblocks command extension for
// websocket connections.
func handleNotifyBlocks(s *rpcServer, icmd btcjson.Cmd, c handlerChans) (interface{}, *btcjson.Error) {
	s.ws.AddBlockUpdateRequest(c.n)
	return nil, nil
}

// handleNotifyNewTXs implements the notifynewtxs command extension for
// websocket connections.
func handleNotifyNewTXs(s *rpcServer, icmd btcjson.Cmd, c handlerChans) (interface{}, *btcjson.Error) {
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

		// TODO(jrick) Notifing for non-P2PKH addresses is currently
		// unsuported.
		if _, ok := addr.(*btcutil.AddressPubKeyHash); !ok {
			e := btcjson.Error{
				Code:    btcjson.ErrInvalidAddressOrKey.Code,
				Message: fmt.Sprintf("Invalid address or key: %v", addr.EncodeAddress()),
			}
			return nil, &e
		}

		s.ws.AddTxRequest(c.n, addr.EncodeAddress())
	}

	return nil, nil
}

// handleNotifySpent implements the notifyspent command extension for
// websocket connections.
func handleNotifySpent(s *rpcServer, icmd btcjson.Cmd, c handlerChans) (interface{}, *btcjson.Error) {
	cmd, ok := icmd.(*btcws.NotifySpentCmd)
	if !ok {
		return nil, &btcjson.ErrInternal
	}

	s.ws.AddSpentRequest(c.n, cmd.OutPoint)

	return nil, nil
}

// handleRescan implements the rescan command extension for websocket
// connections.
func handleRescan(s *rpcServer, icmd btcjson.Cmd, c handlerChans) (interface{}, *btcjson.Error) {
	cmd, ok := icmd.(*btcws.RescanCmd)
	if !ok {
		return nil, &btcjson.ErrInternal
	}

	if len(cmd.Addresses) == 1 {
		rpcsLog.Info("Beginning rescan for 1 address")
	} else {
		rpcsLog.Infof("Beginning rescan for %v addresses",
			len(cmd.Addresses))
	}

	minblock := int64(cmd.BeginBlock)
	maxblock := int64(cmd.EndBlock)

	// FetchHeightRange may not return a complete list of block shas for
	// the given range, so fetch range as many times as necessary.
	for {
		blkshalist, err := s.server.db.FetchHeightRange(minblock, maxblock)
		if err != nil {
			rpcsLog.Errorf("Error looking up block range: %v", err)
			return nil, &btcjson.ErrDatabase
		}
		if len(blkshalist) == 0 {
			break
		}

		for i := range blkshalist {
			blk, err := s.server.db.FetchBlockBySha(&blkshalist[i])
			if err != nil {
				rpcsLog.Errorf("Error looking up block sha: %v", err)
				return nil, &btcjson.ErrDatabase
			}

			// A select statement is used to stop rescans if the
			// client requesting the rescan has disconnected.
			select {
			case <-c.disconnected:
				rpcsLog.Debugf("Stopping rescan at height %v for disconnected client",
					blk.Height())
				return nil, nil

			default:
				rescanBlock(s, cmd, c, blk)
			}
		}

		if maxblock-minblock > int64(len(blkshalist)) {
			minblock += int64(len(blkshalist))
		} else {
			break
		}
	}

	rpcsLog.Info("Finished rescan")
	return nil, nil
}

// rescanBlock rescans all transactions in a single block.  This is a
// helper function for handleRescan.
func rescanBlock(s *rpcServer, cmd *btcws.RescanCmd, c handlerChans, blk *btcutil.Block) {
	for _, tx := range blk.Transactions() {
		var txReply *btcdb.TxListReply
	txouts:
		for txOutIdx, txout := range tx.MsgTx().TxOut {
			_, addrs, _, err := btcscript.ExtractPkScriptAddrs(
				txout.PkScript, s.server.btcnet)
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
					txReplyList, err := s.server.db.FetchTxBySha(tx.Sha())
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

				select {
				case <-c.disconnected:
					return

				default:
					c.n <- ntfn
				}
			}
		}
	}
}

// handleWalletSendRawTransaction implements the websocket extended version of
// the sendrawtransaction command.
func handleWalletSendRawTransaction(s *rpcServer, icmd btcjson.Cmd, c handlerChans) (interface{}, *btcjson.Error) {
	result, err := handleSendRawTransaction(s, icmd)
	// TODO: the standard handlers really should be changed to
	// return btcjson.Errors which get used directly in the
	// response.  Wouldn't need this crap here then.
	if err != nil {
		if jsonErr, ok := err.(*btcjson.Error); ok {
			return result, jsonErr
		}
		jsonErr := &btcjson.Error{
			Code:    btcjson.ErrMisc.Code,
			Message: err.Error(),
		}
		return result, jsonErr
	}

	// The result is already guaranteed to be a valid hash string if no
	// error was returned above, so it's safe to ignore the error here.
	txSha, _ := btcwire.NewShaHashFromStr(result.(string))

	// Request to be notified when the transaction is mined.
	s.ws.AddMinedTxRequest(c.n, txSha)
	return result, nil
}

// websocketAuthenticate checks the authenticate command for valid credentials.
// An error is returned if the credentials are invalid or if the connection is
// already authenticated.
//
// This function MUST be called with the websocket lock held.
func websocketAuthenticate(icmd btcjson.Cmd, rc *requestContexts, authSha []byte) error {
	cmd, ok := icmd.(*btcws.AuthenticateCmd)
	if !ok {
		return fmt.Errorf("%s", btcjson.ErrInternal.Message)
	}

	// Already authenticated?
	if rc.authenticated {
		rpcsLog.Warnf("Already authenticated")
		return ErrBadAuth
	}

	// Check credentials.
	login := cmd.Username + ":" + cmd.Passphrase
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(login))
	calcualtedAuthSha := sha256.Sum256([]byte(auth))
	cmp := subtle.ConstantTimeCompare(calcualtedAuthSha[:], authSha)
	if cmp != 1 {
		rpcsLog.Warnf("Auth failure.")
		return ErrBadAuth
	}

	rc.authenticated = true
	return nil
}

// AddWalletListener adds a channel to listen for new messages from a
// wallet.
func (s *rpcServer) AddWalletListener(n ntfnChan, rc *requestContexts) {
	s.ws.Lock()
	s.ws.connections[n] = rc
	s.ws.Unlock()
}

// RemoveWalletListener removes a wallet listener channel.
func (s *rpcServer) RemoveWalletListener(n ntfnChan) {
	s.ws.Lock()

	rc := s.ws.connections[n]
	for k := range rc.txRequests {
		s.ws.removeGlobalTxRequest(n, k)
	}
	for k := range rc.spentRequests {
		s.ws.removeGlobalSpentRequest(n, &k)
	}
	for k := range rc.minedTxRequests {
		s.ws.removeGlobalMinedTxRequest(n, &k)
	}

	delete(s.ws.connections, n)
	s.ws.Unlock()
}

// walletReqsNotifications is the handler function for websocket
// connections from a btcwallet instance.  It reads messages from wallet and
// sends back replies, as well as notififying wallets of chain updates.
func (s *rpcServer) walletReqsNotifications(ws *websocket.Conn, authenticated bool) {
	// Clear the read deadline that was set before the websocket hijacked
	// the connection.
	ws.SetReadDeadline(timeZeroVal)

	// Add wallet notification channel so this handler receives btcd chain
	// notifications.
	n := make(ntfnChan)
	rc := &requestContexts{
		authenticated:   authenticated,
		txRequests:      make(map[string]struct{}),
		spentRequests:   make(map[btcwire.OutPoint]struct{}),
		minedTxRequests: make(map[btcwire.ShaHash]struct{}),
	}
	s.AddWalletListener(n, rc)
	defer s.RemoveWalletListener(n)

	// Channel for responses.
	r := make(chan *btcjson.Reply)

	// Channels for websocket handlers.
	disconnected := make(chan struct{})
	hc := handlerChans{
		n:            n,
		disconnected: disconnected,
	}

	// msgs is a channel for all messages received over the websocket.
	msgs := make(chan string)

	// Receive messages from websocket and send across reqs until the
	// connection is lost.
	go func() {
		for {
			select {
			case <-s.quit:
				return

			default:
				var m string
				if err := websocket.Message.Receive(ws, &m); err != nil {
					// Only close disconnected if not closed yet.
					select {
					case <-disconnected:
						// nothing

					default:
						close(disconnected)
					}
					return
				}
				msgs <- m
			}
		}
	}()

	for {
		select {
		case <-s.quit:
			// Server closed.  Closing disconnected signals handlers to stop
			// and flushes all channels handlers may write to.
			select {
			case <-disconnected:
				// nothing

			default:
				close(disconnected)
			}

		case <-disconnected:
			for {
				select {
				case <-msgs:
				case <-r:
				case <-n:
				default:
					return
				}
			}

		case m := <-msgs:
			// This function internally spawns a new goroutine to
			// the handle request after validating authentication.
			// Responses and notifications are read by channels in
			// this for-select loop.
			if !rc.disconnecting {
				err := s.websocketJSONHandler(r, hc, m)
				if err == ErrBadAuth {
					rc.disconnecting = true
					close(disconnected)
					ws.Close()
				}
			}

		case response := <-r:
			// Marshal and send response.
			mresp, err := json.Marshal(response)
			if err != nil {
				rpcsLog.Errorf("Error unmarshaling response: %v", err)
				continue
			}
			if err := websocket.Message.Send(ws, string(mresp)); err != nil {
				// Only close disconnected if not closed yet.
				select {
				case <-disconnected:
					// nothing

				default:
					close(disconnected)
				}
				return
			}

		case ntfn := <-n:
			// Marshal and send notification.
			mntfn, err := ntfn.MarshalJSON()
			if err != nil {
				rpcsLog.Errorf("Error unmarshaling notification: %v", err)
				continue
			}
			if err := websocket.Message.Send(ws, string(mntfn)); err != nil {
				// Only close disconnected if not closed yet.
				select {
				case <-disconnected:
					// nothing

				default:
					close(disconnected)
				}
				return
			}
		}
	}
}

// websocketJSONHandler parses and handles a marshalled json message,
// sending the marshalled reply to a wallet notification channel.
func (s *rpcServer) websocketJSONHandler(r chan *btcjson.Reply, c handlerChans, msg string) error {
	var resp *btcjson.Reply

	cmd, jsonErr := parseCmd([]byte(msg))
	if jsonErr != nil {
		resp = &btcjson.Reply{}
		if cmd != nil {
			// Unmarshaling at least a valid JSON-RPC message succeeded.
			// Use the provided id for errors.  Requests with no IDs
			// should be ignored.
			id := cmd.Id()
			if id == nil {
				return nil
			}
			resp.Id = &id
		}
		resp.Error = jsonErr
	} else {
		// The first command must be the "authenticate" command if the
		// connection is not already authenticated.
		s.ws.Lock()
		rc := s.ws.connections[c.n]
		if _, ok := cmd.(*btcws.AuthenticateCmd); ok {
			// Validate the provided credentials.
			err := websocketAuthenticate(cmd, rc, s.authsha[:])
			if err != nil {
				s.ws.Unlock()
				return err
			}

			// Generate an empty response to send for the successful
			// authentication.
			id := cmd.Id()
			resp = &btcjson.Reply{
				Id:     &id,
				Result: nil,
				Error:  nil,
			}
		} else if !rc.authenticated {
			rpcsLog.Warnf("Unauthenticated websocket message " +
				"received")
			s.ws.Unlock()
			return ErrBadAuth
		}

		s.ws.Unlock()
	}

	// Find and run handler in new goroutine.
	go func() {
		if resp == nil {
			resp = respondToAnyCmd(cmd, s, c)
		}
		select {
		case <-c.disconnected:
			return

		default:
			r <- resp
		}
	}()

	return nil
}

// NotifyBlockConnected creates and marshalls a JSON message to notify
// of a new block connected to the main chain.  The notification is sent
// to each connected wallet.
func (s *rpcServer) NotifyBlockConnected(block *btcutil.Block) {
	hash, err := block.Sha()
	if err != nil {
		rpcsLog.Error("Bad block; connected block notification dropped")
		return
	}

	// TODO: remove int32 type conversion.
	ntfn := btcws.NewBlockConnectedNtfn(hash.String(), int32(block.Height()))
	for ntfnChan, rc := range s.ws.connections {
		if rc.blockUpdates {
			ntfnChan <- ntfn
		}
	}

	// Inform any interested parties about txs mined in this block.
	s.ws.Lock()
	for _, tx := range block.Transactions() {
		if clist, ok := s.ws.minedTxNotifications[*tx.Sha()]; ok {
			var enext *list.Element
			for e := clist.Front(); e != nil; e = enext {
				enext = e.Next()
				n := e.Value.(ntfnChan)
				// TODO: remove int32 type conversion after
				// the int64 -> int32 switch is made.
				ntfn := btcws.NewTxMinedNtfn(tx.Sha().String(),
					hash.String(), int32(block.Height()),
					block.MsgBlock().Header.Timestamp.Unix(),
					tx.Index())
				n <- ntfn
				s.ws.removeMinedTxRequest(n, tx.Sha())
			}
		}
	}
	s.ws.Unlock()
}

// NotifyBlockDisconnected creates and marshals a JSON message to notify
// of a new block disconnected from the main chain.  The notification is sent
// to each connected wallet.
func (s *rpcServer) NotifyBlockDisconnected(block *btcutil.Block) {
	hash, err := block.Sha()
	if err != nil {
		rpcsLog.Error("Bad block; connected block notification dropped")
		return
	}

	// TODO: remove int32 type conversion.
	ntfn := btcws.NewBlockDisconnectedNtfn(hash.String(),
		int32(block.Height()))
	for ntfnChan, rc := range s.ws.connections {
		if rc.blockUpdates {
			ntfnChan <- ntfn
		}
	}
}

// NotifyBlockTXs creates and marshals a JSON message to notify wallets
// of new transactions (with both spent and unspent outputs) for a watched
// address.
func (s *rpcServer) NotifyBlockTXs(db btcdb.Db, block *btcutil.Block) {
	for _, tx := range block.Transactions() {
		s.newBlockNotifyCheckTxIn(tx)
		s.NotifyForTxOuts(tx, block)
	}
}

func notifySpentData(n ntfnChan, txhash *btcwire.ShaHash, index uint32,
	spender *btcutil.Tx) {

	var buf bytes.Buffer
	// Ignore Serialize's error, as writing to a bytes.buffer
	// cannot fail.
	spender.MsgTx().Serialize(&buf)
	txStr := hex.EncodeToString(buf.Bytes())

	ntfn := btcws.NewTxSpentNtfn(txhash.String(), int(index), txStr)
	n <- ntfn
}

// newBlockNotifyCheckTxIn is a helper function to iterate through
// each transaction input of a new block and perform any checks and
// notify listening frontends when necessary.
func (s *rpcServer) newBlockNotifyCheckTxIn(tx *btcutil.Tx) {
	for _, txin := range tx.MsgTx().TxIn {
		if clist, ok := s.ws.spentNotifications[txin.PreviousOutpoint]; ok {
			var enext *list.Element
			for e := clist.Front(); e != nil; e = enext {
				enext = e.Next()
				n := e.Value.(ntfnChan)
				notifySpentData(n, &txin.PreviousOutpoint.Hash,
					txin.PreviousOutpoint.Index, tx)
				s.ws.RemoveSpentRequest(n, &txin.PreviousOutpoint)
			}
		}
	}
}

// NotifyForTxOuts iterates through all outputs of a tx, performing any
// necessary notifications for wallets.  If a non-nil block is passed,
// additional block information is passed with the notifications.
func (s *rpcServer) NotifyForTxOuts(tx *btcutil.Tx, block *btcutil.Block) {
	// Nothing to do if nobody is listening for transaction notifications.
	if len(s.ws.txNotifications) == 0 {
		return
	}

	for i, txout := range tx.MsgTx().TxOut {
		_, addrs, _, err := btcscript.ExtractPkScriptAddrs(
			txout.PkScript, s.server.btcnet)
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			// Only support pay-to-pubkey-hash right now.
			if _, ok := addr.(*btcutil.AddressPubKeyHash); !ok {
				continue
			}

			encodedAddr := addr.EncodeAddress()
			if idlist, ok := s.ws.txNotifications[encodedAddr]; ok {
				for e := idlist.Front(); e != nil; e = e.Next() {
					n := e.Value.(ntfnChan)

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

					n <- ntfn
				}
			}
		}
	}
}
