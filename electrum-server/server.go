// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Copyright (c) 2022-2023 The utreexo developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package electrum

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/blockchain/indexers"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

const (
	protocolMajor = 1
	protocolMinor = 4
	protocolPatch = 3

	minProtocolMajor = 1
	minProtocolMinor = 4
	minProtocolPatch = 1

	// idleTimeout is the duration of inactivity before we time out a peer.
	idleTimeout = 10 * time.Minute

	mempoolHistogramBinSize = 30_000
)

var (
	delim                      = byte('\n')
	serverName                 = "BTCD Electrum Server 0.1.0"
	electrumProtocolVersion    = fmt.Sprintf("%d.%d.%d", protocolMajor, protocolMinor, protocolPatch)
	minElectrumProtocolVersion = fmt.Sprintf("%d.%d", minProtocolMajor, minProtocolMinor)
)

type commandHandler func(*ElectrumServer, *btcjson.Request, net.Conn, <-chan struct{}) (interface{}, error)

// rpcHandlers maps RPC command strings to appropriate handler functions.
// This is set by init because help references rpcHandlers and thus causes
// a dependency loop.
var rpcHandlers map[string]commandHandler
var rpcHandlersBeforeInit = map[string]commandHandler{
	"blockchain.block.header":            handleBlockchainBlockHeader,
	"blockchain.block.headers":           handleBlockchainBlockHeaders,
	"blockchain.estimatefee":             handleEstimateFee,
	"blockchain.headers.subscribe":       handleHeadersSubscribe,
	"blockchain.relayfee":                handleRelayFee,
	"blockchain.scripthash.get_balance":  handleScriptHashGetBalance,
	"blockchain.scripthash.get_history":  handleScriptHashGetHistory,
	"blockchain.scripthash.get_mempool":  handleScriptHashGetMempool,
	"blockchain.scripthash.listunspent":  handleListUnspent,
	"blockchain.scripthash.subscribe":    handleScriptHashSubscribe,
	"blockchain.scripthash.unsubscribe":  handleScriptHashUnsubscribe,
	"blockchain.transaction.broadcast":   handleTransactionBroadcast,
	"blockchain.transaction.get":         handleGetTransaction,
	"blockchain.transaction.get_merkle":  handleGetMerkle,
	"blockchain.transaction.id_from_pos": handleIDFromPos,
	"mempool.get_fee_histogram":          handleMempoolGetFeeHistogram,
	"server.add_peer":                    handleServerAddPeer,
	"server.banner":                      handleBanner,
	"server.donation_address":            handleDonationAddress,
	"server.features":                    handleServerFeatures,
	"server.peers.subscribe":             handleServerPeersSubscribe,
	"server.ping":                        handlePing,
	"server.version":                     handleVersion,
}

func handleBlockchainBlockHeader(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	var height int32
	err := json.Unmarshal(cmd.Params[0], &height)
	if err != nil {
		// Try to unmarshal as a string.  If it still errors, then return the error.
		var heightStr string
		err := json.Unmarshal(cmd.Params[0], &heightStr)
		if err != nil {
			return nil, err
		}

		heightInt, err := strconv.Atoi(heightStr)
		if err != nil {
			return nil, err
		}

		height = int32(heightInt)
	}
	if height < 0 {
		return nil, fmt.Errorf("Got height %d. Expected height to be non-negative.", height)
	}

	blockhash, err := s.cfg.BlockChain.BlockHashByHeight(height)
	if err != nil {
		return nil, err
	}

	header, err := s.cfg.BlockChain.HeaderByHash(blockhash)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	err = header.Serialize(&buf)
	if err != nil {
		return nil, err
	}

	res := hex.EncodeToString(buf.Bytes())
	s.writeResponse(conn, cmd, res)

	return nil, nil
}

type HeadersResponse struct {
	Count int    `json:"count"`
	Hex   string `json:"hex"`
	Max   int    `json:"max"`
}

func handleBlockchainBlockHeaders(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	var startHeight, count int32
	err := json.Unmarshal(cmd.Params[0], &startHeight)
	if err != nil {
		// Try to unmarshal as a string.  If it still errors, then return the error.
		var startHeightStr string
		err := json.Unmarshal(cmd.Params[0], &startHeightStr)
		if err != nil {
			return nil, err
		}

		startHeightInt, err := strconv.Atoi(startHeightStr)
		if err != nil {
			return nil, err
		}

		startHeight = int32(startHeightInt)
	}
	err = json.Unmarshal(cmd.Params[1], &count)
	if err != nil {
		// Try to unmarshal as a string.  If it still errors, then return the error.
		var countStr string
		err := json.Unmarshal(cmd.Params[1], &countStr)
		if err != nil {
			return nil, err
		}

		countInt, err := strconv.Atoi(countStr)
		if err != nil {
			return nil, err
		}

		count = int32(countInt)
	}
	if startHeight < 0 || count < 0 {
		return nil, fmt.Errorf("Expected both startHeight %d and count %d to be non-negative",
			startHeight, count)
	}

	retCount := 0
	headers := make([]wire.BlockHeader, 0, count)
	for i := startHeight; i < startHeight+count; i++ {
		blockhash, err := s.cfg.BlockChain.BlockHashByHeight(i)
		if err != nil {
			return nil, err
		}
		header, err := s.cfg.BlockChain.HeaderByHash(blockhash)
		if err != nil {
			return nil, err
		}

		headers = append(headers, header)

		retCount++

		// We only send up to 2016 headers at a time per the electrum procotol.
		if retCount >= 2016 {
			break
		}
	}

	var buf bytes.Buffer
	for _, header := range headers {
		err := header.Serialize(&buf)
		if err != nil {
			return nil, err
		}
	}

	res := HeadersResponse{
		Count: retCount,
		Hex:   hex.EncodeToString(buf.Bytes()),
		Max:   2016,
	}
	s.writeResponse(conn, cmd, res)

	return nil, nil
}

func handleEstimateFee(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	var numBlocks int32
	err := json.Unmarshal(cmd.Params[0], &numBlocks)
	if err != nil {
		// Try to unmarshal as a string.  If it still errors, then return the error.
		var numBlocksStr string
		err := json.Unmarshal(cmd.Params[0], &numBlocksStr)
		if err != nil {
			return nil, err
		}

		numBlocksInt, err := strconv.Atoi(numBlocksStr)
		if err != nil {
			return nil, err
		}

		numBlocks = int32(numBlocksInt)
		return nil, err
	}

	if numBlocks < 0 {
		return nil, fmt.Errorf("Expected the number of blocks of %d to be non-negative", numBlocks)
	}

	fee, err := s.cfg.FeeEstimator.EstimateFee(uint32(numBlocks))
	if err != nil {
		s.writeResponse(conn, cmd, -1)
	} else {
		s.writeResponse(conn, cmd, fee)
	}

	return nil, nil
}

