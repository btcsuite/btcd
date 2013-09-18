// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"github.com/conformal/btcchain"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcwire"
	"github.com/davecgh/go-spew/spew"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// rpcServer holds the items the rpc server may need to access (config,
// shutdown, main server, etc.)
type rpcServer struct {
	started   bool
	shutdown  bool
	server    *server
	wg        sync.WaitGroup
	rpcport   string
	username  string
	password  string
	listeners []net.Listener
}

// Start is used by server.go to start the rpc listener.
func (s *rpcServer) Start() {
	if s.started {
		return
	}

	log.Trace("[RPCS] Starting RPC server")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		jsonRPCRead(w, r, s)
	})
	httpServer := &http.Server{}
	for _, listener := range s.listeners {
		s.wg.Add(1)
		go func(listener net.Listener) {
			log.Infof("[RPCS] RPC server listening on %s", listener.Addr())
			httpServer.Serve(listener)
			log.Tracef("[RPCS] RPC listener done for %s", listener.Addr())
			s.wg.Done()
		}(listener)
	}
	s.started = true
}

// Stop is used by server.go to stop the rpc listener.
func (s *rpcServer) Stop() error {
	if s.shutdown {
		log.Infof("[RPCS] RPC server is already in the process of shutting down")
		return nil
	}
	log.Warnf("[RPCS] RPC server shutting down")
	for _, listener := range s.listeners {
		err := listener.Close()
		if err != nil {
			log.Errorf("[RPCS] Problem shutting down rpc: %v", err)
			return err
		}
	}
	log.Infof("[RPCS] RPC server shutdown complete")
	s.wg.Wait()
	s.shutdown = true
	return nil
}

// newRPCServer returns a new instance of the rpcServer struct.
func newRPCServer(s *server) (*rpcServer, error) {
	rpc := rpcServer{
		server: s,
	}
	// Get values from config
	rpc.rpcport = cfg.RPCPort
	rpc.username = cfg.RPCUser
	rpc.password = cfg.RPCPass

	// IPv4 listener.
	var listeners []net.Listener
	listenAddr4 := net.JoinHostPort("127.0.0.1", rpc.rpcport)
	listener4, err := net.Listen("tcp4", listenAddr4)
	if err != nil {
		log.Errorf("[RPCS] Couldn't create listener: %v", err)
		return nil, err
	}
	listeners = append(listeners, listener4)

	// IPv6 listener.
	listenAddr6 := net.JoinHostPort("::1", rpc.rpcport)
	listener6, err := net.Listen("tcp6", listenAddr6)
	if err != nil {
		log.Errorf("[RPCS] Couldn't create listener: %v", err)
		return nil, err
	}
	listeners = append(listeners, listener6)

	rpc.listeners = listeners
	return &rpc, err
}

