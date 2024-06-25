package rpcclient

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrBackendVersion is returned when running against a bitcoind or
	// btcd that is older than the minimum version supported by the
	// rpcclient.
	ErrBackendVersion = errors.New("backend version too low")

	// ErrInvalidParam is returned when the caller provides an invalid
	// parameter to an RPC method.
	ErrInvalidParam = errors.New("invalid param")

	// ErrUndefined is used when an error returned is not recognized. We
	// should gradually increase our error types to avoid returning this
	// error.
	ErrUndefined = errors.New("undefined")
)

// BitcoindRPCErr represents an error returned by bitcoind's RPC server.
type BitcoindRPCErr uint32

// This section defines all possible errors or reject reasons returned from
// bitcoind's `sendrawtransaction` or `testmempoolaccept` RPC.
const (
	// ErrMissingInputsOrSpent is returned when calling
	// `sendrawtransaction` with missing inputs.
	ErrMissingInputsOrSpent BitcoindRPCErr = iota

	// ErrMaxBurnExceeded is returned when calling `sendrawtransaction`
	// with exceeding, falling short of, and equaling maxburnamount.
	ErrMaxBurnExceeded

	// ErrMaxFeeExceeded can happen when passing a signed tx to
	// `testmempoolaccept`, but the tx pays more fees than specified.
	ErrMaxFeeExceeded

	// ErrTxAlreadyKnown is used in the `reject-reason` field of
	// `testmempoolaccept` when a transaction is already in the blockchain.
	ErrTxAlreadyKnown

	// ErrTxAlreadyConfirmed is returned as an error from
	// `sendrawtransaction` when a transaction is already in the
	// blockchain.
	ErrTxAlreadyConfirmed

	// ErrMempoolConflict happens when RBF is not enabled yet the
	// transaction conflicts with an unconfirmed tx.  .
	//
	// NOTE: RBF rule 1.
	ErrMempoolConflict

	// ErrReplacementAddsUnconfirmed is returned when a transaction adds
	// new unconfirmed inputs.
	//
	// NOTE: RBF rule 2.
	ErrReplacementAddsUnconfirmed

	// ErrInsufficientFee is returned when fee rate used or fees paid
	// doesn't meet the requirements.
	//
	// NOTE: RBF rule 3 or 4.
	ErrInsufficientFee

	// ErrTooManyReplacements is returned when a transaction causes too
	// many transactions being replaced. This is set by
	// `MAX_REPLACEMENT_CANDIDATES` in `bitcoind` and defaults to 100.
	//
	// NOTE: RBF rule 5.
	ErrTooManyReplacements

	// ErrMempoolMinFeeNotMet is returned when the transaction doesn't meet
	// the minimum relay fee.
	ErrMempoolMinFeeNotMet

	// ErrConflictingTx is returned when a transaction that spends
	// conflicting tx outputs that are rejected.
	ErrConflictingTx

	// ErrEmptyOutput is returned when a transaction has no outputs.
	ErrEmptyOutput

	// ErrEmptyInput is returned when a transaction has no inputs.
	ErrEmptyInput

	// ErrTxTooSmall is returned when spending a tiny transaction(in
	// non-witness bytes) that is disallowed.
	//
	// NOTE: ErrTxTooLarge must be put after ErrTxTooSmall because it's a
	// subset of ErrTxTooSmall. Otherwise, if bitcoind returns
	// `tx-size-small`, it will be matched to ErrTxTooLarge.
	ErrTxTooSmall

	// ErrDuplicateInput is returned when a transaction has duplicate
	// inputs.
	ErrDuplicateInput

	// ErrEmptyPrevOut is returned when a non-coinbase transaction has
	// coinbase-like outpoint.
	ErrEmptyPrevOut

	// ErrBelowOutValue is returned when a transaction's output value is
	// greater than its input value.
	ErrBelowOutValue

	// ErrNegativeOutput is returned when a transaction has negative output
	// value.
	ErrNegativeOutput

	// ErrLargeOutput is returned when a transaction has too large output
	// value.
	ErrLargeOutput

	// ErrLargeTotalOutput is returned when a transaction has too large sum
	// of output values.
	ErrLargeTotalOutput

	// ErrScriptVerifyFlag is returned when there is invalid OP_IF
	// construction.
	ErrScriptVerifyFlag

	// ErrTooManySigOps is returned when a transaction has too many sigops.
	ErrTooManySigOps

	// ErrInvalidOpcode is returned when a transaction has invalid OP
	// codes.
	ErrInvalidOpcode

	// ErrTxAlreadyInMempool is returned when a transaction is in the
	// mempool.
	ErrTxAlreadyInMempool

	// ErrMissingInputs is returned when a transaction has missing inputs,
	// that never existed or only existed once in the past.
	ErrMissingInputs

	// ErrOversizeTx is returned when a transaction is too large.
	ErrOversizeTx

	// ErrCoinbaseTx is returned when the transaction is coinbase tx.
	ErrCoinbaseTx

	// ErrNonStandardVersion is returned when the transactions are not
	// standard - a version currently non-standard.
	ErrNonStandardVersion

	// ErrNonStandardScript is returned when the transactions are not
	// standard - non-standard script.
	ErrNonStandardScript

	// ErrBareMultiSig is returned when the transactions are not standard -
	// bare multisig script (2-of-3).
	ErrBareMultiSig

	// ErrScriptSigNotPushOnly is returned when the transactions are not
	// standard - not-pushonly scriptSig.
	ErrScriptSigNotPushOnly

	// ErrScriptSigSize is returned when the transactions are not standard
	// - too large scriptSig (>1650 bytes).
	ErrScriptSigSize

	// ErrTxTooLarge is returned when the transactions are not standard -
	// too large tx size.
	ErrTxTooLarge

	// ErrDust is returned when the transactions are not standard - output
	// too small.
	ErrDust

	// ErrMultiOpReturn is returned when the transactions are not standard
	// - muiltiple OP_RETURNs.
	ErrMultiOpReturn

	// ErrNonFinal is returned when spending a timelocked transaction that
	// hasn't expired yet.
	ErrNonFinal

	// ErrNonBIP68Final is returned when a transaction that is locked by
	// BIP68 sequence logic and not expired yet.
	ErrNonBIP68Final

	// ErrSameNonWitnessData is returned when another tx with the same
	// non-witness data is already in the mempool. For instance, these two
	// txns share the same `txid` but different `wtxid`.
	ErrSameNonWitnessData

	// ErrNonMandatoryScriptVerifyFlag is returned when passing a raw tx to
	// `testmempoolaccept`, which gives the error followed by (Witness
	// program hash mismatch).
	ErrNonMandatoryScriptVerifyFlag

	// errSentinel is used to indicate the end of the error list. This
	// should always be the last error code.
	errSentinel
)

