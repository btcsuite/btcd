// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"code.google.com/p/go.net/websocket"
	"container/list"
	"encoding/hex"
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

type walletChan chan []byte

type wsCommandHandler func(*rpcServer, btcjson.Cmd, walletChan) error

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

	// connections holds a map of requests for each wallet using the
	// wallet channel as the key.
	connections map[walletChan]*requestContexts

	// Any chain notifications meant to be received by every connected
	// wallet are sent across this channel.
	walletNotificationMaster walletChan

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

// AddTxRequest adds the request context for new transaction notifications.
func (r *wsContext) AddTxRequest(wallet walletChan, addr string) {
	r.Lock()
	defer r.Unlock()

	clist, ok := r.txNotifications[addr]
	if !ok {
		clist = list.New()
		r.txNotifications[addr] = clist
	}

	clist.PushBack(wallet)

	rc := r.connections[wallet]
	rc.txRequests[addr] = struct{}{}
}

func (r *wsContext) removeGlobalTxRequest(wallet walletChan, addr string) {
	clist := r.txNotifications[addr]
	var enext *list.Element
	for e := clist.Front(); e != nil; e = enext {
		enext = e.Next()
		c := e.Value.(walletChan)
		if c == wallet {
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
func (r *wsContext) AddSpentRequest(wallet walletChan, op *btcwire.OutPoint) {
	r.Lock()
	defer r.Unlock()

	clist, ok := r.spentNotifications[*op]
	if !ok {
		clist = list.New()
		r.spentNotifications[*op] = clist
	}
	clist.PushBack(wallet)

	rc := r.connections[wallet]
	rc.spentRequests[*op] = struct{}{}
}

func (r *wsContext) removeGlobalSpentRequest(wallet walletChan, op *btcwire.OutPoint) {
	clist := r.spentNotifications[*op]
	var enext *list.Element
	for e := clist.Front(); e != nil; e = enext {
		enext = e.Next()
		c := e.Value.(walletChan)
		if c == wallet {
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
func (r *wsContext) RemoveSpentRequest(wallet walletChan, op *btcwire.OutPoint) {
	r.Lock()
	defer r.Unlock()

	r.removeGlobalSpentRequest(wallet, op)
	rc := r.connections[wallet]
	delete(rc.spentRequests, *op)
}

// AddMinedTxRequest adds request contexts for notifications of a
// mined transaction.
func (r *wsContext) AddMinedTxRequest(wallet walletChan, txID *btcwire.ShaHash) {
	r.Lock()
	defer r.Unlock()

	clist, ok := r.minedTxNotifications[*txID]
	if !ok {
		clist = list.New()
		r.minedTxNotifications[*txID] = clist
	}
	clist.PushBack(wallet)

	rc := r.connections[wallet]
	rc.minedTxRequests[*txID] = struct{}{}
}

func (r *wsContext) removeGlobalMinedTxRequest(wallet walletChan, txID *btcwire.ShaHash) {
	clist := r.minedTxNotifications[*txID]
	var enext *list.Element
	for e := clist.Front(); e != nil; e = enext {
		enext = e.Next()
		c := e.Value.(walletChan)
		if c == wallet {
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
func (r *wsContext) RemoveMinedTxRequest(wallet walletChan, txID *btcwire.ShaHash) {
	r.Lock()
	defer r.Unlock()

	r.removeMinedTxRequest(wallet, txID)
}

// removeMinedTxRequest removes request contexts for notifications of a
// mined transaction without grabbing any locks.
func (r *wsContext) removeMinedTxRequest(wallet walletChan, txID *btcwire.ShaHash) {
	r.removeGlobalMinedTxRequest(wallet, txID)
	rc := r.connections[wallet]
	delete(rc.minedTxRequests, *txID)
}

// CloseListeners removes all request contexts for notifications sent
// to a wallet notification channel and closes the channel to stop all
// goroutines currently serving that wallet.
func (r *wsContext) CloseListeners(wallet walletChan) {
	r.Lock()
	defer r.Unlock()

	delete(r.connections, wallet)
	close(wallet)
}

// requestContexts holds all requests for a single wallet connection.
type requestContexts struct {
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
func respondToAnyCmd(cmd btcjson.Cmd, s *rpcServer, wallet walletChan) {
	// Lookup the websocket extension for the command and if it doesn't
	// exist fallback to handling the command as a standard command.
	wsHandler, ok := wsHandlers[cmd.Method()]
	if !ok {
		reply := standardCmdReply(cmd, s)
		mreply, _ := json.Marshal(reply)
		wallet <- mreply
		return
	}

	// Call the appropriate handler which responds unless there was an
	// error in which case the error is marshalled and sent here.
	if err := wsHandler(s, cmd, wallet); err != nil {
		var reply btcjson.Reply
		jsonErr, ok := err.(btcjson.Error)
		if ok {
			reply.Error = &jsonErr
			mreply, _ := json.Marshal(reply)
			wallet <- mreply
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
		wallet <- mreply
	}
}

// handleGetCurrentNet implements the getcurrentnet command extension
// for websocket connections.
func handleGetCurrentNet(s *rpcServer, cmd btcjson.Cmd, wallet walletChan) error {
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
	wallet <- mreply
	return nil
}

// handleGetBestBlock implements the getbestblock command extension
// for websocket connections.
func handleGetBestBlock(s *rpcServer, cmd btcjson.Cmd, wallet walletChan) error {
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
	wallet <- mreply
	return nil
}

// handleNotifyNewTXs implements the notifynewtxs command extension for
// websocket connections.
func handleNotifyNewTXs(s *rpcServer, cmd btcjson.Cmd, wallet walletChan) error {
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

		s.ws.AddTxRequest(wallet, addr.EncodeAddress())
	}

	mreply, _ := json.Marshal(reply)
	wallet <- mreply
	return nil
}

// handleNotifySpent implements the notifyspent command extension for
// websocket connections.
func handleNotifySpent(s *rpcServer, cmd btcjson.Cmd, wallet walletChan) error {
	id := cmd.Id()
	reply := &btcjson.Reply{Id: &id}

	notifyCmd, ok := cmd.(*btcws.NotifySpentCmd)
	if !ok {
		return btcjson.ErrInternal
	}

	s.ws.AddSpentRequest(wallet, notifyCmd.OutPoint)

	mreply, _ := json.Marshal(reply)
	wallet <- mreply
	return nil
}

// handleRescan implements the rescan command extension for websocket
// connections.
func handleRescan(s *rpcServer, cmd btcjson.Cmd, wallet walletChan) error {
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
			txouts:
				for txOutIdx, txout := range tx.MsgTx().TxOut {
					_, addrs, _, err := btcscript.ExtractPkScriptAddrs(
						txout.PkScript, s.server.btcnet)
					if err != nil {
						continue txouts
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
									continue txouts
								}
								for i := range txReplyList {
									if txReplyList[i].Height == blk.Height() {
										txReply = txReplyList[i]
										break
									}
								}

							}

							ntfn := &btcws.ProcessedTxNtfn{
								Receiver:    encodedAddr,
								Amount:      txout.Value,
								TxID:        tx.Sha().String(),
								TxOutIndex:  uint32(txOutIdx),
								PkScript:    hex.EncodeToString(txout.PkScript),
								BlockHash:   blkshalist[i].String(),
								BlockHeight: int32(blk.Height()),
								BlockIndex:  tx.Index(),
								BlockTime:   blk.MsgBlock().Header.Timestamp.Unix(),
								Spent:       txReply.TxSpent[txOutIdx],
							}
							mntfn, _ := ntfn.MarshalJSON()
							wallet <- mntfn
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

	rpcsLog.Info("Finished rescan")

	id := cmd.Id()
	response := &btcjson.Reply{
		Id:     &id,
		Result: nil,
		Error:  nil,
	}
	mresponse, _ := json.Marshal(response)
	wallet <- mresponse

	return nil
}

// handleWalletSendRawTransaction implements the websocket extended version of
// the sendrawtransaction command.
func handleWalletSendRawTransaction(s *rpcServer, cmd btcjson.Cmd, wallet walletChan) error {
	result, err := handleSendRawTransaction(s, cmd)
	if err != nil {
		return err
	}

	// The result is already guaranteed to be a valid hash string if no
	// error was returned above, so it's safe to ignore the error here.
	txSha, _ := btcwire.NewShaHashFromStr(result.(string))

	// Request to be notified when the transaction is mined.
	s.ws.AddMinedTxRequest(wallet, txSha)
	return nil
}

// AddWalletListener adds a channel to listen for new messages from a
// wallet.
func (s *rpcServer) AddWalletListener(c walletChan) {
	s.ws.Lock()
	rc := &requestContexts{
		txRequests:      make(map[string]struct{}),
		spentRequests:   make(map[btcwire.OutPoint]struct{}),
		minedTxRequests: make(map[btcwire.ShaHash]struct{}),
	}
	s.ws.connections[c] = rc
	s.ws.Unlock()
}

// RemoveWalletListener removes a wallet listener channel.
func (s *rpcServer) RemoveWalletListener(c walletChan) {
	s.ws.Lock()

	rc := s.ws.connections[c]
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
	c := make(walletChan)
	s.AddWalletListener(c)
	defer s.RemoveWalletListener(c)

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
			go s.websocketJSONHandler(c, m)
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
func (s *rpcServer) websocketJSONHandler(wallet walletChan, msg []byte) {
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
		wallet <- mreply
		return
	}

	respondToAnyCmd(cmd, s, wallet)
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
				c := e.Value.(walletChan)
				// TODO: remove int32 type conversion after
				// the int64 -> int32 switch is made.
				ntfn := btcws.NewTxMinedNtfn(tx.Sha().String(),
					hash.String(), int32(block.Height()),
					block.MsgBlock().Header.Timestamp.Unix(),
					tx.Index())
				mntfn, _ := json.Marshal(ntfn)
				c <- mntfn
				s.ws.removeMinedTxRequest(c, tx.Sha())
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

func notifySpentData(wallet walletChan, txhash *btcwire.ShaHash, index uint32,
	spender *btcutil.Tx) {

	var buf bytes.Buffer
	// Ignore Serialize's error, as writing to a bytes.buffer
	// cannot fail.
	spender.MsgTx().Serialize(&buf)
	txStr := hex.EncodeToString(buf.Bytes())

	// TODO(jrick): create a new notification in btcws and use that.
	ntfn := btcws.NewTxSpentNtfn(txhash.String(), int(index), txStr)
	mntfn, _ := ntfn.MarshalJSON()
	wallet <- mntfn
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
				c := e.Value.(walletChan)
				notifySpentData(c, &txin.PreviousOutpoint.Hash,
					txin.PreviousOutpoint.Index, tx)
				s.ws.RemoveSpentRequest(c, &txin.PreviousOutpoint)
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
					wallet := e.Value.(walletChan)

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
							rpcsLog.Error("Error getting block sha; dropping Tx notification.")
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

					mntfn, _ := ntfn.MarshalJSON()
					wallet <- mntfn
				}
			}
		}
	}
}
