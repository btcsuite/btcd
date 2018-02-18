// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
)

var (
	// ErrWebsocketsRequired is an error to describe the condition where the
	// caller is trying to use a websocket-only feature, such as requesting
	// notifications or other websocket requests when the client is
	// configured to run in HTTP POST mode.
	ErrWebsocketsRequired = errors.New("a websocket connection is required " +
		"to use this feature")
)

// notificationState is used to track the current state of successfully
// registered notification so the state can be automatically re-established on
// reconnect.
type notificationState struct {
	notifyBlocks                bool
	notifyWinningTickets        bool
	notifySpentAndMissedTickets bool
	notifyNewTickets            bool
	notifyStakeDifficulty       bool
	notifyNewTx                 bool
	notifyNewTxVerbose          bool
}

// Copy returns a deep copy of the receiver.
func (s *notificationState) Copy() *notificationState {
	var stateCopy notificationState
	stateCopy.notifyBlocks = s.notifyBlocks
	stateCopy.notifyWinningTickets = s.notifyWinningTickets
	stateCopy.notifySpentAndMissedTickets = s.notifySpentAndMissedTickets
	stateCopy.notifyNewTickets = s.notifyNewTickets
	stateCopy.notifyStakeDifficulty = s.notifyStakeDifficulty
	stateCopy.notifyNewTx = s.notifyNewTx
	stateCopy.notifyNewTxVerbose = s.notifyNewTxVerbose

	return &stateCopy
}

// newNotificationState returns a new notification state ready to be populated.
func newNotificationState() *notificationState {
	return &notificationState{}
}

// newNilFutureResult returns a new future result channel that already has the
// result waiting on the channel with the reply set to nil.  This is useful
// to ignore things such as notifications when the caller didn't specify any
// notification handlers.
func newNilFutureResult() chan *response {
	responseChan := make(chan *response, 1)
	responseChan <- &response{result: nil, err: nil}
	return responseChan
}

// NotificationHandlers defines callback function pointers to invoke with
// notifications.  Since all of the functions are nil by default, all
// notifications are effectively ignored until their handlers are set to a
// concrete callback.
//
// NOTE: Unless otherwise documented, these handlers must NOT directly call any
// blocking calls on the client instance since the input reader goroutine blocks
// until the callback has completed.  Doing so will result in a deadlock
// situation.
type NotificationHandlers struct {
	// OnClientConnected is invoked when the client connects or reconnects
	// to the RPC server.  This callback is run async with the rest of the
	// notification handlers, and is safe for blocking client requests.
	OnClientConnected func()

	// OnBlockConnected is invoked when a block is connected to the longest
	// (best) chain.  It will only be invoked if a preceding call to
	// NotifyBlocks has been made to register for the notification and the
	// function is non-nil.
	OnBlockConnected func(blockHeader []byte, transactions [][]byte)

	// OnBlockDisconnected is invoked when a block is disconnected from the
	// longest (best) chain.  It will only be invoked if a preceding call to
	// NotifyBlocks has been made to register for the notification and the
	// function is non-nil.
	OnBlockDisconnected func(blockHeader []byte)

	// OnRelevantTxAccepted is invoked when an unmined transaction passes
	// the client's transaction filter.
	OnRelevantTxAccepted func(transaction []byte)

	// OnReorganization is invoked when the blockchain begins reorganizing.
	// It will only be invoked if a preceding call to NotifyBlocks has been
	// made to register for the notification and the function is non-nil.
	OnReorganization func(oldHash *chainhash.Hash, oldHeight int32,
		newHash *chainhash.Hash, newHeight int32)

	// OnWinningTickets is invoked when a block is connected and eligible tickets
	// to be voted on for this chain are given.  It will only be invoked if a
	// preceding call to NotifyWinningTickets has been made to register for the
	// notification and the function is non-nil.
	OnWinningTickets func(blockHash *chainhash.Hash,
		blockHeight int64,
		tickets []*chainhash.Hash)

	// OnSpentAndMissedTickets is invoked when a block is connected to the
	// longest (best) chain and tickets are spent or missed.  It will only be
	// invoked if a preceding call to NotifySpentAndMissedTickets has been made to
	// register for the notification and the function is non-nil.
	OnSpentAndMissedTickets func(hash *chainhash.Hash,
		height int64,
		stakeDiff int64,
		tickets map[chainhash.Hash]bool)

	// OnNewTickets is invoked when a block is connected to the longest (best)
	// chain and tickets have matured to become active.  It will only be invoked
	// if a preceding call to NotifyNewTickets has been made to register for the
	// notification and the function is non-nil.
	OnNewTickets func(hash *chainhash.Hash,
		height int64,
		stakeDiff int64,
		tickets []*chainhash.Hash)

	// OnStakeDifficulty is invoked when a block is connected to the longest
	// (best) chain and a new stake difficulty is calculated.  It will only
	// be invoked if a preceding call to NotifyStakeDifficulty has been
	// made to register for the notification and the function is non-nil.
	OnStakeDifficulty func(hash *chainhash.Hash,
		height int64,
		stakeDiff int64)

	// OnTxAccepted is invoked when a transaction is accepted into the
	// memory pool.  It will only be invoked if a preceding call to
	// NotifyNewTransactions with the verbose flag set to false has been
	// made to register for the notification and the function is non-nil.
	OnTxAccepted func(hash *chainhash.Hash, amount dcrutil.Amount)

	// OnTxAccepted is invoked when a transaction is accepted into the
	// memory pool.  It will only be invoked if a preceding call to
	// NotifyNewTransactions with the verbose flag set to true has been
	// made to register for the notification and the function is non-nil.
	OnTxAcceptedVerbose func(txDetails *dcrjson.TxRawResult)

	// OnBtcdConnected is invoked when a wallet connects or disconnects from
	// dcrd.
	//
	// This will only be available when client is connected to a wallet
	// server such as dcrwallet.
	OnBtcdConnected func(connected bool)

	// OnAccountBalance is invoked with account balance updates.
	//
	// This will only be available when speaking to a wallet server
	// such as dcrwallet.
	OnAccountBalance func(account string, balance dcrutil.Amount, confirmed bool)

	// OnWalletLockState is invoked when a wallet is locked or unlocked.
	//
	// This will only be available when client is connected to a wallet
	// server such as dcrwallet.
	OnWalletLockState func(locked bool)

	// OnTicketsPurchased is invoked when a wallet purchases an SStx.
	//
	// This will only be available when client is connected to a wallet
	// server such as dcrwallet.
	OnTicketsPurchased func(TxHash *chainhash.Hash, amount dcrutil.Amount)

	// OnVotesCreated is invoked when a wallet generates an SSGen.
	//
	// This will only be available when client is connected to a wallet
	// server such as dcrwallet.
	OnVotesCreated func(txHash *chainhash.Hash,
		blockHash *chainhash.Hash,
		height int32,
		sstxIn *chainhash.Hash,
		voteBits uint16)

	// OnRevocationsCreated is invoked when a wallet generates an SSRtx.
	//
	// This will only be available when client is connected to a wallet
	// server such as dcrwallet.
	OnRevocationsCreated func(txHash *chainhash.Hash,
		sstxIn *chainhash.Hash)

	// OnUnknownNotification is invoked when an unrecognized notification
	// is received.  This typically means the notification handling code
	// for this package needs to be updated for a new notification type or
	// the caller is using a custom notification this package does not know
	// about.
	OnUnknownNotification func(method string, params []json.RawMessage)
}

