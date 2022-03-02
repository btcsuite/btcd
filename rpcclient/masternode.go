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
	Response chan *Response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetMasternodeStatusResult) Receive() (*btcjson.MasternodeStatusResult, error) {
	res, err := ReceiveFuture(r.Response)
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
		Response: c.SendCmd(cmd),
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
	Response chan *Response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetMasternodeCountResult) Receive() (*btcjson.MasternodeCountResult, error) {
	res, err := ReceiveFuture(r.Response)
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
		Response: c.SendCmd(cmd),
	}
}

// MasternodeCount returns the masternode count.
func (c *Client) MasternodeCount() (*btcjson.MasternodeCountResult, error) {
	return c.MasternodeCountAsync().Receive()
}

// ----------------- masternode current ---------------------

// FutureGetMasternodeResult is a future promise to deliver the result of a
// MasternodeResultAsync RPC invocation (or an applicable error).
type FutureGetMasternodeResult struct {
	client   *Client
	Response chan *Response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetMasternodeResult) Receive() (*btcjson.MasternodeResult, error) {
	res, err := ReceiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	var result btcjson.MasternodeResult
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// MasternodeCurrentAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) MasternodeCurrentAsync() FutureGetMasternodeResult {
	cmd := btcjson.NewMasternodeCmd(btcjson.MasternodeCurrent)

	return FutureGetMasternodeResult{
		client:   c,
		Response: c.SendCmd(cmd),
	}
}

// MasternodeCurrent returns the masternode count.
func (c *Client) MasternodeCurrent() (*btcjson.MasternodeResult, error) {
	return c.MasternodeCurrentAsync().Receive()
}

// ----------------- masternode outputs ---------------------

// FutureGetMasternodeOutputsResult is a future promise to deliver the result of a
// MasternodeStatusAsync RPC invocation (or an applicable error).
type FutureGetMasternodeOutputsResult struct {
	client   *Client
	Response chan *Response
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetMasternodeOutputsResult) Receive() (map[string]string, error) {
	res, err := ReceiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	var result map[string]string
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// MasternodeOutputsAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) MasternodeOutputsAsync() FutureGetMasternodeOutputsResult {
	cmd := btcjson.NewMasternodeCmd(btcjson.MasternodeOutputs)

	return FutureGetMasternodeOutputsResult{
		client:   c,
		Response: c.SendCmd(cmd),
	}
}

// MasternodeOutputs returns the masternode count.
func (c *Client) MasternodeOutputs() (map[string]string, error) {
	return c.MasternodeOutputsAsync().Receive()
}

// ----------------- masternode winner ---------------------

// MasternodeWinnerAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) MasternodeWinnerAsync() FutureGetMasternodeResult {
	cmd := btcjson.NewMasternodeCmd(btcjson.MasternodeWinner)

	return FutureGetMasternodeResult{
		client:   c,
		Response: c.SendCmd(cmd),
	}
}

// MasternodeWinner returns the masternode count.
func (c *Client) MasternodeWinner() (*btcjson.MasternodeResult, error) {
	return c.MasternodeWinnerAsync().Receive()
}

// ----------------- masternode winners ---------------------

// FutureGetMasternodeWinnersResult is a future promise to deliver the result of a
// MasternodeWinnersAsync RPC invocation (or an applicable error).
type FutureGetMasternodeWinnersResult struct {
	client   *Client
	Response chan *Response
}

// Receive waits for the response promised by the future and returns masternode winners result
func (r FutureGetMasternodeWinnersResult) Receive() (map[string]string, error) {
	res, err := ReceiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	var result map[string]string
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// MasternodeWinnersAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) MasternodeWinnersAsync(count int, filter string) FutureGetMasternodeWinnersResult {
	cmd := btcjson.NewMasternodeWinnersCmd(count, filter)

	return FutureGetMasternodeWinnersResult{
		client:   c,
		Response: c.SendCmd(cmd),
	}
}

// MasternodeWinners returns the masternode winners.
func (c *Client) MasternodeWinners(count int, filter string) (map[string]string, error) {
	return c.MasternodeWinnersAsync(count, filter).Receive()
}

// ----------------- masternodelist ---------------------

// FutureGetMasternodeListResult is a future promise to deliver the result of a
// MasternodeListAsync RPC invocation (or an applicable error).
type FutureGetMasternodeListResult struct {
	client   *Client
	Response chan *Response
	Mode     string
}

// Receive waits for the response promised by the future and returns the member signature for the quorum.
func (r FutureGetMasternodeListResult) Receive() (interface{}, error) {
	res, err := ReceiveFuture(r.Response)
	if err != nil {
		return nil, err
	}

	if r.Mode == "" || r.Mode == "json" {
		var result map[string]btcjson.MasternodelistResultJSON
		err = json.Unmarshal(res, &result)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	var result map[string]string
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// MasternodeListAsync returns an instance of a type that can be used to get
// the result of the RPC at some future time by invoking the Receive function on
// the returned instance.
func (c *Client) MasternodeListAsync(mode, filter string) FutureGetMasternodeListResult {
	cmd := btcjson.NewMasternodeListCmd(mode, filter)

	return FutureGetMasternodeListResult{
		client:   c,
		Response: c.SendCmd(cmd),
		Mode:     mode,
	}
}

// MasternodeList returns the masternodelist.
func (c *Client) MasternodeList(mode, filter string) (interface{}, error) {
	return c.MasternodeListAsync(mode, filter).Receive()
}

// MasternodeListJSON retruns the resullt for mode json
func (c *Client) MasternodeListJSON(filter string) (map[string]btcjson.MasternodelistResultJSON, error) {
	r, err := c.MasternodeListAsync("json", filter).Receive()
	if err != nil {
		return nil, err
	}
	return r.(map[string]btcjson.MasternodelistResultJSON), nil
}
