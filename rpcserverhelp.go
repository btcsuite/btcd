// Copyright (c) 2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/decred/dcrd/dcrjson"
)

// helpDescsEnUS defines the English descriptions used for the help strings.
var helpDescsEnUS = map[string]string{
	// DebugLevelCmd help.
	"debuglevel--synopsis": "Dynamically changes the debug logging level.\n" +
		"The levelspec can either a debug level or of the form:\n" +
		"<subsystem>=<level>,<subsystem2>=<level2>,...\n" +
		"The valid debug levels are trace, debug, info, warn, error, and critical.\n" +
		"The valid subsystems are AMGR, ADXR, BCDB, BMGR, DCRD, CHAN, DISC, PEER, RPCS, SCRP, SRVR, and TXMP.\n" +
		"Finally the keyword 'show' will return a list of the available subsystems.",
	"debuglevel-levelspec":   "The debug level(s) to use or the keyword 'show'",
	"debuglevel--condition0": "levelspec!=show",
	"debuglevel--condition1": "levelspec=show",
	"debuglevel--result0":    "The string 'Done.'",
	"debuglevel--result1":    "The list of subsystems",

	// AddNodeCmd help.
	"addnode--synopsis": "Attempts to add or remove a persistent peer.",
	"addnode-addr":      "IP address and port of the peer to operate on",
	"addnode-subcmd":    "'add' to add a persistent peer, 'remove' to remove a persistent peer, or 'onetry' to try a single connection to a peer",

	// NodeCmd help.
	"node--synopsis":     "Attempts to add or remove a peer.",
	"node-subcmd":        "'disconnect' to remove all matching non-persistent peers, 'remove' to remove a persistent peer, or 'connect' to connect to a peer",
	"node-target":        "Either the IP address and port of the peer to operate on, or a valid peer ID.",
	"node-connectsubcmd": "'perm' to make the connected peer a permanent one, 'temp' to try a single connect to a peer",

	// TransactionInput help.
	"transactioninput-txid": "The hash of the input transaction",
	"transactioninput-vout": "The specific output of the input transaction to redeem",
	"transactioninput-tree": "The tree that the transaction input is located",
	// TODO review cmd help messages for stake stuff
	// CreateRawSSTxCmd help.
	"createrawsstx--synopsis": "Returns a new transaction spending the provided inputs and sending to the provided addresses.\n" +
		"The transaction inputs are not signed in the created transaction.\n" +
		"The signrawtransaction RPC command provided by wallet must be used to sign the resulting transaction.",
	"createrawsstx--result0":      "Hex-encoded bytes of the serialized transaction",
	"createrawsstx-inputs":        "The inputs to the transaction of type sstxinput",
	"sstxinput-txid":              "Unspent tx output hash",
	"sstxinput-vout":              "Amount of utxo",
	"sstxinput-amt":               "Amount of utxu",
	"sstxinput-tree":              "Which tree utxo is located",
	"createrawsstx-amount":        "JSON object with the destination addresses as keys and amounts as values",
	"createrawsstx-amount--key":   "address",
	"createrawsstx-amount--value": "n.nnn",
	"createrawsstx-amount--desc":  "The destination address as the key and the amount in DCR as the value",
	"createrawsstx-couts":         "Array of sstx commit outs to use of type SSTxCommitOut",
	"sstxcommitout-addr":          "Address to send sstx commit",
	"sstxcommitout-commitamt":     "Amount to commit",
	"sstxcommitout-changeamt":     "Amount for change",
	"sstxcommitout-changeaddr":    "Address for change",

	// CreateRawSSGenTxCmd help.
	"createrawssgentx--synopsis": "Returns a new transaction spending the provided inputs and sending to the provided addresses.\n" +
		"The transaction inputs are not signed in the created transaction.\n" +
		"The signrawtransaction RPC command provided by wallet must be used to sign the resulting transaction.",
	"createrawssgentx--result0": "Hex-encoded bytes of the serialized transaction",
	"createrawssgentx-inputs":   "The inputs to the transaction of type sstxinput",
	"createrawssgentx-votebits": "The inputs to the transaction of type sstxinput",

	// CreateRawSSGenTxCmd help.
	"createrawssrtx--synopsis": "Returns a new transaction spending the provided inputs and sending to the provided addresses.\n" +
		"The transaction inputs are not signed in the created transaction.\n" +
		"The signrawtransaction RPC command provided by wallet must be used to sign the resulting transaction.",
	"createrawssrtx--result0": "Hex-encoded bytes of the serialized transaction",
	"createrawssrtx-inputs":   "The inputs to the transaction of type sstxinput",
	"createrawssrtx-fee":      "The fee to apply to the revocation in Coins",

	// CreateRawTransactionCmd help.
	"createrawtransaction--synopsis": "Returns a new transaction spending the provided inputs and sending to the provided addresses.\n" +
		"The transaction inputs are not signed in the created transaction.\n" +
		"The signrawtransaction RPC command provided by wallet must be used to sign the resulting transaction.",
	"createrawtransaction-inputs":         "The inputs to the transaction",
	"createrawtransaction-amounts":        "JSON object with the destination addresses as keys and amounts as values",
	"createrawtransaction-amounts--key":   "address",
	"createrawtransaction-amounts--value": "n.nnn",
	"createrawtransaction-amounts--desc":  "The destination address as the key and the amount in DCR as the value",
	"createrawtransaction-locktime":       "Locktime value; a non-zero value will also locktime-activate the inputs",
	"createrawtransaction--result0":       "Hex-encoded bytes of the serialized transaction",

	// ScriptSig help.
	"scriptsig-asm": "Disassembly of the script",
	"scriptsig-hex": "Hex-encoded bytes of the script",

	// PrevOut help.
	"prevout-addresses": "previous output addresses",
	"prevout-value":     "previous output value",

	// VinPrevOut help.
	"vinprevout-coinbase":    "The hex-encoded bytes of the signature script (coinbase txns only)",
	"vinprevout-stakebase":   "The hash of the stake transaction",
	"vinprevout-txid":        "The hash of the origin transaction (non-coinbase txns only)",
	"vinprevout-vout":        "The index of the output being redeemed from the origin transaction (non-coinbase txns only)",
	"vinprevout-tree":        "The transaction tree of the origin transaction (non-coinbase txns only)",
	"vinprevout-amountin":    "The amount in for this transaction input, in coins",
	"vinprevout-blockheight": "The height of the block that includes the origin transaction (non-coinbase txns only)",
	"vinprevout-blockindex":  "The merkle tree index of the origin transaction (non-coinbase txns only)",
	"vinprevout-scriptSig":   "The signature script used to redeem the origin transaction as a JSON object (non-coinbase txns only)",
	"vinprevout-prevOut":     "Data from the origin transaction output with index vout.",
	"vinprevout-sequence":    "The script sequence number",

	// Vin help.
	"vin-coinbase":    "The hex-encoded bytes of the signature script (coinbase txns only)",
	"vin-stakebase":   "The hash of the stake transaction",
	"vin-txid":        "The hash of the origin transaction (non-coinbase txns only)",
	"vin-vout":        "The index of the output being redeemed from the origin transaction (non-coinbase txns only)",
	"vin-scriptSig":   "The signature script used to redeem the origin transaction as a JSON object (non-coinbase txns only)",
	"vin-sequence":    "The script sequence number",
	"vin-tree":        "The tree of the transaction",
	"vin-blockindex":  "The block idx of the origin transaction",
	"vin-blockheight": "The block height of the origin transaction",
	"vin-amountin":    "The amount in",

	// ScriptPubKeyResult help.
	"scriptpubkeyresult-asm":       "Disassembly of the script",
	"scriptpubkeyresult-hex":       "Hex-encoded bytes of the script",
	"scriptpubkeyresult-reqSigs":   "The number of required signatures",
	"scriptpubkeyresult-type":      "The type of the script (e.g. 'pubkeyhash')",
	"scriptpubkeyresult-addresses": "The decred addresses associated with this script",
	"scriptpubkeyresult-commitamt": "The ticket commitment value if the script is for a staking commitment",

	// Vout help.
	"vout-value":        "The amount in DCR",
	"vout-n":            "The index of this transaction output",
	"vout-scriptPubKey": "The public key script used to pay coins as a JSON object",
	"vout-version":      "The version of the vout",

	// TxRawDecodeResult help.
	"txrawdecoderesult-txid":     "The hash of the transaction",
	"txrawdecoderesult-version":  "The transaction version",
	"txrawdecoderesult-locktime": "The transaction lock time",
	"txrawdecoderesult-vin":      "The transaction inputs as JSON objects",
	"txrawdecoderesult-vout":     "The transaction outputs as JSON objects",
	"txrawdecoderesult-expiry":   "The transaction expiry",

	// DecodeRawTransactionCmd help.
	"decoderawtransaction--synopsis": "Returns a JSON object representing the provided serialized, hex-encoded transaction.",
	"decoderawtransaction-hextx":     "Serialized, hex-encoded transaction",

	// DecodeScriptResult help.
	"decodescriptresult-asm":       "Disassembly of the script",
	"decodescriptresult-reqSigs":   "The number of required signatures",
	"decodescriptresult-type":      "The type of the script (e.g. 'pubkeyhash')",
	"decodescriptresult-addresses": "The decred addresses associated with this script",
	"decodescriptresult-p2sh":      "The script hash for use in pay-to-script-hash transactions (only present if the provided redeem script is not already a pay-to-script-hash script)",

	// DecodeScriptCmd help.
	"decodescript--synopsis": "Returns a JSON object with information about the provided hex-encoded script.",
	"decodescript-hexscript": "Hex-encoded script",

	// ExistsAddressCmd help.
	"existsaddress--synopsis": "Test for the existence of the provided address",
	"existsaddress-address":   "The address to check",
	"existsaddress--result0":  "Bool showing if address exists or not",

	// ExistsAddressesCmd help.
	"existsaddresses--synopsis": "Test for the existence of the provided addresses in the blockchain or memory pool",
	"existsaddresses-addresses": "The addresses to check",
	"existsaddresses--result0":  "Bitset of bools showing if addresses exist or not",

	// ExitsMissedTicketsCmd help.
	"existsmissedtickets--synopsis":  "Test for the existence of the provided tickets in the missed ticket map",
	"existsmissedtickets-txhashblob": "Blob containing the hashes to check",
	"existsmissedtickets--result0":   "Bool blob showing if the ticket exists in the missed ticket database or not",

	// ExistsExpiredTicketsCmd help.
	"existsexpiredtickets--synopsis":  "Test for the existence of the provided tickets in the expired ticket map",
	"existsexpiredtickets-txhashblob": "Blob containing the hashes to check",
	"existsexpiredtickets--result0":   "Bool blob showing if ticket exists in the expired ticket database or not",

	// ExistsLiveTicketCmd help.
	"existsliveticket--synopsis": "Test for the existence of the provided ticket",
	"existsliveticket-txhash":    "The ticket hash to check",
	"existsliveticket--result0":  "Bool showing if address exists in the live ticket database or not",

	// ExistsLiveTicketsCmd help.
	"existslivetickets--synopsis":  "Test for the existence of the provided tickets in the live ticket map",
	"existslivetickets-txhashblob": "Blob containing the hashes to check",
	"existslivetickets--result0":   "Bool blob showing if ticket exists in the live ticket database or not",

	// ExistsMempoolTxsCmd help.
	"existsmempooltxs--synopsis":  "Test for the existence of the provided txs in the mempool",
	"existsmempooltxs-txhashblob": "Blob containing the hashes to check",
	"existsmempooltxs--result0":   "Bool blob showing if txs exist in the mempool or not",

	// GenerateCmd help
	"generate--synopsis": "Generates a set number of blocks (simnet or regtest only) and returns a JSON\n" +
		" array of their hashes.",
	"generate-numblocks": "Number of blocks to generate",
	"generate--result0":  "The hashes, in order, of blocks generated by the call",

	// GetAddedNodeInfoResultAddr help.
	"getaddednodeinforesultaddr-address":   "The ip address for this DNS entry",
	"getaddednodeinforesultaddr-connected": "The connection 'direction' (inbound/outbound/false)",

	// GetAddedNodeInfoResult help.
	"getaddednodeinforesult-addednode": "The ip address or domain of the added peer",
	"getaddednodeinforesult-connected": "Whether or not the peer is currently connected",
	"getaddednodeinforesult-addresses": "DNS lookup and connection information about the peer",

	// GetAddedNodeInfo help.
	"getaddednodeinfo--synopsis":   "Returns information about manually added (persistent) peers.",
	"getaddednodeinfo-dns":         "Specifies whether the returned data is a JSON object including DNS and connection information, or just a list of added peers",
	"getaddednodeinfo-node":        "Only return information about this specific peer instead of all added peers",
	"getaddednodeinfo--condition0": "dns=false",
	"getaddednodeinfo--condition1": "dns=true",
	"getaddednodeinfo--result0":    "List of added peers",

	// GetBestBlockResult help.
	"getbestblockresult-hash":   "Hex-encoded bytes of the best block hash",
	"getbestblockresult-height": "Height of the best block",

	// GetBestBlockCmd help.
	"getbestblock--synopsis": "Get block height and hash of best block in the main chain.",
	"getbestblock--result0":  "Get block height and hash of best block in the main chain.",

	// GetBestBlockHashCmd help.
	"getbestblockhash--synopsis": "Returns the hash of the of the best (most recent) block in the longest block chain.",
	"getbestblockhash--result0":  "The hex-encoded block hash",

	// GetBlockCmd help.
	"getblock--synopsis":   "Returns information about a block given its hash.",
	"getblock-hash":        "The hash of the block",
	"getblock-verbose":     "Specifies the block is returned as a JSON object instead of hex-encoded string",
	"getblock-verbosetx":   "Specifies that each transaction is returned as a JSON object and only applies if the verbose flag is true (dcrd extension)",
	"getblock--condition0": "verbose=false",
	"getblock--condition1": "verbose=true",
	"getblock--result0":    "Hex-encoded bytes of the serialized block",

	// TxRawResult help.
	"txrawresult-hex":           "Hex-encoded transaction",
	"txrawresult-txid":          "The hash of the transaction",
	"txrawresult-version":       "The transaction version",
	"txrawresult-locktime":      "The transaction lock time",
	"txrawresult-vin":           "The transaction inputs as JSON objects",
	"txrawresult-vout":          "The transaction outputs as JSON objects",
	"txrawresult-blockhash":     "Hash of the block the transaction is part of",
	"txrawresult-confirmations": "Number of confirmations of the block",
	"txrawresult-time":          "Transaction time in seconds since 1 Jan 1970 GMT",
	"txrawresult-blocktime":     "Block time in seconds since the 1 Jan 1970 GMT",
	"txrawresult-blockindex":    "Index of the containing block.",
	"txrawresult-blockheight":   "Height of the block the transaction is part of",
	"txrawresult-expiry":        "The transacion expiry",

	// SearchRawTransactionsResult help.
	"searchrawtransactionsresult-hex":           "Hex-encoded transaction",
	"searchrawtransactionsresult-txid":          "The hash of the transaction",
	"searchrawtransactionsresult-version":       "The transaction version",
	"searchrawtransactionsresult-locktime":      "The transaction lock time",
	"searchrawtransactionsresult-vin":           "The transaction inputs as JSON objects",
	"searchrawtransactionsresult-vout":          "The transaction outputs as JSON objects",
	"searchrawtransactionsresult-blockhash":     "Hash of the block the transaction is part of",
	"searchrawtransactionsresult-confirmations": "Number of confirmations of the block",
	"searchrawtransactionsresult-time":          "Transaction time in seconds since 1 Jan 1970 GMT",
	"searchrawtransactionsresult-blocktime":     "Block time in seconds since the 1 Jan 1970 GMT",

	// GetBlockVerboseResult help.
	"getblockverboseresult-hash":              "The hash of the block (same as provided)",
	"getblockverboseresult-confirmations":     "The number of confirmations",
	"getblockverboseresult-size":              "The size of the block",
	"getblockverboseresult-height":            "The height of the block in the block chain",
	"getblockverboseresult-version":           "The block version",
	"getblockverboseresult-merkleroot":        "Root hash of the merkle tree",
	"getblockverboseresult-tx":                "The transaction hashes (only when verbosetx=false)",
	"getblockverboseresult-rawtx":             "The transactions as JSON objects (only when verbosetx=true)",
	"getblockverboseresult-time":              "The block time in seconds since 1 Jan 1970 GMT",
	"getblockverboseresult-nonce":             "The block nonce",
	"getblockverboseresult-bits":              "The bits which represent the block difficulty",
	"getblockverboseresult-difficulty":        "The proof-of-work difficulty as a multiple of the minimum difficulty",
	"getblockverboseresult-previousblockhash": "The hash of the previous block",
	"getblockverboseresult-nextblockhash":     "The hash of the next block (only if there is one)",
	"getblockverboseresult-sbits":             "The stake difficulty of theblock",
	"getblockverboseresult-poolsize":          "The total number of valid, spendable sstx (tickets) in the chain",
	"getblockverboseresult-revocations":       "The number of new ssrtx (tickets) of the given block",
	"getblockverboseresult-freshstake":        "The number of new sstx (tickets) of the given block",
	"getblockverboseresult-voters":            "The number of stake voters (ssgen) of the previous block",
	"getblockverboseresult-potential":         "The number of potential",
	"getblockverboseresult-overflow":          "The number of overflow",
	"getblockverboseresult-winner":            "The winning bucket to determine ssgen",
	"getblockverboseresult-votebits":          "The block's voting results",
	"getblockverboseresult-rawstx":            "The block's raw sstx hashes the were included",
	"getblockverboseresult-stx":               "The block's sstx hashes the were included",
	"getblockverboseresult-stakeroot":         "The block's sstx hashes the were included",
	"getblockverboseresult-finalstate":        "The block's finalstate",
	"getblockverboseresult-extradata":         "Extra data field for the requested block",
	"getblockverboseresult-stakeversion":      "Stake Version of the block",

	// GetBlockCountCmd help.
	"getblockcount--synopsis": "Returns the number of blocks in the longest block chain.",
	"getblockcount--result0":  "The current block count",

	// GetBlockHashCmd help.
	"getblockhash--synopsis": "Returns hash of the block in best block chain at the given height.",
	"getblockhash-index":     "The block height",
	"getblockhash--result0":  "The block hash",

	// GetBlockHeaderCmd help.
	"getblockheader--synopsis":   "Returns information about a block header given its hash.",
	"getblockheader-hash":        "The hash of the block",
	"getblockheader-verbose":     "Specifies the block header is returned as a JSON object instead of hex-encoded string",
	"getblockheader--condition0": "verbose=false",
	"getblockheader--condition1": "verbose=true",
	"getblockheader--result0":    "The block header hash",

	// GetBlockHeaderVerboseResult help.
	"getblockheaderverboseresult-hash":              "The hash of the block (same as provided)",
	"getblockheaderverboseresult-confirmations":     "The number of confirmations",
	"getblockheaderverboseresult-height":            "The height of the block in the block chain",
	"getblockheaderverboseresult-version":           "The block version",
	"getblockheaderverboseresult-merkleroot":        "The merkle root of the regular transaction tree",
	"getblockheaderverboseresult-time":              "The block time in seconds since 1 Jan 1970 GMT",
	"getblockheaderverboseresult-nonce":             "The block nonce",
	"getblockheaderverboseresult-bits":              "The bits which represent the block difficulty",
	"getblockheaderverboseresult-difficulty":        "The proof-of-work difficulty as a multiple of the minimum difficulty",
	"getblockheaderverboseresult-previousblockhash": "The hash of the previous block",
	"getblockheaderverboseresult-nextblockhash":     "The hash of the next block (only if there is one)",
	"getblockheaderverboseresult-size":              "The size of the block in bytes",
	"getblockheaderverboseresult-sbits":             "The stake difficulty in coins",
	"getblockheaderverboseresult-poolsize":          "The size of the live ticket pool",
	"getblockheaderverboseresult-revocations":       "The number of revocations in the block",
	"getblockheaderverboseresult-freshstake":        "The number of new tickets in the block",
	"getblockheaderverboseresult-voters":            "The number of votes in the block",
	"getblockheaderverboseresult-finalstate":        "The final state value of the ticket pool",
	"getblockheaderverboseresult-votebits":          "The vote bits",
	"getblockheaderverboseresult-stakeroot":         "The merkle root of the stake transaction tree",
	"getblockheaderverboseresult-stakeversion":      "The stake version of the block",

	// GetBlockSubsidyCmd help.
	"getblocksubsidy--synopsis": "Returns information regarding subsidy amounts.",
	"getblocksubsidy-height":    "The block height",
	"getblocksubsidy-voters":    "The number of voters",

	// GetBlockSubsidyResult help.
	"getblocksubsidyresult-developer": "The developer subsidy",
	"getblocksubsidyresult-pos":       "The Proof-of-Stake subsidy",
	"getblocksubsidyresult-pow":       "The Proof-of-Work subsidy",
	"getblocksubsidyresult-total":     "The total subsidy",

	// TemplateRequest help.
	"templaterequest-mode":         "This is 'template', 'proposal', or omitted",
	"templaterequest-capabilities": "List of capabilities",
	"templaterequest-longpollid":   "The long poll ID of a job to monitor for expiration; required and valid only for long poll requests ",
	"templaterequest-sigoplimit":   "Number of signature operations allowed in blocks (this parameter is ignored)",
	"templaterequest-sizelimit":    "Number of bytes allowed in blocks (this parameter is ignored)",
	"templaterequest-maxversion":   "Highest supported block version number (this parameter is ignored)",
	"templaterequest-target":       "The desired target for the block template (this parameter is ignored)",
	"templaterequest-data":         "Hex-encoded block data (only for mode=proposal)",
	"templaterequest-workid":       "The server provided workid if provided in block template (not applicable)",

	// GetBlockTemplateResultTx help.
	"getblocktemplateresulttx-data":    "Hex-encoded transaction data (byte-for-byte)",
	"getblocktemplateresulttx-hash":    "Hex-encoded transaction hash (little endian if treated as a 256-bit number)",
	"getblocktemplateresulttx-depends": "Other transactions before this one (by 1-based index in the 'transactions'  list) that must be present in the final block if this one is",
	"getblocktemplateresulttx-fee":     "Difference in value between transaction inputs and outputs (in Atoms)",
	"getblocktemplateresulttx-sigops":  "Total number of signature operations as counted for purposes of block limits",
	"getblocktemplateresulttx-txtype":  "Type of the transaction",

	// GetBlockTemplateResultAux help.
	"getblocktemplateresultaux-flags": "Hex-encoded byte-for-byte data to include in the coinbase signature script",

	// GetBlockTemplateResult help.
	"getblocktemplateresult-bits":              "Hex-encoded compressed difficulty",
	"getblocktemplateresult-curtime":           "Current time as seen by the server (recommended for block time); must fall within mintime/maxtime rules",
	"getblocktemplateresult-height":            "Height of the block to be solved",
	"getblocktemplateresult-previousblockhash": "Hex-encoded big-endian hash of the previous block",
	"getblocktemplateresult-sigoplimit":        "Number of sigops allowed in blocks ",
	"getblocktemplateresult-sizelimit":         "Number of bytes allowed in blocks",
	"getblocktemplateresult-transactions":      "Array of transactions as JSON objects",
	"getblocktemplateresult-version":           "The block version",
	"getblocktemplateresult-coinbaseaux":       "Data that should be included in the coinbase signature script",
	"getblocktemplateresult-coinbasetxn":       "Information about the coinbase transaction",
	"getblocktemplateresult-coinbasevalue":     "Total amount available for the coinbase in Atoms",
	"getblocktemplateresult-workid":            "This value must be returned with result if provided (not provided)",
	"getblocktemplateresult-longpollid":        "Identifier for long poll request which allows monitoring for expiration",
	"getblocktemplateresult-longpolluri":       "An alternate URI to use for long poll requests if provided (not provided)",
	"getblocktemplateresult-submitold":         "Not applicable",
	"getblocktemplateresult-target":            "Hex-encoded big-endian number which valid results must be less than",
	"getblocktemplateresult-expires":           "Maximum number of seconds (starting from when the server sent the response) this work is valid for",
	"getblocktemplateresult-maxtime":           "Maximum allowed time",
	"getblocktemplateresult-mintime":           "Minimum allowed time",
	"getblocktemplateresult-mutable":           "List of mutations the server explicitly allows",
	"getblocktemplateresult-noncerange":        "Two concatenated hex-encoded big-endian 32-bit integers which represent the valid ranges of nonces the miner may scan",
	"getblocktemplateresult-capabilities":      "List of server capabilities including 'proposal' to indicate support for block proposals",
	"getblocktemplateresult-reject-reason":     "Reason the proposal was invalid as-is (only applies to proposal responses)",
	"getblocktemplateresult-stransactions":     "Stake transactions",
	"getblocktemplateresult-header":            "Block header",

	// GetBlockTemplateCmd help.
	"getblocktemplate--synopsis": "Returns a JSON object with information necessary to construct a block to mine or accepts a proposal to validate.\n" +
		"See BIP0022 and BIP0023 for the full specification.",
	"getblocktemplate-request":     "Request object which controls the mode and several parameters",
	"getblocktemplate--condition0": "mode=template",
	"getblocktemplate--condition1": "mode=proposal, rejected",
	"getblocktemplate--condition2": "mode=proposal, accepted",
	"getblocktemplate--result1":    "An error string which represents why the proposal was rejected or nothing if accepted",

	// GetConnectionCountCmd help.
	"getconnectioncount--synopsis": "Returns the number of active connections to other peers.",
	"getconnectioncount--result0":  "The number of connections",

	// GetCurrentNetCmd help.
	"getcurrentnet--synopsis": "Get decred network the server is running on.",
	"getcurrentnet--result0":  "The network identifer",

	// GetDifficultyCmd help.
	"getdifficulty--synopsis": "Returns the proof-of-work difficulty as a multiple of the minimum difficulty.",
	"getdifficulty--result0":  "The difficulty",

	// GetStakeDifficultyCmd help.
	"getstakedifficulty--synopsis":     "Returns the proof-of-stake difficulty.",
	"getstakedifficultyresult-current": "The current top block's stake difficulty",
	"getstakedifficultyresult-next":    "The calculated stake difficulty of the next block",

	// GetStakeVersionInfoCmd help.
	"getstakeversioninfo--synopsis":           "Returns stake version statistics for one or more stake version intervals.",
	"getstakeversioninfo-count":               "Number of intervals to return.",
	"getstakeversioninforesult-currentheight": "Top of the chain height.",
	"getstakeversioninforesult-hash":          "Top of the chain hash.",
	"getstakeversioninforesult-intervals":     "Array of total stake and vote counts.",
	"versioncount-count":                      "Number of votes.",
	"versioncount-version":                    "Version of the vote.",
	"versioninterval-startheight":             "Start of the interval.",
	"versioninterval-endheight":               "End of the interval.",
	"versioninterval-voteversions":            "Tally of all vote versions.",
	"versioninterval-posversions":             "Tally of the stake versions.",

	// GetStakeDifficultyCmd help.
	"getstakeversions--synopsis":           "Returns the stake versions statistics.",
	"getstakeversions-hash":                "The start block hash.",
	"getstakeversions-count":               "The number of blocks that will be returned.",
	"getstakeversionsresult-stakeversions": "Array of stake versions per block.",
	"stakeversions-hash":                   "Hash of the block.",
	"stakeversions-height":                 "Height of the block.",
	"stakeversions-blockversion":           "The block version",
	"stakeversions-stakeversion":           "The stake version of the block",
	"stakeversions-votes":                  "The version and bits of each vote in the block",
	"versionbits-version":                  "The version of the vote.",
	"versionbits-bits":                     "The bits assigned by the vote.",

	// GetVoteInfo
	"getvoteinfo--synopsis":           "Returns the vote info statistics.",
	"getvoteinfo-version":             "The stake version.",
	"getvoteinforesult-currentheight": "Top of the chain height.",
	"getvoteinforesult-startheight":   "The start height of this voting window.",
	"getvoteinforesult-endheight":     "The end height of this voting window.",
	"getvoteinforesult-hash":          "The hash of the current height block.",
	"getvoteinforesult-voteversion":   "Selected vote version.",
	"getvoteinforesult-quorum":        "Minimum amount of votes required.",
	"getvoteinforesult-totalvotes":    "Total votes.",
	"getvoteinforesult-agendas":       "All agendas for this stake version.",
	"agenda-id":                       "Unique identifier of this agenda.",
	"agenda-description":              "Description of this agenda.",
	"agenda-mask":                     "Agenda mask.",
	"agenda-starttime":                "Time aganda becomes valid.",
	"agenda-expiretime":               "Time aganda becomes invalid.",
	"agenda-status":                   "Aganda status.",
	"agenda-quorumprogress":           "Progress of quorum reached.",
	"agenda-choices":                  "All choices in this agenda.",
	"choice-id":                       "Unique identifier of this choice.",
	"choice-description":              "Description of this choice.",
	"choice-bits":                     "Bits that dentify this choice.",
	"choice-isabstain":                "This choice is to abstain from change.",
	"choice-isno":                     "Hard no choice (1 and only 1 per agenda).",
	"choice-count":                    "How many votes received.",
	"choice-progress":                 "Progress of the overall count.",

	// GetGenerateCmd help.
	"getgenerate--synopsis": "Returns if the server is set to generate coins (mine) or not.",
	"getgenerate--result0":  "True if mining, false if not",

	// GetHashesPerSecCmd help.
	"gethashespersec--synopsis": "Returns a recent hashes per second performance measurement while generating coins (mining).",
	"gethashespersec--result0":  "The number of hashes per second",

	// InfoChainResult help.
	"infochainresult-version":         "The version of the server",
	"infochainresult-protocolversion": "The latest supported protocol version",
	"infochainresult-blocks":          "The number of blocks processed",
	"infochainresult-timeoffset":      "The time offset",
	"infochainresult-connections":     "The number of connected peers",
	"infochainresult-proxy":           "The proxy used by the server",
	"infochainresult-difficulty":      "The current target difficulty",
	"infochainresult-testnet":         "Whether or not server is using testnet",
	"infochainresult-relayfee":        "The minimum relay fee for non-free transactions in DCR/KB",
	"infochainresult-errors":          "Any current errors",

	// InfoWalletResult help.
	"infowalletresult-version":         "The version of the server",
	"infowalletresult-protocolversion": "The latest supported protocol version",
	"infowalletresult-walletversion":   "The version of the wallet server",
	"infowalletresult-balance":         "The total decred balance of the wallet",
	"infowalletresult-blocks":          "The number of blocks processed",
	"infowalletresult-timeoffset":      "The time offset",
	"infowalletresult-connections":     "The number of connected peers",
	"infowalletresult-proxy":           "The proxy used by the server",
	"infowalletresult-difficulty":      "The current target difficulty",
	"infowalletresult-testnet":         "Whether or not server is using testnet",
	"infowalletresult-keypoololdest":   "Seconds since 1 Jan 1970 GMT of the oldest pre-generated key in the key pool",
	"infowalletresult-keypoolsize":     "The number of new keys that are pre-generated",
	"infowalletresult-unlocked_until":  "The timestamp in seconds since 1 Jan 1970 GMT that the wallet is unlocked for transfers, or 0 if the wallet is locked",
	"infowalletresult-paytxfee":        "The transaction fee set in DCR/KB",
	"infowalletresult-relayfee":        "The minimum relay fee for non-free transactions in DCR/KB",
	"infowalletresult-errors":          "Any current errors",

	// GetHeadersCmd help.
	"getheaders--synopsis":     "Returns block headers starting with the first known block hash from the request",
	"getheaders-blocklocators": "Concatenated hashes of blocks.  Headers are returned starting from the first known hash in this list",
	"getheaders-hashstop":      "Optional block hash to stop including block headers for",
	"getheadersresult-headers": "Serialized block headers of all located blocks, limited to some arbitrary maximum number of hashes (currently 2000, which matches the wire protocol headers message, but this is not guaranteed)",

	// GetInfoCmd help.
	"getinfo--synopsis": "Returns a JSON object containing various state info.",

	// GetMempoolInfoCmd help.
	"getmempoolinfo--synopsis": "Returns memory pool information",

	// GetMempoolInfoResult help.
	"getmempoolinforesult-bytes": "Size in bytes of the mempool",
	"getmempoolinforesult-size":  "Number of transactions in the mempool",

	// GetMiningInfoResult help.
	"getmininginforesult-blocks":           "Height of the latest best block",
	"getmininginforesult-currentblocksize": "Size of the latest best block",
	"getmininginforesult-currentblocktx":   "Number of transactions in the latest best block",
	"getmininginforesult-difficulty":       "Current target difficulty",
	"getmininginforesult-stakedifficulty":  "Stake difficulty required for the next block",
	"getmininginforesult-errors":           "Any current errors",
	"getmininginforesult-generate":         "Whether or not server is set to generate coins",
	"getmininginforesult-genproclimit":     "Number of processors to use for coin generation (-1 when disabled)",
	"getmininginforesult-hashespersec":     "Recent hashes per second performance measurement while generating coins",
	"getmininginforesult-networkhashps":    "Estimated network hashes per second for the most recent blocks",
	"getmininginforesult-pooledtx":         "Number of transactions in the memory pool",
	"getmininginforesult-testnet":          "Whether or not server is using testnet",

	// GetMiningInfoCmd help.
	"getmininginfo--synopsis": "Returns a JSON object containing mining-related information.",

	// GetNetworkHashPSCmd help.
	"getnetworkhashps--synopsis": "Returns the estimated network hashes per second for the block heights provided by the parameters.",
	"getnetworkhashps-blocks":    "The number of blocks, or -1 for blocks since last difficulty change",
	"getnetworkhashps-height":    "Perform estimate ending with this height or -1 for current best chain block height",
	"getnetworkhashps--result0":  "Estimated hashes per second",

	// GetNetTotalsCmd help.
	"getnettotals--synopsis": "Returns a JSON object containing network traffic statistics.",

	// GetNetTotalsResult help.
	"getnettotalsresult-totalbytesrecv": "Total bytes received",
	"getnettotalsresult-totalbytessent": "Total bytes sent",
	"getnettotalsresult-timemillis":     "Number of milliseconds since 1 Jan 1970 GMT",

	// GetPeerInfoResult help.
	"getpeerinforesult-id":             "A unique node ID",
	"getpeerinforesult-addr":           "The ip address and port of the peer",
	"getpeerinforesult-addrlocal":      "Local address",
	"getpeerinforesult-services":       "Services bitmask which represents the services supported by the peer",
	"getpeerinforesult-relaytxes":      "Peer has requested transactions be relayed to it",
	"getpeerinforesult-lastsend":       "Time the last message was received in seconds since 1 Jan 1970 GMT",
	"getpeerinforesult-lastrecv":       "Time the last message was sent in seconds since 1 Jan 1970 GMT",
	"getpeerinforesult-bytessent":      "Total bytes sent",
	"getpeerinforesult-bytesrecv":      "Total bytes received",
	"getpeerinforesult-conntime":       "Time the connection was made in seconds since 1 Jan 1970 GMT",
	"getpeerinforesult-timeoffset":     "The time offset of the peer",
	"getpeerinforesult-pingtime":       "Number of microseconds the last ping took",
	"getpeerinforesult-pingwait":       "Number of microseconds a queued ping has been waiting for a response",
	"getpeerinforesult-version":        "The protocol version of the peer",
	"getpeerinforesult-subver":         "The user agent of the peer",
	"getpeerinforesult-inbound":        "Whether or not the peer is an inbound connection",
	"getpeerinforesult-startingheight": "The latest block height the peer knew about when the connection was established",
	"getpeerinforesult-currentheight":  "The current height of the peer",
	"getpeerinforesult-banscore":       "The ban score",
	"getpeerinforesult-syncnode":       "Whether or not the peer is the sync peer",

	// GetPeerInfoCmd help.
	"getpeerinfo--synopsis": "Returns data about each connected network peer as an array of json objects.",

	// GetRawMempoolVerboseResult help.
	"getrawmempoolverboseresult-size":             "Transaction size in bytes",
	"getrawmempoolverboseresult-fee":              "Transaction fee in decred",
	"getrawmempoolverboseresult-time":             "Local time transaction entered pool in seconds since 1 Jan 1970 GMT",
	"getrawmempoolverboseresult-height":           "Block height when transaction entered the pool",
	"getrawmempoolverboseresult-startingpriority": "Priority when transaction entered the pool",
	"getrawmempoolverboseresult-currentpriority":  "Current priority",
	"getrawmempoolverboseresult-depends":          "Unconfirmed transactions used as inputs for this transaction",

	// GetRawMempoolCmd help.
	"getrawmempool--synopsis":   "Returns information about all of the transactions currently in the memory pool.",
	"getrawmempool-verbose":     "Returns JSON object when true or an array of transaction hashes when false",
	"getrawmempool-txtype":      "Type of tx to return. (all/regular/tickets/votes/revocations)",
	"getrawmempool--condition0": "verbose=false",
	"getrawmempool--condition1": "verbose=true",
	"getrawmempool--result0":    "Array of transaction hashes",

	// GetRawTransactionCmd help.
	"getrawtransaction--synopsis":   "Returns information about a transaction given its hash.",
	"getrawtransaction-txid":        "The hash of the transaction",
	"getrawtransaction-verbose":     "Specifies the transaction is returned as a JSON object instead of a hex-encoded string",
	"getrawtransaction--condition0": "verbose=false",
	"getrawtransaction--condition1": "verbose=true",
	"getrawtransaction--result0":    "Hex-encoded bytes of the serialized transaction",

	// GetTicketPoolValue help.
	"getticketpoolvalue--synopsis": "Return the current value of all locked funds in the ticket pool",
	"getticketpoolvalue--result0":  "Total value of ticket pool",

	// GetTxOutResult help.
	"gettxoutresult-bestblock":     "The block hash that contains the transaction output",
	"gettxoutresult-confirmations": "The number of confirmations",
	"gettxoutresult-value":         "The transaction amount in DCR",
	"gettxoutresult-scriptPubKey":  "The public key script used to pay coins as a JSON object",
	"gettxoutresult-version":       "The transaction version",
	"gettxoutresult-coinbase":      "Whether or not the transaction is a coinbase",

	// GetTxOutCmd help.
	"gettxout--synopsis":      "Returns information about an unspent transaction output..",
	"gettxout-txid":           "The hash of the transaction",
	"gettxout-vout":           "The index of the output",
	"gettxout-includemempool": "Include the mempool when true",

	// GetWorkResult help.
	"getworkresult-data":     "Hex-encoded block data",
	"getworkresult-hash1":    "(DEPRECATED) Hex-encoded formatted hash buffer",
	"getworkresult-midstate": "(DEPRECATED) Hex-encoded precomputed hash state after hashing first half of the data",
	"getworkresult-target":   "Hex-encoded little-endian hash target",

	// GetWorkCmd help.
	"getwork--synopsis":   "(DEPRECATED - Use getblocktemplate instead) Returns formatted hash data to work on or checks and submits solved data.",
	"getwork-data":        "Hex-encoded data to check",
	"getwork--condition0": "no data provided",
	"getwork--condition1": "data provided",
	"getwork--result1":    "Whether or not the solved data is valid and was added to the chain",

	// HelpCmd help.
	"help--synopsis":   "Returns a list of all commands or help for a specified command.",
	"help-command":     "The command to retrieve help for",
	"help--condition0": "no command provided",
	"help--condition1": "command specified",
	"help--result0":    "List of commands",
	"help--result1":    "Help for specified command",

	// PingCmd help.
	"ping--synopsis": "Queues a ping to be sent to each connected peer.\n" +
		"Ping times are provided by getpeerinfo via the pingtime and pingwait fields.",

	// RebroadcastMissed help.
	"rebroadcastmissed--synopsis": "Asks the daemon to rebroadcast missed votes.\n",

	// RebroadcastWinnerCmd help.
	"rebroadcastwinners--synopsis": "Asks the daemon to rebroadcast the winners of the voting lottery.\n",

	// SearchRawTransactionsCmd help.
	"searchrawtransactions--synopsis": "Returns raw data for transactions involving the passed address.\n" +
		"Returned transactions are pulled from both the database, and transactions currently in the mempool.\n" +
		"Transactions pulled from the mempool will have the 'confirmations' field set to 0.\n" +
		"Usage of this RPC requires the optional --addrindex flag to be activated, otherwise all responses will simply return with an error stating the address index has not yet been built.\n" +
		"Similarly, until the address index has caught up with the current best height, all requests will return an error response in order to avoid serving stale data.",
	"searchrawtransactions-address":     "The Decred address to search for",
	"searchrawtransactions-verbose":     "Specifies the transaction is returned as a JSON object instead of hex-encoded string",
	"searchrawtransactions--condition0": "verbose=0",
	"searchrawtransactions--condition1": "verbose=1",
	"searchrawtransactions-skip":        "The number of leading transactions to leave out of the final response",
	"searchrawtransactions-count":       "The maximum number of transactions to return",
	"searchrawtransactions-vinextra":    "Specify that extra data from previous output will be returned in vin",
	"searchrawtransactions-reverse":     "Specifies that the transactions should be returned in reverse chronological order",
	"searchrawtransactions-filteraddrs": "Address list.  Only inputs or outputs with matching address will be returned",
	"searchrawtransactions--result0":    "Hex-encoded serialized transaction",

	// SendRawTransactionCmd help.
	"sendrawtransaction--synopsis":     "Submits the serialized, hex-encoded transaction to the local peer and relays it to the network.",
	"sendrawtransaction-hextx":         "Serialized, hex-encoded signed transaction",
	"sendrawtransaction-allowhighfees": "Whether or not to allow insanely high fees (dcrd does not yet implement this parameter, so it has no effect)",
	"sendrawtransaction--result0":      "The hash of the transaction",

	// SetGenerateCmd help.
	"setgenerate--synopsis":    "Set the server to generate coins (mine) or not.",
	"setgenerate-generate":     "Use true to enable generation, false to disable it",
	"setgenerate-genproclimit": "The number of processors (cores) to limit generation to or -1 for default",

	// StopCmd help.
	"stop--synopsis": "Shutdown dcrd.",
	"stop--result0":  "The string 'dcrd stopping.'",

	// SubmitBlockOptions help.
	"submitblockoptions-workid": "This parameter is currently ignored",

	// SubmitBlockCmd help.
	"submitblock--synopsis":   "Attempts to submit a new serialized, hex-encoded block to the network.",
	"submitblock-hexblock":    "Serialized, hex-encoded block",
	"submitblock-options":     "This parameter is currently ignored",
	"submitblock--condition0": "Block successfully submitted",
	"submitblock--condition1": "Block rejected",
	"submitblock--result1":    "The reason the block was rejected",

	// ValidateAddressResult help.
	"validateaddresschainresult-isvalid": "Whether or not the address is valid",
	"validateaddresschainresult-address": "The decred address (only when isvalid is true)",

	// ValidateAddressCmd help.
	"validateaddress--synopsis": "Verify an address is valid.",
	"validateaddress-address":   "Decred address to validate",

	// VerifyChainCmd help.
	"verifychain--synopsis": "Verifies the block chain database.\n" +
		"The actual checks performed by the checklevel parameter are implementation specific.\n" +
		"For dcrd this is:\n" +
		"checklevel=0 - Look up each block and ensure it can be loaded from the database.\n" +
		"checklevel=1 - Perform basic context-free sanity checks on each block.",
	"verifychain-checklevel": "How thorough the block verification is",
	"verifychain-checkdepth": "The number of blocks to check",
	"verifychain--result0":   "Whether or not the chain verified",

	// VerifyMessageCmd help.
	"verifymessage--synopsis": "Verify a signed message.",
	"verifymessage-address":   "The decred address to use for the signature",
	"verifymessage-signature": "The base-64 encoded signature provided by the signer",
	"verifymessage-message":   "The signed message",
	"verifymessage--result0":  "Whether or not the signature verified",

	// -------- Websocket-specific help --------

	// Session help.
	"session--synopsis":       "Return details regarding a websocket client's current connection session.",
	"sessionresult-sessionid": "The unique session ID for a client's websocket connection.",

	// NotifySpentAndMissedTicketsCmd help
	"notifyspentandmissedtickets--synopsis": "Request notifications for whenever tickets are spent or missed.",

	// NotifyNewTicketsCmd help
	"notifynewtickets--synopsis": "Request notifications for whenever new tickets are found.",

	// NotifyStakeDifficultyCmd help
	"notifystakedifficulty--synopsis": "Request notifications for whenever stake difficulty goes up.",

	// NotifyWinningTicketsCmd help
	"notifywinningtickets--synopsis": "Request notifications for whenever any tickets is chosen to vote.",

	// NotifyBlocksCmd help.
	"notifyblocks--synopsis": "Request notifications for whenever a block is connected or disconnected from the main (best) chain.",

	// StopNotifyBlocksCmd help.
	"stopnotifyblocks--synopsis": "Cancel registered notifications for whenever a block is connected or disconnected from the main (best) chain.",

	// NotifyNewTransactionsCmd help.
	"notifynewtransactions--synopsis": "Send either a txaccepted or a txacceptedverbose notification when a new transaction is accepted into the mempool.",
	"notifynewtransactions-verbose":   "Specifies which type of notification to receive. If verbose is true, then the caller receives txacceptedverbose, otherwise the caller receives txaccepted",

	// StopNotifyNewTransactionsCmd help.
	"stopnotifynewtransactions--synopsis": "Stop sending either a txaccepted or a txacceptedverbose notification when a new transaction is accepted into the mempool.",

	// OutPoint help.
	"outpoint-hash":  "The hex-encoded bytes of the outpoint hash",
	"outpoint-index": "The index of the outpoint",
	"outpoint-tree":  "The tree of the outpoint",

	// LoadTxFilterCmd help.
	"loadtxfilter--synopsis": "Load, add to, or reload a websocket client's transaction filter for mempool transactions, new blocks and rescans.",
	"loadtxfilter-reload":    "Load a new filter instead of adding data to an existing one",
	"loadtxfilter-addresses": "Array of addresses to add to the transaction filter",
	"loadtxfilter-outpoints": "Array of outpoints to add to the transaction filter",

	// Rescan help.
	"rescan--synopsis":   "Rescan blocks for transactions matching the loaded transaction filter.",
	"rescan-blockhashes": "Concatenated block hashes to rescan.  Each next block must be a child of the previous.",

	// -------- Decred-specific help --------

	// EstimateFee help.
	"estimatefee--synopsis": "Returns the estimated fee in dcr/kb.",
	"estimatefee-numblocks": "(unused)",
	"estimatefee--result0":  "Estimated fee.",

	// EstimateStakeDiff help.
	"estimatestakediff--synopsis":      "Estimate the next minimum, maximum, expected, and user-specified stake difficulty",
	"estimatestakediff-tickets":        "Use this number of new tickets in blocks to estimate the next difficulty",
	"estimatestakediffresult-min":      "Minimum estimate for stake difficulty",
	"estimatestakediffresult-max":      "Maximum estimate for stake difficulty",
	"estimatestakediffresult-expected": "Expected estimate for stake difficulty",
	"estimatestakediffresult-user":     "Estimate for stake difficulty with the passed user amount of tickets",

	// GetCoinSupply help
	"getcoinsupply--synopsis": "Returns current total coin supply in atoms",
	"getcoinsupply--result0":  "Current coin supply in atoms",

	// LiveTickets help.
	"livetickets--synopsis":     "Request tickets the live ticket hashes from the ticket database",
	"liveticketsresult-tickets": "List of live tickets",

	// MissedTickets help.
	"missedtickets--synopsis":     "Request tickets the client missed",
	"missedticketsresult-tickets": "List of missed tickets",

	// TicketBuckets help.
	"ticketbuckets--synopsis": "Request for the number of tickets currently in each bucket of the ticket database.",
	"ticketbucket-tickets":    "Number of tickets in bucket.",
	"ticketbucket-number":     "Bucket number.",

	// TicketFeeInfo help.
	"ticketfeeinfo--synopsis":            "Get various information about ticket fees from the mempool, blocks, and difficulty windows (units: DCR/kB)",
	"ticketfeeinfo-blocks":               "The number of blocks, starting from the chain tip and descending, to return fee information about",
	"ticketfeeinfo-windows":              "The number of difficulty windows to return ticket fee information about",
	"ticketfeeinforesult-feeinfomempool": "Ticket fee information for all tickets in the mempool (units: DCR/kB)",
	"ticketfeeinforesult-feeinfoblocks":  "Ticket fee information for a given list of blocks descending from the chain tip (units: DCR/kB)",
	"ticketfeeinforesult-feeinfowindows": "Ticket fee information for a window period where the stake difficulty was the same (units: DCR/kB)",

	"feeinfomempool-number": "Number of transactions in the mempool",
	"feeinfomempool-min":    "Minimum transaction fee in the mempool",
	"feeinfomempool-max":    "Maximum transaction fee in the mempool",
	"feeinfomempool-mean":   "Mean of transaction fees in the mempool",
	"feeinfomempool-median": "Median of transaction fees in the mempool",
	"feeinfomempool-stddev": "Standard deviation of transaction fees in the mempool",

	"feeinfoblock-height": "Height of the block",
	"feeinfoblock-number": "Number of transactions in the block",
	"feeinfoblock-min":    "Minimum transaction fee in the block",
	"feeinfoblock-max":    "Maximum transaction fee in the block",
	"feeinfoblock-mean":   "Mean of transaction fees in the block",
	"feeinfoblock-median": "Median of transaction fees in the block",
	"feeinfoblock-stddev": "Standard deviation of transaction fees in the block",

	"feeinfowindow-startheight": "First block in the window (inclusive)",
	"feeinfowindow-endheight":   "Last block in the window (exclusive)",
	"feeinfowindow-number":      "Number of transactions in the window",
	"feeinfowindow-min":         "Minimum transaction fee in the window",
	"feeinfowindow-max":         "Maximum transaction fee in the window",
	"feeinfowindow-mean":        "Mean of transaction fees in the window",
	"feeinfowindow-median":      "Median of transaction fees in the window",
	"feeinfowindow-stddev":      "Standard deviation of transaction fees in the window",

	// TicketsForAddress help.
	"ticketsforaddress--synopsis":     "Request all the tickets for an address.",
	"ticketsforaddress-address":       "Address to look for.",
	"ticketsforaddressresult-tickets": "Tickets owned by the specified address.",

	// TicketsForBucket help.
	"ticketsforbucket--synopsis":     "Request all the tickets and owners in a given bucket.",
	"ticketsforbucket-bucket":        "Bucket to look for.",
	"ticketsforbucketresult-tickets": "Result for the ticketsfor bucket command.",
	"ticket-owner":                   "Address owning the ticket.",
	"ticket-hash":                    "Hash of the ticket.",

	// TicketVWAP help.
	"ticketvwap--synopsis": "Calculate the volume weighted average price of tickets for a range of blocks (default: full PoS difficulty adjustment depth)",
	"ticketvwap-start":     "The start height to begin calculating the VWAP from",
	"ticketvwap-end":       "The end height to begin calculating the VWAP from",
	"ticketvwap--result0":  "The volume weighted average price",

	// TxFeeInfo help.
	"txfeeinfo--synopsis":            "Get various information about regular transaction fees from the mempool, blocks, and difficulty windows",
	"txfeeinfo-blocks":               "The number of blocks to calculate transaction fees for, starting from the end of the tip moving backwards",
	"txfeeinfo-rangestart":           "The start height of the block range to calculate transaction fees for",
	"txfeeinfo-rangeend":             "The end height of the block range to calculate transaction fees for",
	"txfeeinforesult-feeinfomempool": "Transaction fee information for all regular transactions in the mempool",
	"txfeeinforesult-feeinfoblocks":  "Transaction fee information for a given list of blocks descending from the chain tip",
	"txfeeinforesult-feeinforange":   "Transaction fee information for a window period where the stake difficulty was the same",

	"feeinforange-number": "Number of transactions in the window",
	"feeinforange-min":    "Minimum transaction fee in the window",
	"feeinforange-max":    "Maximum transaction fee in the window",
	"feeinforange-mean":   "Mean of transaction fees in the window",
	"feeinforange-median": "Median of transaction fees in the window",
	"feeinforange-stddev": "Standard deviation of transaction fees in the window",

	// Version help
	"version--synopsis":       "Returns the JSON-RPC API version (semver)",
	"version--result0--desc":  "Version objects keyed by the program or API name",
	"version--result0--key":   "Program or API name",
	"version--result0--value": "Object containing the semantic version",
}

