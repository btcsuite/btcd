// Copyright (c) 2016-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpctest

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/debugstream"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire/v2"
)

const (
	// These constants define the minimum and maximum p2p and rpc port
	// numbers used by a test harness.  The min port is inclusive while the
	// max port is exclusive.
	minPeerPort = 10000
	maxPeerPort = 35000
	minRPCPort  = maxPeerPort
	maxRPCPort  = 60000

	// BlockVersion is the default block version used when generating
	// blocks.
	BlockVersion = 4

	// DefaultMaxConnectionRetries is the default number of times we re-try
	// to connect to the node after starting it.
	DefaultMaxConnectionRetries = 20

	// DefaultConnectionRetryTimeout is the default duration we wait between
	// two connection attempts.
	DefaultConnectionRetryTimeout = 50 * time.Millisecond
)

var (
	// current number of active test nodes.
	numTestInstances = 0

	// testInstances is a private package-level slice used to keep track of
	// all active test harnesses. This global can be used to perform
	// various "joins", shutdown several active harnesses after a test,
	// etc.
	testInstances = make(map[string]*Harness)

	// Used to protest concurrent access to above declared variables.
	harnessStateMtx sync.RWMutex

	// ListenAddressGenerator is a function that is used to generate two
	// listen addresses (host:port), one for the P2P listener and one for
	// the RPC listener. This is exported to allow overwriting of the
	// default behavior which isn't very concurrency safe (just selecting
	// a random port can produce collisions and therefore flakes).
	ListenAddressGenerator = generateListeningAddresses

	// defaultNodePort is the start of the range for listening ports of
	// harness nodes. Ports are monotonically increasing starting from this
	// number and are determined by the results of nextAvailablePort().
	defaultNodePort uint32 = 8333

	// ListenerFormat is the format string that is used to generate local
	// listener addresses.
	ListenerFormat = "127.0.0.1:%d"

	// lastPort is the last port determined to be free for use by a new
	// node. It should be used atomically.
	//
	// Seed with a random offset so concurrent `go test` processes
	// (e.g. when integration/ and integration/rpctest/ run in parallel
	// under `make unit`) do not race on the same port range. The
	// bind-test in NextAvailablePort closes the listener before
	// returning, leaving a window where another process could grab the
	// same port; staggering each process's starting point avoids the
	// collision. The 50k-port window leaves headroom below 65535.
	lastPort uint32 = defaultNodePort + rand.Uint32N(50000)
)

// HarnessTestCase represents a test-case which utilizes an instance of the
// Harness to exercise functionality.
type HarnessTestCase func(r *Harness, t *testing.T)

// Harness fully encapsulates an active btcd process to provide a unified
// platform for creating rpc driven integration tests involving btcd. The
// active btcd node will typically be run in simnet mode in order to allow for
// easy generation of test blockchains.  The active btcd process is fully
// managed by Harness, which handles the necessary initialization, and teardown
// of the process along with any temporary directories created as a result.
// Multiple Harness instances may be run concurrently, in order to allow for
// testing complex scenarios involving multiple nodes. The harness also
// includes an in-memory wallet to streamline various classes of tests.
type Harness struct {
	// ActiveNet is the parameters of the blockchain the Harness belongs
	// to.
	ActiveNet *chaincfg.Params

	// MaxConnRetries is the maximum number of times we re-try to connect to
	// the node after starting it.
	MaxConnRetries int

	// ConnectionRetryTimeout is the duration we wait between two connection
	// attempts.
	ConnectionRetryTimeout time.Duration

	Client      *rpcclient.Client
	BatchClient *rpcclient.Client
	node        *node
	handlers    *rpcclient.NotificationHandlers

	wallet *memWallet

	debugClient *debugstream.Client

	testNodeDir string
	nodeNum     int

	sync.Mutex
}

// Opts are options that can be passed to [New] when creating the [Harness],
// otherwise default configuration is used.
type Opts struct {
	// Net allows overriding the default SimNet chain config params.
	Net *chaincfg.Params

	// Handlers are websocket handlers that can be passed.
	Handlers *rpcclient.NotificationHandlers

	// CustomExePath can be set to the path of a custom btcd executable,
	// otherwise we build and use a default executable.
	CustomExePath string

	// DebugHandler makes the harness start btcd with the debug stream
	// enabled and use the DebugHandler as the handler of the debug
	// events coming from btcd.
	DebugHandler func(debugstream.Event)
}

