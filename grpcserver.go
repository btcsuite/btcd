package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcrpc"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"golang.org/x/crypto/ripemd160"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// This makes sure the grpcServer struct correctly implements the
// gRPC server interfaces.
var _ btcrpc.BtcdServer = (*grpcServer)(nil)
var _ btcrpc.RegtestServer = (*grpcServer)(nil)

// grpcServer provides an implementation of the gRPC interface.
type grpcServer struct {
	started  int32 // To be used atomically.
	shutdown int32 // To be used atomically.

	server     *server
	listeners  []net.Listener
	grpcServer *grpc.Server

	subscribe chan *rpcEventSubscription
	events    chan interface{}

	wg   sync.WaitGroup
	quit chan struct{}
}

// newGRPCServer creates a new RPC server for the given server.
func newGRPCServer(server *server, listeners []net.Listener, tlsConfig *tls.Config) *grpcServer {
	serverOpts := []grpc.ServerOption{}

	if tlsConfig != nil {
		tlsCredentials := credentials.NewTLS(tlsConfig)
		serverOpts = append(serverOpts, grpc.Creds(tlsCredentials))
	}

	s := &grpcServer{
		server:     server,
		listeners:  listeners,
		grpcServer: grpc.NewServer(serverOpts...),
		subscribe:  make(chan *rpcEventSubscription),
		events:     make(chan interface{}),
		quit:       make(chan struct{}),
	}

	btcrpc.RegisterBtcdServer(s.grpcServer, s)
	if cfg.RegressionTest {
		btcrpc.RegisterRegtestServer(s.grpcServer, s)
	}

	s.server.chain.Subscribe(s.handleBlockchainNotification)

	return s
}

// rpcEventTxAccepted indicates a new tx was accepted into the mempool.
type rpcEventTxAccepted mempool.TxDesc

// rpcEventBlockConnected indicates a new block connected to the current best
// chain.
type rpcEventBlockConnected btcutil.Block

// rpcEventBlockDisconnected indicates a block that was disconnected from the
// current best chain.
type rpcEventBlockDisconnected btcutil.Block

// rpcEventSubscription represents a subscription to events from the RPC server.
type rpcEventSubscription struct {
	in          chan interface{} // rpc events to be put by the dispatcher
	out         chan interface{} // rpc events to be read by the client
	unsubscribe chan struct{}    // close to unsubscribe
}

// Events returns the channel clients listen to to get new events.
func (s *rpcEventSubscription) Events() <-chan interface{} {
	return s.out
}

// Unsubscribe is to be called by the client to stop the subscription.
func (s *rpcEventSubscription) Unsubscribe() {
	close(s.unsubscribe)
}

// subscribeEvents returns a new subscription to all the events the RPC server
// receives.
func (s *grpcServer) subscribeEvents() *rpcEventSubscription {
	sub := &rpcEventSubscription{
		in:          make(chan interface{}),
		out:         make(chan interface{}),
		unsubscribe: make(chan struct{}),
	}

	// Start a queue handler for the subscription so that slow connections don't
	// hold up faster ones.
	go func() {
		s.wg.Add(1)
		queueHandler(sub.in, sub.out, s.quit)
		s.wg.Done()
	}()

	s.subscribe <- sub
	return sub
}

// runEventDispatcher runs a process that will forward new incoming events to
// all the currently active client processes.
//
// It should be run in a goroutine and calls Done on the wait group on finish.
func (s *grpcServer) runEventDispatcher() {
	defer s.wg.Done()

	subscriptions := make(map[*rpcEventSubscription]struct{})
	for {
		select {
		case newSub := <-s.subscribe:
			subscriptions[newSub] = struct{}{}

		case event := <-s.events:
			// Dispatch to all clients.
			for sub := range subscriptions {
				select {
				case <-sub.unsubscribe:
					// Client unsubscribed.
					delete(subscriptions, sub)

				case sub.in <- event:
				}
			}

		case <-s.quit:
			for sub := range subscriptions {
				close(sub.in)
			}
			return
		}
	}
}

// dispatchEvent dispatches an event and makes sure it doesn't block when the
// server is shutting down.
func (s *grpcServer) dispatchEvent(event interface{}) {
	select {
	case s.events <- event:
	case <-s.quit:
	}
}

// HandleNewAcceptedTransactions is called by the server when new transactions
// are accepted in the mempool.
func (s *grpcServer) HandleNewAcceptedTransactions(txs []*mempool.TxDesc) {
	for _, txDesc := range txs {
		s.dispatchEvent((*rpcEventTxAccepted)(txDesc))
	}
}

// handleBlockchainNotification handles the callback from the blockchain package
// that notifies the RPC server about changes in the chain.
func (s *grpcServer) handleBlockchainNotification(notification *blockchain.Notification) {
	switch notification.Type {
	case blockchain.NTBlockAccepted:
		// only relevant for getblocktemplate

	case blockchain.NTBlockConnected:
		block, ok := notification.Data.(*btcutil.Block)
		if !ok {
			rpcsLog.Warnf("Chain connected notification is not a block.")
			break
		}
		s.dispatchEvent((*rpcEventBlockConnected)(block))

	case blockchain.NTBlockDisconnected:
		block, ok := notification.Data.(*btcutil.Block)
		if !ok {
			rpcsLog.Warnf("Chain disconnected notification is not a block.")
			break
		}
		s.dispatchEvent((*rpcEventBlockDisconnected)(block))
	}
}

// Start starts the RPC server by starting to listen on all it's listeners
// and subscribing to blockchain notifications.
func (s *grpcServer) Start() {
	if atomic.AddInt32(&s.started, 1) != 1 {
		return // already started or starting
	}

	for _, listener := range s.listeners {
		s.wg.Add(1)
		go func(listener net.Listener) {
			rpcsLog.Infof("RPC server listening on %s", listener.Addr())
			if err := s.grpcServer.Serve(listener); err != nil {
				rpcsLog.Errorf("RPC server shut down: %v", err)
			} else {
				rpcsLog.Tracef("RPC listener done for %s", listener.Addr())
			}
			s.wg.Done()
		}(listener)
	}

	s.wg.Add(1)
	go s.runEventDispatcher()
}

func (s *grpcServer) Stop() {
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		rpcsLog.Infof("RPC server is already in the process of shutting down")
		return
	}
	rpcsLog.Infof("RPC server gracefully shutting down")

	// This method also closes all the net listeners.
	s.grpcServer.GracefulStop()

	close(s.quit)
	s.wg.Wait()
	rpcsLog.Infof("RPC server shutdown complete")
}

// handleNewTransactions is called by the server to notify the RPC server of
// new transactions being accepted into the mempool.
func (s *grpcServer) handleNewTransactions(txs []*mempool.TxDesc) {
	for _, txDesc := range txs {
		s.dispatchEvent((*rpcEventTxAccepted)(txDesc))
	}
}

// SYSTEM MANAGEMENT

func (s *grpcServer) GetSystemInfo(ctx context.Context, req *btcrpc.GetSystemInfoRequest) (*btcrpc.GetSystemInfoResponse, error) {
	totalBytesReceived, totalBytesSent := s.server.NetTotals()
	return &btcrpc.GetSystemInfoResponse{
		Version:            uint32(1000000*appMajor + 10000*appMinor + 100*appPatch),
		ProtocolVersion:    uint32(maxProtocolVersion),
		CurrentTimeMillis:  time.Now().UnixNano() / int64(time.Millisecond),
		RunningTime:        time.Now().Unix() - s.server.startupTime,
		TotalBytesReceived: totalBytesReceived,
		TotalBytesSent:     totalBytesSent,
		Proxy:              cfg.Proxy,
		NbConnections:      uint32(s.server.ConnectedCount()),
	}, nil
}

func (s *grpcServer) SetDebugLevel(ctx context.Context, req *btcrpc.SetDebugLevelRequest) (*btcrpc.SetDebugLevelResponse, error) {
	subsystem := req.GetSubsystem().String()
	level := req.GetLevel().String()
	if subsystem == "ALL" {
		// Change the logging level for all subsystems.
		setLogLevels(level)
	} else {
		setLogLevel(subsystem, level)
	}

	return &btcrpc.SetDebugLevelResponse{}, nil
}

