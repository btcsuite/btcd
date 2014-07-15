// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcjson

import (
	"errors"
)

// defaultHelpStrings contains the help text for all commands that are supported
// by btcjson by default.
var defaultHelpStrings = map[string]string{
	"addmultisigaddress": `addmultisigaddress nrequired ["key",...] ("account" )
Add a multisignature address to the wallet where 'nrequired' signatures are
required for spending. Each key is an address of publickey. Optionally, account
may be provided to assign the address to that account.`,

	"addnode": `addnode "node" "{add|remove|onetry}"
Add or remove a node from the list of hosts we connect to. If mode is "onetry"
then the host will be connected to once only, otherwise the node will be retried
upon disconnection.`,

	"backupwallet": `backupwallet "destination"
Safely copies the wallet file to the destination provided, either a directory or
a filename.`,

	"createmultisig": `createmultisig nrequired ["key", ...]
Creates a multi-signature address with m keys where "nrequired" signatures are
required from those m. A JSON object is returned containing the address and
redemption script:
{
	"address":"address",	# the value of the new address.
	redeemScript":"script"	# The stringified hex-encoded redemption script.
}`,

	"createrawtransaction": `createrawtransaction [{"txid":"id", "vout":n},...] {"address":amount,...}
Creates a transaction spending the given inputs and outputting to the
given addresses. The return is a hex encoded string of the raw
transaction with *unsigned* inputs. The transaction is not stored in any
wallet.`,

	// TODO(oga) this should be external since it is nonstandard.
	"debuglevel": `debuglevel "levelspec"
Dynamically changes the debug logging level. Levelspec must be either a debug
level, one of the following:
trace,
debug,
info,
warn,
error,
critical.
Alternatively levelspec  may be a specification of the form:
<subsystem>=<level>,<subsystem2>=<level2>
Where the valid subsystem names are:
AMGR,
BCDB,
BMGR,
BTCD,
CHAN,
DISC,
PEER,
RPCS,
SCRP,
SRVR,
TXMP.
Finally the keyword "show" will return a list of the available subsystems.
The command returns a string which will be "Done." if the command was sucessful,
or the list of subsystems if "show" was specified.`,

	"decoderawtransaction": `decoderawtransaction "hexstring"
Decodes the seralized, hex-encoded transaction in hexstring and returns a JSON
object representing it:
{
	"hex":"hex",	# String of the hex encoded transaction provided.
	"txid":"id",	# The sha id of the transaction as a hex string
	"version":n,	# The version of the transaction as a number.
	"locktime":t,	# Locktime of the tx (number).
	"vin": [	# Array of objects for inputs.
		{
			"txid":"id",		# Txid that is spent.
			"vout":n,		# Output number.
			"scriptSig": {
				"asm":"asm",	# Disasembled script as a string.
				"hex":"hex",	# Hex string of script.
			},
			"sequence":n,		# Sequence number of script.
		}
	],
	"vout": [	# Array of objects for outputs.
		{
			"value":x.xxx,		# Value in BTC.
			"n":n,			# Numeric index.
			"scriptPubkey": {	# Object representing script.
				"asm":"asm",	# Disassembled script as string.
				"hex":"hex",	# Hex string of script.
				"reqSigs":n, 	# Number of required signatures.
				"type":"type"	# Type as string, e.g.  pubkeyhash.
			},
		}
	]
	"blockhash":"hash",	# The block hash. as a string.
	"confirmations":n	# Number of confirmations in blockchain.
	"time":t,		# Transaction time in seconds since the epoch.
	"blocktime":t,		# Block time in seconds since the epoch.
}`,

	"decodescript": `decodescript "hex"
Decodes the hex encoded script passed as a string and returns a JSON object:
{
	"asm":"asm",		# disassembled string of the script.
	"hex":"hex",		# hex string of the script.
	"type":"type",		# type of script as a string.
	"reqSigs":n,		# number of required signatures.
	"addresses": [		# JSON array of address strings.
		"address",	# bitcoin address as a string.
	],
	"p2sh","address"	# script address as a string.
}`,

	"dumpprivkey": `dumpprivkey "bitcoinaddress"
Returns the private key corresponding to the provided address in a format
compatible with importprivkey.`,

	"dumpwallet": `dumpwallet "filename"
Dumps all wallet keys into "filename" in a human readable format.`,

	"encryptwallet": `encryptwallet "passphrase"
Encrypts the wallet with "passphrase". This command is for the initial
encryption of an otherwise unencrypted wallet, changing a passphrase
should use walletpassphrasechange.`,

	"estimatefee": `estimatefee "numblocks"
Estimates the approximate fee per kilobyte needed for a transaction to
get confirmed within 'numblocks' blocks.`,

	"estimatepriority": `estimatepriority "numblocks"
Estimates the approximate priority a zero-fee transaction needs to get
confirmed within 'numblocks' blocks.`,

	"getaccount": `getaccount "address"
Returns the account associated with the given "address" as a string.`,

	"getaccountaddress": `getaccountaddress "account"
Returns the current address used for receiving payments in this given account.`,

	"getaddednodeinfo": `getaddednodeinfo dns ( "node" )
Returns a list of JSON objects with information about the local list of
permanent nodes.  If dns is false, only a list of permanent nodes will
be provided, otherwise connected information will also be provided. If
node is not provided then all nodes will be detailed. The JSON return
format is as follows:
[
	{
		"addednode":"1.2.3.4",	# node ip address as a string
		"connected":true|false	# boolean connectionstate.
		"addresses": [
			"address":"1.2.3.4:5678"	# Bitcoin server host and port as a string.
			"connected":"inbound",		# The string "inbound" or "outbound".
		],
	},
	...
]`,

	"getaddressesbyaccount": `getaddressesbyaccount "account"
Returns the list of addresses for the given account:
[
	"address",	# Bitcoin address associated with the given account.
	...
]`,

	"getbalance": `getbalance ("account" "minconf")
Returns the balance for an account. If "account" is not specified this is the
total balance for the server. if "minconf" is provided then only transactions
with at least "minconf" confirmations will be counted for the balance. The
result is a JSON numeric type.`,

	"getbestblockhash": `getbestblockhash
Returns the hash of the last block in the longest blockchain known to
the server as a hex encoded string.`,

	"getblock": `getblock "hash" ( verbose  verbosetx=false)
Returns data about the block with hash "hash". If verbose is false a
string of hex-encoded data for the block is returned. If verbose is true
a JSON object is provided with the following:
{
	"hash":"hash",			# The block hash (same as argument).
	"confirmations":n,		# Number of confirmations as numeric.
	"size":n,			# Block size as numeric.
	"height":n,			# Block height as numeric.
	"version":n,			# Block version as numeric.
	"merkelroot":"...",		# The merkle root of the block.
	"tx" : [	# the transactions in the block as an array of strings.
		"transactionid",
		...
	],
	"time":t,			# The block time in seconds since the epoch.
	"nonce":n,			# The nonce of the block as a number.
	"bits":"1d00ffff",		# the compact representation of the difficulty as bits.
	"difficulty":n,			# the difficulty of the block as a number.
	"previousblockhash":"hash",	# hash of the previous block as a string.
	"nextblockhash":""hash",	# hash of the next block as a string.
}
If "verbosetx" is true, the returned object will contain a "rawtx" member
instead of the "tx" member; It  will contain objects representing the
transactions in the block in format used by getrawtransaction.
Please note that verbosetx is a btcd/btcjson extension.`,

	"getblockcount": `getblockcount
Returns a numeric for the number of blocks in the longest block chain.`,

	"getblockhash": `getblockhash index
Returns the hash of the block (as a string) at the given index in the
current blockchain.`,

	"getblocktemplate": `getblocktemplate ( jsonrequestobject )
Returns a block template for external mining purposes.  The optional
request object follows the following format:
{
	"mode":"template"	# Optional, "template" or omitted.
	"capabilities": [	# List of strings, optional.
		"support",	# Client side features supported. one of:
		...		# "longpoll", "coinbasetxn", "coinbasevalue",
		...		# "proposal", "serverlist", "workid".
	]
}
The result object is of the following format:
{
	"version":n,		# Numeric  block version.
	"previousblockhash":"string"	# Hash of the current tip of the blocktrain.
	"transactions":[
		"data"		# String of hex-encoded serialized transaction.
		"hash"		# Hex encoded hash of the tx.
		"depends": [	# Array of numbers representing the transactions
			n,	# that must be included in the final block if
			...,	# this one is. A 1 based index into the
			...,	# transactions list.
		]
		"fee":n		# Numeric transaction fee in satoshi. This is calculated by the diffrence between the sum of inputs and outputs. For coinbase transaction this is a negative number of total collected block fees. If not present fee is unknown; clients must not assume that there is no fee in this case.
		"sigops":n	# Number of total signature operations calculated for purposes of block limits. If not present the count is unknown but clients must not assume it is zero.
		"required":true|false	# If provided and true this transaction *must* be in the final block.
	],
	"coinbaseaux": {	# Object of data to be included in coinbase's scriptSig.
		"flags":"flags"	# String of flags.
	}
	"coinbasevalue"		# Numeric value in satoshi for maximum allowable input to coinbase transaction, including transaction fees and mining aware.
	"coinbasetxn":{}	# Object contining information for coinbase transaction.
	"target":"target",	# The hash target as a string.
	"mintime":t,		# Minimum timestamp appropriate for next block in seconds since the epoch.
	"mutable":[		# Array of ways the template may be modified.
		"value"		# e.g. "time", "transactions", "prevblock", etc
	]
	"noncerange":"00000000ffffffff"	# Wtring representing the range of valid nonces.
	"sigopliit"		# Numeric limit for max sigops in block.
	"sizelimit"		# Numeric limit of block size.
	"curtime":t		# Current timestamp in seconds since the epoch.
	"bits":"xxx",		# Compressed target for next block as string.
	"height":n,		# Numeric height of the next block.
}`,

	"getconnectioncount": `getconnectioncount
Returns the number of connections to other nodes currently active as a JSON
number.`,

	"getdifficulty": `getdifficulty
Returns the proof-of-work difficulty as a JSON number. The result is a
multiple of the minimum difficulty.`,

	"getgenerate": `getgenerate
Returns JSON boolean whether or not the server is set to "mine" coins or not.`,

	"gethashespersec": `gethashespersec
Returns a JSON number representing a recent measurement of hashes-per-second
during mining.`,

	"getinfo": `getinfo
Returns a JSON object containing state information about the service:
{
	"version":n,		# Numeric server version.
	"protocolversion":n,	# Numeric protocol version.
	"walletversion":n,	# Numeric wallet version.
	"balance":n,		# Total balance in the wallet as a number.
	"blocks":n,		# Numeric detailing current number of blocks.
	"timeoffset":n,		# Numeric server time offset.
	"proxy":"host:port"	# Optional string detailing the proxy in use.
	"difficulty":n,		# Current blockchain difficulty as a number.
	"testnet":true|false	# Boolean if the server is testnet.
	"keypoololdest":t,	# Oldest timstamp for pre generated keys. (in seconds since the epoch).
	"keypoolsize":n,	# Numeric size of the wallet keypool.
	"paytxfee":n,		# Numeric transaction fee that has been set.
	"unlocked_until":t,	# Numeric time the wallet is unlocked for in seconds since epoch.
	"errors":"..."		# Any error messages as a string.
}`,

	"getmininginfo": `getmininginfo
Returns a JSON object containing information related to mining:
{
	"blocks":n,		# Numeric current block
	"currentblocksize":n,	# Numeric last block size.
	currentblocktx":n,	# Numeric last block transaction
	"difficulty":n,		# Numeric current difficulty.
	"errors":"...",		# Current error string.
}`,

	"getnettotals": `getnettotals
Returns JSON object containing network traffic statistics:
{
	"totalbytesrecv":n,	# Numeric total bytes received.
	"totalbytessent":n,	# Numeric total bytes sent.
	"timemilis",t,		# Total numeric of milliseconds since epoch.
}`,

	"getnetworkhashps": `getnetworkhashps ( blocks=120 height=-1 )
Returns the estimated network hash rate per second based on the last
"blocks" blocks. If "blocks" is -1 then the number of blocks since the
last difficulty change will be used. If "height" is set then the
calculation will be carried out for the given block height instead of
the block tip. A JSON number is returned with the hashes per second
estimate.`,

	"getnewaddress": `getnewaddress ( "account" )
Returns a string for a new Bitcoin address for receiving payments. In the case
that "account" is specified then the address will be for "account", else the
default account will be used.`,

	"getpeerinfo": `getpeerinfo
Returns a list of JSON objects containing information about each connected
network node. The objects have the following format:
[
	{
		"addr":"host:port"	# IP and port of the peer as a string.
		"addrlocal":"ip:port"	# Local address as a string.
		"services":"00000001",	# Services bitmask as a string.
		"lastsend":t,		# Time in seconds since epoch since last send.
		"lastrecv":t,		# Time in seconds since epoch since last received message.
		"bytessent":n,		# Total number of bytes sent.
		"bytesrecv":n,		# Total number of bytes received
		"conntime":t,		# Connection time in seconds since epoc.
		"pingtime":n,		# Ping time
		"pingwait":n,		# Ping wait.
		"version":n,		# The numeric peer version.
		"subver":"/btcd:0.1/"	# The peer useragent string.
		"inbound":true|false,	# True or false whether peer is inbound.
		"startingheight":n,	# Numeric block heght of peer at connect time.
		"banscore":n,		# The numeric ban score.
		"syncnode":true|false,	# Boolean if the peer is the current sync node.
	}
]`,
	"getrawchangeaddress": `getrawchangeaddress
Returns a string containing a new Bitcoin addres for receiving change.
This rpc call is for use with raw transactions only.`,

	"getrawmempool": `getrawmempool ( verbose )
Returns the contents of the transaction memory pool as a JSON array. If
verbose is false just the transaction ids will be returned as strings.
If it is true then objects in the following format will be returned:
[
	"transactionid": {
		"size":n,		# Numeric transaction size in bytes.
		"fee":n,		# Numeric transqaction fee in btc.
		"time":t,		# Time transaction entered pool in seconds since the epoch.
		"height":n,		# Numeric block height when the transaction entered pool.
		"startingpriority:n,	# Numeric transaction priority when it entered the pool.
		"currentpriority":n,	# Numeric transaction priority.
		"depends":[		# Unconfirmed transactions used as inputs for this one.  As an array of strings.
			"transactionid",	# Parent transaction id.
		]
	},
	...
]`,

	"getrawtransaction": `getrawtransaction "txid" ( verbose )
Returns raw data related to "txid". If verbose is false, a string containing
hex-encoded serialized data for txid. If verbose is true a JSON object with the
following information about txid is returned:
{
	"hex":"data",	# String of serialized, hex encoded data for txid.
	"txid":"id",	# String containing the transaction id (same as "txid" parameter)
	"version":n	# Numeric tx version number.
	"locktime":t,	# Transaction locktime.
	"vin":[		# Array of objects representing transaction inputs.
		{
			"txid":"id",	# Spent transaction id as a string.
			"vout""n,	# Spent transaction output no.
			"scriptSig":{	# Signature script as an object.
				"asm":"asm",	# Disassembled script string.
				"hex":"hex",	# Hex serialized string.
			},
			"sequence":n,	# Script sequence number.
		},
		...
	],
	vout:[		# Array of objects representing transaction outputs.
		{
			"value":n,	# Numeric value of output in btc.
			"n", n,		# Numeric output index.
			"scriptPubKey":{	# Object representing pubkey script.
				"asm":"asm"	# Disassembled string of script.
				"hex":"hex"	# Hex serialized string.
				"reqSigs":n,	# Number of required signatures.
				"type":"pubkey",	# Type of scirpt. e.g.  pubkeyhash" or "pubkey".
				"addresses":[		# Array of address strings.
					"address",	# Bitcoin address.
					...
				],
			}
		}
	],
	"blockhash":"hash"	# Hash of the block the transaction is part of.
	"confirmations":n,	# Number of numeric confirmations of block.
	"time":t,		# Transaction time in seconds since the epoch.
	"blocktime":t,		# Block time in seconds since the epoch.
}`,

	"getreceivedbyaccount": `getreceivedbyaccount "account" ( minconf=1 )
Returns the total amount of BTC received by addresses related to "account".
Only transactions with at least "minconf" confirmations are considered.`,

	"getreceivedbyaddress": `getreceivedbyaddress "address" ( minconf=1 ) 
Returns the total amount of BTC received by the given address. Only transactions
with "minconf" confirmations will be used for the total.`,

	"gettransaction": `gettransaction "txid"
Returns a JSON object containing detailed information about the in-wallet
transaction "txid". The object follows the following format:
{
	"amount":n,		# Transaction amount in BTC.
	"confirmations":n,	# Number of confirmations for transaction.
	"blockhash":"hash",	# Hash of block transaction is part of.
	"blockindex":n,		# Index of block transaction is part of.
	"blocktime":t,		# Time of block transaction is part of.
	"txid":"id",		# Transaction id.
	"time":t,		# Transaction time in seconds since epoch.
	"timereceived":t,	# Time transaction was received in seconds since epoch.
	"details":[
		{
			"account":"name",	# The acount name involvedi n the transaction. "" means the default.
			"address":"address",	# The address involved in the transaction as a string.
			"category":"send|receive",	# Category - either send or receive.
			"amount":n,		# numeric amount in BTC.
		}
		...
	]
}`,

	"gettxout": `gettxout "txid" n ( includemempool )
Returns an object containing detauls about an unspent transaction output
the "n"th output of "txid":
{
	"bestblock":"hash",		# Block has containing transaction.
	"confirmations":n,		# Number of confirmations for block.
	"value":n			# Transaction value in BTC.
	scriptPubkey" {
		"asm":"asm",		# Disassembled string of script.
		"hex":"hex",		# String script serialized and hex encoded.
		"reqSigs"		# Numeric required signatures.
		"type":"pubkeyhash"	# Type of transaction. e.g. pubkeyhas
		"addresses":[		# Array of strings containing addresses.
			"address",
			...
		]
	},
}`,

	"gettxoutsetinfo": `gettxoutsetinfo
Returns an object containing startstics about the unspent transaction
output set:
{
	"height":n,			# Numeric current block height.
	"bestblock":"hex"		# Hex string of best block hash.
	"transactions":n,		# Numeric count of transactions.
	"txouts":n			# Numeric count of transaction outputs.
	"bytes_serialized":n		# Numeric serialized size.
	"hash_serialized"		# String of serialized hash.
	"total_amount":n,		# Numeric total amount in BTC.
}`,

	"getwork": `getwork ( "data" )
If "data" is present it is a hex encoded block datastruture that has been byte
reversed, if this is the case then the server will try to solve the
block and return true upon success, false upon failure. If data is not
specified the following object is returned to describe the work to be
solved:
{
	"midstate":"xxx",	# String of precomputed hash state for the first half of the data (deprecated).
	"data":"...",		# Block data as string.
	"hash1":"..."		# Hash buffer for second hash as string. (deprecated).
	"target":"...",		# Byte reversed hash target.
}`,

	"help": `help ( "command" )
With no arguemnts, lists the supported commands. If "command" is
specified then help text for that command is returned as a string.`,

	"importprivkey": `importprivkey "privkey" ( "label" rescan=true )
Adds a private key (in the same format as dumpprivkey) to the wallet. If label
is provided this is used as a label for the key. If rescan is true then the
blockchain will be scanned for transaction.`,

	"importwallet": `importwallet "filename"
Imports keys from the wallet dump file in "filename".`,

	"keypoolrefill": `keypoolrefill ( newsize=100 )
Refills the wallet pregenerated key pool to a size of "newsize"`,

	"listaccounts": `listaccounts ( minconf=1)
Returns a JSON object mapping account names to account balances:
{
	"accountname": n  # Account name to numeric balance in BTC.
	...
}`,

	"listaddressgroupings": `listaddressgroupings
Returns a JSON array of array of addreses which have had their relation to each
other made public by common use as inputs or in the change address. The data
takes the following format:
[
	[
		[
			"address",	# Bitcoin address.
			amount,		# Amount in BTC.
			"account"	# Optional account name string.
		],
		...
	],
	...
]`,

	"listlockunspent": `listlockunspent
Returns a JSON array of objects detailing transaction outputs that are
temporarily unspendable due to being processed by the lockunspent call.
[
	{
		"txid":"txid"	# Id of locked transaction as a string.
		"vout":n,	# Numeric index of locked output.
	},
	...
]`,

	"listreceivedbyaccount": `listreceivedbyaccount ( minconf=1 includeempty=false )
Returns a JSON array containing objects for each account detailing their
balances. Only transaction with at least "minconf" confirmations will be
included. If "includeempty" is true then accounts who have received no payments
will also be included, else they will be elided. The format is as follows:
[
	{
		"account":"name",	# Name of the receiving account.
		"amount":n,		# Total received amount in BTC.
		"confirmations":n,	# Total confirmations for most recent transaction.
	}
]`,

	"listreceivedbyaddress": `listreceivedbyaddress ( minconf=1 includeempty=false )
Returns a JSON array containing objects for each address detailing their
balances. Only transaction with at least "minconf" confirmations will be
included. If "includeempty" is true then adresses who have received no payments
will also be included, else they will be elided. The format is as follows:
[
	{
		"account":"name",	# Name of the receiving account.
		"amount":n,		# Total received amount in BTC.
		"confirmations":n,	# Total confirmations for most recent transaction.
	}
]`,

	"listsinceblock": `listsinceblock ("blockhash" minconf=1)
Gets all wallet transactions in block since "blockhash", if blockhash is
omitted then all transactions are provided. If present the only transactions
with "minconf" confirmations are listed.
{
	"transactions":[
		"account"		# String of account related to the transaction.
		"address"		# String of related address of transaction.
		"category":"send|receive"	# String detailing whether transaction was a send or receive of funds.	
		"amount":n,		# Numeric value of transaction. Negative if transaction category was "send"
		"fee":n,		# Numeric value of transaction fee in BTC.
		"confirmations":n,	# Number of transaction confirmations
		"blockhash":"hash"	# String of hash of block transaction is part of.
		"blockindex":n,		# Numeric index of block transaction is part of.
		"blocktime":t,		# Block time in seconds since the epoch.
		"txid":"id",		# Transaction id.
		"time":t,		# Transaction time in second since the epoch.
		"timereceived":t,	# Time transaction received in seconds since the epoch.
		"comment":"...",	# String of the comment associated with the transaction.
		"to":"...",	# String of "to" comment of the transaction.
	]
	"lastblock":"lastblockhash"	# Hash of the last block as a string.
}`,

	"listtransactions": `listtransactions ( "account" count=10 from=0 )
Returns up to "count" most recent wallet transactions for "account" (or all
accounts if none specified) skipping the first "from" transactions.
{
	"transactions":[
		"account"		# String of account related to the transaction.
		"address"		# String of related address of transaction.
		"category":"send|receive|move"	# String detailing whether transaction was a send or receive of funds.Move is a local move between accounts and doesnt touch the blockchain.	
		"amount":n,		# Numeric value of transaction. Negative if transaction category was "send"
		"fee":n,		# Numeric value of transaction fee in BTC.
		"confirmations":n,	# Number of transaction confirmations
		"blockhash":"hash"	# String of hash of block transaction is part of.
		"blockindex":n,		# Numeric index of block transaction is part of.
		"blocktime":t,		# Block time in seconds since the epoch.
		"txid":"id",		# Transaction id.
		"time":t,		# Transaction time in second since the epoch.
		"timereceived":t,	# Time transaction received in seconds since the epoch.
		"comment":"...",	# String of the comment associated with the transaction.
		"to":"...",	# String of "to" comment of the transaction.
	]
	"lastblock":"lastblockhash"	# Hash of the last block as a string.
}`,

	"listunspent": `listunspent (minconf=1 maxconf=9999999 ["address",...]
Returns a JSON array of objects representing unspent transaction outputs with
between minconf and maxconf confirmations (inclusive). If the array of addresses
is present then only txouts paid to the addresses listed will be considered. The
objects take the following format:
[
	{
		"txid":"id",		# The transaction id as a string.
		"vout":n,		# The output number of the tx.
		"address":"add",	# String of the transaction address.
		"account":"acc",	# The associated account, "" for default.
		"scriptPubkey":"key"	# The pubkeyscript as a string.
		"amount":n,		# The value of the transaction in BTC.
		"confirmations":n,	# The numer of confirmations.
	},
	...
]`,

	"lockunspent": `lockunspent unlock [{"txid":id", "vout":n},...]
Changes the llist of temporarily unspendable transaction outputs. The
transacion outputs in the list of objects provided will be marked as
locked (if unlock is false) or unlocked (unlock is true). A locked
transaction will not be chosen automatically to be spent when a
tranaction is created. Boolean is returned whether the command succeeded.`,

	"move": `move "fromaccount" "toaccount" amount ( minconf=1 "comment" )
Moves a specifies amount from "fromaccount" to "toaccount". Only funds
with minconf confirmations are used. If comment is present this comment
will be stored in the wallet with the transaction. Boolean is returned
to denode success.`,

	"ping": `ping
Queues a ping to be sent to each connected peer. Ping times are provided in
getpeerinfo.`,

	"sendfrom": `sendfrom "fromaccount" "tobitcoinaddress" amount ( minconf=1 "comment" "comment-to" )
Sends "amount" (rounded to the nearest 0.00000001) to
"tobitcoindaddress" from "fromaccount". Only funds with at least
"minconf" confirmations will be considered. If "comment" is present this
will be store the purpose of the transaction. If "comment-to" is present
this comment will be recorded locally to store the recipient of the
transaction.`,

	"sendmany": `sendmany "fromaccount" {"address":amount,...} ( minconf=1 "comment" )
Sends funds to multiple recipients. Funds from "fromaccount" are send to the
address/amount pairs specified. Only funds with minconf confirmations are
considered. "comment" if present is recorded locally as the purpose of the
transaction.`,

	"sendrawtransaction": `sendrawtransaction "hexstring" (allowhighfees=false)
Submits the hex-encoded transaction in "hexstring" to the bitcoin network via
the local node. If allowhighfees is true then high fees will be allowed.`,

	"sendtoaddress": `sendtoaddress "bitcoindaddress" amount ( "comment" "comment-to")
Send "amount" BTC (rounded to the nearest 0.00000001) to "bitcoinaddress". If
comment is set, it will be stored locally to describe the purpose of the
transaction. If comment-to" is set it will be stored locally to describe the
recipient of the transaction. A string containing the transaction id is
returned if successful.`,

	"setaccount": `setaccount "address" "account"
Sets the account associated with "address" to "account".`,

	"setgenerate": `setgenerate generate ( genproclimit )
Sets the current mining state to "generate". Up to "genproclimit" processors
will be used, if genproclimit is -1 then it is unlimited.`,

	"settxfee": `settxfee amount
Sets the current transaction fee.`,

	"signmessage": `signmessage "address" "message"
Returns a signature for "message" with the private key of "address"`,

	"signrawtransaction": `signrawtransaction "hexstring" ( prevtxs  ["privatekey",...] sighashtype="ALL" )
Signs the inputs for the serialized and hex encoded transaction in
"hexstring".  "prevtxs" optionally is an is an array of objects
representing the previous tx outputs that are spent here but may not yet
be in the blockchain, it takes the following format:
[
	{
		"txid":id:,		# String of transaction id.
		"vout":n,		# Output number.
		"scriptPubKey":"hex",	# Hex encoded string of pubkey script from transaction.
		"redemScript":"hex"	# Hex encoded redemption script.
	},
	...
]
If the third argument is provided these base58-encoded private keys in
the list will be the only keys used to sign the transaction. sighashtype
optionally denoes the signature hash type to be used. is must be one of the
following:
	"ALL",
	"NONE",
	"SINGLE",
	"ALL|ANYONECANPAY",
	"NONE|ANYONECANPAY",
	"SINGLE|ANYONECANPAY".
The return value takes the following format:
{
	"hex":"value"	# Hex string of raw transactino with signatures applied.
	"complete":n,	# If the transaction has a complete set of signatures. 0 if false.
}`,

	"stop": `stop
Stop the server.`,

	"submitblock": `submitblock "data" ( optionalparameterobject )
Will attempt to submit the block serialized in "data" to the bitcoin network.
optionalparametersobject takes the following format:
{
	"workid":"id"	# If getblocktemplate provided a workid it must be included with submissions.
}`,

	"validateaddress": `validateaddress "address
Returns a JSON objects containing information about the provided "address".
Format:
{
	"isvalid":true|false,	# If the address is valid. If false this is the only property returned.
	"address:"address",	# The address that was validated.
	"ismine":true|false,	# If the address belongs to the server.
	"isscript:true|false,	# If the address is a script address.
	"pubkey":"pk",		# The hex value of the public key associated with the address.
	"iscompressed":true|false,	# If the address is compresssed.
	"account":"account",	# The related account. "" is the default.
}`,

	"verifychain": `verifychain ( checklevel=3 numblocks=288 )
Verifies the stored blockchain database. "checklevel" denotes how thorough the
check is and "numblocks" how many blocks from the tip will be verified.`,

	"verifymessage": `verifymessage "bitcoinaddress" "signature" "message"
Verifies the signature "signature" for "message" from the key related
to "bitcoinaddress". The return is true or false if the signature
verified correctly.`,

	"walletlock": `walletlock
Removes any encryption key for the wallet from memory, unlocking the wallet.
In order to use wallet functionality again the wallet must be unlocked via
walletpassphrase.`,

	"walletpassphrase": `walletpassphrase "passphrase" timeout
Decrypts the wallet for "timeout" seconds with "passphrase". This is required
before using wallet functionality.`,

	"walletpassphrasechange": `walletpassphrasechange "oldpassphrase" "newpassphrase"
Changes the wallet passphrase from "oldpassphrase" to "newpassphrase".`,
}

// GetHelpString returns a string containing help text for "cmdName". If
// cmdName is unknown to btcjson - either via the default command list or those
// registered using RegisterCustomCmd - an error will be returned.
func GetHelpString(cmdName string) (string, error) {
	helpstr := ""

	if help, ok := defaultHelpStrings[cmdName]; ok {
		return help, nil
	}
	if c, ok := customCmds[cmdName]; ok {
		return c.helpString, nil
	}
	return helpstr, errors.New("invalid command specified")
}