// New creates and initializes new instance of the rpc test harness.
//
// To avoid the need of passing an empty or nil [Opts] to get default
// initialization, opts is variadic, but only the first argument (if present)
// is used.
//
// NOTE: This function is safe for concurrent access.
func New(opts ...Opts) (*Harness, error) {
	harnessStateMtx.Lock()
	defer harnessStateMtx.Unlock()

	var o Opts
	if len(opts) != 0 {
		o = opts[0]
	}
	if o.Net == nil {
		// Use SimNet as default network.
		o.Net = &chaincfg.SimNetParams
	}

	var extraArgs []string
	// Add a flag for the appropriate network type based on the provided
	// chain params.
	switch o.Net.Net {
	case wire.MainNet:
		// No extra flags since mainnet is the default
	case wire.TestNet3:
		extraArgs = append(extraArgs, "--testnet")
	case wire.TestNet:
		extraArgs = append(extraArgs, "--regtest")
	case wire.SimNet:
		extraArgs = append(extraArgs, "--simnet")
	default:
		return nil, fmt.Errorf("rpctest.New must be called with one " +
			"of the supported chain networks")
	}

	testDir, err := baseDir()
	if err != nil {
		return nil, err
	}

	testNodeDir, err := os.MkdirTemp(testDir, "rpc-node")
	if err != nil {
		return nil, err
	}

	certFile := filepath.Join(testNodeDir, "rpc.cert")
	keyFile := filepath.Join(testNodeDir, "rpc.key")
	if err := genCertPair(certFile, keyFile); err != nil {
		return nil, err
	}

	wallet, err := newMemWallet(o.Net, uint32(numTestInstances))
	if err != nil {
		return nil, err
	}
	miningAddr := fmt.Sprintf("--miningaddr=%s", wallet.coinbaseAddr)
	extraArgs = append(extraArgs, miningAddr)

	var debugClient *debugstream.Client
	if o.DebugHandler != nil {
		p := NextAvailablePort()
		streamAddr := fmt.Sprintf("127.0.0.1:%d", p)
		debugClient = debugstream.NewClient(streamAddr, o.DebugHandler)
		extraArgs = append(extraArgs, "--debugstream="+streamAddr)
	}

	config, err := newConfig(testNodeDir, certFile, keyFile, extraArgs,
		o.CustomExePath)
	if err != nil {
		return nil, err
	}

	// Generate p2p+rpc listening addresses.
	config.listen, config.rpcListen = ListenAddressGenerator()

	// Create the testing node.
	node, err := newNode(config, testNodeDir)
	if err != nil {
		return nil, err
	}

	nodeNum := numTestInstances
	numTestInstances++

	if o.Handlers == nil {
		o.Handlers = &rpcclient.NotificationHandlers{}
	}

	// If a handler for the OnFilteredBlock{Connected,Disconnected} callback
	// callback has already been set, then create a wrapper callback which
	// executes both the currently registered callback and the mem wallet's
	// callback.
	if o.Handlers.OnFilteredBlockConnected != nil {
		obc := o.Handlers.OnFilteredBlockConnected
		o.Handlers.OnFilteredBlockConnected = func(height int32,
			header *wire.BlockHeader, filteredTxns []*btcutil.Tx) {

			wallet.IngestBlock(height, header, filteredTxns)
			obc(height, header, filteredTxns)
		}
	} else {
		// Otherwise, we can claim the callback ourselves.
		o.Handlers.OnFilteredBlockConnected = wallet.IngestBlock
	}
	if o.Handlers.OnFilteredBlockDisconnected != nil {
		obd := o.Handlers.OnFilteredBlockDisconnected
		o.Handlers.OnFilteredBlockDisconnected = func(height int32,
			header *wire.BlockHeader) {

			wallet.UnwindBlock(height, header)
			obd(height, header)
		}
	} else {
		o.Handlers.OnFilteredBlockDisconnected = wallet.UnwindBlock
	}

	h := &Harness{
		handlers:               o.Handlers,
		node:                   node,
		MaxConnRetries:         DefaultMaxConnectionRetries,
		ConnectionRetryTimeout: DefaultConnectionRetryTimeout,
		testNodeDir:            testNodeDir,
		ActiveNet:              o.Net,
		nodeNum:                nodeNum,
		wallet:                 wallet,
		debugClient:            debugClient,
	}

	// Track this newly created test instance within the package level
	// global map of all active test instances.
	testInstances[h.testNodeDir] = h

	return h, nil
}

