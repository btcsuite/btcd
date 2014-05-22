// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcrpcclient

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/conformal/btcjson"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// FutureGetBestBlockHashResult is a future promise to deliver the result of a
// GetBestBlockAsync RPC invocation (or an applicable error).
type FutureGetBestBlockHashResult chan *futureResult

// Receive waits for the response promised by the future and returns the hash of
// the best block in the longest block chain.
func (r FutureGetBestBlockHashResult) Receive() (*btcwire.ShaHash, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Ensure the returned data is the expected type.
	txHashStr, ok := reply.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected response type for "+
			"getbestblockhash: %T\n", reply)
	}

	return btcwire.NewShaHashFromStr(txHashStr)
}

// GetBestBlockHashAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetBestBlockHash for the blocking version and more details.
func (c *Client) GetBestBlockHashAsync() FutureGetBestBlockHashResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetBestBlockHashCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetBestBlockHash returns the hash of the best block in the longest block
// chain.
func (c *Client) GetBestBlockHash() (*btcwire.ShaHash, error) {
	return c.GetBestBlockHashAsync().Receive()
}

// FutureGetBlockResult is a future promise to deliver the result of a
// GetBlockAsync RPC invocation (or an applicable error).
type FutureGetBlockResult chan *futureResult

// Receive waits for the response promised by the future and returns the raw
// block requested from the server given its hash.
func (r FutureGetBlockResult) Receive() (*btcutil.Block, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Ensure the returned data is the expected type.
	blockHex, ok := reply.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected response type for "+
			"getblock (verbose=0): %T\n", reply)
	}

	// Decode the serialized block hex to raw bytes.
	serializedBlock, err := hex.DecodeString(blockHex)
	if err != nil {
		return nil, err
	}

	// Deserialize the block and return it.
	var msgBlock btcwire.MsgBlock
	msgBlock.Deserialize(bytes.NewBuffer(serializedBlock))
	if err != nil {
		return nil, err
	}
	return btcutil.NewBlock(&msgBlock), nil
}

// GetBlockAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBlock for the blocking version and more details.
func (c *Client) GetBlockAsync(blockHash *btcwire.ShaHash) FutureGetBlockResult {
	hash := ""
	if blockHash != nil {
		hash = blockHash.String()
	}

	id := c.NextID()
	cmd, err := btcjson.NewGetBlockCmd(id, hash, false)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetBlock returns a raw block from the server given its hash.
//
// See GetBlockVerbose to retrieve a data structure with information about the
// block instead.
func (c *Client) GetBlock(blockHash *btcwire.ShaHash) (*btcutil.Block, error) {
	return c.GetBlockAsync(blockHash).Receive()
}

// FutureGetBlockVerboseResult is a future promise to deliver the result of a
// GetBlockVerboseAsync RPC invocation (or an applicable error).
type FutureGetBlockVerboseResult chan *futureResult

// Receive waits for the response promised by the future and returns the data
// structure from the server with information about the requested block.
func (r FutureGetBlockVerboseResult) Receive() (*btcjson.BlockResult, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Ensure the returned data is the expected type.
	blockResult, ok := reply.(*btcjson.BlockResult)
	if !ok {
		return nil, fmt.Errorf("unexpected response type for "+
			"getblock (verbose=1): %T\n", reply)
	}

	return blockResult, nil
}

// GetBlockVerboseAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetBlockVerbose for the blocking version and more details.
func (c *Client) GetBlockVerboseAsync(blockHash *btcwire.ShaHash, verboseTx bool) FutureGetBlockVerboseResult {
	hash := ""
	if blockHash != nil {
		hash = blockHash.String()
	}

	id := c.NextID()
	cmd, err := btcjson.NewGetBlockCmd(id, hash, true, verboseTx)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetBlockVerbose returns a data structure from the server with information
// about a block given its hash.
//
// See GetBlock to retrieve a raw block instead.
func (c *Client) GetBlockVerbose(blockHash *btcwire.ShaHash, verboseTx bool) (*btcjson.BlockResult, error) {
	return c.GetBlockVerboseAsync(blockHash, verboseTx).Receive()
}

// FutureGetBlockCountResult is a future promise to deliver the result of a
// GetBlockCountAsync RPC invocation (or an applicable error).
type FutureGetBlockCountResult chan *futureResult

// Receive waits for the response promised by the future and returns the number
// of blocks in the longest block chain.
func (r FutureGetBlockCountResult) Receive() (int64, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return 0, err
	}

	// Ensure the returned data is the expected type.
	count, ok := reply.(float64)
	if !ok {
		return 0, fmt.Errorf("unexpected response type for "+
			"getblockcount: %T\n", reply)
	}

	return int64(count), nil
}

// GetBlockCountAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBlockCount for the blocking version and more details.
func (c *Client) GetBlockCountAsync() FutureGetBlockCountResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetBlockCountCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetBlockCount returns the number of blocks in the longest block chain.
func (c *Client) GetBlockCount() (int64, error) {
	return c.GetBlockCountAsync().Receive()
}

// FutureGetDifficultyResult is a future promise to deliver the result of a
// GetDifficultyAsync RPC invocation (or an applicable error).
type FutureGetDifficultyResult chan *futureResult

// Receive waits for the response promised by the future and returns the
// proof-of-work difficulty as a multiple of the minimum difficulty.
func (r FutureGetDifficultyResult) Receive() (float64, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return 0, err
	}

	// Ensure the returned data is the expected type.
	difficulty, ok := reply.(float64)
	if !ok {
		return 0, fmt.Errorf("unexpected response type for "+
			"getdifficulty: %T\n", reply)
	}

	return difficulty, nil
}

// GetDifficultyAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetDifficulty for the blocking version and more details.
func (c *Client) GetDifficultyAsync() FutureGetDifficultyResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetDifficultyCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetDifficulty returns the proof-of-work difficulty as a multiple of the
// minimum difficulty.
func (c *Client) GetDifficulty() (float64, error) {
	return c.GetDifficultyAsync().Receive()
}

// FutureGetBlockHashResult is a future promise to deliver the result of a
// GetBlockHashAsync RPC invocation (or an applicable error).
type FutureGetBlockHashResult chan *futureResult

// Receive waits for the response promised by the future and returns the hash of
// the block in the best block chain at the given height.
func (r FutureGetBlockHashResult) Receive() (*btcwire.ShaHash, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Ensure the returned data is the expected type.
	txHashStr, ok := reply.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected response type for "+
			"getblockhash: %T\n", reply)
	}

	return btcwire.NewShaHashFromStr(txHashStr)
}

// GetBlockHashAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBlockHash for the blocking version and more details.
func (c *Client) GetBlockHashAsync(blockHeight int64) FutureGetBlockHashResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetBlockHashCmd(id, blockHeight)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetBlockHash returns the hash of the block in the best block chain at the
// given height.
func (c *Client) GetBlockHash(blockHeight int64) (*btcwire.ShaHash, error) {
	return c.GetBlockHashAsync(blockHeight).Receive()
}

// FutureGetRawMempoolResult is a future promise to deliver the result of a
// GetRawMempoolAsync RPC invocation (or an applicable error).
type FutureGetRawMempoolResult chan *futureResult

// Receive waits for the response promised by the future and returns the hashes
// of all transactions in the memory pool.
func (r FutureGetRawMempoolResult) Receive() ([]*btcwire.ShaHash, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Ensure the returned data is the expected type.
	txHashStrs, ok := reply.([]string)
	if !ok {
		return nil, fmt.Errorf("unexpected response type for "+
			"getrawmempool (verbose=false): %T\n", reply)
	}

	txHashes := make([]*btcwire.ShaHash, 0, len(txHashStrs))
	for _, hashStr := range txHashStrs {
		txHash, err := btcwire.NewShaHashFromStr(hashStr)
		if err != nil {
			return nil, err
		}
		txHashes = append(txHashes, txHash)
	}

	return txHashes, nil
}

// GetRawMempoolAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetRawMempool for the blocking version and more details.
func (c *Client) GetRawMempoolAsync() FutureGetRawMempoolResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetRawMempoolCmd(id, false)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetRawMempool returns the hashes of all transactions in the memory pool.
//
// See GetRawMempoolVerbose to retrieve data structures with information about
// the transactions instead.
func (c *Client) GetRawMempool() ([]*btcwire.ShaHash, error) {
	return c.GetRawMempoolAsync().Receive()
}