// jsonRPCRead is the main function that handles reading messages, getting
// the data the message requests, and writing the reply.
func jsonRPCRead(w http.ResponseWriter, r *http.Request, s *rpcServer) {
	_ = spew.Dump
	r.Close = true
	if s.shutdown == true {
		return
	}
	var rawReply btcjson.Reply
	body, err := btcjson.GetRaw(r.Body)
	if err != nil {
		log.Errorf("[RPCS] Error getting json message: %v", err)
		return
	}
	var message btcjson.Message
	err = json.Unmarshal(body, &message)
	if err != nil {
		log.Errorf("[RPCS] Error unmarshalling json message: %v", err)
		jsonError := btcjson.Error{
			Code:    -32700,
			Message: "Parse error",
		}

		rawReply = btcjson.Reply{
			Result: nil,
			Error:  &jsonError,
			Id:     nil,
		}
		log.Tracef("[RPCS] reply: %v", rawReply)
		msg, err := btcjson.MarshallAndSend(rawReply, w)
		if err != nil {
			log.Errorf(msg)
			return
		}
		log.Debugf(msg)
		return

	}
	log.Tracef("[RPCS] received: %v", message)

	// Deal with commands
	switch message.Method {
	case "stop":
		rawReply = btcjson.Reply{
			Result: "btcd stopping.",
			Error:  nil,
			Id:     &message.Id,
		}
		s.server.Stop()
	case "getblockcount":
		_, maxidx, _ := s.server.db.NewestSha()
		rawReply = btcjson.Reply{
			Result: maxidx,
			Error:  nil,
			Id:     &message.Id,
		}
	// btcd does not do mining so we can hardcode replies here.
	case "getgenerate":
		rawReply = btcjson.Reply{
			Result: false,
			Error:  nil,
			Id:     &message.Id,
		}
	case "setgenerate":
		rawReply = btcjson.Reply{
			Result: nil,
			Error:  nil,
			Id:     &message.Id,
		}
	case "gethashespersec":
		rawReply = btcjson.Reply{
			Result: 0,
			Error:  nil,
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
		sha, err := s.server.db.FetchBlockShaByHeight(int64(idx))
		if err != nil {
			log.Errorf("[RCPS] Error getting block: %v", err)
			jsonError := btcjson.Error{
				Code:    -1,
				Message: "Block number out of range.",
			}

			rawReply = btcjson.Reply{
				Result: nil,
				Error:  &jsonError,
				Id:     &message.Id,
			}
			log.Tracef("[RPCS] reply: %v", rawReply)
			break

		}

		rawReply = btcjson.Reply{
			Result: sha.String(),
			Error:  nil,
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
		sha, err := btcwire.NewShaHashFromStr(hash)
		if err != nil {
			log.Errorf("[RPCS] Error generating sha: %v", err)
			jsonError := btcjson.Error{
				Code:    -5,
				Message: "Block not found",
			}

			rawReply = btcjson.Reply{
				Result: nil,
				Error:  &jsonError,
				Id:     &message.Id,
			}
			log.Tracef("[RPCS] reply: %v", rawReply)
			break
		}
		blk, err := s.server.db.FetchBlockBySha(sha)
		if err != nil {
			log.Errorf("[RPCS] Error fetching sha: %v", err)
			jsonError := btcjson.Error{
				Code:    -5,
				Message: "Block not found",
			}

			rawReply = btcjson.Reply{
				Result: nil,
				Error:  &jsonError,
				Id:     &message.Id,
			}
			log.Tracef("[RPCS] reply: %v", rawReply)
			break
		}
		idx := blk.Height()
		buf, err := blk.Bytes()
		if err != nil {
			log.Errorf("[RPCS] Error fetching block: %v", err)
			jsonError := btcjson.Error{
				Code:    -5,
				Message: "Block not found",
			}

			rawReply = btcjson.Reply{
				Result: nil,
				Error:  &jsonError,
				Id:     &message.Id,
			}
			log.Tracef("[RPCS] reply: %v", rawReply)
			break
		}

		txList, _ := blk.TxShas()

		txNames := make([]string, len(txList))
		for i, v := range txList {
			txNames[i] = v.String()
		}

		_, maxidx, err := s.server.db.NewestSha()
		if err != nil {
			log.Errorf("[RPCS] Cannot get newest sha: %v", err)
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
			shaNext, err := s.server.db.FetchBlockShaByHeight(int64(idx + 1))
			if err != nil {
				log.Errorf("[RPCS] No next block: %v", err)
			} else {
				blockReply.NextHash = shaNext.String()
			}
		}

		rawReply = btcjson.Reply{
			Result: blockReply,
			Error:  nil,
			Id:     &message.Id,
		}
	case "getrawtransaction":
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

		if int(verbose) != 1 {
			// Don't return details
			// not used yet
		} else {
			txSha, _ := btcwire.NewShaHashFromStr(tx)
			var txS *btcwire.MsgTx
			txS, _, blksha, err := s.server.db.FetchTxBySha(txSha)
			if err != nil {
				log.Errorf("[RPCS] Error fetching tx: %v", err)
				jsonError := btcjson.Error{
					Code:    -5,
					Message: "No information available about transaction",
				}

				rawReply = btcjson.Reply{
					Result: nil,
					Error:  &jsonError,
					Id:     &message.Id,
				}
				log.Tracef("[RPCS] reply: %v", rawReply)
				break
			}
			blk, err := s.server.db.FetchBlockBySha(blksha)
			if err != nil {
				log.Errorf("[RPCS] Error fetching sha: %v", err)
				jsonError := btcjson.Error{
					Code:    -5,
					Message: "Block not found",
				}

				rawReply = btcjson.Reply{
					Result: nil,
					Error:  &jsonError,
					Id:     &message.Id,
				}
				log.Tracef("[RPCS] reply: %v", rawReply)
				break
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
				_, addr, err := btcscript.ScriptToAddress(v.PkScript)
				if err != nil {
					log.Errorf("[RPCS] Error getting address for %v: %v", txSha, err)
				} else {
					addrList := make([]string, 1)
					addrList[0] = addr
					voutList[i].ScriptPubKey.Addresses = addrList
				}
			}

			_, maxidx, err := s.server.db.NewestSha()
			if err != nil {
				log.Errorf("[RPCS] Cannot get newest sha: %v", err)
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
			rawReply = btcjson.Reply{
				Result: txReply,
				Error:  nil,
				Id:     &message.Id,
			}
		}
	case "decoderawtransaction":
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
		spew.Dump(hash)
		txReply := btcjson.TxRawDecodeResult{}
		rawReply = btcjson.Reply{
			Result: txReply,
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

	msg, err := btcjson.MarshallAndSend(rawReply, w)
	if err != nil {
		log.Errorf(msg)
		return
	}
	log.Debugf(msg)
	return
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
		log.Errorf("[RPCS] Cannot get difficulty: %v", err)
		return 0
	}
	return diff
}