func (s *grpcServer) StopDaemon(ctx context.Context, req *btcrpc.StopDaemonRequest) (*btcrpc.StopDaemonResponse, error) {
	select {
	case shutdownRequestChannel <- struct{}{}:
	default:
	}
	return &btcrpc.StopDaemonResponse{}, nil
}

// NETWORK

func (s *grpcServer) GetNetworkInfo(ctx context.Context, req *btcrpc.GetNetworkInfoRequest) (*btcrpc.GetNetworkInfoResponse, error) {
	best := s.server.chain.BestSnapshot()

	var net btcrpc.GetNetworkInfoResponse_BitcoinNet
	switch s.server.chainParams.Net {
	case wire.MainNet:
		net = btcrpc.GetNetworkInfoResponse_MAINNET
	case wire.TestNet:
		net = btcrpc.GetNetworkInfoResponse_TESTNET
	case wire.TestNet3:
		net = btcrpc.GetNetworkInfoResponse_TESTNET3
	case wire.SimNet:
		net = btcrpc.GetNetworkInfoResponse_SIMNET
	}

	hashrate, err := calculateNetworkHashRate(
		s.server.chain, s.server.chainParams, best.Height, 0)
	if err != nil {
		return nil, err
	}

	return &btcrpc.GetNetworkInfoResponse{
		BitcoinNet: net,
		BestHeight: best.Height,
		TimeOffset: int64(s.server.timeSource.Offset().Seconds()),
		Difficulty: getDifficultyRatio(best.Bits, s.server.chainParams),
		Hashrate:   hashrate,
		RelayFee:   cfg.minRelayTxFee.ToBTC(),
	}, nil
}

func (s *grpcServer) GetNetworkHashRate(ctx context.Context, req *btcrpc.GetNetworkHashRateRequest) (*btcrpc.GetNetworkHashRateResponse, error) {
	hashrate, err := calculateNetworkHashRate(
		s.server.chain, s.server.chainParams,
		req.GetEndHeight(), req.GetNbBlocks())
	if err != nil {
		return nil, err
	}

	return &btcrpc.GetNetworkHashRateResponse{Hashrate: hashrate}, err
}

// MEMPOOL

func (s *grpcServer) GetMempoolInfo(ctx context.Context, req *btcrpc.GetMempoolInfoRequest) (*btcrpc.GetMempoolInfoResponse, error) {
	mempoolTxs := s.server.txMemPool.TxDescs()

	var numBytes uint64
	for _, txDesc := range mempoolTxs {
		numBytes += uint64(txDesc.Tx.MsgTx().SerializeSize())
	}

	return &btcrpc.GetMempoolInfoResponse{
		NbTransactions: uint32(len(mempoolTxs)),
		NbBytes:        numBytes,
	}, nil
}

func (s *grpcServer) GetMempool(ctx context.Context, req *btcrpc.GetMempoolRequest) (*btcrpc.GetMempoolResponse, error) {
	mempoolTxs := s.server.txMemPool.TxDescs()

	if !req.GetFullTransactions() {
		// Just return the hashes.
		hashes := make([][]byte, len(mempoolTxs))
		for i, txDesc := range mempoolTxs {
			hashes[i] = txDesc.Tx.Hash()[:]
		}
		return &btcrpc.GetMempoolResponse{
			TransactionHashes: hashes,
		}, nil
	}

	txs := make([]*btcrpc.MempoolTransaction, len(mempoolTxs))
	var buf bytes.Buffer
	for i, txDesc := range mempoolTxs {
		if err := txDesc.Tx.MsgTx().BtcEncode(&buf, maxProtocolVersion, wire.WitnessEncoding); err != nil {
			rpcsLog.Warnf("Failed to encode mempool tx %v", txDesc.Tx.Hash())
			return nil, err
		}
		txBytes := buf.Bytes()
		buf.Reset()
		mtx := txDesc.Tx.MsgTx()

		//TODO(stevenroose) consider if these are relevant
		//Size:     int32(mtx.SerializeSize()),
		//Vsize:    int32(mempool.GetTxVirtualSize(btcutil.NewTx(mtx))),
		witnessHash := mtx.WitnessHash()
		txs[i] = &btcrpc.MempoolTransaction{
			Transaction: &btcrpc.Transaction{
				Hash:          txDesc.Tx.Hash()[:],
				Serialized:    txBytes,
				Version:       mtx.Version,
				Inputs:        createTxInputList(mtx),
				Outputs:       createTxOutputList(mtx, s.server.chainParams, nil),
				LockTime:      mtx.LockTime,
				HasWitness:    mtx.HasWitness(),
				WitnessHash:   witnessHash[:],
				Time:          txDesc.Added.Unix(),
				Confirmations: 0,
				BlockHash:     nil,
			},
			AddedTime:        txDesc.Added.Unix(),
			Fee:              txDesc.Fee,
			FeePerByte:       txDesc.FeePerKB,
			Height:           txDesc.Height,
			StartingPriority: txDesc.StartingPriority,
		}
	}

	return &btcrpc.GetMempoolResponse{
		Transactions: txs,
	}, nil
}

func (s *grpcServer) GetRawMempool(ctx context.Context, req *btcrpc.GetRawMempoolRequest) (*btcrpc.GetRawMempoolResponse, error) {
	mempoolTxs := s.server.txMemPool.TxDescs()

	rawTxs := make([][]byte, len(mempoolTxs))
	var buf bytes.Buffer
	for i, txDesc := range mempoolTxs {
		if err := txDesc.Tx.MsgTx().BtcEncode(&buf, maxProtocolVersion, wire.WitnessEncoding); err != nil {
			rpcsLog.Warnf("Failed to encode mempool tx %v", txDesc.Tx.Hash())
			return nil, err
		}
		rawTxs[i] = buf.Bytes()
		buf.Reset()
	}

	return &btcrpc.GetRawMempoolResponse{
		Transactions: rawTxs,
	}, nil
}

// BLOCKS

// getBlockInfo prepares a BlockInfo struct for usage in RPC responses.
// If only one of height and hash are provided, the other will be determined.
// This method assumes that not both are not provided.
func (s *grpcServer) getBlockInfo(height int32, hash *chainhash.Hash) (*btcrpc.BlockInfo, error) {
	// Make sure we have both hash and height.
	var err error
	if hash == nil {
		hash, err = s.server.chain.BlockHashByHeight(height)
		if err != nil {
			return nil, err
		}
	} else if height == 0 {
		height, err = s.server.chain.BlockHeightByHash(hash)
		if err != nil {
			return nil, err
		}
	}

	best := s.server.chain.BestSnapshot()
	var nextHashBytes []byte
	if height < best.Height {
		nextHash, err := s.server.chain.BlockHashByHeight(height + 1)
		if err != nil {
			return nil, err
		}
		nextHashBytes = nextHash[:]
	}

	header, err := s.server.chain.FetchHeader(hash)
	if err != nil {
		return nil, err
	}

	return &btcrpc.BlockInfo{
		Hash:          hash[:],
		Height:        height,
		Confirmations: 1 + best.Height - height,
		Version:       header.Version,
		PreviousBlock: header.PrevBlock[:],
		MerkleRoot:    header.MerkleRoot[:],
		Time:          header.Timestamp.Unix(),
		Bits:          header.Bits,
		Nonce:         header.Nonce,
		Difficulty:    getDifficultyRatio(header.Bits, s.server.chainParams),
		NextBlockHash: nextHashBytes,
	}, nil
}

