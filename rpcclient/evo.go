// Copyright (c) 2014-2017 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"encoding/json"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// FutureGetQuorumSignResult is a future promise to deliver the result of a
// QuorumSignAsync RPC invocation (or an applicable error).
type FutureGetQuorumSignResult struct {
	client   *Client
	quorumType btcjson.LLMQType
	messageHash     string
	requestId string
	quorumHash string
	Response chan *response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetQuorumSignResult) Receive() (*chainhash.Hash, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	// Unmarshal as an array of getaddednodeinfo result objects.
	var quorumSignResultHash chainhash.Hash
	err = json.Unmarshal(res, &quorumSignResultHash)
	if err != nil {
		return nil, err
	}

	return &quorumSignResultHash, nil
}

// QuorumSignAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetBestBlockHash for the blocking version and more details.
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
		client:   c,
		quorumType: quorumType,
		messageHash: messageHashString,
		requestId: requestIdString,
		quorumHash: quorumHashString,
		Response: c.sendCmd(cmd),
	}
}

// GetBestBlockHash returns the hash of the best block in the longest block
// chain.
func (c *Client) QuorumSign(quorumType btcjson.LLMQType, requestId *chainhash.Hash, messageHash *chainhash.Hash, quorumHash *chainhash.Hash, submit bool) (*chainhash.Hash, error) {
	return c.QuorumSignAsync(quorumType, requestId, messageHash, quorumHash, submit).Receive()
}