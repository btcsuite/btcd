// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcrpcclient

import (
	"bytes"
	"encoding/hex"
	"encoding/json"

	"github.com/conformal/btcjson"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// SigHashType enumerates the available signature hashing types that the
// SignRawTransaction function accepts.
type SigHashType string

// Constants used to indicate the signature hash type for SignRawTransaction.
const (
	// SigHashAll indicates ALL of the outputs should be signed.
	SigHashAll SigHashType = "ALL"

	// SigHashNone indicates NONE of the outputs should be signed.  This
	// can be thought of as specifying the signer does not care where the
	// bitcoins go.
	SigHashNone SigHashType = "NONE"

	// SigHashSingle indicates that a SINGLE output should be signed.  This
	// can be thought of specifying the signer only cares about where ONE of
	// the outputs goes, but not any of the others.
	SigHashSingle SigHashType = "SINGLE"

	// SigHashAllAnyoneCanPay indicates that signer does not care where the
	// other inputs to the transaction come from, so it allows other people
	// to add inputs.  In addition, it uses the SigHashAll signing method
	// for outputs.
	SigHashAllAnyoneCanPay SigHashType = "ALL|ANYONECANPAY"

	// SigHashNoneAnyoneCanPay indicates that signer does not care where the
	// other inputs to the transaction come from, so it allows other people
	// to add inputs.  In addition, it uses the SigHashNone signing method
	// for outputs.
	SigHashNoneAnyoneCanPay SigHashType = "NONE|ANYONECANPAY"

	// SigHashSingleAnyoneCanPay indicates that signer does not care where
	// the other inputs to the transaction come from, so it allows other
	// people to add inputs.  In addition, it uses the SigHashSingle signing
	// method for outputs.
	SigHashSingleAnyoneCanPay SigHashType = "SINGLE|ANYONECANPAY"
)

// String returns the SighHashType in human-readable form.
func (s SigHashType) String() string {
	return string(s)
}

// FutureGetRawTransactionResult is a future promise to deliver the result of a
// GetRawTransactionAsync RPC invocation (or an applicable error).
type FutureGetRawTransactionResult chan *response

// Receive waits for the response promised by the future and returns a
// transaction given its hash.
func (r FutureGetRawTransactionResult) Receive() (*btcutil.Tx, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var txHex string
	err = json.Unmarshal(res, &txHex)
	if err != nil {
		return nil, err
	}

	// Decode the serialized transaction hex to raw bytes.
	serializedTx, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, err
	}

	// Deserialize the transaction and return it.
	var msgTx btcwire.MsgTx
	if err := msgTx.Deserialize(bytes.NewReader(serializedTx)); err != nil {
		return nil, err
	}
	return btcutil.NewTx(&msgTx), nil
}

// GetRawTransactionAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See GetRawTransaction for the blocking version and more details.
func (c *Client) GetRawTransactionAsync(txHash *btcwire.ShaHash) FutureGetRawTransactionResult {
	hash := ""
	if txHash != nil {
		hash = txHash.String()
	}

	id := c.NextID()
	cmd, err := btcjson.NewGetRawTransactionCmd(id, hash, 0)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetRawTransaction returns a transaction given its hash.
//
// See GetRawTransactionVerbose to obtain additional information about the
// transaction.
func (c *Client) GetRawTransaction(txHash *btcwire.ShaHash) (*btcutil.Tx, error) {
	return c.GetRawTransactionAsync(txHash).Receive()
}

// FutureGetRawTransactionVerboseResult is a future promise to deliver the
// result of a GetRawTransactionVerboseAsync RPC invocation (or an applicable
// error).
type FutureGetRawTransactionVerboseResult chan *response

// Receive waits for the response promised by the future and returns information
// about a transaction given its hash.
func (r FutureGetRawTransactionVerboseResult) Receive() (*btcjson.TxRawResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a gettrawtransaction result object.
	var rawTxResult btcjson.TxRawResult
	err = json.Unmarshal(res, &rawTxResult)
	if err != nil {
		return nil, err
	}

	return &rawTxResult, nil
}

// GetRawTransactionVerboseAsync returns an instance of a type that can be used
// to get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See GetRawTransactionVerbose for the blocking version and more details.
func (c *Client) GetRawTransactionVerboseAsync(txHash *btcwire.ShaHash) FutureGetRawTransactionVerboseResult {
	hash := ""
	if txHash != nil {
		hash = txHash.String()
	}

	id := c.NextID()
	cmd, err := btcjson.NewGetRawTransactionCmd(id, hash, 1)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// GetRawTransactionVerbose returns information about a transaction given
// its hash.
//
// See GetRawTransaction to obtain only the transaction already deserialized.
func (c *Client) GetRawTransactionVerbose(txHash *btcwire.ShaHash) (*btcjson.TxRawResult, error) {
	return c.GetRawTransactionVerboseAsync(txHash).Receive()
}

// FutureDecodeRawTransactionResult is a future promise to deliver the result
// of a DecodeRawTransactionAsync RPC invocation (or an applicable error).
type FutureDecodeRawTransactionResult chan *response

// Receive waits for the response promised by the future and returns information
// about a transaction given its serialized bytes.
func (r FutureDecodeRawTransactionResult) Receive() (*btcjson.TxRawResult, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a decoderawtransaction result object.
	var rawTxResult btcjson.TxRawResult
	err = json.Unmarshal(res, &rawTxResult)
	if err != nil {
		return nil, err
	}

	return &rawTxResult, nil
}

// DecodeRawTransactionAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See DecodeRawTransaction for the blocking version and more details.
func (c *Client) DecodeRawTransactionAsync(serializedTx []byte) FutureDecodeRawTransactionResult {
	id := c.NextID()
	txHex := hex.EncodeToString(serializedTx)
	cmd, err := btcjson.NewDecodeRawTransactionCmd(id, txHex)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// DecodeRawTransaction returns information about a transaction given its
// serialized bytes.
func (c *Client) DecodeRawTransaction(serializedTx []byte) (*btcjson.TxRawResult, error) {
	return c.DecodeRawTransactionAsync(serializedTx).Receive()
}

// FutureCreateRawTransactionResult is a future promise to deliver the result
// of a CreateRawTransactionAsync RPC invocation (or an applicable error).
type FutureCreateRawTransactionResult chan *response

// Receive waits for the response promised by the future and returns a new
// transaction spending the provided inputs and sending to the provided
// addresses.
func (r FutureCreateRawTransactionResult) Receive() (*btcwire.MsgTx, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a string.
	var txHex string
	err = json.Unmarshal(res, &txHex)
	if err != nil {
		return nil, err
	}

	// Decode the serialized transaction hex to raw bytes.
	serializedTx, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, err
	}

	// Deserialize the transaction and return it.
	var msgTx btcwire.MsgTx
	if err := msgTx.Deserialize(bytes.NewReader(serializedTx)); err != nil {
		return nil, err
	}
	return &msgTx, nil
}

// CreateRawTransactionAsync returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See CreateRawTransaction for the blocking version and more details.
func (c *Client) CreateRawTransactionAsync(inputs []btcjson.TransactionInput,
	amounts map[btcutil.Address]btcutil.Amount) FutureCreateRawTransactionResult {

	id := c.NextID()
	convertedAmts := make(map[string]int64, len(amounts))
	for addr, amount := range amounts {
		convertedAmts[addr.String()] = int64(amount)
	}
	cmd, err := btcjson.NewCreateRawTransactionCmd(id, inputs, convertedAmts)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// CreateRawTransaction returns a new transaction spending the provided inputs
// and sending to the provided addresses.
func (c *Client) CreateRawTransaction(inputs []btcjson.TransactionInput,
	amounts map[btcutil.Address]btcutil.Amount) (*btcwire.MsgTx, error) {

	return c.CreateRawTransactionAsync(inputs, amounts).Receive()
}

// FutureSendRawTransactionResult is a future promise to deliver the result
// of a SendRawTransactionAsync RPC invocation (or an applicable error).
type FutureSendRawTransactionResult chan *response

// Receive waits for the response promised by the future and returns the result
// of submitting the encoded transaction to the server which then relays it to
// the network.
func (r FutureSendRawTransactionResult) Receive() (*btcwire.ShaHash, error) {
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

	return btcwire.NewShaHashFromStr(txHashStr)
}

// SendRawTransactionAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See SendRawTransaction for the blocking version and more details.
func (c *Client) SendRawTransactionAsync(tx *btcwire.MsgTx, allowHighFees bool) FutureSendRawTransactionResult {
	txHex := ""
	if tx != nil {
		// Serialize the transaction and convert to hex string.
		buf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
		if err := tx.Serialize(buf); err != nil {
			return newFutureError(err)
		}
		txHex = hex.EncodeToString(buf.Bytes())
	}

	id := c.NextID()
	cmd, err := btcjson.NewSendRawTransactionCmd(id, txHex, allowHighFees)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SendRawTransaction submits the encoded transaction to the server which will
// then relay it to the network.
func (c *Client) SendRawTransaction(tx *btcwire.MsgTx, allowHighFees bool) (*btcwire.ShaHash, error) {
	return c.SendRawTransactionAsync(tx, allowHighFees).Receive()
}

// FutureSignRawTransactionResult is a future promise to deliver the result
// of one of the SignRawTransactionAsync family of RPC invocations (or an
// applicable error).
type FutureSignRawTransactionResult chan *response

// Receive waits for the response promised by the future and returns the
// signed transaction as well as whether or not all inputs are now signed.
func (r FutureSignRawTransactionResult) Receive() (*btcwire.MsgTx, bool, error) {
	res, err := receiveFuture(r)
	if err != nil {
		return nil, false, err
	}

	// Unmarshal as a signrawtransaction result.
	var signRawTxResult btcjson.SignRawTransactionResult
	err = json.Unmarshal(res, &signRawTxResult)
	if err != nil {
		return nil, false, err
	}

	// Decode the serialized transaction hex to raw bytes.
	serializedTx, err := hex.DecodeString(signRawTxResult.Hex)
	if err != nil {
		return nil, false, err
	}

	// Deserialize the transaction and return it.
	var msgTx btcwire.MsgTx
	if err := msgTx.Deserialize(bytes.NewReader(serializedTx)); err != nil {
		return nil, false, err
	}

	return &msgTx, signRawTxResult.Complete, nil
}

// SignRawTransactionAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See SignRawTransaction for the blocking version and more details.
func (c *Client) SignRawTransactionAsync(tx *btcwire.MsgTx) FutureSignRawTransactionResult {
	txHex := ""
	if tx != nil {
		// Serialize the transaction and convert to hex string.
		buf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
		if err := tx.Serialize(buf); err != nil {
			return newFutureError(err)
		}
		txHex = hex.EncodeToString(buf.Bytes())
	}

	id := c.NextID()
	cmd, err := btcjson.NewSignRawTransactionCmd(id, txHex)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SignRawTransaction signs inputs for the passed transaction and returns the
// signed transaction as well as whether or not all inputs are now signed.
//
// This function assumes the RPC server already knows the input transactions and
// private keys for the passed transaction which needs to be signed and uses the
// default signature hash type.  Use one of the SignRawTransaction# variants to
// specify that information if needed.
func (c *Client) SignRawTransaction(tx *btcwire.MsgTx) (*btcwire.MsgTx, bool, error) {
	return c.SignRawTransactionAsync(tx).Receive()
}

// SignRawTransaction2Async returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See SignRawTransaction2 for the blocking version and more details.
func (c *Client) SignRawTransaction2Async(tx *btcwire.MsgTx, inputs []btcjson.RawTxInput) FutureSignRawTransactionResult {
	txHex := ""
	if tx != nil {
		// Serialize the transaction and convert to hex string.
		buf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
		if err := tx.Serialize(buf); err != nil {
			return newFutureError(err)
		}
		txHex = hex.EncodeToString(buf.Bytes())
	}

	id := c.NextID()
	cmd, err := btcjson.NewSignRawTransactionCmd(id, txHex, inputs)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SignRawTransaction2 signs inputs for the passed transaction given the list
// of information about the input transactions needed to perform the signing
// process.
//
// This only input transactions that need to be specified are ones the
// RPC server does not already know.  Already known input transactions will be
// merged with the specified transactions.
//
// See SignRawTransaction if the RPC server already knows the input
// transactions.
func (c *Client) SignRawTransaction2(tx *btcwire.MsgTx, inputs []btcjson.RawTxInput) (*btcwire.MsgTx, bool, error) {
	return c.SignRawTransaction2Async(tx, inputs).Receive()
}

// SignRawTransaction3Async returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See SignRawTransaction3 for the blocking version and more details.
func (c *Client) SignRawTransaction3Async(tx *btcwire.MsgTx,
	inputs []btcjson.RawTxInput,
	privKeysWIF []string) FutureSignRawTransactionResult {

	txHex := ""
	if tx != nil {
		// Serialize the transaction and convert to hex string.
		buf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
		if err := tx.Serialize(buf); err != nil {
			return newFutureError(err)
		}
		txHex = hex.EncodeToString(buf.Bytes())
	}

	id := c.NextID()
	cmd, err := btcjson.NewSignRawTransactionCmd(id, txHex, inputs,
		privKeysWIF)
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SignRawTransaction3 signs inputs for the passed transaction given the list
// of information about extra input transactions and a list of private keys
// needed to perform the signing process.  The private keys must be in wallet
// import format (WIF).
//
// This only input transactions that need to be specified are ones the
// RPC server does not already know.  Already known input transactions will be
// merged with the specified transactions.  This means the list of transaction
// inputs can be nil if the RPC server already knows them all.
//
// NOTE: Unlike the merging functionality of the input transactions, ONLY the
// specified private keys will be used, so even if the server already knows some
// of the private keys, they will NOT be used.
//
// See SignRawTransaction if the RPC server already knows the input
// transactions and private keys or SignRawTransaction2 if it already knows the
// private keys.
func (c *Client) SignRawTransaction3(tx *btcwire.MsgTx,
	inputs []btcjson.RawTxInput,
	privKeysWIF []string) (*btcwire.MsgTx, bool, error) {

	return c.SignRawTransaction3Async(tx, inputs, privKeysWIF).Receive()
}

// SignRawTransaction4Async returns an instance of a type that can be used to
// get the result of the RPC at some future time by invoking the Receive
// function on the returned instance.
//
// See SignRawTransaction4 for the blocking version and more details.
func (c *Client) SignRawTransaction4Async(tx *btcwire.MsgTx,
	inputs []btcjson.RawTxInput, privKeysWIF []string,
	hashType SigHashType) FutureSignRawTransactionResult {

	txHex := ""
	if tx != nil {
		// Serialize the transaction and convert to hex string.
		buf := bytes.NewBuffer(make([]byte, 0, tx.SerializeSize()))
		if err := tx.Serialize(buf); err != nil {
			return newFutureError(err)
		}
		txHex = hex.EncodeToString(buf.Bytes())
	}

	id := c.NextID()
	cmd, err := btcjson.NewSignRawTransactionCmd(id, txHex, inputs,
		privKeysWIF, string(hashType))
	if err != nil {
		return newFutureError(err)
	}

	return c.sendCmd(cmd)
}

// SignRawTransaction4 signs inputs for the passed transaction using the
// the specified signature hash type given the list of information about extra
// input transactions and a potential list of private keys needed to perform
// the signing process.  The private keys, if specified, must be in wallet
// import format (WIF).
//
// The only input transactions that need to be specified are ones the RPC server
// does not already know.  This means the list of transaction inputs can be nil
// if the RPC server already knows them all.
//
// NOTE: Unlike the merging functionality of the input transactions, ONLY the
// specified private keys will be used, so even if the server already knows some
// of the private keys, they will NOT be used.  The list of private keys can be
// nil in which case any private keys the RPC server knows will be used.
//
// This function should only used if a non-default signature hash type is
// desired.  Otherwise, see SignRawTransaction if the RPC server already knows
// the input transactions and private keys, SignRawTransaction2 if it already
// knows the private keys, or SignRawTransaction3 if it does not know both.
func (c *Client) SignRawTransaction4(tx *btcwire.MsgTx,
	inputs []btcjson.RawTxInput, privKeysWIF []string,
	hashType SigHashType) (*btcwire.MsgTx, bool, error) {

	return c.SignRawTransaction4Async(tx, inputs, privKeysWIF,
		hashType).Receive()
}