// Error implements the error interface. It returns the error message defined
// in `bitcoind`.

// Some of the dashes used in the original error string is removed, e.g.
// "missing-inputs" is now "missing inputs". This is ok since we will normalize
// the errors before matching.
//
// references:
// - https://github.com/bitcoin/bitcoin/blob/master/test/functional/rpc_rawtransaction.py#L342
// - https://github.com/bitcoin/bitcoin/blob/master/test/functional/data/invalid_txs.py
// - https://github.com/bitcoin/bitcoin/blob/master/test/functional/mempool_accept.py
// - https://github.com/bitcoin/bitcoin/blob/master/test/functional/mempool_accept_wtxid.py
// - https://github.com/bitcoin/bitcoin/blob/master/test/functional/mempool_dust.py
// - https://github.com/bitcoin/bitcoin/blob/master/test/functional/mempool_limit.py
// - https://github.com/bitcoin/bitcoin/blob/master/src/validation.cpp
func (r BitcoindRPCErr) Error() string {
	switch r {
	case ErrMissingInputsOrSpent:
		return "bad-txns-inputs-missingorspent"

	case ErrMaxBurnExceeded:
		return "Unspendable output exceeds maximum configured by user (maxburnamount)"

	case ErrMaxFeeExceeded:
		return "max-fee-exceeded"

	case ErrTxAlreadyKnown:
		return "txn-already-known"

	case ErrTxAlreadyConfirmed:
		return "Transaction already in block chain"

	case ErrMempoolConflict:
		return "txn mempool conflict"

	case ErrReplacementAddsUnconfirmed:
		return "replacement adds unconfirmed"

	case ErrInsufficientFee:
		return "insufficient fee"

	case ErrTooManyReplacements:
		return "too many potential replacements"

	case ErrMempoolMinFeeNotMet:
		return "mempool min fee not met"

	case ErrConflictingTx:
		return "bad txns spends conflicting tx"

	case ErrEmptyOutput:
		return "bad txns vout empty"

	case ErrEmptyInput:
		return "bad txns vin empty"

	case ErrTxTooSmall:
		return "tx size small"

	case ErrDuplicateInput:
		return "bad txns inputs duplicate"

	case ErrEmptyPrevOut:
		return "bad txns prevout null"

	case ErrBelowOutValue:
		return "bad txns in belowout"

	case ErrNegativeOutput:
		return "bad txns vout negative"

	case ErrLargeOutput:
		return "bad txns vout toolarge"

	case ErrLargeTotalOutput:
		return "bad txns txouttotal toolarge"

	case ErrScriptVerifyFlag:
		return "mandatory script verify flag failed"

	case ErrTooManySigOps:
		return "bad txns too many sigops"

	case ErrInvalidOpcode:
		return "disabled opcode"

	case ErrTxAlreadyInMempool:
		return "txn already in mempool"

	case ErrMissingInputs:
		return "missing inputs"

	case ErrOversizeTx:
		return "bad txns oversize"

	case ErrCoinbaseTx:
		return "coinbase"

	case ErrNonStandardVersion:
		return "version"

	case ErrNonStandardScript:
		return "scriptpubkey"

	case ErrBareMultiSig:
		return "bare multisig"

	case ErrScriptSigNotPushOnly:
		return "scriptsig not pushonly"

	case ErrScriptSigSize:
		return "scriptsig size"

	case ErrTxTooLarge:
		return "tx size"

	case ErrDust:
		return "dust"

	case ErrMultiOpReturn:
		return "multi op return"

	case ErrNonFinal:
		return "non final"

	case ErrNonBIP68Final:
		return "non BIP68 final"

	case ErrSameNonWitnessData:
		return "txn-same-nonwitness-data-in-mempool"

	case ErrNonMandatoryScriptVerifyFlag:
		return "non-mandatory-script-verify-flag"
	}

	return "unknown error"
}

