// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
btcd is a full-node bitcoin implementation written in Go.

The default options are sane for most users.  This means btcd will work 'out of
the box' for most users.  However, there are also a wide variety of flags that
can be used to control it.

The following section provides a usage overview which enumerates the flags.  An
interesting point to note is that the long form of all of these options
(except -C) can be specified in a configuration file that is automatically
parsed when btcd starts up.  By default, the configuration file is located at
~/.ppcd/ppcd.conf on POSIX-style operating systems and %LOCALAPPDATA%\ppcd\ppcd.conf
on Windows.  The -C (--configfile) flag, as shown below, can be used to override
this location.

Usage:
  btcd [OPTIONS]

Application Options:
  -V, --version            Display version information and exit
  -C, --configfile=        Path to configuration file
  -b, --datadir=           Directory to store data
  -a, --addpeer=           Add a peer to connect with at startup
      --connect=           Connect only to the specified peers at startup
      --nolisten           Disable listening for incoming connections -- NOTE:
                           Listening is automatically disabled if the --connect
                           or --proxy options are used without also specifying
                           listen interfaces via --listen
      --listen=            Add an interface/port to listen for connections
                           (default all interfaces port: 8333, testnet: 18333)
      --maxpeers=          Max number of inbound and outbound peers (125)
      --banduration=       How long to ban misbehaving peers.  Valid time units
                           are {s, m, h}.  Minimum 1 second (24h0m0s)
  -u, --rpcuser=           Username for RPC connections
  -P, --rpcpass=           Password for RPC connections
      --rpclisten=         Add an interface/port to listen for RPC connections
                           (default port: 8334, testnet: 18334)
      --rpccert=           File containing the certificate file
      --rpckey=            File containing the certificate key
      --rpcmaxclients=     Max number of RPC clients for standard connections
                           (10)
      --rpcmaxwebsockets=  Max number of RPC clients for standard connections
                           (25)
      --norpc              Disable built-in RPC server -- NOTE: The RPC server
                           is disabled by default if no rpcuser/rpcpass is
                           specified
      --nodnsseed          Disable DNS seeding for peers
      --externalip:        Add an ip to the list of local addresses we claim to
                           listen on to peers
      --proxy=             Connect via SOCKS5 proxy (eg. 127.0.0.1:9050)
      --proxyuser=         Username for proxy server
      --proxypass=         Password for proxy server
      --onion=             Connect to tor hidden services via SOCKS5 proxy (eg.
                           127.0.0.1:9050)
      --onionuser=         Username for onion proxy server
      --onionpass=         Password for onion proxy server
      --noonion=           Disable connecting to tor hidden services
      --tor=               Specifies the proxy server used is a Tor node
      --testnet=           Use the test network
      --regtest=           Use the regression test network
      --nocheckpoints=     Disable built-in checkpoints.  Don't do this unless
                           you know what you're doing.
      --dbtype=            Database backend to use for the Block Chain (leveldb)
      --profile=           Enable HTTP profiling on given port -- NOTE port must
                           be between 1024 and 65536 (6060)
      --cpuprofile=        Write CPU profile to the specified file
  -d, --debuglevel:        Logging level for all subsystems {trace, debug, info,
                           warn, error, critical} -- You may also specify
                           <subsystem>=<level>,<subsystem2>=<level>,... to set
                           the log level for individual subsystems -- Use show
                           to list available subsystems (info)
      --upnp               Use UPnP to map our listening port outside of NAT
      --limitfreerelay=    Limit relay of transactions with no transaction fee
                           to the given amount in thousands of bytes per minute
                           (15)

      --generate=          Generate (mine) bitcoins using the CPU
      --miningaddr=        Add the specified payment address to the list of
                           addresses to use for generated blocks -- At least
                           one address is required if the generate option is set
      --blockminsize=      Mininum block size in bytes to be used when creating
                           a block
      --blockmaxsize=      Maximum block size in bytes to be used when creating
                           a block (750000)
      --blockprioritysize= Size in bytes for high-priority/low-fee transactions
                           when creating a block (50000)
      --getworkkey=        DEPRECATED -- Use the --miningaddr option instead
Help Options:
  -h, --help           Show this help message

*/
package main
