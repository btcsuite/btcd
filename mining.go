// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcrpcclient

import (
	"encoding/hex"
	"fmt"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcutil"
)

// FutureGetGenerateResult is a future promise to deliver the result of a
// GetGenerateAsync RPC invocation (or an applicable error).
type FutureGetGenerateResult chan *futureResult

// Receive waits for the response promised by the future and returns true if the
// server is set to mine, otherwise false.
func (r FutureGetGenerateResult) Receive() (bool, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return false, err
	}

	// Ensure the returned data is the expected type.
	result, ok := reply.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected response type for "+
			"getgenerate: %T\n", reply)
	}

	return result, nil
}

// GetGenerateAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetGenerate for the blocking version and more details.
func (c *Client) GetGenerateAsync() FutureGetGenerateResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetGenerateCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetGenerate returns true if the server is set to mine, otherwise false.
func (c *Client) GetGenerate() (bool, error) {
	return c.GetGenerateAsync().Receive()
}

// FutureSetGenerateResult is a future promise to deliver the result of a
// SetGenerateAsync RPC invocation (or an applicable error).
type FutureSetGenerateResult chan *futureResult

// Receive waits for the response promised by the future and returns an error if
// any occurred when setting the server to generate coins (mine) or not.
func (r FutureSetGenerateResult) Receive() error {
	_, err := receiveFuture(r)
	if err != nil {
		return err
	}

	return nil
}

// SetGenerateAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See SetGenerate for the blocking version and more details.
func (c *Client) SetGenerateAsync(enable bool, numCPUs int) FutureSetGenerateResult {
	id := c.NextID()
	cmd, err := btcjson.NewSetGenerateCmd(id, enable, numCPUs)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SetGenerate sets the server to generate coins (mine) or not.
func (c *Client) SetGenerate(enable bool, numCPUs int) error {
	return c.SetGenerateAsync(enable, numCPUs).Receive()
}

// FutureGetHashesPerSecResult is a future promise to deliver the result of a
// GetHashesPerSecAsync RPC invocation (or an applicable error).
type FutureGetHashesPerSecResult chan *futureResult

// Receive waits for the response promised by the future and returns a recent
// hashes per second performance measurement while generating coins (mining).
// Zero is returned if the server is not mining.
func (r FutureGetHashesPerSecResult) Receive() (int64, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return -1, err
	}

	// Ensure the returned data is the expected type.
	result, ok := reply.(int64)
	if !ok {
		return -1, fmt.Errorf("unexpected response type for "+
			"getnetworkhashps: %T\n", reply)
	}

	return result, nil
}

// GetHashesPerSecAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetHashesPerSec for the blocking version and more details.
func (c *Client) GetHashesPerSecAsync() FutureGetHashesPerSecResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetHashesPerSecCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetHashesPerSec returns a recent hashes per second performance measurement
// while generating coins (mining).  Zero is returned if the server is not
// mining.
func (c *Client) GetHashesPerSec() (int64, error) {
	return c.GetHashesPerSecAsync().Receive()
}

// FutureGetMiningInfoResult is a future promise to deliver the result of a
// GetMiningInfoAsync RPC invocation (or an applicable error).
type FutureGetMiningInfoResult chan *futureResult

// Receive waits for the response promised by the future and returns the mining
// information.
func (r FutureGetMiningInfoResult) Receive() (*btcjson.GetMiningInfoResult, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Ensure the returned data is the expected type.
	result, ok := reply.(btcjson.GetMiningInfoResult)
	if !ok {
		return nil, fmt.Errorf("unexpected response type for "+
			"getmininginfo: %T\n", reply)
	}

	return &result, nil
}

// GetMiningInfoAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetMiningInfo for the blocking version and more details.
func (c *Client) GetMiningInfoAsync() FutureGetMiningInfoResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetMiningInfoCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetMiningInfo returns mining information.
func (c *Client) GetMiningInfo() (*btcjson.GetMiningInfoResult, error) {
	return c.GetMiningInfoAsync().Receive()
}

// FutureGetNetworkHashPS is a future promise to deliver the result of a
// GetNetworkHashPSAsync RPC invocation (or an applicable error).
type FutureGetNetworkHashPS chan *futureResult

// Receive waits for the response promised by the future and returns the
// estimated network hashes per second for the block heights provided by the
// parameters.
func (r FutureGetNetworkHashPS) Receive() (int64, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return -1, err
	}

	// Ensure the returned data is the expected type.
	result, ok := reply.(int64)
	if !ok {
		return -1, fmt.Errorf("unexpected response type for "+
			"getnetworkhashps: %T\n", reply)
	}

	return result, nil
}

// GetNetworkHashPSAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetNetworkHashPS for the blocking version and more details.
func (c *Client) GetNetworkHashPSAsync() FutureGetNetworkHashPS {
	id := c.NextID()
	cmd, err := btcjson.NewGetNetworkHashPSCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetNetworkHashPS returns the estimated network hashes per second using the
// default number of blocks and the most recent block height.
//
// See GetNetworkHashPS2 to override the number of blocks to use and
// GetNetworkHashPS3 to override the height at which to calculate the estimate.
func (c *Client) GetNetworkHashPS() (int64, error) {
	return c.GetNetworkHashPSAsync().Receive()
}

// GetNetworkHashPS2Async returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetNetworkHashPS2 for the blocking version and more details.
func (c *Client) GetNetworkHashPS2Async(blocks int) FutureGetNetworkHashPS {
	id := c.NextID()
	cmd, err := btcjson.NewGetNetworkHashPSCmd(id, blocks)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetNetworkHashPS2 returns the estimated network hashes per second for the
// specified previous number of blocks working backwards from the most recent
// block height.  The blocks parameter can also be -1 in which case the number
// of blocks since the last difficulty change will be used.
//
// See GetNetworkHashPS to use defaults and GetNetworkHashPS3 to override the
// height at which to calculate the estimate.
func (c *Client) GetNetworkHashPS2(blocks int) (int64, error) {
	return c.GetNetworkHashPS2Async(blocks).Receive()
}

// GetNetworkHashPS3Async returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetNetworkHashPS3 for the blocking version and more details.
func (c *Client) GetNetworkHashPS3Async(blocks, height int) FutureGetNetworkHashPS {
	id := c.NextID()
	cmd, err := btcjson.NewGetNetworkHashPSCmd(id, blocks, height)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetNetworkHashPS3 returns the estimated network hashes per second for the
// specified previous number of blocks working backwards from the specified
// block height.  The blocks parameter can also be -1 in which case the number
// of blocks since the last difficulty change will be used.
//
// See GetNetworkHashPS and GetNetworkHashPS2 to use defaults.
func (c *Client) GetNetworkHashPS3(blocks, height int) (int64, error) {
	return c.GetNetworkHashPS3Async(blocks, height).Receive()
}

// FutureGetWork is a future promise to deliver the result of a
// GetWorkAsync RPC invocation (or an applicable error).
type FutureGetWork chan *futureResult

// Receive waits for the response promised by the future and returns the hash
// data to work on.
func (r FutureGetWork) Receive() (*btcjson.GetWorkResult, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Ensure the returned data is the expected type.
	result, ok := reply.(btcjson.GetWorkResult)
	if !ok {
		return nil, fmt.Errorf("unexpected response type for "+
			"getwork (request data): %T\n", reply)
	}

	return &result, nil
}

// GetWorkAsync returns an instance of a type that can be used to get the result
// of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetWork for the blocking version and more details.
func (c *Client) GetWorkAsync() FutureGetWork {
	id := c.NextID()
	cmd, err := btcjson.NewGetWorkCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetWork returns hash data to work on.
//
// See GetWorkSubmit to submit the found solution.
func (c *Client) GetWork() (*btcjson.GetWorkResult, error) {
	return c.GetWorkAsync().Receive()
}

// FutureGetWorkSubmit is a future promise to deliver the result of a
// GetWorkSubmitAsync RPC invocation (or an applicable error).
type FutureGetWorkSubmit chan *futureResult

// Receive waits for the response promised by the future and returns whether
// or not the submitted block header was accepted.
func (r FutureGetWorkSubmit) Receive() (bool, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return false, err
	}

	// Ensure the returned data is the expected type.
	accepted, ok := reply.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected response type for "+
			"getwork (submit data): %T\n", reply)
	}

	return accepted, nil
}

// GetWorkSubmitAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetWorkSubmit for the blocking version and more details.
func (c *Client) GetWorkSubmitAsync(data string) FutureGetWorkSubmit {
	id := c.NextID()
	cmd, err := btcjson.NewGetWorkCmd(id, data)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetWorkSubmit submits a block header which is a solution to previously
// requested data and returns whether or not the solution was accepted.
//
// See GetWork to request data to work on.
func (c *Client) GetWorkSubmit(data string) (bool, error) {
	return c.GetWorkSubmitAsync(data).Receive()
}

// FutureSubmitBlockResult is a future promise to deliver the result of a
// SubmitBlockAsync RPC invocation (or an applicable error).
type FutureSubmitBlockResult chan *futureResult

// Receive waits for the response promised by the future and returns an error if
// any occurred when submitting the block.
func (r FutureSubmitBlockResult) Receive() error {
	_, err := receiveFuture(r)
	if err != nil {
		return err
	}

	return nil

}

// SubmitBlockAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See SubmitBlock for the blocking version and more details.
func (c *Client) SubmitBlockAsync(block *btcutil.Block, options *btcjson.SubmitBlockOptions) FutureSubmitBlockResult {
	id := c.NextID()
	blockBytes, err := block.Bytes()
	if err != nil {
		return newFutureError(err)
	}

	blockHex := hex.EncodeToString(blockBytes)
	cmd, err := btcjson.NewSubmitBlockCmd(id, blockHex, options)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SubmitBlock attempts to submit a new block into the bitcoin network.
func (c *Client) SubmitBlock(block *btcutil.Block, options *btcjson.SubmitBlockOptions) error {
	return c.SubmitBlockAsync(block, options).Receive()
}

// TODO(davec): Implement GetBlockTemplate
