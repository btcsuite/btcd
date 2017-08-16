// Copyright (c) 2014-2017 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"bytes"
	"encoding/hex"
	"encoding/json"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// FutureGetBestBlockHashResult is a future promise to deliver the result of a
// GetBestBlockAsync RPC invocation (or an applicable error).
type FutureGetBestBlockHashResult chan *response

// Receive waits for the response promised by the future and returns the hash of
// the best block in the longest block chain.
func (r FutureGetBestBlockHashResult) Receive() (*chainhash.Hash, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var txHashStr string
	err = json.Unmarshal(res, &txHashStr)
	if err != nil {
		return nil, err
	}
	return chainhash.NewHashFromStr(txHashStr)
}

// GetBestBlockHashAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetBestBlockHash for the blocking version and more details.
func (c *Client) GetBestBlockHashAsync() FutureGetBestBlockHashResult {
	cmd := btcjson.NewGetBestBlockHashCmd()
	return c.sendCmd(cmd)
}

// GetBestBlockHash returns the hash of the best block in the longest block
// chain.
func (c *Client) GetBestBlockHash() (*chainhash.Hash, error) {
	return c.GetBestBlockHashAsync().Receive()
}

// FutureGetBlockResult is a future promise to deliver the result of a
// GetBlockAsync RPC invocation (or an applicable error).
type FutureGetBlockResult chan *response

// Receive waits for the response promised by the future and returns the raw
// block requested from the server given its hash.
func (r FutureGetBlockResult) Receive() (*wire.MsgBlock, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var blockHex string
	err = json.Unmarshal(res, &blockHex)
	if err != nil {
		return nil, err
	}

	// Decode the serialized block hex to raw bytes.
	serializedBlock, err := hex.DecodeString(blockHex)
	if err != nil {
		return nil, err
	}

	// Deserialize the block and return it.
	var msgBlock wire.MsgBlock
	err = msgBlock.Deserialize(bytes.NewReader(serializedBlock))
	if err != nil {
		return nil, err
	}
	return &msgBlock, nil
}

// GetBlockAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBlock for the blocking version and more details.
func (c *Client) GetBlockAsync(blockHash *chainhash.Hash) FutureGetBlockResult {
	hash := ""
	if blockHash != nil {
		hash = blockHash.String()
	}

	cmd := btcjson.NewGetBlockCmd(hash, btcjson.Bool(false), nil)
	return c.sendCmd(cmd)
}

// GetBlock returns a raw block from the server given its hash.
//
// See GetBlockVerbose to retrieve a data structure with information about the
// block instead.
func (c *Client) GetBlock(blockHash *chainhash.Hash) (*wire.MsgBlock, error) {
	return c.GetBlockAsync(blockHash).Receive()
}

// FutureGetBlockVerboseResult is a future promise to deliver the result of a
// GetBlockVerboseAsync RPC invocation (or an applicable error).
type FutureGetBlockVerboseResult chan *response

// Receive waits for the response promised by the future and returns the data
// structure from the server with information about the requested block.
func (r FutureGetBlockVerboseResult) Receive() (*btcjson.GetBlockVerboseResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal the raw result into a BlockResult.
	var blockResult btcjson.GetBlockVerboseResult
	err = json.Unmarshal(res, &blockResult)
	if err != nil {
		return nil, err
	}
	return &blockResult, nil
}

// GetBlockVerboseAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetBlockVerbose for the blocking version and more details.
func (c *Client) GetBlockVerboseAsync(blockHash *chainhash.Hash) FutureGetBlockVerboseResult {
	hash := ""
	if blockHash != nil {
		hash = blockHash.String()
	}

	cmd := btcjson.NewGetBlockCmd(hash, btcjson.Bool(true), nil)
	return c.sendCmd(cmd)
}

// GetBlockVerbose returns a data structure from the server with information
// about a block given its hash.
//
// See GetBlockVerboseTx to retrieve transaction data structures as well.
// See GetBlock to retrieve a raw block instead.
func (c *Client) GetBlockVerbose(blockHash *chainhash.Hash) (*btcjson.GetBlockVerboseResult, error) {
	return c.GetBlockVerboseAsync(blockHash).Receive()
}

// GetBlockVerboseTxAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetBlockVerboseTx or the blocking version and more details.
func (c *Client) GetBlockVerboseTxAsync(blockHash *chainhash.Hash) FutureGetBlockVerboseResult {
	hash := ""
	if blockHash != nil {
		hash = blockHash.String()
	}

	cmd := btcjson.NewGetBlockCmd(hash, btcjson.Bool(true), btcjson.Bool(true))
	return c.sendCmd(cmd)
}

// GetBlockVerboseTx returns a data structure from the server with information
// about a block and its transactions given its hash.
//
// See GetBlockVerbose if only transaction hashes are preferred.
// See GetBlock to retrieve a raw block instead.
func (c *Client) GetBlockVerboseTx(blockHash *chainhash.Hash) (*btcjson.GetBlockVerboseResult, error) {
	return c.GetBlockVerboseTxAsync(blockHash).Receive()
}

// FutureGetBlockCountResult is a future promise to deliver the result of a
// GetBlockCountAsync RPC invocation (or an applicable error).
type FutureGetBlockCountResult chan *response

// Receive waits for the response promised by the future and returns the number
// of blocks in the longest block chain.
func (r FutureGetBlockCountResult) Receive() (int64, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return 0, err
	}

	// Unmarshal the result as an int64.
	var count int64
	err = json.Unmarshal(res, &count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// GetBlockCountAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBlockCount for the blocking version and more details.
func (c *Client) GetBlockCountAsync() FutureGetBlockCountResult {
	cmd := btcjson.NewGetBlockCountCmd()
	return c.sendCmd(cmd)
}

// GetBlockCount returns the number of blocks in the longest block chain.
func (c *Client) GetBlockCount() (int64, error) {
	return c.GetBlockCountAsync().Receive()
}

// FutureGetDifficultyResult is a future promise to deliver the result of a
// GetDifficultyAsync RPC invocation (or an applicable error).
type FutureGetDifficultyResult chan *response

// Receive waits for the response promised by the future and returns the
// proof-of-work difficulty as a multiple of the minimum difficulty.
func (r FutureGetDifficultyResult) Receive() (float64, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return 0, err
	}

	// Unmarshal the result as a float64.
	var difficulty float64
	err = json.Unmarshal(res, &difficulty)
	if err != nil {
		return 0, err
	}
	return difficulty, nil
}

// GetDifficultyAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetDifficulty for the blocking version and more details.
func (c *Client) GetDifficultyAsync() FutureGetDifficultyResult {
	cmd := btcjson.NewGetDifficultyCmd()
	return c.sendCmd(cmd)
}

// GetDifficulty returns the proof-of-work difficulty as a multiple of the
// minimum difficulty.
func (c *Client) GetDifficulty() (float64, error) {
	return c.GetDifficultyAsync().Receive()
}

// FutureGetBlockChainInfoResult is a promise to deliver the result of a
// GetBlockChainInfoAsync RPC invocation (or an applicable error).
type FutureGetBlockChainInfoResult chan *response

// Receive waits for the response promised by the future and returns chain info
// result provided by the server.
func (r FutureGetBlockChainInfoResult) Receive() (*btcjson.GetBlockChainInfoResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	var chainInfo btcjson.GetBlockChainInfoResult
	if err := json.Unmarshal(res, &chainInfo); err != nil {
		return nil, err
	}
	return &chainInfo, nil
}

// GetBlockChainInfoAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function
// on the returned instance.
//
// See GetBlockChainInfo for the blocking version and more details.
func (c *Client) GetBlockChainInfoAsync() FutureGetBlockChainInfoResult {
	cmd := btcjson.NewGetBlockChainInfoCmd()
	return c.sendCmd(cmd)
}

// GetBlockChainInfo returns information related to the processing state of
// various chain-specific details such as the current difficulty from the tip
// of the main chain.
func (c *Client) GetBlockChainInfo() (*btcjson.GetBlockChainInfoResult, error) {
	return c.GetBlockChainInfoAsync().Receive()
}

// FutureGetBlockHashResult is a future promise to deliver the result of a
// GetBlockHashAsync RPC invocation (or an applicable error).
type FutureGetBlockHashResult chan *response

// Receive waits for the response promised by the future and returns the hash of
// the block in the best block chain at the given height.
func (r FutureGetBlockHashResult) Receive() (*chainhash.Hash, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result as a string-encoded sha.
	var txHashStr string
	err = json.Unmarshal(res, &txHashStr)
	if err != nil {
		return nil, err
	}
	return chainhash.NewHashFromStr(txHashStr)
}

// GetBlockHashAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBlockHash for the blocking version and more details.
func (c *Client) GetBlockHashAsync(blockHeight int64) FutureGetBlockHashResult {
	cmd := btcjson.NewGetBlockHashCmd(blockHeight)
	return c.sendCmd(cmd)
}

// GetBlockHash returns the hash of the block in the best block chain at the
// given height.
func (c *Client) GetBlockHash(blockHeight int64) (*chainhash.Hash, error) {
	return c.GetBlockHashAsync(blockHeight).Receive()
}

// FutureGetBlockHeaderResult is a future promise to deliver the result of a
// GetBlockHeaderAsync RPC invocation (or an applicable error).
type FutureGetBlockHeaderResult chan *response

// Receive waits for the response promised by the future and returns the
// blockheader requested from the server given its hash.
func (r FutureGetBlockHeaderResult) Receive() (*wire.BlockHeader, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var bhHex string
	err = json.Unmarshal(res, &bhHex)
	if err != nil {
		return nil, err
	}

	serializedBH, err := hex.DecodeString(bhHex)
	if err != nil {
		return nil, err
	}

	// Deserialize the blockheader and return it.
	var bh wire.BlockHeader
	err = bh.Deserialize(bytes.NewReader(serializedBH))
	if err != nil {
		return nil, err
	}

	return &bh, err
}

// GetBlockHeaderAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBlockHeader for the blocking version and more details.
func (c *Client) GetBlockHeaderAsync(blockHash *chainhash.Hash) FutureGetBlockHeaderResult {
	hash := ""
	if blockHash != nil {
		hash = blockHash.String()
	}

	cmd := btcjson.NewGetBlockHeaderCmd(hash, btcjson.Bool(false))
	return c.sendCmd(cmd)
}

// GetBlockHeader returns the blockheader from the server given its hash.
//
// See GetBlockHeaderVerbose to retrieve a data structure with information about the
// block instead.
func (c *Client) GetBlockHeader(blockHash *chainhash.Hash) (*wire.BlockHeader, error) {
	return c.GetBlockHeaderAsync(blockHash).Receive()
}

// FutureGetBlockHeaderVerboseResult is a future promise to deliver the result of a
// GetBlockAsync RPC invocation (or an applicable error).
type FutureGetBlockHeaderVerboseResult chan *response

// Receive waits for the response promised by the future and returns the
// data structure of the blockheader requested from the server given its hash.
func (r FutureGetBlockHeaderVerboseResult) Receive() (*btcjson.GetBlockHeaderVerboseResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var bh btcjson.GetBlockHeaderVerboseResult
	err = json.Unmarshal(res, &bh)
	if err != nil {
		return nil, err
	}

	return &bh, nil
}

// GetBlockHeaderVerboseAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetBlockHeader for the blocking version and more details.
func (c *Client) GetBlockHeaderVerboseAsync(blockHash *chainhash.Hash) FutureGetBlockHeaderVerboseResult {
	hash := ""
	if blockHash != nil {
		hash = blockHash.String()
	}

	cmd := btcjson.NewGetBlockHeaderCmd(hash, btcjson.Bool(true))
	return c.sendCmd(cmd)
}

// GetBlockHeaderVerbose returns a data structure with information about the
// blockheader from the server given its hash.
//
// See GetBlockHeader to retrieve a blockheader instead.
func (c *Client) GetBlockHeaderVerbose(blockHash *chainhash.Hash) (*btcjson.GetBlockHeaderVerboseResult, error) {
	return c.GetBlockHeaderVerboseAsync(blockHash).Receive()
}

// FutureGetMempoolEntryResult is a future promise to deliver the result of a
// GetMempoolEntryAsync RPC invocation (or an applicable error).
type FutureGetMempoolEntryResult chan *response

// Receive waits for the response promised by the future and returns a data
// structure with information about the transaction in the memory pool given
// its hash.
func (r FutureGetMempoolEntryResult) Receive() (*btcjson.GetMempoolEntryResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result as an array of strings.
	var mempoolEntryResult btcjson.GetMempoolEntryResult
	err = json.Unmarshal(res, &mempoolEntryResult)
	if err != nil {
		return nil, err
	}

	return &mempoolEntryResult, nil
}

// GetMempoolEntryAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See GetMempoolEntry for the blocking version and more details.
func (c *Client) GetMempoolEntryAsync(txHash string) FutureGetMempoolEntryResult {
	cmd := btcjson.NewGetMempoolEntryCmd(txHash)
	return c.sendCmd(cmd)
}

// GetMempoolEntry returns a data structure with information about the
// transaction in the memory pool given its hash.
func (c *Client) GetMempoolEntry(txHash string) (*btcjson.GetMempoolEntryResult, error) {
	return c.GetMempoolEntryAsync(txHash).Receive()
}

// FutureGetRawMempoolResult is a future promise to deliver the result of a
// GetRawMempoolAsync RPC invocation (or an applicable error).
type FutureGetRawMempoolResult chan *response

// Receive waits for the response promised by the future and returns the hashes
// of all transactions in the memory pool.
func (r FutureGetRawMempoolResult) Receive() ([]*chainhash.Hash, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result as an array of strings.
	var txHashStrs []string
	err = json.Unmarshal(res, &txHashStrs)
	if err != nil {
		return nil, err
	}

	// Create a slice of ShaHash arrays from the string slice.
	txHashes := make([]*chainhash.Hash, 0, len(txHashStrs))
	for _, hashStr := range txHashStrs {
		txHash, err := chainhash.NewHashFromStr(hashStr)
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
	cmd := btcjson.NewGetRawMempoolCmd(btcjson.Bool(false))
	return c.sendCmd(cmd)
}

// GetRawMempool returns the hashes of all transactions in the memory pool.
//
// See GetRawMempoolVerbose to retrieve data structures with information about
// the transactions instead.
func (c *Client) GetRawMempool() ([]*chainhash.Hash, error) {
	return c.GetRawMempoolAsync().Receive()
}

// FutureGetRawMempoolVerboseResult is a future promise to deliver the result of
// a GetRawMempoolVerboseAsync RPC invocation (or an applicable error).
type FutureGetRawMempoolVerboseResult chan *response

// Receive waits for the response promised by the future and returns a map of
// transaction hashes to an associated data structure with information about the
// transaction for all transactions in the memory pool.
func (r FutureGetRawMempoolVerboseResult) Receive() (map[string]btcjson.GetRawMempoolVerboseResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result as a map of strings (tx shas) to their detailed
	// results.
	var mempoolItems map[string]btcjson.GetRawMempoolVerboseResult
	err = json.Unmarshal(res, &mempoolItems)
	if err != nil {
		return nil, err
	}
	return mempoolItems, nil
}

// GetRawMempoolVerboseAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetRawMempoolVerbose for the blocking version and more details.
func (c *Client) GetRawMempoolVerboseAsync() FutureGetRawMempoolVerboseResult {
	cmd := btcjson.NewGetRawMempoolCmd(btcjson.Bool(true))
	return c.sendCmd(cmd)
}

// GetRawMempoolVerbose returns a map of transaction hashes to an associated
// data structure with information about the transaction for all transactions in
// the memory pool.
//
// See GetRawMempool to retrieve only the transaction hashes instead.
func (c *Client) GetRawMempoolVerbose() (map[string]btcjson.GetRawMempoolVerboseResult, error) {
	return c.GetRawMempoolVerboseAsync().Receive()
}

// FutureVerifyChainResult is a future promise to deliver the result of a
// VerifyChainAsync, VerifyChainLevelAsyncRPC, or VerifyChainBlocksAsync
// invocation (or an applicable error).
type FutureVerifyChainResult chan *response

// Receive waits for the response promised by the future and returns whether
// or not the chain verified based on the check level and number of blocks
// to verify specified in the original call.
func (r FutureVerifyChainResult) Receive() (bool, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return false, err
	}

	// Unmarshal the result as a boolean.
	var verified bool
	err = json.Unmarshal(res, &verified)
	if err != nil {
		return false, err
	}
	return verified, nil
}

// VerifyChainAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See VerifyChain for the blocking version and more details.
func (c *Client) VerifyChainAsync() FutureVerifyChainResult {
	cmd := btcjson.NewVerifyChainCmd(nil, nil)
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
	cmd := btcjson.NewVerifyChainCmd(&checkLevel, nil)
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
	cmd := btcjson.NewVerifyChainCmd(&checkLevel, &numBlocks)
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

// FutureGetTxOutResult is a future promise to deliver the result of a
// GetTxOutAsync RPC invocation (or an applicable error).
type FutureGetTxOutResult chan *response

// Receive waits for the response promised by the future and returns a
// transaction given its hash.
func (r FutureGetTxOutResult) Receive() (*btcjson.GetTxOutResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// take care of the special case where the output has been spent already
	// it should return the string "null"
	if string(res) == "null" {
		return nil, nil
	}

	// Unmarshal result as an gettxout result object.
	var txOutInfo *btcjson.GetTxOutResult
	err = json.Unmarshal(res, &txOutInfo)
	if err != nil {
		return nil, err
	}

	return txOutInfo, nil
}

// GetTxOutAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetTxOut for the blocking version and more details.
func (c *Client) GetTxOutAsync(txHash *chainhash.Hash, index uint32, mempool bool) FutureGetTxOutResult {
	hash := ""
	if txHash != nil {
		hash = txHash.String()
	}

	cmd := btcjson.NewGetTxOutCmd(hash, index, &mempool)
	return c.sendCmd(cmd)
}

// GetTxOut returns the transaction output info if it's unspent and
// nil, otherwise.
func (c *Client) GetTxOut(txHash *chainhash.Hash, index uint32, mempool bool) (*btcjson.GetTxOutResult, error) {
	return c.GetTxOutAsync(txHash, index, mempool).Receive()
}

// FutureRescanBlocksResult is a future promise to deliver the result of a
// RescanBlocksAsync RPC invocation (or an applicable error).
//
// NOTE: This is a btcsuite extension ported from
// github.com/decred/dcrrpcclient.
type FutureRescanBlocksResult chan *response

// Receive waits for the response promised by the future and returns the
// discovered rescanblocks data.
//
// NOTE: This is a btcsuite extension ported from
// github.com/decred/dcrrpcclient.
func (r FutureRescanBlocksResult) Receive() ([]btcjson.RescannedBlock, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	var rescanBlocksResult []btcjson.RescannedBlock
	err = json.Unmarshal(res, &rescanBlocksResult)
	if err != nil {
		return nil, err
	}

	return rescanBlocksResult, nil
}

// RescanBlocksAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See RescanBlocks for the blocking version and more details.
//
// NOTE: This is a btcsuite extension ported from
// github.com/decred/dcrrpcclient.
func (c *Client) RescanBlocksAsync(blockHashes []chainhash.Hash) FutureRescanBlocksResult {
	strBlockHashes := make([]string, len(blockHashes))
	for i := range blockHashes {
		strBlockHashes[i] = blockHashes[i].String()
	}

	cmd := btcjson.NewRescanBlocksCmd(strBlockHashes)
	return c.sendCmd(cmd)
}

// RescanBlocks rescans the blocks identified by blockHashes, in order, using
// the client's loaded transaction filter.  The blocks do not need to be on the
// main chain, but they do need to be adjacent to each other.
//
// NOTE: This is a btcsuite extension ported from
// github.com/decred/dcrrpcclient.
func (c *Client) RescanBlocks(blockHashes []chainhash.Hash) ([]btcjson.RescannedBlock, error) {
	return c.RescanBlocksAsync(blockHashes).Receive()
}

// FutureInvalidateBlockResult is a future promise to deliver the result of a
// InvalidateBlockAsync RPC invocation (or an applicable error).
type FutureInvalidateBlockResult chan *response

// Receive waits for the response promised by the future and returns the raw
// block requested from the server given its hash.
func (r FutureInvalidateBlockResult) Receive() error {
	_, err := receiveFuture(r)

	return err
}

// InvalidateBlockAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on the
// returned instance.
//
// See InvalidateBlock for the blocking version and more details.
func (c *Client) InvalidateBlockAsync(blockHash *chainhash.Hash) FutureInvalidateBlockResult {
	hash := ""
	if blockHash != nil {
		hash = blockHash.String()
	}

	cmd := btcjson.NewInvalidateBlockCmd(hash)
	return c.sendCmd(cmd)
}

// InvalidateBlock invalidates a specific block.
func (c *Client) InvalidateBlock(blockHash *chainhash.Hash) error {
	return c.InvalidateBlockAsync(blockHash).Receive()
}
