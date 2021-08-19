# Configuration

lbcd has a number of configuration
options, which can be viewed by running: `$ lbcd --help`.

## Peer server listen interface

lbcd allows you to bind to specific interfaces which enables you to setup
configurations with varying levels of complexity.  The listen parameter can be
specified on the command line as shown below with the -- prefix or in the
configuration file without the -- prefix (as can all long command line options).
The configuration file takes one entry per line.

**NOTE:** The listen flag can be specified multiple times to listen on multiple
interfaces as a couple of the examples below illustrate.

Command Line Examples:

| Flags                                       | Comment                                                                                      |
| ------------------------------------------- | -------------------------------------------------------------------------------------------- |
| --listen=                                   | all interfaces on default port which is changed by `--testnet` and `--regtest` (**default**) |
| --listen=0.0.0.0                            | all IPv4 interfaces on default port which is changed by `--testnet` and `--regtest`          |
| --listen=::                                 | all IPv6 interfaces on default port which is changed by `--testnet` and `--regtest`          |
| --listen=:9246                              | all interfaces on port 9246                                                                  |
| --listen=0.0.0.0:9246                       | all IPv4 interfaces on port 9246                                                             |
| --listen=[::]:9246                          | all IPv6 interfaces on port 9246                                                             |
| --listen=127.0.0.1:9246                     | only IPv4 localhost on port 9246                                                             |
| --listen=[::1]:9246                         | only IPv6 localhost on port 9246                                                             |
| --listen=:9247                              | all interfaces on non-standard port 9247                                                     |
| --listen=0.0.0.0:9247                       | all IPv4 interfaces on non-standard port 9247                                                |
| --listen=[::]:9247                          | all IPv6 interfaces on non-standard port 9247                                                |
| --listen=127.0.0.1:9248 --listen=[::1]:9246 | IPv4 localhost on port 9248 and IPv6 localhost on port 9246                                  |
| --listen=:9246 --listen=:9248               | all interfaces on ports 9246 and 9248                                                        |

The following config file would configure lbcd to only listen on localhost for both IPv4 and IPv6:

```text
[Application Options]

listen=127.0.0.1:9246
listen=[::1]:9246
```

In addition, if you are starting lbcd with TLS and want to make it
available via a hostname, then you will need to generate the TLS
certificates for that host. For example,

```
gencerts --host=myhostname.example.com --directory=/home/me/.lbcd/
```

## RPC server listen interface

lbcd allows you to bind the RPC server to specific interfaces which enables you
to setup configurations with varying levels of complexity.  The `rpclisten`
parameter can be specified on the command line as shown below with the -- prefix
or in the configuration file without the -- prefix (as can all long command line
options).  The configuration file takes one entry per line.

A few things to note regarding the RPC server:

* The RPC server will **not** be enabled unless the `rpcuser` and `rpcpass`
  options are specified.
* When the `rpcuser` and `rpcpass` and/or `rpclimituser` and `rpclimitpass`
  options are specified, the RPC server will only listen on localhost IPv4 and
  IPv6 interfaces by default.  You will need to override the RPC listen
  interfaces to include external interfaces if you want to connect from a remote
  machine.
* The RPC server has TLS enabled by default, even for localhost.  You may use
  the `--notls` option to disable it, but only when all listeners are on
  localhost interfaces.
* The `--rpclisten` flag can be specified multiple times to listen on multiple
  interfaces as a couple of the examples below illustrate.
* The RPC server is disabled by default when using the `--regtest` and
  `--simnet` networks.  You can override this by specifying listen interfaces.

Command Line Examples:

