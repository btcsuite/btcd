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

// FutureGetBLSResult is a future promise to deliver the result of a
// BLSGenerateAsync RPC invocation (or an applicable error).
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

// BLSFromSecretAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) BLSFromSecretAsync(secret string) FutureGetBLSResult {
	cmd := btcjson.NewBLSFromSecret(secret)

	return FutureGetBLSResult{
		client:   c,
		Response: c.sendCmd(cmd),
	}
}

// BLSFromSecret returns a bls generate result
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

// ----------------------------- quorum info -----------------------------

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

// QuorumIsConflictingAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) QuorumIsConflictingAsync(quorumType btcjson.LLMQType, requestID, messageHash string) FutureGetQuorumGetBoolResult {
	cmd := btcjson.NewQuorumIsConflicting(quorumType, requestID, messageHash)

	return FutureGetQuorumGetBoolResult{
		client:   c,
		Response: c.sendCmd(cmd),
	}
}

// QuorumIsConflicting returns a quorum isconflicting result
func (c *Client) QuorumIsConflicting(quorumType btcjson.LLMQType, requestID, messageHash string) (bool, error) {
	return c.QuorumIsConflictingAsync(quorumType, requestID, messageHash).Receive()
}

// ----------------------------- protx register -----------------------------

// FutureGetProTxStringResult is a future promise to deliver the result of a
// string RPC invocation (or an applicable error).
type FutureGetProTxStringResult struct {
	client   *Client
	Response chan *response
}

// Receive waits for the response promised by the future
func (r FutureGetProTxStringResult) Receive() (string, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return "", err
	}
	return string(res), nil
}

// ProTxRegisterAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) ProTxRegisterAsync(collateralHash string, collateralIndex int, ipAndPort, ownerAddress, operatorPubKey, votingAddress string, operatorReward float64, payoutAddress, feeSourceAddress string, submit bool) FutureGetProTxStringResult {
	cmd := btcjson.NewProTxRegisterCmd(collateralHash, collateralIndex, ipAndPort, ownerAddress, operatorPubKey, votingAddress, operatorReward, payoutAddress, feeSourceAddress, submit)
	return FutureGetProTxStringResult{client: c, Response: c.sendCmd(cmd)}
}

// ProTxRegister returns a protx register
func (c *Client) ProTxRegister(collateralHash string, collateralIndex int, ipAndPort, ownerAddress, operatorPubKey, votingAddress string, operatorReward float64, payoutAddress, feeSourceAddress string, submit bool) (string, error) {
	if operatorReward < 0 || operatorReward > 100 {
		return "", fmt.Errorf("operatorReward must be between 0.00 and 100.00")
	}
	return c.ProTxRegisterAsync(collateralHash, collateralIndex, ipAndPort, ownerAddress, operatorPubKey, votingAddress, operatorReward, payoutAddress, feeSourceAddress, submit).Receive()
}

// ----------------------------- protx register_fund -----------------------------

// ProTxRegisterFundAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) ProTxRegisterFundAsync(collateralAddress, ipAndPort, ownerAddress, operatorPubKey, votingAddress string, operatorReward float64, payoutAddress, fundAddress string, submit bool) FutureGetProTxStringResult {
	cmd := btcjson.NewProTxRegisterFundCmd(collateralAddress, ipAndPort, ownerAddress, operatorPubKey, votingAddress, operatorReward, payoutAddress, fundAddress, submit)
	return FutureGetProTxStringResult{client: c, Response: c.sendCmd(cmd)}
}

// ProTxRegisterFund returns a protx register_fund
func (c *Client) ProTxRegisterFund(collateralAddress, ipAndPort, ownerAddress, operatorPubKey, votingAddress string, operatorReward float64, payoutAddress, fundAddress string, submit bool) (string, error) {
	if operatorReward < 0 || operatorReward > 100 {
		return "", fmt.Errorf("operatorReward must be between 0.00 and 100.00")
	}
	return c.ProTxRegisterFundAsync(collateralAddress, ipAndPort, ownerAddress, operatorPubKey, votingAddress, operatorReward, payoutAddress, fundAddress, submit).Receive()
}

