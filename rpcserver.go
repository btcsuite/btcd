// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"code.google.com/p/go.net/websocket"
	"container/list"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/conformal/btcchain"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Errors
var (
	// ErrBadParamsField describes an error where the parameters JSON
	// field cannot be properly parsed.
	ErrBadParamsField = errors.New("bad params field")
)

type commandHandler func(*rpcServer, btcjson.Cmd) (interface{}, error)

// handlers maps RPC command strings to appropriate handler functions.
var rpcHandlers = map[string]commandHandler{
	"addmultisigaddress":     handleAskWallet,
	"addnode":                handleAddNode,
	"backupwallet":           handleAskWallet,
	"createmultisig":         handleAskWallet,
	"createrawtransaction":   handleCreateRawTransaction,
	"debuglevel":             handleDebugLevel,
	"decoderawtransaction":   handleDecodeRawTransaction,
	"decodescript":           handleDecodeScript,
	"dumpprivkey":            handleAskWallet,
	"dumpwallet":             handleAskWallet,
	"encryptwallet":          handleAskWallet,
	"getaccount":             handleAskWallet,
	"getaccountaddress":      handleAskWallet,
	"getaddednodeinfo":       handleUnimplemented,
	"getaddressesbyaccount":  handleAskWallet,
	"getbalance":             handleAskWallet,
	"getbestblockhash":       handleGetBestBlockHash,
	"getblock":               handleGetBlock,
	"getblockcount":          handleGetBlockCount,
	"getblockhash":           handleGetBlockHash,
	"getblocktemplate":       handleUnimplemented,
	"getconnectioncount":     handleGetConnectionCount,
	"getdifficulty":          handleGetDifficulty,
	"getgenerate":            handleGetGenerate,
	"gethashespersec":        handleGetHashesPerSec,
	"getinfo":                handleUnimplemented,
	"getmininginfo":          handleUnimplemented,
	"getnettotals":           handleUnimplemented,
	"getnetworkhashps":       handleUnimplemented,
	"getnewaddress":          handleUnimplemented,
	"getpeerinfo":            handleGetPeerInfo,
	"getrawchangeaddress":    handleAskWallet,
	"getrawmempool":          handleGetRawMempool,
	"getrawtransaction":      handleGetRawTransaction,
	"getreceivedbyaccount":   handleAskWallet,
	"getreceivedbyaddress":   handleAskWallet,
	"gettransaction":         handleAskWallet,
	"gettxout":               handleAskWallet,
	"gettxoutsetinfo":        handleAskWallet,
	"getwork":                handleUnimplemented,
	"help":                   handleUnimplemented,
	"importprivkey":          handleAskWallet,
	"importwallet":           handleAskWallet,
	"keypoolrefill":          handleAskWallet,
	"listaccounts":           handleAskWallet,
	"listaddressgroupings":   handleAskWallet,
	"listlockunspent":        handleAskWallet,
	"listreceivedbyaccount":  handleAskWallet,
	"listreceivedbyaddress":  handleAskWallet,
	"listsinceblock":         handleAskWallet,
	"listtransactions":       handleAskWallet,
	"listunspent":            handleAskWallet,
	"lockunspent":            handleAskWallet,
	"move":                   handleAskWallet,
	"ping":                   handleUnimplemented,
	"sendfrom":               handleAskWallet,
	"sendmany":               handleAskWallet,
	"sendrawtransaction":     handleSendRawTransaction,
	"sendtoaddress":          handleAskWallet,
	"setaccount":             handleAskWallet,
	"setgenerate":            handleSetGenerate,
	"settxfee":               handleAskWallet,
	"signmessage":            handleAskWallet,
	"signrawtransaction":     handleAskWallet,
	"stop":                   handleStop,
	"submitblock":            handleUnimplemented,
	"validateaddress":        handleAskWallet,
	"verifychain":            handleVerifyChain,
	"verifymessage":          handleAskWallet,
	"walletlock":             handleAskWallet,
	"walletpassphrase":       handleAskWallet,
	"walletpassphrasechange": handleAskWallet,
}

// rpcServer holds the items the rpc server may need to access (config,
// shutdown, main server, etc.)
type rpcServer struct {
	started   int32
	shutdown  int32
	server    *server
	authsha   [sha256.Size]byte
	ws        wsContext
	wg        sync.WaitGroup
	listeners []net.Listener
	quit      chan int
}