| Flags                                          | Comment                                                             |
| ---------------------------------------------- | ------------------------------------------------------------------- |
| --rpclisten=                                   | all interfaces on default port which is changed by `--testnet`      |
| --rpclisten=0.0.0.0                            | all IPv4 interfaces on default port which is changed by `--testnet` |
| --rpclisten=::                                 | all IPv6 interfaces on default port which is changed by `--testnet` |
| --rpclisten=:9245                              | all interfaces on port 9245                                         |
| --rpclisten=0.0.0.0:9245                       | all IPv4 interfaces on port 9245                                    |
| --rpclisten=[::]:9245                          | all IPv6 interfaces on port 9245                                    |
| --rpclisten=127.0.0.1:9245                     | only IPv4 localhost on port 9245                                    |
| --rpclisten=[::1]:9245                         | only IPv6 localhost on port 9245                                    |
| --rpclisten=:9247                              | all interfaces on non-standard port 9247                            |
| --rpclisten=0.0.0.0:9247                       | all IPv4 interfaces on non-standard port 9247                       |
| --rpclisten=[::]:9247                          | all IPv6 interfaces on non-standard port 9247                       |
| --rpclisten=127.0.0.1:9248 --listen=[::1]:9245 | IPv4 localhost on port 9248 and IPv6 localhost on port 9245         |
| --rpclisten=:9245 --listen=:9248               | all interfaces on ports 9245 and 9248                               |

The following config file would configure the lbcd RPC server to listen to all interfaces on the default port, including external interfaces, for both IPv4 and IPv6:

```text
[Application Options]

rpclisten=
```

## Default ports

While lbcd is highly configurable when it comes to the network configuration,
the following is intended to be a quick reference for the default ports used so
port forwarding can be configured as required.

lbcd by default will automatically map the
peer-to-peer listening port if your router supports UPnP.  If your router does
not support UPnP, or you don't wish to use it, please note that only the
peer-to-peer port should be forwarded unless you specifically want to allow RPC
access to your lbcd from external sources such as in more advanced network
configurations. You can disable UPnP with the `--noupnp` daemon option.

| Name                      | Port     |
| ------------------------- | -------- |
| Default peer-to-peer port | TCP 9246 |
| Default RPC port          | TCP 9245 |

## Using bootstrap.dat

### What is bootstrap.dat?

It is a flat, binary file containing bitcoin blockchain data starting from the
genesis block and continuing through a relatively recent block height depending
on the last time it was updated.

See [this](https://bitcointalk.org/index.php?topic=145386.0) thread on
bitcointalk for more details.

**NOTE:** Using bootstrap.dat is entirely optional.  lbcd will download the
block chain from other peers through the Bitcoin protocol with no extra
configuration needed.

### What are the pros and cons of using bootstrap.dat?

Pros:

* Typically accelerates the initial process of bringing up a new node as it
  downloads from public P2P nodes and generally is able to achieve faster
  download speeds
* It is particularly beneficial when bringing up multiple nodes as you only need
  to download the data once

Cons:

* Requires you to setup and configure a torrent client if you don't already have
  one available
* Requires roughly twice as much disk space since you'll need the flat file as
  well as the imported database

### Where do I get bootstrap.dat?

The bootstrap.dat file is made available via a torrent.  See
[this](https://bitcointalk.org/index.php?topic=145386.0) thread on bitcointalk
for the torrent download details.

### How do I know I can trust the bootstrap.dat I downloaded?

You don't need to trust the file as the `addblock` utility verifies every block
using the same rules that are used when downloading the block chain normally
through the Bitcoin protocol.  Additionally, the chain rules contain hard-coded
checkpoints for the known-good block chain at periodic intervals.  This ensures
that not only is it a valid chain, but it is the same chain that everyone else
is using.

### How do I use bootstrap.dat with lbcd?

lbcd comes with a separate utility named `addblock` which can be used to import
`bootstrap.dat`.  This approach is used since the import is a one-time operation
and we prefer to keep the daemon itself as lightweight as possible.

1. Stop lbcd if it is already running.  This is required since addblock needs to
   access the database used by lbcd and it will be locked if lbcd is using it.
2. Note the path to the downloaded bootstrap.dat file.
3. Run the addblock utility with the `-i` argument pointing to the location of
   boostrap.dat:

**Windows:**

```bat
"%PROGRAMFILES%\lbcd Suite\lbcd\addblock" -i C:\Path\To\bootstrap.dat
```

**Linux/Unix/BSD/POSIX:**

```bash
$GOPATH/bin/addblock -i /path/to/bootstrap.dat
```
