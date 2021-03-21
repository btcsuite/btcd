// Copyright (c) 2014-2017 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Copyright (c) 2021 Dash Core Group
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"encoding/json"
	"fmt"

	"github.com/dashevo/dashd-go/btcjson"
)

// ----------------------------- bls generate -----------------------------

// FutureGetQuorumInfoResult is a future promise to deliver the result of a
// QuorumInfoAsync RPC invocation (or an applicable error).
type FutureGetBLSResult struct {
	client   *Client
	Response chan *response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetBLSResult) Receive() (*btcjson.BLSResult, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	var result btcjson.BLSResult
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// BLSGenerateAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
func (c *Client) BLSGenerateAsync() FutureGetBLSResult {
	cmd := btcjson.NewBLSGenerate()

	return FutureGetBLSResult{
		client:   c,
		Response: c.sendCmd(cmd),
	}
}

// BLSGenerate returns a bls generate result
func (c *Client) BLSGenerate() (*btcjson.BLSResult, error) {
	return c.BLSGenerateAsync().Receive()
}

// ----------------------------- bls fromsecret -----------------------------

// BLSGenerateAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
func (c *Client) BLSFromSecretAsync(secret string) FutureGetBLSResult {
	cmd := btcjson.NewBLSFromSecret(secret)

	return FutureGetBLSResult{
		client:   c,
		Response: c.sendCmd(cmd),
	}
}

// BLSGenerate returns a bls generate result
func (c *Client) BLSFromSecret(secret string) (*btcjson.BLSResult, error) {
	return c.BLSFromSecretAsync(secret).Receive()
}

// ----------------------------- quorum sign -----------------------------

// FutureGetQuorumSignResult is a future promise to deliver the result of a
// QuorumSignAsync RPC invocation (or an applicable error).
type FutureGetQuorumSignResult struct {
	client   *Client
	Response chan *response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetQuorumSignResult) Receive() (*btcjson.QuorumSignResultWithBool, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	var quorumSignResult btcjson.QuorumSignResultWithBool
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
func (c *Client) QuorumSign(quorumType btcjson.LLMQType, requestID, messageHash, quorumHash string, submit bool) (*btcjson.QuorumSignResultWithBool, error) {
	return c.QuorumSignAsync(quorumType, requestID, messageHash, quorumHash, submit).Receive()
}

// QuorumSignSubmit calls QuorumSign but only returns a boolean to match dash-cli
func (c *Client) QuorumSignSubmit(quorumType btcjson.LLMQType, requestID, messageHash, quorumHash string) (bool, error) {
	r, err := c.QuorumSignAsync(quorumType, requestID, messageHash, quorumHash, true).Receive()
	if err != nil {
		return false, err
	}
	return r.Result, nil
}

// ----------------------------- quorum list -----------------------------

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

// ----------------------------- quorum selectuorum -----------------------------

// FutureGetQuorumSelectQuorumResult is a future promise to deliver the result of a
// QuorumSelectQuorumtAsync RPC invocation (or an applicable error).
type FutureGetQuorumSelectQuorumResult struct {
	client   *Client
	Response chan *response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetQuorumSelectQuorumResult) Receive() (*btcjson.QuorumSelectQuorumResult, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	var quorumSelectQuorumResult btcjson.QuorumSelectQuorumResult
	err = json.Unmarshal(res, &quorumSelectQuorumResult)
	if err != nil {
		return nil, err
	}

	return &quorumSelectQuorumResult, nil
}

// QuorumSelectQuorumAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) QuorumSelectQuorumAsync(quorumType btcjson.LLMQType, requestID string) FutureGetQuorumSelectQuorumResult {
	cmd := btcjson.NewQuorumSelectQuorumCmd(quorumType, requestID)

	return FutureGetQuorumSelectQuorumResult{
		client:   c,
		Response: c.sendCmd(cmd),
	}
}

// QuorumSelectQuorum returns a quorum list result containing a lost of quorums
func (c *Client) QuorumSelectQuorum(quorumType btcjson.LLMQType, requestID string) (*btcjson.QuorumSelectQuorumResult, error) {
	return c.QuorumSelectQuorumAsync(quorumType, requestID).Receive()
}

// ----------------------------- quorum dkgstatus -----------------------------

// FutureGetQuorumDKGStatusResult is a future promise to deliver the result of a
// QuorumDKGStatusAsync RPC invocation (or an applicable error).
type FutureGetQuorumDKGStatusResult struct {
	client      *Client
	Response    chan *response
	detailLevel btcjson.DetailLevel
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetQuorumDKGStatusResult) Receive() (interface{}, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	switch r.detailLevel {
	case btcjson.DetailLevelCounts:
		var result btcjson.QuorumDKGStatusCountsResult
		err = json.Unmarshal(res, &result)
		if err != nil {
			return nil, err
		}
		return &result, nil
	case btcjson.DetailLevelIndexes:
		var result btcjson.QuorumDKGStatusIndexesResult
		err = json.Unmarshal(res, &result)
		if err != nil {
			return nil, err
		}
		return &result, nil
	case btcjson.DetailLevelMembersProTxHashes:
		var result btcjson.QuorumDKGStatusMembersProTxHashesResult
		err = json.Unmarshal(res, &result)
		if err != nil {
			return nil, err
		}
		return &result, nil

	}
	return nil, fmt.Errorf("unknown detail level")
}

// QuorumDKGStatusAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) QuorumDKGStatusAsync(detailLevel btcjson.DetailLevel) FutureGetQuorumDKGStatusResult {
	cmd := btcjson.NewQuorumDKGStatusCmd(detailLevel)

	return FutureGetQuorumDKGStatusResult{
		client:      c,
		Response:    c.sendCmd(cmd),
		detailLevel: detailLevel,
	}
}