// handleNotification examines the passed notification type, performs
// conversions to get the raw notification types into higher level types and
// delivers the notification to the appropriate On<X> handler registered with
// the client.
func (c *Client) handleNotification(ntfn *rawNotification) {
	// Ignore the notification if the client is not interested in any
	// notifications.
	if c.ntfnHandlers == nil {
		return
	}

	switch ntfn.Method {
	// OnBlockConnected
	case dcrjson.BlockConnectedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnBlockConnected == nil {
			return
		}

		blockHeader, transactions, err := parseBlockConnectedParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid blockconnected "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnBlockConnected(blockHeader, transactions)

	// OnBlockDisconnected
	case dcrjson.BlockDisconnectedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnBlockDisconnected == nil {
			return
		}

		blockHeader, err := parseBlockDisconnectedParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid blockdisconnected "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnBlockDisconnected(blockHeader)

	case dcrjson.RelevantTxAcceptedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnRelevantTxAccepted == nil {
			return
		}

		transaction, err := parseRelevantTxAcceptedParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid relevanttxaccepted "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnRelevantTxAccepted(transaction)

	case dcrjson.ReorganizationNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnReorganization == nil {
			return
		}

		oldHash, oldHeight, newHash, newHeight, err :=
			parseReorganizationNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid reorganization "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnReorganization(oldHash, oldHeight, newHash, newHeight)

	// OnWinningTickets
	case dcrjson.WinningTicketsNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnWinningTickets == nil {
			return
		}

		blockHash, blockHeight, tickets, err :=
			parseWinningTicketsNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid winning tickets "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnWinningTickets(blockHash,
			blockHeight,
			tickets)

	// OnSpentAndMissedTickets
	case dcrjson.SpentAndMissedTicketsNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnSpentAndMissedTickets == nil {
			return
		}

		blockSha, blockHeight, stakeDifficulty, tickets, err :=
			parseSpentAndMissedTicketsNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid spend and missed tickets "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnSpentAndMissedTickets(blockSha,
			blockHeight,
			stakeDifficulty,
			tickets)

	// OnNewTickets
	case dcrjson.NewTicketsNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnNewTickets == nil {
			return
		}

		blockSha, blockHeight, stakeDifficulty, tickets, err :=
			parseNewTicketsNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid new tickets "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnNewTickets(blockSha,
			blockHeight,
			stakeDifficulty,
			tickets)

	// OnStakeDifficulty
	case dcrjson.StakeDifficultyNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnStakeDifficulty == nil {
			return
		}

		blockSha, blockHeight, stakeDiff,
			err := parseStakeDifficultyNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid stake difficulty "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnStakeDifficulty(blockSha,
			blockHeight,
			stakeDiff)

	// OnTxAccepted
	case dcrjson.TxAcceptedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnTxAccepted == nil {
			return
		}

		hash, amt, err := parseTxAcceptedNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid tx accepted "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnTxAccepted(hash, amt)

	// OnTxAcceptedVerbose
	case dcrjson.TxAcceptedVerboseNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnTxAcceptedVerbose == nil {
			return
		}

		rawTx, err := parseTxAcceptedVerboseNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid tx accepted verbose "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnTxAcceptedVerbose(rawTx)

	// OnBtcdConnected
	case dcrjson.BtcdConnectedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnBtcdConnected == nil {
			return
		}

		connected, err := parseBtcdConnectedNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid dcrd connected "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnBtcdConnected(connected)

	// OnAccountBalance
	case dcrjson.AccountBalanceNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnAccountBalance == nil {
			return
		}

		account, bal, conf, err := parseAccountBalanceNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid account balance "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnAccountBalance(account, bal, conf)

	// OnTicketPurchased:
	case dcrjson.TicketPurchasedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnTicketsPurchased == nil {
			return
		}

		txHash, amount, err := parseTicketPurchasedNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid ticket purchased "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnTicketsPurchased(txHash, amount)

	// OnVotesCreated:
	case dcrjson.VoteCreatedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnVotesCreated == nil {
			return
		}

		txHash, blockHash, height, sstxIn, voteBits, err :=
			parseVoteCreatedNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid vote created "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnVotesCreated(txHash, blockHash, height, sstxIn, voteBits)

	// OnRevocationsCreated:
	case dcrjson.RevocationCreatedNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnRevocationsCreated == nil {
			return
		}

		txHash, sstxIn, err := parseRevocationCreatedNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid revocation created "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnRevocationsCreated(txHash, sstxIn)

	// OnWalletLockState
	case dcrjson.WalletLockStateNtfnMethod:
		// Ignore the notification if the client is not interested in
		// it.
		if c.ntfnHandlers.OnWalletLockState == nil {
			return
		}

		// The account name is not notified, so the return value is
		// discarded.
		_, locked, err := parseWalletLockStateNtfnParams(ntfn.Params)
		if err != nil {
			log.Warnf("Received invalid wallet lock state "+
				"notification: %v", err)
			return
		}

		c.ntfnHandlers.OnWalletLockState(locked)

	// OnUnknownNotification
	default:
		if c.ntfnHandlers.OnUnknownNotification == nil {
			return
		}

		c.ntfnHandlers.OnUnknownNotification(ntfn.Method, ntfn.Params)
	}
}

