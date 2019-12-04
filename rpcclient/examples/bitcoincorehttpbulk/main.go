package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
)

func main() {
	const (
		url     = "http://localhost:8332"
		rpcUser = "yourrpcuser"
		rpcPass = "yourrpcpass"
	)

	// populate request set
	reqs := []string{
		`{}`,
		`[]`,
		`[1]`,
		`[1,2,3]`,
		`{"foo": "boo"}`,                              // should be an invalid request
		`{"jsonrpc": "1.0", "foo": "boo", "id": "1"}`,
		`{"jsonrpc": "1.0", "method": "getblockcount", "params": [], "id": "1"}`,
		`{"jsonrpc": "1.0", "method": "getblockcount", "params": "a", "id": "1"}`, // should be invalid since params is neither an array nor a json object.
		`[
			{"jsonrpc": "2.0", "method": "getblockcount", "params": [], "id": "1"},
			{"jsonrpc": "2.0", "method": "decodescript", "params": ["ac"]},
			{"jsonrpc": "2.0", "method": "getbestblockhash", "params": [], "id": "2"},
			{"foo": "boo"},
			{"jsonrpc": "2.0", "method": "getblockcount", "id": "9"} 
		]`, // should produce invalid request for the `{"foo": "boo"}`.
	}

	// Connect to local btcd RPC server using websockets.

	client := http.Client{}

	for _, jsonReq := range reqs {
		bodyReader := bytes.NewReader([]byte(jsonReq))
		httpReq, err := http.NewRequest("POST", url, bodyReader)
		if err != nil {
			fmt.Println(err)
			return
		}
		httpReq.Close = true
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.SetBasicAuth(rpcUser, rpcPass)
		resp, err := client.Do(httpReq)
		if err != nil {
			fmt.Println("request:", jsonReq, "response:", err)
			return
		}

		respBytes, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println("request:", jsonReq, "response:", string(respBytes))
	}
}