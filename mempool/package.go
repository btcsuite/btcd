// Copyright (c) 2013-2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// PackageValidationOptions contains options for package validation.
type PackageValidationOptions struct {
	// MaxFeeRate is the maximum fee rate allowed for transactions in the
	// package (satoshis per kilovbyte). If nil, no limit is enforced beyond
	// the absolute 1 BTC/kvB hard limit. If set to 0, no limit is enforced.
	MaxFeeRate *btcutil.Amount

	// MaxBurnAmount is the maximum total value of provably unspendable
	// outputs (OP_RETURN, etc.) allowed in the package. If nil, defaults
	// to 0 (no burns allowed).
	MaxBurnAmount *btcutil.Amount
}

// TxAcceptResult holds the result of attempting to accept a single transaction
// within a package.
type TxAcceptResult struct {
	// TxHash is the transaction hash (txid).
	TxHash chainhash.Hash

	// Wtxid is the witness transaction ID.
	Wtxid chainhash.Hash

	// VSize is the virtual size in vbytes.
	VSize int64

	// Fee is the transaction fee in satoshis.
	Fee btcutil.Amount

	// EffectiveFeeRate is the effective fee rate considering package context
	// (satoshis per kilovbyte). May differ from individual fee rate for
	// transactions in packages.
	EffectiveFeeRate int64

	// Accepted indicates whether the transaction was accepted into the mempool.
	Accepted bool

	// Error contains the rejection reason if the transaction was not accepted.
	// Nil if Accepted is true.
	Error error

	// OtherWtxid is set if a transaction with the same txid but different
	// witness data already exists in the mempool (deduplication case).
	OtherWtxid *chainhash.Hash

	// AlreadyInMempool indicates this transaction was already in the mempool
	// and was skipped (deduplication).
	AlreadyInMempool bool
}

// PackageAcceptResult holds the result of attempting to accept a package of
// transactions to the mempool.
type PackageAcceptResult struct {
	// PackageMsg is a summary message about the package acceptance.
	// "success" if all transactions were processed successfully, otherwise
	// an error description.
	PackageMsg string

	// TxResults maps each transaction's wtxid to its individual result.
	// Includes both successful and failed transactions.
	TxResults map[chainhash.Hash]*TxAcceptResult

	// ReplacedTxs is a list of transaction hashes that were replaced via
	// package RBF. Empty if no replacements occurred.
	ReplacedTxs []chainhash.Hash

	// TotalFees is the sum of fees from all transactions in the package.
	TotalFees btcutil.Amount

	// TotalVSize is the sum of virtual sizes from all transactions in the
	// package.
	TotalVSize int64

	// PackageFeeRate is the aggregate fee rate for the package (satoshis
	// per kilovbyte), calculated as TotalFees / TotalVSize * 1000.
	PackageFeeRate int64

	// AcceptedCount is the number of transactions successfully accepted.
	AcceptedCount int

	// RejectedCount is the number of transactions rejected.
	RejectedCount int
}

const (
	// MaxPackageCount is the maximum number of transactions allowed in a
	// single package. This matches Bitcoin Core's limit.
	MaxPackageCount = 25

	// MaxPackageWeight is the maximum total weight allowed for a package
	// in weight units. This matches Bitcoin Core's 404000 weight unit limit
	// (101000 virtual bytes).
	MaxPackageWeight = 404000
)

// PackageContext provides context about whether a transaction is being
// validated as part of a package. This is used to implement BIP 431 Rule 6,
// which allows TRUC (v3) transactions to have fees below the minimum relay
// fee when they're part of a valid package.
type PackageContext struct {
	// IsPartOfPackage indicates whether the transaction is being validated
	// as part of a package submission (vs. individual submission).
	IsPartOfPackage bool

	// IsTRUCPackage indicates whether this is a TRUC (v3) package that
	// should be allowed to have individual transactions below min relay fee.
	IsTRUCPackage bool

	// PackageFeeRate is the aggregate fee rate for the entire package in
	// satoshis per kilovbyte. Only set when IsPartOfPackage is true.
	PackageFeeRate int64

	// PackageSize is the total virtual size of all transactions in the package.
	PackageSize int64
}