func (s *grpcServer) GetBlockInfo(ctx context.Context, req *btcrpc.GetBlockInfoRequest) (*btcrpc.GetBlockInfoResponse, error) {
	var hash *chainhash.Hash
	if req.Locator.GetHash() != nil {
		var err error
		hash, err = chainhash.NewHash(req.Locator.GetHash())
		if err != nil {
			return nil, err
		}
	}

	info, err := s.getBlockInfo(req.Locator.GetHeight(), hash)
	if err != nil {
		return nil, err
	}

	return &btcrpc.GetBlockInfoResponse{Info: info}, nil
}

func (s *grpcServer) GetBestBlockInfo(ctx context.Context, req *btcrpc.GetBestBlockInfoRequest) (*btcrpc.GetBestBlockInfoResponse, error) {
	best := s.server.chain.BestSnapshot()
	info, err := s.getBlockInfo(best.Height, &best.Hash)
	if err != nil {
		return nil, err
	}

	return &btcrpc.GetBestBlockInfoResponse{Info: info}, nil
}

// createRPCBlock turns a btcutil.Block into a btcrpc.Block.
func createRPCBlock(block *btcutil.Block, bestHeight int32, fullTxs bool, params *chaincfg.Params, nextHash *chainhash.Hash) (*btcrpc.Block, error) {
	header := block.MsgBlock().Header

	var txHashes [][]byte
	var rpcTxs []*btcrpc.Transaction
	if fullTxs {
		txs := block.Transactions()
		rpcTxs = make([]*btcrpc.Transaction, len(txs))
		var buf bytes.Buffer
		for i, tx := range txs {
			mtx := tx.MsgTx()
			if err := mtx.BtcEncode(&buf, maxProtocolVersion, wire.WitnessEncoding); err != nil {
				rpcsLog.Warnf("Failed to encode tx %v", tx.Hash())
				return nil, err
			}
			txBytes := buf.Bytes()
			buf.Reset()

			witnessHash := mtx.WitnessHash()
			rpcTxs[i] = &btcrpc.Transaction{
				Hash:          tx.Hash()[:],
				Serialized:    txBytes,
				Version:       mtx.Version,
				Inputs:        createTxInputList(mtx),
				Outputs:       createTxOutputList(mtx, params, nil),
				LockTime:      mtx.LockTime,
				HasWitness:    mtx.HasWitness(),
				WitnessHash:   witnessHash[:],
				Time:          block.MsgBlock().Header.Timestamp.Unix(),
				Confirmations: 1 + bestHeight - block.Height(),
				BlockHash:     block.Hash()[:],
			}
		}
	} else {
		txs := block.Transactions()
		txHashes = make([][]byte, len(txs))
		for i, tx := range txs {
			txHashes[i] = tx.Hash()[:]
		}
	}

	var nextHashBytes []byte
	if nextHash != nil {
		nextHashBytes = nextHash[:]
	}
	return &btcrpc.Block{
		Info: &btcrpc.BlockInfo{
			Hash:          block.Hash()[:],
			Height:        block.Height(),
			Confirmations: 1 + bestHeight - block.Height(),
			Version:       header.Version,
			PreviousBlock: header.PrevBlock[:],
			MerkleRoot:    header.MerkleRoot[:],
			Time:          header.Timestamp.Unix(),
			Bits:          header.Bits,
			Nonce:         header.Nonce,
			Difficulty:    getDifficultyRatio(header.Bits, params),
			NextBlockHash: nextHashBytes,
		},
		TransactionHashes: txHashes,
		Transactions:      rpcTxs,
	}, nil
}

func (s *grpcServer) GetBlock(ctx context.Context, req *btcrpc.GetBlockRequest) (*btcrpc.GetBlockResponse, error) {
	var block *btcutil.Block
	if hashBytes := req.Locator.GetHash(); hashBytes != nil {
		hash, err := chainhash.NewHash(hashBytes)
		if err != nil {
			return nil, err
		}

		block, err = s.server.chain.BlockByHash(hash)
		if err != nil {
			return nil, err
		}
	} else {
		b, err := s.server.chain.BlockByHeight(req.Locator.GetHeight())
		if err != nil {
			return nil, err
		}
		block = b
	}

	nextHash, err := s.server.chain.BlockHashByHeight(block.Height() + 1)
	if err != nil {
		nextHash = nil
	}

	best := s.server.chain.BestSnapshot()
	rpcBlock, err := createRPCBlock(block, best.Height,
		req.GetFullTransactions(), s.server.chainParams, nextHash)
	if err != nil {
		return nil, err
	}

	return &btcrpc.GetBlockResponse{
		Block: rpcBlock,
	}, nil
}

func (s *grpcServer) GetRawBlock(ctx context.Context, req *btcrpc.GetRawBlockRequest) (*btcrpc.GetRawBlockResponse, error) {
	var hash *chainhash.Hash
	var err error
	if hashBytes := req.Locator.GetHash(); hashBytes != nil {
		hash, err = chainhash.NewHash(hashBytes)
		if err != nil {
			return nil, err
		}
	} else {
		hash, err = s.server.chain.BlockHashByHeight(req.Locator.GetHeight())
		if err != nil {
			return nil, err
		}
	}

	var raw []byte
	err = s.server.db.View(func(tx database.Tx) error {
		var err error
		raw, err = tx.FetchBlock(hash)
		return err
	})
	if err != nil {
		return nil, err
	}

	return &btcrpc.GetRawBlockResponse{Block: raw}, nil
}

func (s *grpcServer) SubmitBlock(ctx context.Context, req *btcrpc.SubmitBlockRequest) (*btcrpc.SubmitBlockResponse, error) {
	block, err := btcutil.NewBlockFromBytes(req.GetBlock())
	if err != nil {
		return nil, err
	}

	_, err = s.server.syncManager.ProcessBlock(block, blockchain.BFNone)
	if err != nil {
		//TODO(stevenroose) rejected
		return nil, err
	}

	return &btcrpc.SubmitBlockResponse{Hash: block.Hash()[:]}, nil
}

// TRANSACTIONS

// createTxInputList constructs a slice of btcrpc.Transaction_Input objects
// from the given transaction.
func createTxInputList(mtx *wire.MsgTx) []*btcrpc.Transaction_Input {
	// Coinbase transactions only have a single txin by definition.
	vinList := make([]*btcrpc.Transaction_Input, len(mtx.TxIn))
	if blockchain.IsCoinBaseTx(mtx) {
		txIn := mtx.TxIn[0]
		vinList[0].Coinbase = true
		vinList[0].SignatureScript = txIn.SignatureScript
		vinList[0].Sequence = txIn.Sequence
		vinList[0].WitnessData = txIn.Witness
		return vinList
	}

	for i, txIn := range mtx.TxIn {
		vinEntry := vinList[i]
		vinEntry.Outpoint = &btcrpc.Transaction_Input_Outpoint{
			Hash:  txIn.PreviousOutPoint.Hash[:],
			Index: txIn.PreviousOutPoint.Index,
		}
		vinEntry.SignatureScript = txIn.SignatureScript
		vinEntry.Sequence = txIn.Sequence
		vinEntry.WitnessData = txIn.Witness
		//TODO(stevenroose) value
	}

	return vinList
}

// createTxOutputList constructs a slice of btcrpc.Transaction_Output objects
// from the given transaction.
//TODO(stevenroose) the filter thing is currently not used
func createTxOutputList(mtx *wire.MsgTx, params *chaincfg.Params, filterAddrMap map[string]struct{}) []*btcrpc.Transaction_Output {
	voutList := make([]*btcrpc.Transaction_Output, 0, len(mtx.TxOut))
	for i, v := range mtx.TxOut {
		// Ignore the error here since an error means the script
		// couldn't parse and there is no additional information about
		// it anyways.
		_, addrs, _, _ := txscript.ExtractPkScriptAddrs(v.PkScript, params)

		// Encode the addresses while checking if the address passes the
		// filter when needed.
		passesFilter := len(filterAddrMap) == 0
		encodedAddrs := make([]string, len(addrs))
		for j, addr := range addrs {
			encodedAddr := addr.EncodeAddress()
			encodedAddrs[j] = encodedAddr

			// No need to check the map again if the filter already
			// passes.
			if passesFilter {
				continue
			}
			if _, exists := filterAddrMap[encodedAddr]; exists {
				passesFilter = true
			}
		}

		if !passesFilter {
			continue
		}

		vout := &btcrpc.Transaction_Output{
			Index:        uint32(i),
			Value:        v.Value,
			PubkeyScript: v.PkScript,
		}
		//TODO(stevenroose) should we put these too?
		//var vout btcjson.Vout
		//vout.N = uint32(i)
		//vout.ScriptPubKey.Addresses = encodedAddrs
		//vout.ScriptPubKey.Asm = disbuf
		//vout.ScriptPubKey.Type = scriptClass.String() //scriptClass is first param in ExtractPkScriptAddrs
		//vout.ScriptPubKey.ReqSigs = int32(reqSigs) // ReqSigs is third param in ExtractPkScriptAddrs

		voutList = append(voutList, vout)
	}

	return voutList
}

