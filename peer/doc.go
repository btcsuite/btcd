// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package peer provides a common base for creating and managing bitcoin network
peers for fully validating nodes, Simplified Payment Verification (SPV) nodes,
proxies, etc. It includes basic protocol exchanges like version negotiation,
responding to pings etc.

Inbound peers accept a connection and respond to the version message to begin
version negotiation.

Outbound peers connect and push the initial version message over a given
connection.

Both peers accept a configuration to customize options such as user agent,
service flag, protocol version, chain parameters, and proxy.

To extend the basic peer functionality provided by package peer, listeners can
be configured for all message types using callbacks in the peer configuration.
*/
package peer
