// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"code.google.com/p/go.net/websocket"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/conformal/btcchain"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/conformal/btcws"
	"github.com/conformal/fastsha256"
	"io/ioutil"
	"math/big"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// rpcAuthTimeoutSeconds is the number of seconds a connection to the
	// RPC server is allowed to stay open without authenticating before it
	// is closed.
	rpcAuthTimeoutSeconds = 10

	// uint256Size is the number of bytes needed to represent an unsigned
	// 256-bit integer.
	uint256Size = 32

	// getworkDataLen is the length of the data field of the getwork RPC.
	// It consists of the serialized block header plus the internal sha256
	// padding.  The internal sha256 padding consists of a single 1 bit
	// followed by enough zeros to pad the message out to 56 bytes followed
	// by length of the message in bits encoded as a big-endian uint64
	// (8 bytes).  Thus, the resulting length is a multiple of the sha256
	// block size (64 bytes).
	getworkDataLen = (1 + ((btcwire.MaxBlockHeaderPayload + 8) /
		fastsha256.BlockSize)) * fastsha256.BlockSize

	// hash1Len is the length of the hash1 field of the getwork RPC.  It
	// consists of a zero hash plus the internal sha256 padding.  See
	// the getworkDataLen comment for details about the internal sha256
	// padding format.
	hash1Len = (1 + ((btcwire.HashSize + 8) / fastsha256.BlockSize)) *
		fastsha256.BlockSize
)

// Errors
var (
	// ErrBadParamsField describes an error where the parameters JSON
	// field cannot be properly parsed.
	ErrBadParamsField = errors.New("bad params field")
)

type commandHandler func(*rpcServer, btcjson.Cmd) (interface{}, error)

// handlers maps RPC command strings to appropriate handler functions.
// this is copied by init because help references rpcHandlers and thus causes
// a dependancy loop.
var rpcHandlers map[string]commandHandler
var rpcHandlersBeforeInit = map[string]commandHandler{
	"addnode":              handleAddNode,
	"createrawtransaction": handleCreateRawTransaction,
	"debuglevel":           handleDebugLevel,
	"decoderawtransaction": handleDecodeRawTransaction,
	"decodescript":         handleDecodeScript,
	"getaddednodeinfo":     handleGetAddedNodeInfo,
	"getbestblock":         handleGetBestBlock,
	"getbestblockhash":     handleGetBestBlockHash,
	"getblock":             handleGetBlock,
	"getblockcount":        handleGetBlockCount,
	"getblockhash":         handleGetBlockHash,
	"getconnectioncount":   handleGetConnectionCount,
	"getcurrentnet":        handleGetCurrentNet,
	"getdifficulty":        handleGetDifficulty,
	"getgenerate":          handleGetGenerate,
	"gethashespersec":      handleGetHashesPerSec,
	"getinfo":              handleGetInfo,
	"getmininginfo":        handleGetMiningInfo,
	"getnettotals":         handleGetNetTotals,
	"getnetworkhashps":     handleGetNetworkHashPS,
	"getpeerinfo":          handleGetPeerInfo,
	"getrawmempool":        handleGetRawMempool,
	"getrawtransaction":    handleGetRawTransaction,
	"getwork":              handleGetWork,
	"help":                 handleHelp,
	"ping":                 handlePing,
	"sendrawtransaction":   handleSendRawTransaction,
	"setgenerate":          handleSetGenerate,
	"stop":                 handleStop,
	"submitblock":          handleSubmitBlock,
	"verifychain":          handleVerifyChain,
}

func init() {
	rpcHandlers = rpcHandlersBeforeInit
}

// list of commands that we recognise, but for which btcd has no support because
// it lacks support for wallet functionality. For these commands the user
// should ask a connected instance of btcwallet.
var rpcAskWallet = map[string]bool{
	"addmultisigaddress":     true,
	"backupwallet":           true,
	"createencryptedwallet":  true,
	"createmultisig":         true,
	"dumpprivkey":            true,
	"dumpwallet":             true,
	"encryptwallet":          true,
	"getaccount":             true,
	"getaccountaddress":      true,
	"getaddressesbyaccount":  true,
	"getbalance":             true,
	"getblocktemplate":       true,
	"getnewaddress":          true,
	"getrawchangeaddress":    true,
	"getreceivedbyaccount":   true,
	"getreceivedbyaddress":   true,
	"gettransaction":         true,
	"gettxout":               true,
	"gettxoutsetinfo":        true,
	"importprivkey":          true,
	"importwallet":           true,
	"keypoolrefill":          true,
	"listaccounts":           true,
	"listaddressgroupings":   true,
	"listlockunspent":        true,
	"listreceivedbyaccount":  true,
	"listreceivedbyaddress":  true,
	"listsinceblock":         true,
	"listtransactions":       true,
	"listunspent":            true,
	"lockunspent":            true,
	"move":                   true,
	"sendfrom":               true,
	"sendmany":               true,
	"sendtoaddress":          true,
	"setaccount":             true,
	"settxfee":               true,
	"signmessage":            true,
	"signrawtransaction":     true,
	"validateaddress":        true,
	"verifymessage":          true,
	"walletlock":             true,
	"walletpassphrase":       true,
	"walletpassphrasechange": true,
}

// Commands that are temporarily unimplemented.
var rpcUnimplemented = map[string]bool{}

// workStateBlockInfo houses information about how to reconstruct a block given
// its template and signature script.
type workStateBlockInfo struct {
	msgBlock        *btcwire.MsgBlock
	signatureScript []byte
}