// wrongNumParams is an error type describing an unparseable JSON-RPC
// notificiation due to an incorrect number of parameters for the
// expected notification type.  The value is the number of parameters
// of the invalid notification.
type wrongNumParams int

// Error satisifies the builtin error interface.
func (e wrongNumParams) Error() string {
	return fmt.Sprintf("wrong number of parameters (%d)", e)
}

func parseHexParam(param json.RawMessage) ([]byte, error) {
	var s string
	err := json.Unmarshal(param, &s)
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(s)
}

// parseBlockConnectedParams parses out the parameters included in a
// blockconnected notification.
func parseBlockConnectedParams(params []json.RawMessage) (blockHeader []byte, transactions [][]byte, err error) {
	if len(params) != 2 {
		return nil, nil, wrongNumParams(len(params))
	}

	blockHeader, err = parseHexParam(params[0])
	if err != nil {
		return nil, nil, err
	}

	var hexTransactions []string
	err = json.Unmarshal(params[1], &hexTransactions)
	if err != nil {
		return nil, nil, err
	}
	transactions = make([][]byte, len(hexTransactions))
	for i, hexTx := range hexTransactions {
		transactions[i], err = hex.DecodeString(hexTx)
		if err != nil {
			return nil, nil, err
		}
	}

	return blockHeader, transactions, nil
}

// parseBlockDisconnectedParams parses out the parameters included in a
// blockdisconnected notification.
func parseBlockDisconnectedParams(params []json.RawMessage) (blockHeader []byte, err error) {
	if len(params) != 1 {
		return nil, wrongNumParams(len(params))
	}

	return parseHexParam(params[0])
}