func (s *grpcServer) GetTransaction(ctx context.Context, req *btcrpc.GetTransactionRequest) (*btcrpc.GetTransactionResponse, error) {
	txHash, err := chainhash.NewHash(req.GetHash())
	if err != nil {
		return nil, err
	}

	// A set of variables that we need to retrieve differently for
	//mempool txs and blockchain txs.
	var (
		txBytes        []byte
		mtx            *wire.MsgTx
		blockHashBytes []byte
		time           time.Time
		confirmations  int32
	)
	txDesc, err := s.server.txMemPool.FetchTxDesc(txHash)
	if err == nil {
		var buf bytes.Buffer
		if err := txDesc.Tx.MsgTx().BtcEncode(&buf, maxProtocolVersion, wire.WitnessEncoding); err != nil {
			rpcsLog.Warnf("Failed to encode mempool tx %v", txDesc.Tx.Hash())
			return nil, err
		}
		txBytes = buf.Bytes()
		mtx = txDesc.Tx.MsgTx()
		time = txDesc.Added
	} else {
		var blockHash *chainhash.Hash
		txBytes, blockHash, err = s.fetchRawTransaction(txHash)
		if err != nil {
			return nil, err
		}
		blockHashBytes = blockHash[:]
		var msgTx wire.MsgTx
		if err := msgTx.Deserialize(bytes.NewReader(txBytes)); err != nil {
			return nil, err
		}
		mtx = &msgTx
		header, err := s.server.chain.FetchHeader(blockHash)
		if err != nil {
			return nil, err
		}
		time = header.Timestamp
		bestHeight := s.server.chain.BestSnapshot().Height
		blockHeight, err := s.server.chain.BlockHeightByHash(blockHash)
		if err != nil {
			return nil, err
		}
		confirmations = 1 + bestHeight - blockHeight
	}

	//TODO(stevenroose) consider if these are relevant
	//Size:     int32(mtx.SerializeSize()),
	//Vsize:    int32(mempool.GetTxVirtualSize(btcutil.NewTx(mtx))),
	witnessHash := mtx.WitnessHash()
	return &btcrpc.GetTransactionResponse{
		Transaction: &btcrpc.Transaction{
			Hash:          txHash[:],
			Serialized:    txBytes,
			Version:       mtx.Version,
			Inputs:        createTxInputList(mtx),
			Outputs:       createTxOutputList(mtx, s.server.chainParams, nil),
			LockTime:      mtx.LockTime,
			HasWitness:    mtx.HasWitness(),
			WitnessHash:   witnessHash[:],
			Time:          time.Unix(),
			Confirmations: confirmations,
			BlockHash:     blockHashBytes,
		},
	}, nil
}

// fetchRawTransaction fetches a raw transaction from the database.
// It returns an error if the tx index is not enabled.
// It returns the raw transaction bytes and the block hash.
func (s *grpcServer) fetchRawTransaction(hash *chainhash.Hash) ([]byte, *chainhash.Hash, error) {
	if s.server.txIndex == nil {
		return nil, nil, errors.New("txindex required for this call")
	}

	// Look up the location of the transaction.
	blockRegion, err := s.server.txIndex.TxBlockRegion(hash)
	if err != nil {
		return nil, nil, err
	}
	if blockRegion == nil {
		return nil, nil, errors.New("tx not found")
	}

	// Load the raw transaction bytes from the database into the txBytes var.
	var txBytes []byte
	err = s.server.db.View(func(dbTx database.Tx) error {
		var err error
		txBytes, err = dbTx.FetchBlockRegion(blockRegion)
		return err
	})
	if err != nil {
		return nil, nil, errors.New("tx not found")
	}

	return txBytes, blockRegion.Hash, nil
}

func (s *grpcServer) GetRawTransaction(ctx context.Context, req *btcrpc.GetRawTransactionRequest) (*btcrpc.GetRawTransactionResponse, error) {
	txHash, err := chainhash.NewHash(req.GetHash())
	if err != nil {
		return nil, err
	}

	var txBytes []byte
	tx, err := s.server.txMemPool.FetchTransaction(txHash)
	if err == nil {
		var buf bytes.Buffer
		if err := tx.MsgTx().BtcEncode(&buf, maxProtocolVersion, wire.WitnessEncoding); err != nil {
			rpcsLog.Warnf("Failed to encode mempool tx %v", tx.Hash())
			return nil, err
		}
		txBytes = buf.Bytes()
	} else {
		txBytes, _, err = s.fetchRawTransaction(txHash)
		if err != nil {
			return nil, err
		}
	}

	return &btcrpc.GetRawTransactionResponse{Transaction: txBytes}, nil
}

// txFilter is used to filter transactions based on a clients interest.
// It supports filtering for TX outpoints and all kinds of addresses.
type txFilter struct {
	outpoints           map[wire.OutPoint]struct{}
	pubKeyHashes        map[[ripemd160.Size]byte]struct{}
	scriptHashes        map[[ripemd160.Size]byte]struct{}
	compressedPubKeys   map[[33]byte]struct{}
	uncompressedPubKeys map[[65]byte]struct{}
	fallbacks           map[string]struct{}
}

// newTxFilter creates a new txFilter.
func newTxFilter() *txFilter {
	return &txFilter{
		outpoints:           map[wire.OutPoint]struct{}{},
		pubKeyHashes:        map[[ripemd160.Size]byte]struct{}{},
		scriptHashes:        map[[ripemd160.Size]byte]struct{}{},
		compressedPubKeys:   map[[33]byte]struct{}{},
		uncompressedPubKeys: map[[65]byte]struct{}{},
		fallbacks:           map[string]struct{}{},
	}
}

// AddOutpoint adds a new outpoint to the filter.
func (f *txFilter) AddOutpoint(op wire.OutPoint) {
	f.outpoints[op] = struct{}{}
}

// RemoveOutpoint removes an outpoint from the filter.
func (f *txFilter) RemoveOutpoint(op wire.OutPoint) {
	delete(f.outpoints, op)
}

// AddAddress adds a new address to the filter.
func (f *txFilter) AddAddress(addr btcutil.Address) {
	switch a := addr.(type) {
	case *btcutil.AddressPubKeyHash:
		f.pubKeyHashes[*a.Hash160()] = struct{}{}

	case *btcutil.AddressScriptHash:
		f.scriptHashes[*a.Hash160()] = struct{}{}

	case *btcutil.AddressPubKey:
		pubkeyBytes := a.ScriptAddress()
		switch len(pubkeyBytes) {
		case 33: // Compressed
			var compressedPubkey [33]byte
			copy(compressedPubkey[:], pubkeyBytes)
			f.compressedPubKeys[compressedPubkey] = struct{}{}

		case 65: // Uncompressed
			var uncompressedPubkey [65]byte
			copy(uncompressedPubkey[:], pubkeyBytes)
			f.uncompressedPubKeys[uncompressedPubkey] = struct{}{}
		}

	default:
		// A new address type must have been added.  Use encoded
		// payment address string as a fallback until a fast path
		// is added.
		addrStr := addr.EncodeAddress()
		rpcsLog.Infof("Unknown address type: %v", addrStr)
		f.fallbacks[addrStr] = struct{}{}
	}
}