// SOpts are options that can be passed to [Harness.SetUp], otherwise default
// configuration is used.
type SOpts struct {
	// UTXOCount is the count of mature outputs to be mined in a generated
	// chain and added to the memwallet.
	UTXOCount uint32

	// Args can be used to pass extra arguments to the btcd instance.
	Args []string

	// NoRPCAndWallet can be set to skip initialization of RPC client
	// and mem wallet. Note that this option also disables the generation
	// of coins via the UTXOCount count.
	NoRPCAndWallet bool

	// NoWalletWait can be set to avoid mem wallet sync blocking on
	// [Harness.SetUp].
	NoWalletWait bool
}

// SetUp initializes the rpc test state. Initialization includes: starting up a
// btcd node, and depending on the harness configuration: starting the debug
// stream client, creating a websockets client, connecting it to the started
// node, and generating and submitting a testchain with the configured number
// of mature coinbase outputs.
//
// To avoid the need of passing an empty or nil [SOpts] to get default
// initialization, opts is variadic, but only the first argument (if present)
// is used.
//
// NOTE: This method and [Harness.TearDown] should always be called from the
// same goroutine as they are not concurrent safe.
func (h *Harness) SetUp(opts ...SOpts) error {
	var o SOpts
	if len(opts) != 0 {
		o = opts[0]
	}

	// Start the btcd node itself. This spawns a new process which will be
	// managed.
	if err := h.node.start(o.Args); err != nil {
		return fmt.Errorf("error starting node: %w", err)
	}

	if h.debugClient != nil {
		if err := h.debugClient.Start(); err != nil {
			return fmt.Errorf("error connecting debug client: %w",
				err)
		}
	}

	if o.NoRPCAndWallet {
		return nil
	}
	if err := h.connectRPCClient(); err != nil {
		return fmt.Errorf("error connecting RPC client: %w",
			err)
	}

	h.wallet.Start()

	// Filter transactions that pay to the coinbase associated with the
	// wallet.
	filterAddrs := []address.Address{h.wallet.coinbaseAddr}
	if err := h.Client.LoadTxFilter(true, filterAddrs, nil); err != nil {
		return err
	}

	// Ensure btcd properly dispatches our registered call-back for each new
	// block. Otherwise, the memWallet won't function properly.
	if err := h.Client.NotifyBlocks(); err != nil {
		return err
	}

	// Create a test chain with the desired number of mature coinbase
	// outputs.
	if o.UTXOCount > 0 {
		coinbaseMaturity := uint32(h.ActiveNet.CoinbaseMaturity)
		numToGenerate := coinbaseMaturity + o.UTXOCount
		_, err := h.Client.Generate(numToGenerate)
		if err != nil {
			return err
		}
	}

	if o.NoWalletWait {
		return nil
	}
	// Block until the wallet has fully synced up to the tip of the main
	// chain.
	_, height, err := h.Client.GetBestBlock()
	if err != nil {
		return err
	}
	ticker := time.NewTicker(time.Millisecond * 100)
	for i := 0; ; i++ {
		if i > 0 {
			<-ticker.C
		}
		walletHeight := h.wallet.SyncedHeight()
		if walletHeight == height {
			break
		}
	}
	ticker.Stop()

	return nil
}

// TOpts are options that can be passed to [Harness.TearDown], otherwise default
// configuration is used.
type TOpts struct {
	// NoNodeCleanup skips cleaning up node data, which enable tests that
	// need to restart the node.
	NoNodeCleanup bool

	// NoShutdownSignal waits for node shutdown without sending a stop
	// signal when stopping the node, which is useful for when we are
	// testing features that stops the node.
	NoShutdownSignal bool
}