// parseRelevantTxAcceptedParams parses out the parameter included in a
// relevanttxaccepted notification.
func parseRelevantTxAcceptedParams(params []json.RawMessage) (transaction []byte, err error) {
	if len(params) != 1 {
		return nil, wrongNumParams(len(params))
	}

	return parseHexParam(params[0])
}

func parseReorganizationNtfnParams(params []json.RawMessage) (*chainhash.Hash,
	int32, *chainhash.Hash, int32, error) {
	errorOut := func(err error) (*chainhash.Hash, int32, *chainhash.Hash,
		int32, error) {
		return nil, 0, nil, 0, err
	}

	if len(params) != 4 {
		return errorOut(wrongNumParams(len(params)))
	}

	// Unmarshal first parameter as a string.
	var oldHashStr string
	err := json.Unmarshal(params[0], &oldHashStr)
	if err != nil {
		return errorOut(err)
	}

	// Unmarshal second parameter as an integer.
	var oldHeight int32
	err = json.Unmarshal(params[1], &oldHeight)
	if err != nil {
		return errorOut(err)
	}

	// Unmarshal first parameter as a string.
	var newHashStr string
	err = json.Unmarshal(params[2], &newHashStr)
	if err != nil {
		return errorOut(err)
	}

	// Unmarshal second parameter as an integer.
	var newHeight int32
	err = json.Unmarshal(params[3], &newHeight)
	if err != nil {
		return errorOut(err)
	}

	// Create hash from block sha string.
	oldHash, err := chainhash.NewHashFromStr(oldHashStr)
	if err != nil {
		return errorOut(err)
	}

	// Create hash from block sha string.
	newHash, err := chainhash.NewHashFromStr(newHashStr)
	if err != nil {
		return errorOut(err)
	}

	return oldHash, oldHeight, newHash, newHeight, nil
}

