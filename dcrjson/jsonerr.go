// Copyright (c) 2013-2014 Conformal Systems LLC.
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson

// Standard JSON-RPC 2.0 errors
var (
	ErrInvalidRequest = Error{
		Code:    -32600,
		Message: "Invalid request",
	}
	ErrMethodNotFound = Error{
		Code:    -32601,
		Message: "Method not found",
	}
	ErrInvalidParams = Error{
		Code:    -32602,
		Message: "Invalid parameters",
	}
	ErrInternal = Error{
		Code:    -32603,
		Message: "Internal error",
	}
	ErrParse = Error{
		Code:    -32700,
		Message: "Parse error",
	}
)

// General application defined JSON errors
var (
	ErrMisc = Error{
		Code:    -1,
		Message: "Miscellaneous error",
	}
	ErrForbiddenBySafeMode = Error{
		Code:    -2,
		Message: "Server is in safe mode, and command is not allowed in safe mode",
	}
	ErrType = Error{
		Code:    -3,
		Message: "Unexpected type was passed as parameter",
	}
	ErrInvalidAddressOrKey = Error{
		Code:    -5,
		Message: "Invalid address or key",
	}
	ErrOutOfMemory = Error{
		Code:    -7,
		Message: "Ran out of memory during operation",
	}
	ErrInvalidParameter = Error{
		Code:    -8,
		Message: "Invalid, missing or duplicate parameter",
	}
	ErrDatabase = Error{
		Code:    -20,
		Message: "Database error",
	}
	ErrDeserialization = Error{
		Code:    -22,
		Message: "Error parsing or validating structure in raw format",
	}
)

// Peer-to-peer client errors
var (
	ErrClientNotConnected = Error{
		Code:    -9,
		Message: "dcrd is not connected",
	}
	ErrClientInInitialDownload = Error{
		Code:    -10,
		Message: "dcrd is downloading blocks...",
	}
)

// Wallet JSON errors
var (
	ErrWallet = Error{
		Code:    -4,
		Message: "Unspecified problem with wallet",
	}
	ErrWalletInsufficientFunds = Error{
		Code:    -6,
		Message: "Not enough funds in wallet or account",
	}
	ErrWalletInvalidAccountName = Error{
		Code:    -11,
		Message: "Invalid account name",
	}
	ErrWalletKeypoolRanOut = Error{
		Code:    -12,
		Message: "Keypool ran out, call keypoolrefill first",
	}
	ErrWalletUnlockNeeded = Error{
		Code:    -13,
		Message: "Enter the wallet passphrase with walletpassphrase first",
	}
	ErrWalletPassphraseIncorrect = Error{
		Code:    -14,
		Message: "The wallet passphrase entered was incorrect",
	}
	ErrWalletWrongEncState = Error{
		Code:    -15,
		Message: "Command given in wrong wallet encryption state",
	}
	ErrWalletEncryptionFailed = Error{
		Code:    -16,
		Message: "Failed to encrypt the wallet",
	}
	ErrWalletAlreadyUnlocked = Error{
		Code:    -17,
		Message: "Wallet is already unlocked",
	}
)

// Specific Errors related to commands.  These are the ones a user of the rpc
// server are most likely to see.  Generally, the codes should match one of the
// more general errors above.
var (
	ErrBlockNotFound = Error{
		Code:    -5,
		Message: "Block not found",
	}
	ErrBlockCount = Error{
		Code:    -5,
		Message: "Error getting block count",
	}
	ErrBestBlockHash = Error{
		Code:    -5,
		Message: "Error getting best block hash",
	}
	ErrDifficulty = Error{
		Code:    -5,
		Message: "Error getting difficulty",
	}
	ErrOutOfRange = Error{
		Code:    -1,
		Message: "Block number out of range",
	}
	ErrNoTxInfo = Error{
		Code:    -5,
		Message: "No information available about transaction",
	}
	ErrNoNewestBlockInfo = Error{
		Code:    -5,
		Message: "No information about newest block",
	}
	ErrInvalidTxVout = Error{
		Code:    -5,
		Message: "Output index number (vout) does not exist for transaction.",
	}
	ErrRawTxString = Error{
		Code:    -32602,
		Message: "Raw tx is not a string",
	}
	ErrDecodeHexString = Error{
		Code:    -22,
		Message: "Unable to decode hex string",
	}
)

// Errors that are specific to dcrd.
var (
	ErrNoWallet = Error{
		Code:    -1,
		Message: "This implementation does not implement wallet commands",
	}
	ErrUnimplemented = Error{
		Code:    -1,
		Message: "Command unimplemented",
	}
)
