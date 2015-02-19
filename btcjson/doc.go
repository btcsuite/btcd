// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package btcjson implements the bitcoin JSON-RPC API.

A complete description of the JSON-RPC protocol as used by bitcoin can
be found on the official wiki at
https://en.bitcoin.it/wiki/API_reference_%28JSON-RPC%29 with a list of
all the supported calls at
https://en.bitcoin.it/wiki/Original_Bitcoin_client/API_Calls_list.

This package provides data structures and code for marshalling and
unmarshalling json for communicating with a running instance of btcd
or bitcoind/bitcoin-qt.  It also provides code for sending those
messages.  It does not provide any code for the client to actually deal
with the messages.  Although it is meant primarily for btcd, it is
possible to use this code elsewhere for interacting with a bitcoin
client programatically.

Protocol

All messages to bitcoin are of the form:

 {"jsonrpc":"1.0","id":"SOMEID","method":"SOMEMETHOD","params":SOMEPARAMS}

The params field can vary in what it contains depending on the
different method (or command) being sent.

Replies will vary in form for the different commands.  The basic form is:

 {"result":SOMETHING,"error":null,"id":"SOMEID"}

The result field can be as simple as an int, or a complex structure
containing many nested fields.  For cases where we have already worked
out the possible types of reply, result is unmarshalled into a
structure that matches the command.  For others, an interface is
returned.  An interface is not as convenient as one needs to do a type
assertion first before using the value, but it means we can handle
arbitrary replies.

The error field is null when there is no error.  When there is an
error it will return a numeric error code as well as a message
describing the error.

id is simply the id of the requester.

RPC Server Authentication

All RPC calls must be authenticated with a username and password.  The specifics
on how to configure the RPC server varies depending on the specific bitcoin
implementation.  For bitcoind, this is accomplished by setting rpcuser and
rpcpassword in the file ~/.bitcoin/bitcoin.conf.  For btcd, this is accomplished
by setting rpcuser and rpcpass in the file ~/.btcd/btcd.conf.  The default local
address of bitcoind is 127.0.0.1:8332 and the default local address of btcd is
127.0.0.1:8334

Usage

The general pattern to use this package consists of generating a message (see
the full list on the official bitcoin wiki), sending the message, and handling
the result after asserting its type.

For commands where the reply structure is known, such as getinfo, one can
directly access the fields in the Reply structure by type asserting the
reply to the appropriate concrete type.

	// Create a getinfo command.
	id := 1
	cmd, err := btcjson.NewGetInfoCmd(id)
	if err != nil {
		// Log and handle error.
	}

	// Send the message to server using the appropriate username and
	// password.
	reply, err := btcjson.RpcSend(user, password, server, cmd)
	if err != nil {
		fmt.Println(err)
		// Log and handle error.
	}

	// Ensure there is a result and type assert it to a btcjson.InfoResult.
	if reply.Result != nil {
		if info, ok := reply.Result.(*btcjson.InfoResult); ok {
			fmt.Println("balance =", info.Balance)
		}
	}

For other commands where this package does not yet provide a concrete
implementation for the reply, such as getrawmempool, the reply uses a generic
interface so one can access individual items as follows:

	// Create a getrawmempool command.
	id := 1
	cmd, err := btcjson.NewGetRawMempoolCmd(id)
	if err != nil {
		// Log and handle error.
	}

	// Send the message to server using the appropriate username and
	// password.
	reply, err := btcjson.RpcSend(user, password, server, cmd)
	if err != nil {
		// Log and handle error.
	}

	// Ensure there is a result and type assert it to a string slice.
	if reply.Result != nil {
		if mempool, ok := reply.Result.([]string); ok {
			fmt.Println("num mempool entries =", len(mempool))
		}
	}
*/
package btcjson