// parseWinningTicketsNtfnParams parses out the list of eligible tickets, block
// hash, and block height from a WinningTickets notification.
func parseWinningTicketsNtfnParams(params []json.RawMessage) (
	*chainhash.Hash,
	int64,
	[]*chainhash.Hash,
	error) {

	if len(params) != 3 {
		return nil, 0, nil, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	var blockHashStr string
	err := json.Unmarshal(params[0], &blockHashStr)
	if err != nil {
		return nil, 0, nil, err
	}

	// Create ShaHash from block sha string.
	bHash, err := chainhash.NewHashFromStr(blockHashStr)
	if err != nil {
		return nil, 0, nil, err
	}

	// Unmarshal second parameter as an integer.
	var blockHeight int32
	err = json.Unmarshal(params[1], &blockHeight)
	if err != nil {
		return nil, 0, nil, err
	}
	bHeight := int64(blockHeight)

	// Unmarshal third parameter as a slice.
	tickets := make(map[string]string)
	err = json.Unmarshal(params[2], &tickets)
	if err != nil {
		return nil, 0, nil, err
	}
	t := make([]*chainhash.Hash, len(tickets))

	for i, ticketHashStr := range tickets {
		// Create and cache Hash from tx hash.
		ticketHash, err := chainhash.NewHashFromStr(ticketHashStr)
		if err != nil {
			return nil, 0, nil, err
		}

		itr, err := strconv.Atoi(i)
		if err != nil {
			return nil, 0, nil, err
		}

		t[itr] = ticketHash
	}

	return bHash, bHeight, t, nil
}

// parseSpentAndMissedTicketsNtfnParams parses out the block header hash, height,
// winner number, and ticket map from a SpentAndMissedTickets notification.
func parseSpentAndMissedTicketsNtfnParams(params []json.RawMessage) (
	*chainhash.Hash,
	int64,
	int64,
	map[chainhash.Hash]bool,
	error) {

	if len(params) != 4 {
		return nil, 0, 0, nil, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	var blockShaStr string
	err := json.Unmarshal(params[0], &blockShaStr)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	// Create ShaHash from block sha string.
	sha, err := chainhash.NewHashFromStr(blockShaStr)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	// Unmarshal second parameter as an integer.
	var blockHeight int32
	err = json.Unmarshal(params[1], &blockHeight)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	bh := int64(blockHeight)

	// Unmarshal third parameter as an integer.
	var stakeDiff int64
	err = json.Unmarshal(params[2], &stakeDiff)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	// Unmarshal fourth parameter as a map[*hash]bool.
	tickets := make(map[string]string)
	err = json.Unmarshal(params[3], &tickets)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	t := make(map[chainhash.Hash]bool)

	for hashStr, spentStr := range tickets {
		isSpent := false
		if spentStr == "spent" {
			isSpent = true
		}

		// Create and cache ShaHash from tx hash.
		ticketSha, err := chainhash.NewHashFromStr(hashStr)
		if err != nil {
			return nil, 0, 0, nil, err
		}

		t[*ticketSha] = isSpent
	}

	return sha, bh, stakeDiff, t, nil
}

// parseNewTicketsNtfnParams parses out the block header hash, height,
// winner number, overflow, and ticket map from a NewTickets notification.
func parseNewTicketsNtfnParams(params []json.RawMessage) (*chainhash.Hash, int64, int64, []*chainhash.Hash, error) {

	if len(params) != 4 {
		return nil, 0, 0, nil, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	var blockShaStr string
	err := json.Unmarshal(params[0], &blockShaStr)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	// Create ShaHash from block sha string.
	sha, err := chainhash.NewHashFromStr(blockShaStr)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	// Unmarshal second parameter as an integer.
	var blockHeight int32
	err = json.Unmarshal(params[1], &blockHeight)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	bh := int64(blockHeight)

	// Unmarshal third parameter as an integer.
	var stakeDiff int64
	err = json.Unmarshal(params[2], &stakeDiff)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	// Unmarshal fourth parameter as a slice.
	var tickets []string
	err = json.Unmarshal(params[3], &tickets)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	t := make([]*chainhash.Hash, len(tickets))

	for i, ticketHashStr := range tickets {
		ticketHash, err := chainhash.NewHashFromStr(ticketHashStr)
		if err != nil {
			return nil, 0, 0, nil, err
		}

		t[i] = ticketHash
	}

	return sha, bh, stakeDiff, t, nil
}

// parseStakeDifficultyNtfnParams parses out the list of block hash, block
// height, and stake difficulty from a WinningTickets notification.
func parseStakeDifficultyNtfnParams(params []json.RawMessage) (
	*chainhash.Hash,
	int64,
	int64,
	error) {

	if len(params) != 3 {
		return nil, 0, 0, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	var blockHashStr string
	err := json.Unmarshal(params[0], &blockHashStr)
	if err != nil {
		return nil, 0, 0, err
	}

	// Create ShaHash from block sha string.
	bHash, err := chainhash.NewHashFromStr(blockHashStr)
	if err != nil {
		return nil, 0, 0, err
	}

	// Unmarshal second parameter as an integer.
	var blockHeight int32
	err = json.Unmarshal(params[1], &blockHeight)
	if err != nil {
		return nil, 0, 0, err
	}
	bHeight := int64(blockHeight)

	// Unmarshal third parameter as an integer.
	var stakeDiff int64
	err = json.Unmarshal(params[2], &stakeDiff)
	if err != nil {
		return nil, 0, 0, err
	}

	return bHash, bHeight, stakeDiff, nil
}

// parseTxAcceptedNtfnParams parses out the transaction hash and total amount
// from the parameters of a txaccepted notification.
func parseTxAcceptedNtfnParams(params []json.RawMessage) (*chainhash.Hash,
	dcrutil.Amount, error) {

	if len(params) != 2 {
		return nil, 0, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	var txHashStr string
	err := json.Unmarshal(params[0], &txHashStr)
	if err != nil {
		return nil, 0, err
	}

	// Unmarshal second parameter as a floating point number.
	var famt float64
	err = json.Unmarshal(params[1], &famt)
	if err != nil {
		return nil, 0, err
	}

	// Bounds check amount.
	amt, err := dcrutil.NewAmount(famt)
	if err != nil {
		return nil, 0, err
	}

	// Decode string encoding of transaction sha.
	txHash, err := chainhash.NewHashFromStr(txHashStr)
	if err != nil {
		return nil, 0, err
	}

	return txHash, amt, nil
}

// parseTxAcceptedVerboseNtfnParams parses out details about a raw transaction
// from the parameters of a txacceptedverbose notification.
func parseTxAcceptedVerboseNtfnParams(params []json.RawMessage) (*dcrjson.TxRawResult,
	error) {

	if len(params) != 1 {
		return nil, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a raw transaction result object.
	var rawTx dcrjson.TxRawResult
	err := json.Unmarshal(params[0], &rawTx)
	if err != nil {
		return nil, err
	}

	// TODO: change txacceptedverbose notification callbacks to use nicer
	// types for all details about the transaction (i.e. decoding hashes
	// from their string encoding).
	return &rawTx, nil
}

// parseBtcdConnectedNtfnParams parses out the connection status of dcrd
// and dcrwallet from the parameters of a btcdconnected notification.
func parseBtcdConnectedNtfnParams(params []json.RawMessage) (bool, error) {
	if len(params) != 1 {
		return false, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a boolean.
	var connected bool
	err := json.Unmarshal(params[0], &connected)
	if err != nil {
		return false, err
	}

	return connected, nil
}

// parseAccountBalanceNtfnParams parses out the account name, total balance,
// and whether or not the balance is confirmed or unconfirmed from the
// parameters of an accountbalance notification.
func parseAccountBalanceNtfnParams(params []json.RawMessage) (account string,
	balance dcrutil.Amount, confirmed bool, err error) {

	if len(params) != 3 {
		return "", 0, false, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	err = json.Unmarshal(params[0], &account)
	if err != nil {
		return "", 0, false, err
	}

	// Unmarshal second parameter as a floating point number.
	var fbal float64
	err = json.Unmarshal(params[1], &fbal)
	if err != nil {
		return "", 0, false, err
	}

	// Unmarshal third parameter as a boolean.
	err = json.Unmarshal(params[2], &confirmed)
	if err != nil {
		return "", 0, false, err
	}

	// Bounds check amount.
	bal, err := dcrutil.NewAmount(fbal)
	if err != nil {
		return "", 0, false, err
	}

	return account, bal, confirmed, nil
}

// parseTicketPurchasedNtfnParams parses out the ticket hash and amount
// from a recent ticket purchase in the wallet.
func parseTicketPurchasedNtfnParams(params []json.RawMessage) (txHash *chainhash.Hash,
	amount dcrutil.Amount, err error) {

	if len(params) != 2 {
		return nil, 0, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string and convert to hash.
	var th string
	err = json.Unmarshal(params[0], &th)
	if err != nil {
		return nil, 0, err
	}
	thHash, err := chainhash.NewHashFromStr(th)
	if err != nil {
		return nil, 0, err
	}

	// Unmarshal second parameter as an int64.
	var amt int64
	err = json.Unmarshal(params[1], &amt)
	if err != nil {
		return nil, 0, err
	}

	return thHash, dcrutil.Amount(amt), nil
}

// parseVoteCreatedNtfnParams parses out the hash, block hash, block height,
// ticket input hash, and votebits from a newly created SSGen in wallet.
// from a recent ticket purchase in the wallet.
func parseVoteCreatedNtfnParams(params []json.RawMessage) (txHash *chainhash.Hash,
	blockHash *chainhash.Hash,
	height int32,
	sstxIn *chainhash.Hash,
	voteBits uint16,
	err error) {

	if len(params) != 5 {
		return nil, nil, 0, nil, 0, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string and convert to hash.
	var th string
	err = json.Unmarshal(params[0], &th)
	if err != nil {
		return nil, nil, 0, nil, 0, err
	}
	thHash, err := chainhash.NewHashFromStr(th)
	if err != nil {
		return nil, nil, 0, nil, 0, err
	}

	// Unmarshal second parameter as a string and convert to hash.
	var bh string
	err = json.Unmarshal(params[1], &bh)
	if err != nil {
		return nil, nil, 0, nil, 0, err
	}
	bhHash, err := chainhash.NewHashFromStr(bh)
	if err != nil {
		return nil, nil, 0, nil, 0, err
	}

	// Unmarshal third parameter as an int32.
	var h int32
	err = json.Unmarshal(params[2], &h)
	if err != nil {
		return nil, nil, 0, nil, 0, err
	}

	// Unmarshal fourth parameter as a string and convert to hash.
	var ss string
	err = json.Unmarshal(params[3], &ss)
	if err != nil {
		return nil, nil, 0, nil, 0, err
	}
	ssHash, err := chainhash.NewHashFromStr(ss)
	if err != nil {
		return nil, nil, 0, nil, 0, err
	}

	// Unmarshal fifth parameter as a uint16.
	var vb uint16
	err = json.Unmarshal(params[4], &vb)
	if err != nil {
		return nil, nil, 0, nil, 0, err
	}

	return thHash, bhHash, h, ssHash, vb, nil
}

// parseRevocationCreatedNtfnParams parses out the hash and ticket input hash
// from a newly created SSGen in wallet.
func parseRevocationCreatedNtfnParams(params []json.RawMessage) (txHash *chainhash.Hash,
	sstxIn *chainhash.Hash, err error) {

	if len(params) != 2 {
		return nil, nil, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string and convert to hash.
	var th string
	err = json.Unmarshal(params[0], &th)
	if err != nil {
		return nil, nil, err
	}
	thHash, err := chainhash.NewHashFromStr(th)
	if err != nil {
		return nil, nil, err
	}

	// Unmarshal second parameter as a string and convert to hash.
	var ss string
	err = json.Unmarshal(params[1], &ss)
	if err != nil {
		return nil, nil, err
	}
	ssHash, err := chainhash.NewHashFromStr(ss)
	if err != nil {
		return nil, nil, err
	}

	return thHash, ssHash, nil
}

// parseWalletLockStateNtfnParams parses out the account name and locked
// state of an account from the parameters of a walletlockstate notification.
func parseWalletLockStateNtfnParams(params []json.RawMessage) (account string,
	locked bool, err error) {

	if len(params) != 2 {
		return "", false, wrongNumParams(len(params))
	}

	// Unmarshal first parameter as a string.
	err = json.Unmarshal(params[0], &account)
	if err != nil {
		return "", false, err
	}

	// Unmarshal second parameter as a boolean.
	err = json.Unmarshal(params[1], &locked)
	if err != nil {
		return "", false, err
	}

	return account, locked, nil
}

// FutureNotifyBlocksResult is a future promise to deliver the result of a
// NotifyBlocksAsync RPC invocation (or an applicable error).
type FutureNotifyBlocksResult chan *response

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r FutureNotifyBlocksResult) Receive() error {
	_, err := receiveFuture(r)
	return err
}

// NotifyBlocksAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See NotifyBlocks for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyBlocksAsync() FutureNotifyBlocksResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return newFutureError(ErrWebsocketsRequired)
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return newNilFutureResult()
	}

	cmd := dcrjson.NewNotifyBlocksCmd()
	return c.sendCmd(cmd)
}

// NotifyBlocks registers the client to receive notifications when blocks are
// connected and disconnected from the main chain.  The notifications are
// delivered to the notification handlers associated with the client.  Calling
// this function has no effect if there are no notification handlers and will
// result in an error if the client is configured to run in HTTP POST mode.
//
// The notifications delivered as a result of this call will be via one of
// OnBlockConnected or OnBlockDisconnected.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyBlocks() error {
	return c.NotifyBlocksAsync().Receive()
}

// FutureNotifyWinningTicketsResult is a future promise to deliver the result of a
// NotifyWinningTicketsAsync RPC invocation (or an applicable error).
type FutureNotifyWinningTicketsResult chan *response

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r FutureNotifyWinningTicketsResult) Receive() error {
	_, err := receiveFuture(r)
	return err
}

// NotifyWinningTicketsAsync returns an instance of a type that can be used
// to  get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See NotifyWinningTickets for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyWinningTicketsAsync() FutureNotifyWinningTicketsResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return newFutureError(ErrWebsocketsRequired)
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return newNilFutureResult()
	}

	cmd := dcrjson.NewNotifyWinningTicketsCmd()

	return c.sendCmd(cmd)
}

// NotifyWinningTickets registers the client to receive notifications when
// blocks are connected to the main chain and tickets are spent or missed.  The
// notifications are delivered to the notification handlers associated with the
// client.  Calling this function has no effect if there are no notification
// handlers and will result in an error if the client is configured to run in HTTP
// POST mode.
//
// The notifications delivered as a result of this call will be those from
// OnWinningTickets.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyWinningTickets() error {
	return c.NotifyWinningTicketsAsync().Receive()
}

// FutureNotifySpentAndMissedTicketsResult is a future promise to deliver the result of a
// NotifySpentAndMissedTicketsAsync RPC invocation (or an applicable error).
type FutureNotifySpentAndMissedTicketsResult chan *response

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r FutureNotifySpentAndMissedTicketsResult) Receive() error {
	_, err := receiveFuture(r)
	return err
}

// NotifySpentAndMissedTicketsAsync returns an instance of a type that can be used
// to  get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See NotifySpentAndMissedTickets for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifySpentAndMissedTicketsAsync() FutureNotifySpentAndMissedTicketsResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return newFutureError(ErrWebsocketsRequired)
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return newNilFutureResult()
	}

	cmd := dcrjson.NewNotifySpentAndMissedTicketsCmd()

	return c.sendCmd(cmd)
}