// Start is used by server.go to start the rpc listener.
func (s *rpcServer) Start() {
	if atomic.AddInt32(&s.started, 1) != 1 {
		return
	}

	rpcsLog.Trace("Starting RPC server")
	rpcServeMux := http.NewServeMux()
	httpServer := &http.Server{Handler: rpcServeMux}
	rpcServeMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := s.checkAuth(r); err != nil {
			jsonAuthFail(w, r, s)
			return
		}
		w.Header().Set("Connection", "close")
		jsonRPCRead(w, r, s)
	})
	go s.walletListenerDuplicator()
	rpcServeMux.HandleFunc("/wallet", func(w http.ResponseWriter, r *http.Request) {
		if err := s.checkAuth(r); err != nil {
			http.Error(w, "401 Unauthorized.", http.StatusUnauthorized)
			return
		}
		websocket.Handler(s.walletReqsNotifications).ServeHTTP(w, r)
	})

	for _, listener := range s.listeners {
		s.wg.Add(1)
		go func(listener net.Listener) {
			rpcsLog.Infof("RPC server listening on %s", listener.Addr())
			httpServer.Serve(listener)
			rpcsLog.Tracef("RPC listener done for %s", listener.Addr())
			s.wg.Done()
		}(listener)
	}
}

// checkAuth checks the HTTP Basic authentication supplied by a wallet
// or RPC client in the HTTP request r.  If the supplied authentication
// does not match the username and password expected, a non-nil error is
// returned.
//
// This check is time-constant.
func (s *rpcServer) checkAuth(r *http.Request) error {
	authhdr := r.Header["Authorization"]
	if len(authhdr) <= 0 {
		rpcsLog.Warnf("Auth failure.")
		return errors.New("auth failure")
	}

	authsha := sha256.Sum256([]byte(authhdr[0]))
	cmp := subtle.ConstantTimeCompare(authsha[:], s.authsha[:])
	if cmp != 1 {
		rpcsLog.Warnf("Auth failure.")
		return errors.New("auth failure")
	}
	return nil
}

// Stop is used by server.go to stop the rpc listener.
func (s *rpcServer) Stop() error {
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		rpcsLog.Infof("RPC server is already in the process of shutting down")
		return nil
	}
	rpcsLog.Warnf("RPC server shutting down")
	for _, listener := range s.listeners {
		err := listener.Close()
		if err != nil {
			rpcsLog.Errorf("Problem shutting down rpc: %v", err)
			return err
		}
	}
	rpcsLog.Infof("RPC server shutdown complete")
	s.wg.Wait()
	close(s.quit)
	return nil
}

// genCertPair generates a key/cert pair to the paths provided.
func genCertPair(certFile, keyFile string) error {
	rpcsLog.Infof("Generating TLS certificates...")

	org := "btcd autogenerated cert"
	validUntil := time.Now().Add(10 * 365 * 24 * time.Hour)
	cert, key, err := btcutil.NewTLSCertPair(org, validUntil, nil)
	if err != nil {
		return err
	}

	// Write cert and key files.
	if err = ioutil.WriteFile(certFile, cert, 0666); err != nil {
		return err
	}
	if err = ioutil.WriteFile(keyFile, key, 0600); err != nil {
		os.Remove(certFile)
		return err
	}

	rpcsLog.Infof("Done generating TLS certificates")
	return nil
}

// newRPCServer returns a new instance of the rpcServer struct.
func newRPCServer(listenAddrs []string, s *server) (*rpcServer, error) {
	login := cfg.RPCUser + ":" + cfg.RPCPass
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(login))
	rpc := rpcServer{
		authsha: sha256.Sum256([]byte(auth)),
		server:  s,
		quit:    make(chan int),
	}

	// initialize memory for websocket connections
	rpc.ws.connections = make(map[walletChan]*requestContexts)
	rpc.ws.walletNotificationMaster = make(chan []byte)
	rpc.ws.txNotifications = make(map[string]*list.List)
	rpc.ws.spentNotifications = make(map[btcwire.OutPoint]*list.List)
	rpc.ws.minedTxNotifications = make(map[btcwire.ShaHash]*list.List)

	// check for existence of cert file and key file
	if !fileExists(cfg.RPCKey) && !fileExists(cfg.RPCCert) {
		// if both files do not exist, we generate them.
		err := genCertPair(cfg.RPCCert, cfg.RPCKey)
		if err != nil {
			return nil, err
		}
	}
	keypair, err := tls.LoadX509KeyPair(cfg.RPCCert, cfg.RPCKey)
	if err != nil {
		return nil, err
	}

	tlsConfig := tls.Config{
		Certificates: []tls.Certificate{keypair},
	}

	// TODO(oga) this code is similar to that in server, should be
	// factored into something shared.
	ipv4ListenAddrs, ipv6ListenAddrs, _, err := parseListeners(listenAddrs)
	if err != nil {
		return nil, err
	}
	listeners := make([]net.Listener, 0,
		len(ipv6ListenAddrs)+len(ipv4ListenAddrs))
	for _, addr := range ipv4ListenAddrs {
		var listener net.Listener
		listener, err = tls.Listen("tcp4", addr, &tlsConfig)
		if err != nil {
			rpcsLog.Warnf("Can't listen on %s: %v", addr,
				err)
			continue
		}
		listeners = append(listeners, listener)
	}

	for _, addr := range ipv6ListenAddrs {
		var listener net.Listener
		listener, err = tls.Listen("tcp6", addr, &tlsConfig)
		if err != nil {
			rpcsLog.Warnf("Can't listen on %s: %v", addr,
				err)
			continue
		}
		listeners = append(listeners, listener)
	}
	if len(listeners) == 0 {
		return nil, errors.New("RPCS: No valid listen address")
	}

	rpc.listeners = listeners

	return &rpc, err
}

