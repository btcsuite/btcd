// Copyright (c) 2014-2017 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Copyright (c) 2021 Dash Core Group
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"encoding/json"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/dashevo/dashd-go/btcjson"
)

// FutureGetQuorumSignResult is a future promise to deliver the result of a
// QuorumSignAsync RPC invocation (or an applicable error).
type FutureGetQuorumSignResult struct {
	client      *Client
	quorumType  btcjson.LLMQType
	messageHash string
	requestId   string
	quorumHash  string
	Response    chan *response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetQuorumSignResult) Receive() (*btcjson.QuorumSignResult, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	// Unmarshal as a Quorum Info Result
	var quorumSignResult btcjson.QuorumSignResult
	err = json.Unmarshal(res, &quorumSignResult)
	if err != nil {
		return nil, err
	}

	return &quorumSignResult, nil
}

// QuorumSignAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
func (c *Client) QuorumSignAsync(quorumType btcjson.LLMQType, requestId *chainhash.Hash, messageHash *chainhash.Hash, quorumHash *chainhash.Hash, submit bool) FutureGetQuorumSignResult {

	messageHashString := ""
	if messageHash != nil {
		messageHashString = messageHash.String()
	}

	requestIdString := ""
	if requestId != nil {
		requestIdString = requestId.String()
	}

	quorumHashString := ""
	if quorumHash != nil {
		quorumHashString = quorumHash.String()
	}

	cmd := btcjson.NewQuorumSignCmd(quorumType, requestIdString, messageHashString, quorumHashString, submit)

	return FutureGetQuorumSignResult{
		client:      c,
		quorumType:  quorumType,
		messageHash: messageHashString,
		requestId:   requestIdString,
		quorumHash:  quorumHashString,
		Response:    c.sendCmd(cmd),
	}
}

// QuorumSign returns a quorum sign result containing a signature signed by the quorum in question.
func (c *Client) QuorumSign(quorumType btcjson.LLMQType, requestId *chainhash.Hash, messageHash *chainhash.Hash, quorumHash *chainhash.Hash, submit bool) (*btcjson.QuorumSignResult, error) {
	return c.QuorumSignAsync(quorumType, requestId, messageHash, quorumHash, submit).Receive()
}

// FutureGetQuorumInfoResult is a future promise to deliver the result of a
// QuorumInfoAsync RPC invocation (or an applicable error).
type FutureGetQuorumInfoResult struct {
	client     *Client
	quorumType btcjson.LLMQType
	quorumHash string
	Response   chan *response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetQuorumInfoResult) Receive() (*btcjson.QuorumInfoResult, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	// Unmarshal as a Quorum Info Result
	var quorumInfoResult btcjson.QuorumInfoResult
	err = json.Unmarshal(res, &quorumInfoResult)
	if err != nil {
		return nil, err
	}

	return &quorumInfoResult, nil
}

// QuorumInfoAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
func (c *Client) QuorumInfoAsync(quorumType btcjson.LLMQType, quorumHash *chainhash.Hash, includeSkShare bool) FutureGetQuorumInfoResult {

	quorumHashString := ""
	if quorumHash != nil {
		quorumHashString = quorumHash.String()
	}

	cmd := btcjson.NewQuorumInfoCmd(quorumType, quorumHashString, includeSkShare)

	return FutureGetQuorumInfoResult{
		client:     c,
		quorumType: quorumType,
		quorumHash: quorumHashString,
		Response:   c.sendCmd(cmd),
	}
}

// QuorumSign returns a quorum sign result containing a signature signed by the quorum in question.
func (c *Client) QuorumInfo(quorumType btcjson.LLMQType, quorumHash *chainhash.Hash, includeSkShare bool) (*btcjson.QuorumInfoResult, error) {
	return c.QuorumInfoAsync(quorumType, quorumHash, includeSkShare).Receive()
}