// RemoveAddress removes an address from the filter.
func (f *txFilter) RemoveAddress(addr btcutil.Address) {
	switch a := addr.(type) {
	case *btcutil.AddressPubKeyHash:
		delete(f.pubKeyHashes, *a.Hash160())

	case *btcutil.AddressScriptHash:
		delete(f.scriptHashes, *a.Hash160())

	case *btcutil.AddressPubKey:
		pubkeyBytes := a.ScriptAddress()
		switch len(pubkeyBytes) {
		case 33: // Compressed
			var compressedPubkey [33]byte
			copy(compressedPubkey[:], pubkeyBytes)
			delete(f.compressedPubKeys, compressedPubkey)

		case 65: // Uncompressed
			var uncompressedPubkey [65]byte
			copy(uncompressedPubkey[:], pubkeyBytes)
			delete(f.uncompressedPubKeys, uncompressedPubkey)
		}

	default:
		// A new address type must have been added.  Use encoded
		// payment address string as a fallback until a fast path
		// is added.
		addrStr := addr.EncodeAddress()
		rpcsLog.Infof("Unknown address type: %v", addrStr)
		delete(f.fallbacks, addrStr)
	}
}

// AddRPCFilter adds all filter properties from the btcrpc.TransactionFilter
// to the filter.
func (f *txFilter) AddRPCFilter(rpcFilter *btcrpc.TransactionFilter, params *chaincfg.Params) error {
	// Add outpoints.
	for _, op := range rpcFilter.GetOutpoints() {
		hash, err := chainhash.NewHash(op.GetHash())
		if err != nil {
			return err
		}
		f.AddOutpoint(wire.OutPoint{Hash: *hash, Index: op.GetIndex()})
	}

	// Interpret and add addresses.
	for _, addrStr := range rpcFilter.GetAddresses() {
		addr, err := btcutil.DecodeAddress(addrStr, params)
		if err != nil {
			return fmt.Errorf("Unable to decode address '%v': %v", addrStr, err)
		}
		f.AddAddress(addr)
	}

	return nil
}

// RemoveRPCFilter removes all filter properties from the
// btcrpc.TransactionFilter from the filter.
func (f *txFilter) RemoveRPCFilter(rpcFilter *btcrpc.TransactionFilter, params *chaincfg.Params) error {
	// Remove outpoints.
	for _, op := range rpcFilter.GetOutpoints() {
		hash, err := chainhash.NewHash(op.GetHash())
		if err != nil {
			return err
		}
		f.RemoveOutpoint(wire.OutPoint{Hash: *hash, Index: op.GetIndex()})
	}

	// Interpret and remove addresses.
	for _, addrStr := range rpcFilter.GetAddresses() {
		addr, err := btcutil.DecodeAddress(addrStr, params)
		if err != nil {
			return fmt.Errorf("Unable to decode address '%v': %v", addrStr, err)
		}
		f.RemoveAddress(addr)
	}

	return nil
}

// MatchAndUpdate returns whether the transaction matches against the filter.
// When the tx contains any matching outputs, all these outputs are added to the
// filter as outpoints for matching further spends of these outputs.
// All matching outpoints are removed from the filter.
func (f *txFilter) MatchAndUpdate(tx *btcutil.Tx, params *chaincfg.Params) bool {
	// We don't return early on a match because we prefer full processing:
	// - all matching outputs need to be added to the filter for later matching
	// - all matching inputs can be removed from the filter for later efficiency

	var matched bool

	for _, txin := range tx.MsgTx().TxIn {
		if _, ok := f.outpoints[txin.PreviousOutPoint]; ok {
			delete(f.outpoints, txin.PreviousOutPoint)

			matched = true
		}
	}

	for txOutIdx, txout := range tx.MsgTx().TxOut {
		_, addrs, _, _ := txscript.ExtractPkScriptAddrs(txout.PkScript, params)

		for _, addr := range addrs {
			switch a := addr.(type) {
			case *btcutil.AddressPubKeyHash:
				if _, ok := f.pubKeyHashes[*a.Hash160()]; !ok {
					continue
				}

			case *btcutil.AddressScriptHash:
				if _, ok := f.scriptHashes[*a.Hash160()]; !ok {
					continue
				}

			case *btcutil.AddressPubKey:
				found := false
				switch sa := a.ScriptAddress(); len(sa) {
				case 33: // Compressed
					var key [33]byte
					copy(key[:], sa)
					if _, ok := f.compressedPubKeys[key]; ok {
						found = true
					}

				case 65: // Uncompressed
					var key [65]byte
					copy(key[:], sa)
					if _, ok := f.uncompressedPubKeys[key]; ok {
						found = true
					}

				default:
					rpcsLog.Warnf("Skipping rescanned pubkey of unknown "+
						"serialized length %d", len(sa))
					continue
				}

				// If the transaction output pays to the pubkey of
				// a rescanned P2PKH address, include it as well.
				if !found {
					pkh := a.AddressPubKeyHash()
					if _, ok := f.pubKeyHashes[*pkh.Hash160()]; !ok {
						continue
					}
				}

			default:
				// A new address type must have been added.  Encode as a
				// payment address string and check the fallback map.
				addrStr := addr.EncodeAddress()
				_, ok := f.fallbacks[addrStr]
				if !ok {
					continue
				}
			}

			// Matching address.
			break
		}

		// Matching output should be added to filter.
		outpoint := wire.OutPoint{
			Hash:  *tx.Hash(),
			Index: uint32(txOutIdx),
		}
		f.outpoints[outpoint] = struct{}{}

		matched = true
	}

	return matched
}

func (s *grpcServer) RescanTransactions(req *btcrpc.RescanTransactionsRequest, ret btcrpc.Btcd_RescanTransactionsServer) error {
	rpcsLog.Infof("Beginning rescan for %d addresses and %d outpoints",
		len(req.GetFilter().GetAddresses()), len(req.GetFilter().GetOutpoints()))

	// Build a tx filter for lookup.
	filter := newTxFilter()
	if err := filter.AddRPCFilter(req.GetFilter(), s.server.chainParams); err != nil {
		return err
	}

	var minBlock, maxBlock int32
	if minHashB := req.GetStartBlock().GetHash(); len(minHashB) > 0 {
		minHash, err := chainhash.NewHash(minHashB)
		if err != nil {
			return err
		}
		minBlock, err = s.server.chain.BlockHeightByHash(minHash)
		if err != nil {
			return err
		}
	}
	if maxHashB := req.GetStopBlock().GetHash(); len(maxHashB) > 0 {
		maxHash, err := chainhash.NewHash(maxHashB)
		if err != nil {
			return err
		}
		maxBlock, err = s.server.chain.BlockHeightByHash(maxHash)
		if err != nil {
			return err
		}
	}

	if minBlock <= 0 || maxBlock <= 0 {
		return errors.New("Both the start block and the stop block " +
			"should be specified")
	}

	best := s.server.chain.BestSnapshot()
	if maxBlock > best.Height {
		return fmt.Errorf("The stop block %d is higher than the current "+
			"highest block %d", maxBlock, best.Height)
	}

	// Construct the method used to push new txs on the stream.
	var buf bytes.Buffer
	sendTransaction := func(tx *btcutil.Tx, block *btcutil.Block) error {
		mtx := tx.MsgTx()
		if err := mtx.BtcEncode(&buf, maxProtocolVersion, wire.WitnessEncoding); err != nil {
			rpcsLog.Warnf("Failed to encode tx %v", tx.Hash())
			return err
		}
		txBytes := buf.Bytes()
		buf.Reset()

		witnessHash := mtx.WitnessHash()
		toSend := &btcrpc.Transaction{
			Hash:          tx.Hash()[:],
			Serialized:    txBytes,
			Version:       mtx.Version,
			Inputs:        createTxInputList(mtx),
			Outputs:       createTxOutputList(mtx, s.server.chainParams, nil),
			LockTime:      mtx.LockTime,
			HasWitness:    mtx.HasWitness(),
			WitnessHash:   witnessHash[:],
			Time:          block.MsgBlock().Header.Timestamp.Unix(),
			Confirmations: 1 + best.Height - block.Height(),
			BlockHash:     block.Hash()[:],
		}
		return ret.Send(toSend)
	}

	// The last block hash is kept to detect reorgs.
	var lastBlockHash *chainhash.Hash
	for currentBlock := minBlock; currentBlock <= maxBlock; currentBlock++ {
		block, err := s.server.chain.BlockByHeight(currentBlock)
		if err != nil {
			return err
		}

		if !block.MsgBlock().Header.PrevBlock.IsEqual(lastBlockHash) && lastBlockHash != nil {
			return fmt.Errorf("Detected reorg: block at height %d (%v) "+
				"does not follow previously scanned block at %d (%v)",
				currentBlock, block.Hash().String(),
				currentBlock-1, lastBlockHash.String())
		}
		lastBlockHash = block.Hash()

		// Scan the block for matching transactions.
		for _, tx := range block.Transactions() {
			if filter.MatchAndUpdate(tx, s.server.chainParams) {
				if err := sendTransaction(tx, block); err != nil {
					return err
				}
			}
		}

		// Stop scanning if client disconnected.
		select {
		case <-ret.Context().Done():
			rpcsLog.Info("Client disconnected during rescan")
			return errors.New("client disconnected")
		default:
		}
	}

	rpcsLog.Info("Finished rescan")
	return nil
}

