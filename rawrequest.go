// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcrpcclient

import (
	"encoding/json"
	"errors"

	"github.com/conformal/btcjson"
)

// rawRequest satisifies the btcjson.Cmd interface for btcjson raw commands.
// This type exists here rather than making btcjson.RawCmd satisify the Cmd
// interface due to conflict between the Id and Method field names vs method
// names.
type rawRequest struct {
	btcjson.RawCmd
}

// Enforce that rawRequest is a btcjson.Cmd.
var _ btcjson.Cmd = &rawRequest{}

// Id returns the JSON-RPC id of the request.
func (r *rawRequest) Id() interface{} {
	return r.RawCmd.Id
}

// Method returns the method string of the request.
func (r *rawRequest) Method() string {
	return r.RawCmd.Method
}

// MarshalJSON marshals the raw request as a JSON-RPC 1.0 object.
func (r *rawRequest) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.RawCmd)
}

// UnmarshalJSON unmarshals a JSON-RPC 1.0 object into the raw request.
func (r *rawRequest) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &r.RawCmd)
}

// FutureRawResult is a future promise to deliver the result of a RawRequest RPC
// invocation (or an applicable error).
type FutureRawResult chan *response

// Receive waits for the response promised by the future and returns the raw
// response, or an error if the request was unsuccessful.
func (r FutureRawResult) Receive() (json.RawMessage, error) {
	return receiveFuture(r)
}

// RawRequestAsync returns an instance of a type that can be used to get the
// result of a custom RPC request at some future time by invoking the Receive
// function on the returned instance.
//
// See RawRequest for the blocking version and more details.
func (c *Client) RawRequestAsync(method string, params []json.RawMessage) FutureRawResult {
	// Method may not be empty.
	if method == "" {
		return newFutureError(errors.New("no method"))
	}

	// Marshal parameters as "[]" instead of "null" when no parameters
	// are passed.
	if params == nil {
		params = []json.RawMessage{}
	}

	cmd := &rawRequest{
		RawCmd: btcjson.RawCmd{
			Jsonrpc: "1.0",
			Id:      c.NextID(),
			Method:  method,
			Params:  params,
		},
	}

	return c.sendCmd(cmd)
}

// RawRequest allows the caller to send a raw or custom request to the server.
// This method may be used to send and receive requests and responses for
// requests that are not handled by this client package, or to proxy partially
// unmarshaled requests to another JSON-RPC server if a request cannot be
// handled directly.
func (c *Client) RawRequest(method string, params []json.RawMessage) (json.RawMessage, error) {
	return c.RawRequestAsync(method, params).Receive()
}
