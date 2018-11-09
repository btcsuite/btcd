// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package memwallet

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/integration"
	"github.com/btcsuite/btcd/integration/harness"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/hdkeychain"
)

// InMemoryWallet is a simple in-memory wallet whose purpose is to provide basic
// wallet functionality to the harness. The wallet uses a hard-coded HD key
// hierarchy which promotes reproducibility between harness test runs.
// Implements harness.TestWallet.
type InMemoryWallet struct {
	coinbaseKey  *btcec.PrivateKey
	coinbaseAddr btcutil.Address

	// hdRoot is the root master private key for the wallet.
	hdRoot *hdkeychain.ExtendedKey

	// hdIndex is the next available key index offset from the hdRoot.
	hdIndex uint32

	// currentHeight is the latest height the wallet is known to be synced
	// to.
	currentHeight int32

	// addrs tracks all addresses belonging to the wallet. The addresses
	// are indexed by their keypath from the hdRoot.
	addrs map[uint32]btcutil.Address

	// utxos is the set of utxos spendable by the wallet.
	utxos map[wire.OutPoint]*utxo

	// reorgJournal is a map storing an undo entry for each new block
	// received. Once a block is disconnected, the undo entry for the
	// particular height is evaluated, thereby rewinding the effect of the
	// disconnected block on the wallet's set of spendable utxos.
	reorgJournal map[int32]*undoEntry

	chainUpdates []*chainUpdate

	// chainUpdateSignal is a wallet event queue
	chainUpdateSignal chan string

	chainMtx sync.Mutex

	net *chaincfg.Params

	nodeRPC *rpcclient.Client

	sync.RWMutex
}

// Network returns current network of the wallet
func (wallet *InMemoryWallet) Network() *chaincfg.Params {
	return wallet.net
}

// Start wallet process
func (wallet *InMemoryWallet) Start(args *harness.TestWalletStartArgs) error {
	handlers := &rpcclient.NotificationHandlers{}

	// executes both the currently registered callback, and the mem
	// wallet's callback.
	// If a handler for the OnFilteredBlock{Connected,Disconnected} callback
	// callback has already been set, then create a wrapper callback which
	// executes both the currently registered callback and the mem wallet's
	// callback.
	if handlers.OnFilteredBlockConnected != nil {
		obc := handlers.OnFilteredBlockConnected
		handlers.OnFilteredBlockConnected = func(height int32, header *wire.BlockHeader, filteredTxns []*btcutil.Tx) {
			wallet.IngestBlock(height, header, filteredTxns)
			obc(height, header, filteredTxns)
		}
	} else {
		// Otherwise, we can claim the callback ourselves.
		handlers.OnFilteredBlockConnected = wallet.IngestBlock
	}
	if handlers.OnFilteredBlockDisconnected != nil {
		obd := handlers.OnFilteredBlockDisconnected
		handlers.OnFilteredBlockDisconnected = func(height int32, header *wire.BlockHeader) {
			wallet.UnwindBlock(height, header)
			obd(height, header)
		}
	} else {
		handlers.OnFilteredBlockDisconnected = wallet.UnwindBlock
	}

	//handlers.OnClientConnected = wallet.onDcrdConnect

	wallet.nodeRPC = harness.NewRPCConnection(args.NodeRPCConfig, 5, handlers)
	integration.AssertNotNil("nodeRPC", wallet.nodeRPC)

	// Filter transactions that pay to the coinbase associated with the
	// wallet.
	wallet.updateTxFilter()

	// Ensure dcrd properly dispatches our registered call-back for each new
	// block. Otherwise, the InMemoryWallet won't function properly.
	err := wallet.nodeRPC.NotifyBlocks()
	integration.CheckTestSetupMalfunction(err)

	go wallet.chainSyncer()
	return nil
}

func (wallet *InMemoryWallet) updateTxFilter() {
	filterAddrs := []btcutil.Address{}
	for _, v := range wallet.addrs {
		filterAddrs = append(filterAddrs, v)
	}
	//pin.D("filterAddrs", filterAddrs)
	err := wallet.nodeRPC.LoadTxFilter(true, filterAddrs, nil)
	integration.CheckTestSetupMalfunction(err)
}

// Stop wallet process gently, by sending stopSignal to the wallet event queue
func (wallet *InMemoryWallet) Stop() {
	go func() {
		wallet.chainUpdateSignal <- stopSignal
	}()
	wallet.nodeRPC.Disconnect()
	wallet.nodeRPC = nil
}

// Sync block until the wallet has fully synced up to the tip of the main
// chain.
func (wallet *InMemoryWallet) Sync() {
	_, height, err := wallet.nodeRPC.GetBestBlock()
	integration.CheckTestSetupMalfunction(err)
	ticker := time.NewTicker(time.Millisecond * 100)
	for range ticker.C {
		walletHeight := wallet.SyncedHeight()
		if walletHeight == height {
			break
		}
	}
}