// jsonAuthFail sends a message back to the client if the http auth is rejected.
func jsonAuthFail(w http.ResponseWriter, r *http.Request, s *rpcServer) {
	fmt.Fprint(w, "401 Unauthorized.\n")
}

// jsonRPCRead is the RPC wrapper around the jsonRead function to handle reading
// and responding to RPC messages.
func jsonRPCRead(w http.ResponseWriter, r *http.Request, s *rpcServer) {
	r.Close = true
	if atomic.LoadInt32(&s.shutdown) != 0 {
		return
	}
	body, err := btcjson.GetRaw(r.Body)
	if err != nil {
		rpcsLog.Errorf("Error getting json message: %v", err)
		return
	}

	var reply btcjson.Reply
	cmd, jsonErr := parseCmd(body)
	if cmd != nil {
		// Unmarshaling at least a valid JSON-RPC message succeeded.
		// Use the provided id for errors.
		id := cmd.Id()
		reply.Id = &id
	}
	if jsonErr != nil {
		reply.Error = jsonErr
	} else {
		reply = standardCmdReply(cmd, s)
	}

	rpcsLog.Tracef("reply: %v", reply)

	msg, err := btcjson.MarshallAndSend(reply, w)
	if err != nil {
		rpcsLog.Errorf(msg)
		return
	}
	rpcsLog.Debugf(msg)
}

// handleUnimplemented is a temporary handler for commands that we should
// support but do not.
func handleUnimplemented(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	return nil, btcjson.ErrUnimplemented
}

// handleAskWallet is the handler for commands that we do recognise as valid
// but that we can not answer correctly since it involves wallet state.
// These commands will be implemented in btcwallet.
func handleAskWallet(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	return nil, btcjson.ErrNoWallet
}

// handleAddNode handles addnode commands.
func handleAddNode(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.AddNodeCmd)

	addr := normalizeAddress(c.Addr, activeNetParams.peerPort)
	var err error
	switch c.SubCmd {
	case "add":
		err = s.server.AddAddr(addr, true)
	case "remove":
		err = s.server.RemoveAddr(addr)
	case "onetry":
		err = s.server.AddAddr(addr, false)
	default:
		err = errors.New("Invalid subcommand for addnode")
	}

	if err != nil {
		return nil, btcjson.Error{
			Code:    btcjson.ErrInternal.Code,
			Message: err.Error(),
		}
	}

	// no data returned unless an error.
	return nil, nil
}

// messageToHex serializes a message to the wire protocol encoding using the
// latest protocol version and returns a hex-encoded string of the result.
func messageToHex(msg btcwire.Message) (string, error) {
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, btcwire.ProtocolVersion)
	if err != nil {
		return "", btcjson.Error{
			Code:    btcjson.ErrInternal.Code,
			Message: err.Error(),
		}
	}
	return hex.EncodeToString(buf.Bytes()), nil
}

