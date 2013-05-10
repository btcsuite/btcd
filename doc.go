// Copyright (c) 2013 Conformal Systems LLC.
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
message.  It does not provide any code for the client to actually deal
with the messages.  Although it is meant primarily for btcd, it is
possible to use this code elsewhere for interacting with a bitcoin
client programatically.

Protocol

All messages to bitcoin are of the form:

 {"jsonrpc":"1.0","id":"SOMEID","method":"SOMEMETHOD","params":SOMEPARAMS}

The params field can vary in what it contains depending on the
different method (or command) being sent.

Replies will vary in form for the different commands.  The basic form is:

 {"result":SOMETHING,"error":null,"id":"btcd"}

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

Usage

To use this package, check it out from github:

 go get github.com/conformal/btcjson

Import it as usual:

 import	"github.com/conformal/btcjson"

Generate the message you want (see the full list on the official bitcoin wiki):

 msg, err := btcjson.CreateMessage("getinfo")

And then send the message:

 reply, err := btcjson.RpcCommand(user, password, server, msg)

Since rpc calls must be authenticated, RpcCommand requires a
username and password along with the address of the server.  For
details, see the documentation for your bitcoin implementation.

For convenience, this can be set for bitcoind by setting rpcuser and
rpcpassword in the file ~/.bitcoin/bitcoin.conf with a default local
address of: 127.0.0.1:8332

For commands where the reply structure is known (such as getblock),
one can directly access the fields in the Reply structure.  For other
commands, the reply uses an interface so one can access individual
items like:

 if reply.Result != nil {
     info := reply.Result.(map[string]interface{})
     balance, ok := info["balance"].(float64)
 }

(with appropriate error checking at all steps of course).

*/
package btcjson