func (s *grpcServer) SubmitTransaction(ctx context.Context, req *btcrpc.SubmitTransactionRequest) (*btcrpc.SubmitTransactionResponse, error) {
	var msgTx wire.MsgTx
	if err := msgTx.Deserialize(bytes.NewReader(req.GetTransaction())); err != nil {
		return nil, err
	}

	tx := btcutil.NewTx(&msgTx)
	// Use 0 for the tag to represent local node.
	acceptedTxs, err := s.server.txMemPool.ProcessTransaction(
		tx, false, false, 0)
	if err != nil {
		// When the error is a rule error, it means the transaction was
		// simply rejected as opposed to something actually going wrong,
		// so log it as such.  Otherwise, something really did go wrong,
		// so log it as an actual error.  In both cases, a JSON-RPC
		// error is returned to the client with the deserialization
		// error code (to match bitcoind behavior).
		if _, ok := err.(mempool.RuleError); ok {
			rpcsLog.Debugf("Rejected transaction %v: %v", tx.Hash(),
				err)
		} else {
			rpcsLog.Errorf("Failed to process transaction %v: %v",
				tx.Hash(), err)
		}
		return nil, err
	}

	// When the transaction was accepted it should be the first item in the
	// returned array of accepted transactions.  The only way this will not
	// be true is if the API for ProcessTransaction changes and this code is
	// not properly updated, but ensure the condition holds as a safeguard.
	//
	// Also, since an error is being returned to the caller, ensure the
	// transaction is removed from the memory pool.
	if len(acceptedTxs) == 0 || !acceptedTxs[0].Tx.Hash().IsEqual(tx.Hash()) {
		s.server.txMemPool.RemoveTransaction(tx, true)

		return nil, fmt.Errorf("transaction %v is not in accepted list",
			tx.Hash())
	}

	// Generate and relay inventory vectors for all newly accepted
	// transactions into the memory pool due to the original being
	// accepted.
	s.server.relayTransactions(acceptedTxs)

	// Notify both websocket and getblocktemplate long poll clients of all
	// newly accepted transactions.
	//TODO(stevenroose) implement notifications
	//s.NotifyNewTransactions(acceptedTxs)

	// Keep track of all the sendrawtransaction request txns so that they
	// can be rebroadcast if they don't make their way into a block.
	txDesc := acceptedTxs[0]
	iv := wire.NewInvVect(wire.InvTypeTx, txDesc.Tx.Hash())
	s.server.AddRebroadcastInventory(iv, txDesc)

	return &btcrpc.SubmitTransactionResponse{Hash: tx.Hash()[:]}, nil
}

func (s *grpcServer) GetAddressTransactions(ctx context.Context, req *btcrpc.GetAddressTransactionsRequest) (*btcrpc.GetAddressTransactionsResponse, error) {
	if s.server.addrIndex == nil {
		return nil, errors.New("addrindex required for this call")
	}

	address, err := btcutil.DecodeAddress(req.GetAddress(), s.server.chainParams)
	if err != nil {
		return nil, err
	}

	nbSkip := req.GetNbSkip()
	nbFetch := req.GetNbFetch()
	if nbFetch == 0 {
		nbFetch = math.MaxUint32
	}

	var blockRegions []database.BlockRegion
	var serializedTxs [][]byte
	err = s.server.db.View(func(tx database.Tx) error {
		var err error
		blockRegions, _, err = s.server.addrIndex.TxRegionsForAddress(
			tx, address, nbSkip, nbFetch, false)
		if err != nil {
			return err
		}

		serializedTxs, err = tx.FetchBlockRegions(blockRegions)
		return err
	})
	if err != nil {
		return nil, err
	}

	best := s.server.chain.BestSnapshot()
	rpcConfirmed := make([]*btcrpc.Transaction, len(serializedTxs))
	for i, serialized := range serializedTxs {
		tx, err := btcutil.NewTxFromBytes(serialized)
		if err != nil {
			return nil, err
		}

		blockHash := blockRegions[i].Hash
		blockHeight, err := s.server.chain.BlockHeightByHash(blockHash)
		if err != nil {
			return nil, err
		}

		mtx := tx.MsgTx()
		witnessHash := mtx.WitnessHash()
		rpcConfirmed[i] = &btcrpc.Transaction{
			Hash:          tx.Hash()[:],
			Serialized:    serialized,
			Version:       mtx.Version,
			Inputs:        createTxInputList(mtx),
			Outputs:       createTxOutputList(mtx, s.server.chainParams, nil),
			LockTime:      mtx.LockTime,
			HasWitness:    mtx.HasWitness(),
			WitnessHash:   witnessHash[:],
			Confirmations: 1 + best.Height - blockHeight,
			BlockHash:     blockHash[:],
			//TODO(stevenroose) should we fetch block to provide timestamp?
		}
	}

	unconfirmed := s.server.addrIndex.UnconfirmedTxnsForAddress(address)
	rpcUnconfirmed := make([]*btcrpc.Transaction, len(unconfirmed))
	var buf bytes.Buffer
	for i, tx := range unconfirmed {
		mtx := tx.MsgTx()
		if err := mtx.BtcEncode(&buf, maxProtocolVersion, wire.WitnessEncoding); err != nil {
			return nil, err
		}
		txBytes := buf.Bytes()
		buf.Reset()

		witnessHash := mtx.WitnessHash()
		rpcUnconfirmed[i] = &btcrpc.Transaction{
			Hash:          tx.Hash()[:],
			Serialized:    txBytes,
			Version:       mtx.Version,
			Inputs:        createTxInputList(mtx),
			Outputs:       createTxOutputList(mtx, s.server.chainParams, nil),
			LockTime:      mtx.LockTime,
			HasWitness:    mtx.HasWitness(),
			WitnessHash:   witnessHash[:],
			Confirmations: 0,
			//TODO(stevenroose) should we fetch block to provide timestamp?
		}
	}

	return &btcrpc.GetAddressTransactionsResponse{
		ConfirmedTransactions:   rpcConfirmed,
		UnconfirmedTransactions: rpcUnconfirmed,
	}, nil
}