// QuorumDKGStatus returns a quorum DKGStatus result
func (c *Client) QuorumDKGStatus(detailLevel btcjson.DetailLevel) (interface{}, error) {
	return c.QuorumDKGStatusAsync(detailLevel).Receive()
}

// QuorumDKGStatusCounts returns a quorum DKGStatus with only detail level of counts
func (c *Client) QuorumDKGStatusCounts() (*btcjson.QuorumDKGStatusCountsResult, error) {
	r, err := c.QuorumDKGStatusAsync(btcjson.DetailLevelCounts).Receive()
	if err != nil {
		return nil, err
	}
	return r.(*btcjson.QuorumDKGStatusCountsResult), nil
}

// ----------------------------- quorum memberof -----------------------------

// FutureGetQuorumMemberOfResult is a future promise to deliver the result of a
// QuorumMemberOfAsync RPC invocation (or an applicable error).
type FutureGetQuorumMemberOfResult struct {
	client   *Client
	Response chan *response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetQuorumMemberOfResult) Receive() ([]btcjson.QuorumMemberOfResult, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	var quorumMemberOfResult []btcjson.QuorumMemberOfResult
	err = json.Unmarshal(res, &quorumMemberOfResult)
	if err != nil {
		return nil, err
	}

	return quorumMemberOfResult, nil
}

// QuorumMemberOfAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
func (c *Client) QuorumMemberOfAsync(proTxHash string, scanQuorumsCount int) FutureGetQuorumMemberOfResult {

	cmd := btcjson.NewQuorumMemberOfCmd(proTxHash, scanQuorumsCount)

	return FutureGetQuorumMemberOfResult{
		client:   c,
		Response: c.sendCmd(cmd),
	}
}

// QuorumMemberOf returns a quorum MemberOf result
func (c *Client) QuorumMemberOf(proTxHash string, scanQuorumsCount int) ([]btcjson.QuorumMemberOfResult, error) {
	return c.QuorumMemberOfAsync(proTxHash, scanQuorumsCount).Receive()
}

// ----------------------------- quorum getrecsig -----------------------------

// FutureGetQuorumGetRecSigResult is a future promise to deliver the result of a
// QuorumMemberOfAsync RPC invocation (or an applicable error).
type FutureGetQuorumGetRecSigResult struct {
	client   *Client
	Response chan *response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetQuorumGetRecSigResult) Receive() ([]btcjson.QuorumSignResult, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	var quorumSignResult []btcjson.QuorumSignResult
	err = json.Unmarshal(res, &quorumSignResult)
	if err != nil {
		return nil, err
	}

	return quorumSignResult, nil
}

// QuorumGetRecSigAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
func (c *Client) QuorumGetRecSigAsync(quorumType btcjson.LLMQType, requestID, messageHash string) FutureGetQuorumGetRecSigResult {
	cmd := btcjson.NewQuorumGetRecSig(quorumType, requestID, messageHash)

	return FutureGetQuorumGetRecSigResult{
		client:   c,
		Response: c.sendCmd(cmd),
	}
}

// QuorumGetRecSig returns a quorum MemberOf result
func (c *Client) QuorumGetRecSig(quorumType btcjson.LLMQType, requestID, messageHash string) ([]btcjson.QuorumSignResult, error) {
	return c.QuorumGetRecSigAsync(quorumType, requestID, messageHash).Receive()
}

// ----------------------------- quorum hasrecsig -----------------------------

// FutureGetQuorumGetBoolResult is a future promise to deliver the result of a
// QuorumMemberOfAsync RPC invocation (or an applicable error).
type FutureGetQuorumGetBoolResult struct {
	client   *Client
	Response chan *response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetQuorumGetBoolResult) Receive() (bool, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return false, err
	}

	var bl bool
	err = json.Unmarshal(res, &bl)
	if err != nil {
		return false, err
	}

	return bl, nil
}

// QuorumHasRecSigAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
func (c *Client) QuorumHasRecSigAsync(quorumType btcjson.LLMQType, requestID, messageHash string) FutureGetQuorumGetBoolResult {
	cmd := btcjson.NewQuorumHasRecSig(quorumType, requestID, messageHash)

	return FutureGetQuorumGetBoolResult{
		client:   c,
		Response: c.sendCmd(cmd),
	}
}

// QuorumHasRecSig returns a quorum MemberOf result
func (c *Client) QuorumHasRecSig(quorumType btcjson.LLMQType, requestID, messageHash string) (bool, error) {
	return c.QuorumHasRecSigAsync(quorumType, requestID, messageHash).Receive()
}

// ----------------------------- quorum isconflicting -----------------------------

// QuorumHasRecSigAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
func (c *Client) QuorumIsConflictingAsync(quorumType btcjson.LLMQType, requestID, messageHash string) FutureGetQuorumGetBoolResult {
	cmd := btcjson.NewQuorumIsConflicting(quorumType, requestID, messageHash)

	return FutureGetQuorumGetBoolResult{
		client:   c,
		Response: c.sendCmd(cmd),
	}
}

// QuorumHasRecSig returns a quorum MemberOf result
func (c *Client) QuorumIsConflicting(quorumType btcjson.LLMQType, requestID, messageHash string) (bool, error) {
	return c.QuorumIsConflictingAsync(quorumType, requestID, messageHash).Receive()
}
