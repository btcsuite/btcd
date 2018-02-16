// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package sampleconfig

// FileContents is a string containing the commented example config for dcrd.
const FileContents = `[Application Options]

; ------------------------------------------------------------------------------
; Data settings
; ------------------------------------------------------------------------------

; The directory to store data such as the block chain and peer addresses.  The
; block chain takes several GB, so this location must have a lot of free space.
; The default is ~/.dcrd/data on POSIX OSes, $LOCALAPPDATA/Dcrd/data on Windows,
; ~/Library/Application Support/Dcrd/data on macOS, and $homed/dcrd/data on
; Plan9.  Environment variables are expanded so they may be used.  NOTE: Windows
; environment variables are typically %VARIABLE%, but they must be accessed with
; $VARIABLE here.
; datadir=~/.dcrd/data                            ; Unix
; datadir=$LOCALAPPDATA/Dcrd/data                 ; Windows
; datadir=~/Library/Application Support/Dcrd/data ; macOS


; ------------------------------------------------------------------------------
; Network settings
; ------------------------------------------------------------------------------

; Use testnet.
; testnet=1

; Use simnet.
; simnet=1

; Connect via a SOCKS5 proxy.  NOTE: Specifying a proxy will disable listening
; for incoming connections unless listen addresses are provided via the 'listen'
; option.
; proxy=127.0.0.1:9050
; proxyuser=
; proxypass=

; The SOCKS5 proxy above is assumed to be Tor (https://www.torproject.org).
; If the proxy  is not tor the the following my be used to prevent using
; tor specific SOCKS queries to lookup addresses (this increases anonymity when
; tor is used by preventing your IP being leaked via DNS).
; noonion=1

; Use an alternative proxy to connect to .onion addresses. The proxy is assumed
; to be a Tor node. Non .onion addresses will be contacted with the main proxy
; or without a proxy if none is set.
; onion=127.0.0.1:9051
; onionuser=
; onionpass=

; Enable Tor stream isolation by randomizing proxy user credentials resulting in
; Tor creating a new circuit for each connection.  This makes it more difficult
; to correlate connections.
; torisolation=1

; Use Universal Plug and Play (UPnP) to automatically open the listen port
; and obtain the external IP address from supported devices.  NOTE: This option
; will have no effect if exernal IP addresses are specified.
; upnp=1

; Specify the external IP addresses your node is listening on.  One address per
; line.  dcrd will not contact 3rd-party sites to obtain external ip addresses.
; This means if you are behind NAT, your node will not be able to advertise a
; reachable address unless you specify it here or enable the 'upnp' option (and
; have a supported device).
; externalip=1.2.3.4
; externalip=2002::1234

; ******************************************************************************
; Summary of 'addpeer' versus 'connect'.
;
; Only one of the following two options, 'addpeer' and 'connect', may be
; specified.  Both allow you to specify peers that you want to stay connected
; with, but the behavior is slightly different.  By default, dcrd will query DNS
; to find peers to connect to, so unless you have a specific reason such as
; those described below, you probably won't need to modify anything here.
;
; 'addpeer' does not prevent connections to other peers discovered from
; the peers you are connected to and also lets the remote peers know you are
; available so they can notify other peers they can to connect to you.  This
; option might be useful if you are having problems finding a node for some
; reason (perhaps due to a firewall).
;
; 'connect', on the other hand, will ONLY connect to the specified peers and
; no others.  It also disables listening (unless you explicitly set listen
; addresses via the 'listen' option) and DNS seeding, so you will not be
; advertised as an available peer to the peers you connect to and won't accept
; connections from any other peers.  So, the 'connect' option effectively allows
; you to only connect to "trusted" peers.
; ******************************************************************************

; Add persistent peers to connect to as desired.  One peer per line.
; You may specify each IP address with or without a port.  The default port will
; be added automatically if one is not specified here.
; addpeer=192.168.1.1
; addpeer=10.0.0.2:9108
; addpeer=fe80::1
; addpeer=[fe80::2]:9108

; Add persistent peers that you ONLY want to connect to as desired.  One peer
; per line.  You may specify each IP address with or without a port.  The
; default port will be added automatically if one is not specified here.
; NOTE: Specifying this option has other side effects as described above in
; the 'addpeer' versus 'connect' summary section.
; connect=192.168.1.1
; connect=10.0.0.2:9108
; connect=fe80::1
; connect=[fe80::2]:9108

; Maximum number of inbound and outbound peers.
; maxpeers=8

; Disable banning of misbehaving peers.
; nobanning=1

; Maximum allowed ban score before disconnecting and banning misbehaving peers.
; banthreshold=100

; How long to ban misbehaving peers. Valid time units are {s, m, h}.
; Minimum 1s.
; banduration=24h
; banduration=11h30m15s

; Add whitelisted IP networks and IPs. Connected peers whose IP matches a
; whitelist will not have their ban score increased.
; whitelist=127.0.0.1
; whitelist=::1
; whitelist=192.168.0.0/24
; whitelist=fd00::/16

; Disable DNS seeding for peers.  By default, when dcrd starts, it will use
; DNS to query for available peers to connect with.
; nodnsseed=1

; Specify the interfaces to listen on.  One listen address per line.
; NOTE: The default port is modified by some options such as 'testnet', so it is
; recommended to not specify a port and allow a proper default to be chosen
; unless you have a specific reason to do otherwise.
; All interfaces on default port (this is the default):
;  listen=
; All ipv4 interfaces on default port:
;  listen=0.0.0.0
; All ipv6 interfaces on default port:
;   listen=::
; All interfaces on port 9108:
;   listen=:9108
; All ipv4 interfaces on port 9108:
;   listen=0.0.0.0:9108
; All ipv6 interfaces on port 9108:
;   listen=[::]:9108
; Only ipv4 localhost on port 9108:
;   listen=127.0.0.1:9108
; Only ipv6 localhost on port 9108:
;   listen=[::1]:9108
; Only ipv4 localhost on non-standard port 8336:
;   listen=127.0.0.1:8336
; All interfaces on non-standard port 8336:
;   listen=:8336
; All ipv4 interfaces on non-standard port 8336:
;   listen=0.0.0.0:8336
; All ipv6 interfaces on non-standard port 8336:
;   listen=[::]:8336

; Disable listening for incoming connections.  This will override all listeners.
; nolisten=1

; Disable peer bloom filtering.  See BIP0111.
; nopeerbloomfilters=1


; ------------------------------------------------------------------------------
; RPC server options - The following options control the built-in RPC server
; which is used to control and query information from a running dcrd process.
;
; NOTE: The RPC server is disabled by default if no rpcuser or rpcpass is
; specified.
; ------------------------------------------------------------------------------

; Secure the RPC API by specifying the username and password.  You must specify
; both or the RPC server will be disabled.
; rpcuser=whatever_username_you_want
; rpcpass=

; Specify the interfaces for the RPC server listen on.  One listen address per
; line.  NOTE: The default port is modified by some options such as 'testnet',
; so it is recommended to not specify a port and allow a proper default to be
; chosen unless you have a specific reason to do otherwise.  By default, the
; RPC server will only listen on localhost for IPv4 and IPv6.
; All interfaces on default port:
;   rpclisten=
; All ipv4 interfaces on default port:
;   rpclisten=0.0.0.0
; All ipv6 interfaces on default port:
;   rpclisten=::
; All interfaces on port 9109:
;   rpclisten=:9109
; All ipv4 interfaces on port 9109:
;   rpclisten=0.0.0.0:9109
; All ipv6 interfaces on port 9109:
;   rpclisten=[::]:9109
; Only ipv4 localhost on port 9109:
;   rpclisten=127.0.0.1:9109
; Only ipv6 localhost on port 9109:
;   rpclisten=[::1]:9109
; Only ipv4 localhost on non-standard port 8337:
;   rpclisten=127.0.0.1:8337
; All interfaces on non-standard port 8337:
;   rpclisten=:8337
; All ipv4 interfaces on non-standard port 8337:
;   rpclisten=0.0.0.0:8337
; All ipv6 interfaces on non-standard port 8337:
;   rpclisten=[::]:8337

; Specify the maximum number of concurrent RPC clients for standard connections.
; rpcmaxclients=10

; Specify the maximum number of concurrent RPC websocket clients.
; rpcmaxwebsockets=25

; Use the following setting to disable the RPC server even if the rpcuser and
; rpcpass are specified above.  This allows one to quickly disable the RPC
; server without having to remove credentials from the config file.
; norpc=1



; ------------------------------------------------------------------------------
; Mempool Settings - The following options
; ------------------------------------------------------------------------------

; Set the minimum transaction fee to be considered a non-zero fee,
; minrelaytxfee=0.001

; Rate-limit free transactions to the value 15 * 1000 bytes per
; minute.
; limitfreerelay=15

; Require high priority for relaying free or low-fee transactions.
; norelaypriority=0

; Limit orphan transaction pool to 1000 transactions.
; maxorphantx=1000

; Do not accept transactions from remote peers.
; blocksonly=1

; Accept and relay non-standard transactions to the network regardless of the
; default network settings.
; acceptnonstd=1

; Reject non-standard transactions regardless of default network settings.
; rejectnonstd=1


; ------------------------------------------------------------------------------
; Optional Transaction Indexes
; ------------------------------------------------------------------------------

; Delete the entire address index on start up, then exit.
; dropaddrindex=0


; ------------------------------------------------------------------------------
; Optional Indexes
; ------------------------------------------------------------------------------

; Build and maintain a full hash-based transaction index which makes all
; transactions available via the getrawtransaction RPC.
; txindex=1

; Build and maintain a full address-based transaction index which makes the
; searchrawtransactions RPC available.
; addrindex=1


; ------------------------------------------------------------------------------
; Signature Verification Cache
; ------------------------------------------------------------------------------

; Limit the signature cache to a max of 50000 entries.
; sigcachemaxsize=50000


; ------------------------------------------------------------------------------
; Coin Generation (Mining) Settings - The following options control the
; generation of block templates used by external mining applications through RPC
; calls as well as the built-in CPU miner (if enabled).
; ------------------------------------------------------------------------------

; Enable built-in CPU mining.
;
; NOTE: This is typically only useful for testing purposes such as testnet or
; simnet since the difficulty on mainnet is far too high for CPU mining to be
; worth your while.
; generate=false

; Add addresses to pay mined blocks to for CPU mining and the block templates
; generated for the getwork RPC as desired.  One address per line.
; miningaddr=youraddress
; miningaddr=youraddress2
; miningaddr=youraddress3

; Specify the minimum block size in bytes to create.  By default, only
; transactions which have enough fees or a high enough priority will be included
; in generated block templates.  Specifying a minimum block size will instead
; attempt to fill generated block templates up with transactions until it is at
; least the specified number of bytes.
; blockminsize=0

; Specify the maximum block size in bytes to create.  This value will be limited
; to the consensus limit if it is larger than this value.
; blockmaxsize=750000

; Specify the size in bytes of the high-priority/low-fee area when creating a
; block.  Transactions which consist of large amounts, old inputs, and small
; sizes have the highest priority.  One consequence of this is that as low-fee
; or free transactions age, they raise in priority thereby making them more
; likely to be included in this section of a new block.  This value is limited
; by the blackmaxsize option and will be limited as needed.
; blockprioritysize=50000


; ------------------------------------------------------------------------------
; Debug
; ------------------------------------------------------------------------------

; Debug logging level.
; Valid levels are {trace, debug, info, warn, error, critical}
; You may also specify <subsystem>=<level>,<subsystem2>=<level>,... to set
; log level for individual subsystems.  Use dcrd --debuglevel=show to list
; available subsystems.
; debuglevel=info

; ------------------------------------------------------------------------------
; Profile - enable the HTTP profiler
; ------------------------------------------------------------------------------

; The profile server will be disabled if this option is not specified.  Profile
; information can be accessed at http://ipaddr:<profileport>/debug/pprof once
; running.  Note that the IP address will default to 127.0.0.1 if an IP address
; is not specified, so that the profiler is not accessible on the network.
; Listen on selected port on localhost only:
;   profile=6061
; Listen on selected port on all network interfaces:
;   profile=:6061
; Listen on single network ipv4 interface:
;   profile=192.168.1.123:6061
; Listen on ipv6 loopback interface:
;   profile=[::1]:6061
`