// handleCreateRawTransaction handles createrawtransaction commands.
func handleCreateRawTransaction(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.CreateRawTransactionCmd)

	// Add all transaction inputs to a new transaction after performing
	// some validty checks.
	mtx := btcwire.NewMsgTx()
	for _, input := range c.Inputs {
		txHash, err := btcwire.NewShaHashFromStr(input.Txid)
		if err != nil {
			return nil, btcjson.ErrDecodeHexString
		}

		if input.Vout < 0 {
			return nil, btcjson.Error{
				Code:    btcjson.ErrInvalidParameter.Code,
				Message: "Invalid parameter, vout must be positive",
			}
		}

		prevOut := btcwire.NewOutPoint(txHash, uint32(input.Vout))
		txIn := btcwire.NewTxIn(prevOut, []byte{})
		mtx.AddTxIn(txIn)
	}

	// Add all transaction outputs to the transaction after performing
	// some validty checks.
	for encodedAddr, amount := range c.Amounts {
		// Ensure amount is in the valid range for monetary amounts.
		if amount <= 0 || amount > btcutil.MaxSatoshi {
			return nil, btcjson.Error{
				Code:    btcjson.ErrType.Code,
				Message: "Invalid amount",
			}
		}

		// Decode the provided address.
		addr, err := btcutil.DecodeAddr(encodedAddr)
		if err != nil {
			return nil, btcjson.Error{
				Code: btcjson.ErrInvalidAddressOrKey.Code,
				Message: btcjson.ErrInvalidAddressOrKey.Message +
					": " + err.Error(),
			}
		}

		// Ensure the address is one of the supported types and that
		// the network encoded with the address matches the network the
		// server is currently on.
		net := s.server.btcnet
		switch addr := addr.(type) {
		case *btcutil.AddressPubKeyHash:
			net = addr.Net()
		case *btcutil.AddressScriptHash:
			net = addr.Net()
		default:
			return nil, btcjson.ErrInvalidAddressOrKey
		}
		if net != s.server.btcnet {
			return nil, btcjson.Error{
				Code: btcjson.ErrInvalidAddressOrKey.Code,
				Message: fmt.Sprintf("%s: %q",
					btcjson.ErrInvalidAddressOrKey.Message,
					encodedAddr),
			}
		}

		// Create a new script which pays to the provided address.
		pkScript, err := btcscript.PayToAddrScript(addr)
		if err != nil {
			return nil, btcjson.Error{
				Code:    btcjson.ErrInternal.Code,
				Message: err.Error(),
			}
		}

		txOut := btcwire.NewTxOut(amount, pkScript)
		mtx.AddTxOut(txOut)
	}

	// Return the serialized and hex-encoded transaction.
	mtxHex, err := messageToHex(mtx)
	if err != nil {
		return nil, err
	}
	return mtxHex, nil
}

// handleDebugLevel handles debuglevel commands.
func handleDebugLevel(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.DebugLevelCmd)

	// Special show command to list supported subsystems.
	if c.LevelSpec == "show" {
		return fmt.Sprintf("Supported subsystems %v",
			supportedSubsystems()), nil
	}

	err := parseAndSetDebugLevels(c.LevelSpec)
	if err != nil {
		return nil, btcjson.Error{
			Code:    btcjson.ErrInvalidParams.Code,
			Message: err.Error(),
		}
	}

	return "Done.", nil
}

// createVinList returns a slice of JSON objects for the inputs of the passed
// transaction.
func createVinList(mtx *btcwire.MsgTx) ([]btcjson.Vin, error) {
	tx := btcutil.NewTx(mtx)
	vinList := make([]btcjson.Vin, len(mtx.TxIn))
	for i, v := range mtx.TxIn {
		if btcchain.IsCoinBase(tx) {
			vinList[i].Coinbase = hex.EncodeToString(v.SignatureScript)
		} else {
			vinList[i].Txid = v.PreviousOutpoint.Hash.String()
			vinList[i].Vout = int(v.PreviousOutpoint.Index)

			disbuf, err := btcscript.DisasmString(v.SignatureScript)
			if err != nil {
				return nil, btcjson.Error{
					Code:    btcjson.ErrInternal.Code,
					Message: err.Error(),
				}
			}
			vinList[i].ScriptSig = new(btcjson.ScriptSig)
			vinList[i].ScriptSig.Asm = disbuf
			vinList[i].ScriptSig.Hex = hex.EncodeToString(v.SignatureScript)
		}
		vinList[i].Sequence = v.Sequence
	}

	return vinList, nil
}

// createVoutList returns a slice of JSON objects for the outputs of the passed
// transaction.
func createVoutList(mtx *btcwire.MsgTx, net btcwire.BitcoinNet) ([]btcjson.Vout, error) {
	voutList := make([]btcjson.Vout, len(mtx.TxOut))
	for i, v := range mtx.TxOut {
		voutList[i].N = i
		voutList[i].Value = float64(v.Value) / float64(btcutil.SatoshiPerBitcoin)

		disbuf, err := btcscript.DisasmString(v.PkScript)
		if err != nil {
			return nil, btcjson.Error{
				Code:    btcjson.ErrInternal.Code,
				Message: err.Error(),
			}
		}
		voutList[i].ScriptPubKey.Asm = disbuf
		voutList[i].ScriptPubKey.Hex = hex.EncodeToString(v.PkScript)

		// Ignore the error here since an error means the script
		// couldn't parse and there is no additional information about
		// it anyways.
		scriptClass, addrs, reqSigs, _ := btcscript.ExtractPkScriptAddrs(v.PkScript, net)
		voutList[i].ScriptPubKey.Type = scriptClass.String()
		voutList[i].ScriptPubKey.ReqSigs = reqSigs

		if addrs == nil {
			voutList[i].ScriptPubKey.Addresses = nil
		} else {
			voutList[i].ScriptPubKey.Addresses = make([]string, len(addrs))
			for j, addr := range addrs {
				voutList[i].ScriptPubKey.Addresses[j] = addr.EncodeAddress()
			}
		}
	}

	return voutList, nil
}