func (s *grpcServer) GetAddressUnspentOutputs(ctx context.Context, req *btcrpc.GetAddressUnspentOutputsRequest) (*btcrpc.GetAddressUnspentOutputsResponse, error) {
	if s.server.addrIndex == nil {
		return nil, errors.New("addrindex required for this call")
	}

	addrStr := req.GetAddress()
	address, err := btcutil.DecodeAddress(addrStr, s.server.chainParams)
	if err != nil {
		return nil, err
	}

	var serializedTxs [][]byte
	err = s.server.db.View(func(tx database.Tx) error {
		regions, _, err := s.server.addrIndex.TxRegionsForAddress(
			tx, address, 0, math.MaxUint32, false)
		if err != nil {
			return err
		}

		serializedTxs, err = tx.FetchBlockRegions(regions)
		return err
	})
	if err != nil {
		return nil, err
	}

	//TODO(stevenroose) considered setting the cap at len(serializedTxs), but I
	// guess most queries have significantly less unspents than normal txs
	//unspents := make([]*btcrpc.UnspentOutput, 0, len(serializedTxs))
	unspents := []*btcrpc.UnspentOutput{}
	for _, serialized := range serializedTxs {
		tx, err := btcutil.NewTxFromBytes(serialized)
		if err != nil {
			return nil, err
		}

		utxoEntry, err := s.server.chain.FetchUtxoEntry(tx.Hash())
		if err != nil {
			return nil, err
		}

		for i, out := range tx.MsgTx().TxOut {
			if utxoEntry.IsOutputSpent(uint32(i)) {
				continue
			}

			// Check if the output is relevant for our address.
			_, addrs, _, _ := txscript.ExtractPkScriptAddrs(
				out.PkScript, s.server.chainParams)
			for _, a := range addrs {
				//TODO(stevenroose) is this check safe enough?
				if a.EncodeAddress() == addrStr {
					unspent := &btcrpc.UnspentOutput{
						Outpoint: &btcrpc.Transaction_Input_Outpoint{
							Hash:  tx.Hash()[:],
							Index: uint32(i),
						},
						PubkeyScript: out.PkScript,
						Value:        out.Value,
						IsCoinbase:   blockchain.IsCoinBase(tx),
						BlockHeight:  utxoEntry.BlockHeight(),
					}
					unspents = append(unspents, unspent)
					break
				}
			}
		}
	}

	return &btcrpc.GetAddressUnspentOutputsResponse{
		Outputs: unspents,
	}, nil
}

// SUBSCRIPTIONS

func (s *grpcServer) SubscribeTransactions(stream btcrpc.Btcd_SubscribeTransactionsServer) error {
	// Put the incoming messages on a channel.
	requests := make(chan *btcrpc.SubscribeTransactionsRequest)
	go func() {
		for {
			req, err := stream.Recv()
			if err != nil {
				close(requests)
				return
			}
			requests <- req
		}
	}()

	subscription := s.subscribeEvents()
	defer subscription.Unsubscribe()

	var buf bytes.Buffer
	filter := newTxFilter()
	for {
		select {
		case req := <-requests:
			// Update filter.
			if err := filter.AddRPCFilter(req.GetSubscribe(), s.server.chainParams); err != nil {
				return err
			}
			if err := filter.RemoveRPCFilter(req.GetUnsubscribe(), s.server.chainParams); err != nil {
				return err
			}

		case event, closed := <-subscription.Events():
			if closed {
				return errors.New("btcd is shutting down")
			}

			switch event.(type) {
			case *rpcEventTxAccepted:
				//TODO(stevenroose) should this cast be checked?
				txDesc := event.(*mempool.TxDesc)
				if !filter.MatchAndUpdate(txDesc.Tx, s.server.chainParams) {
					continue
				}

				mtx := txDesc.Tx.MsgTx()
				if err := mtx.BtcEncode(&buf, maxProtocolVersion, wire.WitnessEncoding); err != nil {
					rpcsLog.Warnf("Failed to encode mempool tx %v",
						txDesc.Tx.Hash())
					return err
				}
				txBytes := buf.Bytes()
				buf.Reset()

				//TODO(stevenroose) consider if these are relevant
				//Size:     int32(mtx.SerializeSize()),
				//Vsize:    int32(mempool.GetTxVirtualSize(btcutil.NewTx(mtx))),
				witnessHash := mtx.WitnessHash()
				toSend := &btcrpc.TransactionNotification{
					Type: btcrpc.TransactionNotification_ACCEPTED,
					Transaction: &btcrpc.TransactionNotification_AcceptedTransaction{
						AcceptedTransaction: &btcrpc.MempoolTransaction{
							Transaction: &btcrpc.Transaction{
								Hash:          txDesc.Tx.Hash()[:],
								Serialized:    txBytes,
								Version:       mtx.Version,
								Inputs:        createTxInputList(mtx),
								Outputs:       createTxOutputList(mtx, s.server.chainParams, nil),
								LockTime:      mtx.LockTime,
								HasWitness:    mtx.HasWitness(),
								WitnessHash:   witnessHash[:],
								Time:          txDesc.Added.Unix(),
								Confirmations: 0,
								BlockHash:     nil,
							},
							AddedTime:        txDesc.Added.Unix(),
							Fee:              txDesc.Fee,
							FeePerByte:       txDesc.FeePerKB,
							Height:           txDesc.Height,
							StartingPriority: txDesc.StartingPriority,
						},
					},
				}

				if err := stream.Send(toSend); err != nil {
					return err
				}

			case *rpcEventBlockConnected:
				// Search for all transactions.
				block := event.(*btcutil.Block)

				for _, tx := range block.Transactions() {
					if !filter.MatchAndUpdate(tx, s.server.chainParams) {
						continue
					}

					mtx := tx.MsgTx()
					if err := mtx.BtcEncode(&buf, maxProtocolVersion, wire.WitnessEncoding); err != nil {
						rpcsLog.Warnf("Failed to encode tx %v", tx.Hash())
						return err
					}
					txBytes := buf.Bytes()
					buf.Reset()

					//TODO(stevenroose) consider if these are relevant
					//Size:     int32(mtx.SerializeSize()),
					//Vsize:    int32(mempool.GetTxVirtualSize(btcutil.NewTx(mtx))),
					witnessHash := mtx.WitnessHash()
					toSend := &btcrpc.TransactionNotification{
						Type: btcrpc.TransactionNotification_CONFIRMED,
						Transaction: &btcrpc.TransactionNotification_ConfirmedTransaction{
							ConfirmedTransaction: &btcrpc.Transaction{
								Hash:          tx.Hash()[:],
								Serialized:    txBytes,
								Version:       mtx.Version,
								Inputs:        createTxInputList(mtx),
								Outputs:       createTxOutputList(mtx, s.server.chainParams, nil),
								LockTime:      mtx.LockTime,
								HasWitness:    mtx.HasWitness(),
								WitnessHash:   witnessHash[:],
								Time:          block.MsgBlock().Header.Timestamp.Unix(),
								Confirmations: 1,
								BlockHash:     block.Hash()[:],
							},
						},
					}

					if err := stream.Send(toSend); err != nil {
						return err
					}
				}
			}

		case <-stream.Context().Done():
			return nil
		}
	}
}

func (s *grpcServer) SubscribeBlocks(req *btcrpc.SubscribeBlocksRequest, stream btcrpc.Btcd_SubscribeBlocksServer) error {
	subscription := s.subscribeEvents()
	defer subscription.Unsubscribe()

	for {
		select {
		case event, closed := <-subscription.Events():
			if closed {
				return errors.New("btcd is shutting down")
			}

			switch event.(type) {
			case *rpcEventBlockConnected:
				// Search for all transactions.
				block := event.(*btcutil.Block)

				best := s.server.chain.BestSnapshot()
				rpcBlock, err := createRPCBlock(block, best.Height,
					req.GetFullTransactions(), s.server.chainParams, nil)
				if err != nil {
					return err
				}

				toSend := &btcrpc.BlockNotification{
					Type:  btcrpc.BlockNotification_CONNECTED,
					Block: rpcBlock,
				}

				if err := stream.Send(toSend); err != nil {
					return err
				}

			case *rpcEventBlockDisconnected:
				// Search for all transactions.
				block := event.(*btcutil.Block)

				rpcBlock, err := createRPCBlock(block, 0,
					req.GetFullTransactions(), s.server.chainParams, nil)
				if err != nil {
					return err
				}
				// Block was disconnected; manually set confirmations to 0.
				rpcBlock.Info.Confirmations = 0

				toSend := &btcrpc.BlockNotification{
					Type:  btcrpc.BlockNotification_DISCONNECTED,
					Block: rpcBlock,
				}

				if err := stream.Send(toSend); err != nil {
					return err
				}
			}

		case <-stream.Context().Done():
			return nil
		}
	}
}