// workState houses state that is used in between multiple RPC invocations to
// getwork.
type workState struct {
	sync.Mutex
	lastTxUpdate  time.Time
	lastGenerated time.Time
	prevHash      *btcwire.ShaHash
	msgBlock      *btcwire.MsgBlock
	extraNonce    uint64
	blockInfo     map[btcwire.ShaHash]*workStateBlockInfo
}

// newWorkState returns a new instance of a workState with all internal fields
// initialized and ready to use.
func newWorkState() *workState {
	return &workState{
		blockInfo: make(map[btcwire.ShaHash]*workStateBlockInfo),
	}
}

// rpcServer holds the items the rpc server may need to access (config,
// shutdown, main server, etc.)
type rpcServer struct {
	started         int32
	shutdown        int32
	server          *server
	authsha         [fastsha256.Size]byte
	ntfnMgr         *wsNotificationManager
	numClients      int
	numClientsMutex sync.Mutex
	wg              sync.WaitGroup
	listeners       []net.Listener
	workState       *workState
	quit            chan int
}

// Start is used by server.go to start the rpc listener.
func (s *rpcServer) Start() {
	if atomic.AddInt32(&s.started, 1) != 1 {
		return
	}

	rpcsLog.Trace("Starting RPC server")
	rpcServeMux := http.NewServeMux()
	httpServer := &http.Server{
		Handler: rpcServeMux,

		// Timeout connections which don't complete the initial
		// handshake within the allowed timeframe.
		ReadTimeout: time.Second * rpcAuthTimeoutSeconds,
	}
	rpcServeMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		w.Header().Set("Content-Type", "application/json")
		r.Close = true

		// Limit the number of connections to max allowed.
		if s.limitConnections(w, r.RemoteAddr) {
			return
		}

		// Keep track of the number of connected clients.
		s.incrementClients()
		defer s.decrementClients()
		if _, err := s.checkAuth(r, true); err != nil {
			jsonAuthFail(w, r, s)
			return
		}
		jsonRPCRead(w, r, s)
	})

	// Websocket endpoint.
	rpcServeMux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		authenticated, err := s.checkAuth(r, false)
		if err != nil {
			http.Error(w, "401 Unauthorized.", http.StatusUnauthorized)
			return
		}
		wsServer := websocket.Server{
			Handler: websocket.Handler(func(ws *websocket.Conn) {
				s.WebsocketHandler(ws, r.RemoteAddr, authenticated)
			}),
		}
		wsServer.ServeHTTP(w, r)
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

	s.ntfnMgr.Start()
}

// limitConnections responds with a 503 service unavailable and returns true if
// adding another client would exceed the maximum allow RPC clients.
//
// This function is safe for concurrent access.
func (s *rpcServer) limitConnections(w http.ResponseWriter, remoteAddr string) bool {
	s.numClientsMutex.Lock()
	defer s.numClientsMutex.Unlock()

	if s.numClients+1 > cfg.RPCMaxClients {
		rpcsLog.Infof("Max RPC clients exceeded [%d] - "+
			"disconnecting client %s", cfg.RPCMaxClients,
			remoteAddr)
		http.Error(w, "503 Too busy.  Try again later.",
			http.StatusServiceUnavailable)
		return true
	}
	return false
}

// incrementClients adds one to the number of connected RPC clients.  Note
// this only applies to standard clients.  Websocket clients have their own
// limits and are tracked separately.
//
// This function is safe for concurrent access.
func (s *rpcServer) incrementClients() {
	s.numClientsMutex.Lock()
	defer s.numClientsMutex.Unlock()

	s.numClients++
}

// decrementClients subtracts one from the number of connected RPC clients.
// Note this only applies to standard clients.  Websocket clients have their own
// limits and are tracked separately.
//
// This function is safe for concurrent access.
func (s *rpcServer) decrementClients() {
	s.numClientsMutex.Lock()
	defer s.numClientsMutex.Unlock()

	s.numClients--
}

// checkAuth checks the HTTP Basic authentication supplied by a wallet
// or RPC client in the HTTP request r.  If the supplied authentication
// does not match the username and password expected, a non-nil error is
// returned.
//
// This check is time-constant.
func (s *rpcServer) checkAuth(r *http.Request, require bool) (bool, error) {
	authhdr := r.Header["Authorization"]
	if len(authhdr) <= 0 {
		if require {
			rpcsLog.Warnf("RPC authentication failure from %s",
				r.RemoteAddr)
			return false, errors.New("auth failure")
		}

		return false, nil
	}

	authsha := fastsha256.Sum256([]byte(authhdr[0]))
	cmp := subtle.ConstantTimeCompare(authsha[:], s.authsha[:])
	if cmp != 1 {
		rpcsLog.Warnf("RPC authentication failure from %s", r.RemoteAddr)
		return false, errors.New("auth failure")
	}
	return true, nil
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
	s.ntfnMgr.Shutdown()
	s.ntfnMgr.WaitForShutdown()
	close(s.quit)
	s.wg.Wait()
	rpcsLog.Infof("RPC server shutdown complete")
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
		authsha:   fastsha256.Sum256([]byte(auth)),
		server:    s,
		workState: newWorkState(),
		quit:      make(chan int),
	}
	rpc.ntfnMgr = newWsNotificationManager(&rpc)

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
		listener, err := tls.Listen("tcp4", addr, &tlsConfig)
		if err != nil {
			rpcsLog.Warnf("Can't listen on %s: %v", addr,
				err)
			continue
		}
		listeners = append(listeners, listener)
	}

	for _, addr := range ipv6ListenAddrs {
		listener, err := tls.Listen("tcp6", addr, &tlsConfig)
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

	return &rpc, nil
}