// handleDecodeRawTransaction handles decoderawtransaction commands.
func handleDecodeRawTransaction(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.DecodeRawTransactionCmd)

	// Deserialize the transaction.
	hexStr := c.HexTx
	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}
	serializedTx, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, btcjson.Error{
			Code: btcjson.ErrInvalidParameter.Code,
			Message: fmt.Sprintf("argument must be hexadecimal "+
				"string (not %q)", hexStr),
		}
	}
	var mtx btcwire.MsgTx
	err = mtx.Deserialize(bytes.NewBuffer(serializedTx))
	if err != nil {
		return nil, btcjson.Error{
			Code:    btcjson.ErrDeserialization.Code,
			Message: "TX decode failed",
		}
	}
	txSha, _ := mtx.TxSha()

	vin, err := createVinList(&mtx)
	if err != nil {
		return nil, err
	}
	vout, err := createVoutList(&mtx, s.server.btcnet)
	if err != nil {
		return nil, err
	}

	// Create and return the result.
	txReply := btcjson.TxRawDecodeResult{
		Txid:     txSha.String(),
		Version:  mtx.Version,
		Locktime: mtx.LockTime,
		Vin:      vin,
		Vout:     vout,
	}
	return txReply, nil
}

// handleDecodeScript handles decodescript commands.
func handleDecodeScript(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.DecodeScriptCmd)

	// Convert the hex script to bytes.
	script, err := hex.DecodeString(c.HexScript)
	if err != nil {
		return nil, btcjson.Error{
			Code: btcjson.ErrInvalidParameter.Code,
			Message: fmt.Sprintf("argument must be hexadecimal "+
				"string (not %q)", c.HexScript),
		}
	}

	// The disassembled string will contain [error] inline if the script
	// doesn't fully parse, so ignore the error here.
	disbuf, _ := btcscript.DisasmString(script)

	// Get information about the script.
	// Ignore the error here since an error means the script couldn't parse
	// and there is no additinal information about it anyways.
	net := s.server.btcnet
	scriptClass, addrs, reqSigs, _ := btcscript.ExtractPkScriptAddrs(script, net)
	addresses := make([]string, len(addrs))
	for i, addr := range addrs {
		addresses[i] = addr.EncodeAddress()
	}

	// Convert the script itself to a pay-to-script-hash address.
	p2sh, err := btcutil.NewAddressScriptHash(script, net)
	if err != nil {
		return nil, btcjson.Error{
			Code:    btcjson.ErrInternal.Code,
			Message: err.Error(),
		}
	}

	// Generate and return the reply.
	reply := btcjson.DecodeScriptResult{
		Asm:       disbuf,
		ReqSigs:   reqSigs,
		Type:      scriptClass.String(),
		Addresses: addresses,
		P2sh:      p2sh.EncodeAddress(),
	}
	return reply, nil
}

// handleGetBestBlockHash implements the getbestblockhash command.
func handleGetBestBlockHash(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	sha, _, err := s.server.db.NewestSha()
	if err != nil {
		rpcsLog.Errorf("Error getting newest sha: %v", err)
		return nil, btcjson.ErrBestBlockHash
	}

	return sha.String(), nil
}

