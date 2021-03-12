// Copyright (c) 2014-2017 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Copyright (c) 2021 Dash Core Group
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"encoding/json"

	"github.com/dashevo/dashd-go/btcjson"
)

// FutureGetQuorumSignResult is a future promise to deliver the result of a
// QuorumSignAsync RPC invocation (or an applicable error).
type FutureGetQuorumSignResult struct {
	client   *Client
	Response chan *response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetQuorumSignResult) Receive() (*btcjson.QuorumSignResult, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

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
func (c *Client) QuorumSignAsync(quorumType btcjson.LLMQType, requestID, messageHash, quorumHash string, submit bool) FutureGetQuorumSignResult {
	cmd := btcjson.NewQuorumSignCmd(quorumType, requestID, messageHash, quorumHash, submit)

	return FutureGetQuorumSignResult{
		client:   c,
		Response: c.sendCmd(cmd),
	}
}

// QuorumSign returns a quorum sign result containing a signature signed by the quorum in question.
func (c *Client) QuorumSign(quorumType btcjson.LLMQType, requestID, messageHash, quorumHash string, submit bool) (*btcjson.QuorumSignResult, error) {
	return c.QuorumSignAsync(quorumType, requestID, messageHash, quorumHash, submit).Receive()
}

// FutureGetQuorumInfoResult is a future promise to deliver the result of a
// QuorumInfoAsync RPC invocation (or an applicable error).
type FutureGetQuorumInfoResult struct {
	client   *Client
	Response chan *response
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
func (c *Client) QuorumInfoAsync(quorumType btcjson.LLMQType, quorumHash string, includeSkShare bool) FutureGetQuorumInfoResult {

	cmd := btcjson.NewQuorumInfoCmd(quorumType, quorumHash, includeSkShare)

	return FutureGetQuorumInfoResult{
		client:   c,
		Response: c.sendCmd(cmd),
	}
}

// QuorumInfo returns a quorum info result
func (c *Client) QuorumInfo(quorumType btcjson.LLMQType, quorumHash string, includeSkShare bool) (*btcjson.QuorumInfoResult, error) {
	return c.QuorumInfoAsync(quorumType, quorumHash, includeSkShare).Receive()
}

// ----------------------------- quorum list -----------------------------

// FutureGetQuorumListResult is a future promise to deliver the result of a
// QuorumListAsync RPC invocation (or an applicable error).
type FutureGetQuorumListResult struct {
	client   *Client
	Response chan *response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetQuorumListResult) Receive() (*btcjson.QuorumListResult, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	var quorumListResult btcjson.QuorumListResult
	err = json.Unmarshal(res, &quorumListResult)
	if err != nil {
		return nil, err
	}

	return &quorumListResult, nil
}

// QuorumListAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) QuorumListAsync() FutureGetQuorumListResult {
	cmd := btcjson.NewQuorumListCmd()

	return FutureGetQuorumListResult{
		client:   c,
		Response: c.sendCmd(cmd),
	}
}

// QuorumList returns a quorum list result containing a lost of quorums
func (c *Client) QuorumList() (*btcjson.QuorumListResult, error) {
	return c.QuorumListAsync().Receive()
}