// FutureGetRawMempoolVerboseResult is a future promise to deliver the result of
// a GetRawMempoolVerboseAsync RPC invocation (or an applicable error).
type FutureGetRawMempoolVerboseResult chan *futureResult

// Receive waits for the response promised by the future and returns a map of
// transaction hashes to an associated data structure with information about the
// transaction for all transactions in the memory pool.
func (r FutureGetRawMempoolVerboseResult) Receive() (map[string]btcjson.GetRawMempoolResult, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Ensure the returned data is the expected type.
	mempoolItems, ok := reply.(map[string]btcjson.GetRawMempoolResult)
	if !ok {
		return nil, fmt.Errorf("unexpected response type for "+
			"getrawmempool (verbose=true): %T\n", reply)
	}

	return mempoolItems, nil
}

// GetRawMempoolVerboseAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetRawMempoolVerbose for the blocking version and more details.
func (c *Client) GetRawMempoolVerboseAsync() FutureGetRawMempoolVerboseResult {
	id := c.NextID()
	cmd, err := btcjson.NewGetRawMempoolCmd(id, true)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetRawMempoolVerbose returns a map of transaction hashes to an associated
// data structure with information about the transaction for all transactions in
// the memory pool.
//
// See GetRawMempool to retrieve only the transaction hashes instead.
func (c *Client) GetRawMempoolVerbose() (map[string]btcjson.GetRawMempoolResult, error) {
	return c.GetRawMempoolVerboseAsync().Receive()
}

// FutureVerifyChainResult is a future promise to deliver the result of a
// VerifyChainAsync, VerifyChainLevelAsyncRPC, or VerifyChainBlocksAsync
// invocation (or an applicable error).
type FutureVerifyChainResult chan *futureResult

// Receive waits for the response promised by the future and returns whether
// or not the chain verified based on the check level and number of blocks
// to verify specified in the original call.
func (r FutureVerifyChainResult) Receive() (bool, error) {
	reply, err := receiveFuture(r)
	if err != nil {
		return false, err
	}

	// Ensure the returned data is the expected type.
	verified, ok := reply.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected response type for "+
			"verifychain: %T\n", reply)
	}

	return verified, nil
}

// VerifyChainAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See VerifyChain for the blocking version and more details.
func (c *Client) VerifyChainAsync() FutureVerifyChainResult {
	id := c.NextID()
	cmd, err := btcjson.NewVerifyChainCmd(id)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// VerifyChain requests the server to verify the block chain database using
// the default check level and number of blocks to verify.
//
// See VerifyChainLevel and VerifyChainBlocks to override the defaults.
func (c *Client) VerifyChain() (bool, error) {
	return c.VerifyChainAsync().Receive()
}

// VerifyChainLevelAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See VerifyChainLevel for the blocking version and more details.
func (c *Client) VerifyChainLevelAsync(checkLevel int32) FutureVerifyChainResult {
	id := c.NextID()
	cmd, err := btcjson.NewVerifyChainCmd(id, checkLevel)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// VerifyChainLevel requests the server to verify the block chain database using
// the passed check level and default number of blocks to verify.
//
// The check level controls how thorough the verification is with higher numbers
// increasing the amount of checks done as consequently how long the
// verification takes.
//
// See VerifyChain to use the default check level and VerifyChainBlocks to
// override the number of blocks to verify.
func (c *Client) VerifyChainLevel(checkLevel int32) (bool, error) {
	return c.VerifyChainLevelAsync(checkLevel).Receive()
}

// VerifyChainBlocksAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See VerifyChainBlocks for the blocking version and more details.
func (c *Client) VerifyChainBlocksAsync(checkLevel, numBlocks int32) FutureVerifyChainResult {
	id := c.NextID()
	cmd, err := btcjson.NewVerifyChainCmd(id, checkLevel, numBlocks)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// VerifyChainBlocks requests the server to verify the block chain database
// using the passed check level and number of blocks to verify.
//
// The check level controls how thorough the verification is with higher numbers
// increasing the amount of checks done as consequently how long the
// verification takes.
//
// The number of blocks refers to the number of blocks from the end of the
// current longest chain.
//
// See VerifyChain and VerifyChainLevel to use defaults.
func (c *Client) VerifyChainBlocks(checkLevel, numBlocks int32) (bool, error) {
	return c.VerifyChainBlocksAsync(checkLevel, numBlocks).Receive()
}