// tearDown stops the running rpc test instance. By default all created
// processes are killed, and temporary directories removed.
//
// To avoid the need of passing an empty or nil [TOpts] to get default
// initialization, opts is variadic, but only the first argument (if present)
// is used.
//
// This function MUST be called with the harness state mutex held (for writes).
func (h *Harness) tearDown(opts ...TOpts) error {
	var o TOpts
	if len(opts) != 0 {
		o = opts[0]
	}

	if h.Client != nil {
		h.Client.Shutdown()
		h.Client.WaitForShutdown()
	}

	if h.BatchClient != nil {
		h.BatchClient.Shutdown()
		h.BatchClient.WaitForShutdown()
	}

	if h.debugClient != nil {
		// Is better to stop the client after stopping btcd, because
		// this way we can use the debugHandler to test the btcd
		// shutdown behavior.
		defer h.debugClient.Stop()
	}

	if h.wallet != nil {
		h.wallet.Stop()
	}

	err := h.node.shutdown(o.NoShutdownSignal)

	if !o.NoNodeCleanup {
		rmErr := os.RemoveAll(h.testNodeDir)
		if rmErr != nil {
			err = errors.Join(rmErr, err)
		}

		delete(testInstances, h.testNodeDir)
	}

	return err
}

// TearDown stops the running rpc test instance. By default all created
// processes are killed, and temporary directories removed.
//
// To avoid the need of passing an empty or nil [TOpts] to get default
// initialization, opts is variadic, but only the first argument (if present)
// is used.
//
// NOTE: This method and [Harness.SetUp] should always be called from the same
// goroutine as they are not concurrent safe.
func (h *Harness) TearDown(opts ...TOpts) error {
	harnessStateMtx.Lock()
	defer harnessStateMtx.Unlock()

	return h.tearDown(opts...)
}

// connectRPCClient attempts to establish an RPC connection to the created btcd
// process belonging to this Harness instance. If the initial connection
// attempt fails, this function will retry h.maxConnRetries times, backing off
// the time between subsequent attempts. If after h.maxConnRetries attempts,
// we're not able to establish a connection, this function returns with an
// error.
func (h *Harness) connectRPCClient() error {
	var client, batchClient *rpcclient.Client
	var err error

	rpcConf := h.node.config.rpcConnConfig()
	batchConf := h.node.config.rpcConnConfig()
	batchConf.HTTPPostMode = true
	for i := 0; i < h.MaxConnRetries; i++ {
		fail := false
		timeout := time.Duration(i) * h.ConnectionRetryTimeout
		if client == nil {
			client, err = rpcclient.New(&rpcConf, h.handlers)
			if err != nil {
				time.Sleep(timeout)
				fail = true
			}
		}
		if batchClient == nil {
			batchClient, err = rpcclient.NewBatch(&batchConf)
			if err != nil {
				time.Sleep(timeout)
				fail = true
			}
		}
		if !fail {
			break
		}
	}

	if client == nil || batchClient == nil {
		return fmt.Errorf("connection timeout, tried %d times with "+
			"timeout %v, last err: %w", h.MaxConnRetries,
			h.ConnectionRetryTimeout, err)
	}

	h.Client = client
	h.wallet.SetRPCClient(client)
	h.BatchClient = batchClient
	return nil
}

// NewAddress returns a fresh address spendable by the Harness' internal
// wallet.
//
// This function is safe for concurrent access.
func (h *Harness) NewAddress() (address.Address, error) {
	return h.wallet.NewAddress()
}

// ConfirmedBalance returns the confirmed balance of the Harness' internal
// wallet.
//
// This function is safe for concurrent access.
func (h *Harness) ConfirmedBalance() btcutil.Amount {
	return h.wallet.ConfirmedBalance()
}

// SendOutputs creates, signs, and finally broadcasts a transaction spending
// the harness' available mature coinbase outputs creating new outputs
// according to targetOutputs.
//
// This function is safe for concurrent access.
func (h *Harness) SendOutputs(targetOutputs []*wire.TxOut,
	feeRate btcutil.Amount) (*chainhash.Hash, error) {

	return h.wallet.SendOutputs(targetOutputs, feeRate)
}

// SendOutputsWithoutChange creates and sends a transaction that pays to the
// specified outputs while observing the passed fee rate and ignoring a change
// output. The passed fee rate should be expressed in sat/b.
//
// This function is safe for concurrent access.
func (h *Harness) SendOutputsWithoutChange(targetOutputs []*wire.TxOut,
	feeRate btcutil.Amount) (*chainhash.Hash, error) {

	return h.wallet.SendOutputsWithoutChange(targetOutputs, feeRate)
}

