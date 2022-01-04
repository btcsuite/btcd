# lbcd Websockets Example

This example shows how to use the rpcclient package to connect to a btcd RPC
server using TLS-secured websockets, register for block connected and block
disconnected notifications, and get the current block count.

## Running the Example

The first step is to clone the lbcd package:

```bash
$ git clone github.com/lbryio/lbcd
```

Display available options:

```bash
$ go run . -h

  -coinid string
        Coin ID (default "1425")
  -rpcpass string
        LBCD RPC password (default "rpcpass")
  -rpcserver string
        LBCD RPC server (default "localhost:9245")
  -rpcuser string
        LBCD RPC username (default "rpcuser")
  -stratum string
        Stratum server (default "lbrypool.net:3334")
  -stratumpass string
        Stratum server password (default "password")
```

Start the program:

```bash
$ go run . -stratumpass <STRATUM PASSWD> -rpcuser <RPC USERNAME> -rpcpass <RPC PASSWD>

2022/01/10 23:16:21 NotifyBlocks: Registration Complete
2022/01/10 23:16:21 Block count: 1093112
...
```

## License

This example is licensed under the [copyfree](http://copyfree.org) ISC License.