type HeadersSubscribeResponse struct {
	Hex    string `json:"hex"`
	Height int    `json:"height"`
}

func handleHeadersSubscribe(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	s.headersSubscribers[conn] = struct{}{}

	bestHash := s.cfg.BlockChain.BestSnapshot().Hash
	header, err := s.cfg.BlockChain.HeaderByHash(&bestHash)
	if err != nil {
		return nil, err
	}

	height, err := s.cfg.BlockChain.BlockHeightByHash(&bestHash)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	err = header.Serialize(&buf)
	if err != nil {
		return nil, err
	}

	serializedHeaderStr := hex.EncodeToString(buf.Bytes())
	res := HeadersSubscribeResponse{
		Height: int(height),
		Hex:    serializedHeaderStr,
	}
	s.writeResponse(conn, cmd, res)

	return nil, nil
}

func handleRelayFee(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	s.writeResponse(conn, cmd, s.cfg.MinRelayFee.ToBTC())
	return nil, nil
}

type Balance struct {
	Confirmed   btcutil.Amount `json:"confirmed"`
	Unconfirmed btcutil.Amount `json:"unconfirmed"`
}

func handleScriptHashGetBalance(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	var hashStr string
	err := json.Unmarshal(cmd.Params[0], &hashStr)
	if err != nil {
		return nil, err
	}

	decodedHash := new(chainhash.Hash)
	err = chainhash.Decode(decodedHash, hashStr)
	if err != nil {
		return nil, err
	}

	var addr btcutil.Address
	err = s.db.View(func(dbTx database.Tx) error {
		addrStr := s.cfg.ScriptHashIndex.AddrFromScriptHash(dbTx, *decodedHash)
		addr, err = btcutil.DecodeAddress(addrStr, s.cfg.Params)
		return err
	})
	if err != nil {
		return nil, err
	}

	unspents, err := getUnspent(s, addr)
	if err != nil {
		return nil, err
	}

	confirmedBalance := uint64(0)
	unconfirmedBalance := uint64(0)

	for _, unspent := range unspents {
		if unspent.Height != 0 {
			confirmedBalance += uint64(unspent.Value)
		} else {
			unconfirmedBalance += uint64(unspent.Value)
		}
	}

	res := Balance{
		Confirmed:   btcutil.Amount(confirmedBalance),
		Unconfirmed: btcutil.Amount(unconfirmedBalance),
	}
	s.writeResponse(conn, cmd, res)

	return nil, nil
}

type ScriptHashHistory struct {
	Height int    `json:"height"`
	TxHash string `json:"tx_hash"`
	Fee    int    `json:"fee,omitempty"`
}

func handleScriptHashGetHistory(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	var hashStr string
	err := json.Unmarshal(cmd.Params[0], &hashStr)
	if err != nil {
		return nil, err
	}

	decodedHash := new(chainhash.Hash)
	err = chainhash.Decode(decodedHash, hashStr)
	if err != nil {
		return nil, err
	}

	log.Infof("scripthash get history got %v\n%v",
		hex.EncodeToString(decodedHash[:]), decodedHash)

	var addrStr string
	s.db.View(func(dbTx database.Tx) error {
		addrStr = s.cfg.ScriptHashIndex.AddrFromScriptHash(dbTx, *decodedHash)
		return nil
	})

	if addrStr == "" {
		log.Infof("addrStr %v", addrStr)
		s.writeResponse(conn, cmd, nil)
		return nil, nil
	}

	addr, err := btcutil.DecodeAddress(addrStr, s.cfg.Params)
	if err != nil {
		return nil, err
	}

	res, err := scriptHashHistory(s, addr)
	if err != nil {
		return nil, err
	}

	mempoolHist, err := scriptHashMempool(s, addr)
	if err != nil {
		return nil, err
	}

	res = append(res, mempoolHist...)
	s.writeResponse(conn, cmd, res)

	return res, nil
}

func scriptHashHistory(s *ElectrumServer, addr btcutil.Address) ([]ScriptHashHistory, error) {
	log.Infof("0")
	var ret []ScriptHashHistory
	err := s.db.View(func(dbTx database.Tx) error {
		log.Infof("1")
		numToSkip := uint32(0)
		regions, _, err := s.cfg.AddrIndex.TxRegionsForAddress(dbTx, addr, numToSkip, math.MaxUint32, false)
		if err != nil {
			return err
		}
		log.Infof("2")

		log.Infof("3")
		ret = make([]ScriptHashHistory, 0, len(regions))
		for _, region := range regions {
			log.Infof("4")
			txBytes, err := dbTx.FetchBlockRegion(&region)
			if err != nil {
				return err
			}
			log.Infof("5")

			var msgTx wire.MsgTx
			err = msgTx.Deserialize(bytes.NewReader(txBytes))
			if err != nil {
				return err
			}
			log.Infof("6")

			blockHeight, err := s.cfg.BlockChain.BlockHeightByHash(region.Hash)
			if err != nil {
				return err
			}
			log.Infof("7")

			ret = append(ret, ScriptHashHistory{
				Height: int(blockHeight),
				TxHash: msgTx.TxHash().String(),
			})
		}
		log.Infof("8")

		return err
	})
	if err != nil {
		return nil, err
	}
	log.Infof("9")

	return ret, nil
}

//func scriptHashMempoolFromView(s *ElectrumServer, addr btcutil.Address,
//	view *blockchain.UtxoViewpoint) ([]ScriptHashHistory, error) {
//
//	txs := s.cfg.AddrIndex.UnconfirmedTxnsForAddress(addr)
//
//	ret := make([]ScriptHashHistory, 0, len(txs))
//	for _, tx := range txs {
//		height := 0
//		inputSats := int64(0)
//		for _, txIn := range tx.MsgTx().TxIn {
//			entry := view.LookupEntry(txIn.PreviousOutPoint)
//			if entry == nil {
//				return nil, fmt.Errorf("outpoint %v is missing its entry",
//					txIn.PreviousOutPoint)
//			}
//			if entry.BlockHeight() == mining.UnminedHeight {
//				height = -1
//			}
//			inputSats += entry.Amount()
//		}
//
//		outputSats := int64(0)
//		for _, txOut := range tx.MsgTx().TxOut {
//			outputSats += txOut.Value
//		}
//
//		ret = append(ret, ScriptHashHistory{
//			Height: height,
//			TxHash: tx.Hash().String(),
//			Fee:    int(inputSats) - int(outputSats),
//		})
//	}
//
//	return ret, nil
//}

