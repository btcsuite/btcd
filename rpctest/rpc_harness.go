// Copyright (c) 2015 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpctest

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	rpc "github.com/btcsuite/btcrpcclient"
	"github.com/btcsuite/btcsim/simnode"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/coinset"
)

var (
	// tempDataDir is the name of the temporary directory used by the test harness.
	tempDataDir = "testnode"

	// current number of active test nodes.
	numTestInstances = 0

	defaultP2pPort = 18555
	defaultRpcPort = 18556

	// Used to protest concurrent access to above declared variables.
	testCreationLock sync.Mutex
)

// RpcTestState represents an instance of the rpc test harness.
type RpcTestState struct {
	*simnode.Node

	minedBlocks []*btcutil.Block
	currentTip  *btcutil.Block

	matureCoinbases []coinset.Coin
	utxoSelector    *coinset.MinNumberCoinSelector
	testNodeDir     string
}

// NewRpcTestState creates and initializes new instance of the rpc test harness.
// Optionally, websocket handlers and a specified configuration may be passed.
// In the case that a nil config is passed, a default configuration will be used.
func New(handlers *rpc.NotificationHandlers, config *simnode.BtcdArgs) (*RpcTestState, error) {
	testCreationLock.Lock()
	defer testCreationLock.Unlock()

	nodeTestData := tempDataDir + strconv.Itoa(numTestInstances)
	certFile := filepath.Join(nodeTestData, "rpc.cert")
	keyFile := filepath.Join(nodeTestData, "rpc.key")

	// Create folder to store our tls info.
	if err := os.Mkdir(nodeTestData, 0700); err != nil {
		return nil, err
	}

	// Generate the default config if needed.
	// TODO(roasbeef): Intelligent merge of configs. Would only really mess
	// with addrindex stuff?
	var c *simnode.BtcdArgs
	if config == nil {
		if err := simnode.GenCertPair(certFile, keyFile); err != nil {
			return nil, err
		}

		args, err := simnode.NewBtcdArgs("rpctest", certFile, keyFile)
		if err != nil {
			return nil, err
		}
		c = args
	} else {
		c = config
	}

	// Generate p2p+rpc listening addresses.
	p2p, rpc := generateListeningAddresses()
	c.Listen = p2p
	c.RPCListen = rpc

	// Create the testing node bounded to the simnet.
	node, err := simnode.NewNodeFromArgs(c, handlers, nil, 20, nodeTestData)
	if err != nil {
		return nil, err
	}

	numTestInstances++

	var blks []*btcutil.Block
	var mOuts []coinset.Coin
	return &RpcTestState{
		Node:            node,
		minedBlocks:     blks,
		currentTip:      nil,
		matureCoinbases: mOuts,
		// TODO(roasbeef): PR to change readme to correct.
		utxoSelector: &coinset.MinNumberCoinSelector{
			MaxInputs:       10,
			MinChangeAmount: 10000,
		},
		testNodeDir: nodeTestData,
	}, nil
}

// TearDown stops the running rpc test instance. All created processes are
// killed, and temporary directories removed.
func (r *RpcTestState) TearDown() error {
	if r.Client != nil {
		r.Client.Shutdown()
	}

	if err := r.Stop(); err != nil {
		return err
	}

	if err := r.Cleanup(); err != nil {
		return err
	}

	if err := os.RemoveAll(r.testNodeDir); err != nil {
		return err
	}
	return nil
}

// SetUp initializes the rpc test state. Initialization includes: starting up a
// simnet node, creating a websocket client and connecting to the started node,
// and finally: optionally generating and submitting a testchain with 25 mature
// coinbase outputs.
func (r *RpcTestState) SetUp(createTestChain bool) error {
	if err := r.Start(); err != nil {
		return err
	}

	if err := r.Connect(); err != nil {
		return err
	}

	// Create a test chain with 25 mature coinbase outputs.
	if createTestChain {
		// Grab info about the genesis block.
		genHash, err := r.Client.GetBestBlockHash()
		if err != nil {
			return nil
		}
		prevBlock, err := r.Client.GetBlock(genHash)
		if err != nil {
			return nil
		}

		// Create a chain of length 125, this'll give us 25 mature
		// coinbase outputs for spending.
		chainLength := 0
		for chainLength < 125 {
			newBlock, err := createBlock(prevBlock, nil)
			if err != nil {
				return err
			}

			if err := r.Client.SubmitBlock(newBlock, nil); err != nil {
				return err
			}

			r.incrementCbConfirmations()

			r.currentTip = newBlock
			r.minedBlocks = append(r.minedBlocks, newBlock)

			chainLength++
			// Once the chain length is greater than 100, begin
			// storing mature coinbase tx's for later use.
			if chainLength > 100 {
				cbTx := r.minedBlocks[chainLength-100].Transactions()[0]
				cbCoin := &coinset.SimpleCoin{
					Tx:         cbTx,
					TxIndex:    0,
					TxNumConfs: 100,
				}
				r.matureCoinbases = append(r.matureCoinbases,
					coinset.Coin(cbCoin))
			}

			prevBlock = newBlock
		}
	}

	return nil
}

