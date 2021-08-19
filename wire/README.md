wire
====

Package wire implements the bitcoin wire protocol.  A comprehensive suite of
tests with 100% test coverage is provided to ensure proper functionality.

There is an associated blog post about the release of this package
[here](https://blog.conformal.com/btcwire-the-bitcoin-wire-protocol-package-from-btcd/).

This package has been augmented from the original btcd implementation.
The block header was modified to contain the claimtrie hash.

## Bitcoin Message Overview

The bitcoin protocol consists of exchanging messages between peers. Each message
is preceded by a header which identifies information about it such as which
bitcoin network it is a part of, its type, how big it is, and a checksum to
verify validity. All encoding and decoding of message headers is handled by this
package.

To accomplish this, there is a generic interface for bitcoin messages named
`Message` which allows messages of any type to be read, written, or passed
around through channels, functions, etc. In addition, concrete implementations
of most of the currently supported bitcoin messages are provided. For these
supported messages, all of the details of marshalling and unmarshalling to and
from the wire using bitcoin encoding are handled so the caller doesn't have to
concern themselves with the specifics.

## Reading Messages Example

In order to unmarshal bitcoin messages from the wire, use the `ReadMessage`
function. It accepts any `io.Reader`, but typically this will be a `net.Conn`
to a remote node running a bitcoin peer.  Example syntax is:

```Go
	// Use the most recent protocol version supported by the package and the
	// main bitcoin network.
	pver := wire.ProtocolVersion
	btcnet := wire.MainNet

	// Reads and validates the next bitcoin message from conn using the
	// protocol version pver and the bitcoin network btcnet.  The returns
	// are a wire.Message, a []byte which contains the unmarshalled
	// raw payload, and a possible error.
	msg, rawPayload, err := wire.ReadMessage(conn, pver, btcnet)
	if err != nil {
		// Log and handle the error
	}
```

See the package documentation for details on determining the message type.

## Writing Messages Example

In order to marshal bitcoin messages to the wire, use the `WriteMessage`
function. It accepts any `io.Writer`, but typically this will be a `net.Conn`
to a remote node running a bitcoin peer. Example syntax to request addresses
from a remote peer is:

```Go
	// Use the most recent protocol version supported by the package and the
	// main bitcoin network.
	pver := wire.ProtocolVersion
	btcnet := wire.MainNet

	// Create a new getaddr bitcoin message.
	msg := wire.NewMsgGetAddr()

	// Writes a bitcoin message msg to conn using the protocol version
	// pver, and the bitcoin network btcnet.  The return is a possible
	// error.
	err := wire.WriteMessage(conn, msg, pver, btcnet)
	if err != nil {
		// Log and handle the error
	}
```