// BtcdErrMap takes the errors returned from btcd's `testmempoolaccept` and
// `sendrawtransaction` RPCs and map them to the errors defined above, which
// are results from calling either `testmempoolaccept` or `sendrawtransaction`
// in `bitcoind`.
//
// Errors not mapped in `btcd`:
//   - deployment error from `validateSegWitDeployment`.
//   - the error when total inputs is higher than max allowed value from
//     `CheckTransactionInputs`.
//   - the error when total outputs is higher than total inputs from
//     `CheckTransactionInputs`.
//   - errors from `CalcSequenceLock`.
//
// NOTE: This is not an exhaustive list of errors, but it covers the
// usage case of LND.
//
//nolint:lll
var BtcdErrMap = map[string]error{
	// BIP125 related errors.
	//
	// When fee rate used or fees paid doesn't meet the requirements.
	"replacement transaction has an insufficient fee rate":     ErrInsufficientFee,
	"replacement transaction has an insufficient absolute fee": ErrInsufficientFee,

	// When a transaction causes too many transactions being replaced. This
	// is set by `MAX_REPLACEMENT_CANDIDATES` in `bitcoind` and defaults to
	// 100.
	"replacement transaction evicts more transactions than permitted": ErrTooManyReplacements,

	// When a transaction adds new unconfirmed inputs.
	"replacement transaction spends new unconfirmed input": ErrReplacementAddsUnconfirmed,

	// A transaction that spends conflicting tx outputs that are rejected.
	"replacement transaction spends parent transaction": ErrConflictingTx,

	// A transaction that conflicts with an unconfirmed tx. Happens when
	// RBF is not enabled.
	"output already spent in mempool": ErrMempoolConflict,

	// A transaction with no outputs.
	"transaction has no outputs": ErrEmptyOutput,

	// A transaction with no inputs.
	"transaction has no inputs": ErrEmptyInput,

	// A transaction with duplicate inputs.
	"transaction contains duplicate inputs": ErrDuplicateInput,

	// A non-coinbase transaction with coinbase-like outpoint.
	"transaction input refers to previous output that is null": ErrEmptyPrevOut,

	// A transaction pays too little fee.
	"fees which is under the required amount":               ErrMempoolMinFeeNotMet,
	"has insufficient priority":                             ErrInsufficientFee,
	"has been rejected by the rate limiter due to low fees": ErrInsufficientFee,

	// A transaction with negative output value.
	"transaction output has negative value": ErrNegativeOutput,

	// A transaction with too large output value.
	"transaction output value is higher than max allowed value": ErrLargeOutput,

	// A transaction with too large sum of output values.
	"total value of all transaction outputs exceeds max allowed value": ErrLargeTotalOutput,

	// A transaction with too many sigops.
	"sigop cost is too hight": ErrTooManySigOps,

	// A transaction already in the blockchain.
	"database contains entry for spent tx output": ErrTxAlreadyKnown,
	"transaction already exists in blockchain":    ErrTxAlreadyConfirmed,

	// A transaction in the mempool.
	//
	// NOTE: For btcd v0.24.2 and beyond, the error message is "already
	// have transaction in mempool".
	"already have transaction": ErrTxAlreadyInMempool,

	// A transaction with missing inputs, that never existed or only
	// existed once in the past.
	"either does not exist or has already been spent": ErrMissingInputs,
	"orphan transaction":                              ErrMissingInputs,

	// A really large transaction.
	"serialized transaction is too big": ErrOversizeTx,

	// A coinbase transaction.
	"transaction is an invalid coinbase": ErrCoinbaseTx,

	// Some nonstandard transactions - a version currently non-standard.
	"transaction version": ErrNonStandardVersion,

	// Some nonstandard transactions - non-standard script.
	"non-standard script form": ErrNonStandardScript,
	"has a non-standard input": ErrNonStandardScript,

	// Some nonstandard transactions - bare multisig script
	// (2-of-3).
	"milti-signature script": ErrBareMultiSig,

	// Some nonstandard transactions - not-pushonly scriptSig.
	"signature script is not push only": ErrScriptSigNotPushOnly,

	// Some nonstandard transactions - too large scriptSig (>1650
	// bytes).
	"signature script size is larger than max allowed": ErrScriptSigSize,

	// Some nonstandard transactions - too large tx size.
	"weight of transaction is larger than max allowed": ErrTxTooLarge,

	// Some nonstandard transactions - output too small.
	"payment is dust": ErrDust,

	// Some nonstandard transactions - muiltiple OP_RETURNs.
	"more than one transaction output in a nulldata script": ErrMultiOpReturn,

	// A timelocked transaction.
	"transaction is not finalized":               ErrNonFinal,
	"tried to spend coinbase transaction output": ErrNonFinal,

	// A transaction that is locked by BIP68 sequence logic.
	"transaction's sequence locks on inputs not met": ErrNonBIP68Final,

	// TODO(yy): find/return the following errors in `btcd`.
	//
	// A tiny transaction(in non-witness bytes) that is disallowed.
	// "unmatched btcd error 1": ErrTxTooSmall,
	// "unmatched btcd error 2": ErrScriptVerifyFlag,
	// // A transaction with invalid OP codes.
	// "unmatched btcd error 3": ErrInvalidOpcode,
	// // Minimally-small transaction(in non-witness bytes) that is
	// // allowed.
	// "unmatched btcd error 4": ErrSameNonWitnessData,
}