// rpcResultTypes specifies the result types that each RPC command can return.
// This information is used to generate the help.  Each result type must be a
// pointer to the type (or nil to indicate no return value).
var rpcResultTypes = map[string][]interface{}{
	"addnode":               nil,
	"createrawsstx":         {(*string)(nil)},
	"createrawssgentx":      {(*string)(nil)},
	"createrawssrtx":        {(*string)(nil)},
	"createrawtransaction":  {(*string)(nil)},
	"debuglevel":            {(*string)(nil), (*string)(nil)},
	"decoderawtransaction":  {(*dcrjson.TxRawDecodeResult)(nil)},
	"decodescript":          {(*dcrjson.DecodeScriptResult)(nil)},
	"estimatefee":           {(*float64)(nil)},
	"estimatestakediff":     {(*dcrjson.EstimateStakeDiffResult)(nil)},
	"existsaddress":         {(*bool)(nil)},
	"existsaddresses":       {(*string)(nil)},
	"existsmissedtickets":   {(*string)(nil)},
	"existsexpiredtickets":  {(*string)(nil)},
	"existsliveticket":      {(*bool)(nil)},
	"existslivetickets":     {(*string)(nil)},
	"existsmempooltxs":      {(*string)(nil)},
	"getaddednodeinfo":      {(*[]string)(nil), (*[]dcrjson.GetAddedNodeInfoResult)(nil)},
	"getbestblock":          {(*dcrjson.GetBestBlockResult)(nil)},
	"generate":              {(*[]string)(nil)},
	"getbestblockhash":      {(*string)(nil)},
	"getblock":              {(*string)(nil), (*dcrjson.GetBlockVerboseResult)(nil)},
	"getblockcount":         {(*int64)(nil)},
	"getblockhash":          {(*string)(nil)},
	"getblockheader":        {(*string)(nil), (*dcrjson.GetBlockHeaderVerboseResult)(nil)},
	"getblocksubsidy":       {(*dcrjson.GetBlockSubsidyResult)(nil)},
	"getblocktemplate":      {(*dcrjson.GetBlockTemplateResult)(nil), (*string)(nil), nil},
	"getconnectioncount":    {(*int32)(nil)},
	"getcurrentnet":         {(*uint32)(nil)},
	"getdifficulty":         {(*float64)(nil)},
	"getstakedifficulty":    {(*dcrjson.GetStakeDifficultyResult)(nil)},
	"getstakeversioninfo":   {(*dcrjson.GetStakeVersionInfoResult)(nil)},
	"getstakeversions":      {(*dcrjson.GetStakeVersionsResult)(nil)},
	"getgenerate":           {(*bool)(nil)},
	"gethashespersec":       {(*float64)(nil)},
	"getheaders":            {(*dcrjson.GetHeadersResult)(nil)},
	"getinfo":               {(*dcrjson.InfoChainResult)(nil)},
	"getmempoolinfo":        {(*dcrjson.GetMempoolInfoResult)(nil)},
	"getmininginfo":         {(*dcrjson.GetMiningInfoResult)(nil)},
	"getnettotals":          {(*dcrjson.GetNetTotalsResult)(nil)},
	"getnetworkhashps":      {(*int64)(nil)},
	"getpeerinfo":           {(*[]dcrjson.GetPeerInfoResult)(nil)},
	"getrawmempool":         {(*[]string)(nil), (*dcrjson.GetRawMempoolVerboseResult)(nil)},
	"getrawtransaction":     {(*string)(nil), (*dcrjson.TxRawResult)(nil)},
	"getticketpoolvalue":    {(*float64)(nil)},
	"gettxout":              {(*dcrjson.GetTxOutResult)(nil)},
	"getvoteinfo":           {(*dcrjson.GetVoteInfoResult)(nil)},
	"getwork":               {(*dcrjson.GetWorkResult)(nil), (*bool)(nil)},
	"getcoinsupply":         {(*int64)(nil)},
	"help":                  {(*string)(nil), (*string)(nil)},
	"livetickets":           {(*dcrjson.LiveTicketsResult)(nil)},
	"missedtickets":         {(*dcrjson.MissedTicketsResult)(nil)},
	"node":                  nil,
	"ping":                  nil,
	"rebroadcastmissed":     nil,
	"rebroadcastwinners":    nil,
	"searchrawtransactions": {(*string)(nil), (*[]dcrjson.SearchRawTransactionsResult)(nil)},
	"sendrawtransaction":    {(*string)(nil)},
	"setgenerate":           nil,
	"stop":                  {(*string)(nil)},
	"submitblock":           {nil, (*string)(nil)},
	"ticketfeeinfo":         {(*dcrjson.TicketFeeInfoResult)(nil)},
	"ticketsforaddress":     {(*dcrjson.TicketsForAddressResult)(nil)},
	"ticketvwap":            {(*float64)(nil)},
	"txfeeinfo":             {(*dcrjson.TxFeeInfoResult)(nil)},
	"validateaddress":       {(*dcrjson.ValidateAddressChainResult)(nil)},
	"verifychain":           {(*bool)(nil)},
	"verifymessage":         {(*bool)(nil)},
	"version":               {(*map[string]dcrjson.VersionResult)(nil)},

	// Websocket commands.
	"loadtxfilter":                nil,
	"session":                     {(*dcrjson.SessionResult)(nil)},
	"notifywinningtickets":        nil,
	"notifyspentandmissedtickets": nil,
	"notifynewtickets":            nil,
	"notifystakedifficulty":       nil,
	"notifyblocks":                nil,
	"notifynewtransactions":       nil,
	"notifyreceived":              nil,
	"notifyspent":                 nil,
	"rescan":                      nil,
	"stopnotifyblocks":            nil,
	"stopnotifynewtransactions":   nil,
	"stopnotifyreceived":          nil,
	"stopnotifyspent":             nil,
}