// Dispose is no needed for InMemoryWallet
func (wallet *InMemoryWallet) Dispose() error {
	return nil
}

// SyncedHeight returns the height the wallet is known to be synced to.
//
// This function is safe for concurrent access.
func (wallet *InMemoryWallet) SyncedHeight() int32 {
	wallet.RLock()
	defer wallet.RUnlock()
	return wallet.currentHeight
}

// IngestBlock is a call-back which is to be triggered each time a new block is
// connected to the main chain. It queues the update for the chain syncer,
// calling the private version in sequential order.
func (wallet *InMemoryWallet) IngestBlock(height int32, header *wire.BlockHeader, filteredTxns []*btcutil.Tx) {
	// Append this new chain update to the end of the queue of new chain
	// updates.
	wallet.chainMtx.Lock()
	wallet.chainUpdates = append(wallet.chainUpdates, &chainUpdate{height,
		filteredTxns, true})
	wallet.chainMtx.Unlock()

	// Start a goroutine to signal the chainSyncer that a new update is
	// available. We do this in a new goroutine in order to avoid blocking
	// the main loop of the rpc client.
	go func() {
		wallet.chainUpdateSignal <- chainUpdateSignal
	}()
}

// ingestBlock updates the wallet's internal utxo state based on the outputs
// created and destroyed within each block.
func (wallet *InMemoryWallet) ingestBlock(update *chainUpdate) {
	// Update the latest synced height, then process each filtered
	// transaction in the block creating and destroying utxos within
	// the wallet as a result.
	wallet.currentHeight = update.blockHeight
	undo := &undoEntry{
		utxosDestroyed: make(map[wire.OutPoint]*utxo),
	}
	for _, tx := range update.filteredTxns {
		mtx := tx.MsgTx()
		isCoinbase := blockchain.IsCoinBaseTx(mtx)
		txHash := mtx.TxHash()
		wallet.evalOutputs(mtx.TxOut, &txHash, isCoinbase, undo)
		wallet.evalInputs(mtx.TxIn, undo)
	}

	// Finally, record the undo entry for this block so we can
	// properly update our internal state in response to the block
	// being re-org'd from the main chain.
	wallet.reorgJournal[update.blockHeight] = undo
}

// chainSyncer is a goroutine dedicated to processing new blocks in order to
// keep the wallet's utxo state up to date.
//
// NOTE: This MUST be run as a goroutine.
func (wallet *InMemoryWallet) chainSyncer() {
	var update *chainUpdate

	for s := range wallet.chainUpdateSignal {
		if s == stopSignal {
			break
		}
		// A new update is available, so pop the new chain update from
		// the front of the update queue.
		wallet.chainMtx.Lock()
		update = wallet.chainUpdates[0]
		wallet.chainUpdates[0] = nil // Set to nil to prevent GC leak.
		wallet.chainUpdates = wallet.chainUpdates[1:]
		wallet.chainMtx.Unlock()

		wallet.Lock()
		if update.isConnect {
			wallet.ingestBlock(update)
		} else {
			wallet.unwindBlock(update)
		}
		wallet.Unlock()
	}
}

// evalOutputs evaluates each of the passed outputs, creating a new matching
// utxo within the wallet if we're able to spend the output.
func (wallet *InMemoryWallet) evalOutputs(outputs []*wire.TxOut, txHash *chainhash.Hash, isCoinbase bool, undo *undoEntry) {
	for i, output := range outputs {
		pkScript := output.PkScript

		// Scan all the addresses we currently control to see if the
		// output is paying to us.
		for keyIndex, addr := range wallet.addrs {
			pkHash := addr.ScriptAddress()
			if !bytes.Contains(pkScript, pkHash) {
				continue
			}

			// If this is a coinbase output, then we mark the
			// maturity height at the proper block height in the
			// future.
			var maturityHeight int32
			if isCoinbase {
				maturityHeight = wallet.currentHeight + int32(wallet.net.CoinbaseMaturity)
			}

			op := wire.OutPoint{Hash: *txHash, Index: uint32(i)}
			wallet.utxos[op] = &utxo{
				value:          btcutil.Amount(output.Value),
				keyIndex:       keyIndex,
				maturityHeight: maturityHeight,
				pkScript:       pkScript,
			}
			undo.utxosCreated = append(undo.utxosCreated, op)
		}
	}
}

// evalInputs scans all the passed inputs, destroying any utxos within the
// wallet which are spent by an input.
func (wallet *InMemoryWallet) evalInputs(inputs []*wire.TxIn, undo *undoEntry) {
	for _, txIn := range inputs {
		op := txIn.PreviousOutPoint
		oldUtxo, ok := wallet.utxos[op]
		if !ok {
			continue
		}

		undo.utxosDestroyed[op] = oldUtxo
		delete(wallet.utxos, op)
	}
}