// GenerateAndSubmitBlock creates a block whose contents include the passed
// transactions and submits it to the running simnet node. For generating
// blocks with only a coinbase tx, callers can simply pass nil instead of
// transactions to be mined.
func (r *RpcTestState) GenerateAndSubmitBlock(txs []*btcutil.Tx) (*btcutil.Block, error) {
	// Create a new block including the specified transactions.
	newBlock, err := createBlock(r.currentTip, txs)
	if err != nil {
		return nil, err
	}

	// Submit the block to the simnet node.
	if err := r.Client.SubmitBlock(newBlock, nil); err != nil {
		return nil, err
	}
	r.incrementCbConfirmations()

	// Update current block state.
	r.currentTip = newBlock
	r.minedBlocks = append(r.minedBlocks, newBlock)

	// Add the coinbase output 100 blocks back to the mature output set.
	cbTx := r.minedBlocks[r.currentTip.Height()-100].Transactions()[0]
	cbCoin := &coinset.SimpleCoin{
		Tx:         cbTx,
		TxIndex:    0,
		TxNumConfs: 100,
	}
	r.matureCoinbases = append(r.matureCoinbases, coinset.Coin(cbCoin))

	// Add newly mined ouputs we can spend.
	for _, tx := range txs {
		for outIdx, txOut := range tx.MsgTx().TxOut {
			if bytes.Equal(txOut.PkScript, miningPkScript) {
				r.matureCoinbases = append(r.matureCoinbases,
					coinset.Coin(&coinset.SimpleCoin{
						Tx:         tx,
						TxIndex:    uint32(outIdx),
						TxNumConfs: 0,
					}))

			}
		}
	}

	return newBlock, nil
}

// incrementCbConfirmations increments the number of confirmations for each
// stored mature coinbase tx by 1.
func (r *RpcTestState) incrementCbConfirmations() {
	for _, cb := range r.matureCoinbases {
		cb.(*coinset.SimpleCoin).TxNumConfs++
	}
}

// CraftCoinbaseSpend returns a signed transaction spending selected coinbase
// ouputs which satisfy the value spending requirements of the passed tx outputs.
// Selected coinbase outputs are removed from the pool of mature coinbase outputs.
// Callers are expected to provide proper change outputs, if needed.
func (r *RpcTestState) CraftCoinbaseSpend(outputs []*wire.TxOut) (*wire.MsgTx, error) {
	total := int64(0)
	for _, txOut := range outputs {
		total += txOut.Value
	}

	selectedCoins, err := r.utxoSelector.CoinSelect(btcutil.Amount(total),
		r.matureCoinbases)
	if err != nil {
		return nil, err
	}

	// Remove selected coins.
	for _, coin := range selectedCoins.Coins() {
		for idx, cb := range r.matureCoinbases {
			if bytes.Equal(cb.Hash()[:], coin.Hash()[:]) {
				s := r.matureCoinbases
				s = append(s[:idx], s[idx+1:]...)
				r.matureCoinbases = s
				break
			}
		}
	}

	mtx := coinset.NewMsgTxWithInputCoins(selectedCoins)
	mtx.TxOut = outputs

	scriptSig, err := genScriptSig(mtx)
	for _, txin := range mtx.TxIn {
		txin.SignatureScript = scriptSig
	}

	return mtx, nil
}

// genScriptSig generates a valid scriptSig for spending a created coinbase
// output. The input transactions of 'redeemTx' must only reference coinbase
// outputs created by the test harness.
func genScriptSig(redeemTx *wire.MsgTx) ([]byte, error) {
	lookup := func(a btcutil.Address) (*btcec.PrivateKey, bool, error) {
		return miningPriv.PrivKey, true, nil
	}

	scriptSig, err := txscript.SignTxOutput(&chaincfg.SimNetParams,
		redeemTx, 0, miningPkScript, txscript.SigHashAll,
		txscript.KeyClosure(lookup), nil, nil)
	if err != nil {
		return nil, err
	}

	return scriptSig, nil
}

// generateListeningAddresses returns two strings representing listening
// addresses designated for the current rpc test. If there haven't been any
// test instances created, the default ports are used. Otherwise, in order to
// support multiple test nodes running at once, the p2p and rpc port are
// incremented after each initialization.
func generateListeningAddresses() (string, string) {
	var p2p, rpc string
	localhost := "127.0.0.1"
	if numTestInstances == 0 {
		p2p = net.JoinHostPort(localhost, strconv.Itoa(defaultP2pPort))
		rpc = net.JoinHostPort(localhost, strconv.Itoa(defaultRpcPort))
	} else {
		p2p = net.JoinHostPort(localhost, strconv.Itoa(defaultP2pPort+numTestInstances))
		rpc = net.JoinHostPort(localhost, strconv.Itoa(defaultRpcPort+numTestInstances))
	}

	return p2p, rpc
}
