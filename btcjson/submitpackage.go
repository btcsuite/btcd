package btcjson

import (
	"encoding/json"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// JsonSubmitPackageCmd models the request payload for Bitcoin Core’s
// experimental `submitpackage` RPC (v26+). It submits a related group of
// transactions (a package) to the node's mempool for validation and acceptance.
//
// Package Requirements:
//   - Topology: Must be a "child-with-unconfirmed-parents" package: exactly one
//     child transaction (last in the list) and all of its unconfirmed parent
//     transactions. Parents cannot depend on each other within the package.
//   - Order: Transactions MUST be topologically sorted (parents before child).
//   - Content: No duplicate transactions. No conflicting transactions (spending
//     the same input) within the package.
//   - Limits: Subject to node limits on package size (e.g., max 25 txs) and
//     total weight (e.g., max 404000 weight units).
//
// Validation & Acceptance:
//   - Individual First: Each transaction is first validated individually
//     against mempool policy (including minimum relay fee `minrelaytxfee`).
//   - Package Logic: Transactions failing individual checks (often due to low
//     fee rate) are then evaluated using package logic.
//   - Package Feerate: The total fee of non-mempool transactions divided by
//     their total virtual size. This can overcome the dynamic mempool minimum
//     fee rate (acting like CPFP), but cannot overcome the static
//     `minrelaytxfee`. Any transaction below `minrelaytxfee` will cause
//     rejection.
//   - Deduplication: Transactions already in the mempool (by txid) are ignored
//     during submission, preventing rejection and double-counting fees. See
//     the `other-wtxid` field in the result if a different witness version
//     exists.
//   - Package RBF: Limited Replace-By-Fee logic applies. See the
//     `replaced-transactions` field in the result.
//
// This RPC is experimental. Refer to Bitcoin Core's `doc/policy/packages.md`
// for details. Successful submission does not guarantee network propagation.
//
// Reference: https://bitcoincore.org/en/doc/29.0.0/rpc/rawtransactions/submitpackage/
type JsonSubmitPackageCmd struct {
	// RawTxs holds the hex-encoded raw transactions forming the package.
	// MUST be topologically sorted (parents first, child last) and
	// represent a valid "child-with-unconfirmed-parents" structure.
	RawTxs []string `jsonrpc:"package"`

	// MaxFeeRate (Optional, BTC/kvB): If set, rejects package transactions
	// exceeding this fee rate. Rates > 1 BTC/kvB are always rejected. If
	// nil, Core's RPC default (e.g., 0.10 BTC/kvB) applies. Set to 0 for no
	// limit (up to 1 BTC/kvB).
	MaxFeeRate *float64 `jsonrpc:"maxfeerate,omitempty"`

	// MaxBurnAmount (Optional, BTC): If set, rejects packages where the
	// total value of provably unspendable outputs (e.g., OP_RETURN) exceeds
	// this amount. If nil, Core's RPC default (0.00 BTC) applies.
	MaxBurnAmount *float64 `jsonrpc:"maxburnamount,omitempty"`
}

// JsonSubmitPackageFees models the "fees" sub-object in a `submitpackage`
// response. Values are in BTC. May be omitted if fee info is not applicable.
type JsonSubmitPackageFees struct {
	// Base is the absolute fee of this specific transaction (in BTC).
	Base float64 `json:"base"`

	// EffectiveFeeRate (Optional, BTC/kvB): The transaction's effective
	// feerate, potentially considering package context or
	// `prioritisetransaction`.
	EffectiveFeeRate *float64 `json:"effective-feerate,omitempty"`

	// EffectiveIncludes (Optional): wtxids contributing to
	// `effective-feerate`.
	EffectiveIncludes []string `json:"effective-includes,omitempty"`
}

// JsonSubmitPackageTxResult represents the processing result for a single
// transaction within the package, keyed by its wtxid in the response map.
type JsonSubmitPackageTxResult struct {
	// TxID is the transaction hash (txid) in hex.
	TxID string `json:"txid"`

	// OtherWtxid (Optional): Set if a conflicting tx with the same txid but
	// different witness was already in the mempool (submitted tx was
	// ignored). Relates to deduplication.
	OtherWtxid *string `json:"other-wtxid,omitempty"`

	// VSize is the virtual size in vbytes. Note: Optional in RPC; defaults
	// to 0 if missing.
	VSize int64 `json:"vsize"`

	// Fees contains fee information. Note: Optional in RPC; defaults to
	// empty struct if missing.
	Fees JsonSubmitPackageFees `json:"fees"`

	// Error (Optional): String describing rejection reason, if any. Can
	// result from individual checks (e.g., below `minrelaytxfee`) or
	// package validation failures.
	Error *string `json:"error,omitempty"`
}

// JsonSubmitPackageResult mirrors the JSON object returned by `submitpackage`.
type JsonSubmitPackageResult struct {
	// PackageMsg is a summary message ("success" or other status).
	PackageMsg string `json:"package_msg"`

	// TxResults maps each submitted transaction's wtxid to its result.
	TxResults map[string]JsonSubmitPackageTxResult `json:"tx-results"`

	// ReplacedTransactions (Optional): txids of transactions evicted via
	// Package RBF.
	ReplacedTransactions []string `json:"replaced-transactions,omitempty"`
}

// NewJsonSubmitPackageCmd constructs a JsonSubmitPackageCmd.
//
// Parameters:
//   - rawTxs: Slice of hex-encoded txs (topologically sorted
//     child-with-parents).
//   - maxFeeRateBtcKvB: Optional max fee rate (BTC/kvB). Nil uses RPC default.
//   - maxBurnAmountBtc: Optional max burn amount (BTC). Nil uses RPC default.
func NewJsonSubmitPackageCmd(rawTxs []string,
	maxFeeRateBtcKvB, maxBurnAmountBtc *float64) *JsonSubmitPackageCmd {

	return &JsonSubmitPackageCmd{
		RawTxs:        rawTxs,
		MaxFeeRate:    maxFeeRateBtcKvB,
		MaxBurnAmount: maxBurnAmountBtc,
	}
}

// SubmitPackageResult mirrors JsonSubmitPackageResult with higher-level types.
type SubmitPackageResult struct {
	// PackageMsg is a summary message ("success" or other status).
	PackageMsg string

	// TxResults maps each submitted transaction's wtxid to its result.
	TxResults map[string]SubmitPackageTxResult

	// ReplacedTransactions (Optional): txids of transactions evicted via
	// Package RBF.
	ReplacedTransactions []chainhash.Hash
}

// SubmitPackageTxResult mirrors (a subset of) JsonSubmitPackageTxResult with
// higher-level types.
type SubmitPackageTxResult struct {
	// TxID is the transaction hash (txid) in hex.
	TxID chainhash.Hash

	// OtherWtxid (Optional): Set if a conflicting tx with the same txid but
	// different witness was already in the mempool (submitted tx was
	// ignored). Relates to deduplication.
	OtherWtxid *chainhash.Hash

	// Error (Optional): String describing rejection reason, if any. Can
	// result from individual checks (e.g., below `minrelaytxfee`) or
	// package validation failures.
	Error *string
}

// UnmarshalJSON unmarshals the JsonSubmitPackageResult from the JSON response
// to the higher-level SubmitPackageResult type. If the function succeeds, the
// receiver is overwritten with the unmarshalled result.
func (s *SubmitPackageResult) UnmarshalJSON(data []byte) error {
	var src JsonSubmitPackageResult
	if err := json.Unmarshal(data, &src); err != nil {
		return err
	}

	dst := SubmitPackageResult{
		PackageMsg: src.PackageMsg,
		TxResults:  make(map[string]SubmitPackageTxResult),
	}

	// Translate TxResults.
	if len(src.TxResults) > 0 {

		for wtxid, srcTxRes := range src.TxResults {
			var dstTxRes SubmitPackageTxResult

			// Translate TxID.
			txID, err := chainhash.NewHashFromStr(srcTxRes.TxID)
			if err != nil {
				return fmt.Errorf("failed to parse txid '%s' "+
					"for wtxid '%s': %w", srcTxRes.TxID,
					wtxid, err)
			}

			dstTxRes.TxID = *txID

			// Translate Error (direct copy).
			dstTxRes.Error = srcTxRes.Error

			// Translate OtherWtxid.
			if srcTxRes.OtherWtxid != nil &&
				*srcTxRes.OtherWtxid != "" {

				otherWtxidHash, err := chainhash.NewHashFromStr(
					*srcTxRes.OtherWtxid,
				)
				if err != nil {
					return fmt.Errorf("failed to parse "+
						"other_wtxid '%s' for wtxid "+
						"'%s': %w",
						*srcTxRes.OtherWtxid, wtxid,
						err)
				}

				dstTxRes.OtherWtxid = otherWtxidHash
			}

			dst.TxResults[wtxid] = dstTxRes
		}
	}

	// Translate ReplacedTransactions.
	if len(src.ReplacedTransactions) > 0 {
		dst.ReplacedTransactions = make(
			[]chainhash.Hash, 0, len(src.ReplacedTransactions),
		)

		for _, txidStr := range src.ReplacedTransactions {
			hash, err := chainhash.NewHashFromStr(txidStr)
			if err != nil {
				return fmt.Errorf("failed to parse "+
					"replaced_transaction txid '%s': %w",
					txidStr, err)
			}

			dst.ReplacedTransactions = append(
				dst.ReplacedTransactions, *hash,
			)
		}
	}

	// Overwrite the receiver with the translated result.
	*s = dst

	return nil
}