func scriptHashMempool(s *ElectrumServer, addr btcutil.Address) ([]ScriptHashHistory, error) {
	log.Infof("scriptHashMempool 0")
	txs := s.cfg.AddrIndex.UnconfirmedTxnsForAddress(addr)
	log.Infof("scriptHashMempool 1")

	ret := make([]ScriptHashHistory, 0, len(txs))
	for _, tx := range txs {
		log.Infof("scriptHashMempool 2")
		view, err := s.cfg.BlockChain.FetchUtxoView(tx)
		if err != nil {
			return nil, err
		}

		log.Infof("scriptHashMempool 3")
		height := 0
		inputSats := int64(0)
		for _, txIn := range tx.MsgTx().TxIn {
			log.Infof("scriptHashMempool 4")
			entry := view.LookupEntry(txIn.PreviousOutPoint)
			if entry == nil {
				log.Infof("scriptHashMempool 5")
				height = -1

				prevTx, err := s.cfg.Mempool.FetchTransaction(&txIn.PreviousOutPoint.Hash)
				if err != nil {
					return nil, err
				}
				log.Infof("scriptHashMempool 6")

				out := prevTx.MsgTx().TxOut[txIn.PreviousOutPoint.Index]
				inputSats += out.Value
			} else {
				log.Infof("scriptHashMempool 7")
				inputSats += entry.Amount()
			}
		}
		log.Infof("scriptHashMempool 8")

		outputSats := int64(0)
		for _, txOut := range tx.MsgTx().TxOut {
			outputSats += txOut.Value
		}

		ret = append(ret, ScriptHashHistory{
			Height: height,
			TxHash: tx.Hash().String(),
			Fee:    int(inputSats) - int(outputSats),
		})
	}

	log.Infof("scriptHashMempool 9")

	return ret, nil
}

func handleScriptHashGetMempool(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	var hashStr string
	err := json.Unmarshal(cmd.Params[0], &hashStr)
	if err != nil {
		return nil, err
	}

	decodedHash := new(chainhash.Hash)
	err = chainhash.Decode(decodedHash, hashStr)
	if err != nil {
		return nil, err
	}

	var addr btcutil.Address
	err = s.db.View(func(dbTx database.Tx) error {
		addrStr := s.cfg.ScriptHashIndex.AddrFromScriptHash(dbTx, *decodedHash)
		addr, err = btcutil.DecodeAddress(addrStr, s.cfg.Params)
		return err
	})
	if err != nil {
		return nil, err
	}

	res, err := scriptHashMempool(s, addr)
	if err != nil {
		return nil, err
	}
	s.writeResponse(conn, cmd, res)

	return nil, nil
}