// UnwindBlock is a call-back which is to be executed each time a block is
// disconnected from the main chain. It queues the update for the chain syncer,
// calling the private version in sequential order.
func (wallet *InMemoryWallet) UnwindBlock(height int32, header *wire.BlockHeader) {
	// Append this new chain update to the end of the queue of new chain
	// updates.
	wallet.chainMtx.Lock()
	wallet.chainUpdates = append(wallet.chainUpdates, &chainUpdate{height,
		nil, false})
	wallet.chainMtx.Unlock()

	// Start a goroutine to signal the chainSyncer that a new update is
	// available. We do this in a new goroutine in order to avoid blocking
	// the main loop of the rpc client.
	go func() {
		wallet.chainUpdateSignal <- chainUpdateSignal
	}()
}

// unwindBlock undoes the effect that a particular block had on the wallet's
// internal utxo state.
func (wallet *InMemoryWallet) unwindBlock(update *chainUpdate) {
	undo := wallet.reorgJournal[update.blockHeight]

	for _, utxo := range undo.utxosCreated {
		delete(wallet.utxos, utxo)
	}

	for outPoint, utxo := range undo.utxosDestroyed {
		wallet.utxos[outPoint] = utxo
	}

	delete(wallet.reorgJournal, update.blockHeight)
}

// integrationdress returns a new address from the wallet's hd key chain.  It also
// loads the address into the RPC client's transaction filter to ensure any
// transactions that involve it are delivered via the notifications.
func (wallet *InMemoryWallet) integrationdress() (btcutil.Address, error) {
	index := wallet.hdIndex

	childKey, err := wallet.hdRoot.Child(index)
	if err != nil {
		return nil, err
	}
	privKey, err := childKey.ECPrivKey()
	if err != nil {
		return nil, err
	}

	addr, err := keyToAddr(privKey, wallet.net)
	if err != nil {
		return nil, err
	}

	err = wallet.nodeRPC.LoadTxFilter(false, []btcutil.Address{addr}, nil)
	if err != nil {
		return nil, err
	}

	wallet.addrs[index] = addr

	wallet.hdIndex++

	return addr, nil
}

// NewAddress returns a fresh address spendable by the wallet.
//
// This function is safe for concurrent access.
func (wallet *InMemoryWallet) NewAddress(_ *harness.NewAddressArgs) (btcutil.Address, error) {
	wallet.Lock()
	defer wallet.Unlock()

	return wallet.integrationdress()
}

// fundTx attempts to fund a transaction sending amt bitcoin. The coins are
// selected such that the final amount spent pays enough fees as dictated by the
// passed fee rate. The passed fee rate should be expressed in
// satoshis-per-byte. The transaction being funded can optionally include a
// change output indicated by the change boolean.
//
// NOTE: The memWallet's mutex must be held when this function is called.
func (wallet *InMemoryWallet) fundTx(tx *wire.MsgTx, amt btcutil.Amount,
	feeRate btcutil.Amount, change bool) error {

	const (
		// spendSize is the largest number of bytes of a sigScript
		// which spends a p2pkh output: OP_DATA_73 <sig> OP_DATA_33 <pubkey>
		spendSize = 1 + 73 + 1 + 33
	)

	var (
		amtSelected btcutil.Amount
		txSize      int
	)

	for outPoint, utxo := range wallet.utxos {
		// Skip any outputs that are still currently immature or are
		// currently locked.
		if !utxo.isMature(wallet.currentHeight) || utxo.isLocked {
			continue
		}

		amtSelected += utxo.value

		// Add the selected output to the transaction, updating the
		// current tx size while accounting for the size of the future
		// sigScript.
		tx.AddTxIn(wire.NewTxIn(&outPoint, nil, nil))
		txSize = tx.SerializeSize() + spendSize*len(tx.TxIn)

		// Calculate the fee required for the txn at this point
		// observing the specified fee rate. If we don't have enough
		// coins from he current amount selected to pay the fee, then
		// continue to grab more coins.
		reqFee := btcutil.Amount(txSize * int(feeRate))
		if amtSelected-reqFee < amt {
			continue
		}

		// If we have any change left over and we should create a change
		// output, then add an additional output to the transaction
		// reserved for it.
		changeVal := amtSelected - amt - reqFee
		if changeVal > 0 && change {
			addr, err := wallet.integrationdress()
			if err != nil {
				return err
			}
			pkScript, err := txscript.PayToAddrScript(addr)
			if err != nil {
				return err
			}
			changeOutput := &wire.TxOut{
				Value:    int64(changeVal),
				PkScript: pkScript,
			}
			tx.AddTxOut(changeOutput)
		}

		return nil
	}

	// If we've reached this point, then coin selection failed due to an
	// insufficient amount of coins.
	return fmt.Errorf("not enough funds for coin selection")
}