// ----------------------------- protx register_prepare -----------------------------

// FutureGetProTxRegisterPrepareResult is a future promise to deliver the result of a
// ProTxInfoAsync RPC invocation (or an applicable error).
type FutureGetProTxRegisterPrepareResult struct {
	client   *Client
	Response chan *response
}

// Receive waits for the response promised by the future
func (r FutureGetProTxRegisterPrepareResult) Receive() (*btcjson.ProTxRegisterPrepareResult, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	var result btcjson.ProTxRegisterPrepareResult
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// ProTxRegisterPrepareAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) ProTxRegisterPrepareAsync(collateralHash string, collateralIndex int, ipAndPort, ownerAddress, operatorPubKey, votingAddress string, operatorReward float64, payoutAddress, feeSourceAddress string) FutureGetProTxRegisterPrepareResult {
	cmd := btcjson.NewProTxRegisterPrepareCmd(collateralHash, collateralIndex, ipAndPort, ownerAddress, operatorPubKey, votingAddress, operatorReward, payoutAddress, feeSourceAddress)
	return FutureGetProTxRegisterPrepareResult{client: c, Response: c.sendCmd(cmd)}
}

// ProTxRegisterPrepare returns a protx register_prepare
func (c *Client) ProTxRegisterPrepare(collateralHash string, collateralIndex int, ipAndPort, ownerAddress, operatorPubKey, votingAddress string, operatorReward float64, payoutAddress, feeSourceAddress string) (*btcjson.ProTxRegisterPrepareResult, error) {
	return c.ProTxRegisterPrepareAsync(collateralHash, collateralIndex, ipAndPort, ownerAddress, operatorPubKey, votingAddress, operatorReward, payoutAddress, feeSourceAddress).Receive()
}

// ----------------------------- protx register_submit -----------------------------

// ProTxRegisterSubmitAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) ProTxRegisterSubmitAsync(tx, sig string) FutureGetProTxStringResult {
	cmd := btcjson.NewProTxRegisterSubmitCmd(tx, sig)
	return FutureGetProTxStringResult{client: c, Response: c.sendCmd(cmd)}
}

// ProTxRegisterSubmit returns a protx register_submit
func (c *Client) ProTxRegisterSubmit(tx, sig string) (string, error) {
	return c.ProTxRegisterSubmitAsync(tx, sig).Receive()
}

// ----------------------------- protx list -----------------------------

// FutureGetProTxListResult is a future promise to deliver the result of a
// ProTxListAsync RPC invocation (or an applicable error).
type FutureGetProTxListResult struct {
	client   *Client
	Response chan *response
	detailed bool
}

// Receive waits for the response promised by the future
func (r FutureGetProTxListResult) Receive() (interface{}, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	if r.detailed {
		var result []btcjson.ProTxInfoResult
		err = json.Unmarshal(res, &result)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	var result []string
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ProTxListAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) ProTxListAsync(cmdType btcjson.ProTxListType, detailed bool, height int) FutureGetProTxListResult {
	cmd := btcjson.NewProTxListCmd(cmdType, detailed, height)
	return FutureGetProTxListResult{client: c, Response: c.sendCmd(cmd), detailed: detailed}
}

// ProTxList returns a protx list result
func (c *Client) ProTxList(cmdType btcjson.ProTxListType, detailed bool, height int) (interface{}, error) {
	return c.ProTxListAsync(cmdType, detailed, height).Receive()
}

// ----------------------------- protx info -----------------------------

// FutureGetProTxInfoResult is a future promise to deliver the result of a
// ProTxInfoAsync RPC invocation (or an applicable error).
type FutureGetProTxInfoResult struct {
	client   *Client
	Response chan *response
}

// Receive waits for the response promised by the future
func (r FutureGetProTxInfoResult) Receive() (*btcjson.ProTxInfoResult, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	var result btcjson.ProTxInfoResult
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// ProTxInfoAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) ProTxInfoAsync(proTxHash string) FutureGetProTxInfoResult {
	cmd := btcjson.NewProTxInfoCmd(proTxHash)
	return FutureGetProTxInfoResult{client: c, Response: c.sendCmd(cmd)}
}

// ProTxInfo returns a protx info result
func (c *Client) ProTxInfo(proTxHash string) (*btcjson.ProTxInfoResult, error) {
	return c.ProTxInfoAsync(proTxHash).Receive()
}

// ----------------------------- protx update_service -----------------------------

// ProTxUpdateServiceAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) ProTxUpdateServiceAsync(proTxHash, ipAndPort, operatorPubKey, operatorPayoutAddress, feeSourceAddress string) FutureGetProTxStringResult {
	cmd := btcjson.NewProTxUpdateServiceCmd(proTxHash, ipAndPort, operatorPubKey, operatorPayoutAddress, feeSourceAddress)
	return FutureGetProTxStringResult{client: c, Response: c.sendCmd(cmd)}
}

