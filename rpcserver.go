// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"code.google.com/p/go.crypto/ripemd160"
	"code.google.com/p/go.net/websocket"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/conformal/btcchain"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Errors
var (
	// ErrBadParamsField describes an error where the parameters JSON
	// field cannot be properly parsed.
	ErrBadParamsField = errors.New("bad params field")

	// ErrMethodNotImplemented describes an error where the RPC or
	// websocket JSON method is not implemented.
	ErrMethodNotImplemented = errors.New("method not implemented")
)

// rpcServer holds the items the rpc server may need to access (config,
// shutdown, main server, etc.)
type rpcServer struct {
	started   int32
	shutdown  int32
	server    *server
	ws        wsContext
	wg        sync.WaitGroup
	rpcport   string
	username  string
	password  string
	listeners []net.Listener
	quit      chan int
}

// wsContext holds the items the RPC server needs to handle websocket
// connections for wallets.
type wsContext struct {
	// requests holds all wallet notification requests.
	requests wsRequests

	// Channel to add a wallet listener.
	addWalletListener chan (chan []byte)

	// Channel to removes a wallet listener.
	removeWalletListener chan (chan []byte)

	// Any chain notifications meant to be received by every connected
	// wallet are sent across this channel.
	walletNotificationMaster chan []byte
}

// wsRequests maps request contexts for wallet notifications to a
// wallet notification channel.  A Mutex is used to protect incorrect
// concurrent access to the map.
type wsRequests struct {
	sync.Mutex
	m map[chan []byte]*requestContexts
}

// getOrCreateContexts gets the request contexts, or creates and adds a
// new context if one for this wallet is not already present.
func (r *wsRequests) getOrCreateContexts(walletNotification chan []byte) *requestContexts {
	rc, ok := r.m[walletNotification]
	if !ok {
		rc = &requestContexts{
			txRequests:    make(map[addressHash]interface{}),
			spentRequests: make(map[btcwire.OutPoint]interface{}),
		}
		r.m[walletNotification] = rc
	}
	return rc
}

// AddTxRequest adds the request context for new transaction notifications.
func (r *wsRequests) AddTxRequest(walletNotification chan []byte, addr addressHash, id interface{}) {
	r.Lock()
	defer r.Unlock()

	rc := r.getOrCreateContexts(walletNotification)
	rc.txRequests[addr] = id
}

// AddSpentRequest adds a request context for notifications of a spent
// Outpoint.
func (r *wsRequests) AddSpentRequest(walletNotification chan []byte, op *btcwire.OutPoint, id interface{}) {
	r.Lock()
	defer r.Unlock()

	rc := r.getOrCreateContexts(walletNotification)
	rc.spentRequests[*op] = id
}

// RemoveSpentRequest removes a request context for notifications of a
// spent Outpoint.
func (r *wsRequests) RemoveSpentRequest(walletNotification chan []byte, op *btcwire.OutPoint) {
	r.Lock()
	defer r.Unlock()

	rc := r.getOrCreateContexts(walletNotification)
	delete(rc.spentRequests, *op)
}

// CloseListeners removes all request contexts for notifications sent
// to a wallet notification channel and closes the channel to stop all
// goroutines currently serving that wallet.
func (r *wsRequests) CloseListeners(walletNotification chan []byte) {
	r.Lock()
	defer r.Unlock()

	delete(r.m, walletNotification)
	close(walletNotification)
}

type addressHash [ripemd160.Size]byte

// requestContexts holds all requests for a single wallet connection.
type requestContexts struct {
	// txRequests maps between a 160-byte pubkey hash and the JSON
	// id of the requester so replies can be correctly routed back
	// to the correct btcwallet callback.
	txRequests map[addressHash]interface{}

	// spentRequests maps between an Outpoint of an unspent
	// transaction output and the JSON id of the requester so
	// replies can be correctly routed back to the correct
	// btcwallet callback.
	spentRequests map[btcwire.OutPoint]interface{}
}

// Start is used by server.go to start the rpc listener.
func (s *rpcServer) Start() {
	if atomic.AddInt32(&s.started, 1) != 1 {
		return
	}

	log.Trace("RPCS: Starting RPC server")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		login := s.username + ":" + s.password
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(login))
		authhdr := r.Header["Authorization"]
		if len(authhdr) > 0 && authhdr[0] == auth {
			jsonRPCRead(w, r, s)
		} else {
			log.Warnf("RPCS: Auth failure.")
			jsonAuthFail(w, r, s)
		}
	})
	go s.walletListenerDuplicator()
	http.Handle("/wallet", websocket.Handler(func(ws *websocket.Conn) {
		s.walletReqsNotifications(ws)
	}))
	httpServer := &http.Server{}
	for _, listener := range s.listeners {
		s.wg.Add(1)
		go func(listener net.Listener) {
			log.Infof("RPCS: RPC server listening on %s", listener.Addr())
			httpServer.Serve(listener)
			log.Tracef("RPCS: RPC listener done for %s", listener.Addr())
			s.wg.Done()
		}(listener)
	}
}

