package wire

import (
	"bytes"
	"net"
	"testing"
	"time"
)

// TestAddrMessagesIgnoreTorV2 checks filtering without losing supported peers.
func TestAddrMessagesIgnoreTorV2(t *testing.T) {
	// Put a retired Tor v2 address between supported addresses on the wire.
	ip := net.ParseIP("8.8.8.8")
	onion := net.ParseIP("fd87:d87e:eb43:2020:2020:2020:2020:2020")
	legacy := []*NetAddress{
		NewNetAddressIPPort(ip, 8333, SFNodeNetwork),
		NewNetAddressIPPort(onion, 8333, SFNodeNetwork),
		NewNetAddressIPPort(
			net.ParseIP("1.1.1.1"), 8333, SFNodeNetwork,
		),
	}
	now := time.Unix(1700000000, 0)
	v2 := []*NetAddressV2{
		NetAddressV2FromBytes(now, SFNodeNetwork, ip, 8333),
		NetAddressV2FromBytes(now, SFNodeNetwork, onion, 8333),
		NetAddressV2FromBytes(
			now, SFNodeNetwork, bytes.Repeat([]byte{1}, TorV3Size),
			8333,
		),
		NetAddressV2FromBytes(
			now, SFNodeNetwork, net.ParseIP("1.1.1.1"), 8333,
		),
	}
	t.Run("legacy", func(t *testing.T) {
		var raw bytes.Buffer
		err := WriteVarInt(&raw, ProtocolVersion, uint64(len(legacy)))
		if err != nil {
			t.Fatal(err)
		}

		for _, addr := range legacy {
			err := writeNetAddress(
				&raw, ProtocolVersion, addr, true,
			)
			if err != nil {
				t.Fatal(err)
			}
		}

		// Decode supported peers and consume retired Tor v2 entries.
		var decoded MsgAddr
		err = decoded.BtcDecode(&raw, ProtocolVersion, BaseEncoding)
		if err != nil {
			t.Fatal(err)
		}

		if len(decoded.AddrList) != 2 || raw.Len() != 0 {
			t.Fatalf("decode retained Tor v2: %d addresses, "+
				"%d trailing bytes", len(decoded.AddrList),
				raw.Len())
		}

		// Encode the filtered count and preserve message framing.
		var encoded bytes.Buffer
		outgoing := &MsgAddr{AddrList: legacy}
		err = outgoing.BtcEncode(
			&encoded, ProtocolVersion, BaseEncoding,
		)
		if err != nil {
			t.Fatal(err)
		}

		if encoded.Bytes()[0] != 2 {
			t.Fatalf("encoded address count %d, want 2",
				encoded.Bytes()[0])
		}

		var roundTrip MsgAddr
		err = roundTrip.BtcDecode(
			&encoded, ProtocolVersion, BaseEncoding,
		)
		if err != nil {
			t.Fatal(err)
		}

		if len(roundTrip.AddrList) != 2 || encoded.Len() != 0 {
			t.Fatal("incorrect filtered payload")
		}
	})

	t.Run("addrv2", func(t *testing.T) {
		var raw bytes.Buffer
		err := WriteVarInt(&raw, ProtocolVersion, uint64(len(v2)))
		if err != nil {
			t.Fatal(err)
		}

		for _, addr := range v2 {
			err := writeNetAddressV2(&raw, ProtocolVersion, addr)
			if err != nil {
				t.Fatal(err)
			}
		}

		// Decode supported peers and consume retired Tor v2 entries.
		var decoded MsgAddrV2
		err = decoded.BtcDecode(&raw, ProtocolVersion, BaseEncoding)
		if err != nil {
			t.Fatal(err)
		}

		if len(decoded.AddrList) != 3 || raw.Len() != 0 {
			t.Fatalf("decode retained Tor v2: %d addresses, "+
				"%d trailing bytes", len(decoded.AddrList),
				raw.Len())
		}

		if !decoded.AddrList[1].IsTorV3() {
			t.Fatal("Tor v3 was not retained")
		}

		// Encode the filtered count and preserve message framing.
		var encoded bytes.Buffer
		outgoing := &MsgAddrV2{AddrList: v2}
		err = outgoing.BtcEncode(
			&encoded, ProtocolVersion, BaseEncoding,
		)
		if err != nil {
			t.Fatal(err)
		}

		if encoded.Bytes()[0] != 3 {
			t.Fatalf("encoded address count %d, want 3",
				encoded.Bytes()[0])
		}

		var roundTrip MsgAddrV2
		err = roundTrip.BtcDecode(
			&encoded, ProtocolVersion, BaseEncoding,
		)
		if err != nil {
			t.Fatal(err)
		}

		if len(roundTrip.AddrList) != 3 || encoded.Len() != 0 {
			t.Fatal("incorrect filtered payload")
		}
	})
}
