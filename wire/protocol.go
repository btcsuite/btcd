// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// InitialProcotolVersion is the initial protocol version for the
	// network.
	InitialProcotolVersion uint32 = 1

	// ProtocolVersion is the latest protocol version this package supports.
	ProtocolVersion uint32 = 5

	// BIP0111Version is the protocol version which added the SFNodeBloom
	// service flag.
	BIP0111Version uint32 = 2

	// SendHeadersVersion is the protocol version which added a new
	// sendheaders message.
	SendHeadersVersion uint32 = 3

	// MaxBlockSizeVersion is the protocol version which increased the
	// original blocksize.
	MaxBlockSizeVersion uint32 = 4

	// FeeFilterVersion is the protocol version which added a new
	// feefilter message.
	FeeFilterVersion uint32 = 5
)

// ServiceFlag identifies services supported by a decred peer.
type ServiceFlag uint64

const (
	// SFNodeNetwork is a flag used to indicate a peer is a full node.
	SFNodeNetwork ServiceFlag = 1 << iota

	// SFNodeBloom is a flag used to indiciate a peer supports bloom
	// filtering.
	SFNodeBloom
)

// Map of service flags back to their constant names for pretty printing.
var sfStrings = map[ServiceFlag]string{
	SFNodeNetwork: "SFNodeNetwork",
	SFNodeBloom:   "SFNodeBloom",
}

// orderedSFStrings is an ordered list of service flags from highest to
// lowest.
var orderedSFStrings = []ServiceFlag{
	SFNodeNetwork,
	SFNodeBloom,
}

// String returns the ServiceFlag in human-readable form.
func (f ServiceFlag) String() string {
	// No flags are set.
	if f == 0 {
		return "0x0"
	}

	// Add individual bit flags.
	s := ""
	for _, flag := range orderedSFStrings {
		if f&flag == flag {
			s += sfStrings[flag] + "|"
			f -= flag
		}
	}

	// Add any remaining flags which aren't accounted for as hex.
	s = strings.TrimRight(s, "|")
	if f != 0 {
		s += "|0x" + strconv.FormatUint(uint64(f), 16)
	}
	s = strings.TrimLeft(s, "|")
	return s
}

// CurrencyNet represents which decred network a message belongs to.
type CurrencyNet uint32

// Constants used to indicate the message decred network.  They can also be
// used to seek to the next message when a stream's state is unknown, but
// this package does not provide that functionality since it's generally a
// better idea to simply disconnect clients that are misbehaving over TCP.
const (
	// MainNet represents the main decred network.
	MainNet CurrencyNet = 0xd9b400f9

	// RegTest represents the regression test network.
	RegTest CurrencyNet = 0xdab500fa

	// TestNet2 represents the 2nd test network.
	TestNet2 CurrencyNet = 0x48e7a065

	// SimNet represents the simulation test network.
	SimNet CurrencyNet = 0x12141c16
)

// bnStrings is a map of decred networks back to their constant names for
// pretty printing.
var bnStrings = map[CurrencyNet]string{
	MainNet:  "MainNet",
	TestNet2: "TestNet2",
	RegTest:  "RegNet",
	SimNet:   "SimNet",
}

// String returns the CurrencyNet in human-readable form.
func (n CurrencyNet) String() string {
	if s, ok := bnStrings[n]; ok {
		return s
	}

	return fmt.Sprintf("Unknown CurrencyNet (%d)", uint32(n))
}