// NotifySpentAndMissedTickets registers the client to receive notifications when
// blocks are connected to the main chain and tickets are spent or missed.  The
// notifications are delivered to the notification handlers associated with the
// client.  Calling this function has no effect if there are no notification
// handlers and will result in an error if the client is configured to run in HTTP
// POST mode.
//
// The notifications delivered as a result of this call will be those from
// OnSpentAndMissedTickets.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifySpentAndMissedTickets() error {
	return c.NotifySpentAndMissedTicketsAsync().Receive()
}

// FutureNotifyNewTicketsResult is a future promise to deliver the result of a
// NotifyNewTicketsAsync RPC invocation (or an applicable error).
type FutureNotifyNewTicketsResult chan *response

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r FutureNotifyNewTicketsResult) Receive() error {
	_, err := receiveFuture(r)
	return err
}

// NotifyNewTicketsAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See NotifyNewTickets for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyNewTicketsAsync() FutureNotifyNewTicketsResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return newFutureError(ErrWebsocketsRequired)
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return newNilFutureResult()
	}

	cmd := dcrjson.NewNotifyNewTicketsCmd()

	return c.sendCmd(cmd)
}

// NotifyNewTickets registers the client to receive notifications when blocks are
// connected to the main chain and new tickets have matured.  The notifications are
// delivered to the notification handlers associated with the client.  Calling
// this function has no effect if there are no notification handlers and will
// result in an error if the client is configured to run in HTTP POST mode.
//
// The notifications delivered as a result of this call will be via OnNewTickets.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyNewTickets() error {
	return c.NotifyNewTicketsAsync().Receive()
}