// MapRPCErr takes an error returned from calling RPC methods from various
// chain backend and map it to an defined error here. It uses the `BtcdErrMap`
// defined above, whose keys are btcd error strings and values are errors made
// from bitcoind error strings.
//
// NOTE: we assume neutrino shares the same error strings as btcd.
func MapRPCErr(rpcErr error) error {
	// Iterate the map and find the matching error.
	for btcdErr, err := range BtcdErrMap {
		// Match it against btcd's error first.
		if matchErrStr(rpcErr, btcdErr) {
			return err
		}
	}

	// If not found, try to match it against bitcoind's error.
	for i := uint32(0); i < uint32(errSentinel); i++ {
		err := BitcoindRPCErr(i)
		if matchErrStr(rpcErr, err.Error()) {
			return err
		}
	}

	// If not matched, return the original error wrapped.
	return fmt.Errorf("%w: %v", ErrUndefined, rpcErr)
}

// matchErrStr takes an error returned from RPC client and matches it against
// the specified string. If the expected string pattern is found in the error
// passed, return true. Both the error strings are normalized before matching.
func matchErrStr(err error, s string) bool {
	// Replace all dashes found in the error string with spaces.
	strippedErrStr := strings.ReplaceAll(err.Error(), "-", " ")

	// Replace all dashes found in the error string with spaces.
	strippedMatchStr := strings.ReplaceAll(s, "-", " ")

	// Match against the lowercase.
	return strings.Contains(
		strings.ToLower(strippedErrStr),
		strings.ToLower(strippedMatchStr),
	)
}
