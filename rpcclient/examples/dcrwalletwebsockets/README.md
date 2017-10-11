dcrwallet Websockets Example
============================

This example shows how to use the rpcclient package to connect to a dcrwallet
RPC server using TLS-secured websockets, register for notifications about
changes to account balances, and get a list of unspent transaction outputs
(utxos) the wallet can sign.

This example also sets a timer to shutdown the client after 10 seconds to
demonstrate clean shutdown.

## Running the Example

The first step is to use `go get` to download and install the rpcclient package:

```bash
$ go get -u github.com/decred/dcrd/rpcclient
```

Next, modify the `main.go` source to specify the correct RPC username and
password for the RPC server:

```Go
	User: "yourrpcuser",
	Pass: "yourrpcpass",
```

Finally, navigate to the example's directory and run it with:

```bash
$ cd $GOPATH/src/github.com/decred/dcrd/rpcclient/examples/dcrwalletwebsockets
$ go run *.go
```

## License

This example is licensed under the [copyfree](http://copyfree.org) ISC License.