// FutureNotifyStakeDifficultyResult is a future promise to deliver the result of a
// NotifyStakeDifficultyAsync RPC invocation (or an applicable error).
type FutureNotifyStakeDifficultyResult chan *response

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r FutureNotifyStakeDifficultyResult) Receive() error {
	_, err := receiveFuture(r)
	return err
}

// NotifyStakeDifficultyAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See NotifyStakeDifficulty for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyStakeDifficultyAsync() FutureNotifyStakeDifficultyResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return newFutureError(ErrWebsocketsRequired)
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return newNilFutureResult()
	}

	cmd := dcrjson.NewNotifyStakeDifficultyCmd()

	return c.sendCmd(cmd)
}

// NotifyStakeDifficulty registers the client to receive notifications when
// blocks are connected to the main chain and stake difficulty is updated.  The
// notifications are delivered to the notification handlers associated with the
// client.  Calling this function has no effect if there are no notification
// handlers and will result in an error if the client is configured to run in
// HTTP POST mode.
//
// The notifications delivered as a result of this call will be via
// OnStakeDifficulty.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyStakeDifficulty() error {
	return c.NotifyStakeDifficultyAsync().Receive()
}

// FutureNotifyNewTransactionsResult is a future promise to deliver the result
// of a NotifyNewTransactionsAsync RPC invocation (or an applicable error).
type FutureNotifyNewTransactionsResult chan *response

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r FutureNotifyNewTransactionsResult) Receive() error {
	_, err := receiveFuture(r)
	return err
}

// NotifyNewTransactionsAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See NotifyNewTransactionsAsync for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyNewTransactionsAsync(verbose bool) FutureNotifyNewTransactionsResult {
	// Not supported in HTTP POST mode.
	if c.config.HTTPPostMode {
		return newFutureError(ErrWebsocketsRequired)
	}

	// Ignore the notification if the client is not interested in
	// notifications.
	if c.ntfnHandlers == nil {
		return newNilFutureResult()
	}

	cmd := dcrjson.NewNotifyNewTransactionsCmd(&verbose)
	return c.sendCmd(cmd)
}

// NotifyNewTransactions registers the client to receive notifications every
// time a new transaction is accepted to the memory pool.  The notifications are
// delivered to the notification handlers associated with the client.  Calling
// this function has no effect if there are no notification handlers and will
// result in an error if the client is configured to run in HTTP POST mode.
//
// The notifications delivered as a result of this call will be via one of
// OnTxAccepted (when verbose is false) or OnTxAcceptedVerbose (when verbose is
// true).
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) NotifyNewTransactions(verbose bool) error {
	return c.NotifyNewTransactionsAsync(verbose).Receive()
}

// FutureLoadTxFilterResult is a future promise to deliver the result
// of a LoadTxFilterAsync RPC invocation (or an applicable error).
type FutureLoadTxFilterResult chan *response

// Receive waits for the response promised by the future and returns an error
// if the registration was not successful.
func (r FutureLoadTxFilterResult) Receive() error {
	_, err := receiveFuture(r)
	return err
}

// LoadTxFilterAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See LoadTxFilter for the blocking version and more details.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) LoadTxFilterAsync(reload bool, addresses []dcrutil.Address,
	outPoints []wire.OutPoint) FutureLoadTxFilterResult {

	addrStrs := make([]string, len(addresses))
	for i, a := range addresses {
		addrStrs[i] = a.EncodeAddress()
	}
	outPointObjects := make([]dcrjson.OutPoint, len(outPoints))
	for i := range outPoints {
		outPointObjects[i] = dcrjson.OutPoint{
			Hash:  outPoints[i].Hash.String(),
			Index: outPoints[i].Index,
			Tree:  outPoints[i].Tree,
		}
	}

	cmd := dcrjson.NewLoadTxFilterCmd(reload, addrStrs, outPointObjects)
	return c.sendCmd(cmd)
}

// LoadTxFilter loads, reloads, or adds data to a websocket client's transaction
// filter.  The filter is consistently updated based on inspected transactions
// during mempool acceptance, block acceptance, and for all rescanned blocks.
//
// NOTE: This is a dcrd extension and requires a websocket connection.
func (c *Client) LoadTxFilter(reload bool, addresses []dcrutil.Address, outPoints []wire.OutPoint) error {
	return c.LoadTxFilterAsync(reload, addresses, outPoints).Receive()
}
