Bitcoin Core Batch HTTP POST Example
==============================

This example shows how to use the rpclient package to connect to a Bitcoin Core RPC server using HTTP POST and batch JSON-RPC mode with TLS disabled.

## Running the Example

The first step is to use `go get` to download and install the rpcclient package:

```bash
$ go get github.com/btcsuite/btcd/rpcclient
```

Next, modify the `main.go` source to specify the correct RPC username and
password for the RPC server:

```Go
	User: "yourrpcuser",
	Pass: "yourrpcpass",
```

Finally, navigate to the example's directory and run it with:

```bash
$ cd $GOPATH/src/github.com/btcsuite/btcd/rpcclient/examples/bitcoincorehttp
$ go run *.go
```

## License

This example is licensed under the [copyfree](http://copyfree.org) ISC License.
