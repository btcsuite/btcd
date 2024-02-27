package rpcclient

import (
	"errors"
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

	// RejectReasonMap takes the error returned from
	// `CheckMempoolAcceptance` in `btcd` and maps it to the reject reason
	// that's returned from calling `testmempoolaccept` in `bitcoind`.
	// references:
	// - https://github.com/bitcoin/bitcoin/blob/master/test/functional/data/invalid_txs.py
	// - https://github.com/bitcoin/bitcoin/blob/master/test/functional/mempool_accept.py
	// - https://github.com/bitcoin/bitcoin/blob/master/src/validation.cpp
	//
	// Errors not mapped in `btcd`:
	// - deployment error from `validateSegWitDeployment`.
	// - the error when total inputs is higher than max allowed value from
	//   `CheckTransactionInputs`.
	// - the error when total outputs is higher than total inputs from
	//   `CheckTransactionInputs`.
	// - errors from `CalcSequenceLock`.
	//
	// NOTE: This is not an exhaustive list of errors, but it covers the
	// usage case of LND.
	//
	//nolint:lll
	RejectReasonMap = map[string]string{
		// BIP125 related errors.
		//
		// When fee rate used or fees paid doesn't meet the
		// requirements.
		"replacement transaction has an insufficient fee rate":     "insufficient fee",
		"replacement transaction has an insufficient absolute fee": "insufficient fee",

		// When a transaction causes too many transactions being
		// replaced. This is set by `MAX_REPLACEMENT_CANDIDATES` in
		// `bitcoind` and defaults to 100.
		"replacement transaction evicts more transactions than permitted": "too many potential replacements",

		// When a transaction adds new unconfirmed inputs.
		"replacement transaction spends new unconfirmed input": "replacement-adds-unconfirmed",

		// A transaction that spends conflicting tx outputs that are
		// rejected.
		"replacement transaction spends parent transaction": "bad-txns-spends-conflicting-tx",

		// A transaction that conflicts with an unconfirmed tx. Happens
		// when RBF is not enabled.
		"output already spent in mempool": "txn-mempool-conflict",

		// A transaction with no outputs.
		"transaction has no outputs": "bad-txns-vout-empty",

		// A transaction with no inputs.
		"transaction has no inputs": "bad-txns-vin-empty",

		// A tiny transaction(in non-witness bytes) that is disallowed.
		// TODO(yy): find/return this error in `btcd`.
		// "": "tx-size-small",

		// A transaction with duplicate inputs.
		"transaction contains duplicate inputs": "bad-txns-inputs-duplicate",

		// A non-coinbase transaction with coinbase-like outpoint.
		"transaction input refers to previous output that is null": "bad-txns-prevout-null",

		// A transaction pays too little fee.
		"fees which is under the required amount":               "bad-txns-in-belowout",
		"has insufficient priority":                             "bad-txns-in-belowout",
		"has been rejected by the rate limiter due to low fees": "bad-txns-in-belowout",

		// A transaction with negative output value.
		"transaction output has negative value": "bad-txns-vout-negative",

		// A transaction with too large output value.
		"transaction output value is higher than max allowed value": "bad-txns-vout-toolarge",

		// A transaction with too large sum of output values.
		"total value of all transaction outputs exceeds max allowed value": "bad-txns-txouttotal-toolarge",

		// TODO(yy): find/return this error in `btcd`.
		// "": "mandatory-script-verify-flag-failed (Invalid OP_IF construction)",

		// A transaction with too many sigops.
		"sigop cost is too hight": "bad-txns-too-many-sigops",

		// A transaction with invalid OP codes.
		// TODO(yy): find/return this error in `btcd`.
		// "": "disabled opcode",

		// A transaction already in the blockchain.
		"database contains entry for spent tx output": "txn-already-known",
		"transaction already exists in blockchain":    "txn-already-known",

		// A transaction in the mempool.
		"already have transaction in mempool": "txn-already-in-mempool",

		// A transaction with missing inputs, that never existed or
		// only existed once in the past.
		"either does not exist or has already been spent": "missing-inputs",

		// A really large transaction.
		"serialized transaction is too big": "bad-txns-oversize",

		// A coinbase transaction.
		"transaction is an invalid coinbase": "coinbase",

		// Some nonstandard transactions - a version currently
		// non-standard.
		"transaction version": "version",

		// Some nonstandard transactions - non-standard script.
		"non-standard script form": "scriptpubkey",
		"has a non-standard input": "scriptpubkey",

		// Some nonstandard transactions - bare multisig script
		// (2-of-3).
		"milti-signature script": "bare-multisig",

		// Some nonstandard transactions - not-pushonly scriptSig.
		"signature script is not push only": "scriptsig-not-pushonly",

		// Some nonstandard transactions - too large scriptSig (>1650
		// bytes).
		"signature script size is larger than max allowed": "scriptsig-size",

		// Some nonstandard transactions - too large tx size.
		"weight of transaction is larger than max allowed": "tx-size",

		// Some nonstandard transactions - output too small.
		"payment is dust": "dust",

		// Some nonstandard transactions - muiltiple OP_RETURNs.
		"more than one transaction output in a nulldata script": "multi-op-return",

		// A timelocked transaction.
		"transaction is not finalized":               "non-final",
		"tried to spend coinbase transaction output": "non-final",

		// A transaction that is locked by BIP68 sequence logic.
		"transaction's sequence locks on inputs not met": "non-BIP68-final",

		// Minimally-small transaction(in non-witness bytes) that is
		// allowed.
		// TODO(yy): find/return this error in `btcd`.
		// "": "txn-same-nonwitness-data-in-mempools",
	}
)

// MapBtcdErrToRejectReason takes an error returned from
// `CheckMempoolAcceptance` and maps the error to a bitcoind reject reason.
func MapBtcdErrToRejectReason(err error) string {
	// Get the error string and turn it into lower case.
	btcErr := strings.ToLower(err.Error())

	// Iterate the map and find the matching error.
	for keyErr, rejectReason := range RejectReasonMap {
		// Match the substring.
		if strings.Contains(btcErr, keyErr) {
			return rejectReason
		}
	}

	// If there's no match, return the error string directly.
	return btcErr
}