// Stop is used by server.go to stop the rpc listener.
func (s *rpcServer) Stop() error {
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		log.Infof("RPCS: RPC server is already in the process of shutting down")
		return nil
	}
	log.Warnf("RPCS: RPC server shutting down")
	for _, listener := range s.listeners {
		err := listener.Close()
		if err != nil {
			log.Errorf("RPCS: Problem shutting down rpc: %v", err)
			return err
		}
	}
	log.Infof("RPCS: RPC server shutdown complete")
	s.wg.Wait()
	close(s.quit)
	return nil
}

// newRPCServer returns a new instance of the rpcServer struct.
func newRPCServer(s *server) (*rpcServer, error) {
	rpc := rpcServer{
		server: s,
		quit:   make(chan int),
	}
	// Get values from config
	rpc.rpcport = cfg.RPCPort
	rpc.username = cfg.RPCUser
	rpc.password = cfg.RPCPass

	// initialize memory for websocket connections
	rpc.ws.requests.m = make(map[chan []byte]*requestContexts)
	rpc.ws.addWalletListener = make(chan (chan []byte))
	rpc.ws.removeWalletListener = make(chan (chan []byte))
	rpc.ws.walletNotificationMaster = make(chan []byte)

	// IPv4 listener.
	var listeners []net.Listener
	listenAddr4 := net.JoinHostPort("127.0.0.1", rpc.rpcport)
	listener4, err := net.Listen("tcp4", listenAddr4)
	if err != nil {
		log.Errorf("RPCS: Couldn't create listener: %v", err)
		return nil, err
	}
	listeners = append(listeners, listener4)

	// IPv6 listener.
	listenAddr6 := net.JoinHostPort("::1", rpc.rpcport)
	listener6, err := net.Listen("tcp6", listenAddr6)
	if err != nil {
		log.Errorf("RPCS: Couldn't create listener: %v", err)
		return nil, err
	}
	listeners = append(listeners, listener6)

	rpc.listeners = listeners

	return &rpc, err
}

// jsonAuthFail sends a message back to the client if the http auth is rejected.
func jsonAuthFail(w http.ResponseWriter, r *http.Request, s *rpcServer) {
	fmt.Fprint(w, "401 Unauthorized.\n")
}

// jsonRPCRead is the RPC wrapper around the jsonRead function to handles
// reading and responding to RPC messages.
func jsonRPCRead(w http.ResponseWriter, r *http.Request, s *rpcServer) {
	r.Close = true
	if atomic.LoadInt32(&s.shutdown) != 0 {
		return
	}
	body, err := btcjson.GetRaw(r.Body)
	if err != nil {
		log.Errorf("RPCS: Error getting json message: %v", err)
		return
	}

	// Error is intentionally ignored here.  It's used in in the
	// websocket handler to tell when a method is not supported by
	// the standard RPC API, and is not needed here.  Error logging
	// is done inside jsonRead, so no need to log the error here.
	reply, _ := jsonRead(body, s)
	log.Tracef("[RPCS] reply: %v", reply)

	msg, err := btcjson.MarshallAndSend(reply, w)
	if err != nil {
		log.Errorf(msg)
		return
	}
	log.Debugf(msg)
}

