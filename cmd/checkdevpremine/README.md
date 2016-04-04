checkdevpremine
===============

The checkdevpremine utility allows transactions to be tested whether or not they
have inputs that trace back to the original dev premine coins.

It works by using the dcrd RPC server to request all of the relevant input
transactions all the way back to the coinbase output that orignated the coins.
Those coinbase outpoints are then compiled into a list and compared against the
original dev premine outpoints.  This is also known as checking for taint.

The utility only accepts one parameter which can either be a single transaction
hash or a JSON-array of transaction hashes in order to facilite checking
multiple at once.  That parameter may also be a single dash, `-`, in order to
indicate it should be read from stdin.

Any outpoints which are found to be part of the original dev premine will be
listed unless the `--quiet` flag is provided to suppress the output.

In addition, in order to facilitate programmatic access the tool returns the
following codes to the Operating System:

|Return Code|Description|
|---|---|
|0|The transaction(s) do _NOT_ have any inputs which trace back to the dev premine coins|
|1|One of more of the transactions _DO_ have at least one input which traces back to the dev premine coins|
|2|Some type of error such as inability to talk to the RPC server occurred|

These codes in addition with the `--quiet` flag allow a fully automated check
with no visible output.

## Configuring

In order to connect and authenticate to the dcrd RPC server, the `--rpcuser` and
`--rpcpass` options must be specified.  These can be placed into a config file
named `checkdevpremine.conf` at the location shown in the help output of the
utility (`checkdevpremine -h`).

Config file example:

```
rpcuser=your_dcrd_RPC_server_username
rpcpass=your_dcrd_RPC_server_password
```

## Example Usage

Checking a single transaction with visible output:
```bash
$ checkdevpremine 25afd7d33ceb8698f5d81eb7ee14a7532419ce1e1c65cc5032e37696f26c5cac
```

Checking for multiple transactions with visible output:
```bash
$ checkdevpremine "[\"25afd7d33ceb8698f5d81eb7ee14a7532419ce1e1c65cc5032e37696f26c5cac\", \"b1527b63c7a76ea28e57604082bec0e8195cd7a33bd10c68296a45bce77bd2db\"]"
```

Mixing tools in order to check all of the transactions in the latest block using
[jq](https://stedolan.github.io/jq/) to extract the transaction hashes and feed
them to `checkdevpremine` via stdin along with `--quiet` to suppress output:
```bash
$ dcrctl getbestblockhash | dcrctl getblock - | jq -c .tx | checkdevpremine --quiet -; echo $?
0
```

## Installation and Updating

```bash
$ go get -u github.com/decred/dcrd/cmd/checkdevpremine
```

## License

The checkdevpremine utility is licensed under the [copyfree](http://copyfree.org)
ISC License.