// jsonAuthFail sends a message back to the client if the http auth is rejected.
func jsonAuthFail(w http.ResponseWriter, r *http.Request, s *rpcServer) {
	w.Header().Add("WWW-Authenticate", `Basic realm="btcd RPC"`)
	http.Error(w, "401 Unauthorized.", http.StatusUnauthorized)
}

// jsonRPCRead is the RPC wrapper around the jsonRead function to handle reading
// and responding to RPC messages.
func jsonRPCRead(w http.ResponseWriter, r *http.Request, s *rpcServer) {
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
	rpcsLog.Tracef(msg)
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
	err := msg.BtcEncode(&buf, maxProtocolVersion)
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
	// some validity checks.
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
	// some validity checks.
	for encodedAddr, amount := range c.Amounts {
		// Ensure amount is in the valid range for monetary amounts.
		if amount <= 0 || amount > btcutil.MaxSatoshi {
			return nil, btcjson.Error{
				Code:    btcjson.ErrType.Code,
				Message: "Invalid amount",
			}
		}

		// Decode the provided address.
		addr, err := btcutil.DecodeAddress(encodedAddr,
			activeNetParams.Net)
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
		switch addr.(type) {
		case *btcutil.AddressPubKeyHash:
		case *btcutil.AddressScriptHash:
		default:
			return nil, btcjson.ErrInvalidAddressOrKey
		}
		if !addr.IsForNet(s.server.btcnet) {
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

// handleGetAddedNodeInfo handles getaddednodeinfo commands.
func handleGetAddedNodeInfo(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.GetAddedNodeInfoCmd)

	// Retrieve a list of persistent (added) peers from the bitcoin server
	// and filter the list of peer per the specified address (if any).
	peers := s.server.AddedNodeInfo()
	if c.Node != "" {
		found := false
		for i, peer := range peers {
			if peer.addr == c.Node {
				peers = peers[i : i+1]
				found = true
			}
		}
		if !found {
			return nil, btcjson.Error{
				Code:    -24,
				Message: "Node has not been added.",
			}
		}
	}

	// Without the dns flag, the result is just a slice of the addresses as
	// strings.
	if !c.Dns {
		results := make([]string, 0, len(peers))
		for _, peer := range peers {
			results = append(results, peer.addr)
		}
		return results, nil
	}

	// With the dns flag, the result is an array of JSON objects which
	// include the result of DNS lookups for each peer.
	results := make([]*btcjson.GetAddedNodeInfoResult, 0, len(peers))
	for _, peer := range peers {
		// Set the "address" of the peer which could be an ip address
		// or a domain name.
		var result btcjson.GetAddedNodeInfoResult
		result.AddedNode = peer.addr
		isConnected := peer.Connected()
		result.Connected = &isConnected

		// Split the address into host and port portions so we can do
		// a DNS lookup against the host.  When no port is specified in
		// the address, just use the address as the host.
		host, _, err := net.SplitHostPort(peer.addr)
		if err != nil {
			host = peer.addr
		}

		// Do a DNS lookup for the address.  If the lookup fails, just
		// use the host.
		var ipList []string
		ips, err := btcdLookup(host)
		if err == nil {
			ipList = make([]string, 0, len(ips))
			for _, ip := range ips {
				ipList = append(ipList, ip.String())
			}
		} else {
			ipList = make([]string, 1)
			ipList[0] = host
		}

		// Add the addresses and connection info to the result.
		addrs := make([]btcjson.GetAddedNodeInfoResultAddr, 0, len(ipList))
		for _, ip := range ipList {
			var addr btcjson.GetAddedNodeInfoResultAddr
			addr.Address = ip
			addr.Connected = "false"
			if ip == host && peer.Connected() {
				addr.Connected = directionString(peer.inbound)
			}
			addrs = append(addrs, addr)
		}
		result.Addresses = &addrs
		results = append(results, &result)
	}
	return results, nil
}

// handleGetBestBlock implements the getbestblock command.
func handleGetBestBlock(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	// All other "get block" commands give either the height, the
	// hash, or both but require the block SHA.  This gets both for
	// the best block.
	sha, height, err := s.server.db.NewestSha()
	if err != nil {
		return nil, btcjson.ErrBestBlockHash
	}

	result := &btcws.GetBestBlockResult{
		Hash:   sha.String(),
		Height: int32(height),
	}
	return result, nil
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
		transactions := blk.Transactions()
		txNames := make([]string, len(transactions))
		for i, tx := range transactions {
			txNames[i] = tx.Sha().String()
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

// handleGetCurrentNet implements the getcurrentnet command.
func handleGetCurrentNet(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	return s.server.btcnet, nil
}

// handleGetDifficulty implements the getdifficulty command.
func handleGetDifficulty(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	sha, _, err := s.server.db.NewestSha()
	if err != nil {
		rpcsLog.Errorf("Error getting sha: %v", err)
		return nil, btcjson.ErrDifficulty
	}
	blockHeader, err := s.server.db.FetchBlockHeaderBySha(sha)
	if err != nil {
		rpcsLog.Errorf("Error getting block: %v", err)
		return nil, btcjson.ErrDifficulty
	}
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

// handleGetInfo implements the getinfo command. We only return the fields
// that are not related to wallet functionality.
func handleGetInfo(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	// We require the current block height and sha.
	sha, height, err := s.server.db.NewestSha()
	if err != nil {
		rpcsLog.Errorf("Error getting sha: %v", err)
		return nil, btcjson.ErrBlockCount
	}
	blkHeader, err := s.server.db.FetchBlockHeaderBySha(sha)
	if err != nil {
		rpcsLog.Errorf("Error getting block: %v", err)
		return nil, btcjson.ErrDifficulty
	}

	ret := &btcjson.InfoResult{
		Version:         int(1000000*appMajor + 10000*appMinor + 100*appPatch),
		ProtocolVersion: int(maxProtocolVersion),
		Blocks:          int(height),
		TimeOffset:      0,
		Connections:     s.server.ConnectedCount(),
		Proxy:           cfg.Proxy,
		Difficulty:      getDifficultyRatio(blkHeader.Bits),
		TestNet:         cfg.TestNet3,
		RelayFee:        float64(minTxRelayFee) / float64(btcutil.SatoshiPerBitcoin),
	}

	return ret, nil
}

// handleGetMiningInfo implements the getmininginfo command. We only return the
// fields that are not related to wallet functionality.
func handleGetMiningInfo(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	sha, height, err := s.server.db.NewestSha()
	if err != nil {
		rpcsLog.Errorf("Error getting sha: %v", err)
		return nil, btcjson.ErrBlockCount
	}
	block, err := s.server.db.FetchBlockBySha(sha)
	if err != nil {
		rpcsLog.Errorf("Error getting block: %v", err)
		return nil, btcjson.ErrBlockNotFound
	}
	blockBytes, err := block.Bytes()
	if err != nil {
		rpcsLog.Errorf("Error getting block: %v", err)
		return nil, btcjson.Error{
			Code:    btcjson.ErrInternal.Code,
			Message: err.Error(),
		}
	}

	// Create a default getnetworkhashps command to use defaults and make
	// use of the existing getnetworkhashps handler.
	gnhpsCmd, err := btcjson.NewGetNetworkHashPSCmd(0)
	if err != nil {
		return nil, btcjson.Error{
			Code:    btcjson.ErrInternal.Code,
			Message: err.Error(),
		}
	}
	rpcsLog.Info(gnhpsCmd.Blocks, gnhpsCmd.Height)
	networkHashesPerSecIface, err := handleGetNetworkHashPS(s, gnhpsCmd)
	if err != nil {
		// This is already a btcjson.Error from the handler.
		return nil, err
	}
	networkHashesPerSec, ok := networkHashesPerSecIface.(int64)
	if !ok {
		return nil, btcjson.Error{
			Code:    btcjson.ErrInternal.Code,
			Message: "networkHashesPerSec is not an int64",
		}
	}

	result := &btcjson.GetMiningInfoResult{
		Blocks:           height,
		CurrentBlockSize: uint64(len(blockBytes)),
		CurrentBlockTx:   uint64(len(block.MsgBlock().Transactions)),
		Difficulty:       getDifficultyRatio(block.MsgBlock().Header.Bits),
		Generate:         false, // no built-in miner
		GenProcLimit:     -1,    // no built-in miner
		HashesPerSec:     0,     // no built-in miner
		NetworkHashPS:    networkHashesPerSec,
		PooledTx:         uint64(s.server.txMemPool.Count()),
		TestNet:          cfg.TestNet3,
	}
	return &result, nil
}

// handleGetNetTotals implements the getnettotals command.
func handleGetNetTotals(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	totalBytesRecv, totalBytesSent := s.server.NetTotals()
	reply := &btcjson.GetNetTotalsResult{
		TotalBytesRecv: totalBytesRecv,
		TotalBytesSent: totalBytesSent,
		TimeMillis:     time.Now().UTC().UnixNano() / int64(time.Millisecond),
	}
	return reply, nil
}

// handleGetNetworkHashPS implements the getnetworkhashps command.
func handleGetNetworkHashPS(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.GetNetworkHashPSCmd)

	_, newestHeight, err := s.server.db.NewestSha()
	if err != nil {
		return nil, btcjson.Error{
			Code:    btcjson.ErrInternal.Code,
			Message: err.Error(),
		}
	}

	// When the passed height is too high or zero, just return 0 now
	// since we can't reasonably calculate the number of network hashes
	// per second from invalid values.  When it's negative, use the current
	// best block height.
	endHeight := int64(c.Height)
	if endHeight > newestHeight || endHeight == 0 {
		return 0, nil
	}
	if endHeight < 0 {
		endHeight = newestHeight
	}

	// Calculate the starting block height based on the passed number of
	// blocks.  When the passed value is negative, use the last block the
	// difficulty changed as the starting height.  Also make sure the
	// starting height is not before the beginning of the chain.
	var startHeight int64
	if c.Blocks <= 0 {
		startHeight = endHeight - ((endHeight % btcchain.BlocksPerRetarget) + 1)
	} else {
		startHeight = endHeight - int64(c.Blocks)
	}
	if startHeight < 0 {
		startHeight = 0
	}
	rpcsLog.Debugf("Calculating network hashes per second from %d to %d",
		startHeight, endHeight)

	// Find the min and max block timestamps as well as calculate the total
	// amount of work that happened between the start and end blocks.
	var minTimestamp, maxTimestamp time.Time
	totalWork := big.NewInt(0)
	for curHeight := startHeight; curHeight <= endHeight; curHeight++ {
		hash, err := s.server.db.FetchBlockShaByHeight(curHeight)
		if err != nil {
			return nil, btcjson.Error{
				Code:    btcjson.ErrInternal.Code,
				Message: err.Error(),
			}
		}

		header, err := s.server.db.FetchBlockHeaderBySha(hash)
		if err != nil {
			return nil, btcjson.Error{
				Code:    btcjson.ErrInternal.Code,
				Message: err.Error(),
			}
		}

		if curHeight == startHeight {
			minTimestamp = header.Timestamp
			maxTimestamp = minTimestamp
		} else {
			totalWork.Add(totalWork, btcchain.CalcWork(header.Bits))

			if minTimestamp.After(header.Timestamp) {
				minTimestamp = header.Timestamp
			}
			if maxTimestamp.Before(header.Timestamp) {
				maxTimestamp = header.Timestamp
			}
		}
	}

	// Calculate the difference in seconds between the min and max block
	// timestamps and avoid division by zero in the case where there is no
	// time difference.
	timeDiff := int64(maxTimestamp.Sub(minTimestamp) / time.Second)
	if timeDiff == 0 {
		return 0, nil
	}

	hashesPerSec := new(big.Int).Div(totalWork, big.NewInt(timeDiff))
	return hashesPerSec.Int64(), nil
}

// handleGetPeerInfo implements the getpeerinfo command.
func handleGetPeerInfo(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	return s.server.PeerInfo(), nil
}

// handleGetRawMempool implements the getrawmempool command.
func handleGetRawMempool(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.GetRawMempoolCmd)
	descs := s.server.txMemPool.TxDescs()

	if c.Verbose {
		result := make(map[string]*btcjson.GetRawMempoolResult, len(descs))
		for _, desc := range descs {
			mpd := &btcjson.GetRawMempoolResult{
				Size: desc.Tx.MsgTx().SerializeSize(),
				Fee: float64(desc.Fee) /
					float64(btcutil.SatoshiPerBitcoin),
				Time:             desc.Added.Unix(),
				Height:           desc.Height,
				StartingPriority: 0, // We don't mine.
				CurrentPriority:  0, // We don't mine.
				Depends:          make([]string, 0),
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
	if c.Verbose == 0 {
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

// bigToLEUint256 returns the passed big integer as an unsigned 256-bit integer
// encoded as little-endian bytes.  Numbers which are larger than the max
// unsigned 256-bit integer are truncated.
func bigToLEUint256(n *big.Int) [uint256Size]byte {
	// Pad or truncate the big-endian big int to correct number of bytes.
	nBytes := n.Bytes()
	nlen := len(nBytes)
	pad := 0
	start := 0
	if nlen <= uint256Size {
		pad = uint256Size - nlen
	} else {
		start = nlen - uint256Size
	}
	var buf [uint256Size]byte
	copy(buf[pad:], nBytes[start:])

	// Reverse the bytes to little endian and return them.
	for i := 0; i < uint256Size/2; i++ {
		buf[i], buf[uint256Size-1-i] = buf[uint256Size-1-i], buf[i]
	}
	return buf
}

// reverseUint32Array treats the passed bytes as a series of uint32s and
// reverses the byte order of each uint32.  The passed byte slice must be a
// multiple of 4 for a correct result.  The passed bytes slice is modified.
func reverseUint32Array(b []byte) {
	blen := len(b)
	for i := 0; i < blen; i += 4 {
		b[i], b[i+3] = b[i+3], b[i]
		b[i+1], b[i+2] = b[i+2], b[i+1]
	}
}

// handleGetWorkRequest is a helper for handleGetWork which deals with
// generating and returning work to the caller.
//
// This function MUST be called with the RPC workstate locked.
func handleGetWorkRequest(s *rpcServer) (interface{}, error) {
	state := s.workState

	// Generate a new block template when the current best block has
	// changed or the transactions in the memory pool have been updated
	// and it has been at least one minute since the last template was
	// generated.
	lastTxUpdate := s.server.txMemPool.LastUpdated()
	latestHash, latestHeight := s.server.blockManager.chainState.Best()
	msgBlock := state.msgBlock
	if msgBlock == nil || state.prevHash == nil ||
		!state.prevHash.IsEqual(latestHash) ||
		(state.lastTxUpdate != lastTxUpdate &&
			time.Now().After(state.lastGenerated.Add(time.Minute))) {

		// Reset the extra nonce and clear all cached template
		// variations if the best block changed.
		if state.prevHash != nil && !state.prevHash.IsEqual(latestHash) {
			state.extraNonce = 0
			state.blockInfo = make(map[btcwire.ShaHash]*workStateBlockInfo)
		}

		// Reset the previous best hash the block template was generated
		// against so any errors below cause the next invocation to try
		// again.
		state.prevHash = nil

		// Choose a payment address at random.
		rand.Seed(time.Now().UnixNano())
		payToAddr := cfg.miningKeys[rand.Intn(len(cfg.miningKeys))]

		template, err := NewBlockTemplate(payToAddr, s.server.txMemPool)
		if err != nil {
			errStr := fmt.Sprintf("Failed to create new block "+
				"template: %v", err)
			rpcsLog.Errorf(errStr)
			return nil, btcjson.Error{
				Code:    btcjson.ErrInternal.Code,
				Message: errStr,
			}
		}
		msgBlock = template.block

		// Update work state to ensure another block template isn't
		// generated until needed.
		state.msgBlock = msgBlock
		state.lastGenerated = time.Now()
		state.lastTxUpdate = lastTxUpdate
		state.prevHash = latestHash

		rpcsLog.Debugf("Generated block template (timestamp %v, extra "+
			"nonce %d, target %064x, merkle root %s, signature "+
			"script %x)", msgBlock.Header.Timestamp,
			state.extraNonce,
			btcchain.CompactToBig(msgBlock.Header.Bits),
			msgBlock.Header.MerkleRoot,
			msgBlock.Transactions[0].TxIn[0].SignatureScript)
	} else {
		// At this point, there is a saved block template and a new
		// request for work was made, but either the available
		// transactions haven't change or it hasn't been long enough to
		// trigger a new block template to be generated.  So, update the
		// existing block template and track the variations so each
		// variation can be regenerated if a caller finds an answer and
		// makes a submission against it.

		// Update the time of the block template to the current time
		// while accounting for the median time of the past several
		// blocks per the chain consensus rules.
		UpdateBlockTime(msgBlock, s.server.blockManager)

		// Increment the extra nonce and update the block template
		// with the new value by regenerating the coinbase script and
		// setting the merkle root to the new value.
		state.extraNonce++
		err := UpdateExtraNonce(msgBlock, latestHeight+1, state.extraNonce)
		if err != nil {
			errStr := fmt.Sprintf("Failed to update extra nonce: "+
				"%v", err)
			rpcsLog.Warnf(errStr)
			return nil, btcjson.Error{
				Code:    btcjson.ErrInternal.Code,
				Message: errStr,
			}
		}

		rpcsLog.Debugf("Updated block template (timestamp %v, extra "+
			"nonce %d, target %064x, merkle root %s, signature "+
			"script %x)", msgBlock.Header.Timestamp,
			state.extraNonce,
			btcchain.CompactToBig(msgBlock.Header.Bits),
			msgBlock.Header.MerkleRoot,
			msgBlock.Transactions[0].TxIn[0].SignatureScript)
	}

	// In order to efficiently store the variations of block templates that
	// have been provided to callers, save a pointer to the block as well as
	// the modified signature script keyed by the merkle root.  This
	// information, along with the data that is included in a work
	// submission, is used to rebuild the block before checking the
	// submitted solution.
	coinbaseTx := msgBlock.Transactions[0]
	state.blockInfo[msgBlock.Header.MerkleRoot] = &workStateBlockInfo{
		msgBlock:        msgBlock,
		signatureScript: coinbaseTx.TxIn[0].SignatureScript,
	}

	// Serialize the block header into a buffer large enough to hold the
	// the block header and the internal sha256 padding that is added and
	// retuned as part of the data below.
	data := make([]byte, 0, getworkDataLen)
	buf := bytes.NewBuffer(data)
	err := msgBlock.Header.Serialize(buf)
	if err != nil {
		errStr := fmt.Sprintf("Failed to serialize data: %v", err)
		rpcsLog.Warnf(errStr)
		return nil, btcjson.Error{
			Code:    btcjson.ErrInternal.Code,
			Message: errStr,
		}
	}

	// Calculate the midstate for the block header.  The midstate here is
	// the internal state of the sha256 algorithm for the first chunk of the
	// block header (sha256 operates on 64-byte chunks) which is before the
	// nonce.  This allows sophisticated callers to avoid hashing the first
	// chunk over and over while iterating the nonce range.
	data = data[:buf.Len()]
	midstate := fastsha256.MidState256(data)

	// Expand the data slice to include the full data buffer and apply the
	// internal sha256 padding which consists of a single 1 bit followed
	// by enough zeros to pad the message out to 56 bytes followed by the
	// length of the message in bits encoded as a big-endian uint64
	// (8 bytes).  Thus, the resulting length is a multiple of the sha256
	// block size (64 bytes).  This makes the data ready for sophisticated
	// caller to make use of only the second chunk along with the midstate
	// for the first chunk.
	data = data[:getworkDataLen]
	data[btcwire.MaxBlockHeaderPayload] = 0x80
	binary.BigEndian.PutUint64(data[len(data)-8:],
		btcwire.MaxBlockHeaderPayload*8)

	// Create the hash1 field which is a zero hash along with the internal
	// sha256 padding as described above.  This field is really quite
	// useless, but it is required for compatibility with the reference
	// implementation.
	var hash1 [hash1Len]byte
	hash1[btcwire.HashSize] = 0x80
	binary.BigEndian.PutUint64(hash1[len(hash1)-8:], btcwire.HashSize*8)

	// The final result reverses the each of the fields to little endian.
	// In particular, the data, hash1, and midstate fields are treated as
	// arrays of uint32s (per the internal sha256 hashing state) which are
	// in big endian, and thus each 4 bytes is byte swapped.  The target is
	// also in big endian, but it is treated as a uint256 and byte swapped
	// to little endian accordingly.
	//
	// The fact the fields are reversed in this way is rather odd and likey
	// an artifact of some legacy internal state in the reference
	// implementation, but it is required for compatibility.
	reverseUint32Array(data)
	reverseUint32Array(hash1[:])
	reverseUint32Array(midstate[:])
	target := bigToLEUint256(btcchain.CompactToBig(msgBlock.Header.Bits))
	reply := &btcjson.GetWorkResult{
		Data:     hex.EncodeToString(data),
		Hash1:    hex.EncodeToString(hash1[:]),
		Midstate: hex.EncodeToString(midstate[:]),
		Target:   hex.EncodeToString(target[:]),
	}
	return reply, nil
}

// handleGetWorkSubmission is a helper for handleGetWork which deals with
// the calling submitting work to be verified and processed.
//
// This function MUST be called with the RPC workstate locked.
func handleGetWorkSubmission(s *rpcServer, hexData string) (interface{}, error) {
	// Ensure the provided data is sane.
	if len(hexData)%2 != 0 {
		hexData = "0" + hexData
	}
	data, err := hex.DecodeString(hexData)
	if err != nil {
		return false, btcjson.Error{
			Code: btcjson.ErrInvalidParameter.Code,
			Message: fmt.Sprintf("argument must be "+
				"hexadecimal string (not %q)", hexData),
		}
	}
	if len(data) != getworkDataLen {
		return false, btcjson.Error{
			Code: btcjson.ErrInvalidParameter.Code,
			Message: fmt.Sprintf("argument must be "+
				"%d bytes (not %d)", getworkDataLen,
				len(data)),
		}
	}

	// Reverse the data as if it were an array of 32-bit unsigned integers.
	// The fact the getwork request and submission data is reversed in this
	// way is rather odd and likey an artifact of some legacy internal state
	// in the reference implementation, but it is required for
	// compatibility.
	reverseUint32Array(data)

	// Deserialize the block header from the data.
	var submittedHeader btcwire.BlockHeader
	bhBuf := bytes.NewBuffer(data[0:btcwire.MaxBlockHeaderPayload])
	err = submittedHeader.Deserialize(bhBuf)
	if err != nil {
		return false, btcjson.Error{
			Code: btcjson.ErrInvalidParameter.Code,
			Message: fmt.Sprintf("argument does not "+
				"contain a valid block header: %v", err),
		}
	}

	// Look up the full block for the provided data based on the
	// merkle root.  Return false to indicate the solve failed if
	// it's not available.
	state := s.workState
	blockInfo, ok := state.blockInfo[submittedHeader.MerkleRoot]
	if !ok {
		rpcsLog.Debugf("Block submitted via getwork has no matching "+
			"template for merkle root %s",
			submittedHeader.MerkleRoot)
		return false, nil
	}

	// Reconstruct the block using the submitted header stored block info.
	msgBlock := blockInfo.msgBlock
	block := btcutil.NewBlock(msgBlock)
	msgBlock.Header.Timestamp = submittedHeader.Timestamp
	msgBlock.Header.Nonce = submittedHeader.Nonce
	msgBlock.Transactions[0].TxIn[0].SignatureScript = blockInfo.signatureScript
	merkles := btcchain.BuildMerkleTreeStore(block.Transactions())
	msgBlock.Header.MerkleRoot = *merkles[len(merkles)-1]

	// Ensure the submitted block hash is less than the target difficulty.
	err = btcchain.CheckProofOfWork(block, activeNetParams.PowLimit)
	if err != nil {
		// Anything other than a rule violation is an unexpected error,
		// so return that error as an internal error.
		if _, ok := err.(btcchain.RuleError); !ok {
			return false, btcjson.Error{
				Code: btcjson.ErrInternal.Code,
				Message: fmt.Sprintf("Unexpected error while "+
					"checking proof of work: %v", err),
			}
		}

		rpcsLog.Debugf("Block submitted via getwork does not meet "+
			"the required proof of work: %v", err)
		return false, nil
	}

	latestHash, _ := s.server.blockManager.chainState.Best()
	if !msgBlock.Header.PrevBlock.IsEqual(latestHash) {
		rpcsLog.Debugf("Block submitted via getwork with previous "+
			"block %s is stale", msgBlock.Header.PrevBlock)
		return false, nil
	}

	// Process this block using the same rules as blocks coming from other
	// nodes.  This will in turn relay it to the network like normal.
	isOrphan, err := s.server.blockManager.ProcessBlock(block)
	if err != nil || isOrphan {
		// Anything other than a rule violation is an unexpected error,
		// so return that error as an internal error.
		if _, ok := err.(btcchain.RuleError); !ok {
			return false, btcjson.Error{
				Code: btcjson.ErrInternal.Code,
				Message: fmt.Sprintf("Unexpected error while "+
					"processing block: %v", err),
			}
		}

		rpcsLog.Infof("Block submitted via getwork rejected: %v", err)
		return false, nil
	}

	// The block was accepted.
	blockSha, _ := block.Sha()
	rpcsLog.Infof("Block submitted via getwork accepted: %s", blockSha)
	return true, nil
}

// handleGetWork implements the getwork command.
func handleGetWork(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.GetWorkCmd)

	// Respond with an error if there are no public keys to pay the created
	// blocks to.
	if len(cfg.miningKeys) == 0 {
		return nil, btcjson.Error{
			Code:    btcjson.ErrInternal.Code,
			Message: "No payment addresses specified via --getworkkey",
		}
	}

	// Return an error if there are no peers connected since there is no
	// way to relay a found block or receive transactions to work on.
	// However, allow this state when running in regression test mode.
	if !cfg.RegressionTest && s.server.ConnectedCount() == 0 {
		return nil, btcjson.ErrClientNotConnected
	}

	// No point in generating or accepting work before the chain is synced.
	_, currentHeight := s.server.blockManager.chainState.Best()
	if currentHeight != 0 && !s.server.blockManager.IsCurrent() {
		return nil, btcjson.ErrClientInInitialDownload
	}

	// Protect concurrent access from multiple RPC invocations for work
	// requests and submission.
	s.workState.Lock()
	defer s.workState.Unlock()

	// When the caller provides data, it is a submission of a supposedly
	// solved block that needs to be checked and submitted to the network
	// if valid.
	if c.Data != "" {
		return handleGetWorkSubmission(s, c.Data)
	}

	// No data was provided, so the caller is requesting work.
	return handleGetWorkRequest(s)
}

var helpAddenda = map[string]string{
	"getgenerate": `
NOTE: btcd does not mine so this will always return false. The call is provided
for compatibility only.`,
	"gethashespersec": `
NOTE: btcd does not mine so this will always return false. The call is provided
for compatibility only.`,
	"sendrawtransaction": `
NOTE: btcd does not currently support the "allowhighfees" parameter.`,
	"setgenerate": `
NOTE: btcd does not mine so command has no effect. The call is provided
for compatibility only.`,
}

// getHelp text retreives help text from btcjson for the command in question.
// If there is any extra btcd specific information related to the given command
// then this is appended to the string.
func getHelpText(cmdName string) (string, error) {
	help, err := btcjson.GetHelpString(cmdName)
	if err != nil {
		return "", err
	}
	if helpAddendum, ok := helpAddenda[cmdName]; ok {
		help += helpAddendum
	}

	return help, nil
}

// handleHelp implements the help command.
func handleHelp(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	help := cmd.(*btcjson.HelpCmd)

	// if no args we give a list of all known commands
	if help.Command == "" {
		commands := ""
		first := true
		// TODO(oga) this should have one liner usage for each command
		// really, but for now just a list of commands is sufficient.
		for k := range rpcHandlers {
			if !first {
				commands += "\n"
			}
			commands += k
			first = false
		}
		return commands, nil
	}

	// Check that we actually support the command asked for. We only
	// search the main list of hanlders since we do not wish to provide help
	// for commands that are unimplemented or relate to wallet
	// functionality.
	if _, ok := rpcHandlers[help.Command]; !ok {
		return "", fmt.Errorf("help: unknown command: %s", help.Command)
	}

	return getHelpText(help.Command)
}

// handlePing implements the ping command.
func handlePing(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	// Ask server to ping \o_
	nonce, err := btcwire.RandomUint64()
	if err != nil {
		return nil, fmt.Errorf("Not sending ping - can not generate "+
			"nonce: %v", err)
	}
	s.server.BroadcastMessage(btcwire.NewMsgPing(nonce))

	return nil, nil
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
	err = s.server.txMemPool.ProcessTransaction(tx, false, false)
	if err != nil {
		// When the error is a rule error, it means the transaction was
		// simply rejected as opposed to something actually going wrong,
		// so log it as such.  Otherwise, something really did go wrong,
		// so log it as an actual error.  In both cases, a JSON-RPC
		// error is returned to the client with the deserialization
		// error code (to match bitcoind behavior).
		if _, ok := err.(TxRuleError); ok {
			rpcsLog.Debugf("Rejected transaction %v: %v", tx.Sha(),
				err)
		} else {
			rpcsLog.Errorf("Failed to process transaction %v: %v",
				tx.Sha(), err)
		}
		err = btcjson.Error{
			Code:    btcjson.ErrDeserialization.Code,
			Message: fmt.Sprintf("TX rejected: %v", err),
		}
		return nil, err
	}

	// We keep track of all the sendrawtransaction request txs so that we
	// can rebroadcast them if they don't make their way into a block.
	iv := btcwire.NewInvVect(btcwire.InvTypeTx, tx.Sha())
	s.server.AddRebroadcastInventory(iv)

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

// handleSubmitBlock implements the submitblock command.
func handleSubmitBlock(s *rpcServer, cmd btcjson.Cmd) (interface{}, error) {
	c := cmd.(*btcjson.SubmitBlockCmd)
	// Deserialize and send off to block processor.
	serializedBlock, err := hex.DecodeString(c.HexBlock)
	if err != nil {
		err := btcjson.Error{
			Code:    btcjson.ErrDeserialization.Code,
			Message: "Block decode failed",
		}
		return nil, err
	}

	block, err := btcutil.NewBlockFromBytes(serializedBlock)
	if err != nil {
		err := btcjson.Error{
			Code:    btcjson.ErrDeserialization.Code,
			Message: "Block decode failed",
		}
		return nil, err
	}

	_, err = s.server.blockManager.ProcessBlock(block)
	if err != nil {
		return fmt.Sprintf("rejected: %s", err.Error()), nil
	}

	return nil, nil
}

func verifyChain(db btcdb.Db, level, depth int32) error {
	_, curHeight64, err := db.NewestSha()
	if err != nil {
		rpcsLog.Errorf("Verify is unable to fetch current block "+
			"height: %v", err)
	}
	curHeight := int32(curHeight64)

	finishHeight := curHeight - depth
	if finishHeight < 0 {
		finishHeight = 0
	}
	rpcsLog.Infof("Verifying chain for %d blocks at level %d",
		curHeight-finishHeight, level)

	for height := curHeight; height > finishHeight; height-- {
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
				activeNetParams.PowLimit)
			if err != nil {
				rpcsLog.Errorf("Verify is unable to "+
					"validate block at sha %v height "+
					"%d: %v", sha, height, err)
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
	if ok {
		goto handled
	}
	_, ok = rpcAskWallet[cmd.Method()]
	if ok {
		handler = handleAskWallet
		goto handled
	}
	_, ok = rpcUnimplemented[cmd.Method()]
	if ok {
		handler = handleUnimplemented
		goto handled
	}
	reply.Error = &btcjson.ErrMethodNotFound
	return reply
handled:

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
	max := btcchain.CompactToBig(activeNetParams.PowLimitBits)
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