// jsonRead abstracts the JSON unmarshalling and reply handling used
// by both RPC and websockets.
func jsonRead(body []byte, s *rpcServer) (reply btcjson.Reply, err error) {
	var message btcjson.Message
	if err := json.Unmarshal(body, &message); err != nil {
		jsonError := btcjson.ErrParse

		reply = btcjson.Reply{
			Result: nil,
			Error:  &jsonError,
			Id:     nil,
		}

		log.Tracef("RPCS: reply: %v", reply)

		return reply, jsonError
	}
	log.Tracef("RPCS: received: %v", message)

	// Set final reply based on error if non-nil.
	defer func() {
		if err != nil {
			if jsonErr, ok := err.(btcjson.Error); ok {
				reply = btcjson.Reply{
					Error: &jsonErr,
					Id:    &message.Id,
				}
				err = errors.New(jsonErr.Message)
			} else {
				// In the case where we did not have a btcjson
				// error to begin with, make a new one to send,
				// but this really should not happen.
				rawJSONError := btcjson.Error{
					Code:    -32603,
					Message: err.Error(),
				}
				reply = btcjson.Reply{
					Error: &rawJSONError,
					Id:    &message.Id,
				}
			}
		}
	}()

	// Deal with commands
	switch message.Method {
	case "stop":
		reply = btcjson.Reply{
			Result: "btcd stopping.",
			Id:     &message.Id,
		}
		s.server.Stop()

	case "getblockcount":
		var maxidx int64
		_, maxidx, err = s.server.db.NewestSha()
		if err != nil {
			log.Errorf("RPCS: Error getting newest sha: %v", err)
			err = btcjson.ErrBlockCount
			return
		}
		reply = btcjson.Reply{
			Result: maxidx,
			Id:     &message.Id,
		}

	case "getbestblockhash":
		var sha *btcwire.ShaHash
		sha, _, err = s.server.db.NewestSha()
		if err != nil {
			log.Errorf("RPCS: Error getting newest sha: %v", err)
			err = btcjson.ErrBestBlockHash
			return
		}
		reply = btcjson.Reply{
			Result: sha.String(),
			Id:     &message.Id,
		}

	case "getconnectioncount":
		var count int

		reply = btcjson.Reply{
			Result: count,
			Id:     &message.Id,
		}

	case "getdifficulty":
		var sha *btcwire.ShaHash
		sha, _, err = s.server.db.NewestSha()
		if err != nil {
			log.Errorf("RPCS: Error getting sha: %v", err)
			err = btcjson.ErrDifficulty
			return
		}
		var blk *btcutil.Block
		blk, err = s.server.db.FetchBlockBySha(sha)
		if err != nil {
			log.Errorf("RPCS: Error getting block: %v", err)
			err = btcjson.ErrDifficulty
			return
		}
		blockHeader := &blk.MsgBlock().Header
		reply = btcjson.Reply{
			Result: getDifficultyRatio(blockHeader.Bits),
			Id:     &message.Id,
		}

	// btcd does not do mining so we can hardcode replies here.
	case "getgenerate":
		reply = btcjson.Reply{
			Result: false,
			Id:     &message.Id,
		}

	case "setgenerate":
		reply = btcjson.Reply{
			Result: nil,
			Id:     &message.Id,
		}

	case "gethashespersec":
		reply = btcjson.Reply{
			Result: 0,
			Id:     &message.Id,
		}

	case "getblockhash":
		var f interface{}
		err = json.Unmarshal(body, &f)
		m := f.(map[string]interface{})
		var idx float64
		for _, v := range m {
			switch vv := v.(type) {
			case []interface{}:
				for _, u := range vv {
					idx, _ = u.(float64)
				}
			default:
			}
		}
		var sha *btcwire.ShaHash
		sha, err = s.server.db.FetchBlockShaByHeight(int64(idx))
		if err != nil {
			log.Errorf("[RCPS] Error getting block: %v", err)
			err = btcjson.ErrOutOfRange
			return
		}
		reply = btcjson.Reply{
			Result: sha.String(),
			Id:     &message.Id,
		}

	case "getblock":
		var f interface{}
		err = json.Unmarshal(body, &f)
		m := f.(map[string]interface{})
		var hash string
		for _, v := range m {
			switch vv := v.(type) {
			case []interface{}:
				for _, u := range vv {
					hash, _ = u.(string)
				}
			default:
			}
		}
		var sha *btcwire.ShaHash
		sha, err = btcwire.NewShaHashFromStr(hash)
		if err != nil {
			log.Errorf("RPCS: Error generating sha: %v", err)
			err = btcjson.ErrBlockNotFound
			return
		}
		var blk *btcutil.Block
		blk, err = s.server.db.FetchBlockBySha(sha)
		if err != nil {
			log.Errorf("RPCS: Error fetching sha: %v", err)
			err = btcjson.ErrBlockNotFound
		}
		idx := blk.Height()
		var buf []byte
		buf, err = blk.Bytes()
		if err != nil {
			log.Errorf("RPCS: Error fetching block: %v", err)
			err = btcjson.ErrBlockNotFound
			return
		}

		txList, _ := blk.TxShas()

		txNames := make([]string, len(txList))
		for i, v := range txList {
			txNames[i] = v.String()
		}

		var maxidx int64
		_, maxidx, err = s.server.db.NewestSha()
		if err != nil {
			log.Errorf("RPCS: Cannot get newest sha: %v", err)
			err = btcjson.ErrBlockNotFound
			return
		}

		blockHeader := &blk.MsgBlock().Header
		blockReply := btcjson.BlockResult{
			Hash:          hash,
			Version:       blockHeader.Version,
			MerkleRoot:    blockHeader.MerkleRoot.String(),
			PreviousHash:  blockHeader.PrevBlock.String(),
			Nonce:         blockHeader.Nonce,
			Time:          blockHeader.Timestamp.Unix(),
			Confirmations: uint64(1 + maxidx - idx),
			Height:        idx,
			Tx:            txNames,
			Size:          len(buf),
			Bits:          strconv.FormatInt(int64(blockHeader.Bits), 16),
			Difficulty:    getDifficultyRatio(blockHeader.Bits),
		}

		// Get next block unless we are already at the top.
		if idx < maxidx {
			var shaNext *btcwire.ShaHash
			shaNext, err = s.server.db.FetchBlockShaByHeight(int64(idx + 1))
			if err != nil {
				log.Errorf("RPCS: No next block: %v", err)
				err = btcjson.ErrBlockNotFound
				return
			}
			blockReply.NextHash = shaNext.String()
		}

		reply = btcjson.Reply{
			Result: blockReply,
			Error:  nil,
			Id:     &message.Id,
		}

	case "getrawmempool":
		hashes := s.server.txMemPool.TxShas()
		hashStrings := make([]string, len(hashes))
		for i := 0; i < len(hashes); i++ {
			hashStrings[i] = hashes[i].String()
		}
		reply = btcjson.Reply{
			Result: hashStrings,
			Id:     &message.Id,
		}

	case "getrawtransaction":
		// TODO: Perform smarter paramter parsing.
		var f interface{}
		err = json.Unmarshal(body, &f)
		m := f.(map[string]interface{})
		var tx string
		var verbose float64
		for _, v := range m {
			switch vv := v.(type) {
			case []interface{}:
				for _, u := range vv {
					switch uu := u.(type) {
					case string:
						tx = uu
					case float64:
						verbose = uu
					default:
					}
				}
			default:
			}
		}

		if int(verbose) != 0 {
			// TODO: check error code. tx is not checked before
			// this point.
			txSha, _ := btcwire.NewShaHashFromStr(tx)
			var txS *btcwire.MsgTx
			var txList []*btcdb.TxListReply
			txList, err = s.server.db.FetchTxBySha(txSha)
			if err != nil {
				log.Errorf("RPCS: Error fetching tx: %v", err)
				err = btcjson.ErrNoTxInfo
				return
			}

			lastTx := len(txList) - 1
			txS = txList[lastTx].Tx
			blksha := txList[lastTx].BlkSha
			var blk *btcutil.Block
			blk, err = s.server.db.FetchBlockBySha(blksha)
			if err != nil {
				log.Errorf("RPCS: Error fetching sha: %v", err)
				err = btcjson.ErrBlockNotFound
				return
			}
			idx := blk.Height()

			txOutList := txS.TxOut
			voutList := make([]btcjson.Vout, len(txOutList))

			txInList := txS.TxIn
			vinList := make([]btcjson.Vin, len(txInList))

			for i, v := range txInList {
				vinList[i].Sequence = float64(v.Sequence)
				disbuf, _ := btcscript.DisasmString(v.SignatureScript)
				vinList[i].ScriptSig.Asm = strings.Replace(disbuf, " ", "", -1)
				vinList[i].Vout = i + 1
				log.Debugf(disbuf)
			}

			for i, v := range txOutList {
				voutList[i].N = i
				voutList[i].Value = float64(v.Value) / 100000000
				isbuf, _ := btcscript.DisasmString(v.PkScript)
				voutList[i].ScriptPubKey.Asm = isbuf
				voutList[i].ScriptPubKey.ReqSig = strings.Count(isbuf, "OP_CHECKSIG")
				_, addrhash, err := btcscript.ScriptToAddrHash(v.PkScript)
				if err != nil {
					// TODO: set and return error?
					log.Errorf("RPCS: Error getting address hash for %v: %v", txSha, err)
				}
				if addr, err := btcutil.EncodeAddress(addrhash, s.server.btcnet); err != nil {
					// TODO: set and return error?
					addrList := make([]string, 1)
					addrList[0] = addr
					voutList[i].ScriptPubKey.Addresses = addrList
				}
			}

			var maxidx int64
			_, maxidx, err = s.server.db.NewestSha()
			if err != nil {
				log.Errorf("RPCS: Cannot get newest sha: %v", err)
				err = btcjson.ErrNoNewestBlockInfo
				return
			}
			confirmations := uint64(1 + maxidx - idx)

			blockHeader := &blk.MsgBlock().Header
			txReply := btcjson.TxRawResult{
				Txid:     tx,
				Vout:     voutList,
				Vin:      vinList,
				Version:  txS.Version,
				LockTime: txS.LockTime,
				// This is not a typo, they are identical in
				// bitcoind as well.
				Time:          blockHeader.Timestamp.Unix(),
				Blocktime:     blockHeader.Timestamp.Unix(),
				BlockHash:     blksha.String(),
				Confirmations: confirmations,
			}
			reply = btcjson.Reply{
				Result: txReply,
				Error:  nil,
				Id:     &message.Id,
			}
		} else {
			// Don't return details
			// not used yet
		}

	case "decoderawtransaction":
		// TODO: Perform smarter paramter parsing.
		var f interface{}
		err = json.Unmarshal(body, &f)
		m := f.(map[string]interface{})
		var hash string
		for _, v := range m {
			switch vv := v.(type) {
			case []interface{}:
				for _, u := range vv {
					hash, _ = u.(string)
				}
			default:
			}
		}
		// TODO: use hash and fill result with info.
		_ = hash
		txReply := btcjson.TxRawDecodeResult{}
		reply = btcjson.Reply{
			Result: txReply,
			Error:  nil,
			Id:     &message.Id,
		}

	case "sendrawtransaction":
		params, ok := message.Params.([]interface{})
		if !ok || len(params) != 1 {
			err = btcjson.ErrInvalidParams
			return
		}
		serializedtxhex, ok := params[0].(string)
		if !ok {
			err = btcjson.ErrRawTxString
			return
		}

		// Deserialize and send off to tx relay
		var serializedTx []byte
		serializedTx, err = hex.DecodeString(serializedtxhex)
		if err != nil {
			err = btcjson.ErrDecodeHexString
			return
		}
		msgtx := btcwire.NewMsgTx()
		err = msgtx.Deserialize(bytes.NewBuffer(serializedTx))
		if err != nil {
			err = btcjson.Error{
				Code:    -22,
				Message: "Unable to deserialize raw tx",
			}
			return
		}
		err = s.server.txMemPool.ProcessTransaction(msgtx)
		if err != nil {
			log.Errorf("RPCS: Failed to process transaction: %v", err)
			err = btcjson.Error{
				Code:    -22,
				Message: "Failed to process transaction",
			}
			return
		}

		var result interface{}
		txsha, err := msgtx.TxSha()
		if err == nil {
			result = txsha.String()
		}
		reply = btcjson.Reply{
			Result: result,
			Error:  nil,
			Id:     &message.Id,
		}

	default:
		jsonError := btcjson.Error{
			Code:    -32601,
			Message: "Method not found",
		}
		reply = btcjson.Reply{
			Result: nil,
			Error:  &jsonError,
			Id:     &message.Id,
		}
		err = ErrMethodNotImplemented
	}

	return
}