// helpCacher provides a concurrent safe type that provides help and usage for
// the RPC server commands and caches the results for future calls.
type helpCacher struct {
	sync.Mutex
	usage      string
	methodHelp map[string]string
}

// rpcMethodHelp returns an RPC help string for the provided method.
//
// This function is safe for concurrent access.
func (c *helpCacher) rpcMethodHelp(method string) (string, error) {
	c.Lock()
	defer c.Unlock()

	// Return the cached method help if it exists.
	if help, exists := c.methodHelp[method]; exists {
		return help, nil
	}

	// Look up the result types for the method.
	resultTypes, ok := rpcResultTypes[method]
	if !ok {
		return "", errors.New("no result types specified for method " +
			method)
	}

	// Generate, cache, and return the help.
	help, err := dcrjson.GenerateHelp(method, helpDescsEnUS, resultTypes...)
	if err != nil {
		return "", err
	}
	c.methodHelp[method] = help
	return help, nil
}

// rpcUsage returns one-line usage for all support RPC commands.
//
// This function is safe for concurrent access.
func (c *helpCacher) rpcUsage(includeWebsockets bool) (string, error) {
	c.Lock()
	defer c.Unlock()

	// Return the cached usage if it is available.
	if c.usage != "" {
		return c.usage, nil
	}

	// Generate a list of one-line usage for every command.
	usageTexts := make([]string, 0, len(rpcHandlers))
	for k := range rpcHandlers {
		usage, err := dcrjson.MethodUsageText(k)
		if err != nil {
			return "", err
		}
		usageTexts = append(usageTexts, usage)
	}

	// Include websockets commands if requested.
	if includeWebsockets {
		for k := range wsHandlers {
			usage, err := dcrjson.MethodUsageText(k)
			if err != nil {
				return "", err
			}
			usageTexts = append(usageTexts, usage)
		}
	}

	sort.Sort(sort.StringSlice(usageTexts))
	c.usage = strings.Join(usageTexts, "\n")
	return c.usage, nil
}

// newHelpCacher returns a new instance of a help cacher which provides help and
// usage for the RPC server commands and caches the results for future calls.
func newHelpCacher() *helpCacher {
	return &helpCacher{
		methodHelp: make(map[string]string),
	}
}