// SendOutputs creates, then sends a transaction paying to the specified output
// while observing the passed fee rate. The passed fee rate should be expressed
// in satoshis-per-byte.
func (wallet *InMemoryWallet) SendOutputs(outputs []*wire.TxOut,
	feeRate btcutil.Amount) (*chainhash.Hash, error) {
	args := &harness.CreateTransactionArgs{
		Outputs: outputs,
		FeeRate: feeRate,
		Change:  true,
	}
	tx, err := wallet.CreateTransaction(args)
	if err != nil {
		return nil, err
	}

	return wallet.nodeRPC.SendRawTransaction(tx, true)
}

// SendOutputsWithoutChange creates and sends a transaction that pays to the
// specified outputs while observing the passed fee rate and ignoring a change
// output. The passed fee rate should be expressed in sat/b.
func (wallet *InMemoryWallet) SendOutputsWithoutChange(outputs []*wire.TxOut,
	feeRate btcutil.Amount) (*chainhash.Hash, error) {
	args := &harness.CreateTransactionArgs{
		Outputs: outputs,
		FeeRate: feeRate,
		Change:  false,
	}
	tx, err := wallet.CreateTransaction(args)
	if err != nil {
		return nil, err
	}

	return wallet.nodeRPC.SendRawTransaction(tx, true)
}

// CreateTransaction returns a fully signed transaction paying to the specified
// outputs while observing the desired fee rate. The passed fee rate should be
// expressed in satoshis-per-byte. The transaction being created can optionally
// include a change output indicated by the change boolean.
//
// This function is safe for concurrent access.
func (wallet *InMemoryWallet) CreateTransaction(args *harness.CreateTransactionArgs) (*wire.MsgTx, error) {

	wallet.Lock()
	defer wallet.Unlock()

	tx := wire.NewMsgTx(wire.TxVersion)

	// Tally up the total amount to be sent in order to perform coin
	// selection shortly below.
	var outputAmt btcutil.Amount
	for _, output := range args.Outputs {
		outputAmt += btcutil.Amount(output.Value)
		tx.AddTxOut(output)
	}

	// Attempt to fund the transaction with spendable utxos.
	if err := wallet.fundTx(tx, outputAmt, args.FeeRate, args.Change); err != nil {
		return nil, err
	}

	// Populate all the selected inputs with valid sigScript for spending.
	// Along the way record all outputs being spent in order to avoid a
	// potential double spend.
	spentOutputs := make([]*utxo, 0, len(tx.TxIn))
	for i, txIn := range tx.TxIn {
		outPoint := txIn.PreviousOutPoint
		utxo := wallet.utxos[outPoint]

		extendedKey, err := wallet.hdRoot.Child(utxo.keyIndex)
		if err != nil {
			return nil, err
		}

		privKey, err := extendedKey.ECPrivKey()
		if err != nil {
			return nil, err
		}

		sigScript, err := txscript.SignatureScript(tx, i, utxo.pkScript,
			txscript.SigHashAll, privKey, true)
		if err != nil {
			return nil, err
		}

		txIn.SignatureScript = sigScript

		spentOutputs = append(spentOutputs, utxo)
	}

	// As these outputs are now being spent by this newly created
	// transaction, mark the outputs are "locked". This action ensures
	// these outputs won't be double spent by any subsequent transactions.
	// These locked outputs can be freed via a call to UnlockOutputs.
	for _, utxo := range spentOutputs {
		utxo.isLocked = true
	}

	return tx, nil
}

// UnlockOutputs unlocks any outputs which were previously locked due to
// being selected to fund a transaction via the CreateTransaction method.
//
// This function is safe for concurrent access.
func (wallet *InMemoryWallet) UnlockOutputs(inputs []*wire.TxIn) {
	wallet.Lock()
	defer wallet.Unlock()

	for _, input := range inputs {
		utxo, ok := wallet.utxos[input.PreviousOutPoint]
		if !ok {
			continue
		}

		utxo.isLocked = false
	}
}

// ConfirmedBalance returns the confirmed balance of the wallet.
//
// This function is safe for concurrent access.
func (wallet *InMemoryWallet) ConfirmedBalance() btcutil.Amount {
	wallet.RLock()
	defer wallet.RUnlock()

	var balance btcutil.Amount
	for _, utxo := range wallet.utxos {
		// Prevent any immature or locked outputs from contributing to
		// the wallet's total confirmed balance.
		if !utxo.isMature(wallet.currentHeight) || utxo.isLocked {
			continue
		}

		balance += utxo.value
	}

	return balance
}