func jsonWSRead(walletNotification chan []byte, replychan chan *btcjson.Reply, body []byte, s *rpcServer) error {
	var message btcjson.Message
	err := json.Unmarshal(body, &message)
	if err != nil {
		jsonError := btcjson.Error{
			Code:    -32700,
			Message: "Parse error",
		}

		reply := btcjson.Reply{
			Result: nil,
			Error:  &jsonError,
			Id:     nil,
		}

		log.Tracef("RPCS: reply: %v", reply)

		replychan <- &reply
		return fmt.Errorf("RPCS: Error unmarshalling json message: %v", err)
	}
	log.Tracef("RPCS: received: %v", message)

	var rawReply btcjson.Reply
	defer func() {
		replychan <- &rawReply
		close(replychan)
	}()

	// Deal with commands
	switch message.Method {
	case "getcurrentnet":
		var net btcwire.BitcoinNet
		if cfg.TestNet3 {
			net = btcwire.TestNet3
		} else {
			net = btcwire.MainNet
		}
		rawReply = btcjson.Reply{
			Result: float64(net),
			Id:     &message.Id,
		}

	case "rescan":
		var addr string
		minblock, maxblock := int64(0), btcdb.AllShas
		params, ok := message.Params.([]interface{})
		if !ok {
			return ErrBadParamsField
		}
		for i, v := range params {
			switch v.(type) {
			case string:
				if i == 0 {
					addr = v.(string)
				}
			case float64:
				if i == 1 {
					minblock = int64(v.(float64))
				} else if i == 2 {
					maxblock = int64(v.(float64))
				}
			}
		}
		addrhash, _, err := btcutil.DecodeAddress(addr)
		if err != nil {
			return err
		}

		// FetchHeightRange may not return a complete list of block shas for
		// the given range, so fetch range as many times as necessary.
		for {
			blkshalist, err := s.server.db.FetchHeightRange(minblock, maxblock)
			if err != nil {
				return err
			}
			if len(blkshalist) == 0 {
				break
			}

			for i := range blkshalist {
				blk, err := s.server.db.FetchBlockBySha(&blkshalist[i])
				if err != nil {
					return err
				}
				txShaList, err := blk.TxShas()
				if err != nil {
					return err
				}
				txList := s.server.db.FetchTxByShaList(txShaList)
				for _, txReply := range txList {
					if txReply.Err != nil || txReply.Tx == nil {
						continue
					}
					for _, txout := range txReply.Tx.TxOut {
						_, txaddrhash, err := btcscript.ScriptToAddrHash(txout.PkScript)
						if err != nil {
							return err
						}
						if !bytes.Equal(addrhash, txaddrhash) {
							reply := btcjson.Reply{
								Result: txReply.Sha,
								Error:  nil,
								Id:     &message.Id,
							}
							replychan <- &reply
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

		rawReply = btcjson.Reply{
			Result: nil,
			Error:  nil,
			Id:     &message.Id,
		}

	case "notifynewtxs":
		params, ok := message.Params.([]interface{})
		if !ok || len(params) != 1 {
			jsonError := btcjson.Error{
				Code:    -32602,
				Message: "Invalid parameters",
			}
			rawReply = btcjson.Reply{
				Result: nil,
				Error:  &jsonError,
				Id:     &message.Id,
			}
			return ErrBadParamsField
		}
		addr, ok := params[0].(string)
		if !ok {
			jsonError := btcjson.Error{
				Code:    -32602,
				Message: "Invalid parameters",
			}
			rawReply = btcjson.Reply{
				Result: nil,
				Error:  &jsonError,
				Id:     &message.Id,
			}
			return ErrBadParamsField
		}
		addrhash, _, err := btcutil.DecodeAddress(addr)
		if err != nil {
			jsonError := btcjson.Error{
				Code:    -32602,
				Message: "Cannot decode address",
			}
			rawReply = btcjson.Reply{
				Result: nil,
				Error:  &jsonError,
				Id:     &message.Id,
			}
			return ErrBadParamsField
		}
		var hash addressHash
		copy(hash[:], addrhash)
		s.ws.requests.AddTxRequest(walletNotification, hash, message.Id)

		rawReply = btcjson.Reply{
			Result: nil,
			Error:  nil,
			Id:     &message.Id,
		}

	case "notifyspent":
		params, ok := message.Params.([]interface{})
		if !ok || len(params) != 2 {
			jsonError := btcjson.Error{
				Code:    -32602,
				Message: "Invalid parameters",
			}
			rawReply = btcjson.Reply{
				Result: nil,
				Error:  &jsonError,
				Id:     &message.Id,
			}
			return ErrBadParamsField
		}
		hashBE, ok1 := params[0].(string)
		index, ok2 := params[1].(float64)
		if !ok1 || !ok2 {
			jsonError := btcjson.Error{
				Code:    -32602,
				Message: "Invalid parameters",
			}
			rawReply = btcjson.Reply{
				Result: nil,
				Error:  &jsonError,
				Id:     &message.Id,
			}
			return ErrBadParamsField
		}
		hash, err := btcwire.NewShaHashFromStr(hashBE)
		if err != nil {
			jsonError := btcjson.Error{
				Code:    -32602,
				Message: "Hash string cannot be parsed.",
			}
			rawReply = btcjson.Reply{
				Result: nil,
				Error:  &jsonError,
				Id:     &message.Id,
			}
			return ErrBadParamsField
		}
		op := btcwire.NewOutPoint(hash, uint32(index))
		s.ws.requests.AddSpentRequest(walletNotification, op, message.Id)

		rawReply = btcjson.Reply{
			Result: nil,
			Error:  nil,
			Id:     &message.Id,
		}

	default:
		jsonError := btcjson.Error{
			Code:    -32601,
			Message: "Method not found",
		}
		rawReply = btcjson.Reply{
			Result: nil,
			Error:  &jsonError,
			Id:     &message.Id,
		}
	}
	return ErrMethodNotImplemented
}

// getDifficultyRatio returns the proof-of-work difficulty as a multiple of the
// minimum difficulty using the passed bits field from the header of a block.
func getDifficultyRatio(bits uint32) float64 {
	// The minimum difficulty is the max possible proof-of-work limit bits
	// converted back to a number.  Note this is not the same as the the
	// proof of work limit directly because the block difficulty is encoded
	// in a block with the compact form which loses precision.
	max := btcchain.CompactToBig(activeNetParams.powLimitBits)
	target := btcchain.CompactToBig(bits)

	difficulty := new(big.Rat).SetFrac(max, target)
	outString := difficulty.FloatString(2)
	diff, err := strconv.ParseFloat(outString, 64)
	if err != nil {
		log.Errorf("RPCS: Cannot get difficulty: %v", err)
		return 0
	}
	return diff
}

// AddWalletListener adds a channel to listen for new messages from a
// wallet.
func (s *rpcServer) AddWalletListener(c chan []byte) {
	s.ws.addWalletListener <- c
}

// RemoveWalletListener removes a wallet listener channel.
func (s *rpcServer) RemoveWalletListener(c chan []byte) {
	s.ws.removeWalletListener <- c
}

// walletListenerDuplicator listens for new wallet listener channels
// and duplicates messages sent to walletNotificationMaster to all
// connected listeners.
func (s *rpcServer) walletListenerDuplicator() {
	// walletListeners is a map holding each currently connected wallet
	// listener as the key.  The value is ignored, as this is only used as
	// a set.
	walletListeners := make(map[chan []byte]bool)

	// Don't want to add or delete a wallet listener while iterating
	// through each to propigate to every attached wallet.  Use a mutex to
	// prevent this.
	var mtx sync.Mutex

	// Check for listener channels to add or remove from set.
	go func() {
		for {
			select {
			case c := <-s.ws.addWalletListener:
				mtx.Lock()
				walletListeners[c] = true
				mtx.Unlock()

			case c := <-s.ws.removeWalletListener:
				mtx.Lock()
				delete(walletListeners, c)
				mtx.Unlock()

			case <-s.quit:
				return
			}
		}
	}()

	// Duplicate all messages sent across walletNotificationMaster to each
	// listening wallet.
	for {
		select {
		case ntfn := <-s.ws.walletNotificationMaster:
			mtx.Lock()
			for c := range walletListeners {
				c <- ntfn
			}
			mtx.Unlock()

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
func (s *rpcServer) websocketJSONHandler(walletNotification chan []byte, msg []byte) {
	s.wg.Add(1)
	reply, err := jsonRead(msg, s)
	s.wg.Done()

	if err != ErrMethodNotImplemented {
		replyBytes, err := json.Marshal(reply)
		if err != nil {
			log.Errorf("RPCS: Error marshalling reply: %v", err)
		}
		walletNotification <- replyBytes
		return
	}

	// Try websocket extensions
	replychan := make(chan *btcjson.Reply)
	go func() {
		for {
			select {
			case reply, ok := <-replychan:
				if !ok {
					// no more replies expected.
					return
				}
				if reply == nil {
					continue
				}
				log.Tracef("[RPCS] reply: %v", *reply)
				replyBytes, err := json.Marshal(reply)
				if err != nil {
					log.Errorf("RPCS: Error Marshalling reply: %v", err)
					return
				}
				walletNotification <- replyBytes

			case <-s.quit:
				return
			}
		}
	}()

	if err == ErrMethodNotImplemented {
		// Try websocket extensions
		s.wg.Add(1)
		err = jsonWSRead(walletNotification, replychan, msg, s)
		s.wg.Done()
	}
	if err != nil && err != ErrMethodNotImplemented {
		log.Error(err)
	}
}

// NotifyBlockConnected creates and marshalls a JSON message to notify
// of a new block connected to the main chain.  The notification is sent
// to each connected wallet.
func (s *rpcServer) NotifyBlockConnected(block *btcutil.Block) {
	var id interface{} = "btcd:blockconnected"
	hash, err := block.Sha()
	if err != nil {
		log.Error("Bad block; connected block notification dropped.")
		return
	}
	ntfn := btcjson.Reply{
		Result: struct {
			Hash   string `json:"hash"`
			Height int64  `json:"height"`
		}{
			Hash:   hash.String(),
			Height: block.Height(),
		},
		Id: &id,
	}
	m, _ := json.Marshal(ntfn)
	s.ws.walletNotificationMaster <- m
}

// NotifyBlockDisconnected creates and marshalls a JSON message to notify
// of a new block disconnected from the main chain.  The notification is sent
// to each connected wallet.
func (s *rpcServer) NotifyBlockDisconnected(block *btcutil.Block) {
	var id interface{} = "btcd:blockdisconnected"
	hash, err := block.Sha()
	if err != nil {
		log.Error("Bad block; connected block notification dropped.")
		return
	}
	ntfn := btcjson.Reply{
		Result: struct {
			Hash   string `json:"hash"`
			Height int64  `json:"height"`
		}{
			Hash:   hash.String(),
			Height: block.Height(),
		},
		Id: &id,
	}
	m, _ := json.Marshal(ntfn)
	s.ws.walletNotificationMaster <- m
}

// NotifyNewTxListeners creates and marshals a JSON message to notify wallets
// of new transactions (with both spent and unspent outputs) for a watched
// address.
func (s *rpcServer) NotifyNewTxListeners(db btcdb.Db, block *btcutil.Block) {
	txShaList, err := block.TxShas()
	if err != nil {
		log.Error("Bad block; All notifications for block dropped.")
		return
	}
	txList := db.FetchTxByShaList(txShaList)
	for _, txReply := range txList {
		if txReply.Err == nil && txReply.Tx != nil {
			go s.newBlockNotifyCheckTxIn(txReply.Tx.TxIn)
			go s.newBlockNotifyCheckTxOut(db, block, txReply)
		}
	}
}

// newBlockNotifyCheckTxIn is a helper function to iterate through
// each transaction input of a new block and perform any checks and
// notify listening frontends when necessary.
func (s *rpcServer) newBlockNotifyCheckTxIn(txins []*btcwire.TxIn) {
	for wltNtfn, cxt := range s.ws.requests.m {
		for _, txin := range txins {
			for op, id := range cxt.spentRequests {
				if txin.PreviousOutpoint != op {
					continue
				}

				reply := &btcjson.Reply{
					Result: struct {
						TxHash string `json:"txhash"`
						Index  uint32 `json:"index"`
					}{
						TxHash: op.Hash.String(),
						Index:  uint32(op.Index),
					},
					Error: nil,
					Id:    &id,
				}
				replyBytes, err := json.Marshal(reply)
				if err != nil {
					log.Errorf("RPCS: Unable to marshal spent notification: %v", err)
					continue
				}
				wltNtfn <- replyBytes
				s.ws.requests.RemoveSpentRequest(wltNtfn, &op)
			}
		}
	}
}

// newBlockNotifyCheckTxOut is a helper function to iterate through
// each transaction output of a new block and perform any checks and
// notify listening frontends when necessary.
func (s *rpcServer) newBlockNotifyCheckTxOut(db btcdb.Db, block *btcutil.Block, tx *btcdb.TxListReply) {
	for wltNtfn, cxt := range s.ws.requests.m {
		for i, txout := range tx.Tx.TxOut {
			_, txaddrhash, err := btcscript.ScriptToAddrHash(txout.PkScript)
			if err != nil {
				log.Debug("Error getting payment address from tx; dropping any Tx notifications.")
				break
			}
			for addr, id := range cxt.txRequests {
				if !bytes.Equal(addr[:], txaddrhash) {
					continue
				}
				blkhash, err := block.Sha()
				if err != nil {
					log.Error("Error getting block sha; dropping Tx notification.")
					break
				}
				txaddr, err := btcutil.EncodeAddress(txaddrhash, s.server.btcnet)
				if err != nil {
					log.Error("Error encoding address; dropping Tx notification.")
					break
				}
				reply := &btcjson.Reply{
					Result: struct {
						Sender    string `json:"sender"`
						Receiver  string `json:"receiver"`
						BlockHash string `json:"blockhash"`
						Height    int64  `json:"height"`
						TxHash    string `json:"txhash"`
						Index     uint32 `json:"index"`
						Amount    int64  `json:"amount"`
						PkScript  string `json:"pkscript"`
						Spent     bool   `json:"spent"`
					}{
						Sender:    "Unknown", // TODO(jrick)
						Receiver:  txaddr,
						BlockHash: blkhash.String(),
						Height:    block.Height(),
						TxHash:    tx.Sha.String(),
						Index:     uint32(i),
						Amount:    txout.Value,
						PkScript:  btcutil.Base58Encode(txout.PkScript),
						Spent:     tx.TxSpent[i],
					},
					Error: nil,
					Id:    &id,
				}
				replyBytes, err := json.Marshal(reply)
				if err != nil {
					log.Errorf("RPCS: Unable to marshal tx notification: %v", err)
					continue
				}
				wltNtfn <- replyBytes
			}
		}
	}
}