// handleGetBlock implements the getblock command.
func handleGetBlock(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.GetBlockCmd)
	sha, err := btcwire.NewShaHashFromStr(c.Hash)
	if err != nil {
		rpcsLog.Errorf("Error generating sha: %v", err)
		return nil, btcjson.ErrBlockNotFound
	}
	blk, err := s.server.db.FetchBlockBySha(sha)
	if err != nil {
		rpcsLog.Errorf("Error fetching sha: %v", err)
		return nil, btcjson.ErrBlockNotFound
	}

	// When the verbose flag isn't set, simply return the network-serialized
	// block as a hex-encoded string.
	if !c.Verbose {
		blkHex, err := messageToHex(blk.MsgBlock())
		if err != nil {
			return nil, err
		}
		return blkHex, nil
	}

	// The verbose flag is set, so generate the JSON object and return it.
	buf, err := blk.Bytes()
	if err != nil {
		rpcsLog.Errorf("Error fetching block: %v", err)
		return nil, btcjson.Error{
			Code:    btcjson.ErrInternal.Code,
			Message: err.Error(),
		}
	}
	idx := blk.Height()
	_, maxidx, err := s.server.db.NewestSha()
	if err != nil {
		rpcsLog.Errorf("Cannot get newest sha: %v", err)
		return nil, btcjson.ErrBlockNotFound
	}

	blockHeader := &blk.MsgBlock().Header
	blockReply := btcjson.BlockResult{
		Hash:          c.Hash,
		Version:       blockHeader.Version,
		MerkleRoot:    blockHeader.MerkleRoot.String(),
		PreviousHash:  blockHeader.PrevBlock.String(),
		Nonce:         blockHeader.Nonce,
		Time:          blockHeader.Timestamp.Unix(),
		Confirmations: uint64(1 + maxidx - idx),
		Height:        idx,
		Size:          len(buf),
		Bits:          strconv.FormatInt(int64(blockHeader.Bits), 16),
		Difficulty:    getDifficultyRatio(blockHeader.Bits),
	}

	if !c.VerboseTx {
		txList, _ := blk.TxShas()

		txNames := make([]string, len(txList))
		for i, v := range txList {
			txNames[i] = v.String()
		}

		blockReply.Tx = txNames
	} else {
		txns := blk.Transactions()
		rawTxns := make([]btcjson.TxRawResult, len(txns))
		for i, tx := range txns {
			txSha := tx.Sha().String()
			mtx := tx.MsgTx()

			rawTxn, err := createTxRawResult(s.server.btcnet, txSha,
				mtx, blk, maxidx, sha)
			if err != nil {
				rpcsLog.Errorf("Cannot create TxRawResult for "+
					"transaction %s: %v", txSha, err)
				return nil, err
			}
			rawTxns[i] = *rawTxn
		}
		blockReply.RawTx = rawTxns
	}

	// Get next block unless we are already at the top.
	if idx < maxidx {
		var shaNext *btcwire.ShaHash
		shaNext, err = s.server.db.FetchBlockShaByHeight(int64(idx + 1))
		if err != nil {
			rpcsLog.Errorf("No next block: %v", err)
			return nil, btcjson.ErrBlockNotFound
		}
		blockReply.NextHash = shaNext.String()
	}

	return blockReply, nil
}

// handleGetBlockCount implements the getblockcount command.
func handleGetBlockCount(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	_, maxidx, err := s.server.db.NewestSha()
	if err != nil {
		rpcsLog.Errorf("Error getting newest sha: %v", err)
		return nil, btcjson.ErrBlockCount
	}

	return maxidx, nil
}

// handleGetBlockHash implements the getblockhash command.
func handleGetBlockHash(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.GetBlockHashCmd)
	sha, err := s.server.db.FetchBlockShaByHeight(c.Index)
	if err != nil {
		rpcsLog.Errorf("Error getting block: %v", err)
		return nil, btcjson.ErrOutOfRange
	}

	return sha.String(), nil
}

// handleGetConnectionCount implements the getconnectioncount command.
func handleGetConnectionCount(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	return s.server.ConnectedCount(), nil
}

// handleGetDifficulty implements the getdifficulty command.
func handleGetDifficulty(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	sha, _, err := s.server.db.NewestSha()
	if err != nil {
		rpcsLog.Errorf("Error getting sha: %v", err)
		return nil, btcjson.ErrDifficulty
	}
	blk, err := s.server.db.FetchBlockBySha(sha)
	if err != nil {
		rpcsLog.Errorf("Error getting block: %v", err)
		return nil, btcjson.ErrDifficulty
	}
	blockHeader := &blk.MsgBlock().Header

	return getDifficultyRatio(blockHeader.Bits), nil
}

// handleGetGenerate implements the getgenerate command.
func handleGetGenerate(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	// btcd does not do mining so we can hardcode replies here.
	return false, nil
}

// handleGetHashesPerSec implements the gethashespersec command.
func handleGetHashesPerSec(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	// btcd does not do mining so we can hardcode replies here.
	return 0, nil
}

// handleGetPeerInfo implements the getpeerinfo command.
func handleGetPeerInfo(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	return s.server.PeerInfo(), nil
}

// mempoolDescriptor describes a JSON object which is returned for each
// transaction in the memory pool in response to a getrawmempool command with
// the verbose flag set.
type mempoolDescriptor struct {
	Size             int      `json:"size"`
	Fee              float64  `json:"fee"`
	Time             int64    `json:"time"`
	Height           int64    `json:"height"`
	StartingPriority int      `json:"startingpriority"`
	CurrentPriority  int      `json:"currentpriority"`
	Depends          []string `json:"depends"`
}

