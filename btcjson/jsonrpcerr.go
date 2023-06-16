// Copyright (c) 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

// Standard JSON-RPC 2.0 errors.
var (
	ErrRPCInvalidRequest = &RPCError{
		Code:    -32600,
		Message: "Invalid request",
	}
	ErrRPCMethodNotFound = &RPCError{
		Code:    -32601,
		Message: "Method not found",
	}
	ErrRPCInvalidParams = &RPCError{
		Code:    -32602,
		Message: "Invalid parameters",
	}
	ErrRPCInternal = &RPCError{
		Code:    -32603,
		Message: "Internal error",
	}
	ErrRPCParse = &RPCError{
		Code:    -32700,
		Message: "Parse error",
	}
)

// General application defined JSON errors.
const (
	// ErrRPCMisc indicates an exception thrown during command handling.
	ErrRPCMisc RPCErrorCode = -1

	// ErrRPCForbiddenBySafeMode indicates that server is in safe mode, and
	// command is not allowed in safe mode.
	ErrRPCForbiddenBySafeMode RPCErrorCode = -2

	// ErrRPCType indicates that an unexpected type was passed as parameter.
	ErrRPCType RPCErrorCode = -3

	// ErrRPCInvalidAddressOrKey indicates an invalid address or key.
	ErrRPCInvalidAddressOrKey RPCErrorCode = -5

	// ErrRPCOutOfMemory indicates that the server ran out of memory during
	// operation.
	ErrRPCOutOfMemory RPCErrorCode = -7

	// ErrRPCInvalidParameter indicates an invalid, missing, or duplicate
	// parameter.
	ErrRPCInvalidParameter RPCErrorCode = -8

	// ErrRPCDatabase indicates a database error.
	ErrRPCDatabase RPCErrorCode = -20

	// ErrRPCDeserialization indicates an error parsing or validating structure
	// in raw format.
	ErrRPCDeserialization RPCErrorCode = -22

	// ErrRPCVerify indicates a general error during transaction or block
	// submission.
	ErrRPCVerify RPCErrorCode = -25

	// ErrRPCVerifyRejected indicates that transaction or block was rejected by
	// network rules.
	ErrRPCVerifyRejected RPCErrorCode = -26

	// ErrRPCVerifyAlreadyInChain indicates that submitted transaction is
	// already in chain.
	ErrRPCVerifyAlreadyInChain RPCErrorCode = -27

	// ErrRPCInWarmup indicates that client is still warming up.
	ErrRPCInWarmup RPCErrorCode = -28

	// ErrRPCInWarmup indicates that the RPC error is deprecated.
	ErrRPCMethodDeprecated RPCErrorCode = -32
)

// Peer-to-peer client errors.
const (
	// ErrRPCClientNotConnected indicates that Bitcoin is not connected.
	ErrRPCClientNotConnected RPCErrorCode = -9

	// ErrRPCClientInInitialDownload indicates that client is still downloading
	// initial blocks.
	ErrRPCClientInInitialDownload RPCErrorCode = -10

	// ErrRPCClientNodeAlreadyAdded indicates that node is already added.
	ErrRPCClientNodeAlreadyAdded RPCErrorCode = -23

	// ErrRPCClientNodeNotAdded indicates that node has not been added before.
	ErrRPCClientNodeNotAdded RPCErrorCode = -24

	// ErrRPCClientNodeNotConnected indicates that node to disconnect was not
	// found in connected nodes.
	ErrRPCClientNodeNotConnected RPCErrorCode = -29

	// ErrRPCClientInvalidIPOrSubnet indicates an invalid IP/Subnet.
	ErrRPCClientInvalidIPOrSubnet RPCErrorCode = -30

	// ErrRPCClientP2PDisabled indicates that no valid connection manager
	// instance was found.
	ErrRPCClientP2PDisabled RPCErrorCode = -31
)

// Chain errors
const (
	// ErrRPCClientMempoolDisabled indicates that no mempool instance was
	// found.
	ErrRPCClientMempoolDisabled RPCErrorCode = -33
)

// Wallet JSON errors
const (
	// ErrRPCWallet indicates an unspecified problem with wallet, for
	// example, key not found, etc.
	ErrRPCWallet RPCErrorCode = -4

	// ErrRPCWalletInvalidAddressType indicates an invalid address type.
	ErrRPCWalletInvalidAddressType RPCErrorCode = -5

	// ErrRPCWalletInsufficientFunds indicates that there are not enough
	// funds in wallet or account.
	ErrRPCWalletInsufficientFunds RPCErrorCode = -6

	// ErrRPCWalletInvalidAccountName indicates an invalid label name.
	ErrRPCWalletInvalidAccountName RPCErrorCode = -11

	// ErrRPCWalletKeypoolRanOut indicates that the keypool ran out, and that
	// keypoolrefill must be called first.
	ErrRPCWalletKeypoolRanOut RPCErrorCode = -12

	// ErrRPCWalletUnlockNeeded indicates that the wallet passphrase must be
	// entered first with the walletpassphrase RPC.
	ErrRPCWalletUnlockNeeded RPCErrorCode = -13

	// ErrRPCWalletPassphraseIncorrect indicates that the wallet passphrase
	// that was entered was incorrect.
	ErrRPCWalletPassphraseIncorrect RPCErrorCode = -14

	// ErrRPCWalletWrongEncState indicates that a command was given in wrong
	// wallet encryption state, for example, encrypting an encrypted wallet.
	ErrRPCWalletWrongEncState RPCErrorCode = -15

	// ErrRPCWalletEncryptionFailed indicates a failure to encrypt the wallet.
	ErrRPCWalletEncryptionFailed RPCErrorCode = -16

	// ErrRPCWalletAlreadyUnlocked indicates an attempt to unlock a wallet
	// that was already unlocked.
	ErrRPCWalletAlreadyUnlocked RPCErrorCode = -17

	// ErrRPCWalletNotFound indicates that an invalid wallet was specified,
	// which does not exist. It can also indicate an attempt to unload a
	// wallet that was not previously loaded.
	//
	// Not to be confused with ErrRPCNoWallet, which is specific to btcd.
	ErrRPCWalletNotFound RPCErrorCode = -18

	// ErrRPCWalletNotSpecified indicates that no wallet was specified, for
	// example, when there are multiple wallets loaded.
	ErrRPCWalletNotSpecified RPCErrorCode = -19
)

// Specific Errors related to commands.  These are the ones a user of the RPC
// server are most likely to see.  Generally, the codes should match one of the
// more general errors above.
const (
	ErrRPCBlockNotFound     RPCErrorCode = -5
	ErrRPCBlockCount        RPCErrorCode = -5
	ErrRPCBestBlockHash     RPCErrorCode = -5
	ErrRPCDifficulty        RPCErrorCode = -5
	ErrRPCOutOfRange        RPCErrorCode = -1
	ErrRPCNoTxInfo          RPCErrorCode = -5
	ErrRPCNoCFIndex         RPCErrorCode = -5
	ErrRPCNoNewestBlockInfo RPCErrorCode = -5
	ErrRPCInvalidTxVout     RPCErrorCode = -5
	ErrRPCRawTxString       RPCErrorCode = -32602
	ErrRPCDecodeHexString   RPCErrorCode = -22
	ErrRPCTxError           RPCErrorCode = -25
	ErrRPCTxRejected        RPCErrorCode = -26
	ErrRPCTxAlreadyInChain  RPCErrorCode = -27
)

// Errors that are specific to btcd.
const (
	ErrRPCNoWallet      RPCErrorCode = -1
	ErrRPCUnimplemented RPCErrorCode = -1
)