// CreateTransaction returns a fully signed transaction paying to the specified
// outputs while observing the desired fee rate. The passed fee rate should be
// expressed in satoshis-per-byte. The transaction being created can optionally
// include a change output indicated by the change boolean. Any unspent outputs
// selected as inputs for the crafted transaction are marked as unspendable in
// order to avoid potential double-spends by future calls to this method. If the
// created transaction is cancelled for any reason then the selected inputs MUST
// be freed via a call to UnlockOutputs. Otherwise, the locked inputs won't be
// returned to the pool of spendable outputs.
//
// This function is safe for concurrent access.
func (h *Harness) CreateTransaction(targetOutputs []*wire.TxOut,
	feeRate btcutil.Amount, change bool) (*wire.MsgTx, error) {

	return h.wallet.CreateTransaction(targetOutputs, feeRate, change)
}

// UnlockOutputs unlocks any outputs which were previously marked as
// unspendabe due to being selected to fund a transaction via the
// CreateTransaction method.
//
// This function is safe for concurrent access.
func (h *Harness) UnlockOutputs(inputs []*wire.TxIn) {
	h.wallet.UnlockOutputs(inputs)
}

// RPCConfig returns the harnesses current rpc configuration. This allows other
// potential RPC clients created within tests to connect to a given test
// harness instance.
func (h *Harness) RPCConfig() rpcclient.ConnConfig {
	return h.node.config.rpcConnConfig()
}

// P2PAddress returns the harness' P2P listening address. This allows potential
// peers (such as SPV peers) created within tests to connect to a given test
// harness instance.
func (h *Harness) P2PAddress() string {
	return h.node.config.listen
}

// GenerateAndSubmitBlock creates a block whose contents include the passed
// transactions and submits it to the running simnet node. For generating
// blocks with only a coinbase tx, callers can simply pass nil instead of
// transactions to be mined. Additionally, a custom block version can be set by
// the caller. A blockVersion of -1 indicates that the current default block
// version should be used. An uninitialized time.Time should be used for the
// blockTime parameter if one doesn't wish to set a custom time.
//
// This function is safe for concurrent access.
func (h *Harness) GenerateAndSubmitBlock(txns []*btcutil.Tx, blockVersion int32,
	blockTime time.Time) (*btcutil.Block, error) {
	return h.GenerateAndSubmitBlockWithCustomCoinbaseOutputs(txns,
		blockVersion, blockTime, []wire.TxOut{})
}

// GenerateAndSubmitBlockWithCustomCoinbaseOutputs creates a block whose
// contents include the passed coinbase outputs and transactions and submits
// it to the running simnet node. For generating blocks with only a coinbase tx,
// callers can simply pass nil instead of transactions to be mined.
// Additionally, a custom block version can be set by the caller. A blockVersion
// of -1 indicates that the current default block version should be used. An
// uninitialized time.Time should be used for the blockTime parameter if one
// doesn't wish to set a custom time. The mineTo list of outputs will be added
// to the coinbase; this is not checked for correctness until the block is
// submitted; thus, it is the caller's responsibility to ensure that the outputs
// are correct. If the list is empty, the coinbase reward goes to the wallet
// managed by the Harness.
//
// This function is safe for concurrent access.
func (h *Harness) GenerateAndSubmitBlockWithCustomCoinbaseOutputs(
	txns []*btcutil.Tx, blockVersion int32, blockTime time.Time,
	mineTo []wire.TxOut) (*btcutil.Block, error) {

	h.Lock()
	defer h.Unlock()

	if blockVersion == -1 {
		blockVersion = BlockVersion
	}

	prevBlockHash, prevBlockHeight, err := h.Client.GetBestBlock()
	if err != nil {
		return nil, err
	}
	mBlock, err := h.Client.GetBlock(prevBlockHash)
	if err != nil {
		return nil, err
	}
	prevBlock := btcutil.NewBlock(mBlock)
	prevBlock.SetHeight(prevBlockHeight)

	// Create a new block including the specified transactions
	newBlock, err := CreateBlock(prevBlock, txns, blockVersion,
		blockTime, h.wallet.coinbaseAddr, mineTo, h.ActiveNet)
	if err != nil {
		return nil, err
	}

	// Submit the block to the simnet node.
	if err := h.Client.SubmitBlock(newBlock, nil); err != nil {
		return nil, err
	}

	return newBlock, nil
}

// generateListeningAddresses is a function that returns two listener
// addresses with unique ports and should be used to overwrite rpctest's
// default generator which is prone to use colliding ports.
func generateListeningAddresses() (string, string) {
	return fmt.Sprintf(ListenerFormat, NextAvailablePort()),
		fmt.Sprintf(ListenerFormat, NextAvailablePort())
}

