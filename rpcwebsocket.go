// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"code.google.com/p/go.net/websocket"
	"container/list"
	"encoding/json"
	"fmt"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/conformal/btcws"
	"sync"
)

type wsCommandHandler func(*rpcServer, btcjson.Cmd, chan []byte, *requestContexts) error

// wsHandlers maps RPC command strings to appropriate websocket handler
// functions.
var wsHandlers = map[string]wsCommandHandler{
	"getcurrentnet":       handleGetCurrentNet,
	"getbestblock":        handleGetBestBlock,
	"notifynewtxs":        handleNotifyNewTXs,
	"notifyspent":         handleNotifySpent,
	"rescan":              handleRescan,
	"sendrawtransaction:": handleWalletSendRawTransaction,
}

// wsContext holds the items the RPC server needs to handle websocket
// connections for wallets.
type wsContext struct {
	sync.RWMutex

	// connections holds a map of each currently connected wallet
	// listener as the key.
	connections map[chan []byte]*requestContexts

	// Any chain notifications meant to be received by every connected
	// wallet are sent across this channel.
	walletNotificationMaster chan []byte

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

type notificationCtx struct {
	id         interface{}
	connection chan []byte
	rc         *requestContexts
}

// AddTxRequest adds the request context for new transaction notifications.
func (r *wsContext) AddTxRequest(walletNotification chan []byte, rc *requestContexts, addr string, id interface{}) {
	r.Lock()
	defer r.Unlock()

	nc := &notificationCtx{
		id:         id,
		connection: walletNotification,
		rc:         rc,
	}

	clist, ok := r.txNotifications[addr]
	if !ok {
		clist = list.New()
		r.txNotifications[addr] = clist
	}

	clist.PushBack(nc)

	rc.txRequests[addr] = id
}

func (r *wsContext) removeGlobalTxRequest(walletNotification chan []byte, addr string) {
	clist := r.txNotifications[addr]
	var enext *list.Element
	for e := clist.Front(); e != nil; e = enext {
		enext = e.Next()
		ctx := e.Value.(*notificationCtx)
		if ctx.connection == walletNotification {
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
func (r *wsContext) AddSpentRequest(walletNotification chan []byte, rc *requestContexts, op *btcwire.OutPoint, id interface{}) {
	r.Lock()
	defer r.Unlock()

	nc := &notificationCtx{
		id:         id,
		connection: walletNotification,
		rc:         rc,
	}
	clist, ok := r.spentNotifications[*op]
	if !ok {
		clist = list.New()
		r.spentNotifications[*op] = clist
	}
	clist.PushBack(nc)
	rc.spentRequests[*op] = id
}

func (r *wsContext) removeGlobalSpentRequest(walletNotification chan []byte, op *btcwire.OutPoint) {
	clist := r.spentNotifications[*op]
	var enext *list.Element
	for e := clist.Front(); e != nil; e = enext {
		enext = e.Next()
		ctx := e.Value.(*notificationCtx)
		if ctx.connection == walletNotification {
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
func (r *wsContext) RemoveSpentRequest(walletNotification chan []byte, rc *requestContexts, op *btcwire.OutPoint) {
	r.Lock()
	defer r.Unlock()

	r.removeGlobalSpentRequest(walletNotification, op)
	delete(rc.spentRequests, *op)
}

// AddMinedTxRequest adds request contexts for notifications of a
// mined transaction.
func (r *wsContext) AddMinedTxRequest(walletNotification chan []byte, txID *btcwire.ShaHash) {
	r.Lock()
	defer r.Unlock()

	rc := r.connections[walletNotification]

	nc := &notificationCtx{
		connection: walletNotification,
		rc:         rc,
	}
	clist, ok := r.minedTxNotifications[*txID]
	if !ok {
		clist = list.New()
		r.minedTxNotifications[*txID] = clist
	}
	clist.PushBack(nc)
	rc.minedTxRequests[*txID] = true
}

func (r *wsContext) removeGlobalMinedTxRequest(walletNotification chan []byte, txID *btcwire.ShaHash) {
	clist := r.minedTxNotifications[*txID]
	var enext *list.Element
	for e := clist.Front(); e != nil; e = enext {
		enext = e.Next()
		ctx := e.Value.(*notificationCtx)
		if ctx.connection == walletNotification {
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
func (r *wsContext) RemoveMinedTxRequest(walletNotification chan []byte, rc *requestContexts, txID *btcwire.ShaHash) {
	r.Lock()
	defer r.Unlock()

	r.removeMinedTxRequest(walletNotification, rc, txID)
}

// removeMinedTxRequest removes request contexts for notifications of a
// mined transaction without grabbing any locks.
func (r *wsContext) removeMinedTxRequest(walletNotification chan []byte, rc *requestContexts, txID *btcwire.ShaHash) {
	r.removeGlobalMinedTxRequest(walletNotification, txID)
	delete(rc.minedTxRequests, *txID)
}

// CloseListeners removes all request contexts for notifications sent
// to a wallet notification channel and closes the channel to stop all
// goroutines currently serving that wallet.
func (r *wsContext) CloseListeners(walletNotification chan []byte) {
	r.Lock()
	defer r.Unlock()

	delete(r.connections, walletNotification)
	close(walletNotification)
}

// requestContexts holds all requests for a single wallet connection.
type requestContexts struct {
	// txRequests maps between a 160-byte pubkey hash and the JSON
	// id of the requester so replies can be correctly routed back
	// to the correct btcwallet callback.  The key must be a stringified
	// address hash.
	txRequests map[string]interface{}

	// spentRequests maps between an Outpoint of an unspent
	// transaction output and the JSON id of the requester so
	// replies can be correctly routed back to the correct
	// btcwallet callback.
	spentRequests map[btcwire.OutPoint]interface{}

	// minedTxRequests holds a set of transaction IDs (tx hashes) of
	// transactions created by a wallet. A wallet may request
	// notifications of when a tx it created is mined into a block and
	// removed from the mempool.  Once a tx has been mined into a
	// block, wallet may remove the raw transaction from its unmined tx
	// pool.
	minedTxRequests map[btcwire.ShaHash]bool
}

// respondToAnyCmd checks that a parsed command is a standard or
// extension JSON-RPC command and runs the proper handler to reply to
// the command.  Any and all responses are sent to the wallet from
// this function.
func respondToAnyCmd(cmd btcjson.Cmd, s *rpcServer,
	walletNotification chan []byte, rc *requestContexts) {

	// Lookup the websocket extension for the command and if it doesn't
	// exist fallback to handling the command as a standard command.
	wsHandler, ok := wsHandlers[cmd.Method()]
	if !ok {
		reply := standardCmdReply(cmd, s)
		mreply, _ := json.Marshal(reply)
		walletNotification <- mreply
		return
	}

	// Call the appropriate handler which responds unless there was an
	// error in which case the error is marshalled and sent here.
	if err := wsHandler(s, cmd, walletNotification, rc); err != nil {
		var reply btcjson.Reply
		jsonErr, ok := err.(btcjson.Error)
		if ok {
			reply.Error = &jsonErr
			mreply, _ := json.Marshal(reply)
			walletNotification <- mreply
			return
		}

		// In the case where we did not have a btcjson
		// error to begin with, make a new one to send,
		// but this really should not happen.
		jsonErr = btcjson.Error{
			Code:    btcjson.ErrInternal.Code,
			Message: err.Error(),
		}
		reply.Error = &jsonErr
		mreply, _ := json.Marshal(reply)
		walletNotification <- mreply
	}
}

// handleGetCurrentNet implements the getcurrentnet command extension
// for websocket connections.
func handleGetCurrentNet(s *rpcServer, cmd btcjson.Cmd,
	walletNotification chan []byte, rc *requestContexts) error {

	id := cmd.Id()
	reply := &btcjson.Reply{Id: &id}

	var net btcwire.BitcoinNet
	if cfg.TestNet3 {
		net = btcwire.TestNet3
	} else {
		net = btcwire.MainNet
	}

	reply.Result = float64(net)
	mreply, _ := json.Marshal(reply)
	walletNotification <- mreply
	return nil
}

// handleGetBestBlock implements the getbestblock command extension
// for websocket connections.
func handleGetBestBlock(s *rpcServer, cmd btcjson.Cmd,
	walletNotification chan []byte, rc *requestContexts) error {

	id := cmd.Id()
	reply := &btcjson.Reply{Id: &id}

	// All other "get block" commands give either the height, the
	// hash, or both but require the block SHA.  This gets both for
	// the best block.
	sha, height, err := s.server.db.NewestSha()
	if err != nil {
		return btcjson.ErrBestBlockHash
	}

	reply.Result = map[string]interface{}{
		"hash":   sha.String(),
		"height": height,
	}
	mreply, _ := json.Marshal(reply)
	walletNotification <- mreply
	return nil
}

// handleNotifyNewTXs implements the notifynewtxs command extension for
// websocket connections.
func handleNotifyNewTXs(s *rpcServer, cmd btcjson.Cmd,
	walletNotification chan []byte, rc *requestContexts) error {

	id := cmd.Id()
	reply := &btcjson.Reply{Id: &id}

	notifyCmd, ok := cmd.(*btcws.NotifyNewTXsCmd)
	if !ok {
		return btcjson.ErrInternal
	}

	for _, addr := range notifyCmd.Addresses {
		addr, err := btcutil.DecodeAddr(addr)
		if err != nil {
			return fmt.Errorf("cannot decode address: %v", err)
		}

		// TODO(jrick) Notifing for non-P2PKH addresses is currently
		// unsuported.
		if _, ok := addr.(*btcutil.AddressPubKeyHash); !ok {
			return fmt.Errorf("address is not P2PKH: %v", addr.EncodeAddress())
		}

		s.ws.AddTxRequest(walletNotification, rc, addr.EncodeAddress(),
			cmd.Id())
	}

	mreply, _ := json.Marshal(reply)
	walletNotification <- mreply
	return nil
}

// handleNotifySpent implements the notifyspent command extension for
// websocket connections.
func handleNotifySpent(s *rpcServer, cmd btcjson.Cmd,
	walletNotification chan []byte, rc *requestContexts) error {

	id := cmd.Id()
	reply := &btcjson.Reply{Id: &id}

	notifyCmd, ok := cmd.(*btcws.NotifySpentCmd)
	if !ok {
		return btcjson.ErrInternal
	}

	s.ws.AddSpentRequest(walletNotification, rc, notifyCmd.OutPoint,
		cmd.Id())

	mreply, _ := json.Marshal(reply)
	walletNotification <- mreply
	return nil
}

// handleRescan implements the rescan command extension for websocket
// connections.
func handleRescan(s *rpcServer, cmd btcjson.Cmd,
	walletNotification chan []byte, rc *requestContexts) error {

	id := cmd.Id()
	reply := &btcjson.Reply{Id: &id}

	rescanCmd, ok := cmd.(*btcws.RescanCmd)
	if !ok {
		return btcjson.ErrInternal
	}

	if len(rescanCmd.Addresses) == 1 {
		rpcsLog.Info("Beginning rescan for 1 address.")
	} else {
		rpcsLog.Infof("Beginning rescan for %v addresses.",
			len(rescanCmd.Addresses))
	}

	minblock := int64(rescanCmd.BeginBlock)
	maxblock := int64(rescanCmd.EndBlock)

	// FetchHeightRange may not return a complete list of block shas for
	// the given range, so fetch range as many times as necessary.
	for {
		blkshalist, err := s.server.db.FetchHeightRange(minblock,
			maxblock)
		if err != nil {
			return err
		}
		if len(blkshalist) == 0 {
			break
		}

		for i := range blkshalist {
			blk, err := s.server.db.FetchBlockBySha(&blkshalist[i])
			if err != nil {
				rpcsLog.Errorf("Error looking up block sha: %v",
					err)
				return err
			}
			for _, tx := range blk.Transactions() {
				var txReply *btcdb.TxListReply
				for txOutIdx, txout := range tx.MsgTx().TxOut {
					_, addrs, _, err := btcscript.ExtractPkScriptAddrs(
						txout.PkScript, s.server.btcnet)
					if err != nil {
						continue
					}

					for i, addr := range addrs {
						encodedAddr := addr.EncodeAddress()
						if _, ok := rescanCmd.Addresses[encodedAddr]; ok {
							// TODO(jrick): This lookup is expensive and can be avoided
							// if the wallet is sent the previous outpoints for all inputs
							// of the tx, so any can removed from the utxo set (since
							// they are, as of this tx, now spent).
							if txReply == nil {
								txReplyList, err := s.server.db.FetchTxBySha(tx.Sha())
								if err != nil {
									rpcsLog.Errorf("Tx Sha %v not found by db.", tx.Sha())
									return err
								}
								for i := range txReplyList {
									if txReplyList[i].Height == blk.Height() {
										txReply = txReplyList[i]
										break
									}
								}
							}

							reply.Result = struct {
								Receiver   string `json:"receiver"`
								Height     int64  `json:"height"`
								BlockHash  string `json:"blockhash"`
								BlockIndex int    `json:"blockindex"`
								BlockTime  int64  `json:"blocktime"`
								TxID       string `json:"txid"`
								TxOutIndex uint32 `json:"txoutindex"`
								Amount     int64  `json:"amount"`
								PkScript   string `json:"pkscript"`
								Spent      bool   `json:"spent"`
							}{
								Receiver:   encodedAddr,
								Height:     blk.Height(),
								BlockHash:  blkshalist[i].String(),
								BlockIndex: tx.Index(),
								BlockTime:  blk.MsgBlock().Header.Timestamp.Unix(),
								TxID:       tx.Sha().String(),
								TxOutIndex: uint32(txOutIdx),
								Amount:     txout.Value,
								PkScript:   btcutil.Base58Encode(txout.PkScript),
								Spent:      txReply.TxSpent[txOutIdx],
							}
							mreply, _ := json.Marshal(reply)
							walletNotification <- mreply
						}
					}
				}
			}
		}

		if maxblock-minblock > int64(len(blkshalist)) {
			minblock += int64(len(blkshalist))
		} else {
			break
		}
	}

	reply.Result = nil
	mreply, _ := json.Marshal(reply)
	walletNotification <- mreply

	rpcsLog.Info("Finished rescan")
	return nil
}

// handleWalletSendRawTransaction implements the websocket extended version of
// the sendrawtransaction command.
func handleWalletSendRawTransaction(s *rpcServer, cmd btcjson.Cmd,
	walletNotification chan []byte, rc *requestContexts) error {

	result, err := handleSendRawTransaction(s, cmd)
	if err != nil {
		return err
	}

	// The result is already guaranteed to be a valid hash string if no
	// error was returned above, so it's safe to ignore the error here.
	txSha, _ := btcwire.NewShaHashFromStr(result.(string))

	// Request to be notified when the transaction is mined.
	s.ws.AddMinedTxRequest(walletNotification, txSha)
	return nil
}

// AddWalletListener adds a channel to listen for new messages from a
// wallet.
func (s *rpcServer) AddWalletListener(c chan []byte) *requestContexts {
	s.ws.Lock()
	rc := &requestContexts{
		// The key is a stringified addressHash.
		txRequests: make(map[string]interface{}),

		spentRequests:   make(map[btcwire.OutPoint]interface{}),
		minedTxRequests: make(map[btcwire.ShaHash]bool),
	}
	s.ws.connections[c] = rc
	s.ws.Unlock()

	return rc
}

// RemoveWalletListener removes a wallet listener channel.
func (s *rpcServer) RemoveWalletListener(c chan []byte, rc *requestContexts) {
	s.ws.Lock()

	for k := range rc.txRequests {
		s.ws.removeGlobalTxRequest(c, k)
	}
	for k := range rc.spentRequests {
		s.ws.removeGlobalSpentRequest(c, &k)
	}
	for k := range rc.minedTxRequests {
		s.ws.removeGlobalMinedTxRequest(c, &k)
	}

	delete(s.ws.connections, c)
	s.ws.Unlock()
}

// walletListenerDuplicator listens for new wallet listener channels
// and duplicates messages sent to walletNotificationMaster to all
// connected listeners.
func (s *rpcServer) walletListenerDuplicator() {
	// Duplicate all messages sent across walletNotificationMaster to each
	// listening wallet.
	for {
		select {
		case ntfn := <-s.ws.walletNotificationMaster:
			s.ws.RLock()
			for c := range s.ws.connections {
				c <- ntfn
			}
			s.ws.RUnlock()

		case <-s.quit:
			return
		}
	}
}

// walletReqsNotifications is the handler function for websocket
// connections from a btcwallet instance.  It reads messages from wallet and
// sends back replies, as well as notififying wallets of chain updates.
func (s *rpcServer) walletReqsNotifications(ws *websocket.Conn) {
	// Add wallet notification channel so this handler receives btcd chain
	// notifications.
	c := make(chan []byte)
	rc := s.AddWalletListener(c)
	defer s.RemoveWalletListener(c, rc)

	// msgs is a channel for all messages received over the websocket.
	msgs := make(chan []byte)

	// Receive messages from websocket and send across reqs until the
	// connection is lost.
	go func() {
		for {
			select {
			case <-s.quit:
				close(msgs)
				return
			default:
				var m []byte
				if err := websocket.Message.Receive(ws, &m); err != nil {
					close(msgs)
					return
				}
				msgs <- m
			}
		}
	}()

	for {
		select {
		case m, ok := <-msgs:
			if !ok {
				// Wallet disconnected.
				return
			}
			// Handle request here.
			go s.websocketJSONHandler(c, rc, m)
		case ntfn, _ := <-c:
			// Send btcd notification to btcwallet instance over
			// websocket.
			if err := websocket.Message.Send(ws, ntfn); err != nil {
				// Wallet disconnected.
				return
			}
		case <-s.quit:
			// Server closed.
			return
		}
	}
}

// websocketJSONHandler parses and handles a marshalled json message,
// sending the marshalled reply to a wallet notification channel.
func (s *rpcServer) websocketJSONHandler(walletNotification chan []byte,
	rc *requestContexts, msg []byte) {

	s.wg.Add(1)
	defer s.wg.Done()

	cmd, jsonErr := parseCmd(msg)
	if jsonErr != nil {
		var reply btcjson.Reply
		if cmd != nil {
			// Unmarshaling at least a valid JSON-RPC message succeeded.
			// Use the provided id for errors.
			id := cmd.Id()
			reply.Id = &id
		}
		reply.Error = jsonErr
		mreply, _ := json.Marshal(reply)
		walletNotification <- mreply
		return
	}

	respondToAnyCmd(cmd, s, walletNotification, rc)
}

// NotifyBlockConnected creates and marshalls a JSON message to notify
// of a new block connected to the main chain.  The notification is sent
// to each connected wallet.
func (s *rpcServer) NotifyBlockConnected(block *btcutil.Block) {
	hash, err := block.Sha()
	if err != nil {
		rpcsLog.Error("Bad block; connected block notification dropped.")
		return
	}

	// TODO: remove int32 type conversion.
	ntfn := btcws.NewBlockConnectedNtfn(hash.String(),
		int32(block.Height()))
	mntfn, _ := json.Marshal(ntfn)
	s.ws.walletNotificationMaster <- mntfn

	// Inform any interested parties about txs mined in this block.
	s.ws.Lock()
	for _, tx := range block.Transactions() {
		if clist, ok := s.ws.minedTxNotifications[*tx.Sha()]; ok {
			var enext *list.Element
			for e := clist.Front(); e != nil; e = enext {
				enext = e.Next()
				ctx := e.Value.(*notificationCtx)
				// TODO: remove int32 type conversion after
				// the int64 -> int32 switch is made.
				ntfn := btcws.NewTxMinedNtfn(tx.Sha().String(),
					hash.String(), int32(block.Height()),
					block.MsgBlock().Header.Timestamp.Unix(),
					tx.Index())
				mntfn, _ := json.Marshal(ntfn)
				ctx.connection <- mntfn
				s.ws.removeMinedTxRequest(ctx.connection, ctx.rc,
					tx.Sha())
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
		rpcsLog.Error("Bad block; connected block notification dropped.")
		return
	}

	// TODO: remove int32 type conversion.
	ntfn := btcws.NewBlockDisconnectedNtfn(hash.String(),
		int32(block.Height()))
	mntfn, _ := json.Marshal(ntfn)
	s.ws.walletNotificationMaster <- mntfn
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

func notifySpentData(ctx *notificationCtx, txhash *btcwire.ShaHash, index uint32,
	spender *btcutil.Tx) {
	txStr := ""
	if spender != nil {
		var buf bytes.Buffer
		err := spender.MsgTx().Serialize(&buf)
		if err != nil {
			// This really shouldn't ever happen...
			rpcsLog.Warnf("Can't serialize tx: %v", err)
			return
		}
		txStr = string(buf.Bytes())
	}

	reply := &btcjson.Reply{
		Result: struct {
			TxHash     string `json:"txhash"`
			Index      uint32 `json:"index"`
			SpendingTx string `json:"spender,omitempty"`
		}{
			TxHash:     txhash.String(),
			Index:      index,
			SpendingTx: txStr,
		},
		Error: nil,
		Id:    &ctx.id,
	}
	replyBytes, err := json.Marshal(reply)
	if err != nil {
		rpcsLog.Errorf("Unable to marshal spent notification: %v", err)
		return
	}
	ctx.connection <- replyBytes
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
				ctx := e.Value.(*notificationCtx)
				notifySpentData(ctx, &txin.PreviousOutpoint.Hash,
					uint32(txin.PreviousOutpoint.Index), tx)
				s.ws.RemoveSpentRequest(ctx.connection, ctx.rc,
					&txin.PreviousOutpoint)
			}
		}
	}
}

// NotifyForTxOuts iterates through all outputs of a tx, performing any
// necessary notifications for wallets.  If a non-nil block is passed,
// additional block information is passed with the notifications.
func (s *rpcServer) NotifyForTxOuts(tx *btcutil.Tx, block *btcutil.Block) {
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
					ctx := e.Value.(*notificationCtx)

					// TODO(jrick): shove this in btcws
					result := struct {
						Receiver   string `json:"receiver"`
						Height     int64  `json:"height"`
						BlockHash  string `json:"blockhash"`
						BlockIndex int    `json:"blockindex"`
						BlockTime  int64  `json:"blocktime"`
						TxID       string `json:"txid"`
						TxOutIndex uint32 `json:"txoutindex"`
						Amount     int64  `json:"amount"`
						PkScript   string `json:"pkscript"`
					}{
						Receiver:   encodedAddr,
						TxID:       tx.Sha().String(),
						TxOutIndex: uint32(i),
						Amount:     txout.Value,
						PkScript:   btcutil.Base58Encode(txout.PkScript),
					}

					if block != nil {
						blkhash, err := block.Sha()
						if err != nil {
							rpcsLog.Error("Error getting block sha; dropping Tx notification.")
							break
						}
						result.Height = block.Height()
						result.BlockHash = blkhash.String()
						result.BlockIndex = tx.Index()
						result.BlockTime = block.MsgBlock().Header.Timestamp.Unix()
					} else {
						result.Height = -1
						result.BlockIndex = -1
					}

					reply := &btcjson.Reply{
						Result: result,
						Error:  nil,
						Id:     &ctx.id,
					}
					mreply, err := json.Marshal(reply)
					if err != nil {
						rpcsLog.Errorf("Unable to marshal tx notification: %v", err)
						continue
					}
					ctx.connection <- mreply
				}
			}
		}
	}
}
