// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package harness

import (
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// TestWallet wraps optional test wallet implementations for different test setups
type TestWallet interface {
	// Network returns current network of the wallet
	Network() *chaincfg.Params

	// NewAddress returns a fresh address spendable by the wallet.
	NewAddress(args *NewAddressArgs) (btcutil.Address, error)

	// Start wallet process
	Start(args *TestWalletStartArgs) error

	// Stops wallet process gently
	Stop()

	// Dispose releases all resources allocated by the wallet
	// This action is final (irreversible)
	Dispose() error

	// Sync block until the wallet has fully synced up to the tip of the main
	// chain.
	Sync()

	// ConfirmedBalance returns wallet balance
	ConfirmedBalance() btcutil.Amount

	// CreateTransaction returns a fully signed transaction paying to the specified
	// outputs while observing the desired fee rate. The passed fee rate should be
	// expressed in satoshis-per-byte. The transaction being created can optionally
	// include a change output indicated by the Change boolean.
	CreateTransaction(args *CreateTransactionArgs) (*wire.MsgTx, error)

	// SendOutputs creates, then sends a transaction paying to the specified output
	// while observing the passed fee rate. The passed fee rate should be expressed
	// in satoshis-per-byte.
	SendOutputs(outputs []*wire.TxOut, feeRate btcutil.Amount) (*chainhash.Hash, error)

	// UnlockOutputs unlocks any outputs which were previously locked due to
	// being selected to fund a transaction via the CreateTransaction method.
	UnlockOutputs(inputs []*wire.TxIn)
}

// TestWalletFactory produces a new TestWallet instance
type TestWalletFactory interface {
	// NewWallet is used by harness builder to setup a wallet component
	NewWallet(cfg *TestWalletConfig) TestWallet
}

// TestWalletConfig bundles settings required to create a new wallet instance
type TestWalletConfig struct {
	Seed          [chainhash.HashSize + 4]byte
	WalletRPCHost string
	WalletRPCPort int
	ActiveNet     *chaincfg.Params
}

// CreateTransactionArgs bundles CreateTransaction() arguments to minimize diff
// in case a new argument for the function is added
type CreateTransactionArgs struct {
	Outputs []*wire.TxOut
	FeeRate btcutil.Amount
	Change  bool
}

// NewAddressArgs bundles NewAddress() arguments to minimize diff
// in case a new argument for the function is added
type NewAddressArgs struct {
	Account string
}

// TestWalletStartArgs bundles Start() arguments to minimize diff
// in case a new argument for the function is added
type TestWalletStartArgs struct {
	NodeRPCCertFile          string
	WalletExtraArguments     map[string]interface{}
	DebugWalletOutput        bool
	MaxSecondsToWaitOnLaunch int
	NodeRPCConfig            *rpcclient.ConnConfig
}