// handleGetRawMempool implements the getrawmempool command.
func handleGetRawMempool(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.GetRawMempoolCmd)
	descs := s.server.txMemPool.TxDescs()

	if c.Verbose {
		result := make(map[string]*mempoolDescriptor, len(descs))
		for _, desc := range descs {
			mpd := &mempoolDescriptor{
				Size: desc.Tx.MsgTx().SerializeSize(),
				Fee: float64(desc.Fee) /
					float64(btcutil.SatoshiPerBitcoin),
				Time:             desc.Added.Unix(),
				Height:           desc.Height,
				StartingPriority: 0, // We don't mine.
				CurrentPriority:  0, // We don't mine.
			}
			for _, txIn := range desc.Tx.MsgTx().TxIn {
				hash := &txIn.PreviousOutpoint.Hash
				if s.server.txMemPool.HaveTransaction(hash) {
					mpd.Depends = append(mpd.Depends,
						hash.String())
				}
			}

			result[desc.Tx.Sha().String()] = mpd
		}

		return result, nil
	}

	// The response is simply an array of the transaction hashes if the
	// verbose flag is not set.
	hashStrings := make([]string, len(descs))
	for i := range hashStrings {
		hashStrings[i] = descs[i].Tx.Sha().String()
	}

	return hashStrings, nil
}

// handleGetRawTransaction implements the getrawtransaction command.
func handleGetRawTransaction(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.GetRawTransactionCmd)

	// Convert the provided transaction hash hex to a ShaHash.
	txSha, err := btcwire.NewShaHashFromStr(c.Txid)
	if err != nil {
		rpcsLog.Errorf("Error generating sha: %v", err)
		return nil, btcjson.Error{
			Code:    btcjson.ErrBlockNotFound.Code,
			Message: "Parameter 1 must be a hexaecimal string",
		}
	}

	// Try to fetch the transaction from the memory pool and if that fails,
	// try the block database.
	var mtx *btcwire.MsgTx
	var blksha *btcwire.ShaHash
	tx, err := s.server.txMemPool.FetchTransaction(txSha)
	if err != nil {
		txList, err := s.server.db.FetchTxBySha(txSha)
		if err != nil || len(txList) == 0 {
			rpcsLog.Errorf("Error fetching tx: %v", err)
			return nil, btcjson.ErrNoTxInfo
		}

		lastTx := len(txList) - 1
		mtx = txList[lastTx].Tx

		blksha = txList[lastTx].BlkSha
	} else {
		mtx = tx.MsgTx()
	}

	// When the verbose flag isn't set, simply return the network-serialized
	// transaction as a hex-encoded string.
	if !c.Verbose {
		mtxHex, err := messageToHex(mtx)
		if err != nil {
			return nil, err
		}
		return mtxHex, nil
	}

	var blk *btcutil.Block
	var maxidx int64
	if blksha != nil {
		blk, err = s.server.db.FetchBlockBySha(blksha)
		if err != nil {
			rpcsLog.Errorf("Error fetching sha: %v", err)
			return nil, btcjson.ErrBlockNotFound
		}

		_, maxidx, err = s.server.db.NewestSha()
		if err != nil {
			rpcsLog.Errorf("Cannot get newest sha: %v", err)
			return nil, btcjson.ErrNoNewestBlockInfo
		}
	}

	rawTxn, jsonErr := createTxRawResult(s.server.btcnet, c.Txid, mtx, blk, maxidx, blksha)
	if err != nil {
		rpcsLog.Errorf("Cannot create TxRawResult for txSha=%s: %v", txSha, err)
		return nil, jsonErr
	}
	return *rawTxn, nil
}

// createTxRawResult converts the passed transaction and associated parameters
// to a raw transaction JSON object.
func createTxRawResult(net btcwire.BitcoinNet, txSha string, mtx *btcwire.MsgTx, blk *btcutil.Block, maxidx int64, blksha *btcwire.ShaHash) (*btcjson.TxRawResult, error) {
	mtxHex, err := messageToHex(mtx)
	if err != nil {
		return nil, err
	}

	vin, err := createVinList(mtx)
	if err != nil {
		return nil, err
	}
	vout, err := createVoutList(mtx, net)
	if err != nil {
		return nil, err
	}

	txReply := &btcjson.TxRawResult{
		Hex:      mtxHex,
		Txid:     txSha,
		Vout:     vout,
		Vin:      vin,
		Version:  mtx.Version,
		LockTime: mtx.LockTime,
	}

	if blk != nil {
		blockHeader := &blk.MsgBlock().Header
		idx := blk.Height()

		// This is not a typo, they are identical in bitcoind as well.
		txReply.Time = blockHeader.Timestamp.Unix()
		txReply.Blocktime = blockHeader.Timestamp.Unix()
		txReply.BlockHash = blksha.String()
		txReply.Confirmations = uint64(1 + maxidx - idx)
	}

	return txReply, nil
}

