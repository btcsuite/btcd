Custom Command Example
======================

This example shows how to use custom commands with the rpcclient package, by
implementing the `name_show` command from Namecoin Core.

## Running the Example

The first step is to use `go get` to download and install the rpcclient package:

```bash
$ go get github.com/btcsuite/btcd/rpcclient
```

Next, modify the `main.go` source to specify the correct RPC username and
password for the RPC server of your Namecoin Core node:

```Go
	User: "yourrpcuser",
	Pass: "yourrpcpass",
```

Finally, navigate to the example's directory and run it with:

```bash
$ cd $GOPATH/src/github.com/btcsuite/btcd/rpcclient/examples/customcommand
$ go run *.go
```

## License

This example is licensed under the [copyfree](http://copyfree.org) ISC License.