// PEERS

func (s *grpcServer) GetPeers(ctx context.Context, req *btcrpc.GetPeersRequest) (*btcrpc.GetPeersResponse, error) {
	// Query the server for all connected peers.
	replyChan := make(chan []*serverPeer)
	if req.GetPermanent() {
		s.server.query <- getAddedNodesMsg{reply: replyChan}
	} else {
		s.server.query <- getPeersMsg{reply: replyChan}
	}
	peers := <-replyChan

	syncPeerID := s.server.syncManager.SyncPeerID()
	infos := make([]*btcrpc.GetPeersResponse_PeerInfo, len(peers))
	for i, p := range peers {
		statsSnap := p.StatsSnapshot()
		info := &btcrpc.GetPeersResponse_PeerInfo{
			Id:             statsSnap.ID,
			Address:        statsSnap.Addr,
			LocalAddress:   p.LocalAddr().String(),
			Version:        statsSnap.Version,
			UserAgent:      statsSnap.UserAgent,
			Services:       fmt.Sprintf("%08d", uint64(statsSnap.Services)),
			Inbound:        statsSnap.Inbound,
			SyncNode:       statsSnap.ID == syncPeerID,
			RelayTxs:       !p.disableRelayTx,
			LastSendTime:   statsSnap.LastSend.Unix(),
			LastRecvTime:   statsSnap.LastRecv.Unix(),
			ConnectionTime: statsSnap.ConnTime.Unix(),
			BytesSent:      statsSnap.BytesSent,
			BytesReceived:  statsSnap.BytesRecv,
			TimeOffset:     statsSnap.TimeOffset,
			LastPingTime:   statsSnap.LastPingTime.Unix(),
			LastPingMicros: statsSnap.LastPingMicros,
			StartingHeight: statsSnap.StartingHeight,
			CurrentHeight:  statsSnap.LastBlock,
			BanScore:       int32(p.banScore.Int()),
			FeeFilter:      atomic.LoadInt64(&p.feeFilter),
		}
		infos[i] = info
	}

	return &btcrpc.GetPeersResponse{Peers: infos}, nil
}

func (s *grpcServer) ConnectPeer(ctx context.Context, req *btcrpc.ConnectPeerRequest) (*btcrpc.ConnectPeerResponse, error) {
	addr := normalizeAddress(req.GetPeerAddress(),
		s.server.chainParams.DefaultPort)
	replyChan := make(chan error)
	s.server.query <- connectNodeMsg{
		addr:      addr,
		permanent: req.GetPermanent(),
		reply:     replyChan,
	}
	if err := <-replyChan; err != nil {
		return nil, err
	}

	return &btcrpc.ConnectPeerResponse{}, nil
}

func (s *grpcServer) DisconnectPeer(ctx context.Context, req *btcrpc.DisconnectPeerRequest) (*btcrpc.DisconnectPeerResponse, error) {
	address := req.GetPeerAddress()
	pid := int32(req.GetPeerId())

	var cmp func(*serverPeer) bool
	if len(address) > 0 {
		addr := normalizeAddress(address, s.server.chainParams.DefaultPort)
		cmp = func(sp *serverPeer) bool { return sp.Addr() == addr }
	} else {
		cmp = func(sp *serverPeer) bool { return sp.ID() == pid }
	}

	replyChan := make(chan error)
	if req.GetPermanent() {
		s.server.query <- removeNodeMsg{
			cmp:   cmp,
			reply: replyChan,
		}
	} else {
		s.server.query <- disconnectNodeMsg{
			cmp:   cmp,
			reply: replyChan,
		}
	}
	if err := <-replyChan; err != nil {
		return nil, err
	}

	return &btcrpc.DisconnectPeerResponse{}, nil
}

// MINING

func (s *grpcServer) GetMiningInfo(ctx context.Context, req *btcrpc.GetMiningInfoRequest) (*btcrpc.GetMiningInfoResponse, error) {
	best := s.server.chain.BestSnapshot()
	hashrate, err := calculateNetworkHashRate(s.server.chain,
		s.server.chainParams, best.Height, 0)
	if err != nil {
		return nil, err
	}

	return &btcrpc.GetMiningInfoResponse{
		NetworkHashrate:            hashrate,
		CurrentBlockHeight:         best.Height,
		CurrentBlockSize:           best.BlockSize,
		CurrentBlockWeight:         best.BlockWeight,
		CurrentBlockNbTransactions: uint32(best.NumTxns),
		Difficulty:                 getDifficultyRatio(best.Bits, s.server.chainParams),
	}, nil
}

// REGTEST SERVICE

func (s *grpcServer) SetGenerate(ctx context.Context, req *btcrpc.SetGenerateRequest) (*btcrpc.SetGenerateResponse, error) {
	if !req.GetGenerate() {
		s.server.cpuMiner.Stop()
	} else {
		// Add newly specified payment addresses.
		if addrStr := req.GetPayoutAddress(); len(addrStr) != 0 {
			addr, err := btcutil.DecodeAddress(addrStr, activeNetParams.Params)
			if err != nil {
				return nil, err
			}
			if !addr.IsForNet(activeNetParams.Params) {
				return nil, fmt.Errorf(
					"Payout address '%s' is on the wrong network", addrStr)
			}
			cfg.miningAddrs = append(cfg.miningAddrs, addr)
		}

		if len(cfg.miningAddrs) == 0 {
			return nil, errors.New("No payment addresses specified")
		}

		nbWorkers := req.GetNbWorkers()
		if nbWorkers == 0 {
			nbWorkers = 1
		}

		s.server.cpuMiner.SetNumWorkers(nbWorkers)
		s.server.cpuMiner.Start()
	}

	return &btcrpc.SetGenerateResponse{}, nil
}

func (s *grpcServer) GetGenerateInfo(ctx context.Context, req *btcrpc.GetGenerateInfoRequest) (*btcrpc.GetGenerateInfoResponse, error) {
	best := s.server.chain.BestSnapshot()
	miner := s.server.cpuMiner
	return &btcrpc.GetGenerateInfoResponse{
		Generate:             miner.IsMining(),
		NbWorkers:            miner.NumWorkers(),
		Hashrate:             uint64(miner.HashesPerSecond()),
		Difficulty:           getDifficultyRatio(best.Bits, s.server.chainParams),
		NbPooledTransactions: uint32(s.server.txMemPool.Count()),
	}, nil
}

func (s *grpcServer) GenerateBlocks(ctx context.Context, req *btcrpc.GenerateBlocksRequest) (*btcrpc.GenerateBlocksResponse, error) {
	if addrStr := req.GetPayoutAddress(); len(addrStr) != 0 {
		addr, err := btcutil.DecodeAddress(addrStr, activeNetParams.Params)
		if err != nil {
			return nil, err
		}
		if !addr.IsForNet(activeNetParams.Params) {
			return nil, fmt.Errorf(
				"Payout address '%s' is on the wrong network", addrStr)
		}
		cfg.miningAddrs = append(cfg.miningAddrs, addr)
	}

	if len(cfg.miningAddrs) == 0 {
		return nil, errors.New("No payment addresses specified")
	}

	blockHashes, err := s.server.cpuMiner.GenerateNBlocks(req.GetNbBlocks())
	if err != nil {
		return nil, err
	}

	blockHashesBytes := make([][]byte, len(blockHashes))
	for i, h := range blockHashes {
		blockHashesBytes[i] = h[:]
	}

	return &btcrpc.GenerateBlocksResponse{
		BlockHashes: blockHashesBytes,
	}, nil
}
