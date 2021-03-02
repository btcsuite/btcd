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

// FutureGetMasternodeStatusResult is a future promise to deliver the result of a
// MasternodeStatusAsync RPC invocation (or an applicable error).
type FutureGetMasternodeStatusResult struct {
	client   *Client
	Response chan *response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetMasternodeStatusResult) Receive() (*btcjson.MasternodeStatusResult, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	var masternodeStatusResult btcjson.MasternodeStatusResult
	err = json.Unmarshal(res, &masternodeStatusResult)
	if err != nil {
		return nil, err
	}

	return &masternodeStatusResult, nil
}

// MasternodeStatusAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
func (c *Client) MasternodeStatusAsync() FutureGetMasternodeStatusResult {
	cmd := btcjson.NewMasternodeCmd(btcjson.MasternodeStatus)

	return FutureGetMasternodeStatusResult{
		client:   c,
		Response: c.sendCmd(cmd),
	}
}

// MasternodeStatus returns the masternode status.
func (c *Client) MasternodeStatus() (*btcjson.MasternodeStatusResult, error) {
	return c.MasternodeStatusAsync().Receive()
}

// ----------------- masternode count ---------------------

// FutureGetMasternodeCountResult is a future promise to deliver the result of a
// MasternodeStatusAsync RPC invocation (or an applicable error).
type FutureGetMasternodeCountResult struct {
	client   *Client
	Response chan *response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetMasternodeCountResult) Receive() (*btcjson.MasternodeCountResult, error) {
	res, err := receiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	var result btcjson.MasternodeCountResult
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// MasternodeCountAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) MasternodeCountAsync() FutureGetMasternodeCountResult {
	cmd := btcjson.NewMasternodeCmd(btcjson.MasternodeCount)

	return FutureGetMasternodeCountResult{
		client:   c,
		Response: c.sendCmd(cmd),
	}
}

// MasternodeCount returns the masternode count.
func (c *Client) MasternodeCount() (*btcjson.MasternodeCountResult, error) {
	return c.MasternodeCountAsync().Receive()
}