func getUnspent(s *ElectrumServer, addr btcutil.Address) ([]Unspent, error) {
	var regions []database.BlockRegion
	err := s.db.View(func(dbTx database.Tx) error {
		numToSkip := uint32(0)
		var err error
		regions, _, err = s.cfg.AddrIndex.TxRegionsForAddress(dbTx, addr, numToSkip, math.MaxUint32, false)
		return err
	})
	if err != nil {
		return nil, err
	}

	spentMap := make(map[wire.OutPoint]struct{}, len(regions))
	unspents := make([]Unspent, 0, len(regions)*2)

	txs := s.cfg.AddrIndex.UnconfirmedTxnsForAddress(addr)
	for _, tx := range txs {
		for _, txIn := range tx.MsgTx().TxIn {
			spentMap[txIn.PreviousOutPoint] = struct{}{}
		}
	}

	for _, tx := range txs {
		for i, txOut := range tx.MsgTx().TxOut {
			outpoint := wire.OutPoint{
				Hash:  *tx.Hash(),
				Index: uint32(i),
			}

			_, found := spentMap[outpoint]
			if !found {
				unspents = append(unspents, Unspent{
					TxPos:  i,
					Value:  int(txOut.Value),
					Height: 0,
					TxHash: tx.Hash().String(),
				})
			}
		}
	}

	err = s.db.View(func(dbTx database.Tx) error {
		numToSkip := uint32(0)
		regions, _, err := s.cfg.AddrIndex.TxRegionsForAddress(dbTx, addr, numToSkip, math.MaxUint32, false)
		if err != nil {
			return err
		}

		for i := len(regions) - 1; i >= 0; i-- {
			region := regions[i]

			txBytes, err := dbTx.FetchBlockRegion(&region)
			if err != nil {
				return err
			}

			var msgTx wire.MsgTx
			err = msgTx.Deserialize(bytes.NewReader(txBytes))
			if err != nil {
				return err
			}
			tx := btcutil.NewTx(&msgTx)

			blockHeight, err := s.cfg.BlockChain.BlockHeightByHash(region.Hash)
			if err != nil {
				return err
			}

			for _, txIn := range msgTx.TxIn {
				spentMap[txIn.PreviousOutPoint] = struct{}{}
			}

			for i, txOut := range msgTx.TxOut {
				outpoint := wire.OutPoint{
					Hash:  *tx.Hash(),
					Index: uint32(i),
				}

				_, found := spentMap[outpoint]
				if !found {
					unspents = append(unspents, Unspent{
						TxPos:  i,
						Value:  int(txOut.Value),
						Height: int(blockHeight),
						TxHash: tx.Hash().String(),
					})
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.Reverse(unspents)

	return unspents, nil
}

type Unspent struct {
	TxPos  int    `json:"tx_pos"`
	Value  int    `json:"value"`
	Height int    `json:"height"`
	TxHash string `json:"tx_hash"`
}

func handleListUnspent(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	var hashStr string
	err := json.Unmarshal(cmd.Params[0], &hashStr)
	if err != nil {
		return nil, err
	}

	decodedHash := new(chainhash.Hash)
	err = chainhash.Decode(decodedHash, hashStr)
	if err != nil {
		return nil, err
	}

	var addr btcutil.Address
	err = s.db.View(func(dbTx database.Tx) error {
		addrStr := s.cfg.ScriptHashIndex.AddrFromScriptHash(dbTx, *decodedHash)
		addr, err = btcutil.DecodeAddress(addrStr, s.cfg.Params)
		return err
	})
	if err != nil {
		return nil, err
	}

	res, err := getUnspent(s, addr)
	if err != nil {
		return nil, err
	}
	s.writeResponse(conn, cmd, res)

	return nil, nil
}

func getStatusFromHistory(hist, mempoolHist []ScriptHashHistory) [32]byte {
	str := ""
	for _, txData := range hist {
		str += fmt.Sprintf("%s:%d:", txData.TxHash, txData.Height)
	}
	for _, txData := range mempoolHist {
		str += fmt.Sprintf("%s:%d:", txData.TxHash, txData.Height)
	}

	hash := sha256.Sum256([]byte(str))
	return hash
}

//func getScriptHashStatus(s *ElectrumServer, addr btcutil.Address) (*[32]byte, error) {
//	ret, err := scriptHashHistory(s, addr)
//	if err != nil {
//		return nil, err
//	}
//
//	mempoolHist, err := scriptHashMempool(s, addr)
//	if err != nil {
//		return nil, err
//	}
//
//	hash := getStatusFromHistory(ret, mempoolHist)
//	return &hash, nil
//
//	//str := ""
//	//for _, txData := range ret {
//	//	str += fmt.Sprintf("%s:%d:", txData.TxHash, txData.Height)
//	//}
//	//for _, txData := range mempoolHist {
//	//	str += fmt.Sprintf("%s:%d:", txData.TxHash, txData.Height)
//	//}
//
//	//hash := sha256.Sum256([]byte(str))
//	//return &hash, nil
//}

func handleScriptHashSubscribe(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	var hashStr string
	err := json.Unmarshal(cmd.Params[0], &hashStr)
	if err != nil {
		return nil, err
	}

	scriptHashMap, found := s.scriptHashSubscribers[conn]
	if !found {
		scriptHashMap = make(map[string]struct{})
		scriptHashMap[hashStr] = struct{}{}
		s.scriptHashSubscribers[conn] = scriptHashMap

	} else {
		scriptHashMap[hashStr] = struct{}{}
	}

	log.Infof("conn %v is subscribed to %v", conn.RemoteAddr(), hashStr)

	decodedHash := new(chainhash.Hash)
	err = chainhash.Decode(decodedHash, hashStr)
	if err != nil {
		return nil, err
	}

	var addrStr string
	err = s.db.View(func(dbTx database.Tx) error {
		addrStr = s.cfg.ScriptHashIndex.AddrFromScriptHash(dbTx, *decodedHash)
		return nil
	})

	if addrStr == "" {
		log.Infof("conn %v is subscribed to %v", conn.RemoteAddr(), hashStr)
		s.writeResponse(conn, cmd, nil)
		return nil, nil
	}

	addr, err := btcutil.DecodeAddress(addrStr, s.cfg.Params)
	if err != nil {
		return nil, err
	}

	ret, err := scriptHashHistory(s, addr)
	if err != nil {
		return nil, err
	}

	mempoolHist, err := scriptHashMempool(s, addr)
	if err != nil {
		return nil, err
	}

	hash := getStatusFromHistory(ret, mempoolHist)

	log.Infof("conn %v is subscribed to %v (%v)", conn.RemoteAddr(), hashStr, addrStr)

	//log.Debugf("total subs for conn %s: %d. new sub for: raw hash %s, decoded hash %s, returned hash: %s\n",
	//	conn.RemoteAddr().String(),
	//	len(scriptHashMap),
	//	hashStr,
	//	decodedHash.String(),
	//	hex.EncodeToString(hash[:]))

	//log.Debugf("total subs for conn %s: %d. new sub for: raw hash %s, decoded hash %s, returned hash: %s\n",
	//	conn.RemoteAddr().String(),
	//	len(s.scriptHashSubscribers[conn]),
	//	hashStr,
	//	decodedHash.String(),
	//	hex.EncodeToString(hash[:]))

	res := hex.EncodeToString(hash[:])
	s.writeResponse(conn, cmd, res)

	return nil, nil
}

func handleScriptHashUnsubscribe(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	var hashStr string
	err := json.Unmarshal(cmd.Params[0], &hashStr)
	if err != nil {
		return nil, err
	}

	scriptHashMap, found := s.scriptHashSubscribers[conn]
	if found {
		log.Debugf("unsubscribed %v from scripthash %v",
			conn.RemoteAddr(), hashStr)
		delete(scriptHashMap, hashStr)

		s.writeResponse(conn, cmd, true)
	} else {
		s.writeResponse(conn, cmd, false)
	}

	return nil, nil
}

func handleTransactionBroadcast(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	var hexStr string
	err := json.Unmarshal(cmd.Params[0], &hexStr)
	if err != nil {
		return nil, err
	}

	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}

	serializedTx, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}

	var msgTx wire.MsgTx
	err = msgTx.Deserialize(bytes.NewReader(serializedTx))
	if err != nil {
		return nil, &btcjson.RPCError{
			Code:    btcjson.ErrRPCDeserialization,
			Message: "TX decode failed: " + err.Error(),
		}
	}

	tx := btcutil.NewTx(&msgTx)
	_, err = s.cfg.Mempool.CheckMempoolAcceptance(tx)
	if err != nil {
		// When the error is a rule error, it means the transaction was
		// simply rejected as opposed to something actually going wrong,
		// so log it as such. Otherwise, something really did go wrong,
		// so log it as an actual error and return.
		ruleErr, ok := err.(mempool.RuleError)
		if !ok {
			log.Errorf("Failed to process transaction %v: %v",
				tx.Hash(), err)

			return nil, &btcjson.RPCError{
				Code:    btcjson.ErrRPCTxError,
				Message: "TX rejected: " + err.Error(),
			}
		}

		log.Debugf("Rejected transaction %v: %v", tx.Hash(), err)

		// We'll then map the rule error to the appropriate RPC error,
		// matching bitcoind's behavior.
		code := btcjson.ErrRPCTxError
		if txRuleErr, ok := ruleErr.Err.(mempool.TxRuleError); ok {
			errDesc := txRuleErr.Description
			switch {
			case strings.Contains(
				strings.ToLower(errDesc), "orphan transaction",
			):
				code = btcjson.ErrRPCTxError

			case strings.Contains(
				strings.ToLower(errDesc), "transaction already exists",
			):
				code = btcjson.ErrRPCTxAlreadyInChain

			default:
				code = btcjson.ErrRPCTxRejected
			}
		}

		return nil, &btcjson.RPCError{
			Code:    code,
			Message: "TX rejected: " + err.Error(),
		}
	}

	// The transaction is gonna get accepted so alert the client.
	s.writeResponse(conn, cmd, tx.Hash().String())

	_, acceptedTx, err := s.cfg.Mempool.MaybeAcceptTransaction(tx, true, false)
	if err != nil {
		log.Warnf("Failed to accept tx %v when it's already passed CheckMempoolAcceptance",
			tx.Hash())
		return nil, err
	}

	s.cfg.AnnounceNewTransactions([]*mempool.TxDesc{acceptedTx})

	iv := wire.NewInvVect(wire.InvTypeTx, tx.Hash())
	s.cfg.AddRebroadcastInventory(iv, acceptedTx)

	return nil, nil

	//log.Infof("putting tx %v into mempool", tx.Hash().String())
	//acceptedTxs, err := s.cfg.Mempool.ProcessTransaction(tx, false, false, 0)
	//if err != nil {
	//	// When the error is a rule error, it means the transaction was
	//	// simply rejected as opposed to something actually going wrong,
	//	// so log it as such. Otherwise, something really did go wrong,
	//	// so log it as an actual error and return.
	//	ruleErr, ok := err.(mempool.RuleError)
	//	if !ok {
	//		log.Errorf("Failed to process transaction %v: %v",
	//			tx.Hash(), err)

	//		return nil, &btcjson.RPCError{
	//			Code:    btcjson.ErrRPCTxError,
	//			Message: "TX rejected: " + err.Error(),
	//		}
	//	}

	//	log.Debugf("Rejected transaction %v: %v", tx.Hash(), err)

	//	// We'll then map the rule error to the appropriate RPC error,
	//	// matching bitcoind's behavior.
	//	code := btcjson.ErrRPCTxError
	//	if txRuleErr, ok := ruleErr.Err.(mempool.TxRuleError); ok {
	//		errDesc := txRuleErr.Description
	//		switch {
	//		case strings.Contains(
	//			strings.ToLower(errDesc), "orphan transaction",
	//		):
	//			code = btcjson.ErrRPCTxError

	//		case strings.Contains(
	//			strings.ToLower(errDesc), "transaction already exists",
	//		):
	//			code = btcjson.ErrRPCTxAlreadyInChain

	//		default:
	//			code = btcjson.ErrRPCTxRejected
	//		}
	//	}

	//	return nil, &btcjson.RPCError{
	//		Code:    code,
	//		Message: "TX rejected: " + err.Error(),
	//	}
	//}

	//// When the transaction was accepted it should be the first item in the
	//// returned array of accepted transactions.  The only way this will not
	//// be true is if the API for ProcessTransaction changes and this code is
	//// not properly updated, but ensure the condition holds as a safeguard.
	////
	//// Also, since an error is being returned to the caller, ensure the
	//// transaction is removed from the memory pool.
	//if len(acceptedTxs) == 0 || !acceptedTxs[0].Tx.Hash().IsEqual(tx.Hash()) {
	//	s.cfg.Mempool.RemoveTransaction(tx, true)

	//	errStr := fmt.Errorf("transaction %v is not in accepted list",
	//		tx.Hash())
	//	return nil, errStr
	//}

	// Generate and relay inventory vectors for all newly accepted
	// transactions into the memory pool due to the original being
	// accepted.

	//// Keep track of all the sendrawtransaction request txns so that they
	//// can be rebroadcast if they don't make their way into a block.
	//txD := acceptedTxs[0]
	//iv := wire.NewInvVect(wire.InvTypeTx, txD.Tx.Hash())
	//s.cfg.AddRebroadcastInventory(iv, txD)

	//log.Infof("tx %v accepted. Sending the hash as response", tx.Hash().String())

	//return tx.Hash().String(), nil
}

func handleGetTransaction(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	var hashStr string
	err := json.Unmarshal(cmd.Params[0], &hashStr)
	if err != nil {
		return nil, err
	}

	decodedHash := new(chainhash.Hash)
	err = chainhash.Decode(decodedHash, hashStr)
	if err != nil {
		return nil, err
	}

	blockRegion, err := s.cfg.TxIndex.TxBlockRegion(decodedHash)
	if err != nil {
		return nil, err
	}

	if blockRegion != nil {
		var txBytes []byte
		err = s.db.View(func(dbTx database.Tx) error {
			// Load the raw transaction bytes from the database.
			txBytes, err = dbTx.FetchBlockRegion(blockRegion)
			return err
		})
		if err != nil {
			return nil, err
		}

		res := hex.EncodeToString(txBytes)
		s.writeResponse(conn, cmd, res)

		return nil, nil
	}

	tx, err := s.cfg.Mempool.FetchTransaction(decodedHash)
	if err != nil {
		return nil, err
	}

	if tx == nil {
		s.writeResponse(conn, cmd, nil)
		return nil, nil
	}

	buf := bytes.NewBuffer(make([]byte, 0, tx.MsgTx().SerializeSize()))
	err = tx.MsgTx().BtcEncode(buf, 0, wire.LatestEncoding)
	if err != nil {
		return nil, err
	}

	res := hex.EncodeToString(buf.Bytes())
	s.writeResponse(conn, cmd, res)

	return nil, nil
}

type GetMerkleRes struct {
	Merkle      []string `json:"merkle"`
	BlockHeight int      `json:"block_height"`
	Pos         int      `json:"pos"`
}

func handleGetMerkle(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	var hashStr string
	err := json.Unmarshal(cmd.Params[0], &hashStr)
	if err != nil {
		return nil, err
	}
	var height int
	err = json.Unmarshal(cmd.Params[1], &height)
	if err != nil {
		// Try to unmarshal as a string.  If it still errors, then return the error.
		var heightStr string
		err := json.Unmarshal(cmd.Params[1], &heightStr)
		if err != nil {
			return nil, err
		}

		height, err = strconv.Atoi(heightStr)
		if err != nil {
			return nil, err
		}
	}
	if height < 0 {
		return nil, fmt.Errorf("Got height %d. Expected height to be non-negative.", height)
	}

	decodedHash := new(chainhash.Hash)
	err = chainhash.Decode(decodedHash, hashStr)
	if err != nil {
		return nil, err
	}

	block, err := s.cfg.BlockChain.BlockByHeight(int32(height))
	if err != nil {
		return nil, err
	}

	txs := block.Transactions()
	index := slices.IndexFunc(txs, func(tx *btcutil.Tx) bool {
		return tx.Hash().IsEqual(decodedHash)
	})

	merkles := blockchain.BuildMerkleTreeStore(txs, false)
	hashes := blockchain.ExtractMerkleBranch(merkles, *decodedHash)

	merklesStr := make([]string, 0, len(merkles))
	for _, merkle := range hashes {
		merklesStr = append(merklesStr, merkle.String())
	}
	res := GetMerkleRes{Merkle: merklesStr, BlockHeight: height, Pos: index}
	s.writeResponse(conn, cmd, res)

	return nil, nil
}

type IDFromPosRes struct {
	TxHash string   `json:"tx_hash"`
	Merkle []string `json:"merkle"`
}

func handleIDFromPos(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	var height, txPos int
	var getMerkles bool

	err := json.Unmarshal(cmd.Params[0], &height)
	if err != nil {
		return nil, err
	}
	if height < 0 {
		return nil, fmt.Errorf("Expected received height of %d to be non-negative", height)
	}
	err = json.Unmarshal(cmd.Params[1], &txPos)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(cmd.Params[2], &getMerkles)
	if err != nil {
		return nil, err
	}

	block, err := s.cfg.BlockChain.BlockByHeight(int32(height))
	if err != nil {
		return nil, err
	}

	txs := block.Transactions()

	if txPos >= len(txs) {
		return nil, fmt.Errorf("at height %d, have %v txs but the requested txPos is %v",
			height, len(txs), txPos)
	}
	txHash := txs[txPos].Hash()

	merkles := blockchain.BuildMerkleTreeStore(txs, false)
	hashes := blockchain.ExtractMerkleBranch(merkles, *txHash)
	merklesStr := make([]string, len(hashes))
	for _, merkle := range hashes {
		merklesStr = append(merklesStr, merkle.String())
	}

	res := IDFromPosRes{TxHash: txHash.String(), Merkle: merklesStr}
	s.writeResponse(conn, cmd, res)

	return nil, nil
}

func handleMempoolGetFeeHistogram(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	txs := s.cfg.Mempool.MiningDescs()

	type feeAndSize struct {
		fee  int64
		size int64
	}

	feeAndSizes := make([]feeAndSize, len(txs))
	for i, tx := range txs {
		feeAndSizes[i] = feeAndSize{
			fee:  tx.FeePerKB / 1000,
			size: mempool.GetTxVirtualSize(tx.Tx),
		}
	}

	sort.Slice(feeAndSizes, func(a, b int) bool {
		return feeAndSizes[a].fee < feeAndSizes[b].fee
	})

	binSize := int64(mempoolHistogramBinSize)
	histogram := make([][]int64, 0, len(txs))
	var prevFeeRate, cumSize int64
	for _, fs := range feeAndSizes {
		//    If there is a big lump of txns at this specific size,
		//    consider adding the previous item now (if not added already)
		if fs.size > 2*binSize &&
			prevFeeRate != 0 &&
			cumSize > 0 {

			histogram = append(histogram, []int64{prevFeeRate, cumSize})
			cumSize = 0
			binSize = (binSize * 11) / 10
		}
		// Now consider adding this item
		cumSize += fs.size
		if cumSize > binSize {
			histogram = append(histogram, []int64{fs.fee, cumSize})
			cumSize = 0
			binSize = (binSize * 11) / 10
		}
		prevFeeRate = fs.fee
	}

	s.writeResponse(conn, cmd, histogram)
	return nil, nil
}

func handleServerAddPeer(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	// We always return false as we don't supoprt adding peers.
	s.writeResponse(conn, cmd, false)
	return nil, nil
}

func handleBanner(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	// (kcalvinalvin) Best I can do. Looks awesome imo.
	str := `
		 /$$$$$$$  /$$$$$$$$ / $$$$$$$ /$$$$$$$
  		| $$__  $$|__  $$__/| $$_____/| $$__  $$
  		| $$  \ $$   | $$   | $$      | $$  \ $$
  		| $$$$$$$    | $$   | $$      | $$  | $$
  		| $$__  $$   | $$   | $$      | $$  | $$
  		| $$  \ $$   | $$   | $$      | $$  | $$
  		| $$$$$$$/   | $$   |  $$$$$$$| $$$$$$$/
  		|_______/    |__/    \_______/|_______/
  		`
	s.writeResponse(conn, cmd, str)

	return nil, nil
}

func handleDonationAddress(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	// Don't mind if I do!
	str := "bc1qhjeu95vx0c7zycfumv6jarl32dvcsyfd0egeue"
	s.writeResponse(conn, cmd, str)

	return nil, nil
}

type HostPort struct {
	TCPPort int `json:"tcp_port"`
	SslPort int `json:"ssl_port"`
}

type ServerFeaturesResponse struct {
	GenesisHash   string              `json:"genesis_hash"`
	Hosts         map[string]HostPort `json:"hosts"`
	ProtocolMax   string              `json:"protocol_max"`
	ProtocolMin   string              `json:"protocol_min"`
	Pruning       bool                `json:"pruning,omitempty"`
	ServerVersion string              `json:"server_version"`
	HashFunction  string              `json:"hash_function"`
}

func handleServerFeatures(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	hostsMap := make(map[string]HostPort)

	for i, listener := range s.cfg.Listeners {
		hostAddr, port, err := net.SplitHostPort(listener.Addr().String())
		if err != nil {
			log.Errorf("handleServerFeatures error. Err: %v", err)
			continue
		}
		portNum, _ := strconv.Atoi(port)

		if s.cfg.ListenerTLS[i] {
			host, found := hostsMap[hostAddr]
			if found {
				host.SslPort = portNum
			} else {
				host = HostPort{
					SslPort: portNum,
				}
			}

			hostsMap[hostAddr] = host
		} else {
			host, found := hostsMap[hostAddr]
			if found {
				host.TCPPort = portNum
			} else {
				host = HostPort{
					TCPPort: portNum,
				}
			}

			hostsMap[hostAddr] = host
		}
	}

	res := ServerFeaturesResponse{
		GenesisHash:   s.cfg.Params.GenesisHash.String(),
		Hosts:         hostsMap,
		ProtocolMin:   minElectrumProtocolVersion,
		ProtocolMax:   electrumProtocolVersion,
		Pruning:       false,
		ServerVersion: "btcd v0.1.0",
		HashFunction:  "sha256",
	}
	s.writeResponse(conn, cmd, res)

	return nil, nil
}

func handleServerPeersSubscribe(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	// Purposely left as an emtpy []string because we don't support having peers.
	res := []string{}
	s.writeResponse(conn, cmd, res)

	return nil, nil
}

func handlePing(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	s.writeResponse(conn, cmd, nil)
	return nil, nil
}

func handleVersion(s *ElectrumServer, cmd *btcjson.Request, conn net.Conn, closeChan <-chan struct{}) (interface{}, error) {
	res := []string{serverName, minElectrumProtocolVersion}
	s.writeResponse(conn, cmd, res)

	return nil, nil
}

type connAndBytes struct {
	conn  net.Conn
	bytes []byte
}

type ElectrumServer struct {
	started   int32
	shutdown  int32
	db        database.DB
	cfg       *Config
	quit      chan int
	wg        sync.WaitGroup
	writeChan chan []connAndBytes

	headersSubscribers    map[net.Conn]struct{}
	scriptHashSubscribers map[net.Conn]map[string]struct{}
}

func (s *ElectrumServer) writeErrorResponse(conn net.Conn, rpcError btcjson.RPCError, pid *interface{}) {
	resp := btcjson.Response{
		Jsonrpc: btcjson.RpcVersion2,
		Error:   &rpcError,
		ID:      pid,
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		log.Warnf("error while marhsalling rpc invalid request. "+
			"Error: %s", err)
		return
	}
	bytes = append(bytes, delim)
	s.writeChan <- []connAndBytes{{conn, bytes}}
}

func (s *ElectrumServer) writeResponse(conn net.Conn, msg *btcjson.Request, result interface{}) {
	marshalledResult, err := json.Marshal(result)
	if err != nil {
		log.Warnf("Errored while marshaling result for method %s. Sending error message to %v err: %v\n",
			msg.Method, conn.RemoteAddr().String(), err)
		rpcError := btcjson.RPCError{
			Code: btcjson.ErrRPCInternal.Code,
			Message: fmt.Sprintf("%s: error: %s",
				btcjson.ErrRPCInternal.Message, err.Error()),
		}
		pid := &msg.ID
		s.writeErrorResponse(conn, rpcError, pid)
		return
	}

	pid := &msg.ID
	resp := btcjson.Response{
		Jsonrpc: btcjson.RpcVersion2,
		Result:  json.RawMessage(marshalledResult),
		ID:      pid,
	}

	bytes, err := json.Marshal(resp)
	if err != nil {
		log.Warnf("Errored while marshaling response for method %s. Sending error message to %v err: %v\n",
			msg.Method, conn.RemoteAddr().String(), err)
		rpcError := btcjson.RPCError{
			Code: btcjson.ErrRPCInternal.Code,
			Message: fmt.Sprintf("%s: error: %s",
				btcjson.ErrRPCInternal.Message, err.Error()),
		}
		pid := &msg.ID
		s.writeErrorResponse(conn, rpcError, pid)
		return
	}
	bytes = append(bytes, delim)

	log.Debugf("put %v to be written to %v\n", result, conn.RemoteAddr().String())
	s.writeChan <- []connAndBytes{{conn, bytes}}
	//_, err = conn.Write(bytes)
	//if err != nil {
	//	log.Warnf("error while writing to %s. Error: %s",
	//		conn.RemoteAddr().String(), err)
	//}
}

func (s *ElectrumServer) handleSingleMsg(msg btcjson.Request, conn net.Conn) {
	log.Debugf("unmarshalled %v from conn %s\n", msg, conn.RemoteAddr().String())
	handler, found := rpcHandlers[msg.Method]
	if !found || handler == nil {
		log.Warnf("handler not found for method %v. Sending error msg to %v",
			msg.Method, conn.RemoteAddr().String())
		pid := &msg.ID
		s.writeErrorResponse(conn, *btcjson.ErrRPCMethodNotFound, pid)
		return
	}

	_, err := handler(s, &msg, conn, nil)
	if err != nil {
		log.Warnf("Errored while handling method %s. Sending error message to %v err: %v\n",
			msg.Method, conn.RemoteAddr().String(), err)

		pid := &msg.ID
		rpcError := btcjson.RPCError{
			Code:    1,
			Message: err.Error(),
		}
		s.writeErrorResponse(conn, rpcError, pid)
		return
	}
}

func (s *ElectrumServer) handleConnection(conn net.Conn) {
	// The timer is stopped when a new message is received and reset after it
	// is processed.
	idleTimer := time.AfterFunc(idleTimeout, func() {
		log.Infof("Electrum client %s no answer for %s -- disconnecting",
			conn.RemoteAddr(), idleTimeout)
		conn.Close()
	})

	defer conn.Close()
	reader := bufio.NewReader(conn)
	for atomic.LoadInt32(&s.shutdown) == 0 {
		line, err := reader.ReadBytes(delim)
		if err != nil {
			if err != io.EOF {
				log.Warnf("error while reading message from peer. "+
					"Error: %v", err)
			}

			break
		}
		idleTimer.Stop()

		// Attempt to unmarshal as a batched json request.
		msgs := []btcjson.Request{}
		err = json.Unmarshal(line, &msgs)
		if err != nil {
			// If that fails, attempt to  unmarshal as a single request.
			msg := btcjson.Request{}
			err = msg.UnmarshalJSON(line)
			if err != nil {
				log.Warnf("error while unmarshalling %v. Sending error message to %v. Error: %v",
					hex.EncodeToString(line), conn.RemoteAddr().String(), err)
				if e, ok := err.(*json.SyntaxError); ok {
					log.Warnf("syntax error at byte offset %d", e.Offset)
				}

				pid := &msgs[0].ID
				s.writeErrorResponse(conn, *btcjson.ErrRPCParse, pid)
				continue
			}

			// If the single request unmarshal was successful, append to the
			// msgs for processing below.
			msgs = append(msgs, msg)
		}

		for _, msg := range msgs {
			s.handleSingleMsg(msg, conn)
			idleTimer.Reset(idleTimeout)
		}
	}

	idleTimer.Stop()

	delete(s.headersSubscribers, conn)
	delete(s.scriptHashSubscribers, conn)

	log.Infof("Electrum client %s closed connection", conn.RemoteAddr())
}

func (s *ElectrumServer) writeToClient() {
	for {
		select {
		case <-s.quit:
			return
		case d := <-s.writeChan:
			for _, cb := range d {
				_, err := cb.conn.Write(cb.bytes)
				if err != nil {
					log.Warnf("error while writing to %s. Error: %s",
						cb.conn.RemoteAddr().String(), err)
					continue
				}
			}
		}
	}
}

func (s *ElectrumServer) listen(listener net.Listener) {
	defer s.wg.Done()

	var tempDelay time.Duration // how long to sleep on accept failure
	for atomic.LoadInt32(&s.shutdown) == 0 {
		conn, err := listener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				log.Infof("Electrum server: Accept error: %v; "+
					"retrying in %v", err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return
		}

		go s.handleConnection(conn)
	}
}

func (s *ElectrumServer) handleBlockChainNotification(notification *blockchain.Notification) {
	switch notification.Type {
	// A block has been accepted into the block chain.
	case blockchain.NTBlockConnected:
		block, ok := notification.Data.(*btcutil.Block)
		if !ok {
			log.Warnf("Chain connected notification is not a block.")
			return
		}

		stxos, err := s.cfg.BlockChain.FetchSpendJournal(block)
		if err != nil {
			log.Warnf("Couldn't serialize the header for block %s",
				block.Hash().String())
		}
		for _, stxo := range stxos {
			cp := make([]byte, len(stxo.PkScript))
			copy(cp, stxo.PkScript)

			go func(pkScript []byte) {
				s.maybeUpdateSubscribers(pkScript)
			}(cp)
		}

		for _, tx := range block.Transactions() {
			for _, txOut := range tx.MsgTx().TxOut {
				cp := make([]byte, len(txOut.PkScript))
				copy(cp, txOut.PkScript)

				go func(pkScript []byte) {
					s.maybeUpdateSubscribers(pkScript)
				}(cp)
			}
		}

		var buf bytes.Buffer
		err = block.MsgBlock().Header.Serialize(&buf)
		if err != nil {
			log.Warnf("Couldn't serialize the header for block %s",
				block.Hash().String())
			return
		}
		params := HeadersSubscribeResponse{
			Hex:    hex.EncodeToString(buf.Bytes()),
			Height: int(block.Height()),
		}
		paramBytes, err := json.Marshal(params)
		if err != nil {
			log.Warnf("error while marshalling json response for block header %s. "+
				"Error: %v", block.Hash().String(), err)
			return
		}

		resp := btcjson.Request{
			Jsonrpc: btcjson.RpcVersion2,
			Method:  "blockchain.headers.subscribe",
			Params:  []json.RawMessage{paramBytes},
		}

		bytes, err := json.Marshal(resp)
		if err != nil {
			log.Warnf("error while marshalling block header %s. "+
				"Error: %v", block.Hash().String(), err)
			return
		}
		bytes = append(bytes, delim)

		cb := make([]connAndBytes, 0, len(s.headersSubscribers))
		for conn := range s.headersSubscribers {
			log.Debugf("blockchain.headers.subscribe handler: put %v to be written to %v\n",
				resp, conn.RemoteAddr().String())
			cb = append(cb, connAndBytes{conn, bytes})
		}

		s.writeChan <- cb
	}
}

func (s *ElectrumServer) handleMempoolNotification(notification *mempool.Notification) {
	switch notification.Type {
	case mempool.NTTxAccepted:
		acceptedData, ok := notification.Data.(*mempool.NTTxAcceptedData)
		if !ok {
			log.Warnf("Tx accepted notification is not an tx accepted data. Got %T",
				notification.Data)
			break
		}
		tx := acceptedData.Tx
		utxoView := acceptedData.UtxoView

		log.Infof("got mempool notification for tx %v", tx.Hash().String())

		for _, txIn := range tx.MsgTx().TxIn {
			entry := utxoView.LookupEntry(txIn.PreviousOutPoint)
			if entry == nil || entry.PkScript() == nil {
				// Ignore missing entries.  This should never happen
				// in practice since the function comments specifically
				// call out all inputs must be available.
				continue
			}

			cp := make([]byte, len(entry.PkScript()))
			copy(cp, entry.PkScript())

			go func(pkScript []byte) {
				s.maybeUpdateSubscribers(pkScript)
			}(cp)
		}

		for _, txOut := range tx.MsgTx().TxOut {
			cp := make([]byte, len(txOut.PkScript))
			copy(cp, txOut.PkScript)

			go func(pkScript []byte) {
				s.maybeUpdateSubscribers(pkScript)
			}(cp)
		}
	}
}

func getScriptHashStr(scriptHash [32]byte) string {
	sort.SliceStable(scriptHash[:], func(i, j int) bool {
		return i > j
	})
	scriptHashStr := hex.EncodeToString(scriptHash[:])

	return scriptHashStr
}

func (s *ElectrumServer) maybeUpdateSubscribers(pkScript []byte) {
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(
		pkScript, s.cfg.Params)
	if err != nil || len(addrs) != 1 {
		// We skip when there are multiple addresses as
		// that means the pkscript is for a raw multisig
		// output.  Since the address manager isn't
		// keeping track of them anyways, we simply
		// skip.
		return
	}
	addr := addrs[0]

	scriptHash := sha256.Sum256(pkScript)
	scriptHashStr := getScriptHashStr(scriptHash)

	for conn, scriptHashMap := range s.scriptHashSubscribers {
		_, found := scriptHashMap[scriptHashStr]
		if found {
			hist, err := scriptHashHistory(s, addr)
			if err != nil {
				log.Infof("scripthash %v history not found", scriptHashStr)
				return
			}

			mempoolHist, err := scriptHashMempool(s, addr)
			if err != nil {
				log.Infof("scripthash %v mempool history not found", scriptHashStr)
				return
			}

			log.Infof("found scripthash %v", scriptHashStr)
			status := getStatusFromHistory(hist, mempoolHist)
			log.Infof("scripthash %v status %v",
				scriptHashStr, hex.EncodeToString(status[:]))

			sendScriptStatus(s, conn, scriptHash, status)
		}
	}
}

func sendScriptStatus(s *ElectrumServer, conn net.Conn, scriptHash, status [32]byte) {
	params := []string{
		hex.EncodeToString(scriptHash[:]),
		hex.EncodeToString(status[:]),
	}

	rawParams := make([]json.RawMessage, 0, len(params))
	for _, param := range params {
		marshalledParam, err := json.Marshal(param)
		if err != nil {
			log.Warnf("error while marshalling json response for script status for: %s. "+
				"Error: %v", hex.EncodeToString(scriptHash[:]), err)
			break
		}
		rawMessage := json.RawMessage(marshalledParam)
		rawParams = append(rawParams, rawMessage)
	}
	resp := btcjson.Request{
		Jsonrpc: btcjson.RpcVersion2,
		Method:  "blockchain.scripthash.subscribe",
		Params:  rawParams,
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		log.Warnf("error while marshalling script status for script hash %s. "+
			"Error: %v", hex.EncodeToString(scriptHash[:]), err)
		return
	}
	bytes = append(bytes, delim)

	log.Infof("sending scriptHash %v, status %v to conn %v",
		params[0], params[1], conn.RemoteAddr())
	s.writeChan <- []connAndBytes{{conn, bytes}}
	log.Infof("sent scriptHash %v, status %v to conn %v",
		params[0], params[1], conn.RemoteAddr())
}

func (s *ElectrumServer) Start() {
	if atomic.AddInt32(&s.started, 1) != 1 {
		return
	}

	log.Infof("Starting the electrum server")

	for i, listener := range s.cfg.Listeners {
		s.wg.Add(1)
		log.Infof("Electrum server listening on %s. ssl %v",
			listener.Addr().String(), s.cfg.ListenerTLS[i])

		go func(l net.Listener) {
			go s.listen(l)
		}(listener)
	}
}

func (s *ElectrumServer) Stop() {
	if atomic.AddInt32(&s.shutdown, 1) != 1 {
		log.Infof("Electrum server is already in the process of shutting down")
		return
	}
	log.Infof("Stopping Electrum server...")

	for _, listener := range s.cfg.Listeners {
		err := listener.Close()
		if err != nil {
			log.Errorf("Problem shutting down electrum server: %v", err)
		}
	}

	close(s.quit)
	s.wg.Wait()

	log.Infof("Electrum server stopped")
}

// Config is a configuration struct used to initialize a new electrum server.
type Config struct {
	// Listeners defines a slice of listeners for which the RPC server will
	// take ownership of and accept connections.  Since the RPC server takes
	// ownership of these listeners, they will be closed when the RPC server
	// is stopped.
	Listeners []net.Listener

	ListenerTLS []bool

	// MaxClients is the amount of clients that are allowed to connect to the
	// electrum server. Set to -1 to have no limits.
	MaxClients int32

	Params          *chaincfg.Params
	BlockChain      *blockchain.BlockChain
	TxIndex         *indexers.TxIndex
	AddrIndex       *indexers.AddrIndex
	ScriptHashIndex *indexers.ScriptHashIndex
	FeeEstimator    *mempool.FeeEstimator
	Mempool         *mempool.TxPool
	MinRelayFee     btcutil.Amount

	AddRebroadcastInventory func(iv *wire.InvVect, data interface{})
	AnnounceNewTransactions func(txns []*mempool.TxDesc)
}

// New constructs a new instance of the electrum server.
func New(config *Config, db database.DB) (*ElectrumServer, error) {
	s := ElectrumServer{
		cfg:                   config,
		db:                    db,
		quit:                  make(chan int),
		writeChan:             make(chan []connAndBytes, 100),
		headersSubscribers:    make(map[net.Conn]struct{}),
		scriptHashSubscribers: make(map[net.Conn]map[string]struct{}),
	}

	s.cfg.BlockChain.Subscribe(s.handleBlockChainNotification)
	s.cfg.Mempool.Subscribe(s.handleMempoolNotification)

	go s.writeToClient()

	return &s, nil
}

func init() {
	rpcHandlers = rpcHandlersBeforeInit
	rand.Seed(time.Now().UnixNano())
}
