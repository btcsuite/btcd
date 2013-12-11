// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"code.google.com/p/go.net/websocket"
	"container/list"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	_ "crypto/sha512" // for cert generation
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/conformal/btcchain"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/conformal/btcws"
	"math/big"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
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
func (r *wsContext) AddTxRequest(walletNotification chan []byte, rc *requestContexts, addrhash string, id interface{}) {
	r.Lock()
	defer r.Unlock()

	nc := &notificationCtx{
		id:         id,
		connection: walletNotification,
		rc:         rc,
	}

	clist, ok := r.txNotifications[addrhash]
	if !ok {
		clist = list.New()
		r.txNotifications[addrhash] = clist
	}

	clist.PushBack(nc)

	rc.txRequests[addrhash] = id
}

func (r *wsContext) removeGlobalTxRequest(walletNotification chan []byte, addrhash string) {
	clist := r.txNotifications[addrhash]
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
		delete(r.txNotifications, addrhash)
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

// genkey generates a key/cert pair to the paths provided.
// TODO(oga) wrap errors with fmt.Errorf for more context?
func genKey(key, cert string) error {
	rpcsLog.Infof("Generating TLS certificates...")
	priv, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(10 * 365 * 24 * time.Hour)

	// end of ASN.1 time
	endOfTime := time.Date(2049, 12, 31, 23, 59, 59, 0, time.UTC)
	if notAfter.After(endOfTime) {
		notAfter = endOfTime
	}

	template := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			Organization: []string{"btcd autogenerated cert"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:        true, // so can sign self.
		BasicConstraintsValid: true,
	}

	host, err := os.Hostname()
	if err != nil {
		return err
	}
	template.DNSNames = append(template.DNSNames, host, "localhost")

	needLocalhost := true
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return err
	}
	for _, a := range addrs {
		ip, _, err := net.ParseCIDR(a.String())
		if err == nil {
			if ip.String() == "127.0.0.1" {
				needLocalhost = false
			}
			template.IPAddresses = append(template.IPAddresses, ip)
		}
	}
	if needLocalhost {
		localHost := net.ParseIP("127.0.0.1")
		template.IPAddresses = append(template.IPAddresses, localHost)
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template,
		&template, &priv.PublicKey, priv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create certificate: %v\n", err)
		os.Exit(-1)
	}

	certOut, err := os.Create(cert)
	if err != nil {
		return err
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()

	keyOut, err := os.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		0600)
	if err != nil {
		os.Remove(cert)
		return err
	}
	keybytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		os.Remove(key)
		os.Remove(cert)
		return err
	}
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keybytes})
	keyOut.Close()

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
	rpc.ws.connections = make(map[chan []byte]*requestContexts)
	rpc.ws.walletNotificationMaster = make(chan []byte)
	rpc.ws.txNotifications = make(map[string]*list.List)
	rpc.ws.spentNotifications = make(map[btcwire.OutPoint]*list.List)
	rpc.ws.minedTxNotifications = make(map[btcwire.ShaHash]*list.List)

	// check for existence of cert file and key file
	if !fileExists(cfg.RPCKey) && !fileExists(cfg.RPCCert) {
		// if both files do not exist, we generate them.
		err := genKey(cfg.RPCKey, cfg.RPCCert)
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

// jsonRPCRead is the RPC wrapper around the jsonRead function to handles
// reading and responding to RPC messages.
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
		reply = standardCmdReply(cmd, s, nil)
	}

	rpcsLog.Tracef("reply: %v", reply)

	msg, err := btcjson.MarshallAndSend(reply, w)
	if err != nil {
		rpcsLog.Errorf(msg)
		return
	}
	rpcsLog.Debugf(msg)
}

// TODO(jrick): Remove the wallet notification chan.
type commandHandler func(*rpcServer, btcjson.Cmd, chan []byte) (interface{}, error)