// ProTxUpdateService returns a protx update_service result
func (c *Client) ProTxUpdateService(proTxHash, ipAndPort, operatorPubKey, operatorPayoutAddress, feeSourceAddress string) (string, error) {
	return c.ProTxUpdateServiceAsync(proTxHash, ipAndPort, operatorPubKey, operatorPayoutAddress, feeSourceAddress).Receive()
}

// ----------------------------- protx update_registrar -----------------------------

// ProTxUpdateRegistrarAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) ProTxUpdateRegistrarAsync(proTxHash, operatorPubKey, votingAddress, payoutAddress, feeSourceAddress string) FutureGetProTxStringResult {
	cmd := btcjson.NewProTxUpdateRegistrarCmd(proTxHash, operatorPubKey, votingAddress, payoutAddress, feeSourceAddress)
	return FutureGetProTxStringResult{client: c, Response: c.sendCmd(cmd)}
}

// ProTxUpdateRegistrar returns a protx update_registrar result
func (c *Client) ProTxUpdateRegistrar(proTxHash, operatorPubKey, votingAddress, payoutAddress, feeSourceAddress string) (string, error) {
	return c.ProTxUpdateRegistrarAsync(proTxHash, operatorPubKey, votingAddress, payoutAddress, feeSourceAddress).Receive()
}

// ----------------------------- protx revoke -----------------------------

// ProTxRevokeAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) ProTxRevokeAsync(proTxHash, operatorPrivateKey string, reason int, feeSourceAddress string) FutureGetProTxStringResult {
	cmd := btcjson.NewProTxRevokeCmd(proTxHash, operatorPrivateKey, reason, feeSourceAddress)
	return FutureGetProTxStringResult{client: c, Response: c.sendCmd(cmd)}
}

// ProTxRevoke returns a protx register_submit
func (c *Client) ProTxRevoke(proTxHash, operatorPrivateKey string, reason int, feeSourceAddress string) (string, error) {
	return c.ProTxRevokeAsync(proTxHash, operatorPrivateKey, reason, feeSourceAddress).Receive()
}

// ----------------------------- protx diff -----------------------------

// FutureGetProTxDiffResult is a future promise to deliver the result of a
// ProTxDiffAsync RPC invocation (or an applicable error).
type FutureGetProTxDiffResult struct {
	client   *Client
	Response chan *response
}

// Receive waits for the response promised by the future
func (r FutureGetProTxDiffResult) Receive() (*btcjson.ProTxDiffResult, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}
	var result btcjson.ProTxDiffResult
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// ProTxDiffAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) ProTxDiffAsync(baseBlock, block int) FutureGetProTxDiffResult {
	cmd := btcjson.NewProTxDiffCmd(baseBlock, block)
	return FutureGetProTxDiffResult{client: c, Response: c.sendCmd(cmd)}
}

// ProTxDiff returns a protx diff result
func (c *Client) ProTxDiff(baseBlock, block int) (*btcjson.ProTxDiffResult, error) {
	return c.ProTxDiffAsync(baseBlock, block).Receive()
}