// NextAvailablePort returns the first port that is available for listening by
// a new node. It panics if no port is found and the maximum available TCP port
// is reached.
func NextAvailablePort() int {
	port := atomic.AddUint32(&lastPort, 1)
	for port < 65535 {
		// If there are no errors while attempting to listen on this
		// port, close the socket and return it as available. While it
		// could be the case that some other process picks up this port
		// between the time the socket is closed and it's reopened in
		// the harness node, in practice in CI servers this seems much
		// less likely than simply some other process already being
		// bound at the start of the tests.
		addr := fmt.Sprintf(ListenerFormat, port)
		l, err := net.Listen("tcp4", addr)
		if err == nil {
			err := l.Close()
			if err == nil {
				return int(port)
			}
		}
		port = atomic.AddUint32(&lastPort, 1)
	}

	// No ports available? Must be a mistake.
	panic("no ports available for listening")
}

// NextAvailablePortForProcess returns the first port that is available for
// listening by a new node, using a lock file to make sure concurrent access for
// parallel tasks within the same process don't re-use the same port. It panics
// if no port is found and the maximum available TCP port is reached.
func NextAvailablePortForProcess(pid int) int {
	lockFile := filepath.Join(
		os.TempDir(), fmt.Sprintf("rpctest-port-pid-%d.lock", pid),
	)
	timeout := time.After(time.Second)

	var (
		lockFileHandle *os.File
		err            error
	)
	for {
		// Attempt to acquire the lock file. If it already exists, wait
		// for a bit and retry.
		lockFileHandle, err = os.OpenFile(
			lockFile, os.O_CREATE|os.O_EXCL, 0600,
		)
		if err == nil {
			// Lock acquired.
			break
		}

		// Wait for a bit and retry.
		select {
		case <-timeout:
			panic("timeout waiting for lock file")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Release the lock file when we're done.
	defer func() {
		// Always close file first, Windows won't allow us to remove it
		// otherwise.
		_ = lockFileHandle.Close()
		err := os.Remove(lockFile)
		if err != nil {
			panic(fmt.Errorf("couldn't remove lock file: %w", err))
		}
	}()

	portFile := filepath.Join(
		os.TempDir(), fmt.Sprintf("rpctest-port-pid-%d", pid),
	)
	port, err := os.ReadFile(portFile)
	if err != nil {
		if !os.IsNotExist(err) {
			panic(fmt.Errorf("error reading port file: %w", err))
		}
		port = []byte(strconv.Itoa(int(defaultNodePort)))
	}

	lastPort, err := strconv.Atoi(string(port))
	if err != nil {
		panic(fmt.Errorf("error parsing port: %w", err))
	}

	// We take the next one.
	lastPort++
	for lastPort < 65535 {
		// If there are no errors while attempting to listen on this
		// port, close the socket and return it as available. While it
		// could be the case that some other process picks up this port
		// between the time the socket is closed and it's reopened in
		// the harness node, in practice in CI servers this seems much
		// less likely than simply some other process already being
		// bound at the start of the tests.
		addr := fmt.Sprintf(ListenerFormat, lastPort)
		l, err := net.Listen("tcp4", addr)
		if err == nil {
			err := l.Close()
			if err == nil {
				err := os.WriteFile(
					portFile,
					[]byte(strconv.Itoa(lastPort)), 0600,
				)
				if err != nil {
					panic(fmt.Errorf("error updating "+
						"port file: %w", err))
				}

				return lastPort
			}
		}
		lastPort++
	}

	// No ports available? Must be a mistake.
	panic("no ports available for listening")
}

// GenerateProcessUniqueListenerAddresses is a function that returns two
// listener addresses with unique ports per the given process id and should be
// used to overwrite rpctest's default generator which is prone to use colliding
// ports.
func GenerateProcessUniqueListenerAddresses(pid int) (string, string) {
	port1 := NextAvailablePortForProcess(pid)
	port2 := NextAvailablePortForProcess(pid)
	return fmt.Sprintf(ListenerFormat, port1),
		fmt.Sprintf(ListenerFormat, port2)
}

// baseDir is the directory path of the temp directory for all rpctest files.
func baseDir() (string, error) {
	dirPath := filepath.Join(os.TempDir(), "btcd", "rpctest")
	err := os.MkdirAll(dirPath, 0755)
	return dirPath, err
}