// handleSendRawTransaction implements the sendrawtransaction command.
func handleSendRawTransaction(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.SendRawTransactionCmd)
	// Deserialize and send off to tx relay
	serializedTx, err := hex.DecodeString(c.HexTx)
	if err != nil {
		return nil, btcjson.ErrDecodeHexString
	}
	msgtx := btcwire.NewMsgTx()
	err = msgtx.Deserialize(bytes.NewBuffer(serializedTx))
	if err != nil {
		err := btcjson.Error{
			Code:    btcjson.ErrDeserialization.Code,
			Message: "TX decode failed",
		}
		return nil, err
	}

	tx := btcutil.NewTx(msgtx)
	err = s.server.txMemPool.ProcessTransaction(tx)
	if err != nil {
		// When the error is a rule error, it means the transaction was
		// simply rejected as opposed to something actually going wrong,
		// so log it as such.  Otherwise, something really did go wrong,
		// so log it as an actual error.
		if _, ok := err.(TxRuleError); ok {
			rpcsLog.Debugf("Rejected transaction %v: %v", tx.Sha(),
				err)
		} else {
			rpcsLog.Errorf("Failed to process transaction %v: %v",
				tx.Sha(), err)
			err = btcjson.Error{
				Code:    btcjson.ErrDeserialization.Code,
				Message: "TX rejected",
			}
			return nil, err
		}
	}

	return tx.Sha().String(), nil
}

// handleSetGenerate implements the setgenerate command.
func handleSetGenerate(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	// btcd does not do mining so we can hardcode replies here.
	return nil, nil
}

// handleStop implements the stop command.
func handleStop(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	s.server.Stop()
	return "btcd stopping.", nil
}

func verifyChain(db btcdb.Db, level, depth int32) error {
	_, curheight64, err := db.NewestSha()
	if err != nil {
		rpcsLog.Errorf("Verify is unable to fetch current block "+
			"height: %v", err)
	}

	curheight := int32(curheight64)

	if depth > curheight {
		depth = curheight
	}

	for height := curheight; height > (curheight - depth); height-- {
		// Level 0 just looks up the block.
		sha, err := db.FetchBlockShaByHeight(int64(height))
		if err != nil {
			rpcsLog.Errorf("Verify is unable to fetch block at "+
				"height %d: %v", height, err)
			return err
		}

		block, err := db.FetchBlockBySha(sha)
		if err != nil {
			rpcsLog.Errorf("Verify is unable to fetch block at "+
				"sha %v height %d: %v", sha, height, err)
			return err
		}

		// Level 1 does basic chain sanity checks.
		if level > 0 {
			err := btcchain.CheckBlockSanity(block,
				activeNetParams.powLimit)
			if err != nil {
				rpcsLog.Errorf("Verify is unable to "+
					"validate block at sha %v height "+
					"%s: %v", sha, height, err)
				return err
			}
		}
	}
	rpcsLog.Infof("Chain verify completed successfully")

	return nil
}

func handleVerifyChain(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.VerifyChainCmd)

	err := verifyChain(s.server.db, c.CheckLevel, c.CheckDepth)
	return err == nil, nil
}

// parseCmd parses a marshaled known command, returning any errors as a
// btcjson.Error that can be used in replies.  The returned cmd may still
// be non-nil if b is at least a valid marshaled JSON-RPC message.
func parseCmd(b []byte) (btcjson.Cmd, *btcjson.Error) {
	cmd, err := btcjson.ParseMarshaledCmd(b)
	if err != nil {
		jsonErr, ok := err.(btcjson.Error)
		if !ok {
			jsonErr = btcjson.Error{
				Code:    btcjson.ErrParse.Code,
				Message: err.Error(),
			}
		}
		return cmd, &jsonErr
	}
	return cmd, nil
}

// standardCmdReply checks that a parsed command is a standard
// Bitcoin JSON-RPC command and runs the proper handler to reply to the
// command.
func standardCmdReply(cmd btcjson.Cmd, s *rpcServer) (reply btcjson.Reply) {
	id := cmd.Id()
	reply.Id = &id

	handler, ok := rpcHandlers[cmd.Method()]
	if !ok {
		reply.Error = &btcjson.ErrMethodNotFound
		return reply
	}

	result, err := handler(s, cmd)
	if err != nil {
		jsonErr, ok := err.(btcjson.Error)
		if !ok {
			// In the case where we did not have a btcjson
			// error to begin with, make a new one to send,
			// but this really should not happen.
			jsonErr = btcjson.Error{
				Code:    btcjson.ErrInternal.Code,
				Message: err.Error(),
			}
		}
		reply.Error = &jsonErr
	} else {
		reply.Result = result
	}
	return reply
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
		rpcsLog.Errorf("Cannot get difficulty: %v", err)
		return 0
	}
	return diff
}