var handlers = map[string]commandHandler{
	"addmultisigaddress":     handleAskWallet,
	"addnode":                handleAddNode,
	"backupwallet":           handleAskWallet,
	"createmultisig":         handleAskWallet,
	"createrawtransaction":   handleUnimplemented,
	"debuglevel":             handleDebugLevel,
	"decoderawtransaction":   handleDecodeRawTransaction,
	"decodescript":           handleUnimplemented,
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

type wsCommandHandler func(*rpcServer, btcjson.Cmd, chan []byte, *requestContexts) error

var wsHandlers = map[string]wsCommandHandler{
	"getcurrentnet": handleGetCurrentNet,
	"getbestblock":  handleGetBestBlock,
	"rescan":        handleRescan,
	"notifynewtxs":  handleNotifyNewTXs,
	"notifyspent":   handleNotifySpent,
}

// handleUnimplemented is a temporary handler for commands that we should
// support but do not.
func handleUnimplemented(s *rpcServer, cmd btcjson.Cmd,
	walletNotification chan []byte) (interface{}, error) {
	return nil, btcjson.ErrUnimplemented
}

// handleAskWallet is the handler for commands that we do recognise as valid
// but that we can not answer correctly since it involves wallet state.
// These commands will be implemented in btcwallet.
func handleAskWallet(s *rpcServer, cmd btcjson.Cmd,
	walletNotification chan []byte) (interface{}, error) {
	return nil, btcjson.ErrNoWallet
}

// handleDecodeRawTransaction handles decoderawtransaction commands.
func handleAddNode(s *rpcServer, cmd btcjson.Cmd,
	walletNotification chan []byte) (interface{}, error) {
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

	// no data returned unless an error.
	return nil, err
}

// handleDebugLevel handles debuglevel commands.
func handleDebugLevel(s *rpcServer, cmd btcjson.Cmd,
	walletNotification chan []byte) (interface{}, error) {
	c := cmd.(*btcjson.DebugLevelCmd)

	// Special show command to list supported subsystems.
	if c.LevelSpec == "show" {
		return fmt.Sprintf("Supported subsystems %v",
			supportedSubsystems()), nil
	}

	err := parseAndSetDebugLevels(c.LevelSpec)
	if err != nil {
		jsonErr := btcjson.Error{
			Code:    btcjson.ErrInvalidParams.Code,
			Message: err.Error(),
		}
		return nil, jsonErr
	}

	return "Done.", nil
}

// handleDecodeRawTransaction handles decoderawtransaction commands.
func handleDecodeRawTransaction(s *rpcServer, cmd btcjson.Cmd,
	walletNotification chan []byte) (interface{}, error) {
	// TODO: use c.HexTx and fill result with info.
	return btcjson.TxRawDecodeResult{}, nil
}

// handleGetBestBlockHash implements the getbestblockhash command.
func handleGetBestBlockHash(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
	var sha *btcwire.ShaHash
	sha, _, err := s.server.db.NewestSha()
	if err != nil {
		rpcsLog.Errorf("Error getting newest sha: %v", err)
		return nil, btcjson.ErrBestBlockHash
	}

	return sha.String(), nil
}

// handleGetBlock implements the getblock command.
func handleGetBlock(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
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
	idx := blk.Height()
	buf, err := blk.Bytes()
	if err != nil {
		rpcsLog.Errorf("Error fetching block: %v", err)
		return nil, btcjson.ErrBlockNotFound
	}

	_, maxidx, err := s.server.db.NewestSha()
	if err != nil {
		rpcsLog.Errorf("Cannot get newest sha: %v", err)
		return nil, btcjson.ErrBlockNotFound
	}

	if !c.Verbose {
		var wireBuf bytes.Buffer
		err := blk.MsgBlock().BtcEncode(&wireBuf, btcwire.ProtocolVersion)
		if err != nil {
			return nil, btcjson.Error{Code: btcjson.ErrInternal.Code, Message: err.Error()}
		}
		blkHex := hex.EncodeToString(wireBuf.Bytes())
		return btcjson.BlockResult{Hex: blkHex}, nil
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

			rawTxn, err := createTxRawResult(s.server.btcnet, txSha, mtx, blk, maxidx, sha, true)
			if err != nil {
				rpcsLog.Errorf("Cannot create TxRawResult for txSha=%s: %v", txSha, err)
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
func handleGetBlockCount(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
	_, maxidx, err := s.server.db.NewestSha()
	if err != nil {
		rpcsLog.Errorf("Error getting newest sha: %v", err)
		return nil, btcjson.ErrBlockCount
	}

	return maxidx, nil
}

// handleGetBlockHash implements the getblockhash command.
func handleGetBlockHash(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
	c := cmd.(*btcjson.GetBlockHashCmd)
	sha, err := s.server.db.FetchBlockShaByHeight(c.Index)
	if err != nil {
		rpcsLog.Errorf("Error getting block: %v", err)
		return nil, btcjson.ErrOutOfRange
	}

	return sha.String(), nil
}

// handleGetConnectionCount implements the getconnectioncount command.
func handleGetConnectionCount(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
	return s.server.ConnectedCount(), nil
}

// handleGetDifficulty implements the getdifficulty command.
func handleGetDifficulty(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
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
func handleGetGenerate(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
	// btcd does not do mining so we can hardcode replies here.
	return false, nil
}

// handleGetHashesPerSec implements the gethashespersec command.
func handleGetHashesPerSec(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
	// btcd does not do mining so we can hardcode replies here.
	return 0, nil
}

// handleGetPeerInfo implements the getpeerinfo command.
func handleGetPeerInfo(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
	return s.server.PeerInfo(), nil
}

type mempoolDescriptor struct {
	Size	int	`json:"size"`
	Fee int64		`json:"fee"`
	Time int64	`json:"time"`
	Height int64	`json:"height"`
	StartingPriority int `json:"startingpriority"`
	CurrentPriority	int `json:"currentpriority"`
	Depends		[]string `json:"depends"`

}

// handleGetRawMempool implements the getrawmempool command.
func handleGetRawMempool(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
	c := cmd.(*btcjson.GetRawMempoolCmd)
	descs := s.server.txMemPool.TxDescs()

	if c.Verbose {
		result := make(map[string]*mempoolDescriptor, len(descs))
		for _, desc := range descs {
			mpd := &mempoolDescriptor{
				Size: desc.Tx.MsgTx().SerializeSize(),
				Fee:  desc.Fee,
				Time: desc.Added.Unix(),
				Height: desc.Height,
				StartingPriority: 0, // We don't mine.
				CurrentPriority: 0, // We don't mine.
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

		return  result, nil
	}
	hashStrings := make([]string, len(descs))
	for i := range(hashStrings) {
		hashStrings[i] = descs[i].Tx.Sha().String()
	}


	return hashStrings, nil
}

// handleGetRawTransaction implements the getrawtransaction command.
func handleGetRawTransaction(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
	c := cmd.(*btcjson.GetRawTransactionCmd)

	// TODO: check error code. tx is not checked before
	// this point.
	txSha, _ := btcwire.NewShaHashFromStr(c.Txid)
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

	rawTxn, jsonErr := createTxRawResult(s.server.btcnet, c.Txid, mtx, blk, maxidx, blksha, c.Verbose)
	if err != nil {
		rpcsLog.Errorf("Cannot create TxRawResult for txSha=%s: %v", txSha, err)
		return nil, *jsonErr
	}
	return *rawTxn, nil
}

func createTxRawResult(net btcwire.BitcoinNet, txSha string, mtx *btcwire.MsgTx, blk *btcutil.Block, maxidx int64, blksha *btcwire.ShaHash, verbose bool) (*btcjson.TxRawResult, *btcjson.Error) {
	tx := btcutil.NewTx(mtx)

	var buf bytes.Buffer
	err := mtx.BtcEncode(&buf, btcwire.ProtocolVersion)
	if err != nil {
		return nil, &btcjson.Error{Code: btcjson.ErrInternal.Code, Message: err.Error()}
	}
	mtxHex := hex.EncodeToString(buf.Bytes())

	if !verbose {
		return &btcjson.TxRawResult{Hex: mtxHex}, nil
	}

	txOutList := mtx.TxOut
	voutList := make([]btcjson.Vout, len(txOutList))

	txInList := mtx.TxIn
	vinList := make([]btcjson.Vin, len(txInList))

	for i, v := range txInList {
		if btcchain.IsCoinBase(tx) {
			vinList[i].Coinbase = hex.EncodeToString(v.SignatureScript)
		} else {
			vinList[i].Txid = v.PreviousOutpoint.Hash.String()
			vinList[i].Vout = int(uint32(v.PreviousOutpoint.Index))

			disbuf, _ := btcscript.DisasmString(v.SignatureScript)
			vinList[i].ScriptSig.Asm = disbuf
			vinList[i].ScriptSig.Hex = hex.EncodeToString(v.SignatureScript)
		}
		vinList[i].Sequence = float64(v.Sequence)
	}

	for i, v := range txOutList {
		voutList[i].N = i
		voutList[i].Value = float64(v.Value) / float64(btcutil.SatoshiPerBitcoin)

		_, addrhash, err := btcscript.ScriptToAddrHash(v.PkScript)
		if err != nil {
			// TODO: set and return error?
			rpcsLog.Errorf("Error getting address hash for %v: %v", txSha, err)
		}
		if addr, err := btcutil.EncodeAddress(addrhash, net); err == nil {
			// TODO: set and return error?
			addrList := make([]string, 1)
			addrList[0] = addr
			voutList[i].ScriptPubKey.Addresses = addrList
		}

		disbuf, _ := btcscript.DisasmString(v.PkScript)
		voutList[i].ScriptPubKey.Asm = disbuf
		voutList[i].ScriptPubKey.Hex = hex.EncodeToString(v.PkScript)
		voutList[i].ScriptPubKey.ReqSigs = strings.Count(disbuf, "OP_CHECKSIG")
		voutList[i].ScriptPubKey.Type = btcscript.GetScriptClass(v.PkScript).String()
	}

	txReply := &btcjson.TxRawResult{
		Hex:      mtxHex,
		Txid:     txSha,
		Vout:     voutList,
		Vin:      vinList,
		Version:  mtx.Version,
		LockTime: mtx.LockTime,
	}

	if blk != nil {
		blockHeader := &blk.MsgBlock().Header
		idx := blk.Height()

		// This is not a typo, they are identical in
		// bitcoind as well.
		txReply.Time = blockHeader.Timestamp.Unix()
		txReply.Blocktime = blockHeader.Timestamp.Unix()
		txReply.BlockHash = blksha.String()
		txReply.Confirmations = uint64(1 + maxidx - idx)
	}

	return txReply, nil
}

// handleSendRawTransaction implements the sendrawtransaction command.
func handleSendRawTransaction(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
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

	// If called from websocket code, add a mined tx hashes
	// request.
	if walletNotification != nil {
		s.ws.AddMinedTxRequest(walletNotification, tx.Sha())
	}

	return tx.Sha().String(), nil
}

// handleSetGenerate implements the setgenerate command.
func handleSetGenerate(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
	// btcd does not do mining so we can hardcode replies here.
	return nil, nil
}

// handleStop implements the stop command.
func handleStop(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
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

func handleVerifyChain(s *rpcServer, cmd btcjson.Cmd, walletNotification chan []byte) (interface{}, error) {
	c := cmd.(*btcjson.VerifyChainCmd)

	err := verifyChain(s.server.db, c.CheckLevel, c.CheckDepth)
	if err != nil {
	}
	return "", nil
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
func standardCmdReply(cmd btcjson.Cmd, s *rpcServer,
	walletNotification chan []byte) (reply btcjson.Reply) {

	id := cmd.Id()
	reply.Id = &id

	handler, ok := handlers[cmd.Method()]
	if !ok {
		reply.Error = &btcjson.ErrMethodNotFound
		return reply
	}

	result, err := handler(s, cmd, walletNotification)
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

// respondToAnyCmd checks that a parsed command is a standard or
// extension JSON-RPC command and runs the proper handler to reply to
// the command.  Any and all responses are sent to the wallet from
// this function.
func respondToAnyCmd(cmd btcjson.Cmd, s *rpcServer,
	walletNotification chan []byte, rc *requestContexts) {

	reply := standardCmdReply(cmd, s, walletNotification)
	if reply.Error != &btcjson.ErrMethodNotFound {
		mreply, _ := json.Marshal(reply)
		walletNotification <- mreply
		return
	}

	wsHandler, ok := wsHandlers[cmd.Method()]
	if !ok {
		reply.Error = &btcjson.ErrMethodNotFound
		mreply, _ := json.Marshal(reply)
		walletNotification <- mreply
		return
	}

	if err := wsHandler(s, cmd, walletNotification, rc); err != nil {
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
		rpcsLog.Info("Begining rescan for 1 address.")
	} else {
		rpcsLog.Infof("Begining rescan for %v addresses.",
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
				rpcsLog.Errorf("Error looking up block sha: %v", err)
				return err
			}
			txs := blk.Transactions()
			for _, tx := range txs {
				var txReply *btcdb.TxListReply
				for txOutIdx, txout := range tx.MsgTx().TxOut {
					st, txaddrhash, err := btcscript.ScriptToAddrHash(txout.PkScript)
					if st != btcscript.ScriptAddr || err != nil {
						continue
					}
					txaddr, err := btcutil.EncodeAddress(txaddrhash, s.server.btcnet)
					if err != nil {
						rpcsLog.Errorf("Error encoding address: %v", err)
						return err
					}

					if _, ok := rescanCmd.Addresses[txaddr]; ok {
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
							Receiver:   txaddr,
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
		hash, _, err := btcutil.DecodeAddress(addr)
		if err != nil {
			return fmt.Errorf("cannot decode address: %v", err)
		}
		s.ws.AddTxRequest(walletNotification, rc, string(hash),
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
		s.newBlockNotifyCheckTxOut(block, tx)
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

// newBlockNotifyCheckTxOut is a helper function to iterate through
// each transaction output of a new block and perform any checks and
// notify listening frontends when necessary.
func (s *rpcServer) newBlockNotifyCheckTxOut(block *btcutil.Block,
	tx *btcutil.Tx) {

	for i, txout := range tx.MsgTx().TxOut {
		stype, txaddrhash, err := btcscript.ScriptToAddrHash(txout.PkScript)
		if stype != btcscript.ScriptAddr || err != nil {
			// Only support pay-to-pubkey-hash right now.
			continue
		}
		if idlist, ok := s.ws.txNotifications[string(txaddrhash)]; ok {
			for e := idlist.Front(); e != nil; e = e.Next() {
				ctx := e.Value.(*notificationCtx)

				blkhash, err := block.Sha()
				if err != nil {
					rpcsLog.Error("Error getting block sha; dropping Tx notification.")
					break
				}
				txaddr, err := btcutil.EncodeAddress(txaddrhash, s.server.btcnet)
				if err != nil {
					rpcsLog.Error("Error encoding address; dropping Tx notification.")
					break
				}
				reply := &btcjson.Reply{
					Result: struct {
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
						Receiver:   txaddr,
						Height:     block.Height(),
						BlockHash:  blkhash.String(),
						BlockIndex: tx.Index(),
						BlockTime:  block.MsgBlock().Header.Timestamp.Unix(),
						TxID:       tx.Sha().String(),
						TxOutIndex: uint32(i),
						Amount:     txout.Value,
						PkScript:   btcutil.Base58Encode(txout.PkScript),
					},
					Error: nil,
					Id:    &ctx.id,
				}
				replyBytes, err := json.Marshal(reply)
				if err != nil {
					rpcsLog.Errorf("Unable to marshal tx notification: %v", err)
					continue
				}
				ctx.connection <- replyBytes
			}
		}
	}
}
