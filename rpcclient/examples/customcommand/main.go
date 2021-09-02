// Copyright (c) 2014-2017 The btcsuite developers
// Copyright (c) 2019-2020 The Namecoin developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"log"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/rpcclient"
)

// NameShowCmd defines the name_show JSON-RPC command.
type NameShowCmd struct {
	Name string
}

// NameShowResult models the data from the name_show command.
type NameShowResult struct {
	Name          string `json:"name"`
	NameEncoding  string `json:"name_encoding"`
	NameError     string `json:"name_error"`
	Value         string `json:"value"`
	ValueEncoding string `json:"value_encoding"`
	ValueError    string `json:"value_error"`
	TxID          string `json:"txid"`
	Vout          uint32 `json:"vout"`
	Address       string `json:"address"`
	IsMine        bool   `json:"ismine"`
	Height        int32  `json:"height"`
	ExpiresIn     int32  `json:"expires_in"`
	Expired       bool   `json:"expired"`
}

// FutureNameShowResult is a future promise to deliver the result
// of a NameShowAsync RPC invocation (or an applicable error).
type FutureNameShowResult chan *rpcclient.Response

// Receive waits for the Response promised by the future and returns detailed
// information about a name.
func (r FutureNameShowResult) Receive() (*NameShowResult, error) {
	res, err := rpcclient.ReceiveFuture(r)
	if err != nil {
		return nil, err
	}

	// Unmarshal result as a name_show result object
	var nameShow NameShowResult
	err = json.Unmarshal(res, &nameShow)
	if err != nil {
		return nil, err
	}

	return &nameShow, nil
}

// NameShowAsync returns an instance of a type that can be used to get the
// result of the RPC at some future time by invoking the Receive function on
// the returned instance.
//
// See NameShow for the blocking version and more details.
func NameShowAsync(c *rpcclient.Client, name string) FutureNameShowResult {
	cmd := &NameShowCmd{
		Name: name,
	}
	return c.SendCmd(cmd)
}

// NameShow returns detailed information about a name.
func NameShow(c *rpcclient.Client, name string) (*NameShowResult, error) {
	return NameShowAsync(c, name).Receive()
}

func init() {
	// No special flags for commands in this file.
	flags := btcjson.UsageFlag(0)

	btcjson.MustRegisterCmd("name_show", (*NameShowCmd)(nil), flags)
}

func main() {
	// Connect to local namecoin core RPC server using HTTP POST mode.
	connCfg := &rpcclient.ConnConfig{
		Host:         "localhost:8336",
		User:         "yourrpcuser",
		Pass:         "yourrpcpass",
		HTTPPostMode: true, // Namecoin core only supports HTTP POST mode
		DisableTLS:   true, // Namecoin core does not provide TLS by default
	}
	// Notice the notification parameter is nil since notifications are
	// not supported in HTTP POST mode.
	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Shutdown()

	// Get the current block count.
	result, err := NameShow(client, "d/namecoin")
	if err != nil {
		log.Fatal(err)
	}

	value := result.Value

	log.Printf("Value of d/namecoin: %s", value)
}
