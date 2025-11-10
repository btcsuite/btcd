Bitcoin Core HTTP POST Over Unix Socket Example
==============================

This example shows how to use the rpcclient package to connect to a Bitcoin
Core RPC server using HTTP POST mode over a Unix Socket with TLS disabled 
and gets the current block count.

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

As Bitcoin Core supports only TCP/IP, we'll redirect RPC requests from the 
Unix Socket to Bitcoin Core. For this example, we'll use the `socat` command:

```bash
$ socat -d UNIX-LISTEN:"my-unix-socket-path",fork TCP:"host-address"
$ socat -d UNIX-LISTEN:/tmp/test.XXXX,fork TCP:localhost:8332
```

Finally, navigate to the example's directory and run it with:

```bash
$ cd $GOPATH/src/github.com/btcsuite/btcd/rpcclient/examples/bitcoincorehttp
$ go run *.go
```

## License

This example is licensed under the [copyfree](http://copyfree.org) ISC License.
