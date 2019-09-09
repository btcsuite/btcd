# Configuring TOR

btcd provides full support for anonymous networking via the
[Tor Project](https://www.torproject.org/), including [client-only](#Client)
and [hidden service](#HiddenService) configurations along with
[stream isolation](#TorStreamIsolation).  In addition, btcd supports a hybrid,
[bridge mode](#Bridge) which is not anonymous, but allows it to operate as a
bridge between regular nodes and hidden service nodes without routing the
regular connections through Tor.

While it is easier to only run as a client, it is more beneficial to the Bitcoin
network to run as both a client and a server so others may connect to you to as
you are connecting to them.  We recommend you take the time to setup a Tor
hidden service for this reason.

## Client-only

Configuring btcd as a Tor client is straightforward.  The first step is
obviously to install Tor and ensure it is working. Once that is done, all that
typically needs to be done is to specify the `--proxy` flag via the btcd command
line or in the btcd configuration file.  Typically the Tor proxy address will be
127.0.0.1:9050 (if using standalone Tor) or 127.0.0.1:9150 (if using the Tor
Browser Bundle).  If you have Tor configured to require a username and password,
you may specify them with the `--proxyuser` and `--proxypass` flags.

By default, btcd assumes the proxy specified with `--proxy` is a Tor proxy and
hence will send all traffic, including DNS resolution requests, via the
specified proxy.

NOTE: Specifying the `--proxy` flag disables listening by default since you will
not be reachable for inbound connections unless you also configure a Tor
[hidden service](#HiddenService).

### Command line example

```bash
./btcd --proxy=127.0.0.1:9050
```

### Config file example

```text
[Application Options]

proxy=127.0.0.1:9050
```

## Client-server via Tor hidden service

The first step is to configure Tor to provide a hidden service.  Documentation
for this can be found on the Tor project website
[here](https://www.torproject.org/docs/tor-hidden-service.html.en).  However,
there is no need to install a web server locally as the linked instructions
discuss since btcd will act as the server.

In short, the instructions linked above entail modifying your `torrc` file to
add something similar to the following, restarting Tor, and opening the
`hostname` file in the `HiddenServiceDir` to obtain your hidden service .onion
address.

```text
HiddenServiceDir /var/tor/btcd
HiddenServicePort 8333 127.0.0.1:8333
```

Once Tor is configured to provide the hidden service and you have obtained your
generated .onion address, configuring btcd as a Tor hidden service requires
three flags:

* `--proxy` to identify the Tor (SOCKS 5) proxy to use for outgoing traffic.
  This is typically 127.0.0.1:9050.
* `--listen` to enable listening for inbound connections since `--proxy`
  disables listening by default
* `--externalip` to set the .onion address that is advertised to other peers

### Command line example

```bash
./btcd --proxy=127.0.0.1:9050 --listen=127.0.0.1 --externalip=fooanon.onion
```

### Config file example

```text
[Application Options]

proxy=127.0.0.1:9050
listen=127.0.0.1
externalip=fooanon.onion
```

## Bridge mode (not anonymous)

btcd provides support for operating as a bridge between regular nodes and hidden
service nodes.  In particular this means only traffic which is directed to or
from a .onion address is sent through Tor while other traffic is sent normally.
_As a result, this mode is **NOT** anonymous._

This mode works by specifying an onion-specific proxy, which is pointed at Tor,
by using the `--onion` flag via the btcd command line or in the btcd
configuration file.  If you have Tor configured to require a username and
password, you may specify them with the `--onionuser` and `--onionpass` flags.

NOTE: This mode will also work in conjunction with a hidden service which means
you could accept inbound connections both via the normal network and to your
hidden service through the Tor network.  To enable your hidden service in bridge
mode, you only need to specify your hidden service's .onion address via the
`--externalip` flag since traffic to and from .onion addresses are already
routed via Tor due to the `--onion` flag.

### Command line example

```bash
./btcd --onion=127.0.0.1:9050 --externalip=fooanon.onion
```

### Config file example

```text
[Application Options]

onion=127.0.0.1:9050
externalip=fooanon.onion
```

## Tor stream isolation

Tor stream isolation forces Tor to build a new circuit for each connection
making it harder to correlate connections.

btcd provides support for Tor stream isolation by using the `--torisolation`
flag.  This option requires --proxy or --onionproxy to be set.

### Command line example

```bash
./btcd --proxy=127.0.0.1:9050 --torisolation
```

### Config file example

```text
[Application Options]

proxy=127.0.0.1:9050
torisolation=1
```
